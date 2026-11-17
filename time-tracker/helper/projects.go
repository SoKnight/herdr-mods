package main

import (
	"cmp"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"slices"
	"strings"
)

var projectColors = []string{"ansi1", "ansi2", "ansi3", "ansi4", "ansi5", "ansi6"}

// editConfig changes projects.json as it is now, not as it was loaded: it may have been edited since.
// f's errors come back as they are; nothing is written then.
func (d *Daemon) editConfig(f func(c *Config) error) (Config, error) {
	c, err := readConfig(d.Home)
	if err != nil {
		return Config{}, fail(http.StatusConflict, "projects.json не читается: %v", err)
	}
	if err := f(&c); err != nil {
		return Config{}, err
	}
	if c, _, err = migrate(c); err == nil {
		_, err = c.Compile()
	}
	if err != nil {
		return Config{}, fail(http.StatusBadRequest, "%v", err)
	}
	path := filepath.Join(d.Home, "projects.json")
	if err := writeJSON(path, c); err != nil {
		return Config{}, err
	}
	d.cfgMod = modTime(path)
	return c, d.setConfig(c)
}

// scopeFrom is when an added or edited rule starts: today or the first day of this month.
func scopeFrom(s Settings, scope string, now int64) (int64, error) {
	day := s.DayOf(now)
	switch scope {
	case "today":
	case "month", "":
		day = day[:8] + "01"
	default:
		return 0, fail(http.StatusBadRequest, "scope: want today or month")
	}
	from, _, err := dayRange(s, day)
	return from, err
}

// newProjectID makes an id from the name's ASCII letters and digits, other runs becoming "-".
func newProjectID(name string, taken func(string) bool) string {
	var b strings.Builder
	gap := false
	for _, r := range strings.ToLower(name) {
		if ('a' <= r && r <= 'z') || ('0' <= r && r <= '9') {
			if gap && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			gap = false
		} else {
			gap = true
		}
	}
	base := cmp.Or(b.String(), "project")
	id := base
	for n := 2; taken(id) || id == excludeID || id == leisureID; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}

// moveBefore moves rule id to just before rule before when it is below it.
func moveBefore(rules []Rule, id, before int64) []Rule {
	i := slices.IndexFunc(rules, func(r Rule) bool { return r.ID == id })
	j := slices.IndexFunc(rules, func(r Rule) bool { return r.ID == before })
	if i < 0 || j < 0 || i <= j {
		return rules
	}
	r := rules[i]
	return slices.Insert(slices.Delete(rules, i, i+1), j, r)
}

type catalog struct {
	Projects   []catalogItem `json:"projects"`
	Activities []catalogItem `json:"activities"`
}

type catalogItem struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Label    string `json:"label,omitempty"`
	Color    string `json:"color,omitempty"`
	Archived bool   `json:"archived,omitempty"`
}

// getProjects lists the projects and the kinds of activity in config order, for the mod's menus and names.
func (d *Daemon) getProjects(r *http.Request) (int, any, error) {
	out := catalog{Projects: []catalogItem{}, Activities: []catalogItem{}}
	for _, p := range d.Config.Projects {
		out.Projects = append(out.Projects, catalogItem{ID: p.ID, Name: p.Name, Color: p.Color, Archived: p.Archived})
	}
	for _, a := range d.Config.Activities {
		label := activityLabels[a]
		if label == "" {
			label = a
		}
		out.Activities = append(out.Activities, catalogItem{ID: a, Label: label})
	}
	return http.StatusOK, out, nil
}

type projectRequest struct {
	Name     *string `json:"name"`
	Color    *string `json:"color"`
	Archived *bool   `json:"archived"`
}

