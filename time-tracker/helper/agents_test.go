package main

import (
	"strings"
	"testing"
)

func TestParseEventPicksLiveAgents(t *testing.T) {
	states, err := parseEvent([]byte(`{"seq":1,"terminals":[
	 {"address":"t1","cwd":"C:\\Dev\\easyx\\EasyLauncher","state":"live","status":{"app":"claude-code","state":"working"}},
	 {"address":"t2","cwd":"C:\\Dev","state":"live","status":null},
	 {"address":"t3","cwd":"C:\\Dev","state":"finished","status":{"app":"claude-code","state":"done"}}]}`))
	must(t, err)
	assertEq(t, len(states), 1)
	assertEq(t, states["t1"].Working, true)
	s, err := parseEvent([]byte(`{"seq":2}`))
	must(t, err)
	assertEq(t, s == nil, true)
}

func working(cwd string) map[string]AgentState {
	return map[string]AgentState{"t1": {Cwd: cwd, Working: true}}
}

func TestAgentTrackerSpans(t *testing.T) {
	var a AgentTracker
	assertEq(t, len(a.Feed(working(`C:\Dev`), 0)), 0)
	assertEq(t, len(a.Feed(working(`C:\Dev`), 30000)), 0) // same state: no-op
	got := a.Feed(map[string]AgentState{"t1": {Cwd: `C:\Dev`}}, 60000)
	assertEq(t, len(got), 1)
	assertEq(t, got[0], Span{Kind: "agent", Start: 0, End: 60000, Term: "t1", Cwd: `C:\Dev`})
	assertEq(t, len(a.Open(70000)), 0)
}

func TestAgentStreamLostMarksUnknown(t *testing.T) {
	var a AgentTracker
	a.Feed(working(`C:\Dev`), 0)
	got := a.Lost(30000)
	assertEq(t, len(got), 1)
	assertEq(t, got[0].End, int64(30000))
	assertEq(t, len(a.Lost(40000)), 0)
	open := a.Open(50000)
	assertEq(t, open[0], Span{Kind: "agents_unknown", Start: 30000, End: 50000})
	got = a.Feed(working(`C:\Dev`), 90000)
	assertEq(t, len(got), 1)
	assertEq(t, got[0], Span{Kind: "agents_unknown", Start: 30000, End: 90000})
}

func TestReadEventsLongLinesAndGarbage(t *testing.T) {
	long := `{"terminals":[{"address":"t1","cwd":"` + strings.Repeat("x", 200_000) + `","state":"live","status":{"app":"a","state":"working"}}]}`
	n := 0
	err := readEvents(strings.NewReader("garbage\n"+long+"\n{\"seq\":1}\n"), func(m map[string]AgentState) { n++ })
	must(t, err)
	assertEq(t, n, 1)
}

func TestAgentVanishedAndConcurrent(t *testing.T) {
	var a AgentTracker
	a.Feed(map[string]AgentState{"t1": {Cwd: "a", Working: true}, "t2": {Cwd: "b", Working: true}}, 0)
	assertEq(t, len(a.Open(10)), 2)
	got := a.Feed(map[string]AgentState{"t2": {Cwd: "b", Working: true}}, 100)
	assertEq(t, len(got), 1)
	assertEq(t, got[0], Span{Kind: "agent", Start: 0, End: 100, Term: "t1", Cwd: "a"})
	got = a.Lost(200)
	assertEq(t, len(got), 1)
	assertEq(t, got[0].Term, "t2")
}

func TestAgentCutAcrossSleep(t *testing.T) {
	var a AgentTracker
	a.Feed(working(`C:\Dev`), 0)
	got := a.Cut(10_000, 600_000)
	assertEq(t, len(got), 1)
	assertEq(t, got[0], Span{Kind: "agent", Start: 0, End: 10_000, Term: "t1", Cwd: `C:\Dev`})
	assertEq(t, a.Open(700_000)[0].Start, int64(600_000))

	var lost AgentTracker
	lost.Lost(0)
	got = lost.Cut(10_000, 600_000)
	assertEq(t, got[0], Span{Kind: "agents_unknown", Start: 0, End: 10_000})
	assertEq(t, lost.Open(700_000)[0].Start, int64(600_000))
}

