package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
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
	var list []map[string]string
	assertEq(t, api.do("GET", "/v1/projects", nil, &list), http.StatusOK)
	assertEq(t, len(list), 2)
	assertEq(t, list[0]["id"], "easy")
	assertEq(t, list[0]["name"], "Easy")
	assertEq(t, list[1]["id"], "other")

	c, err := buildCall(d.S, "projects", nil)
	must(t, err)
	assertEq(t, c.path, "/v1/projects")
}

func TestRuleAddRewritesUnassigned(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	seed(t, d, power(0, 60), inp(0, 30), win("herdr-app.exe", "shell", 0, 30))
	for i := 0; i < 30; i++ {
		must(t, d.Store.AppendBeat(Heartbeat{Source: "herdr", Time: at(i), ProjectPath: `C:\Dev\new`}))
	}
	q := fmt.Sprintf("/v1/unassigned?from=%d&to=%d", at(0), at(60))
	var un []struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
		Exe   string `json:"exe"`
		Ms    int64  `json:"ms"`
	}
	assertEq(t, api.do("GET", q, nil, &un), http.StatusOK)
	assertEq(t, len(un), 1)
	assertEq(t, un[0].Kind, "path")
	assertEq(t, un[0].Value, `C:\Dev\new`)
	assertEq(t, un[0].Ms, int64(30*minMs))

	file := filepath.Join(d.Home, "projects.json")
	before, err := os.ReadFile(file)
	must(t, err)
	assertEq(t, api.do("POST", "/v1/rules", map[string]string{"project": "easy", "kind": "title", "value": "("}, nil), http.StatusBadRequest)
	after, err := os.ReadFile(file)
	must(t, err)
	assertEq(t, string(after), string(before))

	assertEq(t, api.do("POST", "/v1/rules", map[string]string{"project": "easy", "kind": "path", "value": `C:\Dev\new`}, nil), http.StatusNoContent)
	assertEq(t, api.do("GET", q, nil, &un), http.StatusOK)
	assertEq(t, len(un), 0)

	var day struct {
		Timeline Timeline `json:"timeline"`
	}
	assertEq(t, api.do("GET", "/v1/timeline?day=2026-11-16", nil, &day), http.StatusOK)
	var total int64
	for _, s := range day.Timeline.Me {
		assertEq(t, s.Project, "easy")
		total += s.End - s.Start
	}
	assertEq(t, total, int64(30*minMs))

	c, err := LoadConfig(d.Home)
	must(t, err)
	assertEq(t, c.Projects[0].Paths[1], `C:\Dev\new`)
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

func TestRuleAddReadsFileFresh(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	file := filepath.Join(d.Home, "projects.json")
	c := serverConfig()
	c.Projects[1].Titles = []string{"UserEdit"} // edited after the last reload
	must(t, writeJSON(file, c))
	rule := map[string]string{"project": "easy", "kind": "path", "value": `C:\Dev\new`}
	assertEq(t, api.do("POST", "/v1/rules", rule, nil), http.StatusNoContent)
	got, err := readConfig(d.Home)
	must(t, err)
	assertEq(t, got.Projects[1].Titles[0], "UserEdit")
	assertEq(t, got.Projects[0].Paths[1], `C:\Dev\new`)

	must(t, os.WriteFile(file, []byte("{"), 0o644))
	var e struct {
		Error string `json:"error"`
	}
	assertEq(t, api.do("POST", "/v1/rules", rule, &e), http.StatusConflict)
	assertEq(t, strings.HasPrefix(e.Error, "projects.json не читается"), true)
	data, err := os.ReadFile(file)
	must(t, err)
	assertEq(t, string(data), "{")
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
