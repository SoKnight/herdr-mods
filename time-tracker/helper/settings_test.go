package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestSettingsDefaults(t *testing.T) {
	s := DefaultSettings()
	assertEq(t, s.SessionIdleMinutes, 45)
	assertEq(t, s.PauseMinutes, 15)
	assertEq(t, s.DayStart, "06:00")
	assertEq(t, s.VideoActivity && s.Autostart && s.SilentWarning && s.CloseReminder, true)
	assertEq(t, s.HiddenTitles != nil && len(s.HiddenTitles) == 0, true)
}

func settingsFile(d *Daemon) string { return filepath.Join(d.Home, "settings.json") }

func TestPutSettingsMerges(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	must(t, os.WriteFile(settingsFile(d), []byte(`{"port":47613,"minecraft_logs":["x"],"pause_minutes":20}`), 0o644))
	var got editableSettings
	assertEq(t, api.do("PUT", "/v1/settings", `{"session_idle_minutes":30,"hidden_titles":["KeePassXC.exe","*банк*"]}`, &got), http.StatusOK)
	assertEq(t, got.SessionIdleMinutes, 30)
	assertEq(t, got.PauseMinutes, 20)
	assertEq(t, slices.Equal(got.HiddenTitles, []string{"KeePassXC.exe", "*банк*"}), true)
	b, err := os.ReadFile(settingsFile(d))
	must(t, err)
	for _, want := range []string{`"port": 47613`, `"minecraft_logs"`, `"pause_minutes": 20`} {
		assertEq(t, strings.Contains(string(b), want), true)
	}
	assertEq(t, d.S.SessionIdleMinutes, 30) // no wait for the mtime
	var again editableSettings
	assertEq(t, api.do("GET", "/v1/settings", nil, &again), http.StatusOK)
	assertEq(t, again.SessionIdleMinutes, 30)
}

func TestPutSettingsRejects(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	orig := `{"port":47613,"pause_minutes":20}`
	must(t, os.WriteFile(settingsFile(d), []byte(orig), 0o644))
	for _, body := range []string{
		`{"day_start":"25:00"}`, `{"pause_minutes":0}`, `{"session_idle_minutes":-1}`,
		`{"hidden_titles":["["]}`, `{"hidden_titles":[""]}`, `{"port":1}`, `{"bogus":1}`,
	} {
		assertEq(t, fmt.Sprint(body, " ", api.do("PUT", "/v1/settings", body, nil)), body+" 400")
		b, err := os.ReadFile(settingsFile(d))
		must(t, err)
		assertEq(t, string(b), orig)
	}
}

func TestPutAutostart(t *testing.T) {
	d := testDaemon(t)
	api := serve(t, d)
	var calls []bool
	swap(t, &setRun, func(on bool) error { calls = append(calls, on); return nil })
	assertEq(t, api.do("PUT", "/v1/settings", `{"autostart":false}`, nil), http.StatusOK)
	assertEq(t, slices.Equal(calls, []bool{false}), true)
	assertEq(t, api.do("PUT", "/v1/settings", `{"session_idle_minutes":40}`, nil), http.StatusOK)
	assertEq(t, len(calls), 1)
}

func TestDayStartChangeKeepsClosedMonth(t *testing.T) {
	d, api := november(t)
	seedWindow(t, d, mskAt(2026, 11, 17, 5, 0), "notepad.exe", "Easy late", 60)
	report := func() monthReport {
		var m monthReport
		assertEq(t, api.do("GET", "/v1/months/2026-11/report?project=easy&rows=day", nil, &m), http.StatusOK)
		return m
	}
	assertEq(t, api.do("POST", "/v1/months/2026-11/close", nil, nil), http.StatusOK)
	before := report()
	assertEq(t, api.do("PUT", "/v1/settings", `{"day_start":"04:00"}`, nil), http.StatusOK)
	after := report()
	assertEq(t, after.Total.Me, before.Total.Me)
	labels := func(m monthReport) string {
		var l []string
		for _, r := range m.Rows {
			l = append(l, r.Label)
		}
		return strings.Join(l, "|")
	}
	assertEq(t, labels(after), labels(before))
}
