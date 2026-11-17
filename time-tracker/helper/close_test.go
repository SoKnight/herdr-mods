package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// November with two hours of Easy on the 16th and a goal, looked at on 2 December.
func november(t *testing.T) (*Daemon, testAPI) {
	d := testDaemon(t)
	d.S.CarryMinutes = 0
	api := serve(t, d)
	now := mskAt(2026, 12, 2, 12, 0)
	swap(t, &nowMs, func() int64 { return now })
	api.do("PUT", "/v1/projects/easy/goal", map[string]any{"from": "2026-11-01", "dir": "min", "minutes": 390, "period": "day", "weekdays": []int{1, 2, 3, 4, 5}, "calendar": true}, nil)
	api.do("PUT", "/v1/projects/easy/pay", map[string]any{"from": "2026-11", "kind": "hourly", "amount": 2000}, nil)
	seedWindow(t, d, mskAt(2026, 11, 16, 12, 0), "notepad.exe", "Easy notes", 120)
	return d, api
}

func monthJSON(t *testing.T, api testAPI) string {
	var m monthReport
	assertEq(t, api.do("GET", "/v1/months/2026-11/report?project=easy&rows=day", nil, &m), http.StatusOK)
	b, err := json.Marshal(m)
	must(t, err)
	return string(b)
}

// dropRule removes a rule for all time, as no route does: deleteRule keeps a rule's past months.
func dropRule(t *testing.T, d *Daemon, id int64) {
	_, err := d.editConfig(func(c *Config) error {
		c.Rules = slices.DeleteFunc(c.Rules, func(r Rule) bool { return r.ID == id })
		return nil
	})
	must(t, err)
}

// changeOf is the newest change of kind in History around now.
func changeOf(t *testing.T, api testAPI, kind string) int64 {
	t.Helper()
	var h history
	api.do("GET", fmt.Sprintf("/v1/history?from=%d&to=%d", nowMs()-minMs, nowMs()+minMs), nil, &h)
	for _, c := range h.Changes {
		if c.Kind == kind {
			return c.ID
		}
	}
	t.Fatalf("no %s change in History", kind)
	return 0
}

func TestCloseFreezesTheMonth(t *testing.T) {
	d, api := november(t)
	closed := monthJSON(t, api)
	var c struct {
		Period int64 `json:"period"`
	}
	assertEq(t, api.do("POST", "/v1/months/2026-11/close", map[string]any{"sent": true}, &c), http.StatusOK)

	// Everything that would move November now: the rule that gives Easy its window, the goal, the pay, the calendar.
	dropRule(t, d, 3)
	api.do("PUT", "/v1/projects/easy/goal", map[string]any{"from": "2026-11-01", "dir": "min", "minutes": 480, "period": "day", "weekdays": []int{1, 2, 3, 4, 5, 6, 7}}, nil)
	api.do("PUT", "/v1/projects/easy/pay", map[string]any{"from": "2026-11", "kind": "fixed", "amount": 1}, nil)
	must(t, os.WriteFile(filepath.Join(d.Store.Dir, "calendar.json"), []byte(`{"2026": {"holidays": ["2026-11-16"], "workdays": []}}`), 0o644))
	cal, err := loadCalendar(d.Store.Dir)
	must(t, err)
	d.Cal = cal
	assertEq(t, monthJSON(t, api), closed)
	payChange := changeOf(t, api, "pay-set") // the pay set after the close; undoing it changes nothing in November either
	assertEq(t, api.do("POST", fmt.Sprintf("/v1/history/%d/undo", payChange), nil, nil), http.StatusNoContent)
	assertEq(t, monthJSON(t, api), closed)

	assertEq(t, api.do("POST", "/v1/months/2026-11/reopen", nil, nil), http.StatusNoContent)
	assertEq(t, monthJSON(t, api) != closed, true) // the window is Другое now, the goal 480, the 16th a holiday
	assertEq(t, api.do("POST", fmt.Sprintf("/v1/history/%d/undo", changeOf(t, api, "month-reopen")), nil, nil), http.StatusNoContent)
	assertEq(t, monthJSON(t, api), closed)
	ps, err := d.Store.AllPeriods()
	must(t, err)
	assertEq(t, fmt.Sprintf("%d %v %d %v", len(ps), ps[0].ID == c.Period, ps[0].Reopened, ps[0].Sent), "1 true 0 true")
}

