package main

import (
	"cmp"
	"math"
	"net/http"
	"slices"
	"strings"
	"time"
)

// Goal is one version of a project's goal; it applies from From until the next version.
type Goal struct {
	From     string `json:"from"`               // YYYY-MM-DD, the first day it applies
	Dir      string `json:"dir"`                // min | max
	Minutes  int64  `json:"minutes"`            // per Period
	Period   string `json:"period"`             // day | week | month
	Weekdays []int  `json:"weekdays,omitempty"` // day goals: 1 = Monday … 7 = Sunday
	Calendar bool   `json:"calendar,omitempty"` // day goals: holidays out, moved working days in
	Weekends bool   `json:"weekends,omitempty"` // day goals: time on unplanned days counts
	Off      bool   `json:"off,omitempty"`      // no goal from From on (Убрать цель)
}

type DayOff struct {
	ID       int64    `json:"id"`
	From     string   `json:"from"`     // YYYY-MM-DD
	To       string   `json:"to"`       // inclusive
	Kind     string   `json:"kind"`     // otgul | vacation | sick
	Projects []string `json:"projects"` // whose plan these days drop
	Made     int64    `json:"made"`
}

type planDay struct {
	Day     string             `json:"day"`
	Work    bool               `json:"work"` // a working day by the production calendar
	Holiday bool               `json:"holiday,omitempty"`
	Off     string             `json:"off,omitempty"` // the kind of a day off on it
	Me      map[string]int64   `json:"me"`            // my minutes by project ("" = Другое), rounded per day
	Agents  map[string]int64   `json:"agents"`        // agents' minutes by project, rounded per day
	Plan    map[string]float64 `json:"plan"`          // planned minutes of every project with a goal that day
	Fact    map[string]int64   `json:"fact"`          // its minutes that count toward that goal
}

type goalSum struct {
	Plan       int64 `json:"plan"`         // over the whole range, rounded half up
	PlanToDate int64 `json:"plan_to_date"` // days up to and including today
	Fact       int64 `json:"fact"`         // counted minutes up to today
	Me         int64 `json:"me"`           // all my minutes on the project up to today
	Balance    int64 `json:"balance"`      // Fact − PlanToDate
	Forecast   int64 `json:"forecast"`     // Fact × Plan / PlanToDate, half up; Fact when PlanToDate is 0
	Need       int64 `json:"need"`         // (Plan − Fact) / DaysLeft rounded up; 0 when met or DaysLeft is 0
	Days       int   `json:"days"`         // planned days in the range
	DaysLeft   int   `json:"days_left"`    // planned days after today
}

// dated is one version of a project's goal or pay: it applies from start() on; an ended() one
// means nothing from then.
type dated interface {
	start() string
	ended() bool
}

func (g Goal) start() string { return g.From }
func (g Goal) ended() bool   { return g.Off }

// versionOn is the version in force at at, the latest start up to it in any order; nil when none or ended.
func versionOn[T dated](vs []T, at string) *T {
	var v *T
	for i := range vs {
		if vs[i].start() <= at && (v == nil || vs[i].start() >= (*v).start()) {
			v = &vs[i]
		}
	}
	if v == nil || (*v).ended() {
		return nil
	}
	return v
}

// setVersion replaces every version from v's start on with v; an ended one with nothing before it is dropped too.
func setVersion[T dated](vs []T, v T) []T {
	out := []T{}
	for _, x := range vs {
		if x.start() < v.start() {
			out = append(out, x)
		}
	}
	if v.ended() && len(out) == 0 {
		return out
	}
	out = append(out, v)
	slices.SortStableFunc(out, func(a, b T) int { return strings.Compare(a.start(), b.start()) })
	return out
}

func goalOn(goals []Goal, day string) *Goal { return versionOn(goals, day) }
func setGoal(goals []Goal, g Goal) []Goal   { return setVersion(goals, g) }

// dayPlan is g's planned minutes on day: day goals on their planned days, week and month goals spread
// over every calendar day.
func dayPlan(g *Goal, day string, cal Calendar) float64 {
	if g == nil {
		return 0
	}
	switch g.Period {
	case "day":
		in := slices.Contains(g.Weekdays, weekday(day))
		if g.Calendar {
			in = (in && !cal.Holiday(day)) || cal.Moved(day)
		}
		if in {
			return float64(g.Minutes)
		}
	case "week":
		return float64(g.Minutes) / 7
	case "month":
		if t, err := time.Parse("2006-01-02", day); err == nil {
			return float64(g.Minutes) / float64(time.Date(t.Year(), t.Month()+1, 0, 0, 0, 0, 0, time.UTC).Day())
		}
	}
	return 0
}

