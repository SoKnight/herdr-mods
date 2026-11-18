package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const version = "1.0.0"

// Seams for tests, like run and lookPath in branch-pr's deploy.
var (
	nowMs        = func() int64 { return time.Now().UnixMilli() }
	takeSample   = sample
	bootAt       = bootTime
	logonAt      = logonTime
	followAgents = streamAgents
)

type Daemon struct {
	Home   string
	Store  *Store
	Rules  *Rules
	Config Config
	S      Settings
	Cal    Calendar

	// mu guards everything above and the trackers. The poll loop, the agent stream and every HTTP
	// handler hold it, and closed spans reach the store before it is released, so a timeline never
	// sees a span neither open nor stored.
	mu       sync.Mutex
	tracker  Tracker
	agents   AgentTracker
	cfgMod   time.Time
	setMod   time.Time
	calMod   time.Time
	quit     chan struct{}
	quitOnce sync.Once

	// A bad projects.json or settings.json found at start: collection runs on the defaults and the
	// read endpoints answer with the error until a reload reads the file.
	cfgErr, setErr, calErr error
}

func NewDaemon(home string) (*Daemon, error) {
	d := &Daemon{Home: home, quit: make(chan struct{})}
	if err := d.loadConfig(); err != nil {
		log.Printf("projects.json: %v; collecting with the default rules", err)
		if err := d.setConfig(DefaultConfig()); err != nil {
			return nil, err
		}
		d.cfgErr = err
	}
	if err := d.loadSettings(); err != nil {
		log.Printf("settings.json: %v; collecting with the default settings", err)
		d.S = DefaultSettings()
		d.tracker.PollMs = int64(d.S.PollSeconds) * 1000
		d.setErr = err
	}
	cal, err := loadCalendar(home)
	d.calMod = modTime(filepath.Join(home, "calendar.json")) // after loadCalendar: it may create the file
	if err != nil {
		log.Printf("%v; collecting with the shipped calendar", err)
		d.calErr = err
		if cal, err = parseCalendar(shippedCalendar); err != nil {
			return nil, err
		}
	}
	d.Cal = cal
	d.Store = &Store{Dir: home, S: d.S}
	return d, nil
}

// configErr is the start-up config error, as the read endpoints answer it.
func (d *Daemon) configErr() error {
	if err := cmp.Or(d.cfgErr, d.setErr, d.calErr); err != nil {
		// The version tells a starting copy that this one is ours, not a stranger on the port.
		return &apiError{http.StatusInternalServerError, map[string]any{"error": "config: " + err.Error(), "version": version}}
	}
	return nil
}

func modTime(path string) time.Time {
	fi, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return fi.ModTime()
}

// loadConfig reads projects.json at startup, writing the default one if it is missing.
func (d *Daemon) loadConfig() error {
	c, err := LoadConfig(d.Home)
	d.cfgMod = modTime(filepath.Join(d.Home, "projects.json")) // after LoadConfig: it may create the file
	if err != nil {
		return err
	}
	return d.setConfig(c)
}

// readConfig reads projects.json as it is now; unlike LoadConfig it never creates it. A v1 file is
// migrated and written back, its original bytes kept once in projects.v1.json; a file that cannot be
// migrated is left as it is.
func readConfig(home string) (Config, error) {
	path := filepath.Join(home, "projects.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	c, changed, err := migrate(c)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if !changed {
		return c, nil
	}
	backup := filepath.Join(home, "projects.v1.json")
	if _, err := os.Stat(backup); errors.Is(err, os.ErrNotExist) {
		if err := writeFile(backup, data); err != nil {
			return Config{}, err
		}
	} else if err != nil {
		return Config{}, err
	}
	return c, writeJSON(path, c)
}

