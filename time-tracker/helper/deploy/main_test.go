package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakes struct {
	calls   [][]string
	runKey  string
	started string
	quit    []int
	order   []string
}

// fakeTools stands in for Go, the registry, the quit call and the launch.
func fakeTools(t *testing.T) *fakes {
	t.Helper()
	f := &fakes{}
	oldRun, oldLook, oldOut, oldSet, oldStart, oldQuit, oldBuild := run, lookPath, out, setAutostart, startDetached, quit, buildPath
	t.Cleanup(func() {
		run, lookPath, out, setAutostart, startDetached, quit, buildPath = oldRun, oldLook, oldOut, oldSet, oldStart, oldQuit, oldBuild
	})
	built := filepath.Join(t.TempDir(), "bin", executable()) // never the repo's own bin/
	buildPath = func() string { return built }
	out = &strings.Builder{}
	lookPath = func(string) (string, error) { return "available", nil }
	run = func(_ string, args ...string) error {
		f.calls = append(f.calls, args)
		f.order = append(f.order, "build")
		return os.WriteFile(args[len(args)-2], []byte("built"), 0o755) // ... -o <path> .
	}
	setAutostart = func(v string) error { f.runKey = v; return nil }
	startDetached = func(exe string) error { f.started = exe; f.order = append(f.order, "start"); return nil }
	quit = func(p int) error { f.quit = append(f.quit, p); f.order = append(f.order, "quit"); return nil }
	return f
}

func TestInstallBuildsAndCopiesItToAppHome(t *testing.T) {
	f := fakeTools(t)
	t.Setenv("HERDR_TIME_HOME", t.TempDir())
	app := filepath.Join(t.TempDir(), "app home")
	built := buildPath()
	if err := install(app); err != nil {
		t.Fatal(err)
	}
	call := strings.Join(f.calls[0], " ")
	if !strings.Contains(call, "-H=windowsgui") || !strings.Contains(call, built) {
		t.Fatalf("build ran %q", call)
	}
	installed := filepath.Join(app, "mods", mod, "bin", executable())
	if b, _ := os.ReadFile(installed); string(b) != "built" {
		t.Fatalf("installed exe = %q", b)
	}
	if f.runKey != runKey(installed) || f.started != installed || len(f.quit) != 1 {
		t.Fatalf("runKey %q started %q quit %v", f.runKey, f.started, f.quit)
	}
}

func TestDeployQuitsTheCollectorOnItsPortBeforeReplacing(t *testing.T) {
	f := fakeTools(t)
	home := t.TempDir()
	t.Setenv("HERDR_TIME_HOME", home)
	if err := os.WriteFile(filepath.Join(home, "settings.json"), []byte(`{"port":5555}`), 0o644); err != nil {
		t.Fatal(err)
	}
	app := t.TempDir()
	installed := filepath.Join(app, "mods", mod, "bin", executable())
	if err := os.MkdirAll(filepath.Dir(installed), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installed, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := install(app); err != nil {
		t.Fatal(err)
	}
	if len(f.quit) != 1 || f.quit[0] != 5555 {
		t.Fatalf("quit %v", f.quit)
	}
	if strings.Join(f.order, ",") != "build,quit,start" {
		t.Fatalf("order %v", f.order)
	}
	if b, _ := os.ReadFile(installed); string(b) != "built" {
		t.Fatalf("installed exe = %q", b)
	}
}

func TestRunKeyQuotesPathsWithSpaces(t *testing.T) {
	got := runKey(`C:\Users\A B\herdr-time.exe`)
	if got != `"C:\Users\A B\herdr-time.exe" run` {
		t.Fatal(got)
	}
}

func TestDefaultPortWithoutSettings(t *testing.T) {
	t.Setenv("HERDR_TIME_HOME", t.TempDir())
	if port() != 47613 {
		t.Fatal(port())
	}
}

func TestMissingGoFailsBeforeBuilding(t *testing.T) {
	f := fakeTools(t)
	lookPath = func(string) (string, error) { return "", os.ErrNotExist }
	if install(t.TempDir()) == nil || len(f.calls) != 0 {
		t.Fatal("built without go")
	}
}
