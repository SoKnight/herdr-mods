package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func nov(d int) string { return fmt.Sprintf("2026-11-%02d", d) }

func novDays() []string {
	var out []string
	for d := 1; d <= 30; d++ {
		out = append(out, nov(d))
	}
	return out
}

var universeGoal = Goal{From: "2026-11-01", Dir: "min", Minutes: 390, Period: "day", Weekdays: []int{1, 2, 3, 4, 5}, Calendar: true, Weekends: true}

func TestGoalVersions(t *testing.T) {
	dec := Goal{From: "2026-12-01", Dir: "min", Minutes: 420, Period: "day", Weekdays: []int{1, 2, 3, 4, 5}}
	gs := setGoal(setGoal(nil, universeGoal), dec)
	assertEq(t, goalOn(gs, "2026-10-31") == nil, true)
	assertEq(t, goalOn(gs, "2026-11-30").Minutes, int64(390))
	assertEq(t, goalOn(gs, "2026-12-01").Minutes, int64(420))
	mid := universeGoal
	mid.From, mid.Minutes = "2026-11-15", 360
	gs = setGoal(gs, mid) // the December version goes, the 1st–14th keep 390
	assertEq(t, len(gs), 2)
	assertEq(t, goalOn(gs, "2026-11-14").Minutes, int64(390))
	assertEq(t, goalOn(gs, "2026-12-05").Minutes, int64(360))
	gs = setGoal(gs, Goal{From: "2026-11-20", Off: true})
	assertEq(t, goalOn(gs, "2026-11-21") == nil, true)
	assertEq(t, goalOn(gs, "2026-11-19").Minutes, int64(360))
	assertEq(t, len(setGoal(nil, Goal{From: "2026-11-20", Off: true})), 0)
}

func TestDayPlan(t *testing.T) {
	cal := Calendar{"2026": {Holidays: []string{"2026-11-04"}, Workdays: []string{"2026-11-28"}}}
	g := universeGoal
	assertEq(t, fmt.Sprint(dayPlan(&g, nov(16), cal), dayPlan(&g, nov(4), cal), dayPlan(&g, nov(28), cal), dayPlan(&g, nov(29), cal)), "390 0 390 0")
	g.Calendar = false
	assertEq(t, fmt.Sprint(dayPlan(&g, nov(4), cal), dayPlan(&g, nov(28), cal)), "390 0")
	week := Goal{Minutes: 480, Period: "week"}
	assertEq(t, dayPlan(&week, nov(1), cal), 480.0/7)
	month := Goal{Minutes: 2400, Period: "month"}
	assertEq(t, dayPlan(&month, nov(7), cal), 80.0)
	assertEq(t, dayPlan(nil, nov(16), cal), 0.0)
}

func TestPlanDaysMinutesAndFact(t *testing.T) {
	g := universeGoal
	g.Weekends = false
	projects := []Project{{ID: "u", Goals: []Goal{g}}, {ID: "e"}}
	rep := Report{Projects: []ProjectTotal{
		{Project: "u", Days: map[string]DayTotal{nov(16): {Me: 181*minMs + 30_000, Agents: 343 * minMs}, nov(21): {Me: 60 * minMs}, nov(23): {Me: 30 * minMs}}},
		{Project: "", Days: map[string]DayTotal{nov(16): {Me: 59_999}}},
	}}
	off := []DayOff{{From: nov(23), To: nov(27), Kind: "otgul", Projects: []string{"u"}}}
	days := planDays([]string{nov(16), nov(21), nov(23)}, shipped(t), projects, off, rep)
	assertEq(t, fmt.Sprint(days[0].Me, days[0].Agents, days[0].Plan, days[0].Fact), "map[:1 u:182] map[u:343] map[u:390] map[u:182]")
	assertEq(t, fmt.Sprint(days[1].Work, days[1].Plan["u"], days[1].Fact["u"]), "false 0 0") // Saturday, weekends not counted
	assertEq(t, fmt.Sprintf("%s %v %v", days[2].Off, days[2].Plan["u"], days[2].Fact["u"]), "otgul 0 0")
	_, has := days[0].Plan["e"]
	assertEq(t, has, false) // no goal, no plan entry
}

