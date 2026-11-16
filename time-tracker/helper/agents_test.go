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
