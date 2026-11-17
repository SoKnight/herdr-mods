package main

import (
	"cmp"
	"net/http"
	"slices"
)

// Change is a rule change in history.json, kept so it can be undone.
type Change struct {
	ID     int64  `json:"id"`
	At     int64  `json:"at"`
	Kind   string `json:"kind"`             // rule-add | rule-edit | rule-delete
	Rule   *Rule  `json:"rule"`             // the rule as the change left it; nil when a delete removed it
	Prev   *Rule  `json:"prev"`             // the rule before it; nil for an add
	Index  int    `json:"index"`            // Prev's place in the list, to put a removed rule back
	From   int64  `json:"from"`             // the time it applies from
	Ms     int64  `json:"ms"`               // my time and Досуг the rule had over [From, At) right after
	Source string `json:"source,omitempty"` // "inbox" for rules made in Разбор
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
func (d *Daemon) logChange(c Change) error {
	c.At = nowMs()
	if c.Rule != nil && c.Kind != "rule-delete" {
		ms, err := d.ruleTime([]int64{c.Rule.ID}, c.From, c.At)
		if err != nil {
			return err
		}
		c.Ms = ms[c.Rule.ID]
	}
	_, err := d.Store.AddChange(c)
	return err
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
		if c.Prev == nil || c.Rule.ID != c.Prev.ID {
			rules = slices.Delete(rules, i, i+1)
		}
	}
	if c.Prev != nil {
		if i := find(c.Prev.ID); i >= 0 {
			if c.Rule == nil {
				return nil, changed
			}
			rules[i] = *c.Prev
		} else {
			rules = slices.Insert(rules, min(max(c.Index, 0), len(rules)), *c.Prev)
		}
	}
	return rules, nil
}

type history struct {
	Corrections []Correction `json:"corrections"`
	Changes     []Change     `json:"changes"`
}

// getHistory lists corrections by when they were made (an old one by its start) and rule changes, newest first.
func (d *Daemon) getHistory(r *http.Request) (int, any, error) {
	from, to, err := rangeParams(r)
	if err != nil {
		return 0, nil, err
	}
	in := func(t int64) bool { return from <= t && t < to }
	made := func(c Correction) int64 { return cmp.Or(c.Made, c.Start) }
	out := history{Corrections: []Correction{}, Changes: []Change{}}
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
		if in(c.At) {
			out.Changes = append(out.Changes, c)
		}
	}
	return http.StatusOK, out, nil
}

// postUndo undoes a rule change and drops it from the history.
func (d *Daemon) postUndo(r *http.Request) (int, any, error) {
	id, err := intParam(r.PathValue("id"), "id")
	if err != nil {
		return 0, nil, err
	}
	changes, err := d.Store.Changes()
	if err != nil {
		return 0, nil, err
	}
	i := slices.IndexFunc(changes, func(c Change) bool { return c.ID == id })
	if i < 0 {
		return 0, nil, fail(http.StatusNotFound, "no change %d", id)
	}
	if _, err := d.editConfig(func(c *Config) error {
		rules, err := undoChange(c.Rules, changes[i])
		c.Rules = rules
		return err
	}); err != nil {
		return 0, nil, err
	}
	return http.StatusNoContent, nil, d.Store.RemoveChange(id)
}
