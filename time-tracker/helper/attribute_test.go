package main

import (
	"strings"
	"testing"
)

// Times in these tests are minutes from 10:00.
var base = mskAt(2026, 11, 16, 10, 0)

func at(minute int) int64 { return base + int64(minute)*60_000 }

func m(s, e int) Interval { return Interval{at(s), at(e)} }

func win(exe, title string, s, e int) Span {
	return Span{Kind: "window", Start: at(s), End: at(e), Exe: exe, Title: title}
}

func inp(s, e int) Span    { return Span{Kind: "input", Start: at(s), End: at(e)} }
func lock(s, e int) Span   { return Span{Kind: "lock", Start: at(s), End: at(e)} }
func call(s, e int) Span   { return Span{Kind: "call", Start: at(s), End: at(e), Exe: "telemost.exe"} }
func spans(sp ...Span) Raw { return Raw{Spans: sp} }

func beat(source string, minute int, path, entity string) Heartbeat {
	return Heartbeat{Source: source, Time: at(minute), ProjectPath: path, Entity: entity}
}

func (r Raw) with(b ...Heartbeat) Raw {
	r.Beats = append(r.Beats, b...)
	return r
}

func testRules(t *testing.T) *Rules {
	return mustCompile(t, Config{
		Projects: []Project{
			{ID: "easylauncher", Name: "EasyLauncher", Paths: []string{`C:\Dev\easyx`}},
			{ID: "tricky", Name: "Tricky Network", Titles: []string{"Tricky"}},
			{ID: "site", Name: "Site", URLs: []string{`^https://site\.example/`}},
			{ID: "study", Name: "Учёба", Apps: []string{"telemost.exe"}},
		},
		Apps: []AppRule{
			{Exe: "idea64.exe", Activity: "development", Source: "idea"},
			{Exe: "herdr-app.exe", Activity: "development", Source: "herdr"},
			{Exe: "firefox.exe", Source: "firefox"},
			{Exe: "telemost.exe", Activity: "study", Presence: "call"},
		},
		Files: []FileRule{{Match: `\.mdx?$`, Activity: "documentation"}},
	})
}

func attrRange(t *testing.T, raw Raw, from, to int) Timeline {
	t.Helper()
	s := DefaultSettings()
	s.Loc = msk
	return Attribute(raw, testRules(t), nil, s, at(from), at(to))
}

func attr(t *testing.T, raw Raw) Timeline { return attrRange(t, raw, -60, 240) }

// meIntervals glues touching Me segments, so presence can be checked apart from attribution.
func meIntervals(tl Timeline) []Interval {
	var out []Interval
	for _, s := range tl.Me {
		if n := len(out); n > 0 && out[n-1].End == s.Start {
			out[n-1].End = s.End
			continue
		}
		out = append(out, Interval{s.Start, s.End})
	}
	return out
}

func assertMeIntervals(t *testing.T, tl Timeline, want []Interval) {
	t.Helper()
	got := meIntervals(tl)
	if len(got) != len(want) {
		t.Fatalf("Me = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("Me = %v, want %v", got, want)
		}
	}
}

func assertMe(t *testing.T, tl Timeline, s, e int) {
	t.Helper()
	assertMeIntervals(t, tl, []Interval{m(s, e)})
}

// segAt returns the Me segment covering the given minute (plus seconds), failing if none does.
func segAt(t *testing.T, tl Timeline, minute int, sec int64) Segment {
	t.Helper()
	ms := at(minute) + sec*1000
	for _, s := range tl.Me {
		if s.Start <= ms && ms < s.End {
			return s
		}
	}
	t.Fatalf("no Me segment at %d:%02d", minute, sec)
	return Segment{}
}

func assertWhyHas(t *testing.T, s Segment, part string) {
	t.Helper()
	if !strings.Contains(s.Why, part) {
		t.Errorf("Why %q lacks %q", s.Why, part)
	}
}

func TestIdleUnderThresholdCountsWholeGap(t *testing.T) {
	tl := attr(t, spans(win("idea64.exe", "EasyLauncher – A.kt", 0, 60), inp(0, 10), inp(14, 30)))
	assertMe(t, tl, 0, 30) // 4-min gap < 5 → present 0..30
}

