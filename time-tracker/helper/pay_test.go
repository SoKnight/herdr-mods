package main

import (
	"bytes"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func TestMonthPay(t *testing.T) {
	fixed := &Pay{From: "2026-11", Kind: "fixed", Amount: 100_000}
	p := monthPay(fixed, 2451, 7800, nil)
	assertEq(t, fmt.Sprint(p.Base, p.Rate, p.BonusSum, p.Total), "100000 769 0 100000")
	p = monthPay(fixed, 2451, 7800, []Bonus{{Title: "Релиз платёжного модуля в срок", Amount: 15_000, Day: 12}})
	assertEq(t, fmt.Sprint(p.BonusSum, p.Total), "15000 115000")
	assertEq(t, monthPay(fixed, 0, 0, nil).Rate, int64(0))
	hourly := &Pay{From: "2026-11", Kind: "hourly", Amount: 2000}
	assertEq(t, monthPay(hourly, 2451, 7800, nil).Total, int64(81_700))
	assertEq(t, monthPay(&Pay{Kind: "hourly", Amount: 1000}, 7, 0, nil).Base, int64(117)) // 116.67
	assertEq(t, monthPay(&Pay{Kind: "hourly", Amount: 1010}, 3, 0, nil).Base, int64(51))  // 50.5, half up
	assertEq(t, monthPay(nil, 100, 0, nil) == nil, true)
	assertEq(t, monthPay(nil, 100, 0, []Bonus{{Amount: 500}}).Total, int64(500))
}

func TestPayVersions(t *testing.T) {
	ps := setPay(setPay(nil, Pay{From: "2026-10", Kind: "fixed", Amount: 90_000}), Pay{From: "2026-12", Kind: "hourly", Amount: 2000})
	assertEq(t, payOn(ps, "2026-09") == nil, true)
	assertEq(t, payOn(ps, "2026-11").Amount, int64(90_000))
	assertEq(t, payOn(ps, "2027-01").Kind, "hourly")
	ps = setPay(ps, Pay{From: "2026-11", Kind: "none"})
	assertEq(t, fmt.Sprint(len(ps), payOn(ps, "2026-12") == nil), "2 true")
}

func TestPayAndBonusRoutes(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	assertEq(t, api.do("PUT", "/v1/projects/easy/pay", map[string]any{"from": "2026-11", "kind": "fixed", "amount": 100_000}, nil), http.StatusOK)
	assertEq(t, d.project("easy").Pay[0].Amount, int64(100_000))
	for _, bad := range []map[string]any{{"from": "ноябрь", "kind": "fixed", "amount": 1}, {"from": "2026-11", "kind": "daily", "amount": 1}, {"from": "2026-11", "kind": "fixed", "amount": 0}} {
		assertEq(t, api.do("PUT", "/v1/projects/easy/pay", bad, nil), http.StatusBadRequest)
	}
	var b Bonus
	assertEq(t, api.do("POST", "/v1/bonuses", map[string]any{"project": "easy", "month": "2026-11", "day": 12, "title": "Релиз", "amount": 15_000}, &b), http.StatusCreated)
	assertEq(t, b.ID, int64(1))
	assertEq(t, api.do("POST", "/v1/bonuses", map[string]any{"project": "easy", "month": "2026-11", "day": 31, "title": "x", "amount": 1}, nil), http.StatusBadRequest)
	assertEq(t, api.do("POST", "/v1/bonuses", map[string]any{"project": "easy", "month": "2026-11", "day": 1, "title": "", "amount": 1}, nil), http.StatusBadRequest)
	assertEq(t, api.do("DELETE", "/v1/bonuses/1", nil, nil), http.StatusNoContent)
	assertEq(t, api.do("DELETE", "/v1/bonuses/1", nil, nil), http.StatusNotFound)
}

func TestPayChangesAreLoggedAndUndone(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	cliHome(t, portOf(t, api.url))
	now := mskAt(2026, 11, 20, 12, 0)
	swap(t, &nowMs, func() int64 { return now })
	fixed := map[string]any{"from": "2026-11", "kind": "fixed", "amount": 100_000}
	api.do("PUT", "/v1/projects/easy/pay", fixed, nil)
	var out, errOut bytes.Buffer
	assertEq(t, runCLI([]string{"pay", "set", "easy", `{"from":"2026-11","kind":"fixed","amount":100000}`}, &out, &errOut), 0) // the same again: nothing new
	api.do("PUT", "/v1/projects/easy/pay", map[string]any{"from": "2026-12", "kind": "none"}, nil)
	var h struct {
		Changes []Change `json:"changes"`
	}
	api.do("GET", fmt.Sprintf("/v1/history?from=%d&to=%d", now-minMs, now+minMs), nil, &h)
	assertEq(t, len(h.Changes), 2)
	none, set := h.Changes[0], h.Changes[1]
	assertEq(t, fmt.Sprintf("%s %s %s %d %d", none.Kind, none.Project, none.Pay.Kind, len(none.Pays), len(none.PrevPays)), "pay-set easy none 2 1")
	assertEq(t, fmt.Sprint(len(set.PrevPays), set.Pay.Amount), "0 100000")
	assertEq(t, runCLI([]string{"history", "--from", strconv.FormatInt(now-minMs, 10), "--to", strconv.FormatInt(now+minMs, 10)}, &out, &errOut), 0)
	assertEq(t, strings.Contains(out.String(), fmt.Sprintf("pay-set easy  history undo %d\n", none.ID)), true)
	undo := func(id int64) int { return api.do("POST", fmt.Sprintf("/v1/history/%d/undo", id), nil, nil) }
	assertEq(t, undo(set.ID), http.StatusConflict) // the removal came after it
	assertEq(t, undo(none.ID), http.StatusNoContent)
	assertEq(t, len(d.project("easy").Pay), 1)
	assertEq(t, undo(set.ID), http.StatusNoContent)
	assertEq(t, len(d.project("easy").Pay), 0)
}
