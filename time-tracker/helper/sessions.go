package main

import "sort"

type SessionMark struct {
	ID   int64  `json:"id"`
	Kind string `json:"kind"` // "start" | "end"
	At   int64  `json:"at"`
}
type Share struct {
	Project string `json:"project"`
	Me      int64  `json:"me"`
}
type Stretch struct {
	Start     int64   `json:"start"`
	End       int64   `json:"end"`
	Project   string  `json:"project"`    // the project with the most time in it; "" = Другое
	Me        int64   `json:"me"`         // active time, all projects
	Shares    []Share `json:"shares"`     // the other projects, most first; never nil
	GapBefore int64   `json:"gap_before"` // the break before it (> pause), 0 for the first and for project splits
	Current   bool    `json:"current"`
}
type Session struct {
	Start     int64     `json:"start"`
	End       int64     `json:"end"` // last activity, or the manual end
	Open      bool      `json:"open"`
	Manual    bool      `json:"manual"` // opened with Начать
	Me        int64     `json:"me"`
	Breaks    int64     `json:"breaks"`
	Switches  int       `json:"switches"`
	Focus     int64     `json:"focus"`
	Stretches []Stretch `json:"stretches"` // never nil
}
type DayStats struct {
	Switches int   `json:"switches"`
	Focus    int64 `json:"focus"`
	Breaks   int64 `json:"breaks"`
}

type sweepEv struct {
	at, end, id int64
	piece       bool
	kind        string
}

// Sessions sweeps activity pieces (me split at mark times) and marks by time; at one instant marks
// come first, in ID order. Fills Start, End, Open, Manual only.
func Sessions(me []Segment, marks []SessionMark, idleStop, now int64) []Session {
	var evs []sweepEv
	for _, m := range marks {
		evs = append(evs, sweepEv{at: m.At, id: m.ID, kind: m.Kind})
	}
	for _, sg := range me {
		s := sg.Start
		for _, m := range marks {
			if m.At > s && m.At < sg.End {
				evs = append(evs, sweepEv{at: s, end: m.At, piece: true})
				s = m.At
			}
		}
		evs = append(evs, sweepEv{at: s, end: sg.End, piece: true})
	}
	sort.SliceStable(evs, func(i, j int) bool {
		a, b := evs[i], evs[j]
		if a.at != b.at {
			return a.at < b.at
		}
		if a.piece != b.piece {
			return !a.piece
		}
		return a.id < b.id
	})
	var out []Session
	var cur *Session
	var last int64
	held := false // after a manual end, activity starts nothing until a gap of idleStop
	closeCur := func() {
		cur.Open = false
		if cur.End > cur.Start {
			out = append(out, *cur)
		}
		cur = nil
	}
	for _, ev := range evs {
		switch {
		case ev.piece:
			if cur != nil && ev.at-last >= idleStop {
				closeCur()
			}
			if cur == nil {
				if held && ev.at-last < idleStop {
					last = max(last, ev.end)
					continue
				}
				held = false
				cur = &Session{Start: ev.at}
			}
			cur.End, last = ev.end, ev.end
		case ev.kind == "start":
			if cur != nil && ev.at-last >= idleStop {
				closeCur()
			}
			if cur == nil {
				cur = &Session{Start: ev.at, End: ev.at, Manual: true}
				last, held = ev.at, false
			}
		case ev.kind == "end":
			if cur != nil {
				if ev.at-last < idleStop {
					cur.End = ev.at
				}
				closeCur()
			}
			held, last = true, max(last, ev.at)
		}
	}
	if cur != nil {
		if now-last < idleStop {
			cur.Open = true
			out = append(out, *cur)
		} else {
			closeCur()
		}
	}
	return out
}

type run struct {
	key        string // the project the run belongs to; "" only for a block of just Другое
	ms         map[string]int64
	start, end int64
	total      int64
}

func newRun(key string, start int64) *run {
	return &run{key: key, ms: map[string]int64{}, start: start}
}

func (r *run) add(sg Segment) {
	d := sg.End - sg.Start
	r.ms[sg.Project] += d
	r.total += d
	r.end = sg.End
}

func (r *run) absorb(o *run) {
	for p, d := range o.ms {
		r.ms[p] += d
	}
	r.total += o.total
	r.start, r.end = min(r.start, o.start), max(r.end, o.end)
}

