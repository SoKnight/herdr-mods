package main

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func shipped(t *testing.T) Calendar {
	c, err := loadCalendar(t.TempDir())
	must(t, err)
	return c
}

func TestShippedCalendar2026(t *testing.T) {
	c := shipped(t)
	count := func(prefix string) int {
		n := 0
		for d := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC); d.Year() == 2026; d = d.AddDate(0, 0, 1) {
			if day := d.Format("2006-01-02"); strings.HasPrefix(day, prefix) && c.Working(day) {
				n++
			}
		}
		return n
	}
	assertEq(t, count("2026"), 247)
	assertEq(t, count("2026-11"), 20)
	assertEq(t, fmt.Sprint(c.Working("2026-11-04"), c.Working("2026-01-09"), c.Working("2026-11-16"), c.Working("2026-11-21")), "false false true false")
	assertEq(t, c.Holiday("2026-03-08"), true)
	assertEq(t, weekday("2026-11-16"), 1)
	assertEq(t, weekday("2026-11-01"), 7)
	assertEq(t, fmt.Sprint(c.Missing([]string{"2026-12-31", "2027-01-11", "2027-02-01"})), "[2027]")
}

func TestLoadCalendar(t *testing.T) {
	dir := t.TempDir()
	_, err := loadCalendar(dir)
	must(t, err)
	data, err := os.ReadFile(filepath.Join(dir, "calendar.json"))
	must(t, err)
	assertEq(t, string(data), string(shippedCalendar))

	must(t, os.WriteFile(filepath.Join(dir, "calendar.json"), []byte(`{"2026": {"holidays": ["2026-11-16"], "workdays": ["2026-11-28"]}, "2027": {"holidays": ["2027-01-01"], "workdays": []}}`), 0o644))
	c, err := loadCalendar(dir)
	must(t, err)
	assertEq(t, fmt.Sprint(c.Working("2026-11-16"), c.Working("2026-11-28"), c.Working("2026-11-04"), c.Working("2027-01-01")), "false true true false")

	for _, bad := range []string{`{`, `{"2026": {"holidays": ["2026-13-01"]}}`, `{"2026": {"holidays": ["2027-01-01"]}}`} {
		must(t, os.WriteFile(filepath.Join(dir, "calendar.json"), []byte(bad), 0o644))
		_, err := loadCalendar(dir)
		assertEq(t, fmt.Sprint(bad, err != nil && strings.HasPrefix(err.Error(), "calendar.json: ")), fmt.Sprint(bad, true))
	}
	_, err = loadCalendar(dir)
	assertEq(t, err.Error(), `calendar.json: 2026: "2027-01-01" — не дата этого года`)
}

func TestBadCalendarIsAConfigError(t *testing.T) {
	home := t.TempDir()
	must(t, writeJSON(filepath.Join(home, "projects.json"), serverConfig()))
	must(t, os.WriteFile(filepath.Join(home, "calendar.json"), []byte(`{"2026": {"holidays": ["4 ноября"]}}`), 0o644))
	d, err := NewDaemon(home)
	must(t, err)
	var e struct {
		Error string `json:"error"`
	}
	assertEq(t, serve(t, d).do("GET", "/v1/status", nil, &e), http.StatusInternalServerError)
	assertEq(t, strings.Contains(e.Error, "calendar.json"), true)
	assertEq(t, d.Cal.Working("2026-11-04"), false) // collecting on with the shipped one
}

func TestCalendarReload(t *testing.T) {
	home := t.TempDir()
	must(t, writeJSON(filepath.Join(home, "projects.json"), serverConfig()))
	d, err := NewDaemon(home)
	must(t, err)
	path := filepath.Join(home, "calendar.json")
	edit := func(s string, mod time.Time) {
		must(t, os.WriteFile(path, []byte(s), 0o644))
		must(t, os.Chtimes(path, mod, mod))
		d.reload()
	}
	edit(`{"2026": {"holidays": ["2026-11-16"]}}`, time.Now().Add(time.Hour))
	assertEq(t, d.Cal.Working("2026-11-16"), false)
	edit(`{`, time.Now().Add(2*time.Hour)) // broken: the previous calendar stays
	assertEq(t, d.Cal.Working("2026-11-16"), false)
}
