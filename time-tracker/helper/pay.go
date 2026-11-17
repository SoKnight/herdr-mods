package main

import (
	"net/http"
	"slices"
	"time"
	"unicode/utf8"
)

// Pay is one version of a project's pay; it applies from the month From until the next version.
type Pay struct {
	From   string `json:"from"`   // YYYY-MM, the first month it applies
	Kind   string `json:"kind"`   // fixed | hourly | none
	Amount int64  `json:"amount"` // ₽ a month (fixed) or an hour (hourly)
}

func (p Pay) start() string { return p.From }
func (p Pay) ended() bool   { return p.Kind == "none" }

// Bonus is a sum paid on top of a project's pay for a month.
type Bonus struct {
	ID      int64  `json:"id"`
	Project string `json:"project"`
	Month   string `json:"month"` // YYYY-MM
	Day     int    `json:"day"`   // of that month
	Title   string `json:"title"`
	Amount  int64  `json:"amount"`
	Made    int64  `json:"made"`
}

// payment is a project's pay for a month. Money is whole rubles, each amount rounded half up once.
type payment struct {
	Kind     string  `json:"kind"`   // fixed | hourly | "" (bonuses only)
	Amount   int64   `json:"amount"` // as set
	Minutes  int64   `json:"minutes"`
	Plan     int64   `json:"plan"` // the month's plan minutes
	Base     int64   `json:"base"` // fixed: Amount; hourly: Minutes × Amount / 60, half up
	Rate     int64   `json:"rate"` // ₽/ч: hourly: Amount; fixed: Amount × 60 / Plan, half up; 0 without a plan
	Bonuses  []Bonus `json:"bonuses"`
	BonusSum int64   `json:"bonus_sum"`
	Total    int64   `json:"total"`
}

// payOn is the version in force in month; nil when none or Kind none.
func payOn(pays []Pay, month string) *Pay { return versionOn(pays, month) }
func setPay(pays []Pay, p Pay) []Pay      { return setVersion(pays, p) }

// halfUp is a / b rounded half up, for a ≥ 0 and b > 0.
func halfUp(a, b int64) int64 { return (2*a + b) / (2 * b) }

// monthPay is the payment for my minutes of a month with plan minutes; nil when p is nil and there are no bonuses.
func monthPay(p *Pay, minutes, plan int64, bonuses []Bonus) *payment {
	if p == nil && len(bonuses) == 0 {
		return nil
	}
	out := &payment{Minutes: minutes, Plan: plan, Bonuses: nonNil(bonuses)}
	if p != nil {
		out.Kind, out.Amount = p.Kind, p.Amount
		switch p.Kind {
		case "fixed":
			out.Base = p.Amount
			if plan > 0 {
				out.Rate = halfUp(p.Amount*60, plan)
			}
		case "hourly":
			out.Base, out.Rate = halfUp(minutes*p.Amount, 60), p.Amount
		}
	}
	for _, b := range bonuses {
		out.BonusSum += b.Amount
	}
	out.Total = out.Base + out.BonusSum
	return out
}

const maxMoney = 100_000_000

// checkPay validates p; a none keeps only From.
func checkPay(p Pay) (Pay, error) {
	if _, err := time.Parse("2006-01", p.From); err != nil {
		return p, fail(http.StatusBadRequest, "from: want YYYY-MM")
	}
	switch p.Kind {
	case "none":
		return Pay{From: p.From, Kind: "none"}, nil
	case "fixed", "hourly":
	default:
		return p, fail(http.StatusBadRequest, "kind: want fixed, hourly or none")
	}
	if p.Amount < 1 || p.Amount > maxMoney {
		return p, fail(http.StatusBadRequest, "amount: want 1..100000000")
	}
	return p, nil
}

// putPay sets a project's pay from p.From on; earlier versions stay. A change is logged in history.json.
func (d *Daemon) putPay(r *http.Request) (int, any, error) {
	id := r.PathValue("id")
	var p Pay
	if err := decode(r, &p); err != nil {
		return 0, nil, err
	}
	var out Project
	ch := Change{Kind: "pay-set", Project: id}
	_, err := d.editConfig(func(c *Config) error {
		i := slices.IndexFunc(c.Projects, func(x Project) bool { return x.ID == id })
		if i < 0 {
			return fail(http.StatusNotFound, "no project %q", id)
		}
		p, err := checkPay(p)
		if err != nil {
			return err
		}
		ch.Pay, ch.PrevPays = &p, c.Projects[i].Pay
		c.Projects[i].Pay = setPay(c.Projects[i].Pay, p)
		ch.Pays, out = c.Projects[i].Pay, c.Projects[i]
		return nil
	})
	if err == nil && !slices.Equal(ch.PrevPays, ch.Pays) {
		d.logChange(ch)
	}
	return http.StatusOK, out, err
}

func (d *Daemon) postBonus(r *http.Request) (int, any, error) {
	var b Bonus
	if err := decode(r, &b); err != nil {
		return 0, nil, err
	}
	if d.project(b.Project) == nil {
		return 0, nil, fail(http.StatusBadRequest, "unknown project %q", b.Project)
	}
	month, err := time.Parse("2006-01", b.Month)
	if err != nil {
		return 0, nil, fail(http.StatusBadRequest, "month: want YYYY-MM")
	}
	if b.Day < 1 || b.Day > month.AddDate(0, 1, -1).Day() {
		return 0, nil, fail(http.StatusBadRequest, "day: want a day of that month")
	}
	if n := utf8.RuneCountInString(b.Title); n < 1 || n > 200 {
		return 0, nil, fail(http.StatusBadRequest, "title: want 1..200 characters")
	}
	if b.Amount < 1 || b.Amount > maxMoney {
		return 0, nil, fail(http.StatusBadRequest, "amount: want 1..100000000")
	}
	b.ID, b.Made = 0, nowMs()
	created, err := d.Store.AddBonus(b)
	return http.StatusCreated, created, err
}

func (d *Daemon) deleteBonus(r *http.Request) (int, any, error) {
	id, err := intParam(r.PathValue("id"), "id")
	if err != nil {
		return 0, nil, err
	}
	bs, err := d.Store.Bonuses()
	if err != nil {
		return 0, nil, err
	}
	if !slices.ContainsFunc(bs, func(b Bonus) bool { return b.ID == id }) {
		return 0, nil, fail(http.StatusNotFound, "no bonus %d", id)
	}
	return http.StatusNoContent, nil, d.Store.RemoveBonus(id)
}
