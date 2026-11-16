package main

import "testing"

func smp(at, last int64, exe, title string) Sample {
	return Sample{At: at, LastInput: last, Exe: exe, Title: title}
}

func feed(tr *Tracker, ss ...Sample) (out []Span) {
	for _, s := range ss {
		out = append(out, tr.Feed(s)...)
	}
	return
}

func find(sp []Span, kind string) Span {
	for _, s := range sp {
		if s.Kind == kind {
			return s
		}
	}
	return Span{}
}

func count(sp []Span, kind string) (n int) {
	for _, s := range sp {
		if s.Kind == kind {
			n++
		}
	}
	return
}

func TestTrackerSleepGapClosesSpans(t *testing.T) {
	tr := &Tracker{PollMs: 5000}
	feed(tr, smp(0, 0, "idea64.exe", "A"), smp(5000, 4000, "idea64.exe", "A"))
	closed := tr.Feed(smp(3_600_000, 3_599_000, "idea64.exe", "A")) // woke an hour later
	assertEq(t, find(closed, "window").End, int64(5000))
	assertEq(t, find(closed, "power").End, int64(5000))
	assertEq(t, find(tr.Open(3_600_000), "window").Start, int64(3_600_000))
}

func TestTrackerInputRuns(t *testing.T) {
	tr := &Tracker{PollMs: 5000}
	closed := feed(tr,
		smp(0, -100_000, "a.exe", "A"),
		smp(5000, 4000, "a.exe", "A"),
		smp(10000, 9000, "a.exe", "A"),
		smp(15000, 9000, "a.exe", "A"),
		smp(20000, 9000, "a.exe", "A"),
		smp(25000, 9000, "a.exe", "A"),
		smp(30000, 29000, "a.exe", "A"),
	)
	assertEq(t, count(closed, "input"), 1)
	in := find(closed, "input")
	assertEq(t, in.Start, int64(4000))
	assertEq(t, in.End, int64(10000))
	assertEq(t, find(tr.Open(30000), "input").Start, int64(29000))
}

func TestTrackerWindowChange(t *testing.T) {
	tr := &Tracker{PollMs: 5000}
	closed := feed(tr,
		smp(0, 0, "a.exe", "A"), smp(5000, 0, "a.exe", "A"),
		smp(10000, 0, "a.exe", "B"))
	w := find(closed, "window")
	assertEq(t, w.Title, "A")
	assertEq(t, w.Start, int64(0))
	assertEq(t, w.End, int64(10000))
	assertEq(t, find(tr.Open(10000), "window").Title, "B")
}

func TestTrackerCallFromMicUsers(t *testing.T) {
	tr := &Tracker{PollMs: 5000, CallApps: []string{"Telemost.exe"}}
	var closed []Span
	for at := int64(0); at <= 25000; at += 5000 {
		s := smp(at, 0, "a.exe", "A")
		if at < 20000 {
			s.CallExes = []string{"telemost.exe", "other.exe"}
		}
		closed = append(closed, tr.Feed(s)...)
	}
	c := find(closed, "call")
	assertEq(t, c.Start, int64(0))
	assertEq(t, c.End, int64(20000))
	assertEq(t, c.Exe, "Telemost.exe")
}

func TestTrackerLock(t *testing.T) {
	tr := &Tracker{PollMs: 5000}
	var closed []Span
	for at := int64(0); at <= 25000; at += 5000 {
		s := smp(at, 0, "a.exe", "A")
		s.Locked = at >= 10000 && at < 20000
		closed = append(closed, tr.Feed(s)...)
	}
	l := find(closed, "lock")
	assertEq(t, l.Start, int64(10000))
	assertEq(t, l.End, int64(20000))
}

func TestTrackerCloseAll(t *testing.T) {
	tr := &Tracker{PollMs: 5000}
	feed(tr, smp(0, 0, "a.exe", "A"))
	assertEq(t, len(tr.Close(5000)) > 0, true)
	assertEq(t, len(tr.Open(6000)), 0)
}

func TestTrackerOpenStopsAtTheLastPollAfterSleep(t *testing.T) {
	tr := &Tracker{PollMs: 5000}
	feed(tr, smp(0, 0, "a.exe", "A"), smp(5000, 5000, "a.exe", "A"))
	assertEq(t, find(tr.Open(20_000), "window").End, int64(20_000)) // three polls: still awake
	assertEq(t, find(tr.Open(3_600_000), "window").End, int64(5000))
}
