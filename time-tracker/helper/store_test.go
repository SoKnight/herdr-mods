package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

var msk = time.FixedZone("MSK", 3*3600)

func mskAt(y, mo, d, h, mi int) int64 {
	return time.Date(y, time.Month(mo), d, h, mi, 0, 0, msk).UnixMilli()
}

func tempStore(t *testing.T) *Store {
	s := DefaultSettings()
	s.Loc = msk
	return &Store{Dir: t.TempDir(), S: s}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func mustLoad(t *testing.T, st *Store, from, to int64) Raw {
	t.Helper()
	raw, err := st.Load(from, to)
	must(t, err)
	return raw
}

func fileExists(t *testing.T, dir, rel string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
		t.Fatal(err)
	}
}

func TestAppendSplitsAtMidnight(t *testing.T) {
	st := tempStore(t)
	start, end := mskAt(2026, 11, 16, 23, 0), mskAt(2026, 11, 17, 1, 0)
	must(t, st.AppendSpan(Span{Kind: "window", Start: start, End: end, Exe: "idea64.exe"}))
	raw := mustLoad(t, st, start, end)
	assertEq(t, len(raw.Spans), 2)
	assertEq(t, raw.Spans[0].End, mskAt(2026, 11, 17, 0, 0))
	assertEq(t, raw.Spans[1].Start, mskAt(2026, 11, 17, 0, 0))
	fileExists(t, st.Dir, "raw/2026-11-16.jsonl")
	fileExists(t, st.Dir, "raw/2026-11-17.jsonl")
}

func TestLoadClipsToRange(t *testing.T) {
	st := tempStore(t)
	must(t, st.AppendSpan(Span{Kind: "window", Start: mskAt(2026, 11, 16, 10, 0), End: mskAt(2026, 11, 16, 12, 0)}))
	raw := mustLoad(t, st, mskAt(2026, 11, 16, 11, 0), mskAt(2026, 11, 16, 13, 0))
	assertEq(t, len(raw.Spans), 1)
	assertEq(t, raw.Spans[0].Start, mskAt(2026, 11, 16, 11, 0))
	assertEq(t, raw.Spans[0].End, mskAt(2026, 11, 16, 12, 0))
}

func TestLoadSkipsBrokenLine(t *testing.T) {
	st := tempStore(t)
	s, e := mskAt(2026, 11, 16, 10, 0), mskAt(2026, 11, 16, 11, 0)
	must(t, st.AppendSpan(Span{Kind: "window", Start: s, End: e}))
	f, err := os.OpenFile(filepath.Join(st.Dir, "raw", "2026-11-16.jsonl"), os.O_APPEND|os.O_WRONLY, 0o644)
	must(t, err)
	_, err = f.WriteString("{\"k\":\"window\",\"s\":1\n")
	must(t, err)
	must(t, f.Close())
	raw := mustLoad(t, st, s, e)
	assertEq(t, len(raw.Spans), 1)
}

func TestLoadSkipsOverlongLineOnly(t *testing.T) {
	st := tempStore(t)
	s := mskAt(2026, 11, 16, 10, 0)
	must(t, st.AppendSpan(Span{Kind: "window", Start: s, End: s + 1000}))
	must(t, st.AppendBeat(Heartbeat{Source: "x", Time: s, Entity: strings.Repeat("a", 2<<20)}))
	must(t, st.AppendSpan(Span{Kind: "input", Start: s, End: s + 1000}))
	raw := mustLoad(t, st, s, s+1000)
	assertEq(t, len(raw.Beats), 0)
	assertEq(t, len(raw.Spans), 2) // the line after the long one is still read
	assertEq(t, raw.Spans[1].Kind, "input")
}

func TestBeatRoundTrip(t *testing.T) {
	st := tempStore(t)
	at := mskAt(2026, 11, 16, 10, 0)
	must(t, st.AppendBeat(Heartbeat{Source: "ide", Time: at, ProjectName: "p", Focused: true}))
	raw := mustLoad(t, st, at-1000, at+1000)
	assertEq(t, len(raw.Beats), 1)
	assertEq(t, raw.Beats[0].ProjectName, "p")
	assertEq(t, len(raw.Spans), 0)
}

