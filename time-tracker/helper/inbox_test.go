package main

import (
	"fmt"
	"net/http"
	"path/filepath"
	"testing"
)

func TestInboxGroupsOtherWindows(t *testing.T) {
	d := testDaemon(t)
	d.S.CarryMinutes = 0
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return at(60) })
	seed(t, d, power(0, 60), inp(0, 30), win("notepad.exe", "chat", 0, 5), win("idea64.exe", "Easy – Main.kt", 5, 20), win("notepad.exe", "chat", 20, 23))
	var got struct {
		Ms    int64       `json:"ms"`
		Items []inboxItem `json:"items"`
	}
	assertEq(t, api.do("GET", "/v1/inbox?day=2026-11-16", nil, &got), http.StatusOK)
	assertEq(t, len(got.Items), 1)
	it := got.Items[0]
	assertEq(t, fmt.Sprint(it.App, " ", it.Title, it.Ms, it.Start, it.End), fmt.Sprint("notepad.exe chat", 8*minMs, at(0), at(23)))
	assertEq(t, got.Ms, int64(8*minMs))
}

func TestInboxSuggests(t *testing.T) {
	projects := []Project{{ID: "easy", Name: "Easy"}, {ID: "tricky", Name: "Tricky Network"}, {ID: "old", Name: "Archive", Archived: true}}
	s := suggest(inboxItem{App: "chrome.exe", Title: "tricky-network/core · Pull requests"}, projects)
	assertEq(t, fmt.Sprint(s.Project, " ", s.Rule.Kind, " ", s.Rule.Value), "tricky title tricky")
	s = suggest(inboxItem{App: "herdr-app.exe", Title: "pwsh", Path: `C:\Dev\easy-site`}, projects)
	assertEq(t, fmt.Sprint(s.Project, " ", s.Rule.Kind, " ", s.Rule.Value), `easy folder C:\Dev\easy-site`)
	s = suggest(inboxItem{App: "Telegram.exe", Title: "Избранное"}, projects)
	assertEq(t, fmt.Sprint(s.Project, " ", s.Rule.Kind, " ", s.Rule.Value), "exclude process Telegram.exe")
	assertEq(t, suggest(inboxItem{App: "x.exe", Title: "Archive notes"}, projects) == nil, true)
}

func TestInboxApply(t *testing.T) {
	d := testDaemon(t)
	d.S.CarryMinutes = 0
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return mskAt(2026, 11, 16, 12, 0) })
	day := mskAt(2026, 11, 10, 12, 0) // a past day
	seedWindow(t, d, day, "notepad.exe", "Spec draft", 10)
	seedWindow(t, d, day+20*minMs, "Telegram.exe", "chat", 5)
	body := map[string]any{"day": "2026-11-10", "past": false, "decisions": []map[string]any{
		{"app": "notepad.exe", "title": "Spec draft", "project": "other", "rule": map[string]string{"kind": "title", "value": "Spec"}},
		{"app": "Telegram.exe", "title": "chat", "project": "exclude"},
	}}
	var res struct{ Rules, Corrections int }
	assertEq(t, api.do("POST", "/v1/inbox", body, &res), http.StatusOK)
	assertEq(t, fmt.Sprint(res.Rules, res.Corrections), "1 1")
	r := d.Config.Rules[len(d.Config.Rules)-1]
	assertEq(t, r.From, mskAt(2026, 11, 10, 6, 0)) // the day itself is covered
	assertEq(t, dayProject(t, api, "2026-11-10"), "other")
	cs, err := d.Store.Corrections()
	must(t, err)
	assertEq(t, fmt.Sprint(cs[0].Kind, cs[0].Start, cs[0].End), fmt.Sprint("exclude", day+20*minMs, day+25*minMs))
	ch, err := d.Store.Changes()
	must(t, err)
	assertEq(t, ch[0].Source, "inbox")

	body["day"], body["past"] = "2026-11-16", true
	body["decisions"] = []map[string]any{{"app": "notepad.exe", "title": "x", "project": "easy", "rule": map[string]string{"kind": "title", "value": "Plan"}}}
	api.do("POST", "/v1/inbox", body, nil)
	assertEq(t, d.Config.Rules[len(d.Config.Rules)-1].From, mskAt(2026, 11, 1, 6, 0))
}