func TestSumGoalNovember(t *testing.T) {
	projects := []Project{{ID: "u", Goals: []Goal{universeGoal}}}
	ms := func(min int64) DayTotal { return DayTotal{Me: min * minMs} }
	rep := Report{Projects: []ProjectTotal{{Project: "u", Days: map[string]DayTotal{nov(2): ms(300), nov(4): ms(60), nov(7): ms(120), nov(16): ms(181)}}}}
	s := sumGoal(planDays(novDays(), shipped(t), projects, nil, rep), "u", nov(16))
	assertEq(t, s, goalSum{Plan: 7800, PlanToDate: 3900, Fact: 661, Me: 661, Balance: -3239, Forecast: 1322, Need: 714, Days: 20, DaysLeft: 10})
	projects[0].Goals[0].Weekends = false
	s = sumGoal(planDays(novDays(), shipped(t), projects, nil, rep), "u", nov(16))
	assertEq(t, fmt.Sprint(s.Fact, s.Me), "481 661") // the holiday and the Saturday stop counting
}

func TestWeeklyGoalProRata(t *testing.T) {
	projects := []Project{{ID: "t", Goals: []Goal{{From: "2026-11-01", Dir: "min", Minutes: 480, Period: "week"}}}}
	s := sumGoal(planDays(novDays(), shipped(t), projects, nil, Report{}), "t", nov(16))
	assertEq(t, fmt.Sprint(s.PlanToDate, s.Plan, s.Days), "1097 2057 30")
	off := []DayOff{{From: nov(23), To: nov(27), Kind: "vacation", Projects: []string{"t"}}}
	s = sumGoal(planDays(novDays(), shipped(t), projects, off, Report{}), "t", nov(16))
	assertEq(t, s.Plan, int64(1714))
}

// 2401 / 30 a day over 15 days is exactly 1200.5: the sum must round half up to 1201, never drift below.
func TestMonthlyGoalExactHalf(t *testing.T) {
	projects := []Project{{ID: "m", Goals: []Goal{{From: "2026-11-01", Dir: "min", Minutes: 2401, Period: "month"}}}}
	s := sumGoal(planDays(novDays(), shipped(t), projects, nil, Report{}), "m", nov(15))
	assertEq(t, fmt.Sprint(s.PlanToDate, s.Plan, s.Forecast, s.Need), "1201 2401 0 161") // need = ceil(2401 / 15)
}

// A hand-edited projects.json may list versions out of order: the latest From up to the day still wins.
func TestGoalOnOutOfOrder(t *testing.T) {
	gs := []Goal{{From: "2026-12-01", Minutes: 420}, {From: "2026-11-01", Minutes: 390}, {From: "2026-11-15", Minutes: 360}}
	assertEq(t, fmt.Sprint(goalOn(gs, "2026-11-10").Minutes, goalOn(gs, "2026-11-20").Minutes, goalOn(gs, "2026-12-02").Minutes), "390 360 420")
	assertEq(t, goalOn(gs, "2026-10-31") == nil, true)
}

func TestGoalSetKeepsPastVersions(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	set := func(g map[string]any) int { return api.do("PUT", "/v1/projects/easy/goal", g, nil) }
	day := map[string]any{"from": "2026-11-01", "dir": "min", "minutes": 390, "period": "day", "weekdays": []int{1, 2, 3, 4, 5}, "calendar": true, "weekends": true}
	assertEq(t, set(day), http.StatusOK)
	day["from"], day["minutes"] = "2026-12-01", 420
	set(day)
	day["from"], day["minutes"] = "2026-11-15", 360
	set(day)
	gs := d.project("easy").Goals
	assertEq(t, fmt.Sprintf("%v %v %v %v %v", gs[0].From, gs[0].Minutes, gs[1].From, gs[1].Minutes, len(gs)), "2026-11-01 390 2026-11-15 360 2")
	week := map[string]any{"from": "2026-11-01", "dir": "min", "minutes": 480, "period": "week", "weekdays": []int{1}, "calendar": true}
	assertEq(t, api.do("PUT", "/v1/projects/other/goal", week, nil), http.StatusOK)
	assertEq(t, d.project("other").Goals[0].Weekdays == nil && !d.project("other").Goals[0].Calendar, true)
	var cat catalog
	api.do("GET", "/v1/projects", nil, &cat)
	assertEq(t, len(cat.Projects[0].Goals), 2)
}

