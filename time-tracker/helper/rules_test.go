package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// The live projects.json of 17.11.2026 as v1 wrote it, plus "desk" to pin the longest-folder order.
const v1Projects = `{
  "activities": ["development", "architecture", "design", "documentation", "study"],
  "projects": [
    {"id": "universe", "name": "Universe", "client": "StarMC", "paths": ["C:\\Users\\SoKnight\\Development\\starmc"], "titles": ["(?i)universe"]},
    {"id": "easylauncher", "name": "EasyLauncher", "paths": ["C:\\Users\\SoKnight\\Development\\easyx"], "figma": ["(?i)easylauncher"]},
    {"id": "tricky", "name": "Tricky Network", "paths": ["C:\\Users\\SoKnight\\Development\\tricky-network"]},
    {"id": "study", "name": "Учёба", "paths": ["C:\\Users\\SoKnight\\Development\\university"], "apps": ["YandexTelemost.exe"]},
    {"id": "desk", "name": "Desk", "paths": ["C:\\Users\\SoKnight\\Development\\starmc\\universe-desk"]}
  ],
  "apps": [
    {"exe": "idea64.exe", "activity": "development", "source": "idea"},
    {"exe": "herdr-app.exe", "activity": "development", "source": "herdr"},
    {"exe": "firefox.exe", "source": "firefox"},
    {"exe": "Figma.exe", "activity": "design"},
    {"exe": "YandexTelemost.exe", "activity": "study", "presence": "call"}
  ],
  "files": [{"match": "\\.mdx?$", "activity": "documentation"}]
}`

