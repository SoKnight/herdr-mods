package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"strings"
	"time"
)

const usage = `herdr-time <команда> [--json]
  run                        запустить сборщик
  probe                      один опрос Windows, без сборщика
  status
  timeline [--day YYYY-MM-DD]
  report --from --to [--project id]
  export --from --to --project id [--format md|csv] [--round 1|5|15] [--agents] [--out файл]
  correct add <json> | rm <id> | ls
  beat <json>
  unassigned --from --to
  rule add <json>
  periods [--project id]
  reopen <id>
--from и --to: Unix ms или YYYY-MM-DD (день в --to входит в диапазон).`

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
		fmt.Fprintln(stderr, err)
		return 1
	}
	switch args[0] {
	case "run":
		return runCollector(s, stdout, stderr)
	case "probe":
		return probe(stdout, stderr)
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
		return apiCall{method: "GET", path: "/v1/timeline?" + q.Encode(), show: showTimeline}, nil
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
		from, to, err := span()
		if err != nil {
			return apiCall{}, err
		}
		body, err := json.Marshal(map[string]any{"project": *project, "from": from, "to": to,
			"format": *format, "round": *round, "agents": *agents})
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
		resp, code, err := send(s, "GET", "/v1/status", nil)
		var st struct {
			Version string `json:"version"`
		}
		if err == nil && code == http.StatusOK && json.Unmarshal(resp, &st) == nil && st.Version != "" {
			fmt.Fprintln(stdout, "уже запущен")
			return 0
		}
		fmt.Fprintf(stderr, "порт %d занят другим процессом\n", s.Port)
		return 1
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
