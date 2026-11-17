package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
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
	return writeFile(path, b)
}

// writeFile replaces path with b through a synced temp file, so a crash leaves the old or the new file.
func writeFile(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync() // a crash after the rename must not leave an empty file
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
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
		err = eachLine(f, func(line []byte) {
			var kind struct {
				K string `json:"k"`
			}
			if json.Unmarshal(line, &kind) != nil {
				return // broken line
			}
			if kind.K == "beat" {
				var b Heartbeat
				if json.Unmarshal(line, &b) == nil && b.Time >= from && b.Time < to {
					raw.Beats = append(raw.Beats, b)
				}
				return
			}
			var sp Span
			if json.Unmarshal(line, &sp) != nil {
				return
			}
			sp.Start, sp.End = max(sp.Start, from), min(sp.End, to)
			if sp.Start < sp.End {
				raw.Spans = append(raw.Spans, sp)
			}
		})
		f.Close()
		if err != nil {
			return raw, err
		}
	}
	return raw, nil
}

// eachLine calls fn with every line up to 1 MiB; a longer one is skipped, not the rest of the file.
func eachLine(r io.Reader, fn func(line []byte)) error {
	br := bufio.NewReaderSize(r, 1<<20)
	for {
		line, err := br.ReadSlice('\n')
		long := false
		for errors.Is(err, bufio.ErrBufferFull) {
			long = true
			_, err = br.ReadSlice('\n')
		}
		if !long && len(line) > 0 {
			fn(line)
		}
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
	}
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

// RecoverOpen reads what the last run left open; ClearOpen drops it once it is stored. A file that
// does not decode moves to open.json.bad, so it cannot stop every start after it.
func (st *Store) RecoverOpen() ([]Span, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	path := filepath.Join(st.Dir, "open.json")
	var o openFile
	if err := readJSON(path, &o); err != nil {
		var syn *json.SyntaxError
		var typ *json.UnmarshalTypeError
		if !errors.As(err, &syn) && !errors.As(err, &typ) {
			return nil, err
		}
		log.Printf("open.json: %v; moved to open.json.bad", err)
		return nil, os.Rename(path, path+".bad")
	}
	for i := range o.Spans {
		o.Spans[i].End = o.At
	}
	return o.Spans, nil
}

func (st *Store) ClearOpen() error {
	st.mu.Lock()
	defer st.mu.Unlock()
	if err := os.Remove(filepath.Join(st.Dir, "open.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// addItem appends to a JSON array file, assigning an id above every one it ever held: max(id)+1, or past
// the highest removed id that removeItem keeps in <file>.maxid, so a stale id never names a new item.
func addItem[T any](st *Store, file string, v T, id func(*T) *int64) (T, error) {
	vs, err := addItems(st, file, []T{v}, id)
	return vs[0], err
}

// addItems appends vs in one write, so they are stored all or none, each given the next free id.
func addItems[T any](st *Store, file string, vs []T, id func(*T) *int64) ([]T, error) {
	st.mu.Lock()
	defer st.mu.Unlock()
	path := filepath.Join(st.Dir, file)
	var all []T
	if err := readJSON(path, &all); err != nil {
		return vs, err
	}
	var removed int64 // the highest id removeItem took out; missing for a file that never lost one
	if err := readJSON(path+".maxid", &removed); err != nil {
		return vs, err
	}
	next := removed + 1
	for i := range all {
		next = max(next, *id(&all[i])+1)
	}
	vs = slices.Clone(vs)
	for i := range vs {
		*id(&vs[i]) = next + int64(i)
	}
	return vs, writeJSON(path, append(all, vs...))
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
	if len(kept) == len(all) {
		return nil
	}
	// The high-water mark goes first: a crash before the file is rewritten only skips an id.
	var removed int64
	if err := readJSON(path+".maxid", &removed); err != nil {
		return err
	}
	if err := writeJSON(path+".maxid", max(removed, want)); err != nil {
		return err
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

func corrID(c *Correction) *int64  { return &c.ID }
func perID(p *Period) *int64       { return &p.ID }
func markID(m *SessionMark) *int64 { return &m.ID }
func changeID(c *Change) *int64    { return &c.ID }
func dayOffID(o *DayOff) *int64    { return &o.ID }

func (st *Store) DaysOff() ([]DayOff, error) { return listItems[DayOff](st, "daysoff.json") }
func (st *Store) AddDayOff(o DayOff) (DayOff, error) {
	return addItem(st, "daysoff.json", o, dayOffID)
}
func (st *Store) RemoveDayOff(id int64) error { return removeItem(st, "daysoff.json", id, dayOffID) }

func (st *Store) SessionMarks() ([]SessionMark, error) {
	return listItems[SessionMark](st, "sessions.json")
}
func (st *Store) AddSessionMark(m SessionMark) (SessionMark, error) {
	return addItem(st, "sessions.json", m, markID)
}

func (st *Store) Corrections() ([]Correction, error) {
	return listItems[Correction](st, "corrections.json")
}
func (st *Store) AddCorrection(c Correction) (Correction, error) {
	return addItem(st, "corrections.json", c, corrID)
}
func (st *Store) AddCorrections(cs []Correction) ([]Correction, error) {
	return addItems(st, "corrections.json", cs, corrID)
}
func (st *Store) RemoveCorrection(id int64) error {
	return removeItem(st, "corrections.json", id, corrID)
}

// Changes lists the rule changes newest first, undone ones included.
func (st *Store) Changes() ([]Change, error) {
	cs, err := listItems[Change](st, "history.json")
	slices.Reverse(cs)
	return cs, err
}
func (st *Store) AddChange(c Change) (Change, error) {
	return addItem(st, "history.json", c, changeID)
}

// MarkUndone marks a change undone at at; it stays, so neither its id nor its rules' ids come back.
func (st *Store) MarkUndone(id, at int64) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	path := filepath.Join(st.Dir, "history.json")
	var all []Change
	if err := readJSON(path, &all); err != nil {
		return err
	}
	for i := range all {
		if all[i].ID == id {
			all[i].Undone = at
		}
	}
	return writeJSON(path, all)
}

func (st *Store) Periods() ([]Period, error) { return listItems[Period](st, "periods.json") }
func (st *Store) AddPeriod(p Period) (Period, error) {
	return addItem(st, "periods.json", p, perID)
}
func (st *Store) RemovePeriod(id int64) error { return removeItem(st, "periods.json", id, perID) }
