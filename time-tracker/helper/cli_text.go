package main

import (
	"cmp"
	"encoding/json"
	"fmt"
	"io"
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
		_, err := fmt.Fprintf(w, "сохранено в %s, период #%d закрыт\n", out, ex.Period)
		return err
	}
}

type exportResult struct {
	Content string `json:"content"`
	Period  int64  `json:"period"`
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

func showUnassigned(w io.Writer, resp []byte) error {
	var list []unassigned
	if err := json.Unmarshal(resp, &list); err != nil {
		return err
	}
	for _, u := range list {
		fmt.Fprintf(w, "%8s  %-5s  %s\n", dur(u.Ms), u.Kind, strings.TrimSpace(u.Value+" "+u.Exe))
	}
	return nil
}

func showPeriods(w io.Writer, resp []byte) error {
	var list []periodInfo
	if err := json.Unmarshal(resp, &list); err != nil {
		return err
	}
	for _, p := range list {
		fmt.Fprintf(w, "#%d %s %s – %s, выгружен %s, правила изменили бы на %+dм\n",
			p.ID, p.Project, clock(p.From), clock(p.To), clock(p.ExportedAt), p.DeltaMs/60_000)
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

// showHistory prints corrections (undone with correct rm) and rule changes (history undo) newest first.
func showHistory(w io.Writer, resp []byte) error {
	var h history
	if err := json.Unmarshal(resp, &h); err != nil {
		return err
	}
	type line struct {
		at   int64
		kind string
		id   int64
	}
	var lines []line
	for _, c := range h.Corrections {
		lines = append(lines, line{cmp.Or(c.Made, c.Start), "correct " + c.Kind, c.ID})
	}
	for _, c := range h.Changes {
		lines = append(lines, line{c.At, c.Kind, c.ID})
	}
	slices.SortStableFunc(lines, func(a, b line) int { return cmp.Compare(b.at, a.at) })
	for _, l := range lines {
		fmt.Fprintf(w, "%s  %s  #%d\n", time.UnixMilli(l.at).Format("02.01 15:04"), l.kind, l.id)
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
