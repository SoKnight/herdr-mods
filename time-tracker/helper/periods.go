package main

import (
	"net/http"
	"slices"
)

// closedBy returns the first closed period, of the project or of any project when project is "",
// that [s, e) overlaps.
func closedBy(periods []Period, project string, s, e int64) *Period {
	for i, p := range periods {
		if (project == "" || p.Project == project) && s < p.To && p.From < e {
			return &periods[i]
		}
	}
	return nil
}

// overlaps tells whether any of a shares time with any of b; each is already cut to its own period.
// ponytail: pairwise, O(len(a)·len(b)); sort and sweep if long exports get slow.
func overlaps(a, b []Segment) bool {
	for _, x := range a {
		for _, y := range b {
			if x.Start < y.End && y.Start < x.End {
				return true
			}
		}
	}
	return false
}

func (d *Daemon) postExport(r *http.Request) (int, any, error) {
	var req struct {
		Project string `json:"project"`
		From    int64  `json:"from"`
		To      int64  `json:"to"`
		Format  string `json:"format"`
		Round   int    `json:"round"`
		Agents  bool   `json:"agents"`
		DryRun  bool   `json:"dry_run"` // the content only: no period is closed
	}
	if err := decode(r, &req); err != nil {
		return 0, nil, err
	}
	proj := d.project(req.Project)
	req.To = min(req.To, nowMs()) // time that has not happened yet must stay open
	switch {
	case proj == nil:
		return 0, nil, fail(http.StatusBadRequest, "unknown project %q", req.Project)
	case req.From >= req.To:
		return 0, nil, fail(http.StatusBadRequest, "from must be before to")
	case req.Format != "csv" && req.Format != "md":
		return 0, nil, fail(http.StatusBadRequest, "format: want csv or md")
	case req.Round != 1 && req.Round != 5 && req.Round != 15:
		return 0, nil, fail(http.StatusBadRequest, "round: want 1, 5 or 15")
	}
	periods, err := d.Store.Periods()
	if err != nil {
		return 0, nil, err
	}
	// Exporting a closed range again would stack snapshots; reopen it first.
	if p := closedBy(periods, req.Project, req.From, req.To); p != nil {
		return 0, nil, closedPeriod(p.ID)
	}
	tl, err := d.fresh(req.From, req.To)
	if err != nil {
		return 0, nil, err
	}
	snap := Snapshot(tl, req.Project, req.From, req.To)
	// The same minutes never go to two clients: another project's export may already hold some of them.
	for _, p := range periods {
		if p.Project != req.Project && overlaps(snap.Me, p.Me) {
			return 0, nil, closedPeriod(p.ID)
		}
	}
	total := ProjectTotal{Project: req.Project, Activities: map[string]int64{}, Days: map[string]DayTotal{}}
	if rep := Summarize(Timeline{Me: snap.Me, Agents: snap.Agents}, d.S, nil); len(rep.Projects) > 0 {
		total = rep.Projects[0]
	}
	var notes []Segment // the snapshot is already cut to [from, to)
	for _, seg := range snap.Me {
		if seg.Note != "" {
			notes = append(notes, seg)
		}
	}
	name := proj.Name
	if name == "" {
		name = proj.ID
	}
	var content string
	if req.Format == "csv" {
		content = ExportCSV(total, notes, req.Round, req.Agents, d.S)
	} else {
		content = ExportMarkdown(name, req.From, req.To, total, notes, req.Round, req.Agents, d.S)
	}
	if req.DryRun {
		return http.StatusOK, map[string]any{"content": content}, nil
	}
	snap.ExportedAt = nowMs()
	saved, err := d.Store.AddPeriod(snap)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, map[string]any{"content": content, "period": saved.ID}, nil
}

type periodInfo struct {
	ID         int64  `json:"id"`
	Project    string `json:"project"`
	From       int64  `json:"from"`
	To         int64  `json:"to"`
	ExportedAt int64  `json:"exported_at"`
	DeltaMs    int64  `json:"delta_ms"`
}

// getPeriods lists closed periods with how far fresh rules would move each one's time.
func (d *Daemon) getPeriods(r *http.Request) (int, any, error) {
	periods, err := d.Store.Periods()
	if err != nil {
		return 0, nil, err
	}
	project := r.URL.Query().Get("project")
	out := []periodInfo{}
	for _, p := range periods {
		if project != "" && p.Project != project {
			continue
		}
		tl, err := d.fresh(p.From, p.To)
		if err != nil {
			return 0, nil, err
		}
		info := periodInfo{p.ID, p.Project, p.From, p.To, p.ExportedAt, 0}
		if _, div := ApplyPeriods(tl, []Period{p}); len(div) > 0 {
			info.DeltaMs = div[0].DeltaMs
		}
		out = append(out, info)
	}
	return http.StatusOK, out, nil
}

func (d *Daemon) reopenPeriod(r *http.Request) (int, any, error) {
	id, err := intParam(r.PathValue("id"), "id")
	if err != nil {
		return 0, nil, err
	}
	periods, err := d.Store.Periods()
	if err != nil {
		return 0, nil, err
	}
	if !slices.ContainsFunc(periods, func(p Period) bool { return p.ID == id }) {
		return 0, nil, fail(http.StatusNotFound, "no period %d", id)
	}
	return http.StatusNoContent, nil, d.Store.RemovePeriod(id)
}
