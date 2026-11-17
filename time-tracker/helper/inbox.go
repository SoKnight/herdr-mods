package main

import (
	"cmp"
	"log"
	"net/http"
	"slices"
	"strings"
	"unicode/utf8"
)

// Разбор: a day's Другое time by window, with a guess at where it belongs, and the decisions on it.

type suggestion struct {
	Project string `json:"project"` // a project id or excludeID
	Rule    Rule   `json:"rule"`    // kind, value, title only
}

type inboxItem struct {
	App     string      `json:"app"`
	Title   string      `json:"title"`
	Path    string      `json:"path"`
	Server  string      `json:"server"`
	Ms      int64       `json:"ms"`
	Start   int64       `json:"start"`
	End     int64       `json:"end"`
	Suggest *suggestion `json:"suggest"`
}

type inboxDecision struct {
	App     string `json:"app"`
	Title   string `json:"title"`
	Path    string `json:"path"`
	Server  string `json:"server"`
	Project string `json:"project"` // a project id or excludeID
	Rule    *Rule  `json:"rule"`    // nil: corrections for this window on this day only
}

var notWorkApps = []string{"Telegram.exe", "Discord.exe", "WhatsApp.exe", "steam.exe", "Spotify.exe"}

// suggest guesses a window's project: the first open project whose name, else id, is in its path or title,
// else Не работа for a messenger.
func suggest(it inboxItem, projects []Project) *suggestion {
	for _, p := range projects {
		if p.Archived {
			continue
		}
		for _, s := range []string{p.Name, p.ID} {
			switch {
			case utf8.RuneCountInString(s) < 3:
			case containsFold(it.Path, s):
				return &suggestion{p.ID, Rule{Kind: "folder", Value: it.Path}}
			case containsFold(it.Title, s):
				return &suggestion{p.ID, Rule{Kind: "title", Value: s}}
			}
		}
	}
	// ponytail: a fixed list of messengers; learn from earlier exclusions if it guesses badly
	if slices.ContainsFunc(notWorkApps, func(a string) bool { return strings.EqualFold(a, it.App) }) {
		return &suggestion{excludeID, Rule{Kind: "process", Value: it.App}}
	}
	return nil
}

func windowOf(app, title, path, server string) [4]string { return [4]string{app, title, path, server} }

// inboxDay is the day's [from, to), today when day is ""; a day that has not begun is refused.
func (d *Daemon) inboxDay(day string) (string, int64, int64, error) {
	now := nowMs()
	day = cmp.Or(day, d.S.DayOf(now))
	from, to, err := dayRange(d.S, day)
	if err != nil {
		return "", 0, 0, fail(http.StatusBadRequest, "day: want YYYY-MM-DD, got %q", day)
	}
	if from >= now {
		return "", 0, 0, fail(http.StatusBadRequest, "day %s has not begun", day)
	}
	return day, from, to, nil
}

// otherTime is my Другое time over [from, min(to, now)) that a window shows, by start. Callers hold mu.
func (d *Daemon) otherTime(from, to int64) ([]Segment, error) {
	tl, _, err := d.timeline(from, min(to, nowMs()))
	if err != nil {
		return nil, err
	}
	out := slices.DeleteFunc(tl.Me, func(s Segment) bool { return s.Project != "" || s.Manual || s.App == "" && s.Title == "" })
	slices.SortStableFunc(out, func(a, b Segment) int { return cmp.Compare(a.Start, b.Start) })
	return out, nil
}

// getInbox lists the day's Другое windows by when each first showed, with a suggestion each.
func (d *Daemon) getInbox(r *http.Request) (int, any, error) {
	day, from, to, err := d.inboxDay(r.URL.Query().Get("day"))
	if err != nil {
		return 0, nil, err
	}
	segs, err := d.otherTime(from, to)
	if err != nil {
		return 0, nil, err
	}
	items := []inboxItem{}
	index := map[[4]string]int{}
	var total int64
	for _, s := range segs {
		k := windowOf(s.App, s.Title, s.Path, s.Server)
		i, ok := index[k]
		if !ok {
			i, index[k] = len(items), len(items)
			items = append(items, inboxItem{App: s.App, Title: s.Title, Path: s.Path, Server: s.Server, Start: s.Start})
		}
		items[i].Ms += s.End - s.Start
		items[i].End = max(items[i].End, s.End)
		total += s.End - s.Start
	}
	for i := range items {
		items[i].Suggest = suggest(items[i], d.Config.Projects)
	}
	return http.StatusOK, map[string]any{"day": day, "from": from, "to": to, "ms": total, "items": items}, nil
}