func TestGoalRequestsChecked(t *testing.T) {
	api := serve(t, testDaemon(t))
	for body, want := range map[string]int{
		`{"from":"1 ноября","dir":"min","minutes":390,"period":"day","weekdays":[1]}`:    http.StatusBadRequest,
		`{"from":"2026-11-01","dir":"less","minutes":390,"period":"day","weekdays":[1]}`: http.StatusBadRequest,
		`{"from":"2026-11-01","dir":"min","minutes":1441,"period":"day","weekdays":[1]}`: http.StatusBadRequest,
		`{"from":"2026-11-01","dir":"min","minutes":390,"period":"day","weekdays":[]}`:   http.StatusBadRequest,
		`{"from":"2026-11-01","dir":"min","minutes":390,"period":"year"}`:                http.StatusBadRequest,
		`{"from":"2026-11-01","off":true}`:                                               http.StatusOK,
	} {
		assertEq(t, fmt.Sprint(body, api.do("PUT", "/v1/projects/easy/goal", body, nil)), fmt.Sprint(body, want))
	}
	assertEq(t, api.do("PUT", "/v1/projects/nope/goal", `{"from":"2026-11-01","off":true}`, nil), http.StatusNotFound)
}

func TestDaysOffAddAndRemove(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	now := mskAt(2026, 11, 16, 12, 0)
	swap(t, &nowMs, func() int64 { return now })
	var off DayOff
	assertEq(t, api.do("POST", "/v1/daysoff", map[string]any{"from": "2026-11-23", "to": "2026-11-27", "kind": "otgul", "projects": []string{"easy"}}, &off), http.StatusCreated)
	assertEq(t, fmt.Sprint(off.ID, off.Made), fmt.Sprint(1, now))
	for _, bad := range []map[string]any{
		{"from": "2026-11-27", "to": "2026-11-23", "kind": "otgul"},
		{"from": "2026-11-01", "to": "2027-01-15", "kind": "otgul"},
		{"from": "2026-11-23", "to": "2026-11-23", "kind": "holiday"},
		{"from": "2026-11-23", "to": "2026-11-23", "kind": "sick", "projects": []string{"nope"}},
	} {
		assertEq(t, api.do("POST", "/v1/daysoff", bad, nil), http.StatusBadRequest)
	}
	var h history
	api.do("GET", fmt.Sprintf("/v1/history?from=%d&to=%d", now-minMs, now+minMs), nil, &h)
	assertEq(t, len(h.DaysOff), 1)
	assertEq(t, api.do("DELETE", "/v1/daysoff/1", nil, nil), http.StatusNoContent)
	assertEq(t, api.do("DELETE", "/v1/daysoff/1", nil, nil), http.StatusNotFound)
}

type planAnswer struct {
	Today      string             `json:"today"`
	Days       []planDay          `json:"days"`
	Goals      map[string]Goal    `json:"goals"`
	Sums       map[string]goalSum `json:"sums"`
	NoCalendar []string           `json:"no_calendar"`
}

func TestPlanRoute(t *testing.T) {
	d := testDaemon(t)
	d.S.CarryMinutes = 0
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return mskAt(2026, 11, 16, 22, 0) })
	api.do("PUT", "/v1/projects/easy/goal", map[string]any{"from": "2026-11-01", "dir": "min", "minutes": 390, "period": "day", "weekdays": []int{1, 2, 3, 4, 5}, "calendar": true, "weekends": true}, nil)
	seedWindow(t, d, mskAt(2026, 11, 4, 12, 0), "notepad.exe", "Easy notes", 60) // the holiday
	seedWindow(t, d, mskAt(2026, 11, 16, 12, 0), "notepad.exe", "Easy notes", 181)
	var p planAnswer
	assertEq(t, api.do("GET", "/v1/plan?from=2026-11-01&to=2026-11-30", nil, &p), http.StatusOK)
	assertEq(t, fmt.Sprintf("%v %v %v %v %v", p.Today, len(p.Days), p.Days[3].Holiday, p.Days[3].Me["easy"], p.Days[3].Fact["easy"]), "2026-11-16 30 true 60 60")
	assertEq(t, p.Sums["easy"], goalSum{Plan: 7800, PlanToDate: 3900, Fact: 241, Me: 241, Balance: -3659, Forecast: 482, Need: 756, Days: 20, DaysLeft: 10})
	assertEq(t, len(p.NoCalendar), 0)
	assertEq(t, api.do("GET", "/v1/plan?from=2026-12-20&to=2027-01-10", nil, &p), http.StatusOK)
	assertEq(t, fmt.Sprint(p.NoCalendar), "[2027]")
	assertEq(t, api.do("GET", "/v1/plan?from=2026-10-01&to=2026-12-31", nil, nil), http.StatusBadRequest)
}