func TestMigrationKeepsAttribution(t *testing.T) {
	dir := t.TempDir()
	must(t, os.WriteFile(filepath.Join(dir, "projects.json"), []byte(v1Projects), 0o644))
	c, err := LoadConfig(dir)
	must(t, err)
	rules := mustCompile(t, c)
	const dev = `C:\Users\SoKnight\Development\`
	cases := []struct {
		exe, title, source, path string // source, path: a signal at the window's start
		call                     bool   // Telemost holds the microphone
		want                     string
	}{
		{"herdr-app.exe", "pwsh", "herdr", dev + `starmc\universe-platform\.claude\worktrees\referrals`, false, "universe"},
		{"herdr-app.exe", "pwsh", "herdr", dev + `starmc\universe-desk\src`, false, "desk"},
		{"herdr-app.exe", "pwsh", "herdr", dev + `tricky-network`, false, "tricky"},
		{"herdr-app.exe", "pwsh", "herdr", dev + `easyx\EasyLauncher`, false, "easylauncher"},
		{"herdr-app.exe", "pwsh", "herdr", dev + `university\labs`, false, "study"},
		{"herdr-app.exe", "pwsh", "herdr", dev + `opensource\herdr-mods`, false, ""},
		{"herdr-app.exe", "Universe notes", "herdr", dev + `tricky-network`, false, "tricky"},
		{"herdr-app.exe", "Universe notes", "herdr", dev + `opensource\herdr-mods`, false, "universe"},
		{"idea64.exe", "x", "herdr", dev + `tricky-network`, false, ""},
		{"firefox.exe", "Universe Desk screens - Claude — Mozilla Firefox", "", "", false, "universe"},
		{"Figma.exe", "EasyLauncher UI – Figma", "", "", false, "easylauncher"},
		{"Figma.exe", "Universe — App", "", "", false, "universe"},
		{"chrome.exe", "EasyLauncher UI", "", "", false, ""},
		{"Telegram.exe", "UNIVERSE chat", "", "", false, "universe"},
		{"YandexTelemost.exe", "Звонок", "", "", false, "study"},
		{"Telegram.exe", "Избранное", "", "", true, "study"},
		{"javaw.exe", "Minecraft 26.3 - Singleplayer", "", "", false, ""},
	}
	n := 2 * len(cases)
	raw := spans(power(-1, n), inp(-1, n))
	for i, c := range cases {
		raw.Spans = append(raw.Spans, win(c.exe, c.title, 2*i, 2*i+1))
		if c.source != "" {
			raw.Beats = append(raw.Beats, Heartbeat{Source: c.source, Time: at(2 * i), ProjectPath: c.path})
		}
		if c.call {
			raw.Spans = append(raw.Spans, Span{Kind: "call", Start: at(2 * i), End: at(2*i + 1), Exe: "YandexTelemost.exe"})
		}
	}
	for i, cwd := range []string{dev + `starmc\universe-platform`, dev + `starmc\universe-desk`, dev + `opensource\herdr-mods`, dev + `university`} {
		raw.Spans = append(raw.Spans, agent(fmt.Sprintf("t%d", i), cwd, 2*i, 2*i+1))
	}
	s := DefaultSettings()
	s.Loc, s.CarryMinutes = msk, 0 // rules alone; carry is unchanged code
	tl := Attribute(raw, rules, nil, s, at(-1), at(n))
	for i, c := range cases {
		if got := segAt(t, tl, 2*i, 30).Project; got != c.want {
			t.Errorf("case %d (%s %q): got %q, want %q", i, c.exe, c.title, got, c.want)
		}
	}
	assertAgents(t, tl, "universe", m(0, 1))
	assertAgents(t, tl, "desk", m(2, 3))
	assertAgents(t, tl, "", m(4, 5))
	assertAgents(t, tl, "study", m(6, 7))
}

func mustCompile(t *testing.T, c Config) *Rules {
	t.Helper()
	c, _, err := migrate(c)
	if err != nil {
		t.Fatal(err)
	}
	r, err := c.Compile()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestRuleMatches(t *testing.T) {
	w := window{exe: "IDEA64.exe", title: "Universe-backend – Main.kt", path: `c:/dev/StarMC/universe-desk/src`, at: at(10)}
	for i, r := range []Rule{
		{Kind: "folder", Value: `C:\Dev\starmc`},
		{Kind: "folder", Value: `C:\Dev\starmc\universe-desk\`},
		{Kind: "process", Value: "idea64.exe"},
		{Kind: "process", Value: "idea64.exe", Title: "universe-"},
		{Kind: "title", Value: "main.kt"},
		{Kind: "title", Value: "main.kt", From: at(10), To: at(11)},
	} {
		assertEq(t, fmt.Sprint(i, r.matches(w)), fmt.Sprint(i, true))
	}
	for i, r := range []Rule{
		{Kind: "folder", Value: `C:\Dev\starmc\universe`}, // a prefix of the name, not a parent folder
		{Kind: "process", Value: "idea64.exe", Title: "tricky"},
		{Kind: "figma", Value: "universe"}, // not Figma.exe
		{Kind: "minecraft", Value: "localhost:25565"},
		{Kind: "title", Value: "main.kt", From: at(11)},
		{Kind: "title", Value: "main.kt", To: at(10)},
	} {
		assertEq(t, fmt.Sprint(i, r.matches(w)), fmt.Sprint(i, false))
	}
	assertEq(t, Rule{Kind: "figma", Value: "UNIVERSE"}.matches(window{exe: "Figma.exe", title: "Universe — App"}), true)
	assertEq(t, Rule{Kind: "minecraft", Value: "Play.Example.net:25565"}.matches(window{exe: "javaw.exe", server: "play.example.net:25565"}), true)
}

func TestFirstMatchingRuleWins(t *testing.T) {
	r := mustCompile(t, Config{Projects: []Project{{ID: "a"}, {ID: "b"}}, Rules: []Rule{
		{ID: 1, Kind: "title", Value: "spec", Project: "a"},
		{ID: 2, Kind: "folder", Value: `C:\Dev\b`, Project: "b"},
	}})
	got, ok := r.Match(window{title: "Spec", path: `C:\Dev\b`})
	assertEq(t, ok, true)
	assertEq(t, got.ID, int64(1))
	id, _ := r.FolderProject(`C:\Dev\b\x`, 0)
	assertEq(t, id, "b")
	_, err := Config{Rules: []Rule{{ID: 1, Kind: "title", Value: "x", Project: "nope"}}}.Compile()
	assertEq(t, err != nil, true)
}

func TestMigrationOrdersLikeV1(t *testing.T) {
	c, changed, err := migrate(Config{Projects: []Project{
		{ID: "u", Paths: []string{`C:\Dev\starmc`}, Titles: []string{"(?i)universe"}},
		{ID: "e", Paths: []string{`C:\Dev\easyx`}, Figma: []string{"(?i)easylauncher"}},
		{ID: "d", Paths: []string{`C:\Dev\starmc\desk`}},
		{ID: "s", Apps: []string{"YandexTelemost.exe"}, Titles: []string{"Lecture"}},
	}})
	must(t, err)
	assertEq(t, changed, true)
	want := []Rule{
		{ID: 1, Kind: "folder", Value: `C:\Dev\starmc\desk`, Project: "d"},
		{ID: 2, Kind: "folder", Value: `C:\Dev\starmc`, Project: "u"},
		{ID: 3, Kind: "folder", Value: `C:\Dev\easyx`, Project: "e"},
		{ID: 4, Kind: "process", Value: "YandexTelemost.exe", Project: "s"},
		{ID: 5, Kind: "title", Value: "universe", Project: "u"},
		{ID: 6, Kind: "title", Value: "Lecture", Project: "s"},
		{ID: 7, Kind: "figma", Value: "easylauncher", Project: "e"},
	}
	if !reflect.DeepEqual(c.Rules, want) {
		t.Fatalf("rules:\n%+v\nwant\n%+v", c.Rules, want)
	}
	assertEq(t, c.Projects[2].Color, "ansi3")
	assertEq(t, c.Projects[0].Paths == nil && c.Projects[0].Titles == nil, true)
	_, changed, err = migrate(c)
	must(t, err)
	assertEq(t, changed, false)
}

func TestMigrationRefusesWhatItCannotKeep(t *testing.T) {
	for _, p := range []Project{{ID: "x", URLs: []string{"^https://a"}}, {ID: "x", Titles: []string{"^Main$"}}, {ID: "x", Figma: []string{"("}}} {
		_, _, err := migrate(Config{Projects: []Project{p}})
		assertEq(t, err != nil && strings.Contains(err.Error(), "project x"), true)
	}
}

func TestLoadConfigMigratesOnceWithBackup(t *testing.T) {
	dir := t.TempDir()
	file, backup := filepath.Join(dir, "projects.json"), filepath.Join(dir, "projects.v1.json")
	must(t, os.WriteFile(file, []byte(v1Projects), 0o644))
	c, err := LoadConfig(dir)
	must(t, err)
	saved, err := os.ReadFile(backup)
	must(t, err)
	assertEq(t, string(saved), v1Projects)
	data, err := os.ReadFile(file)
	must(t, err)
	assertEq(t, strings.Contains(string(data), `"paths"`), false)
	assertEq(t, strings.Contains(string(data), `"rules"`), true)

	c.Projects[2].Paths = []string{`C:\Dev\more`} // the old form written again by hand
	must(t, writeJSON(file, c))
	again, err := readConfig(dir)
	must(t, err)
	assertEq(t, slices.ContainsFunc(again.Rules, func(r Rule) bool { return r.Kind == "folder" && r.Value == `C:\Dev\more` && r.Project == "tricky" }), true)
	saved, err = os.ReadFile(backup)
	must(t, err)
	assertEq(t, string(saved), v1Projects) // the first backup stays
}

func TestMigrationErrorWritesNothing(t *testing.T) {
	dir := t.TempDir()
	bad := strings.Replace(v1Projects, `"(?i)universe"`, `"^Universe$"`, 1)
	must(t, os.WriteFile(filepath.Join(dir, "projects.json"), []byte(bad), 0o644))
	_, err := LoadConfig(dir)
	assertEq(t, err != nil, true)
	data, err := os.ReadFile(filepath.Join(dir, "projects.json"))
	must(t, err)
	assertEq(t, string(data), bad)
	_, err = os.Stat(filepath.Join(dir, "projects.v1.json"))
	assertEq(t, os.IsNotExist(err), true)
}

func assertEq[T comparable](t *testing.T, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestFolderProjectLongestFirst(t *testing.T) {
	r := mustCompile(t, Config{Projects: []Project{
		{ID: "universe", Paths: []string{`C:\Dev\starmc`}},
		{ID: "desk", Paths: []string{`C:\Dev\starmc\universe-desk`}},
	}})
	id, why := r.FolderProject(`c:/dev/StarMC/universe-desk/src`, 0)
	assertEq(t, id, "desk")
	assertEq(t, why, `папка C:\Dev\starmc\universe-desk → desk`)
	id, _ = r.FolderProject(`C:\Dev\starmc\universe-paper`, 0)
	assertEq(t, id, "universe")
	id, _ = r.FolderProject(`C:\Dev\starmcx`, 0)
	assertEq(t, id, "")
}

func TestFigmaRuleOnlyForFigma(t *testing.T) {
	r := mustCompile(t, DefaultConfig())
	got, _ := r.Match(window{exe: "Figma.exe", title: "EasyLauncher UI – Figma"})
	assertEq(t, got.Project, "easylauncher")
	_, ok := r.Match(window{exe: "chrome.exe", title: "EasyLauncher UI – Figma"})
	assertEq(t, ok, false)
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
	got, _ := r.Match(window{exe: "yandextelemost.exe"})
	assertEq(t, got.Project, "study")
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
	assertEq(t, len(c.Rules), 6)
	assertEq(t, c.Projects[0].Color, "ansi1")
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
	_, _, err := migrate(Config{Projects: []Project{{ID: "broken", Titles: []string{"("}}}})
	if err == nil || !strings.Contains(err.Error(), "broken") {
		t.Errorf("err = %v", err)
	}
}