func TestIdleOverThresholdDropsWholeGap(t *testing.T) {
	tl := attr(t, spans(win("idea64.exe", "x", 0, 60), inp(0, 10), inp(16, 30)))
	assertMeIntervals(t, tl, []Interval{m(0, 10), m(16, 30)})
}

func TestIdleExactlyThresholdIsAbsence(t *testing.T) {
	tl := attr(t, spans(win("idea64.exe", "x", 0, 60), inp(0, 10), inp(15, 30)))
	assertMeIntervals(t, tl, []Interval{m(0, 10), m(15, 30)})
}

func TestCallCountsWithoutInput(t *testing.T) {
	tl := attr(t, spans(win("telemost.exe", "Лекция", 0, 90), call(0, 90), inp(0, 1)))
	assertMe(t, tl, 0, 90)
	for _, s := range tl.Me {
		assertEq(t, s.Project, "study")
		assertEq(t, s.Activity, "study")
		assertEq(t, s.App, "telemost.exe")
	}
}

func TestCallInBackgroundStillStudy(t *testing.T) {
	tl := attr(t, spans(call(0, 60), win("firefox.exe", "Mozilla Firefox", 0, 60)))
	assertMe(t, tl, 0, 60)
	s := segAt(t, tl, 30, 0)
	assertEq(t, s.Project, "study")
	assertEq(t, s.Activity, "study")
	assertEq(t, s.App, "telemost.exe")
	assertWhyHas(t, s, "звонок telemost.exe")
}

func TestLockIsAbsence(t *testing.T) {
	tl := attr(t, spans(win("idea64.exe", "x", 0, 30), inp(0, 30), lock(10, 20)))
	assertMeIntervals(t, tl, []Interval{m(0, 10), m(20, 30)})
}

func TestLockBeatsCall(t *testing.T) {
	tl := attr(t, spans(win("telemost.exe", "Лекция", 0, 30), call(0, 30), lock(10, 20)))
	assertMeIntervals(t, tl, []Interval{m(0, 10), m(20, 30)})
}

func TestBeatBeatsTitleOnlyForMatchingApp(t *testing.T) {
	b := beat("herdr", 0, `C:\Dev\easyx\EasyLauncher`, "")

	tl := attr(t, spans(win("idea64.exe", "Tricky", 0, 1), inp(0, 1)).with(b))
	s := segAt(t, tl, 0, 30)
	assertEq(t, s.Project, "tricky") // beat ignored: source herdr ≠ idea
	assertWhyHas(t, s, "заголовок")

	tl = attr(t, spans(win("herdr-app.exe", "Tricky", 0, 1), inp(0, 1)).with(b))
	s = segAt(t, tl, 0, 30)
	assertEq(t, s.Project, "easylauncher") // beat beats the title rule
	assertWhyHas(t, s, "сигнал herdr")
}

func TestBeatProjectByURLAndName(t *testing.T) {
	byURL := Heartbeat{Source: "firefox", Time: at(0), URL: "https://site.example/page"}
	byName := Heartbeat{Source: "firefox", Time: at(1), ProjectName: "easylauncher"}
	tl := attr(t, spans(win("firefox.exe", "x", 0, 2), inp(0, 2)).with(byURL, byName))
	assertEq(t, segAt(t, tl, 0, 30).Project, "site")
	assertEq(t, segAt(t, tl, 1, 30).Project, "easylauncher")
	assertWhyHas(t, segAt(t, tl, 1, 30), "имя проекта")
}

func TestBeatExpiresAfter120s(t *testing.T) {
	tl := attr(t, spans(win("herdr-app.exe", "herdr", 0, 30), inp(0, 30)).with(beat("herdr", 0, `C:\Dev\easyx`, "")))
	s := segAt(t, tl, 1, 59)
	assertEq(t, s.Project, "easylauncher")
	assertWhyHas(t, s, "сигнал herdr")
	s = segAt(t, tl, 2, 0)
	assertEq(t, s.Project, "easylauncher")
	assertWhyHas(t, s, "перенос")
	assertEq(t, segAt(t, tl, 16, 59).Project, "easylauncher")
	s = segAt(t, tl, 17, 0)
	assertEq(t, s.Project, "")
	assertWhyHas(t, s, "без проекта")
	assertMe(t, tl, 0, 30)
}

