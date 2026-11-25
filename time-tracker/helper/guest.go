package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"
)

// GuestConfig tells the host collector how to poll a VM guest's foreground window and Claude Desktop agents.
type GuestConfig struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	MatchExe   string `json:"match_exe,omitempty"`
	MatchTitle string `json:"match_title,omitempty"`
}

// GuestSnapshot is what `herdr-time guest` returns to the host collector on GET /v1/guest.
type GuestSnapshot struct {
	Exe       string                `json:"exe"`
	Title     string                `json:"title"`
	CallExes  []string              `json:"call_exes,omitempty"`
	SoundExes []string              `json:"sound_exes,omitempty"`
	Agents    map[string]AgentState `json:"agents,omitempty"`
}

type claudeSessionFile struct {
	PID             int    `json:"pid"`
	Cwd             string `json:"cwd"`
	Entrypoint      string `json:"entrypoint"`
	Status          string `json:"status"`
	UpdatedAt       int64  `json:"updatedAt"`
	StatusUpdatedAt int64  `json:"statusUpdatedAt"`
}

const guestTimeout = 500 * time.Millisecond

var (
	fetchGuest       = httpFetchGuest
	guestHTTPClient  = &http.Client{Timeout: guestTimeout}
	isProcessAlive   = processAlive
	claudeSessionDir = defaultClaudeSessionDir
)

func defaultClaudeSessionDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "sessions")
}

// readClaudeDesktopSessions reads live Claude Desktop Code sessions from ~/.claude/sessions/*.json.
// CLI sessions are skipped because `hc events` already tracks them.
func readClaudeDesktopSessions(dir string, alive func(int) bool, now int64) map[string]AgentState {
	if dir == "" {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := map[string]AgentState{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var s claudeSessionFile
		if json.Unmarshal(data, &s) != nil {
			continue
		}
		if s.Entrypoint != "claude-desktop" || s.PID <= 0 || !alive(s.PID) {
			continue
		}
		st := strings.ToLower(strings.TrimSpace(s.Status))
		working := st == "working" || st == "busy" || st == "running" || st == "active" || st == "streaming" || st == "thinking"
		waiting := st == "waiting" || st == "blocked" || st == "permission" || st == "approval" || st == "needs_input"
		since := s.StatusUpdatedAt
		if since <= 0 {
			since = s.UpdatedAt
		}
		if since <= 0 {
			since = now
		}
		out[fmt.Sprintf("claude-desktop/%d", s.PID)] = AgentState{
			Cwd:     s.Cwd,
			Working: working,
			Waiting: waiting,
			Since:   since,
		}
	}
	return out
}

func guestMatchesWindow(g GuestConfig, exe, title string) bool {
	wantExe := g.MatchExe
	if wantExe == "" {
		wantExe = "mstsc.exe"
	}
	if !strings.EqualFold(exe, wantExe) {
		return false
	}
	return g.MatchTitle == "" || containsFold(title, g.MatchTitle)
}

func httpFetchGuest(ctx context.Context, baseURL string) (GuestSnapshot, error) {
	ctx, cancel := context.WithTimeout(ctx, guestTimeout)
	defer cancel()
	url := strings.TrimRight(baseURL, "/") + "/v1/guest"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return GuestSnapshot{}, err
	}
	req.Header.Set("X-Herdr-Time", "1")
	resp, err := guestHTTPClient.Do(req)
	if err != nil {
		return GuestSnapshot{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return GuestSnapshot{}, fmt.Errorf("guest %s: status %d", baseURL, resp.StatusCode)
	}
	var snap GuestSnapshot
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&snap); err != nil {
		return GuestSnapshot{}, err
	}
	return snap, nil
}

func collectGuestSnapshot(now int64) GuestSnapshot {
	var snap GuestSnapshot
	if s, err := takeSample(now); err == nil && !s.Locked {
		snap.Exe = s.Exe
		snap.Title = s.Title
		snap.CallExes = s.CallExes
		snap.SoundExes = s.SoundExes
	}
	snap.Agents = readClaudeDesktopSessions(claudeSessionDir(), isProcessAlive, now)
	return snap
}

func newGuestServer(quit func()) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /v1/guest", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Herdr-Time") != "1" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(collectGuestSnapshot(nowMs()))
	})
	mux.HandleFunc("POST /v1/quit", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Herdr-Time") != "1" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		go quit()
	})
	return mux
}

func runGuest(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("guest", flag.ContinueOnError)
	fs.SetOutput(stderr)
	listen := fs.String("listen", "0.0.0.0:47614", "address to listen on")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		fmt.Fprintf(stderr, "guest listen %s: %v\n", *listen, err)
		return 1
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	srv := &http.Server{Handler: newGuestServer(stop)}
	go func() {
		<-ctx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
	}()
	if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(stderr, "guest serve: %v\n", err)
		return 1
	}
	return 0
}
