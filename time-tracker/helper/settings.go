package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"os"
	pathpkg "path"
	"path/filepath"
	"strings"
	"time"
)

// setRun turns the collector's start with Windows on or off; a seam for tests.
var setRun = setRunValue

type Settings struct {
	IdleMinutes  int    `json:"idle_minutes"`
	CarryMinutes int    `json:"carry_minutes"`
	Port         int    `json:"port"`
	PollSeconds  int    `json:"poll_seconds"`
	DayStart     string `json:"day_start"`

	SessionIdleMinutes int `json:"session_idle_minutes"`
	PauseMinutes       int `json:"pause_minutes"`

	MinecraftLogs []string `json:"minecraft_logs"` // latest.log files to read the server from

	VideoActivity bool     `json:"video_activity"`
	Autostart     bool     `json:"autostart"`
	SilentWarning bool     `json:"silent_warning"`
	CloseReminder bool     `json:"close_reminder"`
	HiddenTitles  []string `json:"hidden_titles"` // exe or title patterns (path.Match, case-insensitive) whose titles are not recorded

	Loc *time.Location `json:"-"`
}

func DefaultSettings() Settings {
	return Settings{IdleMinutes: 5, CarryMinutes: 15, Port: 47613, PollSeconds: 5, DayStart: "06:00", SessionIdleMinutes: 45, PauseMinutes: 15, MinecraftLogs: []string{`%APPDATA%\.minecraft\logs\latest.log`}, VideoActivity: true, Autostart: true, SilentWarning: true, CloseReminder: true, HiddenTitles: []string{}, Loc: time.Local}
}

// Home is where data lives, outside the mod folder so reinstalling the mod keeps it.
func Home() string {
	if h := os.Getenv("HERDR_TIME_HOME"); h != "" {
		return h
	}
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "herdr-time")
}

// LoadConfig is readConfig, writing the default projects.json first when there is none.
func LoadConfig(home string) (Config, error) {
	path := filepath.Join(home, "projects.json")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		c := DefaultConfig()
		return c, writeJSON(path, c)
	}
	return readConfig(home)
}

// LoadSettings overlays settings.json on the defaults; a missing file is not an error.
func LoadSettings(home string) (Settings, error) {
	s := DefaultSettings()
	path := filepath.Join(home, "settings.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("%s: %w", path, err)
	}
	if err := s.check(); err != nil {
		return s, fmt.Errorf("%s: %w", path, err)
	}
	return s, nil
}

// validHidden reports whether p is a usable hidden-title pattern: non-empty and well-formed.
func validHidden(p string) bool {
	if p == "" {
		return false
	}
	_, err := pathpkg.Match(strings.ToLower(p), "")
	return err == nil
}

// check is what a settings file must satisfy; LoadSettings and the settings route share it.
func (s Settings) check() error {
	if s.IdleMinutes <= 0 || s.Port <= 0 || s.PollSeconds <= 0 || s.SessionIdleMinutes <= 0 || s.PauseMinutes <= 0 || s.CarryMinutes < 0 {
		return errors.New("idle_minutes, port, poll_seconds, session_idle_minutes and pause_minutes must be positive and carry_minutes not negative")
	}
	if _, err := time.Parse("15:04", s.DayStart); err != nil {
		return fmt.Errorf("day_start %q: %w", s.DayStart, err)
	}
	for _, p := range s.HiddenTitles {
		if !validHidden(p) {
			return fmt.Errorf("hidden_titles: bad pattern %q", p)
		}
	}
	return nil
}

// editableSettings is what GET and PUT /v1/settings speak; the rest of settings.json is the file's own.
type editableSettings struct {
	SessionIdleMinutes int      `json:"session_idle_minutes"`
	PauseMinutes       int      `json:"pause_minutes"`
	DayStart           string   `json:"day_start"`
	VideoActivity      bool     `json:"video_activity"`
	Autostart          bool     `json:"autostart"`
	SilentWarning      bool     `json:"silent_warning"`
	CloseReminder      bool     `json:"close_reminder"`
	HiddenTitles       []string `json:"hidden_titles"`
}

func (s Settings) editable() editableSettings {
	h := s.HiddenTitles
	if h == nil {
		h = []string{}
	}
	return editableSettings{s.SessionIdleMinutes, s.PauseMinutes, s.DayStart, s.VideoActivity, s.Autostart, s.SilentWarning, s.CloseReminder, h}
}

func (d *Daemon) getSettings(r *http.Request) (int, any, error) {
	return http.StatusOK, d.S.editable(), nil
}

// putSettings overlays the body on settings.json (other keys stay), checks the result and only then
// writes it; the daemon applies it at once instead of waiting for the file's mtime.
func (d *Daemon) putSettings(r *http.Request) (int, any, error) {
	var body map[string]json.RawMessage
	if err := decode(r, &body); err != nil {
		return 0, nil, err
	}
	var known map[string]json.RawMessage
	b, _ := json.Marshal(editableSettings{})
	_ = json.Unmarshal(b, &known)
	for k := range body {
		if _, ok := known[k]; !ok {
			return 0, nil, fail(http.StatusBadRequest, "unknown setting %q", k)
		}
	}
	file := filepath.Join(d.Home, "settings.json")
	merged := map[string]json.RawMessage{}
	if data, err := os.ReadFile(file); err == nil {
		if err := json.Unmarshal(data, &merged); err != nil {
			return 0, nil, fmt.Errorf("%s: %w", file, err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return 0, nil, err
	}
	maps.Copy(merged, body)
	out, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return 0, nil, err
	}
	s := DefaultSettings()
	if err := json.Unmarshal(out, &s); err != nil {
		return 0, nil, fail(http.StatusBadRequest, "%v", err)
	}
	if err := s.check(); err != nil {
		return 0, nil, fail(http.StatusBadRequest, "%v", err)
	}
	if err := writeFile(file, out); err != nil {
		return 0, nil, err
	}
	if err := d.loadSettings(); err != nil {
		return 0, nil, err
	}
	if _, ok := body["autostart"]; ok {
		if err := setRun(d.S.Autostart); err != nil {
			return 0, nil, err
		}
	}
	return http.StatusOK, d.S.editable(), nil
}

// dayClock is the location and the offset of the day start from local midnight; DayOf and the
// day-boundary splitting both go through it, so labels and split points cannot drift apart.
func (s Settings) dayClock() (*time.Location, time.Duration) {
	loc := s.Loc
	if loc == nil {
		loc = time.Local
	}
	start, err := time.Parse("15:04", s.DayStart)
	if err != nil {
		start = time.Time{}
	}
	return loc, time.Duration(start.Hour())*time.Hour + time.Duration(start.Minute())*time.Minute
}

// DayOf returns the YYYY-MM-DD the instant belongs to; the day starts at DayStart.
func (s Settings) DayOf(ms int64) string {
	loc, shift := s.dayClock()
	return time.UnixMilli(ms).In(loc).Add(-shift).Format("2006-01-02")
}
