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
	Cwd     string
	Working bool
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
			out[t.Address] = AgentState{Cwd: t.Cwd, Working: t.Status.State == "working"}
		}
	}
	return out, nil
}

// AgentTracker turns snapshots into agent spans, and "unknown" spans while the stream is down.
type AgentTracker struct {
	open    map[string]*Span // by terminal address
	unknown *Span
}

// Feed applies a full snapshot and returns the spans it closed.
func (a *AgentTracker) Feed(states map[string]AgentState, at int64) []Span {
	var out []Span
	if a.open == nil {
		a.open = map[string]*Span{}
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
	for term, s := range states {
		if s.Working && a.open[term] == nil {
			a.open[term] = &Span{Kind: "agent", Start: at, Term: term, Cwd: s.Cwd}
		}
	}
	return out
}

// Lost closes every agent span and starts an agents_unknown one.
func (a *AgentTracker) Lost(at int64) []Span {
	var out []Span
	for term, o := range a.open {
		out = append(out, finish(*o, at)...)
		delete(a.open, term)
	}
	if a.unknown == nil {
		a.unknown = &Span{Kind: "agents_unknown", Start: at}
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
		cmd := hidden(exec.CommandContext(ctx, hcPath(), "events"))
		if out, err := cmd.StdoutPipe(); err == nil && cmd.Start() == nil {
			readEvents(out, emit)
			cmd.Wait()
		}
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
