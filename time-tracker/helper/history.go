package main

import (
	"cmp"
	"log"
	"net/http"
	"slices"
)

// Change is a rule, goal or pay change in history.json, kept so it can be undone.
type Change struct {
	ID     int64  `json:"id"`
	At     int64  `json:"at"`
	Kind   string `json:"kind"`             // rule-add | rule-edit | rule-delete | goal-set | pay-set
	Rule   *Rule  `json:"rule"`             // the rule as the change left it; nil when a delete removed it
	Prev   *Rule  `json:"prev"`             // the rule before it; nil for an add
	Index  int    `json:"index"`            // Prev's place in the list, to put a removed rule back
	Next   int64  `json:"next,omitempty"`   // the rule that followed Prev, to put it back before it
	From   int64  `json:"from"`             // the time it applies from
	Ms     int64  `json:"ms"`               // my time and Досуг the rule had over [From, At) right after
	Source string `json:"source,omitempty"` // "inbox" for rules made in Разбор
	Undone int64  `json:"undone,omitempty"` // when it was undone; kept so its ids are never handed out again

	Project   string `json:"project,omitempty"`    // goal-set, pay-set: whose goal or pay
	Goal      *Goal  `json:"goal,omitempty"`       // the version sent
	Goals     []Goal `json:"goals,omitempty"`      // the project's versions as the change left them
	PrevGoals []Goal `json:"prev_goals,omitempty"` // and before it
	Pay       *Pay   `json:"pay,omitempty"`        // pay-set, as goal-set
	Pays      []Pay  `json:"pays,omitempty"`
	PrevPays  []Pay  `json:"prev_pays,omitempty"`
}

// sameGoals tells whether two lists of versions are equal; nil and empty are.
func sameGoals(a, b []Goal) bool {
	return slices.EqualFunc(a, b, func(x, y Goal) bool {
		return x.From == y.From && x.Dir == y.Dir && x.Minutes == y.Minutes && x.Period == y.Period &&
			slices.Equal(x.Weekdays, y.Weekdays) && x.Calendar == y.Calendar && x.Weekends == y.Weekends && x.Off == y.Off
	})
}

// undoVersions puts a project's goals (goal-set) or pay (pay-set) back as they were before ch,
// unless they changed since.
func undoVersions(c *Config, ch Change) error {
	i := slices.IndexFunc(c.Projects, func(p Project) bool { return p.ID == ch.Project })
	if i < 0 {
		return fail(http.StatusNotFound, "no project %q", ch.Project)
	}
	p := &c.Projects[i]
	if ch.Kind == "pay-set" {
		if !slices.Equal(p.Pay, ch.Pays) {
			return fail(http.StatusConflict, "оплата изменилась после этой правки")
		}
		p.Pay = ch.PrevPays
		return nil
	}
	if !sameGoals(p.Goals, ch.Goals) {
		return fail(http.StatusConflict, "цель изменилась после этой правки")
	}
	p.Goals = ch.PrevGoals
	return nil
}

// nextAfter is the id of the rule after rules[i], 0 for the last.
func nextAfter(rules []Rule, i int) int64 {
	if i+1 < len(rules) {
		return rules[i+1].ID
	}
	return 0
}

// ruleTime sums my time and Досуг over [from, to) by the rule that matched it.
func (d *Daemon) ruleTime(ids []int64, from, to int64) (map[int64]int64, error) {
	tl, err := d.fresh(from, to)
	if err != nil {
		return nil, err
	}
	out := map[int64]int64{}
	for _, s := range slices.Concat(tl.Me, tl.Leisure) {
		if s.Rule != 0 && slices.Contains(ids, s.Rule) {
			out[s.Rule] += s.End - s.Start
		}
	}
	return out, nil
}

// logChange stores a rule change made now; an add or edit gets the time its rule has from c.From on.
// The change is already in projects.json, so a failure here is only logged.
func (d *Daemon) logChange(c Change) {
	c.At = nowMs()
	if c.Rule != nil && c.Kind != "rule-delete" {
		ms, err := d.ruleTime([]int64{c.Rule.ID}, c.From, c.At)
		if err != nil {
			log.Printf("history: %s #%d: %v", c.Kind, c.Rule.ID, err)
		}
		c.Ms = ms[c.Rule.ID]
	}
	if _, err := d.Store.AddChange(c); err != nil {
		log.Printf("history: %s: %v", c.Kind, err)
	}
}

