package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// fakeTools stands in for Go and herdr-app, recording what ran.
func fakeTools(t *testing.T) (*bytes.Buffer, *[][]string) {
	t.Helper()
	printed, calls := &bytes.Buffer{}, &[][]string{}
	oldRun, oldLook, oldOut := run, lookPath, out
	t.Cleanup(func() { run, lookPath, out = oldRun, oldLook, oldOut })
	out = printed
	lookPath = func(string) (string, error) { return "available", nil }
	run = func(_ string, _ []string, args ...string) error {
		*calls = append(*calls, args)
		if args[0] == "go" {
			return os.WriteFile(args[3], []byte("built helper"), 0o755)
		}
		return nil
	}
	return printed, calls
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestInstallsAndUpdatesWithoutTouchingAnythingElse(t *testing.T) {
	printed, calls := fakeTools(t)
	app := filepath.Join(t.TempDir(), "app")
	write(t, filepath.Join(app, "layouts/personal.json"), `{"personal":true}`)
	write(t, filepath.Join(app, "keys.json"), `{"global":{"cmd-p":"custom"}}`)
	write(t, filepath.Join(app, "mods/independent/main.js"), "// personal")
	if err := install(app, false); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(app, "mods", mod)
	if got := read(t, filepath.Join(target, "bin", executable())); got != "built helper" {
		t.Fatalf("binary = %q", got)
	}
	if _, err := os.Stat(filepath.Join(target, "main.js")); err != nil {
		t.Fatal(err)
	}
	if last := (*calls)[len(*calls)-1]; last[0] != "herdr-app" || last[1] != "check" {
		t.Fatalf("last ran %v", last)
	}
	if err := install(app, false); err == nil {
		t.Fatal("a second install needs --update")
	}
	if err := install(app, true); err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`Installed .* \d+\.\d+\.\d+ \(was \d+\.\d+\.\d+\)`).MatchString(printed.String()) {
		t.Fatalf("printed %q", printed)
	}
	if read(t, filepath.Join(app, "layouts/personal.json")) != `{"personal":true}` ||
		read(t, filepath.Join(app, "keys.json")) != `{"global":{"cmd-p":"custom"}}` ||
		read(t, filepath.Join(app, "mods/independent/main.js")) != "// personal" {
		t.Fatal("something else changed")
	}
}

func TestUpdateKeepsEditedSettingsAndReportsStaleKeys(t *testing.T) {
	printed, _ := fakeTools(t)
	app := t.TempDir()
	target := filepath.Join(app, "mods", mod)
	shipped, err := readManifest(filepath.Join(root(), "mods", mod))
	if err != nil {
		t.Fatal(err)
	}
	first := ""
	for key := range knownSettings(filepath.Join(root(), "mods", mod), nil) {
		first = key
		break
	}
	if first == "" {
		t.Fatal("mod.schema.json describes no settings")
	}
	edited := map[string]any{"title": title, "version": "0.0.1", "settings": map[string]any{first: "edited", "gone": true}}
	b, _ := json.Marshal(edited)
	write(t, filepath.Join(target, "mod.json"), string(b))
	if err := install(app, true); err != nil {
		t.Fatal(err)
	}
	got, err := readManifest(target)
	if err != nil {
		t.Fatal(err)
	}
	kept := got["settings"].(map[string]any)
	if got["version"] != shipped["version"] || kept[first] != "edited" || kept["gone"] != nil || len(kept) != 1 {
		t.Fatalf("manifest = %v", got)
	}
	if !strings.Contains(printed.String(), "dropped: gone") {
		t.Fatalf("printed %q", printed)
	}
}

func TestNestedSettingsMergeKeepOnlyChangesAndNameWhatIsDropped(t *testing.T) {
	known := map[string]any{"a": map[string]any{"b": 1.0, "c": 2.0}}
	settings := map[string]any{}
	dropped := merge(settings, known, map[string]any{"a": map[string]any{"b": 9.0, "c": 2.0, "old": true}}, "")
	inner := settings["a"].(map[string]any)
	if inner["b"] != 9.0 || len(inner) != 1 || len(dropped) != 1 || dropped[0] != "a.old" {
		t.Fatalf("settings %v, dropped %v", settings, dropped)
	}
}

func TestUnrelatedFoldersAndSymlinksAreNeverOverwritten(t *testing.T) {
	fakeTools(t)
	app := t.TempDir()
	target := filepath.Join(app, "mods", mod)
	write(t, filepath.Join(target, "mod.json"), `{"title":"Personal"}`)
	if err := install(app, true); err == nil {
		t.Fatal("overwrote someone else's folder")
	}
	if got := read(t, filepath.Join(target, "mod.json")); got != `{"title":"Personal"}` {
		t.Fatalf("mod.json = %q", got)
	}
	if err := os.RemoveAll(target); err != nil {
		t.Fatal(err)
	}
	if err := link(app, target); err != nil {
		t.Skip("can't make a link here:", err)
	}
	if err := install(app, true); err == nil {
		t.Fatal("overwrote a symlink")
	}
}

func TestMissingToolsFailBeforeCopying(t *testing.T) {
	fakeTools(t)
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	app := t.TempDir()
	if err := install(app, false); err == nil {
		t.Fatal("installed without its tools")
	}
	if _, err := os.Stat(filepath.Join(app, "mods", mod)); err == nil {
		t.Fatal("copied before checking")
	}
}

// link points target at dir. A Windows symlink needs Developer Mode or an administrator,
// so there it's a junction, which anyone can make and which points elsewhere just the same.
func link(dir, target string) error {
	if runtime.GOOS != "windows" {
		return os.Symlink(dir, target)
	}
	return exec.Command("cmd", "/c", "mklink", "/J", target, dir).Run()
}

func TestReplacesAHelperThatIsRunning(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only Windows refuses to overwrite a running program")
	}
	ping, err := exec.LookPath("ping")
	if err != nil {
		t.Skip(err)
	}
	dir := t.TempDir()
	target := filepath.Join(dir, executable())
	if err := copyFile(ping, target); err != nil {
		t.Fatal(err)
	}
	running := exec.Command(target, "-n", "30", "127.0.0.1")
	if err := running.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { running.Process.Kill(); running.Wait() })
	fresh := filepath.Join(dir, "."+executable()+".new")
	write(t, fresh, "new helper")
	if err := replace(fresh, target); err != nil {
		t.Fatal(err)
	}
	if got := read(t, target); got != "new helper" {
		t.Fatalf("helper = %q", got)
	}
}
