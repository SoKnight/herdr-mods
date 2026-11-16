package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Store struct {
	Dir string
	S   Settings
	mu  sync.Mutex
}

func (st *Store) loc() *time.Location {
	if st.S.Loc == nil {
		return time.Local
	}
	return st.S.Loc
}

func (st *Store) dayFile(ms int64) string {
	return filepath.Join(st.Dir, "raw", time.UnixMilli(ms).In(st.loc()).Format("2006-01-02")+".jsonl")
}

func appendLine(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	_, err = f.Write(append(b, '\n'))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// readJSON leaves v untouched when the file is missing.
func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}

func (st *Store) AppendSpan(sp Span) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	for {
		t := time.UnixMilli(sp.Start).In(st.loc())
		mid := time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, st.loc()).UnixMilli()
		part := sp
		if sp.End > mid {
			part.End = mid
		}
		if err := appendLine(st.dayFile(part.Start), part); err != nil {
			return err
		}
		if part.End == sp.End {
			return nil
		}
		sp.Start = mid
	}
}

func (st *Store) AppendBeat(b Heartbeat) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	return appendLine(st.dayFile(b.Time), struct {
		K string `json:"k"`
		Heartbeat
	}{"beat", b})
}

func (st *Store) Load(from, to int64) (Raw, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	var raw Raw
	first := time.UnixMilli(from).In(st.loc())
	last := time.UnixMilli(to).In(st.loc())
	for d := time.Date(first.Year(), first.Month(), first.Day()-1, 0, 0, 0, 0, st.loc()); !d.After(last); d = d.AddDate(0, 0, 1) {
		f, err := os.Open(st.dayFile(d.UnixMilli()))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return raw, err
		}
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<20)
		for sc.Scan() {
			var kind struct {
				K string `json:"k"`
			}
			line := sc.Bytes()
			if json.Unmarshal(line, &kind) != nil {
				continue // broken line
			}
			if kind.K == "beat" {
				var b Heartbeat
				if json.Unmarshal(line, &b) == nil && b.Time >= from && b.Time < to {
					raw.Beats = append(raw.Beats, b)
				}
				continue
			}
			var sp Span
			if json.Unmarshal(line, &sp) != nil {
				continue
			}
			sp.Start, sp.End = max(sp.Start, from), min(sp.End, to)
			if sp.Start < sp.End {
				raw.Spans = append(raw.Spans, sp)
			}
		}
		f.Close()
	}
	return raw, nil
}

type openFile struct {
	At    int64  `json:"at"`
	Spans []Span `json:"spans"`
}

func (st *Store) SaveOpen(spans []Span, at int64) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	return writeJSON(filepath.Join(st.Dir, "open.json"), openFile{at, spans})
}

func (st *Store) RecoverOpen() ([]Span, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	path := filepath.Join(st.Dir, "open.json")
	var o openFile
	if err := readJSON(path, &o); err != nil {
		return nil, err
	}
	for i := range o.Spans {
		o.Spans[i].End = o.At
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	return o.Spans, nil
}

// addItem appends to a JSON array file, assigning max(id)+1.
func addItem[T any](st *Store, file string, v T, id func(*T) *int64) (T, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	path := filepath.Join(st.Dir, file)
	var all []T
	if err := readJSON(path, &all); err != nil {
		return v, err
	}
	var next int64 = 1
	for i := range all {
		next = max(next, *id(&all[i])+1)
	}
	*id(&v) = next
	return v, writeJSON(path, append(all, v))
}

func removeItem[T any](st *Store, file string, want int64, id func(*T) *int64) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	path := filepath.Join(st.Dir, file)
	var all []T
	if err := readJSON(path, &all); err != nil {
		return err
	}
	kept := all[:0]
	for i := range all {
		if *id(&all[i]) != want {
			kept = append(kept, all[i])
		}
	}
	return writeJSON(path, kept)
}

func listItems[T any](st *Store, file string) ([]T, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	all := []T{}
	err := readJSON(filepath.Join(st.Dir, file), &all)
	return all, err
}

func corrID(c *Correction) *int64 { return &c.ID }
func perID(p *Period) *int64      { return &p.ID }

func (st *Store) Corrections() ([]Correction, error) {
	return listItems[Correction](st, "corrections.json")
}
func (st *Store) AddCorrection(c Correction) (Correction, error) {
	return addItem(st, "corrections.json", c, corrID)
}
func (st *Store) RemoveCorrection(id int64) error {
	return removeItem(st, "corrections.json", id, corrID)
}

func (st *Store) Periods() ([]Period, error) { return listItems[Period](st, "periods.json") }
func (st *Store) AddPeriod(p Period) (Period, error) {
	return addItem(st, "periods.json", p, perID)
}
func (st *Store) RemovePeriod(id int64) error { return removeItem(st, "periods.json", id, perID) }