const testCap = 45 * minMs

func TestParseWaiting(t *testing.T) {
	line := func(state string, ms string) []byte {
		return []byte(`{"terminals":[{"address":"t1","cwd":"C:\\Dev","state":"live","status":{"app":"claude-code","state":"` + state + `"},"status_ms":` + ms + `}]}`)
	}
	s, err := parseEvent(line("done", "5"))
	must(t, err)
	assertEq(t, s["t1"], AgentState{Cwd: `C:\Dev`, Waiting: true, Since: 5})
	s, _ = parseEvent(line("blocked", "7"))
	assertEq(t, s["t1"].Waiting, true)
	s, _ = parseEvent(line("idle", "9"))
	assertEq(t, s["t1"].Waiting, false)
	assertEq(t, s["t1"].Working, false)
}

func waiting(cwd string, since int64) map[string]AgentState {
	return map[string]AgentState{"t1": {Cwd: cwd, Waiting: true, Since: since}}
}

func TestWaitSpans(t *testing.T) {
	a := AgentTracker{Cap: testCap}
	a.Feed(working(`C:\Dev`), at(0))
	got := a.Feed(waiting(`C:\Dev`, at(5)), at(5))
	assertEq(t, len(got), 1)
	assertEq(t, got[0], Span{Kind: "agent", Start: at(0), End: at(5), Term: "t1", Cwd: `C:\Dev`})
	got = a.Feed(working(`C:\Dev`), at(9))
	assertEq(t, len(got), 1)
	assertEq(t, got[0], Span{Kind: "wait", Start: at(5), End: at(9), Term: "t1", Cwd: `C:\Dev`})
	open := a.Open(at(12))
	assertEq(t, len(open), 1)
	assertEq(t, open[0], Span{Kind: "agent", Start: at(9), End: at(12), Term: "t1", Cwd: `C:\Dev`})
}

func TestStaleDoneIsNotWaiting(t *testing.T) {
	a := AgentTracker{Cap: testCap}
	a.Feed(waiting(`C:\Dev`, at(-3*24*60)), at(0))
	assertEq(t, len(a.Open(at(1))), 0)

	a.Feed(waiting(`C:\Dev`, at(-10)), at(0))
	open := a.Open(at(1))
	assertEq(t, len(open), 1)
	assertEq(t, open[0].Start, at(-10))
	assertEq(t, a.Open(at(100))[0].End, at(35)) // never past Since+cap

	got := a.Feed(waiting(`C:\Dev`, at(-10)), at(50))
	assertEq(t, len(got), 1)
	assertEq(t, got[0], Span{Kind: "wait", Start: at(-10), End: at(35), Term: "t1", Cwd: `C:\Dev`})
	assertEq(t, len(a.Open(at(60))), 0)
	assertEq(t, len(a.Feed(waiting(`C:\Dev`, at(-10)), at(70))), 0) // and does not reopen
}

func TestWaitCutLostAndCwd(t *testing.T) {
	a := AgentTracker{Cap: testCap}
	a.Feed(waiting(`C:\A`, at(0)), at(0))
	got := a.Feed(waiting(`C:\B`, at(0)), at(3))
	assertEq(t, len(got), 1) // cwd changed: the old wait closes, a new one opens
	assertEq(t, got[0].Cwd, `C:\A`)
	got = a.Cut(at(5), at(20))
	assertEq(t, len(got), 1)
	assertEq(t, got[0].End, at(5))
	assertEq(t, a.Open(at(25))[0].Start, at(20))
	got = a.Lost(at(30))
	assertEq(t, len(got), 1)
	assertEq(t, got[0].Kind, "wait")
}

func TestExpireClosesPastCap(t *testing.T) {
	a := AgentTracker{Cap: testCap}
	a.Feed(waiting(`C:\Dev`, at(0)), at(1))
	assertEq(t, len(a.Expire(at(44))), 0)
	got := a.Expire(at(100))
	assertEq(t, len(got), 1)
	assertEq(t, got[0], Span{Kind: "wait", Start: at(0), End: at(45), Term: "t1", Cwd: `C:\Dev`})
	assertEq(t, len(a.Open(at(100))), 0)
}
