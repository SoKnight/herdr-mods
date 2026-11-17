package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// swap replaces a test seam for the rest of the test.
func swap[T any](t *testing.T, p *T, v T) {
	old := *p
	*p = v
	t.Cleanup(func() { *p = old })
}

func kindIntervals(raw Raw, kind string) []Interval {
	var sp []Span
	for _, s := range raw.Spans {
		if s.Kind == kind {
			sp = append(sp, s)
		}
	}
	var out []Interval
	for _, s := range mergeSpans(sp, 1) {
		out = append(out, Interval{s.Start, s.End})
	}
	return out
}

func TestDaemonRestartBackfillsFromBoot(t *testing.T) {
	swap(t, &nowMs, func() int64 { return at(30) })
	for _, tc := range []struct {
		name        string
		boot, logon int
		gap         Interval
	}{
		{"killed", -120, -110, m(0, 30)},      // booted and logged on before the last poll: the gap starts there
		{"rebooted", 10, 12, m(12, 30)},       // after a reboot the gap starts at logon
		{"fast startup", -600, 10, m(10, 30)}, // the tick count survived the "shutdown", the logon did not
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := testDaemon(t)
			must(t, d.Store.SaveOpen([]Span{{Kind: "power", Start: at(-60)}, {Kind: "window", Start: at(-10), Exe: "idea64.exe"}}, at(0)))
			swap(t, &bootAt, func(int64) int64 { return at(tc.boot) })
			swap(t, &logonAt, func() int64 { return at(tc.logon) })
			must(t, d.recoverOpen())
			raw := mustLoad(t, d.Store, at(-700), at(60))
			nodata, pw := kindIntervals(raw, "nodata"), kindIntervals(raw, "power")
			assertEq(t, len(nodata), 1)
			assertEq(t, nodata[0], tc.gap)
			assertEq(t, pw[0].Start, at(-60))
			last := tc.gap // power is on through the gap; it joins the recovered power when it starts there
			if last.Start == at(0) {
				last.Start = at(-60)
			}
			assertEq(t, pw[len(pw)-1], last)
			assertEq(t, kindIntervals(raw, "window")[0], m(-10, 0))
		})
	}
}

func TestStatusCountsOpenSpans(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	now := at(0)
	swap(t, &nowMs, func() int64 { return now })
	swap(t, &takeSample, func(ms int64) (Sample, error) {
		return Sample{At: ms, LastInput: ms, Exe: "idea64.exe", Title: "Easy – Main.kt"}, nil
	})
	d.agentsSeen(working(`C:\Dev\easy`))
	for ; now <= at(1); now += 5000 {
		d.poll()
	}
	now = at(1)
	assertEq(t, len(mustLoad(t, d.Store, at(-10), at(10)).Spans), 0) // everything is still open

	var st struct {
		Version  string `json:"version"`
		Me       int64  `json:"today_me_ms"`
		Agents   int64  `json:"today_agents_ms"`
		Project  string `json:"project"`
		Name     string `json:"project_name"`
		Activity string `json:"activity"`

		LastSample int64 `json:"last_sample"`
	}
	assertEq(t, api.do("GET", "/v1/status", nil, &st), http.StatusOK)
	assertEq(t, st.Version, version)
	assertEq(t, st.Me, int64(minMs))
	assertEq(t, st.Agents, int64(minMs))
	assertEq(t, st.Project, "easy")
	assertEq(t, st.Name, "Easy")
	assertEq(t, st.Activity, "development")
	assertEq(t, st.LastSample, at(1))
}