// setConfig migrates in memory too: configs built in code (defaults, tests) may still use v1's lists.
func (d *Daemon) setConfig(c Config) error {
	c, _, err := migrate(c)
	if err != nil {
		return err
	}
	r, err := c.Compile()
	if err != nil {
		return err
	}
	d.Config, d.Rules, d.cfgErr = c, r, nil
	d.tracker.CallApps = nil
	for _, a := range c.Apps {
		if a.Presence == "call" {
			d.tracker.CallApps = append(d.tracker.CallApps, a.Exe)
		}
	}
	return nil
}

// loadSettings reads settings.json; on error the current settings stay.
func (d *Daemon) loadSettings() error {
	d.setMod = modTime(filepath.Join(d.Home, "settings.json"))
	s, err := LoadSettings(d.Home)
	if err != nil {
		return err
	}
	if d.S.Loc != nil {
		s.Loc = d.S.Loc
	}
	d.S, d.setErr = s, nil
	d.tracker.PollMs = int64(s.PollSeconds) * 1000
	d.tracker.Hidden = s.HiddenTitles
	return nil
}

// reload picks up edited projects.json, settings.json and calendar.json. A port change needs a restart.
// A missing projects.json (an editor saving by delete and rename) keeps the rules until it is back.
func (d *Daemon) reload() {
	if mod := modTime(filepath.Join(d.Home, "projects.json")); !mod.IsZero() && mod != d.cfgMod {
		d.cfgMod = mod
		c, err := readConfig(d.Home)
		if err == nil {
			err = d.setConfig(c)
		}
		if err != nil {
			log.Printf("projects.json: %v; keeping the previous rules", err)
		}
	}
	if modTime(filepath.Join(d.Home, "settings.json")) != d.setMod {
		if err := d.loadSettings(); err != nil {
			log.Printf("settings.json: %v; keeping the previous settings", err)
		}
	}
	if mod := modTime(filepath.Join(d.Home, "calendar.json")); !mod.IsZero() && mod != d.calMod {
		d.calMod = mod
		if c, err := loadCalendar(d.Home); err != nil {
			log.Printf("%v; keeping the previous calendar", err)
		} else {
			d.Cal, d.calErr = c, nil
		}
	}
}

// recoverOpen stores what the last run left open, then marks the time from the latest of boot, logon
// and the last known power-on until now as on without data: the collector did not watch it. Logon
// counts because a Fast Startup "shutdown" does not reset the boot time.
func (d *Daemon) recoverOpen() error {
	spans, err := d.Store.RecoverOpen()
	if err != nil {
		return err
	}
	var lastPower int64
	for _, sp := range spans {
		if sp.End <= sp.Start {
			continue
		}
		if err := d.Store.AppendSpan(sp); err != nil {
			return err
		}
		if sp.Kind == "power" {
			lastPower = max(lastPower, sp.End)
		}
	}
	if err := d.Store.ClearOpen(); err != nil { // only once its spans are in the day files
		return err
	}
	now := nowMs()
	start := max(bootAt(now), logonAt(), lastPower)
	if start <= 0 || start >= now {
		return nil
	}
	for _, kind := range []string{"power", "nodata"} {
		if err := d.Store.AppendSpan(Span{Kind: kind, Start: start, End: now}); err != nil {
			return err
		}
	}
	return nil
}

// Run polls Windows every poll_seconds and follows the agents until ctx is done or /v1/quit.
func (d *Daemon) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	if err := d.recoverOpen(); err != nil {
		return err
	}
	go followAgents(ctx, d.agentsSeen, d.agentsLost)
	for {
		d.poll()
		d.mu.Lock()
		wait := time.Duration(d.S.PollSeconds) * time.Second
		d.mu.Unlock()
		select {
		case <-ctx.Done():
			return d.saveOpen()
		case <-d.quit:
			return d.saveOpen()
		case <-time.After(wait):
		}
	}
}

// lastSample is the time of the last poll, 0 before the first. Callers hold mu.
func (d *Daemon) lastSample() int64 {
	if d.tracker.prev == nil {
		return 0
	}
	return d.tracker.prev.At
}

func (d *Daemon) stop() { d.quitOnce.Do(func() { close(d.quit) }) }

