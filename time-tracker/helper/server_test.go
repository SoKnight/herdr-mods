package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func serverConfig() Config {
	return Config{
		Activities: []string{"development", "architecture", "design", "documentation", "study"},
		Projects: []Project{
			{ID: "easy", Name: "Easy", Paths: []string{`C:\Dev\easy`}, Titles: []string{"Easy"}},
			{ID: "other", Name: "Other", Paths: []string{`C:\Dev\other`}},
		},
		Apps: []AppRule{
			{Exe: "idea64.exe", Activity: "development", Source: "idea"},
			{Exe: "herdr-app.exe", Activity: "development", Source: "herdr"},
		},
	}
}

func testDaemon(t *testing.T) *Daemon {
	t.Helper()
	home := t.TempDir()
	must(t, writeJSON(filepath.Join(home, "projects.json"), serverConfig()))
	d, err := NewDaemon(home)
	must(t, err)
	d.S.Loc, d.Store.S.Loc = msk, msk
	return d
}

func seed(t *testing.T, d *Daemon, sp ...Span) {
	t.Helper()
	for _, s := range sp {
		must(t, d.Store.AppendSpan(s))
	}
}

type testAPI struct {
	t   *testing.T
	url string
}

func serve(t *testing.T, d *Daemon) testAPI {
	srv := httptest.NewServer(newServer(d))
	t.Cleanup(srv.Close)
	return testAPI{t, srv.URL}
}

// do sends body (a string as is, anything else as JSON) with the API header and decodes the answer into out.
func (a testAPI) do(method, path string, body, out any) int {
	a.t.Helper()
	var r io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		r = strings.NewReader(b)
	default:
		data, err := json.Marshal(b)
		must(a.t, err)
		r = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, a.url+path, r)
	must(a.t, err)
	req.Header.Set("X-Herdr-Time", "1")
	resp, err := http.DefaultClient.Do(req)
	must(a.t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	must(a.t, err)
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			a.t.Fatalf("%s %s: %v in %s", method, path, err, data)
		}
	}
	return resp.StatusCode
}

func exportBody(project string, from, to int64, round int) map[string]any {
	return map[string]any{"project": project, "from": from, "to": to, "format": "csv", "round": round}
}

func TestHeartbeatRequiresHeader(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	body := fmt.Sprintf(`{"source":"herdr","time":%d,"project_path":"C:\\Dev\\easy"}`, at(0))
	resp, err := http.Post(api.url+"/v1/heartbeat", "application/json", strings.NewReader(body))
	must(t, err)
	resp.Body.Close()
	assertEq(t, resp.StatusCode, http.StatusForbidden)
	assertEq(t, len(mustLoad(t, d.Store, at(-1), at(1)).Beats), 0)

	assertEq(t, api.do("POST", "/v1/heartbeat", body, nil), http.StatusNoContent)
	beats := mustLoad(t, d.Store, at(-1), at(1)).Beats
	assertEq(t, len(beats), 1)
	assertEq(t, beats[0].ProjectPath, `C:\Dev\easy`)

	bad := fmt.Sprintf(`{"source":"herdr","time":%d,"activity":"sleeping"}`, at(0))
	assertEq(t, api.do("POST", "/v1/heartbeat", bad, nil), http.StatusBadRequest)
	assertEq(t, api.do("POST", "/v1/heartbeat", `{"time":1}`, nil), http.StatusBadRequest)
	huge := fmt.Sprintf(`{"source":"herdr","time":%d,"entity":"%s"}`, at(0), strings.Repeat("a", 64<<10))
	assertEq(t, api.do("POST", "/v1/heartbeat", huge, nil), http.StatusRequestEntityTooLarge)
	assertEq(t, len(mustLoad(t, d.Store, at(-1), at(1)).Beats), 1)
}

