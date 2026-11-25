package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestReadClaudeDesktopSessions(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("101.json", `{"pid":101,"cwd":"P:\\starmc\\star-core","entrypoint":"claude-desktop","status":"working","statusUpdatedAt":1000}`)
	write("102.json", `{"pid":102,"cwd":"P:\\easyx\\app","entrypoint":"claude-desktop","status":"waiting","updatedAt":2000}`)
	write("103.json", `{"pid":103,"cwd":"P:\\starmc\\star-core","entrypoint":"cli","status":"working","statusUpdatedAt":3000}`)
	write("104.json", `{"pid":104,"cwd":"P:\\starmc\\star-core","entrypoint":"claude-desktop","status":"working","statusUpdatedAt":4000}`)
	write("105.json", `{"pid":105,"cwd":"P:\\claude-import","entrypoint":"claude-desktop","status":"idle","statusUpdatedAt":5000}`)
	write("101.abc.key", `{"peerToken":"x"}`)

	alive := func(pid int) bool { return pid != 104 }
	got := readClaudeDesktopSessions(dir, alive, 9000)

	if len(got) != 3 {
		t.Fatalf("got %d sessions (%v), want 3 (101, 102, 105)", len(got), got)
	}
	if s := got["claude-desktop/101"]; !s.Working || s.Waiting || s.Cwd != `P:\starmc\star-core` || s.Since != 1000 {
		t.Fatalf("session 101 = %+v", s)
	}
	if s := got["claude-desktop/102"]; s.Working || !s.Waiting || s.Cwd != `P:\easyx\app` || s.Since != 2000 {
		t.Fatalf("session 102 = %+v", s)
	}
	if s := got["claude-desktop/105"]; s.Working || s.Waiting || s.Cwd != `P:\claude-import` {
		t.Fatalf("session 105 = %+v", s)
	}
}

func TestGuestMatchesWindow(t *testing.T) {
	g := GuestConfig{Name: "tricky-fox", URL: "http://10.10.20.10:47614", MatchTitle: "10.10.20.10"}
	if !guestMatchesWindow(g, "mstsc.exe", "Tricky Device - 10.10.20.10:3389 - Remote Desktop Connection") {
		t.Fatal("expected mstsc.exe with matching title to match")
	}
	if guestMatchesWindow(g, "mstsc.exe", "Other Box - 10.0.0.2 - Remote Desktop Connection") {
		t.Fatal("expected non-matching title not to match")
	}
	if guestMatchesWindow(g, "firefox.exe", "10.10.20.10") {
		t.Fatal("expected non-mstsc exe not to match")
	}
}

func TestGuestServerAndDaemonPoll(t *testing.T) {
	home := t.TempDir()
	d, err := NewDaemon(home)
	if err != nil {
		t.Fatal(err)
	}
	d.S.Guests = []GuestConfig{{Name: "tricky-fox", URL: "http://guest.test", MatchTitle: "10.10.20.10"}}

	origNow, origSample, origFetch := nowMs, takeSample, fetchGuest
	t.Cleanup(func() { nowMs, takeSample, fetchGuest = origNow, origSample, origFetch })

	t0 := int64(1_700_000_000_000)
	nowMs = func() int64 { return t0 }
	takeSample = func(now int64) (Sample, error) {
		return Sample{At: now, LastInput: now, Exe: "mstsc.exe", Title: "Tricky Device - 10.10.20.10:3389 - Remote Desktop Connection"}, nil
	}
	fetchGuest = func(ctx context.Context, url string) (GuestSnapshot, error) {
		return GuestSnapshot{
			Exe:   "Claude.exe",
			Title: "Star Core - Claude",
			Agents: map[string]AgentState{
				"claude-desktop/5324": {Cwd: `P:\starmc\star-core`, Working: true, Since: t0},
			},
		}, nil
	}

	d.poll()

	d.mu.Lock()
	defer d.mu.Unlock()
	w := d.tracker.open["window"]
	if w == nil || w.Exe != "Claude.exe" || w.Title != "Star Core - Claude" {
		t.Fatalf("window span = %+v, want Claude.exe / Star Core - Claude", w)
	}
	ag := d.agents.open["tricky-fox/claude-desktop/5324"]
	if ag == nil || ag.Cwd != `P:\starmc\star-core` {
		t.Fatalf("agent span = %+v, want tricky-fox/claude-desktop/5324", ag)
	}
}

func TestGuestServerGuard(t *testing.T) {
	srv := httptest.NewServer(newGuestServer(func() {}))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/guest")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("without header: status %d, want 403", resp.StatusCode)
	}

	origSample, origDir := takeSample, claudeSessionDir
	t.Cleanup(func() { takeSample, claudeSessionDir = origSample, origDir })
	takeSample = func(now int64) (Sample, error) { return Sample{At: now, Exe: "Claude.exe", Title: "Claude"}, nil }
	claudeSessionDir = func() string { return t.TempDir() }

	req, _ := http.NewRequest("GET", srv.URL+"/v1/guest", nil)
	req.Header.Set("X-Herdr-Time", "1")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("with header: status %d, want 200", resp.StatusCode)
	}
	var snap GuestSnapshot
	if err := json.NewDecoder(resp.Body).Decode(&snap); err != nil {
		t.Fatal(err)
	}
	if snap.Exe != "Claude.exe" || snap.Title != "Claude" {
		t.Fatalf("snap = %+v", snap)
	}
}