// A goal removed or changed inside the range keeps its plan for the days it applied; the goal shown is the
// latest one in force up to today.
func TestPlanRouteKeepsChangedGoals(t *testing.T) {
	d := testDaemon(t)
	d.S.CarryMinutes = 0
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return mskAt(2026, 11, 25, 12, 0) })
	put := func(id string, g map[string]any) { assertEq(t, api.do("PUT", "/v1/projects/"+id+"/goal", g, nil), http.StatusOK) }
	put("easy", map[string]any{"from": "2026-11-01", "dir": "min", "minutes": 390, "period": "day", "weekdays": []int{1, 2, 3, 4, 5}, "calendar": true, "weekends": true})
	put("easy", map[string]any{"from": "2026-11-20", "off": true})
	put("other", map[string]any{"from": "2026-11-01", "dir": "min", "minutes": 480, "period": "day", "weekdays": []int{1, 2, 3, 4, 5}})
	put("other", map[string]any{"from": "2026-11-16", "dir": "min", "minutes": 700, "period": "week"})
	seedWindow(t, d, mskAt(2026, 11, 16, 12, 0), "notepad.exe", "Easy notes", 60)
	seedWindow(t, d, mskAt(2026, 11, 23, 12, 0), "notepad.exe", "Easy notes", 30) // after Убрать цель: not counted
	var p planAnswer
	assertEq(t, api.do("GET", "/v1/plan?from=2026-11-01&to=2026-11-30", nil, &p), http.StatusOK)
	// 13 working days of 1–19 November (the 4th a holiday) × 390
	assertEq(t, p.Sums["easy"], goalSum{Plan: 5070, PlanToDate: 5070, Fact: 60, Me: 90, Balance: -5010, Forecast: 60, Days: 13})
	assertEq(t, fmt.Sprintf("%v %v", p.Goals["easy"].Period, p.Goals["easy"].Minutes), "day 390")
	// 10 weekdays of 1–13 November × 480, then 15 days × 700 / 7 (10 of them up to the 25th)
	s := p.Sums["other"]
	assertEq(t, fmt.Sprint(s.Plan, s.PlanToDate, s.Days, s.DaysLeft), "6300 5800 25 5")
	assertEq(t, fmt.Sprintf("%v %v", p.Goals["other"].Period, p.Goals["other"].Minutes), "week 700")
	assertEq(t, api.do("GET", "/v1/plan?from=2026-11-01&to=2026-11-10", nil, &p), http.StatusOK)
	assertEq(t, fmt.Sprintf("%v %v", p.Goals["other"].Period, p.Sums["other"].Plan), "day 3360") // 7 weekdays × 480
}

// A weekend day moved to a working one is planned for a calendar day goal and its time counts.
func TestPlanDaysMovedWorkday(t *testing.T) {
	g := universeGoal
	g.Weekends = false
	cal := Calendar{"2026": {Workdays: []string{"2026-11-28"}}}
	rep := Report{Projects: []ProjectTotal{{Project: "u", Days: map[string]DayTotal{nov(28): {Me: 45 * minMs}, nov(29): {Me: 20 * minMs}}}}}
	days := planDays([]string{nov(28), nov(29)}, cal, []Project{{ID: "u", Goals: []Goal{g}}}, nil, rep)
	assertEq(t, fmt.Sprint(days[0].Work, days[0].Plan["u"], days[0].Fact["u"], days[1].Work, days[1].Plan["u"], days[1].Fact["u"]), "true 390 45 false 0 0")
	s := sumGoal(days, "u", nov(29))
	assertEq(t, fmt.Sprint(s.Plan, s.Fact, s.Me), "390 45 65")
}

func TestGoalWeekdaysDeduped(t *testing.T) {
	g, err := checkGoal(Goal{From: "2026-11-01", Dir: "min", Minutes: 60, Period: "day", Weekdays: []int{3, 1, 1, 1}})
	must(t, err)
	assertEq(t, fmt.Sprint(g.Weekdays), "[1 3]")
}

