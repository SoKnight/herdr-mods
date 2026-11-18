package main

import (
	"maps"
	"slices"
	"sort"
	"time"
)

type DayTotal struct {
	Me         int64            `json:"me"`
	Agents     int64            `json:"agents"`
	Activities map[string]int64 `json:"activities"`
}

type ProjectTotal struct {
	Project    string              `json:"project"`
	Me         int64               `json:"me"`
	Agents     int64               `json:"agents"`
	Activities map[string]int64    `json:"activities"`
	Days       map[string]DayTotal `json:"days"`
}

type Report struct {
	Uptime   int64          `json:"uptime"`
	Me       int64          `json:"me"`
	Leisure  int64          `json:"leisure"` // Досуг, apart from my time
	Agents   int64          `json:"agents"`
	Waiting  int64          `json:"waiting"` // agents waiting for me, apart from every other total
	Projects []ProjectTotal `json:"projects"`
}

// Divergence says a closed period no longer matches what fresh rules would give.
type Divergence struct {
	Period  int64  `json:"period"`
	Month   string `json:"month,omitempty"` // a month record's
	Project string `json:"project"`
	DeltaMs int64  `json:"delta_ms"`
}

// RoundMinutes rounds ms to the nearest multiple of step minutes, exactly half up.
func RoundMinutes(ms int64, step int) int64 {
	unit := int64(step) * 60_000
	return (ms + unit/2) / unit * int64(step)
}

// nextDayStart is the first instant after ms at which the next day begins, shifted the way DayOf shifts.
func nextDayStart(s Settings, ms int64) int64 {
	loc, shift := s.dayClock()
	y, mo, d := time.UnixMilli(ms).In(loc).Add(-shift).Date()
	next := time.Date(y, mo, d+1, 0, 0, 0, 0, loc).Add(shift).UnixMilli()
	if next <= ms { // never stall
		next = ms + 1
	}
	return next
}

// byDay hands each part of [start, end) that lies in one day to f.
func byDay(s Settings, start, end int64, f func(day string, ms int64)) {
	for start < end {
		e := min(end, nextDayStart(s, start))
		f(s.DayOf(start), e-start)
		start = e
	}
}