func TestOnlyMyTimeTakesActivityNoteAndAdd(t *testing.T) {
	api := serve(t, testDaemon(t))
	for _, kind := range []string{"activity", "note", "add"} {
		for _, target := range []string{"agents", "both"} {
			var e struct {
				Error string `json:"error"`
			}
			c := Correction{Kind: kind, Target: target, Start: at(0), End: at(10), Activity: "design", Note: "n"}
			assertEq(t, api.do("POST", "/v1/corrections", c, &e), http.StatusBadRequest)
			assertEq(t, e.Error, "target")
		}
	}
	c := Correction{Kind: "exclude", Target: "agents", Start: at(0), End: at(10)}
	assertEq(t, api.do("POST", "/v1/corrections", c, nil), http.StatusCreated)
}

func TestPreflightOnlyForExtensions(t *testing.T) {
	api := serve(t, testDaemon(t))
	preflight := func(origin string) *http.Response {
		req, err := http.NewRequest("OPTIONS", api.url+"/v1/heartbeat", nil)
		must(t, err)
		req.Header.Set("Origin", origin)
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "x-herdr-time,content-type")
		resp, err := http.DefaultClient.Do(req)
		must(t, err)
		resp.Body.Close()
		return resp
	}
	assertEq(t, preflight("https://evil.example").StatusCode, http.StatusForbidden)
	ok := preflight("moz-extension://abc")
	assertEq(t, ok.StatusCode, http.StatusNoContent)
	assertEq(t, strings.Contains(ok.Header.Get("Access-Control-Allow-Headers"), "X-Herdr-Time"), true)
	assertEq(t, ok.Header.Get("Access-Control-Allow-Origin"), "moz-extension://abc")
}

func TestForeignHostRefused(t *testing.T) {
	api := serve(t, testDaemon(t))
	req, err := http.NewRequest("GET", api.url+"/v1/corrections", nil)
	must(t, err)
	req.Host = "evil.example:" + strings.Split(api.url, ":")[2] // DNS rebinding
	req.Header.Set("X-Herdr-Time", "1")
	resp, err := http.DefaultClient.Do(req)
	must(t, err)
	resp.Body.Close()
	assertEq(t, resp.StatusCode, http.StatusForbidden)
}

func TestCorrectionInClosedPeriodConflicts(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	var ex exportResult
	assertEq(t, api.do("POST", "/v1/export", exportBody("easy", at(0), at(60), 15), &ex), http.StatusOK)

	corr := Correction{Kind: "project", Target: "me", Start: at(30), End: at(40), Project: "easy"}
	var conflict struct {
		Error  string `json:"error"`
		Period int64  `json:"period"`
	}
	assertEq(t, api.do("POST", "/v1/corrections", corr, &conflict), http.StatusConflict)
	assertEq(t, conflict.Error, "closed")
	assertEq(t, conflict.Period, ex.Period)
	cs, err := d.Store.Corrections()
	must(t, err)
	assertEq(t, len(cs), 0)

	assertEq(t, api.do("POST", fmt.Sprintf("/v1/periods/%d/reopen", ex.Period), nil, nil), http.StatusNoContent)
	var created Correction
	assertEq(t, api.do("POST", "/v1/corrections", corr, &created), http.StatusCreated)
	assertEq(t, created.ID, int64(1))
	assertEq(t, created.Project, "easy")

	// Closing the period again protects the correction from removal too.
	assertEq(t, api.do("POST", "/v1/export", exportBody("easy", at(0), at(60), 15), &ex), http.StatusOK)
	assertEq(t, api.do("DELETE", "/v1/corrections/1", nil, &conflict), http.StatusConflict)
	assertEq(t, conflict.Period, ex.Period)
	var list []Correction
	assertEq(t, api.do("GET", "/v1/corrections", nil, &list), http.StatusOK)
	assertEq(t, len(list), 1)
}

