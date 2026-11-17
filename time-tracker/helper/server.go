package main

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

// apiError is an error answer: its status and JSON body.
type apiError struct {
	code int
	body map[string]any
}

func (e *apiError) Error() string { return fmt.Sprint(e.body["error"]) }

func fail(code int, format string, a ...any) error {
	return &apiError{code, map[string]any{"error": fmt.Sprintf(format, a...)}}
}

func closedPeriod(id int64) error {
	return &apiError{http.StatusConflict, map[string]any{"error": "closed", "period": id}}
}

// api runs f under the daemon lock and writes its answer as JSON; no value means 204.
// ponytail: one lock for every request and the poll; a month report delays a poll by its run time.
func (d *Daemon) api(f func(r *http.Request) (int, any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
		code, v, err := func() (int, any, error) {
			d.mu.Lock()
			defer d.mu.Unlock() // a handler that panics must not keep the collector locked
			return f(r)
		}()
		if err != nil {
			var ae *apiError
			if !errors.As(err, &ae) {
				ae = &apiError{http.StatusInternalServerError, map[string]any{"error": err.Error()}}
			}
			code, v = ae.code, ae.body
		}
		if v == nil {
			w.WriteHeader(code)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(v)
	}
}

// guard lets in only requests to 127.0.0.1 or localhost (no DNS rebinding) carrying X-Herdr-Time: 1,
// which a web page cannot send without a preflight; preflights pass only for Firefox extensions.
func guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deny := func() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			fmt.Fprintln(w, `{"error":"forbidden"}`)
		}
		if host, _, _ := net.SplitHostPort(r.Host); host != "127.0.0.1" && host != "localhost" {
			deny()
			return
		}
		origin := r.Header.Get("Origin")
		ext := strings.HasPrefix(origin, "moz-extension://")
		if ext {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}
		if r.Method == http.MethodOptions {
			if !ext {
				deny()
				return
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE")
			w.Header().Set("Access-Control-Allow-Headers", "X-Herdr-Time, Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.Header.Get("X-Herdr-Time") != "1" {
			deny()
			return
		}
		next.ServeHTTP(w, r)
	})
}

func newServer(d *Daemon) http.Handler {
	mux := http.NewServeMux()
	type handler = func(*http.Request) (int, any, error)
	// read answers with the start-up config error while it lasts: numbers from default rules would mislead.
	read := func(f handler) handler {
		return func(r *http.Request) (int, any, error) {
			if err := d.configErr(); err != nil {
				return 0, nil, err
			}
			return f(r)
		}
	}
	routes := map[string]handler{
		"POST /v1/heartbeat":           d.postHeartbeat,
		"GET /v1/status":               read(d.getStatus),
		"GET /v1/timeline":             read(d.getTimeline),
		"GET /v1/session":              read(d.getSession),
		"POST /v1/session":             d.postSession,
		"GET /v1/report":               read(d.getReport),
		"POST /v1/export":              read(d.postExport),
		"GET /v1/corrections":          d.getCorrections,
		"POST /v1/corrections":         d.postCorrection,
		"DELETE /v1/corrections/{id}":  d.deleteCorrection,
		"GET /v1/inbox":                read(d.getInbox),
		"POST /v1/inbox":               d.postInbox,
		"GET /v1/rules":                read(d.getRules),
		"POST /v1/rules":               d.postRule,
		"PUT /v1/rules/{id}":           d.putRule,
		"DELETE /v1/rules/{id}":        d.deleteRule,
		"POST /v1/rules/preview":       read(d.postRulePreview),
		"GET /v1/hints":                read(d.getHints),
		"GET /v1/history":              d.getHistory,
		"POST /v1/history/{id}/undo":   d.postUndo,
		"GET /v1/periods":              d.getPeriods,
		"GET /v1/projects":             read(d.getProjects),
		"POST /v1/projects":            d.postProject,
		"PATCH /v1/projects/{id}":      d.patchProject,
		"POST /v1/periods/{id}/reopen": d.reopenPeriod,
		"POST /v1/quit":                d.postQuit,
	}
	for pattern, f := range routes {
		mux.Handle(pattern, d.api(f))
	}
	return guard(mux)
}

