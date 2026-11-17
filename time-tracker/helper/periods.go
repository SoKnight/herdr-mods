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