// A start near the beginning of int64 time must not overflow the 62-day guard into a timeline from year −292M.
func TestCorrectionFromTheDawnOfTimeAnswers(t *testing.T) {
	d := testDaemon(t)
	d.S.CarryMinutes = 0 // with a carry, from−carry wraps past to and hides the walk
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return at(30) })
	done := make(chan int, 1)
	go func() {
		done <- api.do("POST", "/v1/corrections", Correction{Kind: "exclude", Start: math.MinInt64 + 1, End: at(20)}, nil)
	}()
	select {
	case code := <-done:
		assertEq(t, code, http.StatusCreated)
	case <-time.After(5 * time.Second):
		t.Fatal("the correction did not answer in 5 s")
	}
}

func TestCorrectionNeverReachesTheFuture(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return at(30) })
	var e struct {
		Error string `json:"error"`
	}
	assertEq(t, api.do("POST", "/v1/corrections", Correction{Kind: "exclude", Start: at(30), End: at(40)}, &e), http.StatusBadRequest)
	assertEq(t, e.Error, "start must be in the past")
	var created Correction
	assertEq(t, api.do("POST", "/v1/corrections", Correction{Kind: "exclude", Start: at(20), End: at(60)}, &created), http.StatusCreated)
	assertEq(t, created.End, at(30))
}

func TestExportClosesPeriodAndRounds(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	seed(t, d, power(0, 120), inp(0, 22), win("idea64.exe", "Easy – Main.kt", 0, 22))

	var ex exportResult
	assertEq(t, api.do("POST", "/v1/export", exportBody("easy", at(0), at(60), 15), &ex), http.StatusOK)
	for _, want := range []string{"2026-11-16,development,22,15,\n", "total,,22,15,\n"} {
		if !strings.Contains(ex.Content, want) {
			t.Errorf("export lacks %q:\n%s", want, ex.Content)
		}
	}

	var periods []map[string]any
	assertEq(t, api.do("GET", "/v1/periods?project=easy", nil, &periods), http.StatusOK)
	assertEq(t, len(periods), 1)
	p := periods[0]
	assertEq(t, int64(p["id"].(float64)), ex.Period)
	assertEq(t, int64(p["from"].(float64)), at(0))
	assertEq(t, int64(p["to"].(float64)), at(60))
	assertEq(t, p["delta_ms"].(float64), 0.0)
	if _, ok := p["me"]; ok {
		t.Errorf("period list carries segments: %v", p)
	}
	assertEq(t, api.do("GET", "/v1/periods?project=other", nil, &periods), http.StatusOK)
	assertEq(t, len(periods), 0)

	// The snapshot stays inside its period: the next day knows nothing of it.
	var next struct {
		Timeline    Timeline     `json:"timeline"`
		Divergences []Divergence `json:"divergences"`
	}
	assertEq(t, api.do("GET", "/v1/timeline?day=2026-11-17", nil, &next), http.StatusOK)
	assertEq(t, len(next.Timeline.Me), 0)
	assertEq(t, len(next.Divergences), 0)

	// A new rule that takes the time elsewhere shows up as a divergence, the export stays.
	must(t, d.setConfig(func() Config {
		c := serverConfig()
		c.Projects[0].Titles = nil
		return c
	}()))
	assertEq(t, api.do("GET", "/v1/periods", nil, &periods), http.StatusOK)
	assertEq(t, periods[0]["delta_ms"].(float64), float64(-22*minMs))

	assertEq(t, api.do("POST", "/v1/export", exportBody("easy", at(30), at(90), 15), nil), http.StatusConflict)
	assertEq(t, api.do("POST", "/v1/export", exportBody("other", at(0), at(60), 10), nil), http.StatusBadRequest)
}

