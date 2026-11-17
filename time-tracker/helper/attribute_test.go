package main

import (
	"reflect"
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
func power(s, e int) Span  { return Span{Kind: "power", Start: at(s), End: at(e)} }
func nodata(s, e int) Span { return Span{Kind: "nodata", Start: at(s), End: at(e)} }
func spans(sp ...Span) Raw { return Raw{Spans: sp} }

func hasKind(raw Raw, kind string) bool {
	for _, sp := range raw.Spans {
		if sp.Kind == kind {
			return true
		}
	}
	return false
}

func agent(term, cwd string, s, e int) Span {
	return Span{Kind: "agent", Start: at(s), End: at(e), Term: term, Cwd: cwd}
}

func fix(id int64, kind, target string, s, e int, project string) Correction {
	return Correction{ID: id, Kind: kind, Target: target, Start: at(s), End: at(e), Project: project}
}

const minMs = 60_000

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
			{ID: "study", Name: "Учёба", Apps: []string{"telemost.exe"}},
			{ID: "universe", Name: "Universe", Paths: []string{`C:\Dev\starmc`}},
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

// attrRange keeps the computer on over the whole range (and the hour of data before it), unless the
// test brings its own power spans.
func attrRange(t *testing.T, raw Raw, from, to int, corr ...Correction) Timeline {
	t.Helper()
	if !hasKind(raw, "power") {
		raw.Spans = append(raw.Spans, power(from-60, to))
	}
	s := DefaultSettings()
	s.Loc = msk
	return Attribute(raw, testRules(t), corr, s, at(from), at(to))
}

func attr(t *testing.T, raw Raw, corr ...Correction) Timeline {
	return attrRange(t, raw, -60, 240, corr...)
}

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

func TestShortSleepIsNotPresence(t *testing.T) {
	// A 3-minute sleep is shorter than idle, yet the computer was off: never glued into presence.
	tl := attr(t, spans(power(0, 10), power(13, 30), win("idea64.exe", "x", 0, 30), inp(0, 10), inp(13, 30)))
	assertMeIntervals(t, tl, []Interval{m(0, 10), m(13, 30)})
	var total int64
	for _, s := range tl.Me {
		total += s.End - s.Start
	}
	assertEq(t, total, 27*minMs)
}

func TestPresenceNeedsPowerForCalls(t *testing.T) {
	tl := attr(t, spans(power(0, 40), call(0, 60)))
	assertMe(t, tl, 0, 40)
}

func TestNoDataIsNotPresence(t *testing.T) {
	tl := attr(t, spans(power(0, 30), nodata(0, 10), win("idea64.exe", "x", 0, 30), inp(0, 30)))
	assertMe(t, tl, 10, 30)
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

func TestBeatProjectByName(t *testing.T) {
	byName := Heartbeat{Source: "firefox", Time: at(0), ProjectName: "easylauncher"}
	tl := attr(t, spans(win("firefox.exe", "x", 0, 1), inp(0, 1)).with(byName))
	assertEq(t, segAt(t, tl, 0, 30).Project, "easylauncher")
	assertWhyHas(t, segAt(t, tl, 0, 30), "имя проекта")
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

func TestSegmentKeepsSignalPath(t *testing.T) {
	tl := attr(t, spans(win("herdr-app.exe", "shell", 0, 4), inp(0, 4)).with(
		beat("herdr", 0, `C:\Dev\a`, ""),
		beat("herdr", 2, `C:\Dev\b`, ""),
	))
	// Same attribution apart from the folder: still two segments, one per folder.
	assertEq(t, len(tl.Me), 2)
	assertEq(t, tl.Me[0].Path, `C:\Dev\a`)
	assertEq(t, tl.Me[1].Path, `C:\Dev\b`)
	assertEq(t, tl.Me[1].Start, at(2))
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

func agentIntervals(tl Timeline, project string) []Interval {
	var out []Interval
	for _, a := range tl.Agents {
		if a.Project == project {
			out = append(out, Interval{a.Start, a.End})
		}
	}
	return out
}

func assertAgents(t *testing.T, tl Timeline, project string, want ...Interval) {
	t.Helper()
	if got := agentIntervals(tl, project); !reflect.DeepEqual(got, want) {
		t.Errorf("agents %q = %v, want %v", project, got, want)
	}
}

func assertAgentTotal(t *testing.T, tl Timeline, project string, want int64) {
	t.Helper()
	var got int64
	for _, iv := range agentIntervals(tl, project) {
		got += iv.End - iv.Start
	}
	assertEq(t, got, want)
}

func TestAgentsUnionPerProject(t *testing.T) {
	tl := attr(t, spans(agent("t1", `C:\Dev\easyx\EasyLauncher`, 0, 30), agent("t2", `C:\Dev\easyx\EasyLauncher`, 20, 50), agent("t3", `C:\Dev\starmc\x`, 0, 10)))
	assertAgentTotal(t, tl, "easylauncher", 50*minMs)
	assertAgentTotal(t, tl, "universe", 10*minMs)
	assertAgents(t, tl, "easylauncher", m(0, 50))
	assertEq(t, len(tl.Me), 0)
}

func TestAgentsIndependentOfPresence(t *testing.T) {
	tl := attr(t, spans(agent("t1", `C:\Dev\easyx`, 0, 30), agent("t2", `C:\Elsewhere`, 0, 5)))
	assertEq(t, len(tl.Me), 0)
	assertAgents(t, tl, "easylauncher", m(0, 30))
	assertAgents(t, tl, "", m(0, 5)) // a folder outside every project still shows
	for _, a := range tl.Agents {
		if !strings.Contains(a.Why, "агент t") {
			t.Errorf("Why %q lacks the terminal", a.Why)
		}
	}
}

func TestAgentCorrectionBeforeUnion(t *testing.T) {
	raw := spans(agent("t1", `C:\Dev\easyx`, 0, 30), agent("t2", `C:\Dev\easyx`, 20, 50))
	tl := attr(t, raw, fix(1, "project", "agents", 0, 25, "tricky"))
	assertAgents(t, tl, "easylauncher", m(25, 50))
	assertAgents(t, tl, "tricky", m(0, 25))
	for _, a := range tl.Agents {
		assertEq(t, a.Corrected, a.Project == "tricky")
		if a.Project == "tricky" && !strings.Contains(a.Why, "правка #1") {
			t.Errorf("Why %q lacks the correction", a.Why)
		}
	}

	tl = attr(t, raw, fix(1, "exclude", "agents", 10, 40, ""))
	assertAgents(t, tl, "easylauncher", m(0, 10), m(40, 50))
}

func TestCorrectionTargetMeLeavesAgents(t *testing.T) {
	raw := spans(win("idea64.exe", "Tricky", 0, 60), inp(0, 60), agent("t1", `C:\Dev\easyx`, 0, 60))
	tl := attr(t, raw, fix(1, "exclude", "me", 0, 60, ""))
	assertEq(t, len(tl.Me), 0)
	want := attr(t, raw).Agents
	if len(want) == 0 || !reflect.DeepEqual(tl.Agents, want) {
		t.Errorf("Agents = %+v, want %+v", tl.Agents, want)
	}
}

func TestLaterCorrectionWins(t *testing.T) {
	raw := spans(win("idea64.exe", "Tricky", 0, 60), inp(0, 60), agent("t1", `C:\Dev\easyx`, 0, 60))
	// Passed out of order: the ID decides, not the position.
	tl := attr(t, raw, fix(2, "project", "both", 30, 60, "b"), fix(1, "project", "both", 0, 60, "a"))
	assertEq(t, segAt(t, tl, 29, 59).Project, "a")
	s := segAt(t, tl, 30, 0)
	assertEq(t, s.Project, "b")
	assertEq(t, s.Corrected, true)
	if !strings.HasPrefix(s.Why, "правка #2: ") || !strings.Contains(s.Why, "правка #1: ") {
		t.Errorf("Why %q: want the newest correction first, then the older one", s.Why)
	}
	assertWhyHas(t, s, "заголовок") // the original reason stays after the prefixes
	assertMe(t, tl, 0, 60)
	assertAgents(t, tl, "a", m(0, 30))
	assertAgents(t, tl, "b", m(30, 60))
}

func TestAddManualTimeNoDoubleCount(t *testing.T) {
	add := Correction{ID: 1, Kind: "add", Target: "me", Start: at(20), End: at(40), Project: "study", Activity: "study"}
	tl := attr(t, spans(win("idea64.exe", "Tricky", 0, 30), inp(0, 30)), add)
	assertMe(t, tl, 0, 40)
	s := segAt(t, tl, 19, 59)
	assertEq(t, s.Project, "tricky")
	assertEq(t, s.Manual, false)
	assertEq(t, s.Corrected, false)
	s = segAt(t, tl, 20, 0)
	assertEq(t, s.End, at(40))
	assertEq(t, s.Project, "study")
	assertEq(t, s.Activity, "study")
	assertEq(t, s.Manual, true)
	assertEq(t, s.Corrected, true)
	assertWhyHas(t, s, "правка #1")
}

func TestRemovedCorrectionRestores(t *testing.T) {
	raw := spans(win("idea64.exe", "Tricky", 0, 60), inp(0, 60), agent("t1", `C:\Dev\easyx`, 0, 60))
	base := attr(t, raw)
	note := Correction{ID: 3, Kind: "note", Target: "me", Start: at(0), End: at(10), Note: "созвон"}
	act := Correction{ID: 4, Kind: "activity", Target: "agents", Start: at(0), End: at(10), Activity: "design"}
	fixed := attr(t, raw, fix(1, "project", "both", 0, 30, "site"), fix(2, "exclude", "both", 50, 60, ""), note, act)
	s := segAt(t, fixed, 5, 0)
	assertEq(t, s.Note, "созвон")
	assertEq(t, s.Activity, "design") // activity and note touch Me whatever the target
	assertEq(t, s.Project, "site")
	assertMe(t, fixed, 0, 50)
	assertAgents(t, fixed, "site", m(0, 30))
	assertAgents(t, fixed, "easylauncher", m(30, 50))
	if again := attr(t, raw); !reflect.DeepEqual(again, base) {
		t.Errorf("without corrections = %+v, want %+v", again, base)
	}
}

func TestPowerAndNoData(t *testing.T) {
	tl := attr(t, spans(
		Span{Kind: "power", Start: at(-90), End: at(-30)},
		Span{Kind: "power", Start: at(0), End: at(120)},
		Span{Kind: "nodata", Start: at(5), End: at(12)},
		Span{Kind: "nodata", Start: at(0), End: at(10)},
		Span{Kind: "nodata", Start: at(12), End: at(15)},
		Span{Kind: "agents_unknown", Start: at(200), End: at(300)},
	))
	if !reflect.DeepEqual(tl.Power, []Interval{m(-60, -30), m(0, 120)}) {
		t.Errorf("Power = %v", tl.Power)
	}
	if !reflect.DeepEqual(tl.NoData, []Interval{m(0, 15)}) {
		t.Errorf("NoData = %v", tl.NoData)
	}
	if !reflect.DeepEqual(tl.AgentsUnknown, []Interval{m(200, 240)}) {
		t.Errorf("AgentsUnknown = %v", tl.AgentsUnknown)
	}
}

func targetRules(t *testing.T) *Rules {
	return mustCompile(t, Config{
		Projects: []Project{{ID: "tricky"}, {ID: "study"}},
		Rules: []Rule{
			{ID: 1, Kind: "process", Value: "Telegram.exe", Project: excludeID},
			{ID: 2, Kind: "title", Value: "Minecraft", Project: leisureID},
			{ID: 3, Kind: "title", Value: "Tricky", Project: "tricky"},
			{ID: 4, Kind: "process", Value: "telemost.exe", Project: "study"},
			{ID: 5, Kind: "folder", Value: `C:\Games`, Project: leisureID},
		},
		Apps: []AppRule{{Exe: "telemost.exe", Presence: "call"}},
	})
}

func attrTargets(t *testing.T, raw Raw, corr ...Correction) Timeline {
	t.Helper()
	raw.Spans = append(raw.Spans, power(-60, 240))
	s := DefaultSettings()
	s.Loc = msk
	return Attribute(raw, targetRules(t), corr, s, at(-60), at(240))
}

func TestNotWorkAndLeisureAreNotMyTime(t *testing.T) {
	tl := attrTargets(t, spans(win("Telegram.exe", "chat", 0, 10), win("javaw.exe", "Minecraft 26.3", 10, 30), inp(0, 30)))
	assertEq(t, len(tl.Me), 0)
	assertEq(t, len(tl.Leisure), 1)
	assertEq(t, tl.Leisure[0].Start, at(10))
	assertEq(t, tl.Leisure[0].End, at(30))
	rep := Summarize(tl, tsettings(), nil)
	assertEq(t, rep.Me, int64(0))
	assertEq(t, rep.Leisure, int64(20*minMs))
}

func TestNotWorkAndLeisureDoNotCarry(t *testing.T) {
	tl := attrTargets(t, spans(win("idea64.exe", "Tricky", 0, 10), win("Telegram.exe", "chat", 10, 12), win("javaw.exe", "Minecraft", 12, 14), win("notepad.exe", "x", 14, 20), inp(0, 20)))
	assertMeIntervals(t, tl, []Interval{m(0, 10), m(14, 20)})
	assertEq(t, segAt(t, tl, 15, 0).Project, "tricky") // carried from 10, across both
}

func TestCallDuringLeisureStaysLeisure(t *testing.T) {
	tl := attrTargets(t, spans(win("javaw.exe", "Minecraft", 0, 10), call(0, 10), inp(0, 10)))
	assertEq(t, len(tl.Me), 0)
	assertEq(t, len(tl.Leisure), 1)
	tl = attrTargets(t, spans(win("notepad.exe", "x", 0, 10), call(0, 10), inp(0, 10)))
	assertEq(t, segAt(t, tl, 5, 0).Project, "study") // elsewhere the call still wins
}

func TestCorrectionBeatsNotWorkRule(t *testing.T) {
	tl := attrTargets(t, spans(win("Telegram.exe", "client chat", 0, 10), inp(0, 10)), fix(1, "project", "me", 0, 10, "tricky"))
	assertEq(t, segAt(t, tl, 5, 0).Project, "tricky")
}

func TestAgentsInLeisureFoldersAreDropped(t *testing.T) {
	assertEq(t, len(attrTargets(t, spans(agent("t1", `C:\Games\mc`, 0, 10))).Agents), 0)
}

func TestCorrectionMovesLeisureToProject(t *testing.T) {
	tl := attrTargets(t, spans(win("javaw.exe", "Minecraft", 0, 10), inp(0, 10)), fix(1, "project", "me", 0, 10, "tricky"))
	assertEq(t, len(tl.Leisure), 0)
	assertEq(t, segAt(t, tl, 5, 0).Project, "tricky")
}

func TestMinecraftRuleMatchesServer(t *testing.T) {
	rules := mustCompile(t, Config{Projects: []Project{{ID: "tricky"}}, Rules: []Rule{{ID: 1, Kind: "minecraft", Value: "play.tricky.net:25565", Project: "tricky"}}})
	w := win("javaw.exe", "Minecraft 26.3 - Multiplayer (3rd-party Server)", 0, 10)
	w.Server = "play.tricky.net:25565"
	s := DefaultSettings()
	s.Loc = msk
	tl := Attribute(spans(power(0, 10), inp(0, 10), w), rules, nil, s, at(0), at(10))
	assertEq(t, tl.Me[0].Project, "tricky")
	assertEq(t, tl.Me[0].Server, "play.tricky.net:25565")
}
