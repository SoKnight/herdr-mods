package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// AgentState is one live agent terminal as `hc events` last told it.
type AgentState struct {
	Cwd              string
	Working, Waiting bool  // Waiting: blocked or done, i.e. waiting for me
	Since            int64 // when the terminal got its state: status_ms in a snapshot, the receive time of a delta
}

type hcStatus struct {
	App   string `json:"app"`
	State string `json:"state"`
}

// hcTerminal is one terminal of an `hc events` snapshot. cwd is the launch folder: it never changes.
type hcTerminal struct {
	Address  string    `json:"address"`
	Cwd      string    `json:"cwd"`
	State    string    `json:"state"` // live or finished
	Status   *hcStatus `json:"status"`
	StatusMs int64     `json:"status_ms"`
}

// hcLine is one line of `hc events`: a bare snapshot first, then JSON-RPC notifications: method
// event (params.type status, created, lost, restarted, keep, finished, removed, turn, title, labels,
// size, doc), heartbeat, and missed with a fresh snapshot for a subscriber that fell behind.
type hcLine struct {
	Method    string        `json:"method"`
	Seq       int64         `json:"seq"`
	Terminals *[]hcTerminal `json:"terminals"`
	Params    struct {
		Type      string        `json:"type"`
		Terminal  string        `json:"terminal"`
		Seq       int64         `json:"seq"`
		Terminals *[]hcTerminal `json:"terminals"`
		Data      struct {
			Status *hcStatus `json:"status"`
		} `json:"data"`
	} `json:"params"`
}

// hcTerminals are the terminals as the stream last told them: the last snapshot with the deltas since.
type hcTerminals struct {
	seq   int64
	terms map[string]hcTerminal
}

// apply takes one line received at now and reports whether the terminals changed. A line that is not
// JSON is skipped. errResnapshot: an event the map cannot follow (a new terminal, a restarted core).
func (h *hcTerminals) apply(line []byte, now int64) (bool, error) {
	var ev hcLine
	if json.Unmarshal(line, &ev) != nil {
		return false, nil
	}
	snap, seq := ev.Terminals, ev.Seq
	if ev.Params.Terminals != nil {
		snap, seq = ev.Params.Terminals, ev.Params.Seq
	}
	if snap != nil {
		h.seq, h.terms = seq, map[string]hcTerminal{}
		for _, t := range *snap {
			h.terms[t.Address] = t
		}
		return true, nil
	}
	if ev.Method == "missed" {
		return false, errResnapshot
	}
	// Heartbeats, events before any snapshot and the events a missed snapshot already has.
	if ev.Method != "event" || h.terms == nil || ev.Params.Seq <= h.seq {
		return false, nil
	}
	p := ev.Params
	t, known := h.terms[p.Terminal]
	switch p.Type {
	case "status":
		if !known {
			return false, errResnapshot
		}
		if old, st := t.Status, p.Data.Status; old == nil || st == nil || old.State != st.State {
			t.StatusMs = now // a new msg in the same state keeps its time: a wait's cap counts from its start
		}
		t.Status = p.Data.Status
	case "finished":
		t.State = "finished"
	case "removed":
		delete(h.terms, p.Terminal)
		return known, nil
	case "created", "lost", "restarted":
		return false, errResnapshot
	default:
		return false, nil
	}
	if known {
		h.terms[p.Terminal] = t
	}
	return known, nil
}

// agents are the live terminals running an agent.
func (h *hcTerminals) agents() map[string]AgentState {
	out := map[string]AgentState{}
	for addr, t := range h.terms {
		if t.State == "live" && t.Status != nil && t.Status.App != "" {
			st := t.Status.State
			out[addr] = AgentState{Cwd: t.Cwd, Working: st == "working", Waiting: st == "blocked", Since: t.StatusMs}
		}
	}
	return out
}

// waitCap is how long a terminal's wait counts: a session's idle limit.
func waitCap(s Settings) int64 { return int64(s.SessionIdleMinutes) * 60_000 }

// openWait is an open wait span and when its terminal began waiting.
type openWait struct {
	sp    Span
	since int64
}

// AgentTracker turns snapshots into agent and wait spans, and "unknown" spans while the stream is down.
// A wait lasts at most Cap from its terminal's Since: a terminal left "blocked" for days is not waiting for me.
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

// readEvents follows the snapshot and its deltas in r, emitting the agents on every change; bad lines
// are skipped. It stops with errResnapshot on an event it cannot follow.
func readEvents(r io.Reader, emit func(map[string]AgentState)) error {
	var h hcTerminals
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 8<<20)
	for sc.Scan() {
		changed, err := h.apply(sc.Bytes(), nowMs())
		if err != nil {
			return err
		}
		if changed {
			emit(h.agents())
		}
	}
	return sc.Err()
}

var errResnapshot = errors.New("hc events: an event the terminals map cannot follow")

func hcPath() string {
	if p, err := exec.LookPath("hc"); err == nil {
		return p
	}
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "Programs", "herdr-core", "hc.exe")
}

// streamAgents follows `hc events` until ctx is done, calling lost() and retrying after 10s on any break.
// An event the map cannot follow restarts hc after 1s for a new snapshot, quietly: the agents were not lost.
func streamAgents(ctx context.Context, emit func(map[string]AgentState), lost func()) {
	for ctx.Err() == nil {
		run, stop := context.WithCancel(ctx)
		cmd := hidden(exec.CommandContext(run, hcPath(), "events"))
		quiet := false
		if out, err := cmd.StdoutPipe(); err == nil && cmd.Start() == nil {
			quiet = errors.Is(readEvents(out, emit), errResnapshot)
			stop() // kill hc so Wait cannot block on a full pipe after a scan error
			cmd.Wait()
		}
		stop()
		if ctx.Err() != nil {
			return
		}
		wait := time.Second
		if !quiet {
			lost()
			wait = 10 * time.Second
		}
		select {
		case <-ctx.Done():
		case <-time.After(wait):
		}
	}
}
