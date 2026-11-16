package main

import (
	"fmt"
	"regexp"
	"strings"
)

type Config struct {
	Activities []string   `json:"activities"`
	Projects   []Project  `json:"projects"`
	Apps       []AppRule  `json:"apps"`
	Files      []FileRule `json:"files"`
}

type Project struct {
	ID     string   `json:"id"`
	Name   string   `json:"name"`
	Client string   `json:"client,omitempty"`
	Paths  []string `json:"paths,omitempty"`
	Titles []string `json:"titles,omitempty"`
	URLs   []string `json:"urls,omitempty"`
	Figma  []string `json:"figma,omitempty"`
	Apps   []string `json:"apps,omitempty"`
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
	telemostExe = "telemost.exe" // placeholder until the real exe is known (Task 9)
	figmaExe    = "Figma.exe"
	devRoot     = `C:\Users\SoKnight\Development\`
)

func DefaultConfig() Config {
	return Config{
		Activities: []string{"development", "design-work", "design", "documentation", "study"},
		Projects: []Project{
			{ID: "universe", Name: "Universe", Client: "StarMC", Paths: []string{devRoot + "starmc"}, Titles: []string{"(?i)universe"}},
			{ID: "easylauncher", Name: "EasyLauncher", Paths: []string{devRoot + "easyx"}, Figma: []string{"(?i)easylauncher"}},
			{ID: "tricky", Name: "Tricky Network", Paths: []string{devRoot + "tricky-network"}},
			{ID: "study", Name: "Учёба", Apps: []string{telemostExe}},
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

type cproject struct {
	Project
	paths  []string // normalized
	titles []*regexp.Regexp
	urls   []*regexp.Regexp
	figma  []*regexp.Regexp
}

type Rules struct {
	projects []cproject
	apps     []AppRule
	files    []*regexp.Regexp
	fileActs []string
}

func compileAll(id string, pats []string) ([]*regexp.Regexp, error) {
	var out []*regexp.Regexp
	for _, p := range pats {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("project %s: bad regex %q: %w", id, p, err)
		}
		out = append(out, re)
	}
	return out, nil
}

func (c Config) Compile() (*Rules, error) {
	r := &Rules{apps: c.Apps}
	for _, p := range c.Projects {
		cp := cproject{Project: p}
		for _, path := range p.Paths {
			cp.paths = append(cp.paths, normPath(path))
		}
		var err error
		if cp.titles, err = compileAll(p.ID, p.Titles); err != nil {
			return nil, err
		}
		if cp.urls, err = compileAll(p.ID, p.URLs); err != nil {
			return nil, err
		}
		if cp.figma, err = compileAll(p.ID, p.Figma); err != nil {
			return nil, err
		}
		r.projects = append(r.projects, cp)
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

func (r *Rules) ProjectByPath(path string) (id, why string) {
	n := normPath(path)
	best := -1
	for _, p := range r.projects {
		for i, pp := range p.paths {
			if pp != "" && (n == pp || strings.HasPrefix(n, pp+`\`)) && len(pp) > best {
				best, id, why = len(pp), p.ID, "папка "+p.Paths[i]+" → "+p.ID
			}
		}
	}
	return
}

func (r *Rules) matchRe(get func(cproject) []*regexp.Regexp, s, label string) (id, why string) {
	for _, p := range r.projects {
		for _, re := range get(p) {
			if re.MatchString(s) {
				return p.ID, label + " " + re.String() + " → " + p.ID
			}
		}
	}
	return "", ""
}

func (r *Rules) ProjectByTitle(exe, title string) (id, why string) {
	if id, why = r.matchRe(func(p cproject) []*regexp.Regexp { return p.titles }, title, "заголовок"); id != "" {
		return
	}
	if strings.EqualFold(exe, figmaExe) {
		return r.matchRe(func(p cproject) []*regexp.Regexp { return p.figma }, title, "Figma")
	}
	return "", ""
}

func (r *Rules) ProjectByApp(exe string) (id, why string) {
	for _, p := range r.projects {
		for _, a := range p.Apps {
			if strings.EqualFold(a, exe) {
				return p.ID, "приложение " + a + " → " + p.ID
			}
		}
	}
	return "", ""
}

func (r *Rules) ProjectByURL(url string) (id, why string) {
	return r.matchRe(func(p cproject) []*regexp.Regexp { return p.urls }, url, "адрес")
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