func planDays(days []string, cal Calendar, projects []Project, off []DayOff, rep Report) []planDay {
	out := make([]planDay, 0, len(days))
	for _, day := range days {
		d := planDay{Day: day, Work: cal.Working(day), Holiday: cal.Holiday(day),
			Me: map[string]int64{}, Agents: map[string]int64{}, Plan: map[string]float64{}, Fact: map[string]int64{}}
		var offOn []DayOff
		var latest int64
		for _, o := range off {
			if o.From <= day && day <= o.To {
				offOn = append(offOn, o)
				if d.Off == "" || o.Made >= latest {
					d.Off, latest = o.Kind, o.Made
				}
			}
		}
		for _, p := range rep.Projects {
			t := p.Days[day]
			if m := RoundMinutes(t.Me, 1); m != 0 {
				d.Me[p.Project] += m
			}
			if m := RoundMinutes(t.Agents, 1); m != 0 {
				d.Agents[p.Project] += m
			}
		}
		for _, p := range projects {
			g := goalOn(p.Goals, day)
			if g == nil {
				continue
			}
			plan := dayPlan(g, day, cal)
			if slices.ContainsFunc(offOn, func(o DayOff) bool { return slices.Contains(o.Projects, p.ID) }) {
				plan = 0
			}
			d.Plan[p.ID] = plan
			if plan > 0 || g.Period != "day" || g.Weekends {
				d.Fact[p.ID] = d.Me[p.ID]
			} else {
				d.Fact[p.ID] = 0
			}
		}
		out = append(out, d)
	}
	return out
}

// planUnit is the least common multiple of every dayPlan divisor (1, 7, 28…31 days): a day's plan times it
// is a whole number, so plans add up exactly and a sum ending in .5 rounds up instead of drifting.
const planUnit = 4 * 3 * 5 * 7 * 29 * 31

// planUnits is a day's plan in 1/planUnit minutes; roundUnits is a sum of them in minutes, half up.
func planUnits(p float64) int64 { return int64(math.Round(p * planUnit)) }
func roundUnits(u int64) int64  { return (u + planUnit/2) / planUnit }

func sumGoal(days []planDay, project, today string) goalSum {
	var s goalSum
	var plan, toDate int64 // in 1/planUnit minutes
	for _, d := range days {
		p, ok := d.Plan[project]
		if ok && p > 0 {
			u := planUnits(p)
			plan += u
			s.Days++
			if d.Day <= today {
				toDate += u
			} else {
				s.DaysLeft++
			}
		}
		if d.Day <= today {
			s.Fact += d.Fact[project]
			s.Me += d.Me[project]
		}
	}
	s.Plan = roundUnits(plan)
	s.PlanToDate = roundUnits(toDate)
	s.Balance = s.Fact - s.PlanToDate
	s.Forecast = s.Fact
	if s.PlanToDate > 0 {
		s.Forecast = (2*s.Fact*s.Plan + s.PlanToDate) / (2 * s.PlanToDate)
	}
	if left := s.Plan - s.Fact; left > 0 && s.DaysLeft > 0 {
		s.Need = (left + int64(s.DaysLeft) - 1) / int64(s.DaysLeft)
	}
	return s
}

// maxPlanDays caps every range a plan is built over: the timeline behind it is read under d.mu.
const maxPlanDays = 62

// dayList is the days from first to last, both in; more than maxPlanDays of them is an error.
func dayList(first, last string) ([]string, error) {
	a, err1 := time.Parse(time.DateOnly, first)
	b, err2 := time.Parse(time.DateOnly, last)
	if err1 != nil || err2 != nil || b.Before(a) || b.Sub(a) >= maxPlanDays*24*time.Hour {
		return nil, fail(http.StatusBadRequest, "from, to: want days YYYY-MM-DD, from not after to, at most %d days", maxPlanDays)
	}
	var days []string
	for t := a; !t.After(b); t = t.AddDate(0, 0, 1) {
		days = append(days, t.Format(time.DateOnly))
	}
	return days, nil
}

// frozenDays maps each day of a plan range that a closed month covers to that month's record.
type frozenDays map[string]*Period

// goalOn is p's goal on day: from the record's goal versions on a closed day, else from the config.
func (f frozenDays) goalOn(p Project, day string) *Goal {
	if rec := f[day]; rec != nil {
		return goalOn(rec.GoalVersions[p.ID], day)
	}
	return goalOn(p.Goals, day)
}

