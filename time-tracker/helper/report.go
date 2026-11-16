package main

import (
	"sort"
	"time"
)

type DayTotal struct {
	Me, Agents int64
	Activities map[string]int64
}

type ProjectTotal struct {
	Project    string
	Me, Agents int64
	Activities map[string]int64
	Days       map[string]DayTotal
}

type Report struct {
	Uptime, Me, Agents int64
	Projects           []ProjectTotal
}

// Divergence says a closed period no longer matches what fresh rules would give.
type Divergence struct {
	Period  int64
	Project string
	DeltaMs int64
}

// RoundMinutes rounds ms to the nearest multiple of step minutes, exactly half up.
func RoundMinutes(ms int64, step int) int64 {
	unit := int64(step) * 60_000
	return (ms + unit/2) / unit * int64(step)
}

// nextDayStart is the first instant after ms at which the next day begins.
func nextDayStart(s Settings, ms int64) int64 {
	loc := s.Loc
	if loc == nil {
		loc = time.Local
	}
	start, err := time.Parse("15:04", s.DayStart)
	if err != nil {
		start = time.Time{}
	}
	shift := time.Duration(start.Hour())*time.Hour + time.Duration(start.Minute())*time.Minute
	y, mo, d := time.UnixMilli(ms).In(loc).Add(-shift).Date()
	next := time.Date(y, mo, d+1, start.Hour(), start.Minute(), 0, 0, loc).UnixMilli()
	if next <= ms { // clock oddities around DST: never stall
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

// Snapshot freezes the project's time inside [from, to) for a closed period.
func Snapshot(t Timeline, project string, from, to int64) Period {
	p := Period{Project: project, From: from, To: to}
	for _, m := range t.Me {
		if s, e, ok := clipSeg(m.Start, m.End, from, to); ok && m.Project == project {
			m.Start, m.End = s, e
			p.Me = append(p.Me, m)
		}
	}
	for _, a := range t.Agents {
		if s, e, ok := clipSeg(a.Start, a.End, from, to); ok && a.Project == project {
			a.Start, a.End = s, e
			p.Agents = append(p.Agents, a)
		}
	}
	return p
}

// ApplyPeriods replaces each closed period's project time with its snapshot. Other projects' fresh
// time stays, so time that moved elsewhere shows up as a divergence, not as a silent change.
func ApplyPeriods(t Timeline, periods []Period) (Timeline, []Divergence) {
	out := t
	out.Me = append([]Segment(nil), t.Me...)
	out.Agents = append([]AgentSeg(nil), t.Agents...)
	var div []Divergence
	for _, p := range periods {
		var fresh, snap int64
		var me []Segment
		for _, m := range out.Me {
			if m.Project != p.Project {
				me = append(me, m)
				continue
			}
			splitBy(m.Start, m.End, p.From, p.To, func(s, e int64, inside bool) {
				if inside {
					fresh += e - s
					return
				}
				c := m
				c.Start, c.End = s, e
				me = append(me, c)
			})
		}
		var ag []AgentSeg
		for _, a := range out.Agents {
			if a.Project != p.Project {
				ag = append(ag, a)
				continue
			}
			splitBy(a.Start, a.End, p.From, p.To, func(s, e int64, inside bool) {
				if inside {
					fresh += e - s
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
				snap += e - s
			}
		}
		for _, a := range p.Agents {
			if s, e, ok := clipSeg(a.Start, a.End, p.From, p.To); ok {
				a.Start, a.End = s, e
				ag = append(ag, a)
				snap += e - s
			}
		}
		sort.SliceStable(me, func(a, b int) bool { return me[a].Start < me[b].Start })
		sort.SliceStable(ag, func(a, b int) bool { return ag[a].Start < ag[b].Start })
		out.Me, out.Agents = me, ag
		if fresh != snap {
			div = append(div, Divergence{Period: p.ID, Project: p.Project, DeltaMs: fresh - snap})
		}
	}
	return out, div
}
