package main

import (
	"cmp"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

var activityLabels = map[string]string{
	"development": "Разработка", "architecture": "Проектирование", "design": "Дизайн",
	"documentation": "Документация", "study": "Учёба", "": "Без вида",
}

var monthNames = [12]string{"январь", "февраль", "март", "апрель", "май", "июнь", "июль", "август", "сентябрь", "октябрь", "ноябрь", "декабрь"}

// exportOpts is what the text and the CSV carry besides my time.
type exportOpts struct {
	Notes  bool `json:"notes"`
	Pay    bool `json:"pay"`
	Agents bool `json:"agents"`
	Goal   bool `json:"goal"`
}

// ruMin is mins with 0м for no time.
func ruMin(m int64) string {
	if m == 0 {
		return "0м"
	}
	return mins(m)
}

// money is whole rubles with plain spaces between thousands: 100 000 ₽.
func money(rub int64) string {
	s := strconv.FormatInt(rub, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + " " + s[i:]
	}
	return s + " ₽"
}

// payLine is the text export's pay: base, bonuses, total; "" without a payment.
func payLine(p *payment) string {
	if p == nil {
		return ""
	}
	line := "Оплата: "
	switch p.Kind {
	case "fixed":
		line += money(p.Base)
	case "hourly":
		line += fmt.Sprintf("%s (%s × %s/ч)", money(p.Base), ruMin(p.Minutes), money(p.Rate))
	}
	if len(p.Bonuses) == 0 {
		return line
	}
	word := "премия"
	if len(p.Bonuses) > 1 {
		word = "премии"
	}
	if p.Kind == "" {
		return line + word + " " + money(p.BonusSum)
	}
	return line + fmt.Sprintf(" + %s %s = %s", word, money(p.BonusSum), money(p.Total))
}

func monthOf(m monthReport) time.Time {
	t, _ := time.Parse("2006-01", m.Month)
	return t
}

// exportText is the month as the client reads it; rows without time or notes are left out.
func exportText(name string, m monthReport, o exportOpts) string {
	first := monthOf(m)
	on := ""
	if m.Running {
		today, _ := time.Parse(time.DateOnly, m.Today)
		on = " на " + today.Format("02.01")
	}
	goal := o.Goal && m.Goal != nil
	var b strings.Builder
	fmt.Fprintf(&b, "%s — %s %d\n", name, monthNames[first.Month()-1], first.Year())
	fmt.Fprintf(&b, "Отработано%s: %s\n", on, ruMin(m.Total.Me))
	if goal {
		fmt.Fprintf(&b, "Цель%s: %s · разница %s\n", on, ruMin(m.Total.Plan), signed(m.Total.Fact-m.Total.Plan))
	}
	b.WriteString("\n")
	for _, r := range m.Rows {
		if r.Me == 0 && (!o.Notes || len(r.Notes) == 0) {
			continue
		}
		b.WriteString(r.Text + " · " + ruMin(r.Me))
		if o.Agents {
			b.WriteString(" · агенты " + ruMin(r.Agents))
		}
		if goal {
			b.WriteString(" · цель " + mins(r.Plan) + " · " + signed(r.Fact-r.Plan)) // mins(0) is —
		}
		b.WriteString("\n")
		if o.Notes {
			for _, n := range r.Notes {
				b.WriteString("  — " + n + "\n")
			}
		}
	}
	if o.Pay && m.Pay != nil {
		b.WriteString("\n" + payLine(m.Pay) + "\n")
	}
	return b.String()
}

// textCell keeps Excel from reading a text cell as a formula.
func textCell(s string) string {
	if s != "" && strings.IndexByte("=+-@", s[0]) >= 0 {
		return "'" + s
	}
	return s
}

// exportCSV is the month for Russian Excel: BOM, ';', CRLF, dates as 02.11.2026, whole minutes, rubles as bare digits.
func exportCSV(m monthReport, o exportOpts) string {
	num := func(v int64) string { return strconv.FormatInt(v, 10) }
	type column struct {
		head string
		cell func(monthRow) string
	}
	var cols []column
	goal := o.Goal && m.Goal != nil
	if goal {
		cols = append(cols, column{"Раб. дн.", func(r monthRow) string { return strconv.Itoa(r.Workdays) }},
			column{"Цель, мин", func(r monthRow) string { return num(r.Plan) }})
	}
	cols = append(cols, column{"Моё, мин", func(r monthRow) string { return num(r.Me) }})
	if goal {
		cols = append(cols, column{"Разница, мин", func(r monthRow) string { return num(r.Fact - r.Plan) }})
	}
	if o.Agents {
		cols = append(cols, column{"Агенты, мин", func(r monthRow) string { return num(r.Agents) }})
	}
	if o.Notes {
		cols = append(cols, column{"Заметки", func(r monthRow) string { return textCell(strings.Join(r.Notes, "; ")) }})
	}
	date := func(day string) string {
		t, _ := time.Parse(time.DateOnly, day)
		return t.Format("02.01.2006")
	}
	var b strings.Builder
	b.WriteString("\ufeff")
	w := csv.NewWriter(&b)
	w.Comma, w.UseCRLF = ';', true
	write := func(from, to string, r *monthRow) {
		rec := []string{from, to}
		for _, c := range cols {
			if r == nil {
				rec = append(rec, c.head)
			} else {
				rec = append(rec, c.cell(*r))
			}
		}
		w.Write(rec)
	}
	write("С", "По", nil)
	for _, r := range m.Rows {
		write(date(r.From), date(r.To), &r)
	}
	write("Итого", "", &m.Total)
	if p := m.Pay; o.Pay && p != nil {
		first := monthOf(m)
		w.Write([]string{})
		switch p.Kind {
		case "fixed":
			w.Write([]string{"Фикс за " + monthNames[first.Month()-1], num(p.Base)})
		case "hourly":
			w.Write([]string{fmt.Sprintf("Почасовая: %s × %d ₽/ч", ruMin(p.Minutes), p.Rate), num(p.Base)})
		}
		for _, bn := range p.Bonuses {
			w.Write([]string{fmt.Sprintf("Премия: %s (%d %s)", bn.Title, bn.Day, shortMonths[first.Month()-1]), num(bn.Amount)})
		}
		w.Write([]string{"Итого к оплате", num(p.Total)})
	}
	w.Flush()
	return b.String()
}

// postExport renders a project's month report as text or CSV. It never closes anything.
func (d *Daemon) postExport(r *http.Request) (int, any, error) {
	var req struct {
		Project string `json:"project"`
		Month   string `json:"month"`
		Rows    string `json:"rows"`
		Format  string `json:"format"`
		exportOpts
	}
	if err := decode(r, &req); err != nil {
		return 0, nil, err
	}
	if req.Format != "text" && req.Format != "csv" {
		return 0, nil, fail(http.StatusBadRequest, "format: want text or csv")
	}
	m, err := d.monthReport(req.Project, req.Month, req.Rows)
	if err != nil {
		return 0, nil, err
	}
	content := exportCSV(m, req.exportOpts)
	if req.Format == "text" {
		p := d.project(req.Project)
		content = exportText(cmp.Or(p.Name, p.ID), m, req.exportOpts)
	}
	return http.StatusOK, map[string]string{"content": content}, nil
}
