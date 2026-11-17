package main

import (
	"net/http"
	"slices"
	"time"
)

// closedBy returns the first closed period, of the project or of any project when project is "",
// that [s, e) overlaps; a month record closes every project.
func closedBy(periods []Period, project string, s, e int64) *Period {
	for i, p := range periods {
		if (project == "" || p.Project == project || p.Month != "") && s < p.To && p.From < e {
			return &periods[i]
		}
	}
	return nil
}

// closedMonth returns the first month record among periods whose month is in first..last (days or
// months: only YYYY-MM counts).
func closedMonth(periods []Period, first, last string) *Period {
	first, last = first[:min(7, len(first))], last[:min(7, len(last))]
	for i, p := range periods {
		if p.Month != "" && first <= p.Month && p.Month <= last {
			return &periods[i]
		}
	}
	return nil
}

// refuseClosed answers 409 closed when a month in first..last is closed: its days off and bonuses stay.
func (d *Daemon) refuseClosed(first, last string) error {
	periods, err := d.Store.Periods()
	if err != nil {
		return err
	}
	if p := closedMonth(periods, first, last); p != nil {
		return closedPeriod(p)
	}
	return nil
}

// monthRecord is the month's record in force; nil when the month is open.
func (d *Daemon) monthRecord(month string) (*Period, error) {
	periods, err := d.Store.Periods()
	if err != nil {
		return nil, err
	}
	return closedMonth(periods, month, month), nil
}

type periodInfo struct {
	ID         int64  `json:"id"`
	Project    string `json:"project"`
	Month      string `json:"month,omitempty"`
	From       int64  `json:"from"`
	To         int64  `json:"to"`
	ExportedAt int64  `json:"exported_at"`
	DeltaMs    int64  `json:"delta_ms"` // v1 periods only
}

// getPeriods lists closed periods with how far fresh rules would move each v1 period's time.
func (d *Daemon) getPeriods(r *http.Request) (int, any, error) {
	periods, err := d.Store.Periods()
	if err != nil {
		return 0, nil, err
	}
	project := r.URL.Query().Get("project")
	out := []periodInfo{}
	for _, p := range periods {
		if project != "" && p.Project != project && p.Month == "" {
			continue
		}
		info := periodInfo{p.ID, p.Project, p.Month, p.From, p.To, p.ExportedAt, 0}
		if p.Month == "" {
			tl, err := d.fresh(p.From, p.To)
			if err != nil {
				return 0, nil, err
			}
			if _, div := ApplyPeriods(tl, []Period{p}); len(div) > 0 {
				info.DeltaMs = div[0].DeltaMs
			}
		}
		out = append(out, info)
	}
	return http.StatusOK, out, nil
}

// reopenPeriod removes a v1 period; a month record is reopened as by its month.
func (d *Daemon) reopenPeriod(r *http.Request) (int, any, error) {
	id, err := intParam(r.PathValue("id"), "id")
	if err != nil {
		return 0, nil, err
	}
	periods, err := d.Store.Periods()
	if err != nil {
		return 0, nil, err
	}
	i := slices.IndexFunc(periods, func(p Period) bool { return p.ID == id })
	if i < 0 {
		return 0, nil, fail(http.StatusNotFound, "no period %d", id)
	}
	if periods[i].Month != "" {
		return http.StatusNoContent, nil, d.reopenMonth(periods[i])
	}
	return http.StatusNoContent, nil, d.Store.RemovePeriod(id)
}

// monthState is what Отчёт's month card and Закрыть месяц need to know about a month.
type monthState struct {
	Month    string      `json:"month"`
	From     int64       `json:"from"` // the start of the month's first day
	Over     bool        `json:"over"` // its last day has ended
	Closed   bool        `json:"closed"`
	Period   int64       `json:"period"`
	ClosedAt int64       `json:"closed_at"`
	Sent     bool        `json:"sent"`
	Other    int64       `json:"other"`     // Другое minutes in the month
	OtherDay string      `json:"other_day"` // the last day with any
	Edits    int         `json:"edits"`     // corrections starting in the month
	Holidays []string    `json:"holidays"`
	DaysOff  []DayOff    `json:"days_off"`
	NoBonus  []string    `json:"no_bonus"`       // projects with pay that month and no bonus
	Prev     *monthState `json:"prev,omitempty"` // the month before: Month, Closed, Sent only
}

