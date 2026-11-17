package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"time"
)

func dur(ms int64) string {
	mins := RoundMinutes(ms, 1)
	if mins >= 60 {
		return fmt.Sprintf("%dч %dм", mins/60, mins%60)
	}
	return fmt.Sprintf("%dм", mins)
}

func clock(ms int64) string { return time.UnixMilli(ms).Format("2006-01-02 15:04") }

// mins is the panel's duration format over whole minutes: —, Nм, Nч, Nч Mм.
func mins(m int64) string {
	switch {
	case m == 0:
		return "—"
	case m < 60:
		return fmt.Sprintf("%dм", m)
	case m%60 == 0:
		return fmt.Sprintf("%dч", m/60)
	}
	return fmt.Sprintf("%dч %dм", m/60, m%60)
}

// signed is mins with + or − (U+2212), 0 for no difference.
func signed(m int64) string {
	switch {
	case m == 0:
		return "0"
	case m < 0:
		return "−" + mins(-m)
	}
	return "+" + mins(m)
}

func goalLine(project string, s goalSum) string {
	return fmt.Sprintf("%s  %s / %s  баланс %s  прогноз %s", project, mins(s.Fact), mins(s.Plan), signed(s.Balance), mins(s.Forecast))
}

func showPlan(w io.Writer, resp []byte) error {
	var v struct {
		Sums map[string]goalSum `json:"sums"`
	}
	if err := json.Unmarshal(resp, &v); err != nil {
		return err
	}
	ids := slices.Sorted(maps.Keys(v.Sums))
	for _, id := range ids {
		fmt.Fprintln(w, goalLine(id, v.Sums[id]))
	}
	return nil
}

func showGoals(w io.Writer, resp []byte) error {
	var v struct {
		Goals []goalInfo `json:"goals"`
	}
	if err := json.Unmarshal(resp, &v); err != nil {
		return err
	}
	for _, g := range v.Goals {
		fmt.Fprintln(w, goalLine(g.Project, g.Month))
	}
	return nil
}

// showMonth prints the month's rows, Итого and the pay line; without a goal, working days, goal and difference are left out.
func showMonth(w io.Writer, resp []byte) error {
	var m monthReport
	if err := json.Unmarshal(resp, &m); err != nil {
		return err
	}
	line := func(label string, r monthRow) {
		if m.Goal == nil {
			fmt.Fprintf(w, "%s  моё %s  агенты %s\n", label, mins(r.Me), mins(r.Agents))
			return
		}
		fmt.Fprintf(w, "%s  %d дн.  цель %s  моё %s  %s  агенты %s\n", label, r.Workdays, mins(r.Plan), mins(r.Me),
			signed(r.Fact-r.Plan), mins(r.Agents))
	}
	for _, r := range m.Rows {
		line(r.Label, r)
	}
	line("Итого", m.Total)
	if m.Pay != nil {
		fmt.Fprintln(w, payLine(m.Pay))
	}
	return nil
}

func showStatus(w io.Writer, resp []byte) error {
	var st struct {
		Me      int64  `json:"today_me_ms"`
		Agents  int64  `json:"today_agents_ms"`
		Project string `json:"project"`
	}
	if err := json.Unmarshal(resp, &st); err != nil {
		return err
	}
	line := fmt.Sprintf("сегодня: моё %s, агенты %s", dur(st.Me), dur(st.Agents))
	if st.Project != "" {
		line += " · " + st.Project
	}
	_, err := fmt.Fprintln(w, line)
	return err
}

func showSession(w io.Writer, resp []byte) error {
	var v struct {
		Session *Session `json:"session"`
	}
	if err := json.Unmarshal(resp, &v); err != nil {
		return err
	}
	if v.Session == nil || !v.Session.Open {
		_, err := fmt.Fprintln(w, "сессии нет")
		return err
	}
	_, err := fmt.Fprintf(w, "сессия с %s · моё %s · перерывы %s\n", time.UnixMilli(v.Session.Start).Format("15:04"),
		dur(v.Session.Me), dur(v.Session.Breaks))
	return err
}

