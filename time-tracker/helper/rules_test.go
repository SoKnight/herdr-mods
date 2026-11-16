package main

import (
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
	id, _ := r.ProjectByApp("Telemost.exe")
	assertEq(t, id, "study")
}

func TestDayOfRespectsDayStart(t *testing.T) {
	s := DefaultSettings()
	s.Loc = time.FixedZone("MSK", 3*3600)
	at := time.Date(2026, 11, 17, 3, 59, 0, 0, s.Loc).UnixMilli()
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
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"idleMinutes": 10}`), 0o644)
	s, err = LoadSettings(dir)
	if err != nil {
		t.Fatal(err)
	}
	assertEq(t, s.IdleMinutes, 10)
	assertEq(t, s.CarryMinutes, 15)
}

func TestCompileRejectsBadRegex(t *testing.T) {
	_, err := Config{Projects: []Project{{ID: "broken", Titles: []string{"("}}}}.Compile()
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Errorf("err = %v", err)
	}
}