func decode(r *http.Request, v any) error {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		var big *http.MaxBytesError
		if errors.As(err, &big) {
			return fail(http.StatusRequestEntityTooLarge, "body over %d bytes", big.Limit)
		}
		return fail(http.StatusBadRequest, "bad JSON: %v", err)
	}
	return nil
}

func intParam(s, name string) (int64, error) {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fail(http.StatusBadRequest, "%s: want an integer, got %q", name, s)
	}
	return n, nil
}

// rangeParams reads ?from=&to= in Unix ms.
func rangeParams(r *http.Request) (from, to int64, err error) {
	q := r.URL.Query()
	if from, err = intParam(q.Get("from"), "from"); err != nil {
		return
	}
	if to, err = intParam(q.Get("to"), "to"); err != nil {
		return
	}
	if from >= to {
		err = fail(http.StatusBadRequest, "from must be before to")
	}
	return
}

func (d *Daemon) checkActivity(a string) error {
	if a != "" && !slices.Contains(d.Config.Activities, a) {
		return fail(http.StatusBadRequest, "unknown activity %q", a)
	}
	return nil
}

func (d *Daemon) postHeartbeat(r *http.Request) (int, any, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, 64<<10) // every beat lands in the day file
	var hb Heartbeat
	if err := decode(r, &hb); err != nil {
		return 0, nil, err
	}
	if hb.Source == "" || hb.Time <= 0 {
		return 0, nil, fail(http.StatusBadRequest, "source and time are required")
	}
	if err := d.checkActivity(hb.Activity); err != nil {
		return 0, nil, err
	}
	return http.StatusNoContent, nil, d.Store.AppendBeat(hb)
}

func (d *Daemon) getStatus(r *http.Request) (int, any, error) {
	now := nowMs()
	from, _, err := dayRange(d.S, d.S.DayOf(now))
	if err != nil {
		return 0, nil, err
	}
	tl, _, err := d.timeline(now-24*60*60_000, now)
	if err != nil {
		return 0, nil, err
	}
	rep := Summarize(clipTimeline(tl, from, now), d.S, d.order())
	ss, err := d.sessions(tl.Me, now)
	if err != nil {
		return 0, nil, err
	}
	var project, name, activity string
	var lastActive, sessionStart int64
	state := "none"
	if n := len(tl.Me); n > 0 {
		lastActive = tl.Me[n-1].End
	}
	if n := len(ss); n > 0 && ss[n-1].Open {
		sessionStart = ss[n-1].Start
		state = "working"
		// Away only from activity inside the session: a session opened by hand has none to be away from yet.
		if lastActive >= sessionStart && now-lastActive >= int64(d.S.IdleMinutes)*60_000 {
			state = "away"
		}
	}
	// I am still on the last segment unless I have been away for the idle threshold.
	if n := len(tl.Me); n > 0 && tl.Me[n-1].End >= now-int64(d.S.IdleMinutes)*60_000 {
		last := tl.Me[n-1]
		activity, project, name = last.Activity, last.Project, last.Project
		if p := d.project(project); p != nil && p.Name != "" {
			name = p.Name
		}
	}
	return http.StatusOK, map[string]any{"version": version, "today_me_ms": rep.Me, "today_agents_ms": rep.Agents,
		"project": project, "project_name": name, "activity": activity, "state": state,
		"session_start": sessionStart, "last_active": lastActive, "last_sample": d.lastSample()}, nil
}