func TestNewerBeatReplacesOlderOfSameSource(t *testing.T) {
	tl := attr(t, spans(win("idea64.exe", "x", 0, 3), inp(0, 3)).with(
		beat("idea", 0, `C:\Dev\easyx`, ""),
		beat("idea", 1, "", ""), // still within the first beat's 120 s, but it replaces it
	))
	assertWhyHas(t, segAt(t, tl, 0, 30), "сигнал idea")
	s := segAt(t, tl, 1, 30)
	assertEq(t, s.Project, "easylauncher")
	assertWhyHas(t, s, "перенос")
}

func TestCarryStopsAfter15Minutes(t *testing.T) {
	tl := attr(t, spans(win("idea64.exe", "Tricky", 0, 10), win("notepad.exe", "x", 10, 40), inp(0, 40)))
	assertEq(t, segAt(t, tl, 24, 59).Project, "tricky")
	assertWhyHas(t, segAt(t, tl, 24, 59), "перенос")
	assertEq(t, segAt(t, tl, 25, 0).Project, "")
}

func TestCarryCountsFromLastPresentProjectAndCrossesFrom(t *testing.T) {
	// Data before from only feeds the carry; output starts at from.
	tl := attrRange(t, spans(win("idea64.exe", "Tricky", -10, 0), win("notepad.exe", "x", 0, 30), inp(-10, 30)), 0, 30)
	assertEq(t, tl.Me[0].Start, at(0))
	assertEq(t, segAt(t, tl, 14, 59).Project, "tricky")
	assertEq(t, segAt(t, tl, 15, 0).Project, "")
	assertMe(t, tl, 0, 30)
}

func TestOutputClippedToRange(t *testing.T) {
	tl := attrRange(t, spans(win("idea64.exe", "Tricky", 0, 60), inp(0, 60)), 10, 20)
	assertMe(t, tl, 10, 20)
}

func TestActivityFromEntityFile(t *testing.T) {
	tl := attr(t, spans(win("idea64.exe", "x", 0, 2), inp(0, 2)).with(
		beat("idea", 0, `C:\Dev\easyx`, "README.md"),
		beat("idea", 1, `C:\Dev\easyx`, "Main.kt"),
	))
	s := segAt(t, tl, 0, 30)
	assertEq(t, s.Activity, "documentation")
	assertWhyHas(t, s, "файл README.md")
	s = segAt(t, tl, 1, 30)
	assertEq(t, s.Activity, "development")
	assertWhyHas(t, s, "приложение idea64.exe")
	assertEq(t, s.Project, "easylauncher")
}

func TestActivityFromBeatWins(t *testing.T) {
	b := beat("idea", 0, `C:\Dev\easyx`, "README.md")
	b.Activity = "architecture"
	tl := attr(t, spans(win("idea64.exe", "x", 0, 1), inp(0, 1)).with(b))
	assertEq(t, segAt(t, tl, 0, 30).Activity, "architecture")
}

func TestSegmentsMergeAndCarryWhy(t *testing.T) {
	raw := spans(win("idea64.exe", "Tricky A", 0, 10), win("idea64.exe", "Tricky B", 10, 20), inp(0, 5), inp(5, 20))
	for i := 0; i < 20; i++ { // beats every 30 s without a project don't split segments
		raw = raw.with(Heartbeat{Source: "idea", Time: at(0) + int64(i)*30_000})
	}
	tl := attr(t, raw)
	assertEq(t, len(tl.Me), 2)
	assertEq(t, tl.Me[0].Title, "Tricky A")
	assertEq(t, tl.Me[1].Title, "Tricky B")
	for _, s := range tl.Me {
		assertEq(t, s.Project, "tricky")
		assertEq(t, s.Activity, "development")
		if s.Why == "" {
			t.Errorf("empty Why in %+v", s)
		}
	}
	assertMe(t, tl, 0, 20)
}

func TestWhyNeverEmptyWithoutProject(t *testing.T) {
	tl := attr(t, spans(win("notepad.exe", "x", 0, 10), inp(0, 10)))
	assertEq(t, len(tl.Me), 1)
	assertEq(t, tl.Me[0].Project, "")
	assertWhyHas(t, tl.Me[0], "без проекта")
}
