package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Settings struct {
	IdleMinutes  int    `json:"idle_minutes"`
	CarryMinutes int    `json:"carry_minutes"`
	Port         int    `json:"port"`
	PollSeconds  int    `json:"poll_seconds"`
	DayStart     string `json:"day_start"`

	Loc *time.Location `json:"-"`
}

func DefaultSettings() Settings {
	return Settings{IdleMinutes: 5, CarryMinutes: 15, Port: 47613, PollSeconds: 5, DayStart: "04:00", Loc: time.Local}
}

// Home is where data lives, outside the mod folder so reinstalling the mod keeps it.
func Home() string {
	if h := os.Getenv("HERDR_TIME_HOME"); h != "" {
		return h
	}
	return filepath.Join(os.Getenv("LOCALAPPDATA"), "herdr-time")
}

func LoadConfig(home string) (Config, error) {
	path := filepath.Join(home, "projects.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		c := DefaultConfig()
		data, _ = json.MarshalIndent(c, "", "  ")
		if err := os.MkdirAll(home, 0o755); err != nil {
			return c, err
		}
		return c, os.WriteFile(path, data, 0o644)
	}
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
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
	if s.IdleMinutes <= 0 || s.Port <= 0 || s.PollSeconds <= 0 || s.CarryMinutes < 0 {
		return s, fmt.Errorf("%s: idle_minutes, port, poll_seconds must be positive and carry_minutes not negative", path)
	}
	if _, err := time.Parse("15:04", s.DayStart); err != nil {
		return s, fmt.Errorf("%s: day_start %q: %w", path, s.DayStart, err)
	}
	return s, nil
}

// DayOf returns the YYYY-MM-DD the instant belongs to; the day starts at DayStart.
func (s Settings) DayOf(ms int64) string {
	loc := s.Loc
	if loc == nil {
		loc = time.Local
	}
	start, err := time.Parse("15:04", s.DayStart)
	if err != nil {
		start = time.Time{}
	}
	shift := time.Duration(start.Hour())*time.Hour + time.Duration(start.Minute())*time.Minute
	return time.UnixMilli(ms).In(loc).Add(-shift).Format("2006-01-02")
}
