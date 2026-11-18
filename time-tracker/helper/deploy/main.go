// Builds and installs the Time Tracker: the collector, its autostart and the mod's folder.
// From the mod's folder: go -C helper run ./deploy (again to update: the running collector is replaced)
// Not called install: Windows asks for an administrator to run a program named so.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

const (
	mod    = "time-tracker"
	title  = "Time Tracker"
	binary = "herdr-time"
	runVal = "herdr-time"
)

// Stand-ins for tests.
var (
	lookPath           = exec.LookPath
	out      io.Writer = os.Stdout
	run                = func(dir string, args ...string) error {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	setAutostart  = registerRun // writes the HKCU Run value
	startDetached = launch      // starts the collector without a console
	quit          = quitCollector
	buildPath     = func() string { return filepath.Join(root(), "mods", mod, "bin", executable()) }
)

func executable() string {
	if runtime.GOOS == "windows" {
		return binary + ".exe"
	}
	return binary
}

// runKey is the Run value: the program's path quoted, since it may have spaces.
func runKey(exe string) string { return `"` + exe + `" run` }

// replace puts fresh where the collector is. Windows won't overwrite a running program but will
// rename it: the old one moves aside and goes once nothing runs it, here or on the next update.
func replace(fresh, target string) error {
	if runtime.GOOS != "windows" {
		return os.Rename(fresh, target)
	}
	old := filepath.Join(filepath.Dir(target), "."+binary+".old.exe")
	_ = os.Remove(old)
	if _, err := os.Stat(target); err == nil {
		if err := os.Rename(target, old); err != nil {
			return err
		}
	}
	if err := os.Rename(fresh, target); err != nil {
		return err
	}
	_ = os.Remove(old)
	return nil
}

// root is the time-tracker folder in this repo, wherever the command was run from.
func root() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..")
}

// dataHome mirrors the collector's Home().
func dataHome() string {
	if h := os.Getenv("HERDR_TIME_HOME"); h != "" {
		return h
	}
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "herdr-time")
}

// readSettings takes the two keys deploy cares about from settings.json; missing means 47613 and on.
func readSettings() (port int, autostart bool) {
	s := struct {
		Port      int
		Autostart *bool
	}{}
	if b, err := os.ReadFile(filepath.Join(dataHome(), "settings.json")); err == nil {
		_ = json.Unmarshal(b, &s)
	}
	if s.Port <= 0 {
		s.Port = 47613
	}
	return s.Port, s.Autostart == nil || *s.Autostart
}

func port() int {
	p, _ := readSettings()
	return p
}

// quitCollector asks a running collector to exit and waits for its port to close; none running is fine.
func quitCollector(port int) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	req, _ := http.NewRequest("POST", "http://"+addr+"/v1/quit", nil)
	req.Header.Set("X-Herdr-Time", "1")
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		return nil
	}
	resp.Body.Close()
	for end := time.Now().Add(5 * time.Second); time.Now().Before(end); time.Sleep(100 * time.Millisecond) {
		c, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			return nil
		}
		c.Close()
	}
	return nil // replace() renames a program that is still running anyway
}

// copyTree copies from into to, leaving out the bin folder: the collector is installed separately.
func copyTree(from, to string) error {
	return filepath.WalkDir(from, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		if relative == "bin" {
			return filepath.SkipDir
		}
		destination := filepath.Join(to, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o755)
		}
		return copyFile(path, destination)
	})
}

func copyFile(from, to string) error {
	b, err := os.ReadFile(from)
	if err != nil {
		return err
	}
	return os.WriteFile(to, b, 0o755)
}

func install(appHome string) error {
	if _, err := lookPath("go"); err != nil {
		return errors.New("go is required (Go 1.23+ builds the collector)")
	}
	src := filepath.Join(root(), "mods", mod)
	built := buildPath()
	if err := os.MkdirAll(filepath.Dir(built), 0o755); err != nil {
		return err
	}
	// Build before touching the install; windowsgui keeps autostart from opening a console.
	if err := run(filepath.Join(root(), "helper"), "go", "build", "-ldflags", "-H=windowsgui", "-o", built, "."); err != nil {
		return err
	}
	// Always: a collector left running would keep the old program in use.
	if err := quit(port()); err != nil {
		return err
	}
	target := filepath.Join(appHome, "mods", mod)
	if err := copyTree(src, target); err != nil {
		return err
	}
	bin := filepath.Join(target, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		return err
	}
	installed := filepath.Join(bin, executable())
	fresh := filepath.Join(bin, "."+executable()+".new")
	if err := copyFile(built, fresh); err != nil {
		return err
	}
	if err := replace(fresh, installed); err != nil {
		return err
	}
	if _, on := readSettings(); on {
		if err := setAutostart(runKey(installed)); err != nil {
			return err
		}
	}
	if err := startDetached(installed); err != nil {
		return err
	}
	fmt.Fprintf(out, "Installed %s: %s\n", title, target)
	return nil
}

func main() {
	if runtime.GOOS != "windows" {
		fmt.Fprintln(os.Stderr, title+" collector is Windows-only")
		os.Exit(1)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	appHome := os.Getenv("HERDR_APP_HOME")
	if appHome == "" {
		appHome = filepath.Join(home, ".config", "herdr-app")
	}
	if err := install(appHome); err != nil { // an --update from older docs changes nothing
		fmt.Fprintln(os.Stderr, err)
		fmt.Fprintln(os.Stderr, title+" installation failed")
		os.Exit(1)
	}
}
