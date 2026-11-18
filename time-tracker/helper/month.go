package main

import (
	"cmp"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"
)

// monthRow is a week (Monday–Sunday, clipped to the month) or a day of a month report, cut at today.
type monthRow struct {
	Label    string   `json:"label"`    // the table's: "1 ноя", "2 – 8 ноя", "16 – 22 ноя"
	Text     string   `json:"text"`     // the text export's, ending today: "2–8 ноя", "16 ноя"
	From     string   `json:"from"`     // first day, inside the month
	To       string   `json:"to"`       // last day, inside the month and not after today
	Workdays int      `json:"workdays"` // working days From..To
	Plan     int64    `json:"plan"`     // plan minutes From..To: the plan to To less the plan before From, each rounded half up
	Me       int64    `json:"me"`
	Fact     int64    `json:"fact"` // counted toward the goal
	Agents   int64    `json:"agents"`
	Current  bool     `json:"current,omitempty"`
	Notes    []string `json:"notes"`   // the project's segment notes in the row, in time order, repeats dropped
	Commits  []string `json:"commits"` // the project's commit messages in the row, in time order, repeats dropped
}

// monthReport is a project's month: the one place its minutes are added up for the screen, the text and the CSV.
type monthReport struct {
	Project    string           `json:"project"`
	Month      string           `json:"month"` // YYYY-MM
	Today      string           `json:"today"`
	Running    bool             `json:"running"` // today is in the month
	Rows       []monthRow       `json:"rows"`
	Total      monthRow         `json:"total"`      // field by field the sum of Rows (Label, Text, Notes empty)
	Goal       *Goal            `json:"goal"`       // the latest in force on a day of the month up to today
	Plan       int64            `json:"plan"`       // the whole month's plan minutes (rate, forecast)
	PlanDays   int              `json:"plan_days"`  // the whole month's planned days
	Activities map[string]int64 `json:"activities"` // my minutes by kind of work
	Pay        *payment         `json:"pay"`
	Holidays   []string         `json:"holidays"` // production-calendar holidays in the month
	DaysOff    []DayOff         `json:"days_off"` // overlapping the month, listing the project
	NoCalendar []string         `json:"no_calendar"`
}