func (d *Daemon) getMonthState(r *http.Request) (int, any, error) {
	month := r.PathValue("month")
	first, last, from, to, err := monthSpan(d.S, month)
	if err != nil {
		return 0, nil, err
	}
	today := d.S.DayOf(nowMs())
	st := monthState{Month: month, From: from, Over: today > last, Holidays: []string{}, NoBonus: []string{}}
	rec, err := d.monthRecord(month)
	if err != nil {
		return 0, nil, err
	}
	if rec != nil {
		st.Closed, st.Period, st.ClosedAt, st.Sent = true, rec.ID, rec.ExportedAt, rec.Sent
	}
	t, _ := time.Parse("2006-01", month)
	prevMonth := t.AddDate(0, -1, 0).Format("2006-01")
	prev, err := d.monthRecord(prevMonth)
	if err != nil {
		return 0, nil, err
	}
	st.Prev = &monthState{Month: prevMonth, Closed: prev != nil, Sent: prev != nil && prev.Sent}

	days, err := d.planRange(first, last)
	if err != nil {
		return 0, nil, err
	}
	for _, pd := range days {
		if m := pd.Me[""]; m > 0 {
			st.Other, st.OtherDay = st.Other+m, pd.Day
		}
		if pd.Holiday {
			st.Holidays = append(st.Holidays, pd.Day)
		}
	}
	cs, err := d.Store.Corrections()
	if err != nil {
		return 0, nil, err
	}
	for _, c := range cs {
		if from <= c.Start && c.Start < to {
			st.Edits++
		}
	}
	if st.DaysOff, err = d.daysOffIn(first, last, ""); err != nil {
		return 0, nil, err
	}
	var pays map[string]*payment
	if rec != nil {
		pays = rec.Pay
	} else if pays, err = d.monthPays(month, days, today); err != nil {
		return 0, nil, err
	}
	for _, p := range d.Config.Projects {
		if pay := pays[p.ID]; pay != nil && pay.Kind != "" && len(pay.Bonuses) == 0 {
			st.NoBonus = append(st.NoBonus, p.ID)
		}
	}
	return http.StatusOK, st, nil
}

// postMonthClose closes a month that is over for every project: a record freezes its timeline (v1 periods
// applied), its plan days and each project's payment.
func (d *Daemon) postMonthClose(r *http.Request) (int, any, error) {
	month := r.PathValue("month")
	first, last, from, to, err := monthSpan(d.S, month)
	if err != nil {
		return 0, nil, err
	}
	today := d.S.DayOf(nowMs())
	if today <= last {
		return 0, nil, fail(http.StatusConflict, "месяц ещё идёт")
	}
	rec, err := d.monthRecord(month)
	if err != nil {
		return 0, nil, err
	}
	if rec != nil {
		return 0, nil, fail(http.StatusConflict, "%s уже закрыт", monthTitle(month))
	}
	var req struct {
		Sent bool `json:"sent"`
	}
	if r.ContentLength != 0 { // no body is {}
		if err := decode(r, &req); err != nil {
			return 0, nil, err
		}
	}
	tl, _, err := d.timeline(from, to)
	if err != nil {
		return 0, nil, err
	}
	days, err := d.planRange(first, last)
	if err != nil {
		return 0, nil, err
	}
	pays, err := d.monthPays(month, days, today)
	if err != nil {
		return 0, nil, err
	}
	p := Snapshot(tl, "", from, to)
	p.Month, p.Sent, p.ExportedAt, p.Days, p.Goals, p.Pay = month, req.Sent, nowMs(), days, d.monthGoals(days, today), pays
	if p, err = d.Store.AddPeriod(p); err != nil {
		return 0, nil, err
	}
	d.logChange(Change{Kind: "month-close", Month: month, Period: p.ID, Sent: req.Sent})
	return http.StatusOK, map[string]int64{"period": p.ID}, nil
}

func (d *Daemon) postMonthReopen(r *http.Request) (int, any, error) {
	month := r.PathValue("month")
	if _, _, _, _, err := monthSpan(d.S, month); err != nil {
		return 0, nil, err
	}
	rec, err := d.monthRecord(month)
	if err != nil {
		return 0, nil, err
	}
	if rec == nil {
		return 0, nil, fail(http.StatusNotFound, "%s не закрыт", monthTitle(month))
	}
	return http.StatusNoContent, nil, d.reopenMonth(*rec)
}

// reopenMonth marks the record reopened (it stays, for undo), its close change undone, and logs the reopen.
func (d *Daemon) reopenMonth(p Period) error {
	now := nowMs()
	if err := d.Store.SetReopened(p.ID, now); err != nil {
		return err
	}
	if err := d.markClose(p.ID, now); err != nil {
		return err
	}
	d.logChange(Change{Kind: "month-reopen", Month: p.Month, Period: p.ID})
	return nil
}

// markClose marks the change that closed record id undone at at; 0 puts it back.
func (d *Daemon) markClose(id, at int64) error {
	changes, err := d.Store.Changes()
	if err != nil {
		return err
	}
	for _, c := range changes {
		if c.Kind == "month-close" && c.Period == id {
			return d.Store.MarkUndone(c.ID, at)
		}
	}
	return nil
}

// undoMonth undoes a month-close (a reopen) or a month-reopen (the same record back in force, unless
// the month was closed again since).
func (d *Daemon) undoMonth(ch Change) error {
	rec, err := d.monthRecord(ch.Month)
	if err != nil {
		return err
	}
	if ch.Kind == "month-close" {
		if rec == nil || rec.ID != ch.Period {
			return fail(http.StatusNotFound, "%s не закрыт", monthTitle(ch.Month))
		}
		return d.reopenMonth(*rec)
	}
	if rec != nil {
		return fail(http.StatusConflict, "%s уже закрыт заново", monthTitle(ch.Month))
	}
	if err := d.Store.SetReopened(ch.Period, 0); err != nil {
		return err
	}
	return d.markClose(ch.Period, 0)
}
