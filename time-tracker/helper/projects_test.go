package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func ruleIDs(rs []Rule) []int64 {
	var ids []int64
	for _, r := range rs {
		ids = append(ids, r.ID)
	}
	return ids
}

func seedWindow(t *testing.T, d *Daemon, start int64, exe, title string, minutes int) {
	end := start + int64(minutes)*minMs
	seed(t, d, Span{Kind: "power", Start: start, End: end}, Span{Kind: "input", Start: start, End: end}, Span{Kind: "window", Start: start, End: end, Exe: exe, Title: title})
}

func dayProject(t *testing.T, api testAPI, day string) string {
	var a struct {
		Timeline Timeline `json:"timeline"`
	}
	assertEq(t, api.do("GET", "/v1/timeline?day="+day, nil, &a), http.StatusOK)
	return a.Timeline.Me[0].Project
}

func TestProjectsCreateAndChange(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	var p Project
	assertEq(t, api.do("POST", "/v1/projects", map[string]string{"name": "Easy Payments"}, &p), http.StatusCreated)
	assertEq(t, p.ID, "easy-payments")
	assertEq(t, p.Color, "ansi3")
	api.do("POST", "/v1/projects", map[string]string{"name": "Easy Payments"}, &p)
	assertEq(t, p.ID, "easy-payments-2")
	api.do("POST", "/v1/projects", map[string]string{"name": "Учёба", "color": "ansi6"}, &p)
	assertEq(t, p.ID, "project")
	assertEq(t, p.Color, "ansi6")
	assertEq(t, api.do("POST", "/v1/projects", map[string]string{"name": " "}, nil), http.StatusBadRequest)
	assertEq(t, api.do("PATCH", "/v1/projects/easy", map[string]any{"name": "Easy 2", "color": "ansi5", "archived": true}, &p), http.StatusOK)
	e := d.project("easy")
	assertEq(t, fmt.Sprintf("%s %s %v", e.Name, e.Color, e.Archived), "Easy 2 ansi5 true")
	assertEq(t, api.do("PATCH", "/v1/projects/easy", map[string]string{"color": "red"}, nil), http.StatusBadRequest)
	assertEq(t, api.do("PATCH", "/v1/projects/nope", map[string]string{"name": "x"}, nil), http.StatusNotFound)
}

func TestRuleAddPlacesAndDates(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return mskAt(2026, 11, 16, 12, 0) })
	var r Rule
	assertEq(t, api.do("POST", "/v1/rules", map[string]any{"kind": "folder", "value": `C:\Dev\new\**`, "project": "other"}, &r), http.StatusCreated)
	assertEq(t, r.Value, `C:\Dev\new`)
	assertEq(t, r.From, mskAt(2026, 11, 1, 6, 0))
	assertEq(t, fmt.Sprint(ruleIDs(d.Config.Rules)), "[1 2 4 3]") // after the folders, before the titles
	api.do("POST", "/v1/rules", map[string]any{"kind": "title", "value": "Spec", "project": "exclude", "scope": "today", "before": 1}, &r)
	assertEq(t, r.From, mskAt(2026, 11, 16, 6, 0))
	assertEq(t, fmt.Sprint(ruleIDs(d.Config.Rules)), "[5 1 2 4 3]")
}

func TestRuleEditFromTodayKeepsThePast(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	yesterday, today := mskAt(2026, 11, 15, 12, 0), mskAt(2026, 11, 16, 12, 0)
	swap(t, &nowMs, func() int64 { return today + 30*minMs })
	seedWindow(t, d, yesterday, "notepad.exe", "Spec draft", 20)
	seedWindow(t, d, today, "notepad.exe", "Spec draft", 20)
	var r, r2 Rule
	assertEq(t, api.do("POST", "/v1/rules", map[string]any{"kind": "title", "value": "Spec", "project": "other"}, &r), http.StatusCreated)
	assertEq(t, api.do("PUT", fmt.Sprintf("/v1/rules/%d", r.ID), map[string]any{"kind": "title", "value": "Spec", "project": "easy", "scope": "today"}, &r2), http.StatusOK)
	assertEq(t, r2.ID != r.ID, true)
	assertEq(t, dayProject(t, api, "2026-11-15"), "other")
	assertEq(t, dayProject(t, api, "2026-11-16"), "easy")
	var list struct {
		Rules []ruleInfo `json:"rules"`
	}
	api.do("GET", "/v1/rules", nil, &list)
	assertEq(t, slices.ContainsFunc(list.Rules, func(x ruleInfo) bool { return x.ID == r.ID }), false)
	assertEq(t, api.do("PUT", fmt.Sprintf("/v1/rules/%d", r.ID), map[string]any{"kind": "title", "value": "x", "project": "easy"}, nil), http.StatusNotFound)
}

