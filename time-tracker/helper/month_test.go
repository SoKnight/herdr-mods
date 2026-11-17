package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestBuildMonthWeeks(t *testing.T) {
	g := universeGoal // stage 3a's test goal: 390 a working day, calendar, weekends count
	projects := []Project{{ID: "u", Goals: []Goal{g}}}
	ms := func(min int64) DayTotal { return DayTotal{Me: min * minMs, Agents: 2 * min * minMs} }
	rep := Report{Projects: []ProjectTotal{{Project: "u", Days: map[string]DayTotal{nov(1): ms(90), nov(2): ms(300), nov(7): ms(570), nov(16): ms(181)}}}}
	days := planDays(novDays(), shipped(t), projects, nil, rep)
	notes := []Segment{{Start: mskAt(2026, 11, 3, 12, 0), Project: "u", Note: "созвон по ТЗ"}, {Start: mskAt(2026, 11, 3, 15, 0), Project: "u", Note: "созвон по ТЗ"}, {Start: mskAt(2026, 11, 3, 16, 0), Project: "x", Note: "чужое"}}
	m := buildMonth("u", "2026-11", "week", nov(16), days, notes, tsettings())
	var got []string
	for _, r := range m.Rows {
		got = append(got, fmt.Sprint(r.Label, "|", r.Text, "|", r.Workdays, r.Plan, r.Me, r.Fact, r.Agents, r.Current, r.Notes))
	}
	assertEq(t, strings.Join(got, "\n"), strings.Join([]string{
		"1 ноя|1 ноя|0 0 90 90 180 false []",
		"2 – 8 ноя|2–8 ноя|4 1560 870 870 1740 false [созвон по ТЗ]",
		"9 – 15 ноя|9–15 ноя|5 1950 0 0 0 false []",
		"16 – 22 ноя|16 ноя|1 390 181 181 362 true []",
	}, "\n"))
	assertEq(t, fmt.Sprint(m.Total.Workdays, m.Total.Plan, m.Total.Me, m.Total.Fact, m.Total.Agents, m.Plan, m.Running), "10 3900 1141 1141 2282 7800 true")
	assertEq(t, len(buildMonth("u", "2026-11", "day", nov(16), days, nil, tsettings()).Rows), 16)
}

func TestWeeklyGoalRowsAddUp(t *testing.T) {
	projects := []Project{{ID: "t", Goals: []Goal{{From: "2026-11-01", Dir: "min", Minutes: 480, Period: "week"}}}}
	m := buildMonth("t", "2026-11", "week", "2026-11-30", planDays(novDays(), shipped(t), projects, nil, Report{}), nil, tsettings())
	var sum int64
	for _, r := range m.Rows {
		sum += r.Plan
	}
	assertEq(t, fmt.Sprint(len(m.Rows), m.Rows[0].Plan, m.Rows[1].Plan, m.Total.Plan == sum), "6 69 480 true")
	assertEq(t, m.Plan, int64(2057)) // 30 × 480 / 7 rounded once, while the rows add up to 2058
}

func TestMonthReportRoute(t *testing.T) {
	d := testDaemon(t)
	d.S.CarryMinutes = 0
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return mskAt(2026, 11, 16, 22, 0) })
	api.do("PUT", "/v1/projects/easy/goal", map[string]any{"from": "2026-11-01", "dir": "min", "minutes": 390, "period": "day", "weekdays": []int{1, 2, 3, 4, 5}, "calendar": true, "weekends": true}, nil)
	api.do("PUT", "/v1/projects/easy/pay", map[string]any{"from": "2026-11", "kind": "hourly", "amount": 2000}, nil)
	seedWindow(t, d, mskAt(2026, 11, 2, 12, 0), "notepad.exe", "Easy notes", 120)
	seedWindow(t, d, mskAt(2026, 11, 16, 12, 0), "notepad.exe", "Easy notes", 181)
	var m monthReport
	assertEq(t, api.do("GET", "/v1/months/2026-11/report?project=easy", nil, &m), http.StatusOK)
	assertEq(t, fmt.Sprint(len(m.Rows), m.Total.Me, m.Total.Plan, m.Pay.Total, m.Holidays), "4 301 3900 10033 [2026-11-04]")
	assertEq(t, api.do("GET", "/v1/months/2026-12/report?project=easy", nil, nil), http.StatusBadRequest)
	assertEq(t, api.do("GET", "/v1/months/2026-11/report?project=easy&rows=year", nil, nil), http.StatusBadRequest)
	assertEq(t, api.do("GET", "/v1/months/ноябрь/report?project=easy", nil, nil), http.StatusBadRequest)
	assertEq(t, api.do("GET", "/v1/months/2026-11/report?project=nope", nil, nil), http.StatusBadRequest)
	assertEq(t, fmt.Sprint(m.PlanDays, m.Activities, m.DaysOff, m.NoCalendar), "20 map[:301] [] []")

	resp, _ := json.Marshal(m)
	var out bytes.Buffer
	must(t, showMonth(&out, resp))
	assertEq(t, out.String(), strings.Join([]string{
		"1 ноя  0 дн.  цель —  моё —  0  агенты —",
		"2 – 8 ноя  4 дн.  цель 26ч  моё 2ч  −24ч  агенты —",
		"9 – 15 ноя  5 дн.  цель 32ч 30м  моё —  −32ч 30м  агенты —",
		"16 – 22 ноя  1 дн.  цель 6ч 30м  моё 3ч 1м  −3ч 29м  агенты —",
		"Итого  10 дн.  цель 65ч  моё 5ч 1м  −59ч 59м  агенты —",
		"",
	}, "\n"))
	m.Goal = nil
	resp, _ = json.Marshal(m)
	out.Reset()
	must(t, showMonth(&out, resp))
	assertEq(t, strings.Split(out.String(), "\n")[1], "2 – 8 ноя  моё 2ч  агенты —")
}

func TestMonthCLIReadsFlagsAfterTheMonth(t *testing.T) {
	d := testDaemon(t)
	c, err := buildCall(d.S, "month", []string{"report", "2026-11", "--project", "easy", "--rows", "day"})
	must(t, err)
	assertEq(t, c.method+" "+c.path, "GET /v1/months/2026-11/report?project=easy&rows=day")
	c, err = buildCall(d.S, "month", []string{"--project", "easy", "report", "2026-11"})
	must(t, err)
	assertEq(t, c.path, "/v1/months/2026-11/report?project=easy")
	_, err = buildCall(d.S, "month", []string{"report"})
	assertEq(t, err != nil, true)
}
