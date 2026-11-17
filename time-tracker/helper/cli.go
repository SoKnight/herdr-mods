package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

const usage = `herdr-time <команда> [--json]
  run                        запустить сборщик
  probe                      один опрос Windows, без сборщика
  restart                    перезапустить сборщик
  status
  timeline [--day YYYY-MM-DD] [--fresh]   --fresh: без снимков закрытых периодов
  projects
  report --from --to [--project id]
  export --from --to --project id [--format md|csv] [--round 1|5|15] [--agents] [--out файл] [--dry-run]
  correct add <json> | rm <id> | ls
  beat <json>
  unassigned --from --to
  rule add <json>
  session [start|end]
  periods [--project id]
  reopen <id>
--from и --to: Unix ms или YYYY-MM-DD (день в --to входит в диапазон).`

// Seams for tests.
var (
	launch      = launchCollector
	restartWait = 5 * time.Second
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// apiCall is one CLI command as a request to the collector; show prints the answer without --json.
type apiCall struct {
	method, path string
	body         []byte
	save         string // export --out: the file that gets the content
	show         func(w io.Writer, resp []byte) error
}

func runCLI(args []string, stdout, stderr io.Writer) int {
	asJSON := slices.Contains(args, "--json")
	args = slices.DeleteFunc(slices.Clone(args), func(a string) bool { return a == "--json" })
	if len(args) == 0 {
		fmt.Fprintln(stderr, usage)
		return 2
	}
	s, err := LoadSettings(Home())
	if err != nil {
		fmt.Fprintln(stderr, err) // the collector runs on the defaults then, so the CLI does too
		s = DefaultSettings()
	}
	switch args[0] {
	case "run":
		return runCollector(s, stdout, stderr)
	case "probe":
		return probe(stdout, stderr)
	case "restart":
		return restart(s, asJSON, stdout, stderr)
	}
	c, err := buildCall(s, args[0], args[1:])
	if err != nil {
		fmt.Fprintf(stderr, "%v\n\n%s\n", err, usage)
		return 2
	}
	resp, code, err := send(s, c.method, c.path, c.body)
	if err != nil {
		if asJSON {
			fmt.Fprintln(stdout, `{"error":"down"}`)
		} else {
			fmt.Fprintln(stderr, "сборщик не запущен")
		}
		return 3
	}
	if code >= 400 {
		if asJSON {
			stdout.Write(resp)
		} else {
			var e struct {
				Error  string `json:"error"`
				Period int64  `json:"period"`
			}
			if json.Unmarshal(resp, &e) != nil || e.Error == "" {
				e.Error = strings.TrimSpace(string(resp))
			}
			if e.Period != 0 {
				e.Error += fmt.Sprintf(": период #%d закрыт выгрузкой, herdr-time reopen %d", e.Period, e.Period)
			}
			fmt.Fprintln(stderr, e.Error)
		}
		return 1
	}
	if c.save != "" {
		var ex exportResult
		if err := json.Unmarshal(resp, &ex); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if err := os.WriteFile(c.save, []byte(ex.Content), 0o644); err != nil {
			if ex.Period != 0 { // no file, no closed period
				send(s, "POST", fmt.Sprintf("/v1/periods/%d/reopen", ex.Period), nil)
			}
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	if asJSON {
		if len(bytes.TrimSpace(resp)) == 0 {
			resp = []byte("{}\n")
		}
		stdout.Write(resp)
		return 0
	}
	if c.show != nil {
		if err := c.show(stdout, resp); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	return 0
}

func send(s Settings, method, path string, body []byte) ([]byte, int, error) {
	req, err := http.NewRequest(method, fmt.Sprintf("http://127.0.0.1:%d%s", s.Port, path), bytes.NewReader(body))
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("X-Herdr-Time", "1")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	return data, resp.StatusCode, err
}

// when reads Unix ms or a YYYY-MM-DD day; a day as the end of a range means its end.
func when(s Settings, v string, end bool) (int64, error) {
	if v == "" {
		return 0, errors.New("--from and --to are required")
	}
	if ms, err := strconv.ParseInt(v, 10, 64); err == nil {
		return ms, nil
	}
	from, to, err := dayRange(s, v)
	if err != nil {
		return 0, fmt.Errorf("%q: want Unix ms or YYYY-MM-DD", v)
	}
	if end {
		return to, nil
	}
	return from, nil
}

func buildCall(s Settings, cmd string, args []string) (apiCall, error) {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	day := fs.String("day", "", "")
	fromArg, toArg := fs.String("from", "", ""), fs.String("to", "", "")
	project := fs.String("project", "", "")
	format := fs.String("format", "md", "")
	round := fs.Int("round", 15, "")
	agents := fs.Bool("agents", false, "")
	out := fs.String("out", "", "")
	fresh := fs.Bool("fresh", false, "")
	dry := fs.Bool("dry-run", false, "")
	if err := fs.Parse(args); err != nil {
		return apiCall{}, err
	}
	pos := fs.Args()
	arg := func(i int, what string) (string, error) {
		if i < len(pos) && pos[i] != "" {
			return pos[i], nil
		}
		return "", fmt.Errorf("%s: missing %s", cmd, what)
	}
	q := url.Values{}
	span := func() (int64, int64, error) {
		from, err := when(s, *fromArg, false)
		if err != nil {
			return 0, 0, err
		}
		to, err := when(s, *toArg, true)
		q.Set("from", strconv.FormatInt(from, 10))
		q.Set("to", strconv.FormatInt(to, 10))
		return from, to, err
	}
	sub, _ := arg(0, "")
	switch {
	case cmd == "status":
		return apiCall{method: "GET", path: "/v1/status", show: showStatus}, nil
	case cmd == "timeline":
		if *day != "" {
			q.Set("day", *day)
		}
		if *fresh {
			q.Set("fresh", "1")
		}
		return apiCall{method: "GET", path: "/v1/timeline?" + q.Encode(), show: showTimeline}, nil
	case cmd == "session" && sub == "":
		return apiCall{method: "GET", path: "/v1/session", show: showSession}, nil
	case cmd == "session" && (sub == "start" || sub == "end"):
		body, _ := json.Marshal(map[string]string{"action": sub})
		return apiCall{method: "POST", path: "/v1/session", body: body}, nil
	case cmd == "projects":
		return apiCall{method: "GET", path: "/v1/projects", show: showProjects}, nil
	case cmd == "report":
		if _, _, err := span(); err != nil {
			return apiCall{}, err
		}
		if *project != "" {
			q.Set("project", *project)
		}
		return apiCall{method: "GET", path: "/v1/report?" + q.Encode(), show: showReport}, nil
	case cmd == "unassigned":
		if _, _, err := span(); err != nil {
			return apiCall{}, err
		}
		return apiCall{method: "GET", path: "/v1/unassigned?" + q.Encode(), show: showUnassigned}, nil
	case cmd == "export":
		if *dry && *out != "" {
			return apiCall{}, fmt.Errorf("export: --dry-run never writes a file, drop --out")
		}
		from, to, err := span()
		if err != nil {
			return apiCall{}, err
		}
		body, err := json.Marshal(map[string]any{"project": *project, "from": from, "to": to,
			"format": *format, "round": *round, "agents": *agents, "dry_run": *dry})
		return apiCall{method: "POST", path: "/v1/export", body: body, save: *out, show: showExport(*out)}, err
	case cmd == "correct" && sub == "add":
		body, err := arg(1, "<json>")
		return apiCall{method: "POST", path: "/v1/corrections", body: []byte(body), show: showCorrection}, err
	case cmd == "correct" && sub == "rm":
		id, err := arg(1, "<id>")
		return apiCall{method: "DELETE", path: "/v1/corrections/" + url.PathEscape(id)}, err
	case cmd == "correct" && sub == "ls":
		return apiCall{method: "GET", path: "/v1/corrections", show: showCorrections}, nil
	case cmd == "beat":
		body, err := arg(0, "<json>")
		return apiCall{method: "POST", path: "/v1/heartbeat", body: []byte(body)}, err
	case cmd == "rule" && sub == "add":
		body, err := arg(1, "<json>")
		return apiCall{method: "POST", path: "/v1/rules", body: []byte(body)}, err
	case cmd == "periods":
		if *project != "" {
			q.Set("project", *project)
		}
		return apiCall{method: "GET", path: "/v1/periods?" + q.Encode(), show: showPeriods}, nil
	case cmd == "reopen":
		id, err := arg(0, "<id>")
		return apiCall{method: "POST", path: "/v1/periods/" + url.PathEscape(id) + "/reopen"}, err
	}
	return apiCall{}, fmt.Errorf("unknown command %q", strings.TrimSpace(cmd+" "+sub))
}

// runCollector serves the API and runs the daemon; a second copy finds the port taken and steps aside.
func runCollector(s Settings, stdout, stderr io.Writer) int {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.Port))
	if err != nil {
		resp, _, err := send(s, "GET", "/v1/status", nil) // ours answers with its version, even with a config error
		var st struct {
			Version string `json:"version"`
		}
		if err == nil && json.Unmarshal(resp, &st) == nil && st.Version != "" {
			fmt.Fprintln(stdout, "уже запущен")
			return 0
		}
		fmt.Fprintf(stderr, "порт %d занят другим процессом\n", s.Port)
		return 1
	}
	if f := openLog(Home()); f != nil {
		defer f.Close()
		defer log.SetOutput(os.Stderr)
	}
	d, err := NewDaemon(Home())
	if err != nil {
		ln.Close()
		fmt.Fprintln(stderr, err)
		return 1
	}
	srv := &http.Server{Handler: newServer(d), ReadHeaderTimeout: 10 * time.Second}
	go srv.Serve(ln)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	err = d.Run(ctx)
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	srv.Shutdown(shutdown)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}

// openLog sends the log to collector.log in home: the collector runs without a console. Over 1 MiB it starts afresh.
func openLog(home string) *os.File {
	path := filepath.Join(home, "collector.log")
	flags := os.O_APPEND | os.O_CREATE | os.O_WRONLY
	if fi, err := os.Stat(path); err == nil && fi.Size() > 1<<20 {
		flags |= os.O_TRUNC
	}
	if os.MkdirAll(home, 0o755) != nil {
		return nil
	}
	f, err := os.OpenFile(path, flags, 0o644)
	if err != nil {
		return nil
	}
	log.SetOutput(f)
	return f
}

// restart stops the collector (a down one is fine), starts this exe's, and waits for each step.
func restart(s Settings, asJSON bool, stdout, stderr io.Writer) int {
	fail := func(msg string) int {
		if asJSON {
			b, _ := json.Marshal(map[string]string{"error": msg})
			fmt.Fprintln(stdout, string(b))
		} else {
			fmt.Fprintln(stderr, msg)
		}
		return 1
	}
	wait := func(answers bool) bool {
		for end := time.Now().Add(restartWait); ; time.Sleep(100 * time.Millisecond) {
			if _, _, err := send(s, "GET", "/v1/status", nil); (err == nil) == answers {
				return true
			}
			if time.Now().After(end) {
				return false
			}
		}
	}
	send(s, "POST", "/v1/quit", nil)
	if !wait(false) {
		return fail("сборщик не остановился")
	}
	exe, err := os.Executable()
	if err == nil {
		err = launch(exe)
	}
	if err != nil {
		fmt.Fprintln(stderr, err)
	}
	if err != nil || !wait(true) {
		return fail("сборщик не запустился")
	}
	if asJSON {
		fmt.Fprintln(stdout, "{}")
	}
	return 0
}

func probe(stdout, stderr io.Writer) int {
	sm, err := takeSample(nowMs())
	b, _ := json.MarshalIndent(sm, "", "  ")
	fmt.Fprintln(stdout, string(b))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	return 0
}