// listed gives empty lists as [] instead of null, so the mod can iterate them as is.
func listed(tl Timeline) Timeline {
	tl.Power = nonNil(tl.Power)
	tl.NoData = nonNil(tl.NoData)
	tl.AgentsUnknown = nonNil(tl.AgentsUnknown)
	tl.Me = nonNil(tl.Me)
	tl.Leisure = nonNil(tl.Leisure)
	tl.Agents = nonNil(tl.Agents)
	return tl
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// getTimeline gives the day with closed periods' snapshots in place; ?fresh=1 shows it as the rules give it now.
func (d *Daemon) getTimeline(r *http.Request) (int, any, error) {
	day := r.URL.Query().Get("day")
	if day == "" {
		day = d.S.DayOf(nowMs())
	}
	from, to, err := dayRange(d.S, day)
	if err != nil {
		return 0, nil, fail(http.StatusBadRequest, "day: want YYYY-MM-DD, got %q", day)
	}
	var tl Timeline
	var div []Divergence
	if r.URL.Query().Get("fresh") == "1" {
		tl, err = d.fresh(from, to)
	} else {
		tl, div, err = d.timeline(from, to)
	}
	if err != nil {
		return 0, nil, err
	}
	rep := Summarize(tl, d.S, d.order())
	rep.Projects = nonNil(rep.Projects)
	ss, err := d.sessions(tl.Me, nowMs())
	if err != nil {
		return 0, nil, err
	}
	// Over the day's own timeline: a session from before the day starts at day_start, one after it
	// (a later mark; the sweep runs to the real now) is not the day's, and none runs past its end.
	ss = slices.DeleteFunc(ss, func(s Session) bool { return s.Start >= to })
	for i := range ss {
		ss[i].Start = max(ss[i].Start, from)
		if ss[i].End > to {
			ss[i].End, ss[i].Open = to, false
		}
	}
	return http.StatusOK, map[string]any{"day": day, "from": from, "to": to, "timeline": listed(tl),
		"report": rep, "divergences": nonNil(div), "sessions": nonNil(ss),
		"stats": dayStats(tl.Me, ss, int64(d.S.PauseMinutes)*60_000), "last_sample": d.lastSample()}, nil
}

// sessions are the sessions in me (sorted by start), each described. Callers hold mu.
func (d *Daemon) sessions(me []Segment, now int64) ([]Session, error) {
	marks, err := d.Store.SessionMarks()
	if err != nil {
		return nil, err
	}
	slices.SortStableFunc(marks, func(a, b SessionMark) int {
		if a.At != b.At {
			return cmp.Compare(a.At, b.At)
		}
		return cmp.Compare(a.ID, b.ID)
	})
	out := Sessions(me, marks, int64(d.S.SessionIdleMinutes)*60_000, now)
	for i := range out {
		out[i] = describe(out[i], me, d.S, now)
	}
	return out, nil
}

// getSession gives the latest session of the last 24 hours with its report.
// ponytail: the look-back is 24 hours, so a longer session is cut at it; widen it if one ever runs that long.
func (d *Daemon) getSession(r *http.Request) (int, any, error) {
	now := nowMs()
	tl, _, err := d.timeline(now-24*60*60_000, now)
	if err != nil {
		return 0, nil, err
	}
	ss, err := d.sessions(tl.Me, now)
	if err != nil {
		return 0, nil, err
	}
	var cur *Session
	rep := Report{Projects: []ProjectTotal{}}
	if n := len(ss); n > 0 {
		cur = &ss[n-1]
		to := cur.End
		if cur.Open {
			to = now
		}
		rep = Summarize(clipTimeline(tl, cur.Start, to), d.S, d.order())
		rep.Projects = nonNil(rep.Projects)
	}
	return http.StatusOK, map[string]any{"session": cur, "timeline": listed(tl), "report": rep,
		"last_sample": d.lastSample(), "session_idle_minutes": d.S.SessionIdleMinutes}, nil
}

func (d *Daemon) postSession(r *http.Request) (int, any, error) {
	var req struct {
		Action string `json:"action"`
	}
	if err := decode(r, &req); err != nil {
		return 0, nil, err
	}
	if req.Action != "start" && req.Action != "end" {
		return 0, nil, fail(http.StatusBadRequest, "action: want start or end")
	}
	now := nowMs()
	tl, _, err := d.timeline(now-24*60*60_000, now)
	if err != nil {
		return 0, nil, err
	}
	ss, err := d.sessions(tl.Me, now)
	if err != nil {
		return 0, nil, err
	}
	open := len(ss) > 0 && ss[len(ss)-1].Open
	if open && req.Action == "start" {
		return 0, nil, fail(http.StatusConflict, "session already open")
	}
	if !open && req.Action == "end" {
		return 0, nil, fail(http.StatusConflict, "no open session")
	}
	m, err := d.Store.AddSessionMark(SessionMark{Kind: req.Action, At: now})
	return http.StatusCreated, m, err
}

func (d *Daemon) getReport(r *http.Request) (int, any, error) {
	from, to, err := rangeParams(r)
	if err != nil {
		return 0, nil, err
	}
	tl, div, err := d.timeline(from, to)
	if err != nil {
		return 0, nil, err
	}
	rep := Summarize(tl, d.S, d.order())
	if project := r.URL.Query().Get("project"); project != "" {
		rep.Projects = slices.DeleteFunc(rep.Projects, func(p ProjectTotal) bool { return p.Project != project })
		div = slices.DeleteFunc(div, func(v Divergence) bool { return v.Project != project })
	}
	rep.Projects = nonNil(rep.Projects)
	return http.StatusOK, struct {
		Report
		Divergences []Divergence `json:"divergences"`
	}{rep, nonNil(div)}, nil
}

func (d *Daemon) getCorrections(r *http.Request) (int, any, error) {
	cs, err := d.Store.Corrections()
	return http.StatusOK, cs, err
}

func (d *Daemon) postCorrection(r *http.Request) (int, any, error) {
	var c Correction
	if err := decode(r, &c); err != nil {
		return 0, nil, err
	}
	if c.Target == "" {
		c.Target = "me"
	}
	now := nowMs()
	switch {
	case !slices.Contains([]string{"project", "activity", "exclude", "add", "note"}, c.Kind):
		return 0, nil, fail(http.StatusBadRequest, "kind: want project, activity, exclude, add or note")
	case !slices.Contains([]string{"me", "agents", "both"}, c.Target):
		return 0, nil, fail(http.StatusBadRequest, "target: want me, agents or both")
	case c.Kind != "project" && c.Kind != "exclude" && c.Target != "me": // activity, notes and added time are only mine
		return 0, nil, fail(http.StatusBadRequest, "target")
	case c.Start >= c.End:
		return 0, nil, fail(http.StatusBadRequest, "start must be before end")
	case c.Start >= now: // time that has not happened yet is no one's to mark
		return 0, nil, fail(http.StatusBadRequest, "start must be in the past")
	case c.Project != "" && d.project(c.Project) == nil:
		return 0, nil, fail(http.StatusBadRequest, "unknown project %q", c.Project)
	}
	if err := d.checkActivity(c.Activity); err != nil {
		return 0, nil, err
	}
	c.End = min(c.End, now)
	periods, err := d.Store.Periods()
	if err != nil {
		return 0, nil, err
	}
	if p := closedBy(periods, "", c.Start, c.End); p != nil {
		return 0, nil, closedPeriod(p.ID)
	}
	was := map[string]int64{}
	if c.End-c.Start <= 62*24*60*60_000 { // a longer one would hold the lock over a long timeline
		tl, _, err := d.timeline(c.Start, c.End)
		if err != nil {
			return 0, nil, err
		}
		for _, s := range tl.Me {
			was[s.Project] += s.End - s.Start
		}
	}
	c.Was = nil
	for p, ms := range was {
		if ms > 0 {
			c.Was = append(c.Was, Share{p, ms})
		}
	}
	slices.SortFunc(c.Was, func(a, b Share) int { return cmp.Or(cmp.Compare(b.Me, a.Me), cmp.Compare(a.Project, b.Project)) })
	c.ID, c.Made = 0, now
	created, err := d.Store.AddCorrection(c)
	return http.StatusCreated, created, err
}

func (d *Daemon) deleteCorrection(r *http.Request) (int, any, error) {
	id, err := intParam(r.PathValue("id"), "id")
	if err != nil {
		return 0, nil, err
	}
	cs, err := d.Store.Corrections()
	if err != nil {
		return 0, nil, err
	}
	i := slices.IndexFunc(cs, func(c Correction) bool { return c.ID == id })
	if i < 0 {
		return 0, nil, fail(http.StatusNotFound, "no correction %d", id)
	}
	periods, err := d.Store.Periods()
	if err != nil {
		return 0, nil, err
	}
	if p := closedBy(periods, "", cs[i].Start, cs[i].End); p != nil {
		return 0, nil, closedPeriod(p.ID)
	}
	return http.StatusNoContent, nil, d.Store.RemoveCorrection(id)
}

func (d *Daemon) postQuit(r *http.Request) (int, any, error) {
	d.stop()
	return http.StatusNoContent, nil, nil
}
