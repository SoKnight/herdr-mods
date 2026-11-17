package main

import (
	"fmt"
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