func TestExportRefusesMinutesBilledToAnotherProject(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	seed(t, d, power(0, 120), inp(0, 22), win("idea64.exe", "Easy – Main.kt", 0, 10), win("idea64.exe", "Easy – Spec.kt", 10, 22))
	var a exportResult
	assertEq(t, api.do("POST", "/v1/export", exportBody("easy", at(0), at(60), 15), &a), http.StatusOK)

	c := serverConfig() // a new rule takes 10–22 to the other project
	c.Projects[0].Titles = []string{"Main"}
	c.Projects[1].Titles = []string{"Spec"}
	must(t, d.setConfig(c))
	var conflict struct {
		Error  string `json:"error"`
		Period int64  `json:"period"`
	}
	for _, body := range []map[string]any{exportBody("other", at(0), at(60), 15), exportBody("other", at(15), at(30), 15)} {
		assertEq(t, api.do("POST", "/v1/export", body, &conflict), http.StatusConflict)
		assertEq(t, conflict.Error, "closed")
		assertEq(t, conflict.Period, a.Period)
	}
	dry := exportBody("other", at(0), at(60), 15)
	dry["dry_run"] = true
	assertEq(t, api.do("POST", "/v1/export", dry, nil), http.StatusConflict) // the preview says so too

	// Range overlap alone is no conflict: other's time after 22 was never billed.
	seed(t, d, inp(30, 40), win("idea64.exe", "Spec v2", 30, 40))
	assertEq(t, api.do("POST", "/v1/export", exportBody("other", at(25), at(60), 15), nil), http.StatusOK)

	assertEq(t, api.do("POST", fmt.Sprintf("/v1/periods/%d/reopen", a.Period), nil, nil), http.StatusNoContent)
	var b exportResult
	assertEq(t, api.do("POST", "/v1/export", exportBody("other", at(0), at(25), 15), &b), http.StatusOK)
	if !strings.Contains(b.Content, "total,,12,15,") {
		t.Errorf("other's export:\n%s", b.Content)
	}
}

func TestTimelineFreshSkipsSnapshots(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	seed(t, d, power(0, 120), inp(0, 22), win("idea64.exe", "Easy – Main.kt", 0, 22))
	assertEq(t, api.do("POST", "/v1/export", exportBody("easy", at(0), at(60), 15), nil), http.StatusOK)
	must(t, d.setConfig(func() Config { // the title rule goes: fresh rules no longer find the project
		c := serverConfig()
		c.Projects[0].Titles = nil
		return c
	}()))

	type day struct {
		Timeline    Timeline     `json:"timeline"`
		Report      Report       `json:"report"`
		Divergences []Divergence `json:"divergences"`
	}
	var closed, fresh day
	assertEq(t, api.do("GET", "/v1/timeline?day=2026-11-16", nil, &closed), http.StatusOK)
	assertEq(t, closed.Report.Projects[0].Project, "easy")
	assertEq(t, closed.Report.Projects[0].Me, int64(22*minMs))
	assertEq(t, len(closed.Divergences), 1)

	assertEq(t, api.do("GET", "/v1/timeline?day=2026-11-16&fresh=1", nil, &fresh), http.StatusOK)
	assertEq(t, len(fresh.Report.Projects), 1)
	assertEq(t, fresh.Report.Projects[0].Project, "")
	assertEq(t, fresh.Report.Me, int64(22*minMs))
	assertEq(t, len(fresh.Divergences), 0)

	c, err := buildCall(d.S, "timeline", []string{"--day", "2026-11-16", "--fresh"})
	must(t, err)
	assertEq(t, c.path, "/v1/timeline?day=2026-11-16&fresh=1")
}

func TestProjectsInConfigOrder(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	var list catalog
	assertEq(t, api.do("GET", "/v1/projects", nil, &list), http.StatusOK)
	assertEq(t, len(list.Projects), 2)
	assertEq(t, fmt.Sprint(list.Projects[0]), fmt.Sprint(catalogItem{ID: "easy", Name: "Easy", Color: "ansi1"}))
	assertEq(t, list.Projects[1].ID, "other")
	assertEq(t, len(list.Activities), 5)
	assertEq(t, fmt.Sprint(list.Activities[1]), fmt.Sprint(catalogItem{ID: "architecture", Label: "Проектирование"}))

	must(t, d.setConfig(Config{Activities: []string{"review"}}))
	assertEq(t, api.do("GET", "/v1/projects", nil, &list), http.StatusOK)
	assertEq(t, len(list.Activities), 1)
	assertEq(t, fmt.Sprint(list.Activities[0]), fmt.Sprint(catalogItem{ID: "review", Label: "review"})) // an edited list, its id as the label
	assertEq(t, len(list.Projects), 0)

	c, err := buildCall(d.S, "projects", nil)
	must(t, err)
	assertEq(t, c.path, "/v1/projects")
}