func TestInboxApplyRefusesClosedPeriod(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return mskAt(2026, 11, 16, 12, 0) })
	day := mskAt(2026, 11, 10, 12, 0)
	seedWindow(t, d, day, "Telegram.exe", "chat", 5)
	_, err := d.Store.AddPeriod(Period{Project: "easy", From: day - 60*minMs, To: day + 60*minMs})
	must(t, err)
	before := len(d.Config.Rules)
	body := map[string]any{"day": "2026-11-10", "decisions": []map[string]any{
		{"app": "notepad.exe", "title": "y", "project": "other", "rule": map[string]string{"kind": "title", "value": "y"}},
		{"app": "Telegram.exe", "title": "chat", "project": "exclude"},
	}}
	assertEq(t, api.do("POST", "/v1/inbox", body, nil), http.StatusConflict)
	assertEq(t, len(d.Config.Rules), before)
	cs, _ := d.Store.Corrections()
	assertEq(t, len(cs), 0)
	ch, _ := d.Store.Changes()
	assertEq(t, len(ch), 0)
}

func TestInboxApplyRefusedOnConfigError(t *testing.T) {
	home := t.TempDir()
	c := serverConfig()
	c.Projects[0].Titles = []string{"("} // projects.json fails: the defaults collect, and their timeline is not mine
	must(t, writeJSON(filepath.Join(home, "projects.json"), c))
	d, err := NewDaemon(home)
	must(t, err)
	d.S.Loc, d.Store.S.Loc = msk, msk
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return mskAt(2026, 11, 16, 12, 0) })
	seedWindow(t, d, mskAt(2026, 11, 10, 12, 0), "Telegram.exe", "chat", 5)
	body := map[string]any{"day": "2026-11-10", "decisions": []map[string]any{{"app": "Telegram.exe", "title": "chat", "project": "exclude"}}}
	assertEq(t, api.do("POST", "/v1/inbox", body, nil), http.StatusInternalServerError)
	cs, _ := d.Store.Corrections()
	assertEq(t, len(cs), 0)
	ch, _ := d.Store.Changes()
	assertEq(t, len(ch), 0)
}

func TestInboxCorrectsOnlyTheOtherParts(t *testing.T) {
	d := testDaemon(t)
	d.S.CarryMinutes = 0
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return mskAt(2026, 11, 16, 12, 0) })
	day := mskAt(2026, 11, 10, 12, 0)
	for m := 0; m < 30; m += 10 { // a rule decides a span by its start, so the window comes in three
		seedWindow(t, d, day+int64(m)*minMs, "notepad.exe", "Spec draft", 10)
	}
	c := serverConfig() // a rule took 10–20 of the window to another project
	c.Rules = []Rule{{ID: 100, Kind: "title", Value: "Spec", Project: "other", From: day + 10*minMs, To: day + 20*minMs}}
	must(t, d.setConfig(c))
	body := map[string]any{"day": "2026-11-10", "decisions": []map[string]any{{"app": "notepad.exe", "title": "Spec draft", "project": "easy"}}}
	var res struct{ Rules, Corrections int }
	assertEq(t, api.do("POST", "/v1/inbox", body, &res), http.StatusOK)
	assertEq(t, fmt.Sprint(res.Rules, res.Corrections), "0 2")
	cs, err := d.Store.Corrections()
	must(t, err)
	got := ""
	for _, c := range cs {
		got += fmt.Sprintln(c.Start, c.End, c.Was)
	}
	want := fmt.Sprintln(day, day+10*minMs, []Share{{"", 10 * minMs}}) + fmt.Sprintln(day+20*minMs, day+30*minMs, []Share{{"", 10 * minMs}})
	assertEq(t, got, want)
}