func Stretches(me []Segment, pause int64) []Stretch {
	out := []Stretch{}
	for i := 0; i < len(me); {
		j := i + 1
		for j < len(me) && me[j].Start-me[j-1].End <= pause {
			j++
		}
		for k, r := range blockRuns(me[i:j], pause) {
			st := stretchOf(r)
			if k == 0 && i > 0 {
				st.GapBefore = me[i].Start - me[i-1].End
			}
			out = append(out, st)
		}
		i = j
	}
	return out
}

func blockRuns(blk []Segment, pause int64) []*run {
	var runs []*run
	var lead []Segment // Другое before the first project joins the run after it
	for _, sg := range blk {
		if sg.Project == "" {
			if len(runs) == 0 {
				lead = append(lead, sg)
			} else {
				runs[len(runs)-1].add(sg)
			}
			continue
		}
		if n := len(runs); n == 0 || runs[n-1].key != sg.Project {
			start := sg.Start
			if len(lead) > 0 {
				start = lead[0].Start
			}
			runs = append(runs, newRun(sg.Project, start))
		}
		if len(runs) == 1 {
			for _, l := range lead {
				runs[0].add(l)
			}
			lead = nil
		}
		runs[len(runs)-1].add(sg)
	}
	if len(runs) == 0 {
		r := newRun("", lead[0].Start)
		for _, l := range lead {
			r.add(l)
		}
		return []*run{r}
	}
	for len(runs) > 1 {
		f := -1
		for i, r := range runs {
			if r.total < pause {
				f = i
				break
			}
		}
		if f < 0 {
			break
		}
		if f == 0 {
			runs[1].absorb(runs[0])
			runs = runs[1:]
		} else {
			runs[f-1].absorb(runs[f])
			runs = append(runs[:f], runs[f+1:]...)
		}
		for i := 1; i < len(runs); {
			if runs[i].key == runs[i-1].key {
				runs[i-1].absorb(runs[i])
				runs = append(runs[:i], runs[i+1:]...)
			} else {
				i++
			}
		}
	}
	return runs
}

func stretchOf(r *run) Stretch {
	var all []Share
	for p, d := range r.ms {
		all = append(all, Share{p, d})
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].Me != all[j].Me {
			return all[i].Me > all[j].Me
		}
		return all[i].Project < all[j].Project
	})
	return Stretch{Start: r.start, End: r.end, Project: all[0].Project, Me: r.total, Shares: append([]Share{}, all[1:]...)}
}

// ponytail: no minimum length; a brief alt-tab counts two, add a floor if the number reads noisy
func Switches(me []Segment) int {
	n, prev := 0, ""
	for _, sg := range me {
		if sg.Project == "" {
			continue
		}
		if prev != "" && sg.Project != prev {
			n++
		}
		prev = sg.Project
	}
	return n
}

// Focus is the longest run on one project; breaks up to pause and Другое don't interrupt it.
func Focus(me []Segment, pause int64) int64 {
	var best, cur, runEnd int64
	proj := ""
	for _, sg := range me {
		switch {
		case proj != "" && sg.Start-runEnd <= pause && sg.Project == proj:
			cur += sg.End - sg.Start
		case proj != "" && sg.Start-runEnd <= pause && sg.Project == "":
		default:
			proj, cur = sg.Project, 0
			if proj != "" {
				cur = sg.End - sg.Start
			}
		}
		runEnd = sg.End
		best = max(best, cur)
	}
	return best
}

func clipMe(me []Segment, from, to int64) []Segment {
	var out []Segment
	for _, sg := range me {
		sg.Start, sg.End = max(sg.Start, from), min(sg.End, to)
		if sg.End > sg.Start {
			out = append(out, sg)
		}
	}
	return out
}

func describe(ss Session, me []Segment, s Settings, now int64) Session {
	to := ss.End
	if ss.Open {
		to = now
	}
	me = clipMe(me, ss.Start, to)
	pause := int64(s.PauseMinutes) * 60_000
	ss.Stretches = Stretches(me, pause)
	ss.Me, ss.Breaks = 0, 0
	for _, st := range ss.Stretches {
		ss.Me += st.Me
		ss.Breaks += st.GapBefore
	}
	if n := len(ss.Stretches); n > 0 && ss.Open && now-ss.Stretches[n-1].End < int64(s.IdleMinutes)*60_000 {
		ss.Stretches[n-1].Current = true
	}
	ss.Switches = Switches(me)
	ss.Focus = Focus(me, pause)
	return ss
}

func dayStats(me []Segment, sessions []Session, pause int64) DayStats {
	d := DayStats{Switches: Switches(me), Focus: Focus(me, pause)}
	for _, ss := range sessions {
		d.Breaks += ss.Breaks
	}
	return d
}
