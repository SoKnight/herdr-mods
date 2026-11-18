package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mkdirs(t *testing.T, root string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		must(t, os.MkdirAll(filepath.Join(root, p), 0o755))
	}
}

func folderRules(project string, dirs ...string) *Rules {
	r := &Rules{}
	for i, d := range dirs {
		r.rules = append(r.rules, Rule{ID: int64(i + 1), Kind: "folder", Value: d, Project: project})
	}
	return r
}

func TestRepoDirs(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, root, "self/.git", "many/a/.git", "many/b", "many/c", "none")
	must(t, os.WriteFile(filepath.Join(root, "many/b/.git"), []byte("gitdir: x"), 0o644))
	p := func(s string) string { return filepath.Join(root, s) }
	assertEq(t, strings.Join(repoDirs(folderRules("easy", p("self"))), "|"), p("self"))
	assertEq(t, strings.Join(repoDirs(folderRules("easy", p("many"))), "|"), p("many/a")+"|"+p("many/b"))
	assertEq(t, len(repoDirs(folderRules("easy", p("missing"), p("none")))), 0)
	assertEq(t, len(repoDirs(folderRules("easy", p("self"), p("self")))), 1)
}

func TestCommitsSkipsNonRepos(t *testing.T) {
	d := testDaemon(t)
	root := t.TempDir()
	mkdirs(t, root, "plain")
	d.Rules = folderRules("easy", filepath.Join(root, "plain"))
	called := false
	swap(t, &gitLog, func(string, int64, int64) ([]Commit, error) { called = true; return nil, nil })
	assertEq(t, len(d.commits(0, 1000)), 0)
	assertEq(t, called, false)
}

// repoDaemon has a folder rule over a temp folder that is a repo.
func repoDaemon(t *testing.T, project string) (*Daemon, string) {
	d := testDaemon(t)
	repo := filepath.Join(t.TempDir(), "r")
	mkdirs(t, repo, ".git")
	d.Rules = folderRules(project, repo)
	return d, repo
}

func TestCommitsWithoutGit(t *testing.T) {
	d, _ := repoDaemon(t, "easy")
	calls := 0
	swap(t, &gitLog, func(string, int64, int64) ([]Commit, error) { calls++; return nil, exec.ErrNotFound })
	now := mskAt(2026, 11, 10, 12, 0)
	swap(t, &nowMs, func() int64 { return now })
	assertEq(t, len(d.commits(0, now)), 0)
	seedWindow(t, d, mskAt(2026, 11, 10, 10, 0), "idea64.exe", "x", 60)
	api := serve(t, d)
	assertEq(t, api.do("GET", "/v1/timeline", nil, nil), 200)
	assertEq(t, api.do("GET", "/v1/session", nil, nil), 200)
	assertEq(t, calls > 1, true) // the sessions asked
}

func TestCommitsRouteDropsOtherProjects(t *testing.T) {
	d, repo := repoDaemon(t, "easy")
	other := filepath.Join(filepath.Dir(repo), "lei")
	mkdirs(t, other, ".git")
	d.Rules.rules = append(d.Rules.rules, Rule{ID: 10, Kind: "folder", Value: other, Project: leisureID})
	swap(t, &gitLog, func(r string, from, to int64) ([]Commit, error) {
		return []Commit{{At: 2000, Msg: "b " + filepath.Base(r)}, {At: 1000, Msg: "a " + filepath.Base(r)}}, nil
	})
	var out struct {
		Commits []Commit `json:"commits"`
	}
	assertEq(t, serve(t, d).do("GET", "/v1/commits?from=0&to=5000", nil, &out), 200)
	assertEq(t, len(out.Commits), 2)
	assertEq(t, out.Commits[0].Msg+"|"+out.Commits[0].Project+"|"+out.Commits[0].Repo, "a r|easy|r")
}

