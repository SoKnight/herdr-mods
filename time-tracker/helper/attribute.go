package main

import (
	"fmt"
	"sort"
	"strings"
)

// beatTTL is how long a signal stays fresh unless a newer one from the same source replaces it.
const beatTTL = 120_000

type Interval struct{ Start, End int64 }

type Timeline struct {
	Power, NoData, AgentsUnknown []Interval
	Me                           []Segment
	Agents                       []AgentSeg
}

// cursor finds the span covering t among spans sorted by Start, for t that never decreases.
type cursor struct {
	spans []Span
	i     int
}

func newCursor(spans []Span) *cursor {
	sort.SliceStable(spans, func(a, b int) bool { return spans[a].Start < spans[b].Start })
	return &cursor{spans: spans}
}

func (c *cursor) at(t int64) *Span {
	for c.i < len(c.spans) && c.spans[c.i].End <= t {
		c.i++
	}
	if c.i < len(c.spans) && c.spans[c.i].Start <= t {
		return &c.spans[c.i]
	}
	return nil
}

// mergeInput glues input spans whose gap is shorter than idle: such a pause still counts as presence,
// while a gap of idle or more is absence as a whole, from the last input on.
func mergeInput(in []Span, idle int64) []Span {
	sort.Slice(in, func(a, b int) bool { return in[a].Start < in[b].Start })
	var out []Span
	for _, s := range in {
		if n := len(out); n > 0 && s.Start-out[n-1].End < idle {
			out[n-1].End = max(out[n-1].End, s.End)
			continue
		}
		out = append(out, s)
	}
	return out
}

// beatState keeps the latest signal per source up to the current moment.
type beatState struct {
	all  []Heartbeat
	i    int
	last map[string]Heartbeat
}

func (b *beatState) advance(t int64) {
	for b.i < len(b.all) && b.all[b.i].Time <= t {
		b.last[b.all[b.i].Source] = b.all[b.i]
		b.i++
	}
}

// fresh returns the live signal of the app's source, or nil.
func (b *beatState) fresh(app *AppRule, t int64) *Heartbeat {
	if app == nil || app.Source == "" {
		return nil
	}
	if h, ok := b.last[app.Source]; ok && t < h.Time+beatTTL {
		return &h
	}
	return nil
}

func isCallApp(rules *Rules, exe string) bool {
	app := rules.App(exe)
	return app != nil && app.Presence == "call"
}

// Attribute turns raw data into my timeline over [from, to). Data before from only feeds the
// project carry-over, so the caller passes raw starting at from - carry_minutes.
// Corrections are applied by a later layer; corr is unused for now.
func Attribute(raw Raw, rules *Rules, corr []Correction, s Settings, from, to int64) Timeline {
	idle := int64(s.IdleMinutes) * 60_000
	carry := int64(s.CarryMinutes) * 60_000

	byKind := map[string][]Span{}
	cuts := []int64{from, to}
	for _, sp := range raw.Spans {
		byKind[sp.Kind] = append(byKind[sp.Kind], sp)
		cuts = append(cuts, sp.Start, sp.End)
	}
	beats := beatState{all: append([]Heartbeat(nil), raw.Beats...), last: map[string]Heartbeat{}}
	sort.SliceStable(beats.all, func(a, b int) bool { return beats.all[a].Time < beats.all[b].Time })
	for _, b := range beats.all {
		cuts = append(cuts, b.Time, b.Time+beatTTL)
	}
	sort.Slice(cuts, func(a, b int) bool { return cuts[a] < cuts[b] })

	windows, locks, calls := newCursor(byKind["window"]), newCursor(byKind["lock"]), newCursor(byKind["call"])
	input := newCursor(mergeInput(byKind["input"], idle))

	var tl Timeline
	var lastProject, lastWhy string // last project found by a rule while I was here
	var lastEnd int64
	for k := 0; k+1 < len(cuts) && cuts[k+1] <= to; k++ {
		for t := cuts[k]; t < cuts[k+1]; {
			end := cuts[k+1]
			beats.advance(t)
			call := calls.at(t)
			inCall := call != nil && isCallApp(rules, call.Exe)
			if locks.at(t) != nil || (input.at(t) == nil && !inCall) {
				t = end
				continue
			}

			seg := Segment{Start: t}
			if w := windows.at(t); w != nil {
				seg.App, seg.Title = w.Exe, w.Title
			}
			var callWhy string
			if inCall && !strings.EqualFold(seg.App, call.Exe) {
				// A call in the background still is the call: attribute it, not the focused window.
				seg.App, seg.Title = call.Exe, call.Title
				callWhy = "звонок " + call.Exe + " в фоне; "
			}
			app := rules.App(seg.App)
			hb := beats.fresh(app, t)

			project, why := projectOf(rules, seg.App, seg.Title, hb)
			switch {
			case project != "":
				lastProject, lastWhy, lastEnd = project, why, end
			case lastProject != "" && t < lastEnd+carry:
				project = lastProject
				why = fmt.Sprintf("перенос до %d мин: %s", s.CarryMinutes, lastWhy)
				end = min(end, lastEnd+carry)
			default:
				why = "без проекта"
			}
			activity, actWhy := activityOf(rules, app, hb)

			seg.End, seg.Project, seg.Activity = end, project, activity
			seg.Why = callWhy + why + "; " + actWhy
			if t >= from {
				tl.Me = appendSegment(tl.Me, seg)
			}
			t = end
		}
	}
	return tl
}

// projectOf applies the rules after manual corrections: the focused app's own fresh signal,
// then the app rule, then the window title. Carry-over is the caller's.
func projectOf(rules *Rules, exe, title string, hb *Heartbeat) (id, why string) {
	if hb != nil {
		id, why = rules.ProjectByPath(hb.ProjectPath)
		if id == "" && hb.URL != "" {
			id, why = rules.ProjectByURL(hb.URL)
		}
		if id == "" {
			id, why = rules.ProjectByName(hb.ProjectName)
		}
		if id != "" {
			return id, "сигнал " + hb.Source + ": " + why
		}
	}
	if id, why = rules.ProjectByApp(exe); id != "" {
		return id, why
	}
	return rules.ProjectByTitle(exe, title)
}

func activityOf(rules *Rules, app *AppRule, hb *Heartbeat) (activity, why string) {
	if hb != nil && hb.Activity != "" {
		return hb.Activity, "вид: сигнал " + hb.Source + " → " + hb.Activity
	}
	if hb != nil && hb.Entity != "" {
		if a := rules.ActivityByFile(hb.Entity); a != "" {
			return a, "вид: файл " + hb.Entity + " → " + a
		}
	}
	if app != nil && app.Activity != "" {
		return app.Activity, "вид: приложение " + app.Exe + " → " + app.Activity
	}
	return "", "вид не определён"
}

// appendSegment extends the previous segment when the new one continues it with the same attribution.
func appendSegment(me []Segment, seg Segment) []Segment {
	if n := len(me); n > 0 {
		p := &me[n-1]
		if p.End == seg.Start && p.App == seg.App && p.Title == seg.Title &&
			p.Project == seg.Project && p.Activity == seg.Activity && p.Why == seg.Why {
			p.End = seg.End
			return me
		}
	}
	return append(me, seg)
}