func TestGoalsAtNight(t *testing.T) {
	d := testDaemon(t)
	d.S.CarryMinutes = 0
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return mskAt(2026, 11, 17, 3, 0) }) // before 06:00: still the 16th
	api.do("PUT", "/v1/projects/easy/goal", map[string]any{"from": "2026-11-01", "dir": "min", "minutes": 390, "period": "day", "weekdays": []int{1, 2, 3, 4, 5}, "calendar": true}, nil)
	api.do("PUT", "/v1/projects/other/goal", map[string]any{"from": "2026-11-01", "dir": "min", "minutes": 480, "period": "week"}, nil)
	seedWindow(t, d, mskAt(2026, 11, 17, 1, 0), "notepad.exe", "Easy notes", 90)
	var g struct {
		Day   string     `json:"day"`
		Goals []goalInfo `json:"goals"`
	}
	assertEq(t, api.do("GET", "/v1/goals", nil, &g), http.StatusOK)
	assertEq(t, g.Day, "2026-11-16")
	e := g.Goals[0]
	assertEq(t, fmt.Sprintf("%v %v %v %v %v %v", e.Project, e.Day.Plan, e.Day.Fact, e.Day.Balance, e.Month.PlanToDate, e.Month.Fact), "easy 390 90 -300 3900 90")
	assertEq(t, fmt.Sprintf("%v %v %v", g.Goals[1].Project, g.Goals[1].Week.Plan, g.Goals[1].Week.PlanToDate), "other 480 69") // Monday only: 480 / 7
}

func TestCLIGoalsAndPlan(t *testing.T) {
	d := testDaemon(t)
	cliHome(t, portOf(t, serve(t, d).url))
	var out, errOut bytes.Buffer
	assertEq(t, runCLI([]string{"goal", "set", "easy", `{"from":"2026-11-01","dir":"min","minutes":390,"period":"day","weekdays":[1,2,3,4,5]}`, "--json"}, &out, &errOut), 0)
	assertEq(t, runCLI([]string{"dayoff", "add", `{"from":"2026-11-23","to":"2026-11-27","kind":"otgul","projects":["easy"]}`, "--json"}, &out, &errOut), 0)
	out.Reset()
	assertEq(t, runCLI([]string{"plan", "--from", "2026-11-01", "--to", "2026-11-30"}, &out, &errOut), 0)
	assertEq(t, strings.Contains(out.String(), "easy"), true)
	c, err := buildCall(d.S, "plan", []string{"--from", "2026-11-01", "--to", "2026-11-30"})
	must(t, err)
	assertEq(t, c.path, "/v1/plan?from=2026-11-01&to=2026-11-30")
	assertEq(t, runCLI([]string{"goals", "--day", "2026-11-16", "--json"}, &out, &errOut), 0)
	assertEq(t, runCLI([]string{"dayoff", "rm", "1", "--json"}, &out, &errOut), 0)
}

func TestGoalLine(t *testing.T) {
	assertEq(t, goalLine("universe", goalSum{Fact: 3900, Plan: 7800, Balance: -1449, Forecast: 4901}), "universe  65ч / 130ч  баланс −24ч 9м  прогноз 81ч 41м")
	assertEq(t, strings.Join([]string{mins(0), mins(59), signed(0), signed(61)}, " "), "— 59м 0 +1ч 1м")
}

// Plans count working days by the calendar: a bad calendar.json answers as a config error, not with numbers.
func TestPlanRoutesGateOnConfigError(t *testing.T) {
	home := t.TempDir()
	must(t, writeJSON(filepath.Join(home, "projects.json"), serverConfig()))
	must(t, os.WriteFile(filepath.Join(home, "calendar.json"), []byte(`{"2026": {"holidays": ["4 ноября"]}}`), 0o644))
	d, err := NewDaemon(home)
	must(t, err)
	api := serve(t, d)
	assertEq(t, api.do("GET", "/v1/plan?from=2026-11-01&to=2026-11-30", nil, nil), http.StatusInternalServerError)
	assertEq(t, api.do("GET", "/v1/goals", nil, nil), http.StatusInternalServerError)
	assertEq(t, serve(t, testDaemon(t)).do("GET", "/v1/goals?day=16.11", nil, nil), http.StatusBadRequest)
}
