package main

import (
	"cmp"
	"fmt"
	"net/http"
	"slices"
	"time"
)

// monthRow is a week (Monday–Sunday, clipped to the month) or a day of a month report, cut at today.
type monthRow struct {
	Label    string   `json:"label"`    // the table's: "1 ноя", "2 – 8 ноя", "16 – 22 ноя"
	Text     string   `json:"text"`     // the text export's, ending today: "2–8 ноя", "16 ноя"
	From     string   `json:"from"`     // first day, inside the month
	To       string   `json:"to"`       // last day, inside the month and not after today
	Workdays int      `json:"workdays"` // working days From..To
	Plan     int64    `json:"plan"`     // plan minutes From..To, rounded half up per row
	Me       int64    `json:"me"`
	Fact     int64    `json:"fact"` // counted toward the goal
	Agents   int64    `json:"agents"`
	Current  bool     `json:"current,omitempty"`
	Notes    []string `json:"notes"` // the project's segment notes in the row, in time order, repeats dropped
}

// monthReport is a project's month: the one place its minutes are added up for the screen, the text and the CSV.
type monthReport struct {
	Project    string           `json:"project"`
	Month      string           `json:"month"` // YYYY-MM
	Today      string           `json:"today"`
	Running    bool             `json:"running"` // today is in the month
	Rows       []monthRow       `json:"rows"`
	Total      monthRow         `json:"total"`      // field by field the sum of Rows (Label, Text, Notes empty)
	Goal       *Goal            `json:"goal"`       // in force on the last row's day
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
func buildMonth(project, month, rows, today string, days []planDay, notes []Segment, s Settings) monthReport {
	first, _ := time.Parse("2006-01", month)
	last := first.AddDate(0, 1, -1)
	date := func(t time.Time) string { return t.Format(time.DateOnly) }
	whole := sumGoal(days, project, today)
	m := monthReport{Project: project, Month: month, Today: today, Running: date(first) <= today && today <= date(last),
		Rows: []monthRow{}, Total: monthRow{Notes: []string{}}, Plan: whole.Plan, PlanDays: whole.Days, Holidays: []string{}}
	for _, pd := range days {
		if pd.Holiday {
			m.Holidays = append(m.Holidays, pd.Day)
		}
	}
	notes = slices.SortedStableFunc(slices.Values(notes), func(a, b Segment) int { return cmp.Compare(a.Start, b.Start) })
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
			Current: date(upTo) == today, Notes: []string{}}
		in := slices.DeleteFunc(slices.Clone(days), func(pd planDay) bool { return pd.Day < r.From || pd.Day > r.To })
		sum := sumGoal(in, project, today)
		r.Plan, r.Me, r.Fact = sum.Plan, sum.Me, sum.Fact
		for _, pd := range in {
			r.Agents += pd.Agents[project]
			if pd.Work {
				r.Workdays++
			}
		}
		for _, n := range notes {
			if day := s.DayOf(n.Start); n.Project == project && n.Note != "" && r.From <= day && day <= r.To && !slices.Contains(r.Notes, n.Note) {
				r.Notes = append(r.Notes, n.Note)
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

// monthReport is a project's month up to today with rows by week or day. Callers hold mu.
func (d *Daemon) monthReport(project, month, rows string) (monthReport, error) {
	first, err := time.Parse("2006-01", month)
	if err != nil || first.Format("2006-01") != month {
		return monthReport{}, fail(http.StatusBadRequest, "month: want YYYY-MM")
	}
	p := d.project(project)
	if p == nil {
		return monthReport{}, fail(http.StatusBadRequest, "unknown project %q", project)
	}
	rows = cmp.Or(rows, "week")
	if rows != "week" && rows != "day" {
		return monthReport{}, fail(http.StatusBadRequest, "rows: want week or day")
	}
	today := d.S.DayOf(nowMs())
	firstDay, lastDay := first.Format(time.DateOnly), first.AddDate(0, 1, -1).Format(time.DateOnly)
	if firstDay > today {
		return monthReport{}, fail(http.StatusBadRequest, "month: not started yet")
	}
	days, err := d.planRange(firstDay, lastDay)
	if err != nil {
		return monthReport{}, err
	}
	// ponytail: planRange reads the month's timeline too; this second read is for notes and activities.
	from, _, err := dayRange(d.S, firstDay)
	if err != nil {
		return monthReport{}, err
	}
	_, to, err := dayRange(d.S, lastDay)
	if err != nil {
		return monthReport{}, err
	}
	tl, _, err := d.timeline(from, to)
	if err != nil {
		return monthReport{}, err
	}
	m := buildMonth(project, month, rows, today, days, tl.Me, d.S)
	m.Goal = goalOn(p.Goals, m.Rows[len(m.Rows)-1].To)
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
	all, err := d.Store.Bonuses()
	if err != nil {
		return monthReport{}, err
	}
	var bonuses []Bonus
	for _, b := range all {
		if b.Project == project && b.Month == month {
			bonuses = append(bonuses, b)
		}
	}
	m.Pay = monthPay(payOn(p.Pay, month), m.Total.Me, m.Plan, bonuses)
	offs, err := d.Store.DaysOff()
	if err != nil {
		return monthReport{}, err
	}
	m.DaysOff = []DayOff{}
	for _, o := range offs {
		if o.From <= lastDay && firstDay <= o.To && slices.Contains(o.Projects, project) {
			m.DaysOff = append(m.DaysOff, o)
		}
	}
	var names []string
	for _, pd := range days {
		names = append(names, pd.Day)
	}
	m.NoCalendar = d.Cal.Missing(names)
	return m, nil
}

func (d *Daemon) getMonthReport(r *http.Request) (int, any, error) {
	q := r.URL.Query()
	m, err := d.monthReport(q.Get("project"), r.PathValue("month"), q.Get("rows"))
	return http.StatusOK, m, err
}
