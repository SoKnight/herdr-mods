package main

import "testing"

func sg(project string, s, e int) Segment { return Segment{Start: at(s), End: at(e), Project: project} }

const idle45, pause15 = int64(45 * minMs), int64(15 * minMs)

func TestSessionsSplitAtIdleStop(t *testing.T) {
	one := Sessions([]Segment{sg("u", 0, 30), sg("u", 74, 90)}, nil, idle45, at(200))
	assertEq(t, len(one), 1) // 44 min stays inside
	two := Sessions([]Segment{sg("u", 0, 30), sg("u", 75, 90)}, nil, idle45, at(200))
	assertEq(t, len(two), 2) // 45 min or more ends it
	assertEq(t, two[0].End, at(30))
	assertEq(t, two[1].Start, at(75))
	assertEq(t, two[1].Open, false)
}

func TestOverlappingPiecesNeverMoveTheEndBack(t *testing.T) {
	ss := Sessions([]Segment{sg("u", 0, 30), sg("a", 10, 20)}, nil, idle45, at(70))
	assertEq(t, ss[0].End, at(30))
	assertEq(t, ss[0].Open, true) // 40 min after 30, not 50 after 20
}

func TestSessionOpenUntilIdleStopPasses(t *testing.T) {
	me := []Segment{sg("u", 0, 30)}
	assertEq(t, Sessions(me, nil, idle45, at(74))[0].Open, true)
	assertEq(t, Sessions(me, nil, idle45, at(75))[0].Open, false)
}

func TestManualEndHoldsUntilIdleGap(t *testing.T) {
	me := []Segment{sg("u", 0, 30), sg("u", 40, 50), sg("u", 100, 110)}
	ss := Sessions(me, []SessionMark{{ID: 1, Kind: "end", At: at(20)}}, idle45, at(111))
	assertEq(t, len(ss), 2)
	assertEq(t, ss[0].End, at(20))
	assertEq(t, ss[0].Open, false)
	assertEq(t, ss[1].Start, at(100)) // 20–50 belongs to no session
	assertEq(t, ss[1].Open, true)
}

func TestManualStart(t *testing.T) {
	ss := Sessions([]Segment{sg("u", 10, 20)}, []SessionMark{{ID: 1, Kind: "start", At: at(5)}}, idle45, at(30))
	assertEq(t, len(ss), 1)
	assertEq(t, ss[0].Start, at(5))
	assertEq(t, ss[0].End, at(20))
	assertEq(t, ss[0].Manual, true)
	empty := []SessionMark{{ID: 1, Kind: "start", At: at(0)}}
	assertEq(t, len(Sessions(nil, empty, idle45, at(1))), 1)  // just started: open, empty
	assertEq(t, len(Sessions(nil, empty, idle45, at(50))), 0) // closed and empty: dropped
	both := []SessionMark{{ID: 1, Kind: "end", At: at(20)}, {ID: 2, Kind: "start", At: at(30)}}
	ss = Sessions([]Segment{sg("u", 0, 60)}, both, idle45, at(61))
	assertEq(t, len(ss), 2)
	assertEq(t, ss[1].Start, at(30))
	assertEq(t, ss[1].End, at(60))
}

func TestStretchesSplitOnlyAtLongBreaks(t *testing.T) {
	assertEq(t, len(Stretches([]Segment{sg("u", 0, 20), sg("u", 35, 40)}, pause15)), 1) // exactly 15: a counted pause
	st := Stretches([]Segment{sg("u", 0, 20), sg("u", 36, 40)}, pause15)
	assertEq(t, len(st), 2)
	assertEq(t, st[1].GapBefore, int64(16*minMs))
	assertEq(t, st[0].Me, int64(20*minMs))
}

func TestStretchesFoldShortRunsIntoShares(t *testing.T) {
	st := Stretches([]Segment{sg("u", 0, 30), sg("s", 30, 34), sg("u", 34, 50), sg("", 50, 57), sg("u", 57, 60)}, pause15)
	assertEq(t, len(st), 1)
	assertEq(t, st[0].Project, "u")
	assertEq(t, st[0].Me, int64(60*minMs))
	assertEq(t, len(st[0].Shares), 2)
	assertEq(t, st[0].Shares[0], Share{"", 7 * minMs})
	assertEq(t, st[0].Shares[1], Share{"s", 4 * minMs})
}

func TestStretchesSplitOnLongProjectRun(t *testing.T) {
	st := Stretches([]Segment{sg("u", 0, 37), sg("t", 37, 74)}, pause15)
	assertEq(t, len(st), 2)
	assertEq(t, st[1].Project, "t")
	assertEq(t, st[1].GapBefore, int64(0))
	assertEq(t, len(st[0].Shares), 0)
	only := Stretches([]Segment{sg("", 0, 10)}, pause15)
	assertEq(t, only[0].Project, "")
}

func TestSwitchesIgnoreOther(t *testing.T) {
	assertEq(t, Switches([]Segment{sg("u", 0, 5), sg("", 5, 6), sg("u", 6, 9), sg("t", 9, 12), sg("", 12, 13), sg("u", 13, 20)}), 2)
}

func TestFocusBridgesShortBreaksAndOther(t *testing.T) {
	me := []Segment{sg("u", 0, 30), sg("", 30, 35), sg("u", 40, 60), sg("t", 60, 70), sg("u", 90, 100)}
	assertEq(t, Focus(me, pause15), int64(50*minMs)) // Другое and the 5-min pause bridged, not counted
	assertEq(t, Focus([]Segment{sg("u", 0, 30), sg("u", 46, 60)}, pause15), int64(30*minMs))
}

func TestDescribeSession(t *testing.T) {
	me := []Segment{sg("u", 0, 20), sg("u", 40, 60), sg("t", 61, 70)} // t-segment is after the session
	ss := describe(Session{Start: at(0), End: at(60), Open: true}, me[:2], tsettings(), at(61))
	assertEq(t, ss.Me, int64(40*minMs))
	assertEq(t, ss.Breaks, int64(20*minMs))
	assertEq(t, len(ss.Stretches), 2)
	assertEq(t, ss.Stretches[1].Current, true)
	assertEq(t, describe(Session{Start: at(0), End: at(60), Open: true}, me[:2], tsettings(), at(70)).Stretches[1].Current, false) // 10 min ≥ idle_minutes
	cut := describe(Session{Start: at(0), End: at(50)}, me, tsettings(), at(200))                                                  // a manual end inside a segment
	assertEq(t, cut.Me, int64(30*minMs))
}

func TestDayStatsSumsBreaks(t *testing.T) {
	me := []Segment{sg("u", 0, 20), sg("t", 40, 60)}
	st := dayStats(me, []Session{{Breaks: 20 * minMs}, {Breaks: 5 * minMs}}, pause15)
	assertEq(t, st, DayStats{Switches: 1, Focus: 20 * minMs, Breaks: 25 * minMs})
}