func TestCloseRefusals(t *testing.T) {
	d, api := november(t)
	var e struct {
		Error  string `json:"error"`
		Period int64  `json:"period"`
		Month  string `json:"month"`
	}
	assertEq(t, api.do("POST", "/v1/months/ноябрь/close", nil, nil), http.StatusBadRequest)
	assertEq(t, api.do("POST", "/v1/months/2026-12/close", nil, &e), http.StatusConflict)
	assertEq(t, e.Error, "месяц ещё идёт")
	swap(t, &nowMs, func() int64 { return mskAt(2026, 12, 1, 5, 59) }) // 30 November's day runs until 06:00
	assertEq(t, api.do("POST", "/v1/months/2026-11/close", nil, nil), http.StatusConflict)
	swap(t, &nowMs, func() int64 { return mskAt(2026, 12, 2, 12, 0) })
	assertEq(t, api.do("POST", "/v1/months/2026-11/close", nil, nil), http.StatusOK)
	assertEq(t, api.do("POST", "/v1/months/2026-11/close", nil, &e), http.StatusConflict)
	assertEq(t, e.Error, "Ноябрь уже закрыт")
	for _, req := range []struct {
		method, path string
		body         any
	}{
		{"POST", "/v1/corrections", Correction{Kind: "project", Target: "me", Start: mskAt(2026, 11, 16, 12, 0), End: mskAt(2026, 11, 16, 12, 30), Project: "other"}},
		{"POST", "/v1/daysoff", map[string]any{"from": "2026-11-30", "to": "2026-12-01", "kind": "otgul", "projects": []string{"easy"}}},
		{"POST", "/v1/bonuses", map[string]any{"project": "easy", "month": "2026-11", "day": 12, "title": "Релиз", "amount": 1}},
	} {
		e.Month = ""
		assertEq(t, fmt.Sprint(req.path, api.do(req.method, req.path, req.body, &e), e.Month), fmt.Sprint(req.path, http.StatusConflict, "2026-11"))
	}
	assertEq(t, api.do("POST", "/v1/months/2026-10/reopen", nil, nil), http.StatusNotFound)

	cs, err := d.Store.Corrections()
	must(t, err)
	offs, err := d.Store.DaysOff()
	must(t, err)
	bs, err := d.Store.Bonuses()
	must(t, err)
	ps, err := d.Store.AllPeriods()
	must(t, err)
	assertEq(t, fmt.Sprintf("%d %d %d %d", len(cs), len(offs), len(bs), len(ps)), "0 0 0 1")
}

func TestClosedMonthKeepsItsDaysOffAndBonuses(t *testing.T) {
	_, api := november(t)
	var off DayOff
	var bonus Bonus
	assertEq(t, api.do("POST", "/v1/daysoff", map[string]any{"from": "2026-11-27", "to": "2026-11-27", "kind": "otgul", "projects": []string{"easy"}}, &off), http.StatusCreated)
	assertEq(t, api.do("POST", "/v1/bonuses", map[string]any{"project": "easy", "month": "2026-11", "day": 12, "title": "Релиз", "amount": 1}, &bonus), http.StatusCreated)
	api.do("POST", "/v1/months/2026-11/close", nil, nil)
	var e struct {
		Month string `json:"month"`
	}
	assertEq(t, api.do("DELETE", fmt.Sprintf("/v1/daysoff/%d", off.ID), nil, &e), http.StatusConflict)
	assertEq(t, e.Month, "2026-11")
	e.Month = ""
	assertEq(t, api.do("DELETE", fmt.Sprintf("/v1/bonuses/%d", bonus.ID), nil, &e), http.StatusConflict)
	assertEq(t, e.Month, "2026-11")
}

