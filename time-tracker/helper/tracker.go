package main

import (
	"path"
	"strings"
)

// Sample is one poll of the Windows session.
type Sample struct {
	At, LastInput int64
	Locked        bool
	Exe, Title    string
	Server        string   // Minecraft server host:port, from the game log
	CallExes      []string // exes currently holding the microphone
	SoundExes     []string // exes holding an active, audible render session
}

// Tracker turns samples into spans.
type Tracker struct {
	PollMs   int64
	CallApps []string
	Hidden   []string // patterns of exes and titles whose window title is not kept

	prev *Sample
	open map[string]*Span
}

// callExe returns the configured call app that is using the microphone.
func (t *Tracker) callExe(s Sample) string {
	for _, app := range t.CallApps {
		for _, e := range s.CallExes {
			if strings.EqualFold(app, e) {
				return app
			}
		}
	}
	return ""
}

// hiddenTitle tells whether a pattern matches the exe or the title, ignoring case; a bad pattern matches nothing.
func hiddenTitle(patterns []string, exe, title string) bool {
	exe, title = strings.ToLower(exe), strings.ToLower(title)
	for _, p := range patterns {
		p = strings.ToLower(p)
		if ok, err := path.Match(p, exe); ok && err == nil {
			return true
		}
		if ok, err := path.Match(p, title); ok && err == nil {
			return true
		}
	}
	return false
}

// soundExe returns the foreground exe when it is playing sound.
func soundExe(s Sample) string {
	for _, e := range s.SoundExes {
		if s.Exe != "" && strings.EqualFold(e, s.Exe) {
			return s.Exe
		}
	}
	return ""
}

// Feed consumes a sample and returns the spans it closed.
func (t *Tracker) Feed(s Sample) []Span {
	var out []Span
	if t.open == nil {
		t.open = map[string]*Span{}
	}
	lower := s.At - t.PollMs // lower bound for LastInput on the first sample
	if t.prev != nil {
		if t.slept(s.At) {
			out = append(out, t.Close(t.prev.At)...)
		} else {
			lower = t.prev.At
		}
	}
	// set keeps kind open iff want; a changed key (exe/title) closes and reopens.
	set := func(kind string, want bool, start, closeAt int64, exe, title, server string) {
		o := t.open[kind]
		if o != nil && (!want || o.Exe != exe || o.Title != title || o.Server != server) {
			out = append(out, finish(*o, closeAt)...)
			delete(t.open, kind)
			o = nil
		}
		if o == nil && want {
			t.open[kind] = &Span{Kind: kind, Start: start, Exe: exe, Title: title, Server: server}
		}
	}
	prevAt := s.At
	if t.prev != nil {
		prevAt = t.prev.At
	}
	set("power", true, s.At, s.At, "", "", "")
	hasInput := s.LastInput > lower && s.LastInput <= s.At
	if hasInput && t.open["input"] == nil {
		set("input", true, s.LastInput, s.At, "", "", "")
	} else if !hasInput {
		set("input", false, 0, prevAt, "", "", "")
	}
	set("lock", s.Locked, s.At, s.At, "", "", "")
	title := s.Title
	if hiddenTitle(t.Hidden, s.Exe, s.Title) {
		title = ""
	}
	set("window", s.Exe != "", s.At, s.At, s.Exe, title, s.Server)
	ce := t.callExe(s)
	set("call", ce != "", s.At, s.At, ce, "", "")
	se := soundExe(s)
	set("sound", se != "", s.At, s.At, se, "", "")
	t.prev = &s
	return out
}

// finish returns the span ended at end, or nothing if it is empty.
func finish(sp Span, end int64) []Span {
	if end <= sp.Start {
		return nil
	}
	sp.End = end
	return []Span{sp}
}

// slept tells a gap of more than three polls since the last one: sleep or shutdown.
func (t *Tracker) slept(at int64) bool { return t.prev != nil && at-t.prev.At > 3*t.PollMs }

// lastSeen is at, or the last poll when at lies past a sleep gap: open spans never cover the sleep.
func (t *Tracker) lastSeen(at int64) int64 {
	if t.slept(at) {
		return t.prev.At
	}
	return at
}

// Open returns the open spans with End = at (cut at the last poll after a sleep gap), for SaveOpen.
func (t *Tracker) Open(at int64) []Span {
	at = t.lastSeen(at)
	var out []Span
	for _, o := range t.open {
		sp := *o
		sp.End = at
		out = append(out, sp)
	}
	return out
}

// Close ends every open span at the given time.
func (t *Tracker) Close(at int64) []Span {
	var out []Span
	for _, o := range t.open {
		out = append(out, finish(*o, at)...)
	}
	t.open, t.prev = map[string]*Span{}, nil
	return out
}
