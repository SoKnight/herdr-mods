package main

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
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
		name       string
		boot       int
		power, gap Interval
	}{
		{"killed", -120, m(-60, 30), m(0, 30)}, // booted before the last poll: the gap starts at the last poll
		{"rebooted", 10, m(-60, 0), m(10, 30)}, // booted after it: the gap starts at boot
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := testDaemon(t)
			must(t, d.Store.SaveOpen([]Span{{Kind: "power", Start: at(-60)}, {Kind: "window", Start: at(-10), Exe: "idea64.exe"}}, at(0)))
			swap(t, &bootAt, func(int64) int64 { return at(tc.boot) })
			must(t, d.recoverOpen())
			raw := mustLoad(t, d.Store, at(-180), at(60))
			assertEq(t, len(kindIntervals(raw, "nodata")), 1)
			assertEq(t, kindIntervals(raw, "nodata")[0], tc.gap)
			assertEq(t, kindIntervals(raw, "power")[0].Start, at(-60))
			assertEq(t, kindIntervals(raw, "power")[len(kindIntervals(raw, "power"))-1].End, at(30))
			assertEq(t, kindIntervals(raw, "window")[0], m(-10, 0))
			if tc.name == "rebooted" {
				assertEq(t, kindIntervals(raw, "power")[0], tc.power)
			}
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
		Activity string `json:"activity"`
	}
	assertEq(t, api.do("GET", "/v1/status", nil, &st), http.StatusOK)
	assertEq(t, st.Version, version)
	assertEq(t, st.Me, int64(minMs))
	assertEq(t, st.Agents, int64(minMs))
	assertEq(t, st.Project, "Easy")
	assertEq(t, st.Activity, "development")
}

func TestQuitStopsRun(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	swap(t, &bootAt, func(int64) int64 { return 0 })
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