// undoChange puts the rules back as they were before c. It refuses when the rule c left changed since,
// or when a rule c removed would come back over another under its id: undo never restores over newer state.
func undoChange(rules []Rule, c Change) ([]Rule, error) {
	rules = slices.Clone(rules)
	find := func(id int64) int { return slices.IndexFunc(rules, func(r Rule) bool { return r.ID == id }) }
	changed := fail(http.StatusConflict, "правило изменилось после этой правки")
	if c.Rule != nil {
		i := find(c.Rule.ID)
		if i < 0 || rules[i] != *c.Rule {
			return nil, changed
		}
		rules = slices.Delete(rules, i, i+1) // under Prev's id it comes back below, where it was
	}
	if c.Prev == nil {
		return rules, nil
	}
	if i := find(c.Prev.ID); i >= 0 { // the version an edit closed: reopened where it stands
		if c.Rule == nil {
			return nil, changed
		}
		rules[i] = *c.Prev
		return rules, nil
	}
	at := min(max(c.Index, 0), len(rules))
	if j := find(c.Next); j >= 0 {
		at = j
	}
	return slices.Insert(rules, at, *c.Prev), nil
}

type history struct {
	Corrections []Correction `json:"corrections"`
	Changes     []Change     `json:"changes"`
	DaysOff     []DayOff     `json:"days_off"`
}

// getHistory lists corrections by when they were made (an old one by its start), rule changes and days off,
// newest first.
func (d *Daemon) getHistory(r *http.Request) (int, any, error) {
	from, to, err := rangeParams(r)
	if err != nil {
		return 0, nil, err
	}
	in := func(t int64) bool { return from <= t && t < to }
	made := func(c Correction) int64 { return cmp.Or(c.Made, c.Start) }
	out := history{Corrections: []Correction{}, Changes: []Change{}, DaysOff: []DayOff{}}
	cs, err := d.Store.Corrections()
	if err != nil {
		return 0, nil, err
	}
	for _, c := range cs {
		if in(made(c)) {
			out.Corrections = append(out.Corrections, c)
		}
	}
	slices.SortStableFunc(out.Corrections, func(a, b Correction) int {
		return cmp.Or(cmp.Compare(made(b), made(a)), cmp.Compare(b.ID, a.ID))
	})
	changes, err := d.Store.Changes()
	if err != nil {
		return 0, nil, err
	}
	for _, c := range changes {
		if c.Undone == 0 && in(c.At) {
			out.Changes = append(out.Changes, c)
		}
	}
	offs, err := d.Store.DaysOff()
	if err != nil {
		return 0, nil, err
	}
	for _, o := range offs {
		if in(o.Made) {
			out.DaysOff = append(out.DaysOff, o)
		}
	}
	slices.SortStableFunc(out.DaysOff, func(a, b DayOff) int { return cmp.Or(cmp.Compare(b.Made, a.Made), cmp.Compare(b.ID, a.ID)) })
	return http.StatusOK, out, nil
}

// postUndo undoes a rule, goal or pay change; it stays in history.json marked undone and leaves the list.
func (d *Daemon) postUndo(r *http.Request) (int, any, error) {
	id, err := intParam(r.PathValue("id"), "id")
	if err != nil {
		return 0, nil, err
	}
	changes, err := d.Store.Changes()
	if err != nil {
		return 0, nil, err
	}
	i := slices.IndexFunc(changes, func(c Change) bool { return c.ID == id && c.Undone == 0 })
	if i < 0 {
		return 0, nil, fail(http.StatusNotFound, "no change %d", id)
	}
	if _, err := d.editConfig(func(c *Config) error {
		if k := changes[i].Kind; k == "goal-set" || k == "pay-set" {
			return undoVersions(c, changes[i])
		}
		rules, err := undoChange(c.Rules, changes[i])
		c.Rules = rules
		return err
	}); err != nil {
		return 0, nil, err
	}
	return http.StatusNoContent, nil, d.Store.MarkUndone(id, nowMs())
}