// postInbox applies Разбор's decisions: a rule from the day's start (with past, from this month's start
// when that is earlier), or corrections over that window's Другое time that day. Everything is checked
// before anything is written, so a refused call writes nothing.
func (d *Daemon) postInbox(r *http.Request) (int, any, error) {
	var req struct {
		Day       string          `json:"day"`
		Past      bool            `json:"past"`
		Decisions []inboxDecision `json:"decisions"`
	}
	if err := decode(r, &req); err != nil {
		return 0, nil, err
	}
	_, from, to, err := d.inboxDay(req.Day)
	if err != nil {
		return 0, nil, err
	}
	now := nowMs()
	ruleFrom := from
	if req.Past {
		month, err := scopeFrom(d.S, "month", now)
		if err != nil {
			return 0, nil, err
		}
		ruleFrom = min(from, month)
	}
	var rules []ruleRequest
	target := map[[4]string]string{} // window → project, for those decided without a rule
	for _, dec := range req.Decisions {
		if dec.Project != excludeID && d.project(dec.Project) == nil {
			return 0, nil, fail(http.StatusBadRequest, "unknown project %q", dec.Project)
		}
		if dec.Rule == nil {
			target[windowOf(dec.App, dec.Title, dec.Path, dec.Server)] = dec.Project
			continue
		}
		rr := ruleRequest{Rule: *dec.Rule}
		rr.Project = dec.Project
		if _, err := rr.rule(&d.Config); err != nil {
			return 0, nil, err
		}
		rules = append(rules, rr)
	}

	var cs []Correction
	if len(target) > 0 {
		segs, err := d.otherTime(from, to)
		if err != nil {
			return 0, nil, err
		}
		last := map[[4]string]int{} // window → its latest correction in cs
		for _, s := range segs {
			k := windowOf(s.App, s.Title, s.Path, s.Server)
			project, ok := target[k]
			if !ok {
				continue
			}
			if i, ok := last[k]; ok && s.Start <= cs[i].End { // touching: one correction
				cs[i].End = max(cs[i].End, s.End)
				cs[i].Was[0].Me += s.End - s.Start
				continue
			}
			c := Correction{Kind: "project", Target: "me", Project: project, Start: s.Start, End: s.End, Made: now,
				Was: []Share{{"", s.End - s.Start}}}
			if project == excludeID {
				c.Kind, c.Project = "exclude", ""
			}
			last[k] = len(cs)
			cs = append(cs, c)
		}
		periods, err := d.Store.Periods()
		if err != nil {
			return 0, nil, err
		}
		for _, c := range cs {
			if p := closedBy(periods, "", c.Start, c.End); p != nil {
				return 0, nil, closedPeriod(p.ID)
			}
		}
	}

	var added []Rule
	if len(rules) > 0 {
		changes, err := d.Store.Changes()
		if err != nil {
			return 0, nil, err
		}
		if _, err := d.editConfig(func(c *Config) error {
			for _, rr := range rules {
				rule, err := rr.rule(c)
				if err != nil {
					return err
				}
				if slices.ContainsFunc(c.Rules, func(x Rule) bool { return sameOpenRule(x, rule) && x.From <= ruleFrom }) {
					continue // already covers the day, or made twice in this call
				}
				rule.ID, rule.From = nextRuleID(c.Rules, changes), ruleFrom
				c.Rules = insertRule(c.Rules, rule)
				added = append(added, rule)
			}
			return nil
		}); err != nil {
			return 0, nil, err
		}
		d.logInboxRules(added, ruleFrom, now) // before the corrections: rules kept by a failed write stay undoable
	}
	// ponytail: projects.json and corrections.json are two files; a disk error between them keeps the rules
	if len(cs) > 0 {
		if _, err := d.Store.AddCorrections(cs); err != nil {
			return 0, nil, err
		}
	}
	return http.StatusOK, map[string]int{"rules": len(added), "corrections": len(cs)}, nil
}

// logInboxRules stores a rule-add change for each rule Разбор made, with one timeline for all of them.
// The rules are already in projects.json, so a failure here is only logged.
func (d *Daemon) logInboxRules(added []Rule, from, now int64) {
	if len(added) == 0 {
		return
	}
	ids := make([]int64, len(added))
	for i, r := range added {
		ids[i] = r.ID
	}
	ms, err := d.ruleTime(ids, from, now)
	if err != nil {
		log.Printf("history: inbox: %v", err)
	}
	for _, r := range added {
		c := Change{At: now, Kind: "rule-add", Rule: &r, From: from, Ms: ms[r.ID], Source: "inbox"}
		if _, err := d.Store.AddChange(c); err != nil {
			log.Printf("history: inbox rule-add #%d: %v", r.ID, err)
		}
	}
}
