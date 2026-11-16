package main

import (
	"encoding/csv"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const agentsRow = "agents (для справки)"

var activityLabels = map[string]string{
	"development": "Разработка", "architecture": "Проектирование", "design": "Дизайн",
	"documentation": "Документация", "study": "Учёба", "": "Без вида",
}

var activityOrder = []string{"development", "architecture", "design", "documentation", "study"}

// row is one (day, activity) cell of the export.
type row struct {
	day, activity string
	ms            int64
}

func (r row) minutes() int64 { return RoundMinutes(r.ms, 1) }

func activityRank(a string) int {
	for i, id := range activityOrder {
		if id == a {
			return i
		}
	}
	if a == "" {
		return len(activityOrder) + 1
	}
	return len(activityOrder)
}

func exportRows(p ProjectTotal) (rows []row, agents []row) {
	for d, dt := range p.Days {
		for a, ms := range dt.Activities {
			if ms > 0 {
				rows = append(rows, row{d, a, ms})
			}
		}
		if dt.Agents > 0 {
			agents = append(agents, row{d, agentsRow, dt.Agents})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].day != rows[j].day {
			return rows[i].day < rows[j].day
		}
		if ri, rj := activityRank(rows[i].activity), activityRank(rows[j].activity); ri != rj {
			return ri < rj
		}
		return rows[i].activity < rows[j].activity
	})
	sort.Slice(agents, func(i, j int) bool { return agents[i].day < agents[j].day })
	return rows, agents
}

// noteComments joins each (day, activity) cell's notes with "; ", repeats dropped. A note belongs to
// the day its segment starts in.
func noteComments(notes []Segment, s Settings) map[[2]string]string {
	sorted := append([]Segment(nil), notes...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Start < sorted[j].Start })
	out := map[[2]string]string{}
	seen := map[[3]string]bool{}
	for _, n := range sorted {
		k := [2]string{s.DayOf(n.Start), n.Activity}
		if n.Note == "" || seen[[3]string{k[0], k[1], n.Note}] {
			continue
		}
		seen[[3]string{k[0], k[1], n.Note}] = true
		if out[k] != "" {
			out[k] += "; "
		}
		out[k] += n.Note
	}
	return out
}

// ExportCSV takes Settings (not in the brief's signature) because a note's day needs day_start.
func ExportCSV(p ProjectTotal, notes []Segment, step int, agents bool, s Settings) string {
	rows, agentRows := exportRows(p)
	comments := noteComments(notes, s)
	var sb strings.Builder
	w := csv.NewWriter(&sb)
	i := func(n int64) string { return strconv.FormatInt(n, 10) }
	w.Write([]string{"date", "activity", "minutes", "rounded_minutes", "comment"})
	var sumMin, sumRounded int64
	for _, r := range rows {
		rounded := RoundMinutes(r.ms, step)
		sumMin += r.minutes()
		sumRounded += rounded
		w.Write([]string{r.day, r.activity, i(r.minutes()), i(rounded), comments[[2]string{r.day, r.activity}]})
	}
	if agents {
		for _, r := range agentRows {
			w.Write([]string{r.day, r.activity, i(r.minutes()), i(RoundMinutes(r.ms, step)), ""})
		}
	}
	w.Write([]string{"total", "", i(sumMin), i(sumRounded), ""})
	w.Flush()
	return sb.String()
}

func mdCell(s string) string {
	return strings.NewReplacer("|", `\|`, "\n", " ").Replace(s)
}

func ExportMarkdown(name string, from, to int64, p ProjectTotal, notes []Segment, step int, agents bool, s Settings) string {
	rows, agentRows := exportRows(p)
	comments := noteComments(notes, s)
	var sb strings.Builder
	fmt.Fprintf(&sb, "# %s: %s – %s\n", name, s.DayOf(from), s.DayOf(to-1))
	var total int64
	lastDay := ""
	for _, r := range rows {
		if r.day != lastDay {
			fmt.Fprintf(&sb, "\n## %s\n\n| Вид | Минуты | Комментарий |\n|---|---|---|\n", r.day)
			lastDay = r.day
		}
		rounded := RoundMinutes(r.ms, step)
		total += rounded
		label := activityLabels[r.activity]
		if label == "" {
			label = r.activity
		}
		fmt.Fprintf(&sb, "| %s | %d | %s |\n", label, rounded, mdCell(comments[[2]string{r.day, r.activity}]))
	}
	fmt.Fprintf(&sb, "\n**Итого: %d мин**\n", total)
	if agents {
		var a int64
		for _, r := range agentRows {
			a += RoundMinutes(r.ms, step)
		}
		fmt.Fprintf(&sb, "\nВремя агентов (для справки): %d мин\n", a)
	}
	return sb.String()
}