func TestRuleEditInPlaceAndRaised(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return mskAt(2026, 11, 16, 12, 0) })
	var r, r2 Rule
	api.do("POST", "/v1/rules", map[string]any{"kind": "title", "value": "Spec", "project": "other", "scope": "today"}, &r)
	assertEq(t, api.do("PUT", fmt.Sprintf("/v1/rules/%d", r.ID), map[string]any{"kind": "title", "value": "Specs", "project": "other"}, &r2), http.StatusOK)
	assertEq(t, r2.ID, r.ID) // started on the scope's day or later: rewritten in place
	assertEq(t, r2.From, mskAt(2026, 11, 1, 6, 0))
	assertEq(t, api.do("PUT", "/v1/rules/3", map[string]any{"kind": "title", "value": "Easy", "project": "other", "scope": "today", "before": 1}, &r2), http.StatusOK)
	assertEq(t, fmt.Sprint(ruleIDs(d.Config.Rules)), fmt.Sprintf("[%d 1 2 3 %d]", r2.ID, r.ID)) // the new version went above rule 1
}

func TestRuleBadEditWritesNothing(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	file := filepath.Join(d.Home, "projects.json")
	before, err := os.ReadFile(file)
	must(t, err)
	assertEq(t, api.do("PUT", "/v1/rules/3", map[string]any{"kind": "title", "value": "x", "project": "nope"}, nil), http.StatusBadRequest)
	after, err := os.ReadFile(file)
	must(t, err)
	assertEq(t, string(after), string(before))
}

func TestMoveBeforeZeroMovesNothing(t *testing.T) {
	rules := []Rule{{ID: 0}, {ID: 1}, {ID: 2}}
	assertEq(t, fmt.Sprint(ruleIDs(moveBefore(rules, 2, 0))), "[0 1 2]")
}

func TestRuleDeleteKeepsPastMonths(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return mskAt(2026, 11, 16, 12, 0) })
	seedWindow(t, d, mskAt(2026, 10, 30, 12, 0), "notepad.exe", "Easy notes", 20)
	seedWindow(t, d, mskAt(2026, 11, 10, 12, 0), "notepad.exe", "Easy notes", 20)
	assertEq(t, api.do("DELETE", "/v1/rules/3", nil, nil), http.StatusNoContent)
	assertEq(t, dayProject(t, api, "2026-10-30"), "easy")
	assertEq(t, dayProject(t, api, "2026-11-10"), "")
	var r Rule
	api.do("POST", "/v1/rules", map[string]any{"kind": "title", "value": "Notes", "project": "other"}, &r)
	api.do("DELETE", fmt.Sprintf("/v1/rules/%d", r.ID), nil, nil)
	assertEq(t, slices.Contains(ruleIDs(d.Config.Rules), r.ID), false) // started this month: gone for good
}

func TestRuleRequestsChecked(t *testing.T) {
	api := serve(t, testDaemon(t))
	for body, want := range map[string]int{
		`{"kind":"url","value":"x","project":"easy"}`:                  http.StatusBadRequest,
		`{"kind":"title","value":" ","project":"easy"}`:                http.StatusBadRequest,
		`{"kind":"title","value":"x","project":"nope"}`:                http.StatusBadRequest,
		`{"kind":"title","value":"x","project":"leisure"}`:             http.StatusCreated,
		`{"kind":"title","value":"x","project":"easy","scope":"week"}`: http.StatusBadRequest,
	} {
		assertEq(t, fmt.Sprint(body, api.do("POST", "/v1/rules", body, nil)), fmt.Sprint(body, want))
	}
	assertEq(t, api.do("DELETE", "/v1/rules/99", nil, nil), http.StatusNotFound)
	var r, again Rule
	assertEq(t, api.do("POST", "/v1/rules", `{"kind":"title","value":"Main.kt (1)","project":"easy"}`, &r), http.StatusCreated)
	assertEq(t, api.do("POST", "/v1/rules", `{"kind":"title","value":"main.kt (1) ","project":"easy","scope":"today"}`, &again), http.StatusOK)
	assertEq(t, again, r) // the same open rule is answered as is
}

func TestRuleAddReadsFileFresh(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	file := filepath.Join(d.Home, "projects.json")
	c := d.Config
	c.Projects[1].Name = "Edited" // after the last reload
	must(t, writeJSON(file, c))
	assertEq(t, api.do("POST", "/v1/rules", map[string]any{"kind": "folder", "value": `C:\Dev\new`, "project": "easy"}, nil), http.StatusCreated)
	got, err := readConfig(d.Home)
	must(t, err)
	assertEq(t, got.Projects[1].Name, "Edited")
	must(t, os.WriteFile(file, []byte("{"), 0o644))
	var e struct {
		Error string `json:"error"`
	}
	assertEq(t, api.do("POST", "/v1/rules", map[string]any{"kind": "folder", "value": `C:\Dev\x`, "project": "easy"}, &e), http.StatusConflict)
	assertEq(t, strings.HasPrefix(e.Error, "projects.json не читается"), true)
	data, err := os.ReadFile(file)
	must(t, err)
	assertEq(t, string(data), "{")
}