func printReport(w io.Writer, r Report) {
	fmt.Fprintf(w, "аптайм %s · моё %s · агенты %s\n", dur(r.Uptime), dur(r.Me), dur(r.Agents))
	for _, p := range r.Projects {
		name := p.Project
		if name == "" {
			name = "без проекта"
		}
		fmt.Fprintf(w, "  %s: моё %s, агенты %s\n", name, dur(p.Me), dur(p.Agents))
	}
}

func showReport(w io.Writer, resp []byte) error {
	var r Report
	if err := json.Unmarshal(resp, &r); err != nil {
		return err
	}
	printReport(w, r)
	return nil
}

func showTimeline(w io.Writer, resp []byte) error {
	var v struct {
		Day    string `json:"day"`
		Report Report `json:"report"`
	}
	if err := json.Unmarshal(resp, &v); err != nil {
		return err
	}
	fmt.Fprintln(w, v.Day)
	printReport(w, v.Report)
	return nil
}

func showExport(out string) func(io.Writer, []byte) error {
	return func(w io.Writer, resp []byte) error {
		var ex exportResult
		if err := json.Unmarshal(resp, &ex); err != nil {
			return err
		}
		if out == "" {
			_, err := fmt.Fprint(w, ex.Content)
			return err
		}
		_, err := fmt.Fprintf(w, "сохранено в %s\n", out)
		return err
	}
}

type exportResult struct {
	Content string `json:"content"`
}

func correctionLine(c Correction) string {
	what := strings.Join(slices.DeleteFunc([]string{c.Project, c.Activity, c.Note}, func(s string) bool { return s == "" }), ", ")
	return fmt.Sprintf("#%d %s %s %s – %s %s", c.ID, c.Kind, c.Target, clock(c.Start), clock(c.End), what)
}

func showCorrection(w io.Writer, resp []byte) error {
	var c Correction
	if err := json.Unmarshal(resp, &c); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, correctionLine(c))
	return err
}

func showCorrections(w io.Writer, resp []byte) error {
	var cs []Correction
	if err := json.Unmarshal(resp, &cs); err != nil {
		return err
	}
	for _, c := range cs {
		fmt.Fprintln(w, correctionLine(c))
	}
	return nil
}

// showInbox prints the day's Другое windows, each with its suggestion.
func showInbox(w io.Writer, resp []byte) error {
	var v struct {
		Day   string      `json:"day"`
		Ms    int64       `json:"ms"`
		Items []inboxItem `json:"items"`
	}
	if err := json.Unmarshal(resp, &v); err != nil {
		return err
	}
	fmt.Fprintf(w, "%s · другое %s\n", v.Day, dur(v.Ms))
	for _, it := range v.Items {
		what := strings.Join(slices.DeleteFunc([]string{it.App, it.Title, it.Path, it.Server}, func(s string) bool { return s == "" }), " · ")
		if s := it.Suggest; s != nil {
			what += fmt.Sprintf("  → %s (%s %s)", s.Project, s.Rule.Kind, s.Rule.Value)
		}
		fmt.Fprintf(w, "%8s  %s\n", dur(it.Ms), what)
	}
	return nil
}

func showInboxApplied(w io.Writer, resp []byte) error {
	var v struct{ Rules, Corrections int }
	if err := json.Unmarshal(resp, &v); err != nil {
		return err
	}
	_, err := fmt.Fprintf(w, "правил %d, правок %d\n", v.Rules, v.Corrections)
	return err
}

func showPeriods(w io.Writer, resp []byte) error {
	var list []periodInfo
	if err := json.Unmarshal(resp, &list); err != nil {
		return err
	}
	for _, p := range list {
		if p.Month != "" {
			fmt.Fprintf(w, "#%d  %s · все проекты  закрыт %s\n", p.ID, monthLabel(p.Month), shortDay(p.ExportedAt))
			continue
		}
		fmt.Fprintf(w, "#%d %s %s – %s, выгружен %s, правила изменили бы на %+dм\n",
			p.ID, p.Project, clock(p.From), clock(p.To), clock(p.ExportedAt), p.DeltaMs/60_000)
	}
	return nil
}

// monthLabel is "ноябрь 2026"; shortDay "2 дек".
func monthLabel(month string) string {
	t, _ := time.Parse("2006-01", month)
	return fmt.Sprintf("%s %d", monthNames[t.Month()-1], t.Year())
}