// cliHome points the CLI at a temp home whose settings.json names the port.
func cliHome(t *testing.T, port int) {
	home := t.TempDir()
	t.Setenv("HERDR_TIME_HOME", home)
	must(t, os.WriteFile(filepath.Join(home, "settings.json"), []byte(fmt.Sprintf(`{"port":%d}`, port)), 0o644))
}

func portOf(t *testing.T, url string) int {
	p, err := strconv.Atoi(url[strings.LastIndex(url, ":")+1:])
	must(t, err)
	return p
}

func TestSecondInstanceRefuses(t *testing.T) {
	ours := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"version":"1.0.0"}`)
	}))
	defer ours.Close()
	cliHome(t, portOf(t, ours.URL))
	var out, errOut bytes.Buffer
	assertEq(t, runCLI([]string{"run"}, &out, &errOut), 0)
	assertEq(t, strings.Contains(out.String(), "уже запущен"), true)

	foreign := httptest.NewServer(http.NotFoundHandler())
	defer foreign.Close()
	cliHome(t, portOf(t, foreign.URL))
	out.Reset()
	errOut.Reset()
	assertEq(t, runCLI([]string{"run"}, &out, &errOut), 1)
	assertEq(t, strings.Contains(errOut.String(), "занят другим процессом"), true)
}

func TestCLIDownExitCode(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	cliHome(t, port)
	var out, errOut bytes.Buffer
	assertEq(t, runCLI([]string{"status", "--json"}, &out, &errOut), 3)
	assertEq(t, strings.TrimSpace(out.String()), `{"error":"down"}`)
}

func TestCLITalksToServer(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	cliHome(t, portOf(t, api.url))
	var out, errOut bytes.Buffer
	beat := fmt.Sprintf(`{"source":"herdr","time":%d}`, at(0))
	assertEq(t, runCLI([]string{"beat", beat, "--json"}, &out, &errOut), 0)
	assertEq(t, strings.TrimSpace(out.String()), "{}")
	assertEq(t, len(mustLoad(t, d.Store, at(-1), at(1)).Beats), 1)

	seed(t, d, power(0, 120), inp(0, 22), win("idea64.exe", "Easy – Main.kt", 0, 22))
	file := filepath.Join(t.TempDir(), "easy.md")
	args := []string{"export", "--project", "easy", "--from", fmt.Sprint(at(0)), "--to", fmt.Sprint(at(60)), "--format", "md", "--out", file}
	out.Reset()
	assertEq(t, runCLI(args, &out, &errOut), 0)
	md, err := os.ReadFile(file)
	must(t, err)
	assertEq(t, strings.Contains(string(md), "| Разработка | 15 |"), true)

	out.Reset()
	assertEq(t, runCLI(append(args, "--json"), &out, &errOut), 1)
	assertEq(t, strings.Contains(out.String(), `"error":"closed"`), true)
}

func TestExportNeverClosesTheFuture(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return at(30) })
	var ex exportResult
	assertEq(t, api.do("POST", "/v1/export", exportBody("easy", at(0), at(24*60), 15), &ex), http.StatusOK)
	ps, err := d.Store.Periods()
	must(t, err)
	assertEq(t, ps[0].To, at(30))
	assertEq(t, api.do("POST", "/v1/export", exportBody("other", at(40), at(100), 15), nil), http.StatusBadRequest)
}

func TestDryRunExportMatchesTheRealOneAndClosesNothing(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	seed(t, d, power(0, 120), inp(0, 22), win("idea64.exe", "Easy – Main.kt", 0, 22))
	body := exportBody("easy", at(0), at(60), 15)
	body["dry_run"] = true
	var dry, real exportResult
	assertEq(t, api.do("POST", "/v1/export", body, &dry), http.StatusOK)
	ps, err := d.Store.Periods()
	must(t, err)
	assertEq(t, len(ps), 0)

	assertEq(t, api.do("POST", "/v1/export", exportBody("easy", at(0), at(60), 15), &real), http.StatusOK)
	if dry.Content == "" || dry.Content != real.Content {
		t.Errorf("dry run differs:\n%s\nreal:\n%s", dry.Content, real.Content)
	}
	// A closed range answers a dry run with the same conflict.
	assertEq(t, api.do("POST", "/v1/export", body, nil), http.StatusConflict)
}

func TestDryRunRefusesOut(t *testing.T) {
	d := testDaemon(t)
	_, err := buildCall(d.S, "export", []string{"--from", "1", "--to", "2", "--project", "easy", "--dry-run", "--out", "x.md"})
	if err == nil || !strings.Contains(err.Error(), "--dry-run") {
		t.Fatalf("--dry-run with --out: %v", err)
	}
}

func TestCLIReopensWhenOutFails(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	cliHome(t, portOf(t, api.url))
	seed(t, d, power(0, 120), inp(0, 22), win("idea64.exe", "Easy – Main.kt", 0, 22))
	bad := filepath.Join(t.TempDir(), "missing", "easy.md")
	var out, errOut bytes.Buffer
	args := []string{"export", "--project", "easy", "--from", fmt.Sprint(at(0)), "--to", fmt.Sprint(at(60)), "--out", bad}
	assertEq(t, runCLI(args, &out, &errOut), 1)
	assertEq(t, errOut.Len() > 0, true)
	ps, err := d.Store.Periods()
	must(t, err)
	assertEq(t, len(ps), 0) // no file, so nothing stays closed
}

type sessionAnswer struct {
	Session *Session `json:"session"`
	Report  Report   `json:"report"`
	Idle    int      `json:"session_idle_minutes"`
}

func TestSessionStartAndEnd(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return at(0) })
	seed(t, d, power(-60, 0), inp(-30, 0), win("idea64.exe", "Easy – Main.kt", -30, 0))
	var got sessionAnswer
	assertEq(t, api.do("GET", "/v1/session", nil, &got), http.StatusOK)
	assertEq(t, got.Session.Start, at(-30))
	assertEq(t, got.Session.Open, true)
	assertEq(t, got.Report.Me, int64(30*minMs))
	assertEq(t, got.Idle, 45)
	act := func(a string) int { return api.do("POST", "/v1/session", map[string]string{"action": a}, nil) }
	assertEq(t, act("end"), http.StatusCreated)
	assertEq(t, act("end"), http.StatusConflict)
	got = sessionAnswer{}
	api.do("GET", "/v1/session", nil, &got)
	assertEq(t, got.Session.Open, false)
	assertEq(t, got.Session.End, at(0))
	assertEq(t, act("start"), http.StatusCreated)
	assertEq(t, act("start"), http.StatusConflict)
	assertEq(t, act("pause"), http.StatusBadRequest)
	got = sessionAnswer{}
	api.do("GET", "/v1/session", nil, &got)
	assertEq(t, got.Session.Open, true)
	assertEq(t, got.Session.Manual, true)
	assertEq(t, got.Session.Start, at(0))
}

func TestSessionEmpty(t *testing.T) {
	api := serve(t, testDaemon(t))
	var raw map[string]json.RawMessage
	assertEq(t, api.do("GET", "/v1/session", nil, &raw), http.StatusOK)
	assertEq(t, string(raw["session"]), "null")
}

func TestSessionKeepsItsStartAcrossDayStart(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	start, end := mskAt(2026, 11, 16, 5, 30), mskAt(2026, 11, 16, 6, 30)
	swap(t, &nowMs, func() int64 { return end + minMs })
	seed(t, d, Span{Kind: "power", Start: start, End: end}, Span{Kind: "input", Start: start, End: end},
		Span{Kind: "window", Start: start, End: end, Exe: "idea64.exe", Title: "Easy – Main.kt"})
	var got sessionAnswer
	api.do("GET", "/v1/session", nil, &got)
	assertEq(t, got.Session.Start, start)
}

func TestCLISession(t *testing.T) {
	d := testDaemon(t)
	cliHome(t, portOf(t, serve(t, d).url))
	var out, errOut bytes.Buffer
	assertEq(t, runCLI([]string{"session", "start", "--json"}, &out, &errOut), 0)
	out.Reset()
	assertEq(t, runCLI([]string{"session", "--json"}, &out, &errOut), 0)
	assertEq(t, strings.Contains(out.String(), `"manual":true`), true)
	assertEq(t, runCLI([]string{"session", "stop"}, &out, &errOut), 2)
}

type dayAnswer struct {
	Sessions   []Session `json:"sessions"`
	Stats      DayStats  `json:"stats"`
	LastSample int64     `json:"last_sample"`
}

func TestTimelineGivesSessionsAndStats(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return at(61) })
	seed(t, d, power(0, 61), inp(0, 20), inp(40, 60), win("idea64.exe", "Easy – Main.kt", 0, 60))
	assertEq(t, api.do("POST", "/v1/corrections", Correction{Kind: "project", Project: "other", Start: at(40), End: at(60)}, nil), http.StatusCreated)
	var got dayAnswer
	assertEq(t, api.do("GET", "/v1/timeline", nil, &got), http.StatusOK)
	assertEq(t, len(got.Sessions), 1)
	s := got.Sessions[0]
	assertEq(t, len(s.Stretches), 2)
	assertEq(t, s.Stretches[1].Project, "other")
	assertEq(t, s.Stretches[1].GapBefore, int64(20*minMs))
	assertEq(t, s.Breaks, int64(20*minMs))
	assertEq(t, got.Stats, DayStats{Switches: 1, Focus: 20 * minMs, Breaks: 20 * minMs})
	assertEq(t, got.LastSample, int64(0))
}

func TestTimelineSessionsFollowCorrections(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return at(200) })
	seed(t, d, power(0, 200), inp(0, 60), win("idea64.exe", "Easy – Main.kt", 0, 60))
	api.do("POST", "/v1/corrections", Correction{Kind: "exclude", Start: at(20), End: at(40)}, nil)
	var got dayAnswer
	api.do("GET", "/v1/timeline", nil, &got)
	assertEq(t, len(got.Sessions[0].Stretches), 2)
	assertEq(t, got.Sessions[0].Breaks, int64(20*minMs))
}

func TestTimelineCutsSessionsAtDayStart(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	start, end := mskAt(2026, 11, 16, 5, 30), mskAt(2026, 11, 16, 6, 30)
	swap(t, &nowMs, func() int64 { return end + minMs })
	seed(t, d, Span{Kind: "power", Start: start, End: end}, Span{Kind: "input", Start: start, End: end},
		Span{Kind: "window", Start: start, End: end, Exe: "idea64.exe", Title: "Easy – Main.kt"})
	var got dayAnswer
	api.do("GET", "/v1/timeline?day=2026-11-16", nil, &got)
	assertEq(t, got.Sessions[0].Start, mskAt(2026, 11, 16, 6, 0))
}

func TestTimelineKeepsSessionsInsideThePastDay(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	start, end := mskAt(2026, 11, 16, 5, 30), mskAt(2026, 11, 16, 5, 50)
	seed(t, d, Span{Kind: "power", Start: start, End: end}, Span{Kind: "input", Start: start, End: end},
		Span{Kind: "window", Start: start, End: end, Exe: "idea64.exe", Title: "Easy – Main.kt"})
	act := func(a string, at int64) {
		swap(t, &nowMs, func() int64 { return at })
		assertEq(t, api.do("POST", "/v1/session", map[string]string{"action": a}, nil), http.StatusCreated)
	}
	act("end", mskAt(2026, 11, 16, 6, 5))   // ends the session after the day's end
	act("start", mskAt(2026, 11, 16, 6, 10)) // a session of the next day, open now
	var got dayAnswer
	api.do("GET", "/v1/timeline?day=2026-11-15", nil, &got)
	assertEq(t, len(got.Sessions), 1)
	assertEq(t, got.Sessions[0].End, mskAt(2026, 11, 16, 6, 0))
	assertEq(t, got.Sessions[0].Open, false)
}

func TestTimelineEmptyDayGivesEmptyLists(t *testing.T) {
	api := serve(t, testDaemon(t))
	var raw map[string]json.RawMessage
	api.do("GET", "/v1/timeline?day=2026-11-16", nil, &raw)
	assertEq(t, string(raw["sessions"]), "[]")
	assertEq(t, string(raw["stats"]), `{"switches":0,"focus":0,"breaks":0}`)
}

func TestStatusStates(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	seed(t, d, power(-60, 10), inp(-30, 10), win("idea64.exe", "Easy – Main.kt", -30, 10))
	var st struct {
		State        string `json:"state"`
		SessionStart int64  `json:"session_start"`
		LastActive   int64  `json:"last_active"`
	}
	state := func(minute int) string {
		swap(t, &nowMs, func() int64 { return at(minute) })
		st.State, st.SessionStart, st.LastActive = "", -1, -1
		api.do("GET", "/v1/status", nil, &st)
		return st.State
	}
	assertEq(t, state(11), "working")
	assertEq(t, st.SessionStart, at(-30))
	assertEq(t, st.LastActive, at(10))
	assertEq(t, state(20), "away")
	assertEq(t, state(56), "none")
	assertEq(t, state(2), "working") // the input still runs at 2
	assertEq(t, api.do("POST", "/v1/session", map[string]string{"action": "end"}, nil), http.StatusCreated)
	assertEq(t, state(4), "none") // the input 2–4 after Завершить opens nothing
	assertEq(t, st.SessionStart, int64(0))
}

func TestStatusWorkingInAFreshManualSession(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	swap(t, &nowMs, func() int64 { return at(0) })
	assertEq(t, api.do("POST", "/v1/session", map[string]string{"action": "start"}, nil), http.StatusCreated)
	swap(t, &nowMs, func() int64 { return at(20) })
	var st struct {
		State      string `json:"state"`
		LastActive int64  `json:"last_active"`
	}
	api.do("GET", "/v1/status", nil, &st)
	assertEq(t, st.LastActive, int64(0))
	assertEq(t, st.State, "working") // nothing to be away from
}

func TestRestartStopsThenStarts(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	addr := ln.Addr().String()
	cliHome(t, ln.Addr().(*net.TCPAddr).Port)
	// fake answers like a collector; /v1/quit stops it.
	fake := func(ln net.Listener) {
		srv := &http.Server{}
		srv.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{"version":"1.0.0"}`)
			if r.URL.Path == "/v1/quit" {
				go srv.Close()
			}
		})
		t.Cleanup(func() { srv.Close() })
		go srv.Serve(ln)
	}
	fake(ln)
	launched := false
	swap(t, &launch, func(string) error {
		launched = true
		again, err := net.Listen("tcp", addr) // only free once the old one stopped
		if err == nil {
			fake(again)
		}
		return err
	})
	var out, errOut bytes.Buffer
	assertEq(t, runCLI([]string{"restart", "--json"}, &out, &errOut), 0)
	assertEq(t, launched, true)
	assertEq(t, strings.TrimSpace(out.String()), "{}")
}

func TestRestartReportsWhenNothingStarts(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	must(t, err)
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	cliHome(t, port)
	swap(t, &launch, func(string) error { return nil })
	swap(t, &restartWait, 300*time.Millisecond)
	var out, errOut bytes.Buffer
	assertEq(t, runCLI([]string{"restart", "--json"}, &out, &errOut), 1)
	assertEq(t, strings.TrimSpace(out.String()), `{"error":"сборщик не запустился"}`)
}
