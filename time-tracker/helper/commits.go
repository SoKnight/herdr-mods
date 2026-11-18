package main

import (
	"bytes"
	"cmp"
	"context"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const gitTimeout = 5 * time.Second

// Commit is one of my commits in a repo under a project's folder. They are notes, not numbers.
type Commit struct {
	At      int64  `json:"at"`
	Project string `json:"project"`
	Repo    string `json:"repo"` // the repo's folder name
	Msg     string `json:"msg"`
}

// gitLog is the seam for tests: my commits in repo with from <= At < to, At and Msg only.
var gitLog = realGitLog

var gitEmails sync.Map // repo -> user.email, for the process

func gitOut(repo string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout) // git runs under d.mu: a stall must not freeze the collector
	defer cancel()
	cmd := hidden(exec.CommandContext(ctx, "git", append([]string{"-C", repo}, args...)...))
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	return out.Bytes(), err
}

func realGitLog(repo string, from, to int64) ([]Commit, error) {
	email, ok := gitEmails.Load(repo)
	if !ok {
		out, _ := gitOut(repo, "config", "user.email") // exit 1 = not set: every author's then
		email, _ = gitEmails.LoadOrStore(repo, strings.TrimSpace(string(out)))
	}
	args := []string{"log", "--branches", "--tags", "--remotes", "--no-merges", // not --all: stashes and notes are commits too
		"--since=@" + strconv.FormatInt(max(from/1000, 1_000_000_000), 10), "--format=%at%x09%s"} // --since @0 and tiny values are ignored by git, hence the floor; it is the committer date, never before the author's: a superset; no --until, which would drop a rebased commit
	if e := email.(string); e != "" {
		args = append(args, "--fixed-strings", "--author=<"+e+">")
	}
	out, err := gitOut(repo, args...)
	if err != nil {
		return nil, err
	}
	var list []Commit
	for _, line := range strings.Split(string(out), "\n") {
		sec, msg, _ := strings.Cut(strings.TrimRight(line, "\r\n"), "\t")
		if n, err := strconv.ParseInt(sec, 10, 64); err == nil && from <= n*1000 && n*1000 < to {
			list = append(list, Commit{At: n * 1000, Msg: msg})
		}
	}
	return list, nil
}

// repoDirs are the repos the folder rules point at: a rule's folder if it is one, else its subfolders that are.
// A .git file (a worktree) counts. Missing folders give nothing.
func repoDirs(rules *Rules) []string {
	var out []string
	add := func(dir string) bool {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
			return false
		}
		if !slices.Contains(out, dir) {
			out = append(out, dir)
		}
		return true
	}
	if rules == nil {
		return out
	}
	for _, r := range rules.rules {
		if r.Kind != "folder" {
			continue
		}
		if add(r.Value) {
			continue
		}
		subs, _ := os.ReadDir(r.Value)
		for _, e := range subs {
			if e.IsDir() {
				add(filepath.Join(r.Value, e.Name()))
			}
		}
	}
	return out
}

type commitsAt struct {
	at   int64
	list []Commit
}

// commits are my commits in from..to under the folder rules' projects, by time, cached for 60 s.
// A missing git or a broken repo leaves its commits out. Callers hold mu.
func (d *Daemon) commits(from, to int64) []Commit {
	key, now := [2]int64{from, to}, nowMs()
	if c, ok := d.commitCache[key]; ok && now-c.at < 60_000 {
		return c.list
	}
	out := []Commit{}
	for _, repo := range repoDirs(d.Rules) {
		list, err := gitLog(repo, from, to)
		if err != nil {
			continue
		}
		for _, c := range list {
			if id, _ := d.Rules.FolderProject(repo, c.At); id != "" && !dropped(id) {
				c.Project, c.Repo = id, filepath.Base(repo)
				out = append(out, c)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b Commit) int { return cmp.Compare(a.At, b.At) })
	if len(d.commitCache) > 32 {
		clear(d.commitCache)
	}
	if d.commitCache == nil {
		d.commitCache = map[[2]int64]commitsAt{}
	}
	d.commitCache[key] = commitsAt{now, out}
	return out
}

func (d *Daemon) getCommits(r *http.Request) (int, any, error) {
	from, to, err := rangeParams(r)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, map[string]any{"commits": d.commits(from, to)}, nil
}