func TestRulesListWithHits(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return at(60) })
	seed(t, d, power(0, 60), inp(0, 30), win("idea64.exe", "Easy – Main.kt", 0, 30))
	var list struct {
		Rules []ruleInfo `json:"rules"`
	}
	assertEq(t, api.do("GET", "/v1/rules", nil, &list), http.StatusOK)
	assertEq(t, fmt.Sprint(ruleIDs([]Rule{list.Rules[0].Rule, list.Rules[1].Rule, list.Rules[2].Rule})), "[1 2 3]")
	assertEq(t, list.Rules[2].Ms, int64(30*minMs))
	assertEq(t, list.Rules[2].Windows, 1)
	assertEq(t, api.do("POST", "/v1/corrections", Correction{Kind: "project", Project: "other", Start: at(10), End: at(30)}, nil), http.StatusCreated)
	assertEq(t, api.do("GET", "/v1/rules", nil, &list), http.StatusOK)
	assertEq(t, list.Rules[2].Ms, int64(10*minMs)) // the corrected part went to another project
}

func TestCLIRulesAndProjects(t *testing.T) {
	d := testDaemon(t)
	cliHome(t, portOf(t, serve(t, d).url))
	var out, errOut bytes.Buffer
	assertEq(t, runCLI([]string{"project", "add", `{"name":"Site"}`, "--json"}, &out, &errOut), 0)
	assertEq(t, runCLI([]string{"rule", "add", `{"kind":"title","value":"site","project":"site"}`, "--json"}, &out, &errOut), 0)
	out.Reset()
	assertEq(t, runCLI([]string{"rules"}, &out, &errOut), 0)
	assertEq(t, strings.Contains(out.String(), "заголовок «site» → site"), true)
	assertEq(t, runCLI([]string{"rule", "rm", "4", "--json"}, &out, &errOut), 0)
	assertEq(t, runCLI([]string{"project", "set", "site", `{"archived":true}`, "--json"}, &out, &errOut), 0)
}

func TestRulePreviewMatchesAndOverlap(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return at(120) })
	seed(t, d, power(0, 120), inp(0, 60), win("herdr-app.exe", "pwsh", 0, 20), win("notepad.exe", "Easy plan", 20, 30), win("notepad.exe", "Easy plan v2", 30, 35))
	for i := 0; i < 20; i++ {
		must(t, d.Store.AppendBeat(Heartbeat{Source: "herdr", Time: at(i), ProjectPath: `C:\Dev\other\x`}))
	}
	var p preview
	body := map[string]any{"kind": "title", "value": "plan", "project": "other"}
	assertEq(t, api.do("POST", "/v1/rules/preview", body, &p), http.StatusOK)
	assertEq(t, p.Ms, int64(15*minMs))
	assertEq(t, p.Windows, 2)
	assertEq(t, p.Top[0], titleMs{"Easy plan", 10 * minMs})
	assertEq(t, p.Overlap.Project, "easy")
	assertEq(t, p.Overlap.Rule.ID, int64(3))
	assertEq(t, p.Overlap.Ms, int64(15*minMs))
	body["before"] = 3
	p = preview{}
	api.do("POST", "/v1/rules/preview", body, &p)
	assertEq(t, p.Overlap == nil, true) // placed above it, it would win
	p = preview{}
	api.do("POST", "/v1/rules/preview", map[string]any{"kind": "process", "value": "herdr-app.exe", "project": "easy"}, &p)
	assertEq(t, p.Overlap.Rule.ID, int64(1)) // the folder rule above keeps it
	assertEq(t, len(d.Config.Rules), 3)      // nothing written
	p = preview{}
	edit := map[string]any{"id": 3, "kind": "title", "value": "plan", "project": "other"} // rule 3 being edited
	assertEq(t, api.do("POST", "/v1/rules/preview", edit, &p), http.StatusOK)
	assertEq(t, p.Ms, int64(15*minMs))
	assertEq(t, p.Overlap == nil, true)
}

func TestHintsFromOther(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return at(120) })
	seed(t, d, power(0, 120), inp(0, 40), win("herdr-app.exe", "pwsh", 0, 20), win("notepad.exe", "Payments spec", 20, 25), win("Figma.exe", "Payments — UI", 25, 35), win("notepad.exe", "random", 35, 40))
	for i := 0; i < 20; i++ {
		must(t, d.Store.AppendBeat(Heartbeat{Source: "herdr", Time: at(i), ProjectPath: `C:\Dev\payments\api`}))
	}
	var hs []hint
	assertEq(t, api.do("GET", "/v1/hints?name=payments", nil, &hs), http.StatusOK)
	assertEq(t, fmt.Sprint(hs), fmt.Sprint([]hint{{"folder", `C:\Dev\payments\api`, 20 * minMs}, {"title", "payments", 15 * minMs}, {"figma", "Payments — UI", 10 * minMs}}))
	assertEq(t, api.do("GET", "/v1/hints?days=3", nil, &hs), http.StatusOK)
	assertEq(t, len(hs), 3)
	assertEq(t, hs[0].Kind, "folder")
	assertEq(t, api.do("GET", "/v1/hints?days=0", nil, nil), http.StatusBadRequest)
}