func TestStretchCommits(t *testing.T) {
	me := []Segment{sg("u", 0, 20), sg("t", 40, 60)}
	cs := []Commit{
		{At: at(0), Project: "u"}, {At: at(10), Project: "u"}, {At: at(20), Project: "u"}, // start, middle, end
		{At: at(20) + 1, Project: "u"}, {At: at(10), Project: "x"}, {At: at(45), Project: "t"},
	}
	ss := describe(Session{Start: at(0), End: at(60), Open: true}, me, cs, tsettings(), at(61))
	assertEq(t, len(ss.Stretches), 2)
	assertEq(t, ss.Stretches[0].Commits, 3)
	assertEq(t, ss.Stretches[1].Commits, 1)
}

func TestMonthRowCommits(t *testing.T) {
	days := planDays(novDays(), shipped(t), []Project{{ID: "easy"}}, nil, Report{})
	cs := []Commit{
		{At: mskAt(2026, 11, 10, 15, 0), Project: "easy", Msg: "later"},
		{At: mskAt(2026, 11, 3, 12, 0), Project: "easy", Msg: "fix"},
		{At: mskAt(2026, 11, 4, 12, 0), Project: "easy", Msg: "fix"},
		{At: mskAt(2026, 11, 4, 13, 0), Project: "easy", Msg: "more"},
		{At: mskAt(2026, 11, 4, 14, 0), Project: "other", Msg: "not mine"},
	}
	m := buildMonth("easy", "2026-11", "week", nov(16), days, nil, cs, tsettings())
	got := map[string]string{}
	for _, r := range m.Rows {
		assertEq(t, r.Commits != nil, true)
		got[r.Label] = strings.Join(r.Commits, "|")
	}
	assertEq(t, got["2 – 8 ноя"], "fix|more")
	assertEq(t, got["9 – 15 ноя"], "later")
	assertEq(t, got["1 ноя"], "")
	assertEq(t, m.Total.Commits != nil, true)
}

func TestExportCommits(t *testing.T) {
	m := previewMonth()
	m.Rows[0].Commits = []string{"только коммиты"}
	m.Rows[0].Me = 0
	m.Rows[1].Commits = []string{"first", "=sum"}
	on := exportOpts{Commits: true, Notes: true}
	txt := exportText("U", m, on)
	assertEq(t, strings.Contains(txt, "2–8 ноя · 14ч 30м\n  — first\n  — =sum\n  — экран заказов"), true)
	assertEq(t, strings.Contains(txt, "1 ноя · 0м\n  — только коммиты\n"), true) // no time, but commits
	off := exportText("U", m, exportOpts{Notes: true})
	assertEq(t, strings.Contains(off, "first"), false)
	assertEq(t, strings.Contains(off, "1 ноя"), false)
	csv := exportCSV(m, on)
	assertEq(t, strings.HasPrefix(csv, "\xef\xbb\xbfС;По;Моё, мин;Заметки;Коммиты\r\n"), true)
	assertEq(t, strings.Contains(csv, "first; =sum"), true)
	assertEq(t, strings.Contains(exportCSV(m, exportOpts{Notes: true}), "Коммиты"), false)
	m.Rows[0].Commits = []string{"=cmd"}
	assertEq(t, strings.Contains(exportCSV(m, on), "'=cmd"), true)
}

func TestGitLogReal(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo, "-c", "commit.gpgsign=false", "-c", "user.name=T"}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
	}
	git("init")
	git("config", "user.email", "t@x")
	git("commit", "--allow-empty", "-m", "mine")
	git("-c", "user.email=o@x", "commit", "--allow-empty", "-m", "theirs")
	now := time.Now().UnixMilli()
	got, err := gitLog(repo, 0, now+60_000)
	must(t, err)
	assertEq(t, len(got), 1)
	assertEq(t, got[0].Msg, "mine")
	assertEq(t, got[0].At <= now && got[0].At > now-60_000, true)
	none, err := gitLog(repo, 0, 1000)
	must(t, err)
	assertEq(t, len(none), 0)
}
