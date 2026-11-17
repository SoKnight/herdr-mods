package main

import (
	"bytes"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestCorrectionRemembersWhatWasThere(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return at(60) })
	seed(t, d, power(0, 60), inp(0, 30), win("idea64.exe", "Easy – Main.kt", 0, 20), win("notepad.exe", "x", 20, 30))
	var c Correction
	assertEq(t, api.do("POST", "/v1/corrections", Correction{Kind: "project", Project: "other", Start: at(10), End: at(30)}, &c), http.StatusCreated)
	assertEq(t, c.Made, at(60))
	assertEq(t, fmt.Sprint(c.Was), fmt.Sprint([]Share{{"easy", 20 * minMs}})) // 20–30 carried
}

func TestRuleChangesAreLogged(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	now := mskAt(2026, 11, 16, 12, 0)
	swap(t, &nowMs, func() int64 { return now })
	seedWindow(t, d, mskAt(2026, 11, 10, 12, 0), "notepad.exe", "Spec draft", 20)
	var r Rule
	api.do("POST", "/v1/rules", map[string]any{"kind": "title", "value": "Spec", "project": "other"}, &r)
	api.do("PUT", fmt.Sprintf("/v1/rules/%d", r.ID), map[string]any{"kind": "title", "value": "Spec", "project": "easy", "scope": "today"}, nil)
	api.do("DELETE", "/v1/rules/3", nil, nil)
	var h struct {
		Changes []Change `json:"changes"`
	}
	assertEq(t, api.do("GET", fmt.Sprintf("/v1/history?from=%d&to=%d", now-minMs, now+minMs), nil, &h), http.StatusOK)
	assertEq(t, len(h.Changes), 3)
	assertEq(t, h.Changes[0].Kind, "rule-delete")
	assertEq(t, h.Changes[1].Kind, "rule-edit")
	assertEq(t, h.Changes[1].Prev.ID, r.ID)
	add := h.Changes[2]
	assertEq(t, add.Kind, "rule-add")
	assertEq(t, add.From, mskAt(2026, 11, 1, 6, 0))
	assertEq(t, add.Ms, int64(20*minMs))
}

func TestUndoRuleChanges(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return mskAt(2026, 11, 16, 12, 0) })
	changes := func() []Change { cs, err := d.Store.Changes(); must(t, err); return cs }
	undo := func(id int64) int { return api.do("POST", fmt.Sprintf("/v1/history/%d/undo", id), nil, nil) }

	var r Rule
	api.do("POST", "/v1/rules", map[string]any{"kind": "title", "value": "Spec", "project": "other"}, &r)
	add := changes()[0]
	assertEq(t, undo(add.ID), http.StatusNoContent)
	assertEq(t, slices.Contains(ruleIDs(d.Config.Rules), r.ID), false)
	assertEq(t, undo(add.ID), http.StatusNotFound)

	api.do("PUT", "/v1/rules/3", map[string]any{"kind": "title", "value": "Easy", "project": "other", "scope": "today"}, nil)
	edit := changes()[0]
	assertEq(t, undo(edit.ID), http.StatusNoContent)
	assertEq(t, fmt.Sprint(ruleIDs(d.Config.Rules)), "[1 2 3]")
	assertEq(t, d.Config.Rules[2].To, int64(0))

	api.do("POST", "/v1/rules", map[string]any{"kind": "title", "value": "Notes", "project": "other"}, &r)
	api.do("DELETE", fmt.Sprintf("/v1/rules/%d", r.ID), nil, nil) // started this month: removed
	del := changes()[0]
	assertEq(t, undo(del.ID), http.StatusNoContent)
	assertEq(t, d.Config.Rules[len(d.Config.Rules)-1].ID, r.ID) // back in its place

	api.do("PUT", fmt.Sprintf("/v1/rules/%d", r.ID), map[string]any{"kind": "title", "value": "Notes2", "project": "other"}, nil)
	var older Change
	for _, c := range changes() {
		if c.Kind == "rule-add" && c.Rule.ID == r.ID {
			older = c
		}
	}
	assertEq(t, undo(older.ID), http.StatusConflict) // the rule changed since
}

func TestHistoryRange(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	_, err := d.Store.AddCorrection(Correction{Kind: "exclude", Target: "me", Start: at(5), End: at(10)}) // from before `made`
	must(t, err)
	var h struct {
		Corrections []Correction `json:"corrections"`
	}
	assertEq(t, api.do("GET", fmt.Sprintf("/v1/history?from=%d&to=%d", at(0), at(60)), nil, &h), http.StatusOK)
	assertEq(t, len(h.Corrections), 1)
	api.do("GET", fmt.Sprintf("/v1/history?from=%d&to=%d", at(60), at(120)), nil, &h)
	assertEq(t, len(h.Corrections), 0)
}

// A deleted rule's id is never handed to a new rule, so undoing the delete cannot overwrite that rule.
func TestRuleIDsAreNeverReused(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return mskAt(2026, 11, 16, 12, 0) })
	var a, b Rule
	api.do("POST", "/v1/rules", map[string]any{"kind": "title", "value": "Notes", "project": "other"}, &a)
	api.do("DELETE", fmt.Sprintf("/v1/rules/%d", a.ID), nil, nil) // started this month: removed
	cs, err := d.Store.Changes()
	must(t, err)
	del := cs[0]
	assertEq(t, api.do("POST", "/v1/rules", map[string]any{"kind": "title", "value": "Spec", "project": "easy"}, &b), http.StatusCreated)
	assertEq(t, b.ID != a.ID, true)
	assertEq(t, api.do("POST", fmt.Sprintf("/v1/history/%d/undo", del.ID), nil, nil), http.StatusNoContent)
	assertEq(t, slices.Contains(d.Config.Rules, a), true)
	assertEq(t, slices.Contains(d.Config.Rules, b), true)
	assertEq(t, api.do("POST", "/v1/history/x/undo", nil, nil), http.StatusBadRequest)

	_, err = undoChange([]Rule{b}, Change{Kind: "rule-delete", Prev: &Rule{ID: b.ID, Kind: "title", Value: "x", Project: "easy"}})
	assertEq(t, err != nil, true) // a rule under the removed one's id is newer: never restored over
}

func TestCLIHistory(t *testing.T) {
	d := testDaemon(t)
	cliHome(t, portOf(t, serve(t, d).url))
	now := mskAt(2026, 11, 16, 12, 0)
	swap(t, &nowMs, func() int64 { return now })
	var out, errOut bytes.Buffer
	assertEq(t, runCLI([]string{"rule", "add", `{"kind":"title","value":"site","project":"easy"}`, "--json"}, &out, &errOut), 0)
	out.Reset()
	assertEq(t, runCLI([]string{"history", "--from", strconv.FormatInt(now-minMs, 10), "--to", strconv.FormatInt(now+minMs, 10)}, &out, &errOut), 0)
	assertEq(t, strings.HasSuffix(strings.TrimSpace(out.String()), "  rule-add  #1"), true)
	assertEq(t, runCLI([]string{"history", "undo", "1"}, &out, &errOut), 0)
	cs, err := d.Store.Changes()
	must(t, err)
	assertEq(t, len(cs), 0)
}
