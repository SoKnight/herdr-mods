package main

import "testing"

func tsettings() Settings {
	s := DefaultSettings()
	s.Loc = msk
	return s
}

func TestRoundNearestHalfUp(t *testing.T) {
	assertEq(t, RoundMinutes(22*minMs, 15), int64(15))
	assertEq(t, RoundMinutes(23*minMs, 15), int64(30))
	assertEq(t, RoundMinutes(22*minMs+30_000, 15), int64(30)) // 22.5 -> half up
	assertEq(t, RoundMinutes(2*minMs+29_000, 5), int64(0))
	assertEq(t, RoundMinutes(61_000, 1), int64(1))
}

func TestSummarizeByDayWithDayStart(t *testing.T) {
	tl := Timeline{Me: []Segment{{Start: mskAt(2026, 11, 16, 3, 0), End: mskAt(2026, 11, 16, 5, 0), Project: "easylauncher", Activity: "development"}}}
	r := Summarize(tl, tsettings(), []string{"easylauncher"})
	p := r.Projects[0]
	assertEq(t, r.Me, int64(120*minMs))
	assertEq(t, p.Days["2026-11-15"].Me, int64(60*minMs))
	assertEq(t, p.Days["2026-11-16"].Me, int64(60*minMs))
	assertEq(t, p.Days["2026-11-16"].Activities["development"], int64(60*minMs))
	assertEq(t, p.Activities["development"], int64(120*minMs))
}

func TestSummarizeOrderUptimeAndNoProjectLast(t *testing.T) {
	tl := Timeline{
		Power: []Interval{m(0, 30), m(40, 50)},
		Me: []Segment{
			{Start: at(0), End: at(10)},
			{Start: at(10), End: at(20), Project: "universe"},
			{Start: at(20), End: at(30), Project: "easylauncher"},
		},
	}
	r := Summarize(tl, tsettings(), []string{"easylauncher", "universe"})
	assertEq(t, r.Uptime, int64(40*minMs))
	assertEq(t, len(r.Projects), 3)
	assertEq(t, r.Projects[0].Project, "easylauncher")
	assertEq(t, r.Projects[1].Project, "universe")
	assertEq(t, r.Projects[2].Project, "")
	assertEq(t, r.Projects[2].Activities[""], int64(10*minMs))
}

func TestAgentsTotalIsUnionAcrossProjects(t *testing.T) {
	tl := Timeline{Agents: []AgentSeg{
		{Start: at(0), End: at(30), Project: "easylauncher"},
		{Start: at(10), End: at(40), Project: "universe"},
	}}
	r := Summarize(tl, tsettings(), []string{"easylauncher", "universe"})
	assertEq(t, r.Agents, int64(40*minMs))
	assertEq(t, r.Projects[0].Agents, int64(30*minMs))
	assertEq(t, r.Projects[1].Agents, int64(30*minMs))
}

func TestClosedPeriodUsesSnapshotAndReportsDelta(t *testing.T) {
	snap := Snapshot(Timeline{Me: []Segment{{Start: at(0), End: at(60), Project: "easylauncher"}}}, "easylauncher", at(0), at(60))
	snap.ID = 7
	fresh := Timeline{Me: []Segment{
		{Start: at(0), End: at(45), Project: "easylauncher"},
		{Start: at(45), End: at(60), Project: "tricky"},
	}}
	got, div := ApplyPeriods(fresh, []Period{snap})
	if len(div) != 1 || div[0] != (Divergence{Period: 7, Project: "easylauncher", DeltaMs: -15 * minMs}) {
		t.Fatalf("divergence: %+v", div)
	}
	var easy, tricky int64
	for _, s := range got.Me {
		switch s.Project {
		case "easylauncher":
			easy += s.End - s.Start
		case "tricky":
			tricky += s.End - s.Start
		}
	}
	assertEq(t, easy, int64(60*minMs))
	assertEq(t, tricky, int64(15*minMs)) // other projects stay as they are
}

func TestClosedPeriodNoDivergenceWhenEqual(t *testing.T) {
	tl := Timeline{Me: []Segment{{Start: at(0), End: at(60), Project: "easylauncher"}}, Agents: []AgentSeg{{Start: at(0), End: at(10), Project: "easylauncher"}}}
	snap := Snapshot(tl, "easylauncher", at(0), at(60))
	got, div := ApplyPeriods(tl, []Period{snap})
	assertEq(t, len(div), 0)
	assertEq(t, len(got.Me), 1)
	assertEq(t, len(got.Agents), 1)
}

func TestClosedPeriodClipsAtBounds(t *testing.T) {
	snap := Snapshot(Timeline{Me: []Segment{{Start: at(0), End: at(30), Project: "easylauncher"}}}, "easylauncher", at(0), at(30))
	fresh := Timeline{Me: []Segment{{Start: at(0), End: at(60), Project: "easylauncher"}}}
	got, div := ApplyPeriods(fresh, []Period{snap})
	assertEq(t, len(div), 0)
	var sum int64
	for _, s := range got.Me {
		sum += s.End - s.Start
	}
	assertEq(t, sum, int64(60*minMs)) // the part outside the period is untouched
}
