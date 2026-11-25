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
  export --project id --month YYYY-MM [--rows week|day] [--format text|csv] [--notes] [--pay] [--agents] [--goal] [--commits] [--out файл]
  correct add <json> | rm <id> | ls
  beat <json>
  inbox [--day YYYY-MM-DD]
  inbox apply <json>
  rules
  rule add <json> | set <id> <json> | rm <id> | preview <json>
  commits [--days N]         мои коммиты из репозиториев в папках проектов
  hints [--name X] [--days N]
  history --from --to | undo <id>
  project add <json> | set <id> <json>
  goal set <project> <json>
  pay set <project> <json>
  bonus add <json> | rm <id>
  dayoff add <json> | rm <id>
  plan --from YYYY-MM-DD --to YYYY-MM-DD   дни, не Unix ms
  goals [--day YYYY-MM-DD]
  month report <YYYY-MM> --project id [--rows week|day]
  month state <YYYY-MM> | close <YYYY-MM> [--sent] | reopen <YYYY-MM>   закрыть месяц для всех проектов
  settings                   настройки: сессия, паузы, начало суток, напоминания, скрытые заголовки
  settings set <json>        изменить часть настроек, например '{"pause_minutes":20}'
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
	case "guest":
		return runGuest(args[1:], stdout, stderr)
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
				Month  string `json:"month"`
			}
			if json.Unmarshal(resp, &e) != nil || e.Error == "" {
				e.Error = strings.TrimSpace(string(resp))
			}
			if e.Month != "" {
				e.Error += fmt.Sprintf(": месяц %s закрыт, herdr-time month reopen %s", e.Month, e.Month)
			} else if e.Period != 0 {
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
	monthArg := fs.String("month", "", "")
	format := fs.String("format", "text", "")
	var opts exportOpts
	fs.BoolVar(&opts.Notes, "notes", false, "")
	fs.BoolVar(&opts.Pay, "pay", false, "")
	fs.BoolVar(&opts.Agents, "agents", false, "")
	fs.BoolVar(&opts.Goal, "goal", false, "")
	fs.BoolVar(&opts.Commits, "commits", false, "")
	out := fs.String("out", "", "")
	fresh := fs.Bool("fresh", false, "")
	name := fs.String("name", "", "")
	days := fs.Int("days", 30, "")
	rows := fs.String("rows", "", "")
	sent := fs.Bool("sent", false, "")
	if err := fs.Parse(args); err != nil {
		return apiCall{}, err
	}
	pos := fs.Args()
	if cmd == "month" && len(pos) > 2 { // `month report 2026-11 --project P`: the flags come after the month
		if err := fs.Parse(pos[2:]); err != nil {
			return apiCall{}, err
		}
		pos = append(pos[:2:2], fs.Args()...)
	}
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
	case cmd == "settings" && sub == "set":
		body, err := arg(1, "<json>")
		return apiCall{method: "PUT", path: "/v1/settings", body: []byte(body), show: showSettings}, err
	case cmd == "settings" && sub == "":
		return apiCall{method: "GET", path: "/v1/settings", show: showSettings}, nil
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
	case cmd == "inbox" && sub == "apply":
		body, err := arg(1, "<json>")
		return apiCall{method: "POST", path: "/v1/inbox", body: []byte(body), show: showInboxApplied}, err
	case cmd == "inbox" && sub == "":
		if *day != "" {
			q.Set("day", *day)
		}
		return apiCall{method: "GET", path: "/v1/inbox?" + q.Encode(), show: showInbox}, nil
	case cmd == "export":
		body, err := json.Marshal(map[string]any{"project": *project, "month": *monthArg, "rows": *rows, "format": *format,
			"notes": opts.Notes, "pay": opts.Pay, "agents": opts.Agents, "goal": opts.Goal, "commits": opts.Commits})
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
	case cmd == "rules":
		return apiCall{method: "GET", path: "/v1/rules", show: showRules}, nil
	case cmd == "rule" && sub == "add":
		body, err := arg(1, "<json>")
		return apiCall{method: "POST", path: "/v1/rules", body: []byte(body)}, err
	case cmd == "rule" && sub == "set":
		id, err := arg(1, "<id>")
		if err != nil {
			return apiCall{}, err
		}
		body, err := arg(2, "<json>")
		return apiCall{method: "PUT", path: "/v1/rules/" + url.PathEscape(id), body: []byte(body)}, err
	case cmd == "rule" && sub == "rm":
		id, err := arg(1, "<id>")
		return apiCall{method: "DELETE", path: "/v1/rules/" + url.PathEscape(id)}, err
	case cmd == "rule" && sub == "preview":
		body, err := arg(1, "<json>")
		return apiCall{method: "POST", path: "/v1/rules/preview", body: []byte(body), show: showPreview}, err
	case cmd == "history" && sub == "undo":
		id, err := arg(1, "<id>")
		return apiCall{method: "POST", path: "/v1/history/" + url.PathEscape(id) + "/undo"}, err
	case cmd == "history":
		if _, _, err := span(); err != nil {
			return apiCall{}, err
		}
		return apiCall{method: "GET", path: "/v1/history?" + q.Encode(), show: showHistory}, nil
	case cmd == "commits":
		now := time.Now().UnixMilli()
		q.Set("from", strconv.FormatInt(now-int64(*days)*24*60*60_000, 10))
		q.Set("to", strconv.FormatInt(now, 10))
		return apiCall{method: "GET", path: "/v1/commits?" + q.Encode(), show: showCommits}, nil
	case cmd == "hints":
		q.Set("days", strconv.Itoa(*days))
		if *name != "" {
			q.Set("name", *name)
		}
		return apiCall{method: "GET", path: "/v1/hints?" + q.Encode(), show: showHints}, nil
	case cmd == "project" && sub == "add":
		body, err := arg(1, "<json>")
		return apiCall{method: "POST", path: "/v1/projects", body: []byte(body)}, err
	case cmd == "project" && sub == "set":
		id, err := arg(1, "<id>")
		if err != nil {
			return apiCall{}, err
		}
		body, err := arg(2, "<json>")
		return apiCall{method: "PATCH", path: "/v1/projects/" + url.PathEscape(id), body: []byte(body)}, err
	case (cmd == "goal" || cmd == "pay") && sub == "set":
		id, err := arg(1, "<project>")
		if err != nil {
			return apiCall{}, err
		}
		body, err := arg(2, "<json>")
		return apiCall{method: "PUT", path: "/v1/projects/" + url.PathEscape(id) + "/" + cmd, body: []byte(body)}, err
	case cmd == "bonus" && sub == "add":
		body, err := arg(1, "<json>")
		return apiCall{method: "POST", path: "/v1/bonuses", body: []byte(body)}, err
	case cmd == "bonus" && sub == "rm":
		id, err := arg(1, "<id>")
		return apiCall{method: "DELETE", path: "/v1/bonuses/" + url.PathEscape(id)}, err
	case cmd == "dayoff" && sub == "add":
		body, err := arg(1, "<json>")
		return apiCall{method: "POST", path: "/v1/daysoff", body: []byte(body)}, err
	case cmd == "dayoff" && sub == "rm":
		id, err := arg(1, "<id>")
		return apiCall{method: "DELETE", path: "/v1/daysoff/" + url.PathEscape(id)}, err
	case cmd == "plan":
		if *fromArg == "" || *toArg == "" {
			return apiCall{}, errors.New("--from and --to are required")
		}
		q.Set("from", *fromArg)
		q.Set("to", *toArg)
		return apiCall{method: "GET", path: "/v1/plan?" + q.Encode(), show: showPlan}, nil
	case cmd == "goals":
		if *day != "" {
			q.Set("day", *day)
		}
		return apiCall{method: "GET", path: "/v1/goals?" + q.Encode(), show: showGoals}, nil
	case cmd == "month" && sub == "report":
		month, err := arg(1, "<YYYY-MM>")
		if err != nil {
			return apiCall{}, err
		}
		q.Set("project", *project)
		if *rows != "" {
			q.Set("rows", *rows)
		}
		return apiCall{method: "GET", path: "/v1/months/" + url.PathEscape(month) + "/report?" + q.Encode(), show: showMonth}, nil
	case cmd == "month" && sub == "state":
		month, err := arg(1, "<YYYY-MM>")
		return apiCall{method: "GET", path: "/v1/months/" + url.PathEscape(month), show: showMonthState}, err
	case cmd == "month" && sub == "close":
		month, err := arg(1, "<YYYY-MM>")
		body, _ := json.Marshal(map[string]bool{"sent": *sent})
		return apiCall{method: "POST", path: "/v1/months/" + url.PathEscape(month) + "/close", body: body}, err
	case cmd == "month" && sub == "reopen":
		month, err := arg(1, "<YYYY-MM>")
		return apiCall{method: "POST", path: "/v1/months/" + url.PathEscape(month) + "/reopen"}, err
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
	srv := &http.Server{Handler: newServer(d), ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 10 * time.Second} // a stalled body would hold d.mu
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
