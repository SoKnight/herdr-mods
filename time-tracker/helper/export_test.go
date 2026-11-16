package main

import (
	"strings"
	"testing"
)

func exportFixture() ProjectTotal {
	tl := Timeline{
		Me: []Segment{
			{Start: at(0), End: at(22), Project: "easylauncher", Activity: "development"},
			{Start: at(30), End: at(60), Project: "easylauncher", Activity: "design"},
		},
		Agents: []AgentSeg{{Start: at(0), End: at(40), Project: "easylauncher"}},
	}
	return Summarize(tl, tsettings(), []string{"easylauncher"}).Projects[0]
}

func TestCSVAgentsLineNotInTotal(t *testing.T) {
	p := exportFixture()
	with := ExportCSV(p, nil, 15, true, tsettings())
	want := "date,activity,minutes,rounded_minutes,comment\n" +
		"2026-11-16,development,22,15,\n" +
		"2026-11-16,design,30,30,\n" +
		"2026-11-16,agents (для справки),40,45,\n" +
		"total,,52,45,\n"
	assertEq(t, with, want)
	without := ExportCSV(p, nil, 15, false, tsettings())
	if strings.Contains(without, "agents") {
		t.Errorf("agents row without flag: %s", without)
	}
	assertEq(t, strings.HasSuffix(without, "total,,52,45,\n"), true)
}

func TestCSVQuotesNotes(t *testing.T) {
	p := exportFixture()
	notes := []Segment{{Start: at(1), End: at(5), Activity: "development", Note: `a, "b"`}, {Start: at(6), End: at(7), Activity: "development", Note: "c"}}
	out := ExportCSV(p, notes, 1, false, tsettings())
	assertEq(t, strings.Contains(out, `2026-11-16,development,22,22,"a, ""b""; c"`), true)
}

func TestMarkdownHasNotes(t *testing.T) {
	p := exportFixture()
	notes := []Segment{{Start: at(1), End: at(5), Activity: "development", Note: "созвон с заказчиком"}}
	md := ExportMarkdown("EasyLauncher", at(0), mskAt(2026, 11, 17, 4, 0), p, notes, 15, true, tsettings())
	for _, w := range []string{
		"# EasyLauncher: 2026-11-16 – 2026-11-16",
		"| Вид | Минуты | Комментарий |",
		"| Разработка | 15 | созвон с заказчиком |",
		"| Дизайн | 30 |",
		"**Итого: 45 мин**",
		"Время агентов (для справки): 45 мин",
	} {
		if !strings.Contains(md, w) {
			t.Errorf("missing %q in:\n%s", w, md)
		}
	}
}

func TestCSVTwoDaysKeepsOrder(t *testing.T) {
	tl := Timeline{Me: []Segment{
		{Start: mskAt(2026, 11, 17, 10, 0), End: mskAt(2026, 11, 17, 11, 0), Project: "p", Activity: "design"},
		{Start: mskAt(2026, 11, 16, 12, 0), End: mskAt(2026, 11, 16, 13, 0), Project: "p", Activity: "study"},
		{Start: mskAt(2026, 11, 16, 10, 0), End: mskAt(2026, 11, 16, 11, 0), Project: "p", Activity: "development"},
	}}
	out := ExportCSV(Summarize(tl, tsettings(), []string{"p"}).Projects[0], nil, 1, false, tsettings())
	want := "date,activity,minutes,rounded_minutes,comment\n" +
		"2026-11-16,development,60,60,\n2026-11-16,study,60,60,\n2026-11-17,design,60,60,\ntotal,,180,180,\n"
	assertEq(t, out, want)
}
