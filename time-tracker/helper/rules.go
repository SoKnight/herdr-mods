package main

import (
	"cmp"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

type Config struct {
	Activities []string   `json:"activities"`
	Projects   []Project  `json:"projects"`
	Rules      []Rule     `json:"rules"`
	Apps       []AppRule  `json:"apps"`
	Files      []FileRule `json:"files"`
}

// Project's Paths, Titles, URLs, Figma and Apps are v1's match lists, only read to be migrated to rules.
type Project struct {
	ID       string   `json:"id"`
	Name     string   `json:"name"`
	Client   string   `json:"client,omitempty"`
	Color    string   `json:"color,omitempty"`
	Archived bool     `json:"archived,omitempty"`
	Paths    []string `json:"paths,omitempty"`
	Titles   []string `json:"titles,omitempty"`
	URLs     []string `json:"urls,omitempty"`
	Figma    []string `json:"figma,omitempty"`
	Apps     []string `json:"apps,omitempty"`
}

// Rule sends a window to a project; the first rule in Config.Rules that matches wins.
type Rule struct {
	ID      int64  `json:"id"`
	Kind    string `json:"kind"`            // folder | process | title | figma | minecraft
	Value   string `json:"value"`           // folder; exe; title text; Figma file text; server host:port
	Title   string `json:"title,omitempty"` // process only: the window title contains it
	Project string `json:"project"`         // a project id, excludeID or leisureID
	From    int64  `json:"from,omitempty"`  // applies to time in [From, To); 0 = open
	To      int64  `json:"to,omitempty"`
}

const excludeID, leisureID = "exclude", "leisure"

var ruleKinds = []string{"folder", "process", "title", "figma", "minecraft"}

// window is what a rule looks at: the focused app, its title, its signal's folder, the Minecraft server.
type window struct {
	exe, title, path, server string
	at                       int64
}

// AppRule maps an exe to an activity. Presence "call" keeps me present without input;
// Source names the signal sender whose heartbeat belongs to this app.
type AppRule struct {
	Exe      string `json:"exe"`
	Activity string `json:"activity,omitempty"`
	Presence string `json:"presence,omitempty"`
	Source   string `json:"source,omitempty"`
}

type FileRule struct {
	Match    string `json:"match"`
	Activity string `json:"activity"`
}

const (
	telemostExe = "YandexTelemost.exe"
	figmaExe    = "Figma.exe"
	devRoot     = `C:\Users\SoKnight\Development\`
)

func DefaultConfig() Config {
	return Config{
		Activities: []string{"development", "architecture", "design", "documentation", "study"},
		Projects: []Project{
			{ID: "universe", Name: "Universe", Client: "StarMC", Color: "ansi1"},
			{ID: "easylauncher", Name: "EasyLauncher", Color: "ansi2"},
			{ID: "tricky", Name: "Tricky Network", Color: "ansi3"},
			{ID: "study", Name: "Учёба", Color: "ansi4"},
		},
		Rules: []Rule{
			{ID: 1, Kind: "folder", Value: devRoot + "tricky-network", Project: "tricky"},
			{ID: 2, Kind: "folder", Value: devRoot + "starmc", Project: "universe"},
			{ID: 3, Kind: "folder", Value: devRoot + "easyx", Project: "easylauncher"},
			{ID: 4, Kind: "process", Value: telemostExe, Project: "study"},
			{ID: 5, Kind: "title", Value: "universe", Project: "universe"},
			{ID: 6, Kind: "figma", Value: "easylauncher", Project: "easylauncher"},
		},
		Apps: []AppRule{
			{Exe: "idea64.exe", Activity: "development", Source: "idea"},
			{Exe: "herdr-app.exe", Activity: "development", Source: "herdr"},
			{Exe: "firefox.exe", Source: "firefox"},
			{Exe: figmaExe, Activity: "design"},
			{Exe: telemostExe, Activity: "study", Presence: "call"},
		},
		Files: []FileRule{{Match: `\.mdx?$`, Activity: "documentation"}},
	}
}

type Rules struct {
	rules    []Rule
	projects []Project
	apps     []AppRule
	files    []*regexp.Regexp
	fileActs []string
}

func (c Config) Compile() (*Rules, error) {
	r := &Rules{rules: c.Rules, projects: c.Projects, apps: c.Apps}
	ids := map[int64]bool{}
	for _, x := range c.Rules {
		switch {
		case x.ID <= 0:
			return nil, fmt.Errorf("rule %d: id must be positive", x.ID)
		case ids[x.ID]:
			return nil, fmt.Errorf("rule %d: id is used twice", x.ID)
		case !slices.Contains(ruleKinds, x.Kind):
			return nil, fmt.Errorf("rule %d: kind %q: want one of %s", x.ID, x.Kind, strings.Join(ruleKinds, ", "))
		case x.Value == "":
			return nil, fmt.Errorf("rule %d: value is required", x.ID)
		case x.Project != excludeID && x.Project != leisureID && !slices.ContainsFunc(c.Projects, func(p Project) bool { return p.ID == x.Project }):
			return nil, fmt.Errorf("rule %d: unknown project %q", x.ID, x.Project)
		}
		ids[x.ID] = true
	}
	for _, f := range c.Files {
		re, err := regexp.Compile(f.Match)
		if err != nil {
			return nil, fmt.Errorf("file rule: bad regex %q: %w", f.Match, err)
		}
		r.files = append(r.files, re)
		r.fileActs = append(r.fileActs, f.Activity)
	}
	return r, nil
}

// normPath makes paths comparable: lower case, backslashes, no trailing separator.
func normPath(p string) string {
	return strings.TrimRight(strings.ToLower(strings.ReplaceAll(p, "/", `\`)), `\`)
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}

func (r Rule) matches(w window) bool {
	if (r.From != 0 && w.at < r.From) || (r.To != 0 && w.at >= r.To) {
		return false
	}
	switch r.Kind {
	case "folder":
		v, p := normPath(r.Value), normPath(w.path)
		return v != "" && (p == v || strings.HasPrefix(p, v+`\`))
	case "process":
		return strings.EqualFold(w.exe, r.Value) && containsFold(w.title, r.Title)
	case "title":
		return containsFold(w.title, r.Value)
	case "figma":
		return strings.EqualFold(w.exe, figmaExe) && containsFold(w.title, r.Value)
	case "minecraft":
		return strings.EqualFold(w.server, r.Value)
	}
	return false
}

func (r Rule) why() string {
	switch r.Kind {
	case "folder":
		return "папка " + r.Value + " → " + r.Project
	case "process":
		if r.Title != "" {
			return "процесс " + r.Value + " «" + r.Title + "» → " + r.Project
		}
		return "процесс " + r.Value + " → " + r.Project
	case "title":
		return "заголовок «" + r.Value + "» → " + r.Project
	case "figma":
		return "Figma «" + r.Value + "» → " + r.Project
	}
	return "Minecraft " + r.Value + " → " + r.Project
}

// Match returns the first rule that matches the window, whatever its kind.
func (rs *Rules) Match(w window) (Rule, bool) {
	for _, r := range rs.rules {
		if r.matches(w) {
			return r, true
		}
	}
	return Rule{}, false
}

// FolderProject is the project of the first folder rule over path, for agents, which have only a folder.
func (rs *Rules) FolderProject(path string, at int64) (id, why string) {
	w := window{path: path, at: at}
	for _, r := range rs.rules {
		if r.Kind == "folder" && r.matches(w) {
			return r.Project, r.why()
		}
	}
	return "", ""
}

// insertRule puts r after the last rule of its own or an earlier kind, leaving rules as it was.
func insertRule(rules []Rule, r Rule) []Rule {
	k, at := slices.Index(ruleKinds, r.Kind), 0
	for i, x := range rules {
		if slices.Index(ruleKinds, x.Kind) <= k {
			at = i + 1
		}
	}
	return slices.Insert(slices.Clip(rules), at, r)
}

// migrate turns v1's match lists on projects into rules in the order that keeps v1's attribution:
// folders longest first (v1 took the longest prefix), then apps, titles and Figma files, each in project
// order. It refuses what a rule cannot express. It also gives each uncoloured project the colour it had.
func migrate(c Config) (Config, bool, error) {
	legacy := func(p Project) bool { return len(p.Paths)+len(p.Titles)+len(p.URLs)+len(p.Figma)+len(p.Apps) > 0 }
	if !slices.ContainsFunc(c.Projects, legacy) && !slices.ContainsFunc(c.Projects, func(p Project) bool { return p.Color == "" }) {
		return c, false, nil
	}
	var folders, apps, titles, figma []Rule
	text := func(p Project, kind string, pats []string, out *[]Rule) error {
		for _, pat := range pats {
			v := strings.TrimPrefix(pat, "(?i)")
			if v == "" || regexp.QuoteMeta(v) != v {
				return fmt.Errorf("project %s: %q — не простой текст, перепишите его правилом в панели", p.ID, pat)
			}
			*out = append(*out, Rule{Kind: kind, Value: v, Project: p.ID})
		}
		return nil
	}
	for _, p := range c.Projects {
		if len(p.URLs) > 0 {
			return c, false, fmt.Errorf("project %s: правила по адресу (urls) больше не поддерживаются", p.ID)
		}
		for _, v := range p.Paths {
			if normPath(v) != "" { // v1 skipped an empty folder
				folders = append(folders, Rule{Kind: "folder", Value: v, Project: p.ID})
			}
		}
		for _, v := range p.Apps {
			if strings.TrimSpace(v) != "" { // an empty exe never matched in v1
				apps = append(apps, Rule{Kind: "process", Value: v, Project: p.ID})
			}
		}
		if err := text(p, "title", p.Titles, &titles); err != nil {
			return c, false, err
		}
		if err := text(p, "figma", p.Figma, &figma); err != nil {
			return c, false, err
		}
	}
	slices.SortStableFunc(folders, func(a, b Rule) int { return cmp.Compare(len(normPath(b.Value)), len(normPath(a.Value))) })
	var next int64
	for _, r := range c.Rules {
		next = max(next, r.ID)
	}
	for _, r := range slices.Concat(folders, apps, titles, figma) {
		next++
		r.ID = next
		c.Rules = insertRule(c.Rules, r)
	}
	c.Projects = slices.Clone(c.Projects)
	for i := range c.Projects {
		p := &c.Projects[i]
		p.Paths, p.Titles, p.URLs, p.Figma, p.Apps = nil, nil, nil, nil, nil
		if p.Color == "" {
			p.Color = fmt.Sprintf("ansi%d", i%6+1)
		}
	}
	return c, true, nil
}

// ProjectByName matches a signal's project name against a project's name or id, ignoring case.
func (r *Rules) ProjectByName(name string) (id, why string) {
	if name == "" {
		return "", ""
	}
	for _, p := range r.projects {
		if strings.EqualFold(name, p.Name) || strings.EqualFold(name, p.ID) {
			return p.ID, "имя проекта " + name + " → " + p.ID
		}
	}
	return "", ""
}

func (r *Rules) ActivityByFile(name string) string {
	for i, re := range r.files {
		if re.MatchString(name) {
			return r.fileActs[i]
		}
	}
	return ""
}

func (r *Rules) App(exe string) *AppRule {
	for i := range r.apps {
		if strings.EqualFold(r.apps[i].Exe, exe) {
			return &r.apps[i]
		}
	}
	return nil
}