func (d *Daemon) poll() {
	now := nowMs()
	s, err := takeSample(now)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.reload()
	if err != nil {
		log.Printf("sample: %v", err) // a partial sample would close the window span
		return
	}
	if p := d.tracker.prev; d.tracker.slept(s.At) {
		// The tracker closes its spans at the last poll before the sleep; the agents follow it.
		d.appendSpans(d.agents.Cut(p.At, s.At))
	}
	// ponytail: reads the log tail every poll while online; cache by mod time if it ever shows in a profile
	if isMinecraftOnline(s.Exe, s.Title) {
		s.Server = minecraftServer(d.S.MinecraftLogs)
	}
	d.appendSpans(d.tracker.Feed(s))
	if err := d.Store.SaveOpen(d.openSpans(now), d.tracker.lastSeen(now)); err != nil {
		log.Printf("open.json: %v", err)
	}
}

// saveOpen keeps the open spans on exit too, so the next start recovers them as after a crash and
// the time since then counts from here, not from boot.
func (d *Daemon) saveOpen() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := d.tracker.lastSeen(nowMs())
	return d.Store.SaveOpen(d.openSpans(now), now)
}

func (d *Daemon) agentsSeen(states map[string]AgentState) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.appendSpans(d.agents.Feed(states, nowMs()))
}

func (d *Daemon) agentsLost() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.appendSpans(d.agents.Lost(nowMs()))
}

func (d *Daemon) appendSpans(spans []Span) {
	for _, sp := range spans {
		if err := d.Store.AppendSpan(sp); err != nil {
			log.Printf("store: %v", err)
		}
	}
}

// openSpans gives the open spans ended at at, or at the last poll when at is past a sleep gap.
func (d *Daemon) openSpans(at int64) []Span {
	at = d.tracker.lastSeen(at)
	return append(d.tracker.Open(at), d.agents.Open(at)...)
}

// fresh is the timeline over [from, to) as the rules give it now, open spans included. Callers hold mu.
func (d *Daemon) fresh(from, to int64) (Timeline, error) {
	since := from - int64(d.S.CarryMinutes)*60_000
	raw, err := d.Store.Load(since, to)
	if err != nil {
		return Timeline{}, err
	}
	for _, sp := range d.openSpans(nowMs()) {
		if sp.Start, sp.End = max(sp.Start, since), min(sp.End, to); sp.Start < sp.End {
			raw.Spans = append(raw.Spans, sp)
		}
	}
	corr, err := d.Store.Corrections()
	if err != nil {
		return Timeline{}, err
	}
	return Attribute(raw, d.Rules, corr, d.S, from, to), nil
}

// timeline is fresh with the closed periods' snapshots in place, each cut to [from, to). Callers hold mu.
func (d *Daemon) timeline(from, to int64) (Timeline, []Divergence, error) {
	tl, err := d.fresh(from, to)
	if err != nil {
		return tl, nil, err
	}
	periods, err := d.Store.Periods()
	if err != nil {
		return tl, nil, err
	}
	var in []Period
	for _, p := range periods {
		if p.From, p.To = max(p.From, from), min(p.To, to); p.From < p.To {
			in = append(in, p)
		}
	}
	tl, div := ApplyPeriods(tl, in)
	return tl, div, nil
}

// dayRange is the day's [DayStart, next DayStart) in the settings' location.
func dayRange(s Settings, day string) (from, to int64, err error) {
	loc, shift := s.dayClock()
	t, err := time.ParseInLocation("2006-01-02", day, loc)
	if err != nil {
		return 0, 0, err
	}
	from = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, loc).Add(shift).UnixMilli()
	return from, nextDayStart(s, from), nil
}

func (d *Daemon) order() []string {
	var ids []string
	for _, p := range d.Config.Projects {
		ids = append(ids, p.ID)
	}
	return ids
}

func (d *Daemon) project(id string) *Project {
	for i := range d.Config.Projects {
		if d.Config.Projects[i].ID == id {
			return &d.Config.Projects[i]
		}
	}
	return nil
}