func TestQuitStopsRun(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	swap(t, &bootAt, func(int64) int64 { return 0 })
	swap(t, &logonAt, func() int64 { return 0 })
	swap(t, &followAgents, func(context.Context, func(map[string]AgentState), func()) {})
	swap(t, &takeSample, func(ms int64) (Sample, error) { return Sample{At: ms, LastInput: ms, Exe: "idea64.exe"}, nil })
	done := make(chan error)
	go func() { done <- d.Run(context.Background()) }()
	assertEq(t, api.do("POST", "/v1/quit", nil, nil), http.StatusNoContent)
	select {
	case err := <-done:
		must(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop")
	}
	_, err := os.Stat(filepath.Join(d.Home, "open.json"))
	must(t, err) // the next start recovers from it, as after a crash
}

func TestPollGapCutsAgents(t *testing.T) {
	d := testDaemon(t)
	now := at(0)
	swap(t, &nowMs, func() int64 { return now })
	swap(t, &takeSample, func(ms int64) (Sample, error) { return Sample{At: ms, LastInput: ms, Exe: "idea64.exe"}, nil })
	d.agentsSeen(working(`C:\Dev\easy`))
	d.poll()
	now += 5000
	d.poll()
	now = at(10) // slept for ten minutes
	d.poll()
	now += 5000
	var agents []Span
	for _, sp := range append(mustLoad(t, d.Store, at(-1), at(20)).Spans, d.openSpans(now)...) {
		if sp.Kind == "agent" {
			agents = append(agents, sp)
			if sp.Start < at(5) && at(5) < sp.End {
				t.Errorf("agent span %v covers the sleep", sp)
			}
		}
	}
	assertEq(t, len(agents), 2)
	assertEq(t, agents[0].End, at(0)+5000)
	assertEq(t, agents[1].Start, at(10))
}

func TestReloadConfigByMtime(t *testing.T) {
	d := testDaemon(t)
	file := filepath.Join(d.Home, "projects.json")
	stamp := time.Now()
	write := func(data []byte) {
		t.Helper()
		must(t, os.WriteFile(file, data, 0o644))
		stamp = stamp.Add(time.Minute)
		must(t, os.Chtimes(file, stamp, stamp))
	}
	byTitle := func() string { r, _ := d.Rules.Match(window{exe: "x", title: "Other window"}); return r.Project }
	assertEq(t, byTitle(), "")

	c := serverConfig()
	c.Projects[1].Titles = []string{"Other"}
	data, err := json.Marshal(c)
	must(t, err)
	write(data)
	d.reload()
	assertEq(t, byTitle(), "other")

	write([]byte("{"))
	d.reload()
	assertEq(t, byTitle(), "other") // broken file: previous rules kept

	must(t, os.Remove(file))
	d.reload()
	assertEq(t, byTitle(), "other") // missing file: kept, and not recreated
	_, err = os.Stat(file)
	assertEq(t, os.IsNotExist(err), true)
}

func TestLogonTimeIsSane(t *testing.T) {
	now := time.Now().UnixMilli()
	got := logonTime()
	if got == 0 {
		t.Skip("no session logon time here (not Windows, or no interactive session)")
	}
	if got > now || got < now-90*24*3600_000 {
		t.Fatalf("logon %v is not within the last 90 days", time.UnixMilli(got))
	}
}

func TestGarbageOpenJSONStillStarts(t *testing.T) {
	d := testDaemon(t)
	swap(t, &bootAt, func(int64) int64 { return 0 })
	swap(t, &logonAt, func() int64 { return 0 })
	open := filepath.Join(d.Home, "open.json")
	must(t, os.WriteFile(open+".bad", []byte("older"), 0o644))
	must(t, os.WriteFile(open, []byte("\x00garbage{"), 0o644))
	must(t, d.recoverOpen())
	_, err := os.Stat(open)
	assertEq(t, os.IsNotExist(err), true)
	bad, err := os.ReadFile(open + ".bad")
	must(t, err)
	assertEq(t, string(bad), "\x00garbage{") // the older .bad gave way
}

func TestBadProjectsStillCollects(t *testing.T) {
	home := t.TempDir()
	file := filepath.Join(home, "projects.json")
	c := serverConfig()
	c.Projects[0].Titles = []string{"("}
	must(t, writeJSON(file, c))
	d, err := NewDaemon(home)
	must(t, err)
	d.S.Loc, d.Store.S.Loc = msk, msk
	api := serve(t, d)

	now := at(0)
	swap(t, &nowMs, func() int64 { return now })
	swap(t, &takeSample, func(ms int64) (Sample, error) { return Sample{At: ms, LastInput: ms, Exe: "idea64.exe"}, nil })
	d.poll()
	assertEq(t, find(d.openSpans(now), "window").Exe, "idea64.exe") // collecting all the same

	var e struct {
		Error   string `json:"error"`
		Version string `json:"version"`
	}
	assertEq(t, api.do("GET", "/v1/status", nil, &e), http.StatusInternalServerError)
	assertEq(t, strings.HasPrefix(e.Error, "config: "), true)
	assertEq(t, strings.Contains(e.Error, "project easy"), true)
	assertEq(t, e.Version, version)
	assertEq(t, api.do("GET", "/v1/projects", nil, nil), http.StatusInternalServerError)
	assertEq(t, api.do("POST", "/v1/heartbeat", fmt.Sprintf(`{"source":"herdr","time":%d}`, now), nil), http.StatusNoContent)

	c.Projects[0].Titles = nil // fixed: the next reload clears the error
	must(t, writeJSON(file, c))
	stamp := time.Now().Add(time.Minute)
	must(t, os.Chtimes(file, stamp, stamp))
	d.poll()
	assertEq(t, api.do("GET", "/v1/status", nil, nil), http.StatusOK)
}

func TestBadSettingsStillCollects(t *testing.T) {
	home := t.TempDir()
	must(t, writeJSON(filepath.Join(home, "projects.json"), serverConfig()))
	must(t, os.WriteFile(filepath.Join(home, "settings.json"), []byte(`{"poll_seconds":0}`), 0o644))
	d, err := NewDaemon(home)
	must(t, err)
	assertEq(t, d.S.PollSeconds, 5)
	assertEq(t, d.tracker.PollMs, int64(5000))
	var e struct {
		Error string `json:"error"`
	}
	assertEq(t, serve(t, d).do("GET", "/v1/timeline", nil, &e), http.StatusInternalServerError)
	assertEq(t, strings.HasPrefix(e.Error, "config: "), true)
}

func TestOpenSpansStopAtTheLastPollAfterSleep(t *testing.T) {
	d := testDaemon(t)
	now := at(0)
	swap(t, &nowMs, func() int64 { return now })
	swap(t, &takeSample, func(ms int64) (Sample, error) { return Sample{At: ms, LastInput: ms, Exe: "idea64.exe"}, nil })
	d.agentsSeen(working(`C:\Dev\easy`))
	d.poll()
	now += 5000
	d.poll()
	now += 10_000 // within three polls: still awake
	for _, sp := range d.openSpans(now) {
		assertEq(t, sp.End, now)
	}
	now = at(30) // asleep since the last poll, nothing polled yet
	open := d.openSpans(now)
	assertEq(t, len(open) > 1, true)
	for _, sp := range open {
		assertEq(t, sp.End, at(0)+5000)
	}
	must(t, d.saveOpen())
	sp, err := d.Store.RecoverOpen()
	must(t, err)
	assertEq(t, sp[0].End, at(0)+5000)
}
