package main

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"math"
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

// gitCmd is git bound by ctx. WaitDelay: on Windows git.exe is a wrapper whose child keeps the stdout pipe open
// after the kill, and Run would wait for it.
func gitCmd(ctx context.Context, args ...string) *exec.Cmd {
	cmd := hidden(exec.CommandContext(ctx, "git", args...))
	cmd.WaitDelay = time.Second
	return cmd
}

// gitOut runs git in repo for at most gitTimeout; a git killed by ctx gives ctx's error, not its exit.
func gitOut(ctx context.Context, repo string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, gitTimeout)
	defer cancel()
	cmd := gitCmd(ctx, append([]string{"-C", repo}, args...)...)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return out.Bytes(), err
}

func realGitLog(ctx context.Context, repo string, from, to int64) ([]Commit, error) {
	email, ok := gitEmails.Load(repo)
	if !ok {
		out, err := gitOut(ctx, repo, "config", "user.email")
		var exit *exec.ExitError
		if err != nil && !(errors.As(err, &exit) && exit.ExitCode() == 1) { // exit 1 = not set: every author's then
			return nil, err // a timeout or a broken repo: asked again next time, and no one else's commits meanwhile
		}
		email, _ = gitEmails.LoadOrStore(repo, strings.TrimSpace(string(out)))
	}
	args := []string{"log", "--branches", "--tags", "--remotes", "--no-merges", // not --all: stashes and notes are commits too
		"--since=@" + strconv.FormatInt(max(from/1000, 1_000_000_000), 10), "--format=%at%x09%s"} // --since @0 and tiny values are ignored by git, hence the floor; it is the committer date, never before the author's: a superset; no --until, which would drop a rebased commit
	if e := email.(string); e != "" {
		args = append(args, "--fixed-strings", "--author=<"+e+">")
	}
	out, err := gitOut(ctx, repo, args...)
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

// commitBudget bounds an on-demand commits call over all its repos: it runs under d.mu.
var commitBudget = 10 * time.Second

// repoCommits asks git for each repo's commits in from..to until ctx is done; a failing repo gives none.
func repoCommits(ctx context.Context, repos []string, from, to int64) map[string][]Commit {
	out := map[string][]Commit{}
	for _, repo := range repos {
		if ctx.Err() != nil {
			break
		}
		if list, err := gitLog(ctx, repo, from, to); err == nil {
			out[repo] = list
		}
	}
	return out
}

// attach gives each repo's commits the project of their folder rule, leaves out Исключить and Досуг,
// and sorts them by time. Callers hold mu.
func (d *Daemon) attach(byRepo map[string][]Commit) []Commit {
	out := []Commit{}
	for repo, list := range byRepo {
		for _, c := range list {
			if id, _ := d.Rules.FolderProject(repo, c.At); id != "" && !dropped(id) {
				c.Project, c.Repo = id, filepath.Base(repo)
				out = append(out, c)
			}
		}
	}
	slices.SortStableFunc(out, func(a, b Commit) int {
		return cmp.Or(cmp.Compare(a.At, b.At), cmp.Compare(a.Repo, b.Repo), cmp.Compare(a.Msg, b.Msg))
	})
	return out
}

// commits are my commits in from..to under the folder rules' projects, by time, cached for 60 s: for the
// month report, export and /v1/commits. git gets commitBudget for all repos; a cut-short answer is not
// cached. A missing git or a broken repo leaves its commits out. Callers hold mu.
func (d *Daemon) commits(from, to int64) []Commit {
	key, now := [2]int64{from, to}, nowMs()
	if c, ok := d.commitCache[key]; ok && now-c.at < 60_000 {
		return c.list
	}
	ctx, cancel := context.WithTimeout(context.Background(), commitBudget)
	defer cancel()
	out := d.attach(repoCommits(ctx, repoDirs(d.Rules), from, to))
	if ctx.Err() != nil {
		return out
	}
	if len(d.commitCache) > 32 {
		clear(d.commitCache)
	}
	if d.commitCache == nil {
		d.commitCache = map[[2]int64]commitsAt{}
	}
	d.commitCache[key] = commitsAt{now, out}
	return out
}

const recentMs = 48 * 60 * 60_000

// refreshCommits reads the commits since the start of the hour 48 h back into d.recent, running git
// without mu: the polled session, status and timeline read only that, and git never holds the billing lock.
func (d *Daemon) refreshCommits(ctx context.Context) {
	d.mu.Lock()
	rules := d.Rules // compiled rules are never changed in place, only replaced
	d.mu.Unlock()
	from := (nowMs() - recentMs) / 3_600_000 * 3_600_000
	byRepo := repoCommits(ctx, repoDirs(rules), from, math.MaxInt64)
	d.mu.Lock()
	defer d.mu.Unlock()
	d.recent = d.attach(byRepo)
}

// followCommits refreshes the recent commits every minute until ctx is done.
func (d *Daemon) followCommits(ctx context.Context) {
	for ctx.Err() == nil {
		d.refreshCommits(ctx)
		select {
		case <-ctx.Done():
		case <-time.After(time.Minute):
		}
	}
}

// recentCommits are the refreshed commits in from..to: none before the first refresh or before its
// 48 hours. Callers hold mu.
func (d *Daemon) recentCommits(from, to int64) []Commit {
	var out []Commit
	for _, c := range d.recent {
		if from <= c.At && c.At < to {
			out = append(out, c)
		}
	}
	return out
}

func (d *Daemon) getCommits(r *http.Request) (int, any, error) {
	from, to, err := rangeParams(r)
	if err != nil {
		return 0, nil, err
	}
	return http.StatusOK, map[string]any{"commits": d.commits(from, to)}, nil
}