// apply checks what the request names and sets it on p.
func (req projectRequest) apply(p *Project) error {
	if req.Name != nil {
		if p.Name = strings.TrimSpace(*req.Name); p.Name == "" {
			return fail(http.StatusBadRequest, "name is required")
		}
	}
	if req.Color != nil {
		if !slices.Contains(projectColors, *req.Color) {
			return fail(http.StatusBadRequest, "color: want ansi1..ansi6")
		}
		p.Color = *req.Color
	}
	if req.Archived != nil {
		p.Archived = *req.Archived
	}
	return nil
}

func (d *Daemon) postProject(r *http.Request) (int, any, error) {
	var req projectRequest
	if err := decode(r, &req); err != nil {
		return 0, nil, err
	}
	var p Project
	_, err := d.editConfig(func(c *Config) error {
		if req.Name == nil {
			req.Name = new(string)
		}
		if req.Color == nil || *req.Color == "" {
			color := "ansi1"
			for _, col := range projectColors {
				if !slices.ContainsFunc(c.Projects, func(x Project) bool { return !x.Archived && x.Color == col }) {
					color = col
					break
				}
			}
			req.Color = &color
		}
		if err := req.apply(&p); err != nil {
			return err
		}
		p.ID = newProjectID(p.Name, func(id string) bool {
			return slices.ContainsFunc(c.Projects, func(x Project) bool { return x.ID == id })
		})
		c.Projects = append(c.Projects, p)
		return nil
	})
	return http.StatusCreated, p, err
}

func (d *Daemon) patchProject(r *http.Request) (int, any, error) {
	id := r.PathValue("id")
	var req projectRequest
	if err := decode(r, &req); err != nil {
		return 0, nil, err
	}
	var p Project
	_, err := d.editConfig(func(c *Config) error {
		i := slices.IndexFunc(c.Projects, func(x Project) bool { return x.ID == id })
		if i < 0 {
			return fail(http.StatusNotFound, "no project %q", id)
		}
		if err := req.apply(&c.Projects[i]); err != nil {
			return err
		}
		p = c.Projects[i]
		return nil
	})
	return http.StatusOK, p, err
}

type ruleInfo struct {
	Rule
	Ms      int64 `json:"ms"`
	Windows int   `json:"windows"`
}

// getRules lists the open rules in order with what each matched over the last 7 days: my time and
// Досуг it still decides (a correction that moved the project took that time away from it).
func (d *Daemon) getRules(r *http.Request) (int, any, error) {
	now := nowMs()
	tl, err := d.fresh(now-7*24*60*60_000, now)
	if err != nil {
		return 0, nil, err
	}
	segs := slices.Concat(tl.Me, tl.Leisure)
	out := []ruleInfo{}
	for _, rule := range d.Config.Rules {
		if rule.To != 0 {
			continue
		}
		info := ruleInfo{Rule: rule}
		windows := map[[2]string]bool{}
		for _, s := range segs {
			if s.Rule == rule.ID && s.Project == rule.Project {
				info.Ms += s.End - s.Start
				windows[[2]string{s.App, s.Title}] = true
			}
		}
		info.Windows = len(windows)
		out = append(out, info)
	}
	return http.StatusOK, map[string]any{"rules": out}, nil
}

type ruleRequest struct {
	Rule
	Scope  string `json:"scope"`
	Before int64  `json:"before"`
}