func TestInboxApplyWritesNothingWhenRefused(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return mskAt(2026, 11, 16, 12, 0) })
	seedWindow(t, d, mskAt(2026, 11, 10, 12, 0), "Telegram.exe", "chat", 5)
	before := len(d.Config.Rules)
	good := map[string]any{"app": "Telegram.exe", "title": "chat", "project": "exclude"}
	rule := map[string]any{"app": "a.exe", "title": "a", "project": "other", "rule": map[string]string{"kind": "title", "value": "a"}}
	for _, c := range []struct {
		day  string
		decs []map[string]any
		code int
	}{
		{"2026-11-10", []map[string]any{rule, good, {"app": "b.exe", "project": "nope"}}, http.StatusBadRequest},
		{"2026-11-10", []map[string]any{rule, good, {"app": "b.exe", "project": "other", "rule": map[string]string{"kind": "url", "value": "b"}}}, http.StatusBadRequest},
		{"2026-11-10", []map[string]any{good, {"app": "b.exe", "project": "other", "rule": map[string]string{"kind": "title"}}}, http.StatusBadRequest},
		{"10.11.2026", []map[string]any{rule, good}, http.StatusBadRequest},
		{"2026-11-17", []map[string]any{rule, good}, http.StatusBadRequest}, // not yet begun
	} {
		assertEq(t, api.do("POST", "/v1/inbox", map[string]any{"day": c.day, "decisions": c.decs}, nil), c.code)
	}
	assertEq(t, api.do("POST", "/v1/inbox", `{"decisions":[{"rule":null`, nil), http.StatusBadRequest)
	assertEq(t, api.do("GET", "/v1/inbox?day=x", nil, nil), http.StatusBadRequest)
	assertEq(t, len(d.Config.Rules), before)
	cs, _ := d.Store.Corrections()
	assertEq(t, len(cs), 0)
	ch, _ := d.Store.Changes()
	assertEq(t, len(ch), 0)
}

func TestInboxApplyMergesAndDedupes(t *testing.T) {
	d := testDaemon(t)
	d.S.CarryMinutes = 0
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return mskAt(2026, 11, 16, 12, 0) })
	day := mskAt(2026, 11, 10, 12, 0)
	seedWindow(t, d, day, "Telegram.exe", "chat", 5)
	seedWindow(t, d, day+5*minMs, "Telegram.exe", "chat", 5) // touches the first
	seedWindow(t, d, day+30*minMs, "Telegram.exe", "chat", 5)
	before := len(d.Config.Rules)
	spec := map[string]string{"kind": "title", "value": "Spec"}
	body := map[string]any{"day": "2026-11-10", "decisions": []map[string]any{
		{"app": "Telegram.exe", "title": "chat", "project": "easy"},
		{"app": "notepad.exe", "title": "Spec 1", "project": "other", "rule": spec},
		{"app": "notepad.exe", "title": "Spec 2", "project": "other", "rule": spec},
	}}
	var res struct{ Rules, Corrections int }
	assertEq(t, api.do("POST", "/v1/inbox", body, &res), http.StatusOK)
	assertEq(t, fmt.Sprint(res.Rules, res.Corrections), "1 2")
	assertEq(t, len(d.Config.Rules), before+1)
	cs, err := d.Store.Corrections()
	must(t, err)
	c := cs[0]
	assertEq(t, fmt.Sprintln(c.Kind, c.Target, c.Project, c.Start, c.End, c.Made, c.Was),
		fmt.Sprintln("project", "me", "easy", day, day+10*minMs, mskAt(2026, 11, 16, 12, 0), []Share{{"", 10 * minMs}}))
	ch, err := d.Store.Changes()
	must(t, err)
	assertEq(t, fmt.Sprint(len(ch), ch[0].Kind, ch[0].Rule.ID, ch[0].From), fmt.Sprint(1, "rule-add", d.Config.Rules[len(d.Config.Rules)-1].ID, mskAt(2026, 11, 10, 6, 0)))
}

func TestCLIInbox(t *testing.T) {
	d := testDaemon(t)
	c, err := buildCall(d.S, "inbox", []string{"--day", "2026-11-16"})
	must(t, err)
	assertEq(t, c.method+" "+c.path, "GET /v1/inbox?day=2026-11-16")
	c, err = buildCall(d.S, "inbox", []string{"apply", `{"day":"2026-11-16"}`})
	must(t, err)
	assertEq(t, c.method+" "+c.path+" "+string(c.body), `POST /v1/inbox {"day":"2026-11-16"}`)
	_, err = buildCall(d.S, "unassigned", []string{"--from", "1", "--to", "2"})
	assertEq(t, err != nil, true)
}
