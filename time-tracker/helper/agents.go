package main

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// AgentState is one live agent terminal as seen in an `hc events` snapshot.
type AgentState struct {
	Cwd              string
	Working, Waiting bool  // Waiting: blocked or done, i.e. waiting for me
	Since            int64 // status_ms: when the terminal got its state
}

// parseEvent extracts live agent terminals from one `hc events` line.
// Lines without a terminals list yield nil, nil.
func parseEvent(line []byte) (map[string]AgentState, error) {
	var ev struct {
		Terminals *[]struct {
			Address string `json:"address"`
			Cwd     string `json:"cwd"`
			State   string `json:"state"`
			Status  *struct {
				App   string `json:"app"`
				State string `json:"state"`
			} `json:"status"`
			StatusMs int64 `json:"status_ms"`
		} `json:"terminals"`
	}
	if err := json.Unmarshal(line, &ev); err != nil {
		return nil, err
	}
	if ev.Terminals == nil {
		return nil, nil
	}
	out := map[string]AgentState{}
	for _, t := range *ev.Terminals {
		if t.State == "live" && t.Status != nil && t.Status.App != "" {
			st := t.Status.State
			out[t.Address] = AgentState{Cwd: t.Cwd, Working: st == "working", Waiting: st == "blocked" || st == "done", Since: t.StatusMs}
		}
	}
	return out, nil
}

// waitCap is how long a terminal's wait counts: a session's idle limit.
func waitCap(s Settings) int64 { return int64(s.SessionIdleMinutes) * 60_000 }

// openWait is an open wait span and when its terminal began waiting.
type openWait struct {
	sp    Span
	since int64
}

// AgentTracker turns snapshots into agent and wait spans, and "unknown" spans while the stream is down.
// A wait lasts at most Cap from its terminal's Since: a terminal left "done" for days is not waiting for me.
type AgentTracker struct {
	Cap     int64
	open    map[string]*Span // by terminal address
	waiting map[string]*openWait
	unknown *Span
}

// Feed applies a full snapshot and returns the spans it closed.
func (a *AgentTracker) Feed(states map[string]AgentState, at int64) []Span {
	var out []Span
	if a.open == nil {
		a.open, a.waiting = map[string]*Span{}, map[string]*openWait{}
	}
	if a.unknown != nil {
		out = append(out, finish(*a.unknown, at)...)
		a.unknown = nil
	}
	for term, o := range a.open {
		if s, ok := states[term]; !ok || !s.Working || s.Cwd != o.Cwd {
			out = append(out, finish(*o, at)...)
			delete(a.open, term)
		}
	}
	for term, w := range a.waiting {
		if s, ok := states[term]; !ok || !s.Waiting || s.Cwd != w.sp.Cwd || at-w.since >= a.Cap {
			out = append(out, finish(w.sp, min(at, w.since+a.Cap))...)
			delete(a.waiting, term)
		}
	}
	for term, s := range states {
		if s.Working && a.open[term] == nil {
			a.open[term] = &Span{Kind: "agent", Start: at, Term: term, Cwd: s.Cwd}
		}
		if s.Waiting && a.waiting[term] == nil && at-s.Since < a.Cap {
			a.waiting[term] = &openWait{Span{Kind: "wait", Start: max(s.Since, at-a.Cap), Term: term, Cwd: s.Cwd}, s.Since}
		}
	}
	return out
}

// Expire closes the waits that are past Since+Cap, ending them there, without waiting for a snapshot.
func (a *AgentTracker) Expire(at int64) []Span {
	var out []Span
	for term, w := range a.waiting {
		if at-w.since >= a.Cap {
			out = append(out, finish(w.sp, w.since+a.Cap)...)
			delete(a.waiting, term)
		}
	}
	return out
}

// Lost closes every agent and wait span and starts an agents_unknown one.
func (a *AgentTracker) Lost(at int64) []Span {
	var out []Span
	for term, o := range a.open {
		out = append(out, finish(*o, at)...)
		delete(a.open, term)
	}
	for term, w := range a.waiting {
		out = append(out, finish(w.sp, min(at, w.since+a.Cap))...)
		delete(a.waiting, term)
	}
	if a.unknown == nil {
		a.unknown = &Span{Kind: "agents_unknown", Start: at}
	}
	return out
}

// Cut ends every open span at at and reopens it at resume: the computer slept in between.
func (a *AgentTracker) Cut(at, resume int64) []Span {
	var out []Span
	for _, o := range a.open {
		out = append(out, finish(*o, at)...)
		o.Start = resume
	}
	for term, w := range a.waiting {
		out = append(out, finish(w.sp, min(at, w.since+a.Cap))...)
		w.sp.Start = resume
		if resume >= w.since+a.Cap {
			delete(a.waiting, term)
		}
	}
	if a.unknown != nil {
		out = append(out, finish(*a.unknown, at)...)
		a.unknown.Start = resume
	}
	return out
}

// Open returns the open spans with End = at, for SaveOpen.
func (a *AgentTracker) Open(at int64) []Span {
	var out []Span
	for _, o := range a.open {
		sp := *o
		sp.End = at
		out = append(out, sp)
	}
	for _, w := range a.waiting {
		if sp := w.sp; w.since+a.Cap > sp.Start {
			sp.End = min(at, w.since+a.Cap)
			out = append(out, sp)
		}
	}
	if a.unknown != nil {
		sp := *a.unknown
		sp.End = at
		out = append(out, sp)
	}
	return out
}

// readEvents emits every snapshot from r; bad lines are skipped.
func readEvents(r io.Reader, emit func(map[string]AgentState)) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 8<<20)
	for sc.Scan() {
		if states, err := parseEvent(sc.Bytes()); err == nil && states != nil {
			emit(states)
		}
	}
	return sc.Err()
}

func hcPath() string {
	if p, err := exec.LookPath("hc"); err == nil {
		return p
	}
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "herdr-core", "hc.exe")
}

// streamAgents follows `hc events` until ctx is done, calling lost() and retrying after 10s on any break.
func streamAgents(ctx context.Context, emit func(map[string]AgentState), lost func()) {
	for ctx.Err() == nil {
		run, stop := context.WithCancel(ctx)
		cmd := hidden(exec.CommandContext(run, hcPath(), "events"))
		if out, err := cmd.StdoutPipe(); err == nil && cmd.Start() == nil {
			readEvents(out, emit)
			stop() // kill hc so Wait cannot block on a full pipe after a scan error
			cmd.Wait()
		}
		stop()
		if ctx.Err() != nil {
			return
		}
		lost()
		select {
		case <-ctx.Done():
		case <-time.After(10 * time.Second):
		}
	}
}
