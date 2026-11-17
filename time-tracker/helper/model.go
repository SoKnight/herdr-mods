package main

// Span kinds: power, nodata, input, lock, window, call, agent, agents_unknown.
type Span struct {
	Kind   string `json:"k"`
	Start  int64  `json:"s"`
	End    int64  `json:"e"`
	Exe    string `json:"exe,omitempty"`
	Title  string `json:"title,omitempty"`
	Server string `json:"server,omitempty"`
	Term   string `json:"term,omitempty"`
	Cwd    string `json:"cwd,omitempty"`
}

type Heartbeat struct {
	Source      string `json:"source"`
	Time        int64  `json:"time"`
	App         string `json:"app"`
	ProjectPath string `json:"project_path"`
	ProjectName string `json:"project_name"`
	URL         string `json:"url"`
	Entity      string `json:"entity"`
	Activity    string `json:"activity"`
	Focused     bool   `json:"focused"`
}

type Raw struct {
	Spans []Span
	Beats []Heartbeat
}

// Kind: project|activity|exclude|add|note. Target: me|agents|both.
type Correction struct {
	ID       int64  `json:"id"`
	Kind     string `json:"kind"`
	Target   string `json:"target"`
	Start    int64  `json:"start"`
	End      int64  `json:"end"`
	Project  string `json:"project"`
	Activity string `json:"activity"`
	Note     string `json:"note"`
}

type Segment struct {
	Start     int64  `json:"start"`
	End       int64  `json:"end"`
	App       string `json:"app"`
	Title     string `json:"title"`
	Server    string `json:"server,omitempty"`
	Path      string `json:"path"` // folder from the app's fresh signal, for "assign a project"
	Project   string `json:"project"`
	Activity  string `json:"activity"`
	Note      string `json:"note"`
	Why       string `json:"why"`
	Corrected bool   `json:"corrected"`
	Manual    bool   `json:"manual"`
	Rule      int64  `json:"rule,omitempty"` // the rule that matched; 0 for carry, a signal's name, added time
}

type AgentSeg struct {
	Start     int64  `json:"start"`
	End       int64  `json:"end"`
	Project   string `json:"project"`
	Why       string `json:"why"`
	Corrected bool   `json:"corrected"`
}

type Period struct {
	ID         int64      `json:"id"`
	Project    string     `json:"project"`
	From       int64      `json:"from"`
	To         int64      `json:"to"`
	ExportedAt int64      `json:"exported_at"`
	Me         []Segment  `json:"me"`
	Agents     []AgentSeg `json:"agents"`
}