func TestUndoReopenAfterSecondClose(t *testing.T) {
	_, api := november(t)
	api.do("POST", "/v1/months/2026-11/close", nil, nil)
	api.do("POST", "/v1/months/2026-11/reopen", nil, nil)
	reopen := changeOf(t, api, "month-reopen")
	api.do("POST", "/v1/months/2026-11/close", nil, nil)
	var e struct {
		Error string `json:"error"`
	}
	assertEq(t, api.do("POST", fmt.Sprintf("/v1/history/%d/undo", reopen), nil, &e), http.StatusConflict)
	assertEq(t, e.Error, "Ноябрь уже закрыт заново")
}

func TestMonthState(t *testing.T) {
	d, api := november(t)
	seedWindow(t, d, mskAt(2026, 11, 20, 12, 0), "notepad.exe", "Чужое окно", 12) // no rule: Другое
	var s monthState
	assertEq(t, api.do("GET", "/v1/months/2026-11", nil, &s), http.StatusOK)
	assertEq(t, fmt.Sprintf("%v %v %v %v %v %v %v", s.Over, s.Closed, s.Other, s.OtherDay, s.Edits, s.Holidays, s.NoBonus), "true false 12 2026-11-20 0 [2026-11-04] [easy]")
	assertEq(t, fmt.Sprintf("%v %s %v", s.From == mskAt(2026, 11, 1, 6, 0), s.Prev.Month, s.Prev.Closed), "true 2026-10 false")
}

func TestLegacyPeriodStillApplies(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	seed(t, d, win("notepad.exe", "Easy notes", 0, 60), inp(0, 60), power(0, 60))
	legacyPeriod(t, d, "easy", at(0), at(60))
	swap(t, &nowMs, func() int64 { return at(120) }) // the delete closes the rule at this month's start
	assertEq(t, api.do("DELETE", "/v1/rules/3", nil, nil), http.StatusNoContent)
	var tl struct {
		Report      Report       `json:"report"`
		Divergences []Divergence `json:"divergences"`
	}
	api.do("GET", "/v1/timeline?day="+d.S.DayOf(at(0)), nil, &tl)
	assertEq(t, fmt.Sprint(tl.Report.Projects[0].Project, tl.Report.Projects[0].Me, tl.Divergences[0].DeltaMs), fmt.Sprint("easy", 60*minMs, -60*minMs))
	var list []map[string]any
	assertEq(t, api.do("GET", "/v1/periods", nil, &list), http.StatusOK)
	assertEq(t, len(list), 1)
	assertEq(t, fmt.Sprint(list[0]["delta_ms"], list[0]["me"]), fmt.Sprint(float64(-60*minMs), nil))
}

func TestCloseOverLegacyPeriodKeepsTotals(t *testing.T) {
	d, api := november(t)
	legacyPeriod(t, d, "easy", mskAt(2026, 11, 16, 12, 0), mskAt(2026, 11, 16, 13, 0))
	before := monthJSON(t, api)
	api.do("POST", "/v1/months/2026-11/close", nil, nil)
	assertEq(t, monthJSON(t, api), before)
	dropRule(t, d, 3)
	assertEq(t, monthJSON(t, api), before)
}

func TestMonthCLI(t *testing.T) {
	c, err := buildCall(DefaultSettings(), "month", []string{"close", "2026-11", "--sent"})
	must(t, err)
	assertEq(t, fmt.Sprintf("%s %s %s", c.method, c.path, c.body), `POST /v1/months/2026-11/close {"sent":true}`)

	_, api := november(t)
	cliHome(t, portOf(t, api.url))
	api.do("POST", "/v1/months/2026-11/close", nil, nil)
	var out, errOut bytes.Buffer
	corr := fmt.Sprintf(`{"kind":"exclude","start":%d,"end":%d}`, mskAt(2026, 11, 16, 12, 0), mskAt(2026, 11, 16, 12, 30))
	assertEq(t, runCLI([]string{"correct", "add", corr}, &out, &errOut), 1)
	assertEq(t, strings.TrimSpace(errOut.String()), "closed: месяц 2026-11 закрыт, herdr-time month reopen 2026-11")

	out.Reset()
	assertEq(t, runCLI([]string{"periods"}, &out, &errOut), 0)
	assertEq(t, strings.TrimSpace(out.String()), "#1  ноябрь 2026 · все проекты  закрыт 2 дек")
}
