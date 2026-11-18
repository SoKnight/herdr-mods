package main

import (
	"fmt"
	"sort"
	"strings"
)

// beatTTL is how long a signal stays fresh unless a newer one from the same source replaces it.
const beatTTL = 120_000

type Interval struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

type Timeline struct {
	Power         []Interval `json:"power"`
	NoData        []Interval `json:"nodata"`
	AgentsUnknown []Interval `json:"agents_unknown"`
	Me            []Segment  `json:"me"`
	Leisure       []Segment  `json:"leisure"` // Досуг: matched by a leisure rule, not my time
	Agents        []AgentSeg `json:"agents"`
	Waits         []AgentSeg `json:"waits"` // agents waiting for me, in the present and clipped to power minus lock and nodata
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

// mergeSpans glues spans whose gap is shorter than gap. For input with gap = idle: such a pause still
// counts as presence, while a pause of idle or more is absence as a whole, from the last input on.
func mergeSpans(in []Span, gap int64) []Span {
	sort.Slice(in, func(a, b int) bool { return in[a].Start < in[b].Start })
	var out []Span
	for _, s := range in {
		if n := len(out); n > 0 && s.Start-out[n-1].End < gap {
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

// look gives what a window shows at t: its app rule, that app's fresh signal and the window to match.
func (b *beatState) look(rules *Rules, exe, title, server string, t int64) (*AppRule, *Heartbeat, window) {
	app := rules.App(exe)
	hb := b.fresh(app, t)
	w := window{exe: exe, title: title, server: server, at: t}
	if hb != nil {
		w.path = hb.ProjectPath
	}
	return app, hb, w
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

// Attribute turns raw data into my and the agents' timeline over [from, to), corrections applied.
// Data before from only feeds the project carry-over, so the caller passes raw starting at
// from - carry_minutes.
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
	inputs := byKind["input"]
	if s.VideoActivity {
		// Video with sound in the foreground counts as being here, like input.
		inputs = append(append([]Span(nil), inputs...), byKind["sound"]...)
	}
	input := newCursor(mergeSpans(inputs, idle))
	// Presence is asked only while the computer is on and the collector saw it: a sleep or a nodata
	// stretch is absence, so input on both sides of it is never glued across.
	power, nodata := newCursor(mergeSpans(byKind["power"], 1)), newCursor(mergeSpans(byKind["nodata"], 1))

	var tl Timeline
	var lastProject, lastWhy string // last project found by a rule while I was here
	var lastEnd int64
	for k := 0; k+1 < len(cuts) && cuts[k+1] <= to; k++ {
		for t := cuts[k]; t < cuts[k+1]; {
			end := cuts[k+1]
			beats.advance(t)
			call := calls.at(t)
			inCall := call != nil && isCallApp(rules, call.Exe)
			if power.at(t) == nil || nodata.at(t) != nil || locks.at(t) != nil || (input.at(t) == nil && !inCall) {
				t = end
				continue
			}

			seg := Segment{Start: t}
			if w := windows.at(t); w != nil {
				seg.App, seg.Title, seg.Server = w.Exe, w.Title, w.Server
			}
			app, hb, w := beats.look(rules, seg.App, seg.Title, seg.Server, t)
			var callWhy string
			if inCall && !strings.EqualFold(seg.App, call.Exe) {
				// A call in the background still is the call: attribute it, not the focused window,
				// unless that window is Досуг.
				if r, ok := rules.Match(w); !ok || r.Project != leisureID {
					seg.App, seg.Title, seg.Server = call.Exe, call.Title, ""
					callWhy = "звонок " + call.Exe + " в фоне; "
					app, hb, w = beats.look(rules, seg.App, seg.Title, seg.Server, t)
				}
			}

			project, why, rule := projectOf(rules, w, hb)
			switch {
			case project == excludeID || project == leisureID:
				seg.Rule = rule // not my time, and nothing to carry on from
			case project != "":
				lastProject, lastWhy, lastEnd = project, why, end
				seg.Rule = rule
			case lastProject != "" && t < lastEnd+carry:
				project = lastProject
				why = fmt.Sprintf("перенос до %d мин: %s", s.CarryMinutes, lastWhy)
				end = min(end, lastEnd+carry)
			default:
				why = "без проекта"
			}
			activity, actWhy := activityOf(rules, app, hb)

			seg.End, seg.Project, seg.Activity = end, project, activity
			if hb != nil {
				seg.Path = hb.ProjectPath
			}
			seg.Why = callWhy + why + "; " + actWhy
			if t >= from {
				tl.Me = appendSegment(tl.Me, seg)
			}
			t = end
		}
	}

	tl.Power = clipMerge(byKind["power"], from, to)
	tl.NoData = clipMerge(byKind["nodata"], from, to)
	tl.AgentsUnknown = clipMerge(byKind["agents_unknown"], from, to)

	agents := agentPieces(rules, byKind["agent"], from, to)
	fixes := append([]Correction(nil), corr...)
	sort.SliceStable(fixes, func(a, b int) bool { return fixes[a].ID < fixes[b].ID })
	for _, c := range fixes { // a later correction works on what the earlier ones produced
		tl.Me = correctMe(tl.Me, c, from, to)
		agents = correctAgents(agents, c)
	}
	// Не работа and Досуг kept their target through the corrections, so a correction can still claim them.
	var me []Segment
	for _, seg := range tl.Me {
		switch seg.Project {
		case excludeID:
		case leisureID:
			tl.Leisure = append(tl.Leisure, seg)
		default:
			me = append(me, seg)
		}
	}
	tl.Me = me
	tl.Agents = unionAgents(agents)
	tl.Waits = waitPieces(rules, byKind["wait"], mergeSpans(byKind["power"], 1),
		append(append([]Span(nil), byKind["lock"]...), byKind["nodata"]...), from, to)
	return tl
}

// waitPieces gives each wait its folder's project like agentPieces, keeps what lies inside power and
// outside lock and nodata, and unions it. Corrections do not touch waits: waiting is not money.
func waitPieces(rules *Rules, spans, power, absent []Span, from, to int64) []AgentSeg {
	var out []AgentSeg
	for _, p := range agentPieces(rules, spans, from, to) {
		for _, pw := range power {
			if s, e := max(p.Start, pw.Start), min(p.End, pw.End); s < e {
				q := p
				q.Start, q.End = s, e
				out = append(out, q)
			}
		}
	}
	for _, ab := range absent {
		var kept []AgentSeg
		for _, p := range out {
			splitBy(p.Start, p.End, ab.Start, ab.End, func(s, e int64, inside bool) {
				if !inside {
					q := p
					q.Start, q.End = s, e
					kept = append(kept, q)
				}
			})
		}
		out = kept
	}
	return unionAgents(out)
}

func clipMerge(spans []Span, from, to int64) []Interval {
	var out []Interval
	for _, sp := range mergeSpans(spans, 1) {
		if s, e := max(sp.Start, from), min(sp.End, to); s < e {
			out = append(out, Interval{s, e})
		}
	}
	return out
}

// splitBy cuts [s, e) at the bounds of [a, b) and hands each non-empty piece to f.
func splitBy(s, e, a, b int64, f func(s, e int64, inside bool)) {
	lo, hi := max(s, a), min(e, b)
	if lo >= hi {
		f(s, e, false)
		return
	}
	if s < lo {
		f(s, lo, false)
	}
	f(lo, hi, true)
	if hi < e {
		f(hi, e, false)
	}
}

func targets(c Correction, who string) bool { return c.Target == who || c.Target == "both" }

// fixedWhy puts the correction in front of the reason; the UI finds "#<ID>" there to offer undo.
func fixedWhy(c Correction, what, why string) string {
	return fmt.Sprintf("правка #%d: %s; %s", c.ID, what, why)
}

// correctMe applies one correction to my present time; activity, note and add touch only my time.
func correctMe(me []Segment, c Correction, from, to int64) []Segment {
	var what string
	switch {
	case c.Kind == "project" && targets(c, "me"):
		what = "проект → " + c.Project
	case c.Kind == "activity":
		what = "вид → " + c.Activity
	case c.Kind == "note":
		what = "заметка"
	case c.Kind == "exclude" && targets(c, "me"), c.Kind == "add":
	default:
		return me
	}
	var out []Segment
	for _, seg := range me {
		splitBy(seg.Start, seg.End, c.Start, c.End, func(s, e int64, inside bool) {
			p := seg
			p.Start, p.End = s, e
			if inside {
				switch c.Kind {
				case "exclude", "add":
					return
				case "project":
					p.Project = c.Project
				case "activity":
					p.Activity = c.Activity
				case "note":
					p.Note = c.Note
				}
				p.Why, p.Corrected = fixedWhy(c, what, p.Why), true
			}
			out = append(out, p)
		})
	}
	if s, e := max(c.Start, from), min(c.End, to); c.Kind == "add" && s < e {
		// Manual time replaces whatever it overlaps, so nothing counts twice.
		out = append(out, Segment{Start: s, End: e, Project: c.Project, Activity: c.Activity, Note: c.Note,
			Why: fmt.Sprintf("правка #%d: добавлено вручную", c.ID), Corrected: true, Manual: true})
		sort.SliceStable(out, func(a, b int) bool { return out[a].Start < out[b].Start })
	}
	return out
}

// agentPieces gives each agent's working interval its folder's project, before corrections and union.
func agentPieces(rules *Rules, spans []Span, from, to int64) []AgentSeg {
	var out []AgentSeg
	for _, sp := range spans {
		s, e := max(sp.Start, from), min(sp.End, to)
		if s >= e {
			continue
		}
		id, why := rules.FolderProject(sp.Cwd, sp.Start)
		if id == excludeID || id == leisureID {
			continue
		}
		if id == "" {
			why = "папка " + sp.Cwd + " → без проекта"
		}
		out = append(out, AgentSeg{Start: s, End: e, Project: id, Why: "агент " + sp.Term + ", " + why})
	}
	return out
}

func correctAgents(agents []AgentSeg, c Correction) []AgentSeg {
	if (c.Kind != "project" && c.Kind != "exclude") || !targets(c, "agents") {
		return agents
	}
	var out []AgentSeg
	for _, a := range agents {
		splitBy(a.Start, a.End, c.Start, c.End, func(s, e int64, inside bool) {
			p := a
			p.Start, p.End = s, e
			if inside {
				if c.Kind == "exclude" {
					return
				}
				p.Project = c.Project
				p.Why, p.Corrected = fixedWhy(c, "проект → "+c.Project, p.Why), true
			}
			out = append(out, p)
		})
	}
	return out
}

// unionAgents counts a project's time once while at least one of its agents works.
func unionAgents(pieces []AgentSeg) []AgentSeg {
	sort.SliceStable(pieces, func(a, b int) bool {
		if pieces[a].Project != pieces[b].Project {
			return pieces[a].Project < pieces[b].Project
		}
		return pieces[a].Start < pieces[b].Start
	})
	var out []AgentSeg
	for _, x := range pieces {
		if n := len(out); n > 0 && out[n-1].Project == x.Project && x.Start <= out[n-1].End {
			p := &out[n-1]
			p.End = max(p.End, x.End)
			p.Corrected = p.Corrected || x.Corrected
			if !strings.Contains(p.Why, x.Why) {
				p.Why += "; " + x.Why
			}
			continue
		}
		out = append(out, x)
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Start < out[b].Start })
	return out
}

// projectOf applies the rules after manual corrections: the first rule that matches the window (its
// folder comes from the focused app's fresh signal), then the signal's project name. Carry-over is the caller's.
func projectOf(rules *Rules, w window, hb *Heartbeat) (id, why string, rule int64) {
	if r, ok := rules.Match(w); ok {
		why = r.why()
		if r.Kind == "folder" && hb != nil {
			why = "сигнал " + hb.Source + ": " + why
		}
		return r.Project, why, r.ID
	}
	if hb != nil {
		if id, why = rules.ProjectByName(hb.ProjectName); id != "" {
			return id, "сигнал " + hb.Source + ": " + why, 0
		}
	}
	return "", "", 0
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
		if p.End == seg.Start && p.App == seg.App && p.Title == seg.Title && p.Server == seg.Server && p.Path == seg.Path &&
			p.Project == seg.Project && p.Activity == seg.Activity && p.Why == seg.Why && p.Rule == seg.Rule {
			p.End = seg.End
			return me
		}
	}
	return append(me, seg)
}