func TestRecoverOpen(t *testing.T) {
	st := tempStore(t)
	must(t, st.SaveOpen([]Span{{Kind: "window", Start: mskAt(2026, 11, 16, 10, 0)}}, mskAt(2026, 11, 16, 10, 30)))
	sp, err := st.RecoverOpen()
	must(t, err)
	assertEq(t, len(sp), 1)
	assertEq(t, sp[0].End, mskAt(2026, 11, 16, 10, 30))
	sp, err = st.RecoverOpen() // still there until it is cleared
	must(t, err)
	assertEq(t, len(sp), 1)
	must(t, st.ClearOpen())
	sp, err = st.RecoverOpen()
	must(t, err)
	assertEq(t, len(sp), 0)
}

func TestCorrectionsIdsAndRemove(t *testing.T) {
	st := tempStore(t)
	c1, err := st.AddCorrection(Correction{Kind: "note"})
	must(t, err)
	c2, err := st.AddCorrection(Correction{Kind: "exclude"})
	must(t, err)
	assertEq(t, c1.ID, int64(1))
	assertEq(t, c2.ID, int64(2))
	must(t, st.RemoveCorrection(1))
	cs, err := st.Corrections()
	must(t, err)
	assertEq(t, len(cs), 1)
	assertEq(t, cs[0].ID, int64(2))
	c3, err := st.AddCorrection(Correction{Kind: "add"})
	must(t, err)
	assertEq(t, c3.ID, int64(3))
}

// A removed id is never handed out again, so a stale «Отменить» cannot delete another correction.
func TestCorrectionIdsNotReused(t *testing.T) {
	st := tempStore(t)
	for range 2 {
		_, err := st.AddCorrection(Correction{Kind: "note"})
		must(t, err)
	}
	must(t, st.RemoveCorrection(2))
	c, err := st.AddCorrection(Correction{Kind: "note"})
	must(t, err)
	assertEq(t, c.ID, int64(3))
	must(t, st.RemoveCorrection(3))
	must(t, st.RemoveCorrection(1))
	cs, err := st.AddCorrections([]Correction{{Kind: "note"}, {Kind: "note"}})
	must(t, err)
	assertEq(t, fmt.Sprint(cs[0].ID, cs[1].ID), "4 5")

	old := tempStore(t) // a file from before the high-water mark: max+1
	must(t, writeJSON(filepath.Join(old.Dir, "corrections.json"), []Correction{{ID: 7, Kind: "note"}}))
	c, err = old.AddCorrection(Correction{Kind: "note"})
	must(t, err)
	assertEq(t, c.ID, int64(8))
}

func TestPeriodsAddRemove(t *testing.T) {
	st := tempStore(t)
	p, err := st.AddPeriod(Period{Project: "a", Me: []Segment{{Start: 1, End: 2}}})
	must(t, err)
	assertEq(t, p.ID, int64(1))
	ps, err := st.Periods()
	must(t, err)
	assertEq(t, len(ps), 1)
	assertEq(t, len(ps[0].Me), 1)
	must(t, st.RemovePeriod(1))
	ps, err = st.Periods()
	must(t, err)
	assertEq(t, len(ps), 0)
}

func TestSessionMarksIds(t *testing.T) {
	st := tempStore(t)
	a, err := st.AddSessionMark(SessionMark{Kind: "end", At: 5})
	must(t, err)
	b, err := st.AddSessionMark(SessionMark{Kind: "start", At: 6})
	must(t, err)
	assertEq(t, a.ID, int64(1))
	assertEq(t, b.ID, int64(2))
	all, err := st.SessionMarks()
	must(t, err)
	assertEq(t, len(all), 2)
	fileExists(t, st.Dir, "sessions.json")
}