// planRange is the plan of every day from first to last, both in; a closed month's days come from its
// record, and frozen says which ones. Callers hold mu.
func (d *Daemon) planRange(first, last string) ([]planDay, frozenDays, error) {
	days, err := dayList(first, last)
	if err != nil {
		return nil, nil, err
	}
	from, _, err := dayRange(d.S, first)
	if err != nil {
		return nil, nil, err
	}
	_, to, err := dayRange(d.S, last)
	if err != nil {
		return nil, nil, err
	}
	tl, _, err := d.timeline(from, to)
	if err != nil {
		return nil, nil, err
	}
	off, err := d.Store.DaysOff()
	if err != nil {
		return nil, nil, err
	}
	periods, err := d.Store.Periods()
	if err != nil {
		return nil, nil, err
	}
	out := planDays(days, d.Cal, d.Config.Projects, off, Summarize(tl, d.S, d.order()))
	frozen := frozenDays{}
	for i := range periods {
		for _, pd := range periods[i].Days {
			if j := slices.IndexFunc(out, func(x planDay) bool { return x.Day == pd.Day }); j >= 0 {
				out[j], frozen[pd.Day] = pd, &periods[i]
			}
		}
	}
	return out, frozen, nil
}

// checkGoal validates g; week and month goals lose the day-goal fields, an Off keeps only From.
func checkGoal(g Goal) (Goal, error) {
	if _, err := time.Parse(time.DateOnly, g.From); err != nil {
		return g, fail(http.StatusBadRequest, "from: want YYYY-MM-DD")
	}
	if g.Off {
		return Goal{From: g.From, Off: true}, nil
	}
	if g.Dir != "min" && g.Dir != "max" {
		return g, fail(http.StatusBadRequest, "dir: want min or max")
	}
	limit := map[string]int64{"day": 1440, "week": 7 * 1440, "month": 31 * 1440}[g.Period]
	if limit == 0 {
		return g, fail(http.StatusBadRequest, "period: want day, week or month")
	}
	// sumGoal rounds half up assuming no plan is negative: minutes stay at least 1.
	if g.Minutes < 1 || g.Minutes > limit {
		return g, fail(http.StatusBadRequest, "minutes: want 1..1440 a day, 10080 a week, 44640 a month")
	}
	if g.Period != "day" {
		g.Weekdays, g.Calendar, g.Weekends = nil, false, false
		return g, nil
	}
	if len(g.Weekdays) == 0 || slices.ContainsFunc(g.Weekdays, func(w int) bool { return w < 1 || w > 7 }) {
		return g, fail(http.StatusBadRequest, "weekdays: want days 1..7")
	}
	g.Weekdays = slices.Compact(slices.Sorted(slices.Values(g.Weekdays)))
	return g, nil
}

// putGoal sets a project's goal from g.From on; earlier versions stay. A change is logged in history.json.
func (d *Daemon) putGoal(r *http.Request) (int, any, error) {
	id := r.PathValue("id")
	var g Goal
	if err := decode(r, &g); err != nil {
		return 0, nil, err
	}
	var p Project
	ch := Change{Kind: "goal-set", Project: id}
	_, err := d.editConfig(func(c *Config) error {
		i := slices.IndexFunc(c.Projects, func(x Project) bool { return x.ID == id })
		if i < 0 {
			return fail(http.StatusNotFound, "no project %q", id)
		}
		g, err := checkGoal(g)
		if err != nil {
			return err
		}
		ch.Goal, ch.PrevGoals = &g, c.Projects[i].Goals
		c.Projects[i].Goals = setGoal(c.Projects[i].Goals, g)
		ch.Goals, p = c.Projects[i].Goals, c.Projects[i]
		return nil
	})
	if err == nil && !sameGoals(ch.PrevGoals, ch.Goals) {
		d.logChange(ch)
	}
	return http.StatusOK, p, err
}

func (d *Daemon) postDayOff(r *http.Request) (int, any, error) {
	if err := d.configErr(); err != nil {
		return 0, nil, err
	}
	var o DayOff
	if err := decode(r, &o); err != nil {
		return 0, nil, err
	}
	if _, err := dayList(o.From, o.To); err != nil {
		return 0, nil, err
	}
	if !slices.Contains([]string{"otgul", "vacation", "sick"}, o.Kind) {
		return 0, nil, fail(http.StatusBadRequest, "kind: want otgul, vacation or sick")
	}
	for _, p := range o.Projects {
		if d.project(p) == nil {
			return 0, nil, fail(http.StatusBadRequest, "unknown project %q", p)
		}
	}
	if err := d.refuseClosed(o.From, o.To); err != nil {
		return 0, nil, err
	}
	o.ID, o.Made, o.Projects = 0, nowMs(), nonNil(o.Projects)
	created, err := d.Store.AddDayOff(o)
	return http.StatusCreated, created, err
}