func Summarize(t Timeline, s Settings, order []string) Report {
	var r Report
	var power []Span
	for _, p := range t.Power {
		power = append(power, Span{Start: p.Start, End: p.End})
	}
	for _, p := range mergeSpans(power, 1) {
		r.Uptime += p.End - p.Start
	}

	byProject := map[string]*ProjectTotal{}
	get := func(id string) *ProjectTotal {
		p := byProject[id]
		if p == nil {
			p = &ProjectTotal{Project: id, Activities: map[string]int64{}, Days: map[string]DayTotal{}}
			byProject[id] = p
		}
		return p
	}
	day := func(p *ProjectTotal, d string) DayTotal {
		dt := p.Days[d]
		if dt.Activities == nil {
			dt.Activities = map[string]int64{}
		}
		return dt
	}

	for _, seg := range t.Me {
		p := get(seg.Project)
		byDay(s, seg.Start, seg.End, func(d string, ms int64) {
			dt := day(p, d)
			dt.Me += ms
			dt.Activities[seg.Activity] += ms
			p.Days[d] = dt
			p.Me += ms
			p.Activities[seg.Activity] += ms
			r.Me += ms
		})
	}

	for _, seg := range t.Leisure {
		r.Leisure += seg.End - seg.Start
	}

	// A project's agent time counts once while any of its agents works; same across projects.
	var all []Span
	perProject := map[string][]Span{}
	for _, a := range t.Agents {
		perProject[a.Project] = append(perProject[a.Project], Span{Start: a.Start, End: a.End})
		all = append(all, Span{Start: a.Start, End: a.End})
	}
	for id, spans := range perProject {
		p := get(id)
		for _, sp := range mergeSpans(spans, 1) {
			byDay(s, sp.Start, sp.End, func(d string, ms int64) {
				dt := day(p, d)
				dt.Agents += ms
				p.Days[d] = dt
				p.Agents += ms
			})
		}
	}
	for _, sp := range mergeSpans(all, 1) {
		r.Agents += sp.End - sp.Start
	}
	var waits []Span
	for _, w := range t.Waits {
		waits = append(waits, Span{Start: w.Start, End: w.End})
	}
	for _, sp := range mergeSpans(waits, 1) {
		r.Waiting += sp.End - sp.Start
	}

	// Projects in config order, unknown ids after them, "no project" last.
	seen := map[string]bool{"": true}
	var ids []string
	for _, id := range order {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	var rest []string
	for id := range byProject {
		if !seen[id] {
			rest = append(rest, id)
		}
	}
	sort.Strings(rest)
	ids = append(append(ids, rest...), "")
	for _, id := range ids {
		if p := byProject[id]; p != nil {
			r.Projects = append(r.Projects, *p)
		}
	}
	return r
}

// clipSeg cuts [s, e) to [from, to); ok is false when nothing is left.
func clipSeg(s, e, from, to int64) (int64, int64, bool) {
	s, e = max(s, from), min(e, to)
	return s, e, s < e
}

// Snapshot freezes the project's time inside [from, to) for a closed period; project "" takes every
// project's time and Досуг, for a month record.
func Snapshot(t Timeline, project string, from, to int64) Period {
	p := Period{Project: project, From: from, To: to}
	all := project == ""
	for _, m := range t.Me {
		if s, e, ok := clipSeg(m.Start, m.End, from, to); ok && (all || m.Project == project) {
			m.Start, m.End = s, e
			p.Me = append(p.Me, m)
		}
	}
	for _, a := range t.Agents {
		if s, e, ok := clipSeg(a.Start, a.End, from, to); ok && (all || a.Project == project) {
			a.Start, a.End = s, e
			p.Agents = append(p.Agents, a)
		}
	}
	for _, l := range t.Leisure {
		if s, e, ok := clipSeg(l.Start, l.End, from, to); ok && all {
			l.Start, l.End = s, e
			p.Leisure = append(p.Leisure, l)
		}
	}
	return p
}

// ApplyPeriods replaces each closed period's time with its snapshot: a v1 period its project's, other
// projects' fresh time staying, so time that moved elsewhere shows up as a divergence, not as a silent
// change; a month record all of it, Досуг included. v1 periods go first: a month closed over one froze
// the time with it applied.
func ApplyPeriods(t Timeline, periods []Period) (Timeline, []Divergence) {
	out := t
	out.Me = append([]Segment(nil), t.Me...)
	out.Agents = append([]AgentSeg(nil), t.Agents...)
	var div []Divergence
	isMonth := func(p Period) bool { return p.Month != "" }
	legacy := slices.DeleteFunc(slices.Clone(periods), isMonth)
	months := slices.DeleteFunc(slices.Clone(periods), func(p Period) bool { return !isMonth(p) })
	for _, p := range slices.Concat(legacy, months) {
		takes := func(project string) bool { return p.Month != "" || project == p.Project }
		delta := map[string]int64{} // fresh − snapshot by project
		var me []Segment
		for _, m := range out.Me {
			if !takes(m.Project) {
				me = append(me, m)
				continue
			}
			splitBy(m.Start, m.End, p.From, p.To, func(s, e int64, inside bool) {
				if inside {
					delta[m.Project] += e - s
					return
				}
				c := m
				c.Start, c.End = s, e
				me = append(me, c)
			})
		}
		var ag []AgentSeg
		for _, a := range out.Agents {
			if !takes(a.Project) {
				ag = append(ag, a)
				continue
			}
			splitBy(a.Start, a.End, p.From, p.To, func(s, e int64, inside bool) {
				if inside {
					delta[a.Project] += e - s
					return
				}
				c := a
				c.Start, c.End = s, e
				ag = append(ag, c)
			})
		}
		for _, m := range p.Me {
			if s, e, ok := clipSeg(m.Start, m.End, p.From, p.To); ok {
				m.Start, m.End = s, e
				me = append(me, m)
				delta[m.Project] -= e - s
			}
		}
		for _, a := range p.Agents {
			if s, e, ok := clipSeg(a.Start, a.End, p.From, p.To); ok {
				a.Start, a.End = s, e
				ag = append(ag, a)
				delta[a.Project] -= e - s
			}
		}
		// A v1 period is frozen as my time and keeps no Досуг, so none shows inside it; a month record has its own.
		var leisure []Segment
		for _, l := range out.Leisure {
			splitBy(l.Start, l.End, p.From, p.To, func(s, e int64, inside bool) {
				if !inside {
					c := l
					c.Start, c.End = s, e
					leisure = append(leisure, c)
				}
			})
		}
		for _, l := range p.Leisure {
			if s, e, ok := clipSeg(l.Start, l.End, p.From, p.To); ok {
				l.Start, l.End = s, e
				leisure = append(leisure, l)
			}
		}
		sort.SliceStable(me, func(a, b int) bool { return me[a].Start < me[b].Start })
		sort.SliceStable(ag, func(a, b int) bool { return ag[a].Start < ag[b].Start })
		sort.SliceStable(leisure, func(a, b int) bool { return leisure[a].Start < leisure[b].Start })
		out.Me, out.Agents, out.Leisure = me, ag, leisure
		for _, id := range slices.Sorted(maps.Keys(delta)) {
			if delta[id] != 0 {
				div = append(div, Divergence{Period: p.ID, Month: p.Month, Project: id, DeltaMs: delta[id]})
			}
		}
	}
	return out, div
}