func shortDay(ms int64) string {
	t := time.UnixMilli(ms)
	return fmt.Sprintf("%d %s", t.Day(), shortMonths[t.Month()-1])
}

// showMonthState prints whether the month is closed and Закрыть месяц's checklist.
func showMonthState(w io.Writer, resp []byte) error {
	var s monthState
	if err := json.Unmarshal(resp, &s); err != nil {
		return err
	}
	state := "идёт"
	switch {
	case s.Closed && s.Sent:
		state = fmt.Sprintf("закрыт %s · отчёт отправлен", shortDay(s.ClosedAt))
	case s.Closed:
		state = "закрыт " + shortDay(s.ClosedAt)
	case s.Over:
		state = "не закрыт"
	}
	fmt.Fprintf(w, "%s · %s\n", monthLabel(s.Month), state)
	fmt.Fprintf(w, "другое %s · ручных правок %d · праздники %s · отгулов %d\n", mins(s.Other), s.Edits, strings.Join(s.Holidays, ", "), len(s.DaysOff))
	if len(s.NoBonus) > 0 {
		fmt.Fprintf(w, "премии не внесены: %s\n", strings.Join(s.NoBonus, ", "))
	}
	return nil
}

func showRules(w io.Writer, resp []byte) error {
	var v struct {
		Rules []ruleInfo `json:"rules"`
	}
	if err := json.Unmarshal(resp, &v); err != nil {
		return err
	}
	for _, r := range v.Rules {
		fmt.Fprintf(w, "#%d  %s  %s\n", r.ID, r.why(), dur(r.Ms))
	}
	return nil
}

func showPreview(w io.Writer, resp []byte) error {
	var p preview
	if err := json.Unmarshal(resp, &p); err != nil {
		return err
	}
	fmt.Fprintf(w, "за 7 дней: %s, окон %d\n", dur(p.Ms), p.Windows)
	for _, t := range p.Top {
		fmt.Fprintf(w, "%8s  %s\n", dur(t.Ms), t.Title)
	}
	if o := p.Overlap; o != nil {
		fmt.Fprintf(w, "%s из них оставит за собой #%d  %s\n", dur(o.Ms), o.Rule.ID, o.Rule.why())
	}
	return nil
}

// showHistory prints corrections, rule changes and days off newest first, each with the command that undoes it.
func showHistory(w io.Writer, resp []byte) error {
	var h history
	if err := json.Unmarshal(resp, &h); err != nil {
		return err
	}
	type line struct {
		at         int64
		kind, undo string
		id         int64
	}
	var lines []line
	for _, c := range h.Corrections {
		lines = append(lines, line{cmp.Or(c.Made, c.Start), "correct " + c.Kind, "correct rm", c.ID})
	}
	for _, c := range h.Changes {
		lines = append(lines, line{c.At, strings.TrimSpace(c.Kind + " " + cmp.Or(c.Project, c.Month)), "history undo", c.ID}) // goal-set easy, month-close 2026-11
	}
	for _, o := range h.DaysOff {
		lines = append(lines, line{o.Made, fmt.Sprintf("dayoff %s %s–%s", o.Kind, o.From, o.To), "dayoff rm", o.ID})
	}
	slices.SortStableFunc(lines, func(a, b line) int { return cmp.Compare(b.at, a.at) })
	for _, l := range lines {
		fmt.Fprintf(w, "%s  %s  %s %d\n", time.UnixMilli(l.at).Format("02.01 15:04"), l.kind, l.undo, l.id)
	}
	return nil
}

func showHints(w io.Writer, resp []byte) error {
	var hs []hint
	if err := json.Unmarshal(resp, &hs); err != nil {
		return err
	}
	for _, h := range hs {
		fmt.Fprintf(w, "%8s  %-9s  %s\n", dur(h.Ms), h.Kind, h.Value)
	}
	return nil
}

func showProjects(w io.Writer, resp []byte) error {
	var c catalog
	if err := json.Unmarshal(resp, &c); err != nil {
		return err
	}
	for _, p := range c.Projects {
		fmt.Fprintf(w, "%s  %s\n", p.ID, p.Name)
	}
	fmt.Fprintln(w, "виды деятельности:")
	for _, a := range c.Activities {
		fmt.Fprintf(w, "%s  %s\n", a.ID, a.Label)
	}
	return nil
}
