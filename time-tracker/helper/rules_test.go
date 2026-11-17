package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustCompile(t *testing.T, c Config) *Rules {
	t.Helper()
	r, err := c.Compile()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func assertEq[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestProjectByPathLongestPrefixIgnoringCase(t *testing.T) {
	r := mustCompile(t, Config{Projects: []Project{
		{ID: "universe", Paths: []string{`C:\Dev\starmc`}},
		{ID: "desk", Paths: []string{`C:\Dev\starmc\universe-desk`}},
	}})
	id, why := r.ProjectByPath(`c:/dev/StarMC/universe-desk/src`)
	assertEq(t, id, "desk")
	assertEq(t, why, `папка C:\Dev\starmc\universe-desk → desk`)
	id, _ = r.ProjectByPath(`C:\Dev\starmc\universe-paper`)
	assertEq(t, id, "universe")
	id, _ = r.ProjectByPath(`C:\Dev\starmcx`)
	assertEq(t, id, "")
}

func TestFigmaRuleOnlyForFigma(t *testing.T) {
	r := mustCompile(t, DefaultConfig())
	id, _ := r.ProjectByTitle("Figma.exe", "EasyLauncher UI – Figma")
	assertEq(t, id, "easylauncher")
	id, _ = r.ProjectByTitle("chrome.exe", "EasyLauncher UI – Figma")
	assertEq(t, id, "")
}

func TestActivityByFile(t *testing.T) {
	r := mustCompile(t, DefaultConfig())
	assertEq(t, r.ActivityByFile("README.md"), "documentation")
	assertEq(t, r.ActivityByFile("a.mdx"), "documentation")
	assertEq(t, r.ActivityByFile("main.go"), "")
}

func TestAppLookups(t *testing.T) {
	r := mustCompile(t, DefaultConfig())
	if a := r.App("IDEA64.EXE"); a == nil || a.Activity != "development" {
		t.Errorf("App(IDEA64.EXE) = %v", a)
	}
	assertEq(t, r.App("nope.exe") == nil, true)
	id, _ := r.ProjectByApp("yandextelemost.exe")
	assertEq(t, id, "study")
}

func TestDayOfRespectsDayStart(t *testing.T) {
	s := DefaultSettings()
	s.Loc = time.FixedZone("MSK", 3*3600)
	at := time.Date(2026, 11, 17, 5, 59, 0, 0, s.Loc).UnixMilli()
	assertEq(t, s.DayOf(at), "2026-11-16")
	assertEq(t, s.DayOf(at+60_000), "2026-11-17")
}

func TestLoadConfigWritesDefaultWhenMissing(t *testing.T) {
	dir := t.TempDir()
	c, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	assertEq(t, c.Projects[0].ID, "universe")
	if _, err := os.Stat(filepath.Join(dir, "projects.json")); err != nil {
		t.Error(err)
	}
	again, err := LoadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	assertEq(t, len(again.Projects), len(c.Projects))
}

func TestLoadSettingsOverlaysDefaults(t *testing.T) {
	dir := t.TempDir()
	s, err := LoadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	assertEq(t, s.IdleMinutes, 5)
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"idle_minutes": 10}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err = LoadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	assertEq(t, s.IdleMinutes, 10)
	assertEq(t, s.CarryMinutes, 15)
}

func TestDefaultSettings(t *testing.T) {
	s := DefaultSettings()
	assertEq(t, s.IdleMinutes, 5)
	assertEq(t, s.CarryMinutes, 15)
	assertEq(t, s.Port, 47613)
	assertEq(t, s.PollSeconds, 5)
	assertEq(t, s.DayStart, "06:00")
	assertEq(t, s.SessionIdleMinutes, 45)
	assertEq(t, s.PauseMinutes, 15)
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	assertEq(t, string(data), `{"idle_minutes":5,"carry_minutes":15,"port":47613,"poll_seconds":5,"day_start":"06:00","session_idle_minutes":45,"pause_minutes":15}`)
	var back Settings
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	back.Loc = s.Loc
	assertEq(t, back, s)
}

func TestDefaultActivities(t *testing.T) {
	assertEq(t, strings.Join(DefaultConfig().Activities, ","), "development,architecture,design,documentation,study")
}

func TestLoadSettingsRejectsBadValues(t *testing.T) {
	for _, body := range []string{
		`{"idle_minutes": 0}`, `{"port": 0}`, `{"poll_seconds": 0}`, `{"carry_minutes": -1}`, `{"day_start": "9"}`,
		`{"session_idle_minutes": 0}`, `{"pause_minutes": 0}`,
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadSettings(dir); err == nil {
			t.Errorf("%s: want error", body)
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"carry_minutes": 0}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSettings(dir); err != nil {
		t.Errorf("carry_minutes 0 should be allowed: %v", err)
	}
}

func TestCompileRejectsBadRegex(t *testing.T) {
	_, err := Config{Projects: []Project{{ID: "broken", Titles: []string{"("}}}}.Compile()
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Errorf("err = %v", err)
	}
}