func (d *Daemon) deleteDayOff(r *http.Request) (int, any, error) {
	id, err := intParam(r.PathValue("id"), "id")
	if err != nil {
		return 0, nil, err
	}
	offs, err := d.Store.DaysOff()
	if err != nil {
		return 0, nil, err
	}
	i := slices.IndexFunc(offs, func(o DayOff) bool { return o.ID == id })
	if i < 0 {
		return 0, nil, fail(http.StatusNotFound, "no day off %d", id)
	}
	if err := d.refuseClosed(offs[i].From, offs[i].To); err != nil {
		return 0, nil, err
	}
	return http.StatusNoContent, nil, d.Store.RemoveDayOff(id)
}

// getPlan gives the days from ?from= to ?to=, the sums over the range of each project planned on any of
// them, and its goal: the latest version in force on a day of the range up to today.
func (d *Daemon) getPlan(r *http.Request) (int, any, error) {
	first, last := r.URL.Query().Get("from"), r.URL.Query().Get("to")
	days, frozen, err := d.planRange(first, last)
	if err != nil {
		return 0, nil, err
	}
	today := d.S.DayOf(nowMs())
	goals, sums := map[string]Goal{}, map[string]goalSum{}
	for _, p := range d.Config.Projects {
		planned := false
		for _, pd := range days {
			_, ok := pd.Plan[p.ID]
			planned = planned || ok
			if g := frozen.goalOn(p, pd.Day); g != nil && pd.Day <= today {
				goals[p.ID] = *g
			}
		}
		if planned {
			sums[p.ID] = sumGoal(days, p.ID, today)
		}
	}
	var names []string // a closed month's days depend on no calendar
	for _, pd := range days {
		if frozen[pd.Day] == nil {
			names = append(names, pd.Day)
		}
	}
	return http.StatusOK, map[string]any{"today": today, "days": days, "goals": goals, "sums": sums,
		"no_calendar": d.Cal.Missing(names)}, nil
}

type goalInfo struct {
	Project string  `json:"project"`
	Goal    Goal    `json:"goal"`
	Day     goalSum `json:"day"`
	Week    goalSum `json:"week"`
	Month   goalSum `json:"month"`
	Agents  int64   `json:"agents"` // the project's agent minutes that day
}

// getGoals sums each goal in force on ?day= (default today) over that day, its Monday–Sunday week and
// its month, in config order.
func (d *Daemon) getGoals(r *http.Request) (int, any, error) {
	today := d.S.DayOf(nowMs())
	day := cmp.Or(r.URL.Query().Get("day"), today)
	t, err := time.Parse(time.DateOnly, day)
	if err != nil {
		return 0, nil, fail(http.StatusBadRequest, "day: want YYYY-MM-DD, got %q", day)
	}
	date := func(t time.Time) string { return t.Format(time.DateOnly) }
	mon := t.AddDate(0, 0, 1-weekday(day))
	weekFrom, weekTo := date(mon), date(mon.AddDate(0, 0, 6))
	month := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
	monthFrom, monthTo := date(month), date(month.AddDate(0, 1, -1))
	days, frozen, err := d.planRange(min(weekFrom, monthFrom), max(weekTo, monthTo)) // at most 37 days
	if err != nil {
		return 0, nil, err
	}
	in := func(from, to string) []planDay {
		return slices.DeleteFunc(slices.Clone(days), func(pd planDay) bool { return pd.Day < from || pd.Day > to })
	}
	out := []goalInfo{}
	for _, p := range d.Config.Projects {
		g := frozen.goalOn(p, day)
		if g == nil {
			continue
		}
		info := goalInfo{Project: p.ID, Goal: *g, Day: sumGoal(in(day, day), p.ID, today),
			Week: sumGoal(in(weekFrom, weekTo), p.ID, today), Month: sumGoal(in(monthFrom, monthTo), p.ID, today)}
		for _, pd := range in(day, day) {
			info.Agents += pd.Agents[p.ID]
		}
		out = append(out, info)
	}
	return http.StatusOK, map[string]any{"day": day, "today": today, "goals": out}, nil
}