var shortMonths = [12]string{"янв", "фев", "мар", "апр", "мая", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"}

// daySpan names the days a..b of one month: "16 ноя", or "2" dash "8 ноя".
func daySpan(a, b time.Time, dash string) string {
	if a.Equal(b) {
		return fmt.Sprintf("%d %s", a.Day(), shortMonths[a.Month()-1])
	}
	return fmt.Sprintf("%d%s%d %s", a.Day(), dash, b.Day(), shortMonths[b.Month()-1])
}

// buildMonth fills the rows (week | day), their total, the month's plan and its holidays from the month's
// plan days; d.monthReport adds the goal, pay, activities, days off and calendar gaps.
func buildMonth(project, month, rows, today string, days []planDay, notes []Segment, commits []Commit, s Settings) monthReport {
	first, _ := time.Parse("2006-01", month)
	last := first.AddDate(0, 1, -1)
	date := func(t time.Time) string { return t.Format(time.DateOnly) }
	whole := sumGoal(days, project, today)
	m := monthReport{Project: project, Month: month, Today: today, Running: date(first) <= today && today <= date(last),
		Rows: []monthRow{}, Total: monthRow{Notes: []string{}, Commits: []string{}}, Plan: whole.Plan, PlanDays: whole.Days, Holidays: []string{}}
	for _, pd := range days {
		if pd.Holiday {
			m.Holidays = append(m.Holidays, pd.Day)
		}
	}
	notes = slices.SortedStableFunc(slices.Values(notes), func(a, b Segment) int { return cmp.Compare(a.Start, b.Start) })
	commits = slices.SortedStableFunc(slices.Values(commits), func(a, b Commit) int { return cmp.Compare(a.At, b.At) })
	var cum int64 // the plan up to the row's end in 1/planUnit minutes: rows round it once, so they add up to the plan to date
	for t := first; !t.After(last) && date(t) <= today; {
		end := t
		if rows == "week" {
			end = t.AddDate(0, 0, 7-weekday(date(t)))
			if end.After(last) {
				end = last
			}
		}
		upTo := end
		if date(end) > today {
			upTo, _ = time.Parse(time.DateOnly, today)
		}
		r := monthRow{Label: daySpan(t, end, " – "), Text: daySpan(t, upTo, "–"), From: date(t), To: date(upTo),
			Current: date(upTo) == today, Notes: []string{}, Commits: []string{}}
		in := slices.DeleteFunc(slices.Clone(days), func(pd planDay) bool { return pd.Day < r.From || pd.Day > r.To })
		sum := sumGoal(in, project, today)
		r.Me, r.Fact = sum.Me, sum.Fact
		before := roundUnits(cum)
		for _, pd := range in {
			if p := pd.Plan[project]; p > 0 {
				cum += planUnits(p)
			}
			r.Agents += pd.Agents[project]
			if pd.Work {
				r.Workdays++
			}
		}
		r.Plan = roundUnits(cum) - before
		for _, n := range notes {
			if day := s.DayOf(n.Start); n.Project == project && n.Note != "" && r.From <= day && day <= r.To && !slices.Contains(r.Notes, n.Note) {
				r.Notes = append(r.Notes, n.Note)
			}
		}
		for _, c := range commits {
			if day := s.DayOf(c.At); c.Project == project && r.From <= day && day <= r.To && !slices.Contains(r.Commits, c.Msg) {
				r.Commits = append(r.Commits, c.Msg)
			}
		}
		m.Rows = append(m.Rows, r)
		m.Total.Workdays += r.Workdays
		m.Total.Plan += r.Plan
		m.Total.Me += r.Me
		m.Total.Fact += r.Fact
		m.Total.Agents += r.Agents
		t = end.AddDate(0, 0, 1)
	}
	return m
}

// monthSpan is a YYYY-MM month's first and last day and its [from, to) in ms.
func monthSpan(s Settings, month string) (first, last string, from, to int64, err error) {
	t, err := time.Parse("2006-01", month)
	if err != nil || t.Format("2006-01") != month {
		return "", "", 0, 0, fail(http.StatusBadRequest, "month: want YYYY-MM")
	}
	first, last = t.Format(time.DateOnly), t.AddDate(0, 1, -1).Format(time.DateOnly)
	if from, _, err = dayRange(s, first); err == nil {
		_, to, err = dayRange(s, last)
	}
	return first, last, from, to, err
}

// monthTitle names a YYYY-MM month as a sentence starts it: "Ноябрь".
func monthTitle(month string) string {
	t, _ := time.Parse("2006-01", month)
	r := []rune(monthNames[t.Month()-1])
	return strings.ToUpper(string(r[:1])) + string(r[1:])
}

// monthGoals is each project's goal for a month report: as getPlan, the latest version in force on a day
// of the month up to today. Projects without one are left out.
func (d *Daemon) monthGoals(days []planDay, today string) map[string]*Goal {
	out := map[string]*Goal{}
	for _, p := range d.Config.Projects {
		for _, pd := range days {
			if g := goalOn(p.Goals, pd.Day); g != nil && pd.Day <= today {
				out[p.ID] = g
			}
		}
	}
	return out
}

// monthPays is each project's payment for the month of days, over its minutes and plan up to today as the
// month report adds them up. Projects with neither pay nor bonuses are left out.
func (d *Daemon) monthPays(month string, days []planDay, today string) (map[string]*payment, error) {
	all, err := d.Store.Bonuses()
	if err != nil {
		return nil, err
	}
	out := map[string]*payment{}
	for _, p := range d.Config.Projects {
		var bonuses []Bonus
		for _, b := range all {
			if b.Project == p.ID && b.Month == month {
				bonuses = append(bonuses, b)
			}
		}
		sum := sumGoal(days, p.ID, today)
		if pay := monthPay(payOn(p.Pay, month), sum.Me, sum.Plan, bonuses); pay != nil {
			out[p.ID] = pay
		}
	}
	return out, nil
}

// monthReport is a project's month up to today with rows by week or day; a closed month's plan, goal
// and pay come from its record. Callers hold mu.
func (d *Daemon) monthReport(project, month, rows string) (monthReport, error) {
	firstDay, lastDay, from, to, err := monthSpan(d.S, month)
	if err != nil {
		return monthReport{}, err
	}
	if d.project(project) == nil {
		return monthReport{}, fail(http.StatusBadRequest, "unknown project %q", project)
	}
	rows = cmp.Or(rows, "week")
	if rows != "week" && rows != "day" {
		return monthReport{}, fail(http.StatusBadRequest, "rows: want week or day")
	}
	today := d.S.DayOf(nowMs())
	if firstDay > today {
		return monthReport{}, fail(http.StatusBadRequest, "month: not started yet")
	}
	dayNames, err := dayList(firstDay, lastDay)
	if err != nil {
		return monthReport{}, err
	}
	tl, _, err := d.timeline(from, to) // the plan's, and the notes' and activities'
	if err != nil {
		return monthReport{}, err
	}
	days, _, err := d.planOver(dayNames, tl)
	if err != nil {
		return monthReport{}, err
	}
	m := buildMonth(project, month, rows, today, days, tl.Me, d.commits(from, to), d.S) // closed months read commits live: notes, not numbers
	rec, err := d.monthRecord(month)
	if err != nil {
		return monthReport{}, err
	}
	m.NoCalendar = []string{} // nothing in a closed month depends on today's calendar
	var goals map[string]*Goal
	var pays map[string]*payment
	if rec != nil {
		goals, pays = rec.Goals, rec.Pay
	} else {
		if pays, err = d.monthPays(month, days, today); err != nil {
			return monthReport{}, err
		}
		goals = d.monthGoals(days, today)
		var names []string
		for _, pd := range days {
			names = append(names, pd.Day)
		}
		m.NoCalendar = d.Cal.Missing(names)
	}
	m.Goal, m.Pay = goals[project], pays[project]
	m.Activities = map[string]int64{}
	for _, pt := range Summarize(tl, d.S, nil).Projects {
		if pt.Project != project {
			continue
		}
		for kind, ms := range pt.Activities {
			if v := RoundMinutes(ms, 1); v > 0 {
				m.Activities[kind] = v
			}
		}
	}
	m.DaysOff, err = d.daysOffIn(firstDay, lastDay, project)
	return m, err
}

// daysOffIn are the days off overlapping first..last that list project, or any when project is "".
// A closed month refuses adding or removing them, so they stand as it was closed.
func (d *Daemon) daysOffIn(first, last, project string) ([]DayOff, error) {
	offs, err := d.Store.DaysOff()
	return slices.DeleteFunc(offs, func(o DayOff) bool {
		return o.To < first || last < o.From || (project != "" && !slices.Contains(o.Projects, project))
	}), err
}

func (d *Daemon) getMonthReport(r *http.Request) (int, any, error) {
	q := r.URL.Query()
	m, err := d.monthReport(q.Get("project"), r.PathValue("month"), q.Get("rows"))
	return http.StatusOK, m, err
}