// rule checks the request against c and gives the rule it asks for, without id or dates.
func (req ruleRequest) rule(c *Config) (Rule, error) {
	r := req.Rule
	r.ID, r.From, r.To = 0, 0, 0
	r.Value = strings.TrimSpace(r.Value)
	if r.Kind == "folder" {
		for _, glob := range []string{`\**`, `\*`, `\`} {
			if v, ok := strings.CutSuffix(r.Value, glob); ok {
				r.Value = v
				break
			}
		}
	}
	switch {
	case !slices.Contains(ruleKinds, r.Kind):
		return r, fail(http.StatusBadRequest, "kind: want folder, process, title, figma or minecraft")
	case r.Value == "":
		return r, fail(http.StatusBadRequest, "value is required")
	case r.Project != excludeID && r.Project != leisureID && !slices.ContainsFunc(c.Projects, func(p Project) bool { return p.ID == r.Project }):
		return r, fail(http.StatusBadRequest, "unknown project %q", r.Project)
	}
	return r, nil
}

func nextRuleID(rules []Rule) int64 {
	var id int64
	for _, r := range rules {
		id = max(id, r.ID)
	}
	return id + 1
}

// openRule is the index of rule id when it is still open, else -1.
func openRule(rules []Rule, id int64) int {
	return slices.IndexFunc(rules, func(r Rule) bool { return r.ID == id && r.To == 0 })
}

// ruleArgs reads the {id} and the body of a rule request, and the day its scope starts.
func ruleArgs(r *http.Request, s Settings) (id, from int64, req ruleRequest, err error) {
	if r.PathValue("id") != "" {
		if id, err = intParam(r.PathValue("id"), "id"); err != nil {
			return
		}
	}
	if err = decode(r, &req); err != nil {
		return
	}
	from, err = scopeFrom(s, req.Scope, nowMs())
	return
}

// postRule adds a rule from the start of today or this month; the same open rule again is answered as is.
func (d *Daemon) postRule(r *http.Request) (int, any, error) {
	_, from, req, err := ruleArgs(r, d.S)
	if err != nil {
		return 0, nil, err
	}
	var out Rule
	same := errors.New("same rule")
	_, err = d.editConfig(func(c *Config) error {
		rule, err := req.rule(c)
		if err != nil {
			return err
		}
		if i := slices.IndexFunc(c.Rules, func(x Rule) bool {
			return x.To == 0 && x.Kind == rule.Kind && strings.EqualFold(x.Value, rule.Value) &&
				strings.EqualFold(x.Title, rule.Title) && x.Project == rule.Project
		}); i >= 0 {
			out = c.Rules[i]
			return same
		}
		rule.ID, rule.From = nextRuleID(c.Rules), from
		c.Rules = moveBefore(insertRule(c.Rules, rule), rule.ID, req.Before)
		out = rule
		return nil
	})
	if err == same {
		return http.StatusOK, out, nil
	}
	return http.StatusCreated, out, err
}

// putRule edits a rule from its scope's day on: a rule that started before that day is closed there and
// followed by the new version, so the time before keeps its project.
func (d *Daemon) putRule(r *http.Request) (int, any, error) {
	id, from, req, err := ruleArgs(r, d.S)
	if err != nil {
		return 0, nil, err
	}
	var out Rule
	_, err = d.editConfig(func(c *Config) error {
		i := openRule(c.Rules, id)
		if i < 0 {
			return fail(http.StatusNotFound, "no rule %d", id)
		}
		rule, err := req.rule(c)
		if err != nil {
			return err
		}
		rule.From = from
		if c.Rules[i].From >= from {
			rule.ID = id
			c.Rules[i] = rule
		} else {
			c.Rules[i].To = from
			rule.ID = nextRuleID(c.Rules)
			c.Rules = slices.Insert(c.Rules, i+1, rule)
		}
		c.Rules = moveBefore(c.Rules, rule.ID, req.Before)
		out = rule
		return nil
	})
	return http.StatusOK, out, err
}

// deleteRule removes a rule that started this month; an older one is closed at the month's start.
func (d *Daemon) deleteRule(r *http.Request) (int, any, error) {
	id, err := intParam(r.PathValue("id"), "id")
	if err != nil {
		return 0, nil, err
	}
	month, err := scopeFrom(d.S, "month", nowMs())
	if err != nil {
		return 0, nil, err
	}
	_, err = d.editConfig(func(c *Config) error {
		i := openRule(c.Rules, id)
		switch {
		case i < 0:
			return fail(http.StatusNotFound, "no rule %d", id)
		case c.Rules[i].From >= month:
			c.Rules = slices.Delete(c.Rules, i, i+1)
		default:
			c.Rules[i].To = month
		}
		return nil
	})
	return http.StatusNoContent, nil, err
}
