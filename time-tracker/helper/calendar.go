package main

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"time"
)

// calYear is one year of the RF production calendar.
type calYear struct {
	Holidays []string `json:"holidays"` // non-working holidays, weekend ones too, YYYY-MM-DD
	Workdays []string `json:"workdays"` // weekend days moved to working days
}

// Calendar is the production calendar by year, "2026".
type Calendar map[string]calYear

//go:embed calendar.json
var shippedCalendar []byte

func parseCalendar(data []byte) (Calendar, error) {
	var c Calendar
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("calendar.json: %w", err)
	}
	for year, y := range c {
		for _, day := range slices.Concat(y.Holidays, y.Workdays) {
			if t, err := time.Parse("2006-01-02", day); err != nil || fmt.Sprint(t.Year()) != year {
				return nil, fmt.Errorf("calendar.json: %s: %q — не дата этого года", year, day)
			}
		}
	}
	return c, nil
}

// loadCalendar overlays home's calendar.json, year by year, on the shipped years; it writes the shipped
// bytes there when the file is missing.
func loadCalendar(home string) (Calendar, error) {
	c, err := parseCalendar(shippedCalendar)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(home, "calendar.json")
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, writeFile(path, shippedCalendar)
	} else if err != nil {
		return c, err
	}
	own, err := parseCalendar(data)
	if err != nil {
		return c, err
	}
	for year, y := range own {
		c[year] = y
	}
	return c, nil
}

// weekday is 1 = Monday … 7 = Sunday, 0 for what is not a day.
func weekday(day string) int {
	t, err := time.Parse("2006-01-02", day)
	if err != nil {
		return 0
	}
	return (int(t.Weekday())+6)%7 + 1
}

func (c Calendar) Holiday(day string) bool {
	return len(day) >= 4 && slices.Contains(c[day[:4]].Holidays, day)
}

// Moved reports a weekend day moved to a working one.
func (c Calendar) Moved(day string) bool {
	return len(day) >= 4 && slices.Contains(c[day[:4]].Workdays, day)
}

func (c Calendar) Working(day string) bool {
	return c.Moved(day) || (weekday(day) >= 1 && weekday(day) <= 5 && !c.Holiday(day))
}

// Missing lists, sorted, the years of days the calendar has no entry for.
func (c Calendar) Missing(days []string) []string {
	out := []string{}
	for _, day := range days {
		if len(day) < 4 {
			continue
		}
		if _, ok := c[day[:4]]; !ok && !slices.Contains(out, day[:4]) {
			out = append(out, day[:4])
		}
	}
	slices.Sort(out)
	return out
}
