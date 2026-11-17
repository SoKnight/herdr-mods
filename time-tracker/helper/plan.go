package main

import (
	"math"
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

// goalOn is the version in force on day; nil when none or Off.
func goalOn(goals []Goal, day string) *Goal {
	var g *Goal
	for i := range goals {
		if goals[i].From <= day {
			g = &goals[i]
		}
	}
	if g == nil || g.Off {
		return nil
	}
	return g
}

// setGoal replaces every version from g.From on with g; an Off with nothing before it is dropped too.
func setGoal(goals []Goal, g Goal) []Goal {
	out := []Goal{}
	for _, v := range goals {
		if v.From < g.From {
			out = append(out, v)
		}
	}
	if g.Off && len(out) == 0 {
		return out
	}
	out = append(out, g)
	slices.SortStableFunc(out, func(a, b Goal) int { return strings.Compare(a.From, b.From) })
	return out
}

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

func sumGoal(days []planDay, project, today string) goalSum {
	var s goalSum
	var plan, toDate int64 // in 1/planUnit minutes
	for _, d := range days {
		p, ok := d.Plan[project]
		if ok && p > 0 {
			u := int64(math.Round(p * planUnit))
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
	s.Plan = (plan + planUnit/2) / planUnit
	s.PlanToDate = (toDate + planUnit/2) / planUnit
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
