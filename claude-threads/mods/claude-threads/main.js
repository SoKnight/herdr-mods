// Claude Threads: past Claude Code sessions, newest first, for the shown space's repo (or every
// folder). Click one to resume it in a new tab of that space, in the folder it ran in; shift-click
// forks it instead. A session already running in a terminal is focused, not started twice.

import { View } from "gpui-kit";
import { v_flex, h_flex, InputState } from "gpui-base";
import { exec, hc, spaces, panes, agents, run, theme, home } from "herdr";
import { readFile, readdir, exists } from "fs/promises";
import { sessionsFromHistory, projectDir, searchSessions, openSessions, oneLine, ago } from "./threads.js";
import { insidePath, joinPath, baseName, nativePath } from "./paths.js";
import { Panel, PanelHeader, HeaderTab, IconButton, TextField, ItemRow, ClaudeIcon, Badge, Busy, Message, Footer } from "./ui.js";

const REFRESH = 60000;

// Where a space is now: its first terminal's live folder, else where it was made (as the
// spaces sidebar sees it).
function place(s, ps) {
  const tabs = new Set((s.tabs ?? []).map((c) => c.id));
  return ps.find((p) => tabs.has(p.tab) && p.cwd)?.cwd || s.folder;
}

// Claude Code agents on this computer: their sessions are the ones this panel can see.
const claudes = () => agents().filter((a) => a.app === "claude-code" && !a.address.includes("/"));

export default class Threads extends View {
  init(_props, cx) {
    this.claude = joinPath(home(), ".claude");
    this.sessions = [];
    this.open = new Map(); // session id -> terminal address
    this.openKey = "";
    this.error = null;
    this.loaded = false;
    this.loading = false;
    this.scope = "space"; // or "all"
    this.roots = {}; // folder -> its repo root (the folder itself outside a repo)
    this.found = { key: null, rows: [] };
    this.search = InputState.new({ placeholder: "Search prompts" });
    this.search.on("change", (_e, cx) => cx.notify());
    this.refresh(cx);
    cx.timer.every(REFRESH, (cx) => this.refresh(cx));
  }

  refresh(cx) {
    if (this.loading) return;
    this.loading = true;
    cx.spawn(async (cx) => {
      try {
        const [history, files] = await Promise.all([readFile(joinPath(this.claude, "history.jsonl"), "utf8"), this.sessionFiles()]);
        // Claude deletes old session files, and a session whose folder is gone can't resume:
        // either way a click would open a tab that closes at once, so those aren't listed.
        const folders = new Map();
        const resumable = [];
        for (const s of sessionsFromHistory(history)) {
          if (!s.project || !files.has(`${projectDir(s.project)}/${s.id}.jsonl`)) continue;
          if (!folders.has(s.project)) folders.set(s.project, await exists(s.project).catch(() => false));
          if (folders.get(s.project)) resumable.push(s);
        }
        this.sessions = resumable;
        this.found = { key: null, rows: [] };
        this.error = null;
        this.updated = Date.now();
      } catch (e) {
        this.error = String(e?.message ?? e);
      }
      this.loaded = true;
      this.loading = false;
      cx.notify();
    });
    this.refreshOpen(cx);
  }

  // Every "<project folder>/<session>.jsonl" under ~/.claude/projects.
  async sessionFiles() {
    const root = joinPath(this.claude, "projects");
    const files = new Set();
    for (const dir of await readdir(root, { withFileTypes: true }).catch(() => [])) {
      if (!dir.isDirectory()) continue;
      for (const f of await readdir(joinPath(root, dir.name)).catch(() => [])) files.add(`${dir.name}/${f}`);
    }
    return files;
  }

  // Which session each Claude terminal runs: Claude writes ~/.claude/sessions/<pid>.json while
  // it runs, and herdr-core knows each terminal's foreground pid.
  refreshOpen(cx) {
    const live = claudes();
    this.openKey = live.map((a) => a.address).join(" ");
    cx.spawn(async (cx) => {
      const dir = joinPath(this.claude, "sessions");
      const registry = [];
      for (const f of await readdir(dir).catch(() => [])) {
        if (!f.endsWith(".json")) continue;
        try {
          registry.push(JSON.parse(await readFile(joinPath(dir, f), "utf8")));
        } catch {}
      }
      const terminals = [];
      for (const a of live) {
        try {
          const r = await hc(["get", a.address, "--json"]);
          terminals.push({ address: a.address, pid: JSON.parse(r.stdout).foreground?.pid ?? null });
        } catch {}
      }
      this.open = openSessions(registry, terminals);
      cx.notify();
    });
  }

  // The repo root of `dir`, looked up once; `dir` itself until known, and outside a repo.
  root(dir, cx) {
    if (dir in this.roots) return this.roots[dir];
    this.roots[dir] = dir;
    cx.spawn(async (cx) => {
      const r = await exec("git", ["-C", dir, "rev-parse", "--show-toplevel"]).catch(() => null);
      if (r?.code === 0 && r.stdout.trim()) {
        this.roots[dir] = r.stdout.trim();
        cx.notify();
      }
    });
    return dir;
  }

  // Search runs over every prompt, so it's redone only when the query or the list changes,
  // not on each of Herdr's live re-renders.
  matches() {
    const query = this.search.value();
    if (this.found.key !== query) this.found = { key: query, rows: searchSessions(this.sessions, query) };
    return this.found.rows;
  }

  resume(s, fork) {
    const address = this.open.get(s.id);
    if (address && claudes().some((a) => a.address === address)) return run("pane.focus", { terminal: address });
    const argv = ["claude", "--resume", s.id, ...(fork ? ["--fork-session"] : [])];
    const folder = nativePath(s.project);
    const space = spaces().find((x) => x.active);
    // Sessions are this computer's: a space on another machine can't run them.
    if (!space || space.machine) return run("space.new", { folder, argv, machine: "" });
    run("tab.new", { space: space.id, folder, argv, name: oneLine(s.first).slice(0, 28) || undefined });
  }

  header(here, mine, all) {
    const tab = (id, label, count) =>
      HeaderTab({
        id: `threads-${id}`,
        label,
        count,
        active: this.scope === id,
        onClick: (cx) => {
          this.scope = id;
          cx.notify();
        },
      });
    return PanelHeader()
      .child(
        h_flex()
          .flex_1()
          .min_w(0)
          .ml(-8) // Lines the pills up with the rows below.
          .gap(4)
          .items_center()
          .role("tab_list")
          .child(tab("space", here ? baseName(here) : "This space", here ? mine : null))
          .child(tab("all", "All", all)),
      )
      .child(IconButton({ id: "threads-refresh", icon: "refresh", label: "Refresh sessions", spinning: this.loading, onClick: (cx) => this.refresh(cx) }));
  }

  row(t, { session: s, hit }) {
    const open = this.open.has(s.id);
    const detail = oneLine(hit ?? s.last);
    const meta = [
      ago(s.updated, Date.now()),
      this.scope === "all" ? baseName(s.project) : null,
      `${s.count} ${s.count === 1 ? "prompt" : "prompts"}`,
      detail && detail !== oneLine(s.first) ? detail : null,
    ];
    return ItemRow({
      id: `th-${s.id}`,
      icon: ClaudeIcon(),
      title: oneLine(s.first) || "(untitled)",
      // Running in a tab now: a click goes there instead of starting it twice.
      badge: open ? Badge("Open", t.working).tooltip("Running in a tab: click to go to it") : null,
      meta: meta.filter(Boolean).join(" · "),
      code: s.id.slice(0, 8),
      onClick: (e) => this.resume(s, e.modifiers?.shift),
    });
  }

  render(cx) {
    const t = theme();
    if (claudes().map((a) => a.address).join(" ") !== this.openKey) this.refreshOpen(cx);
    const space = spaces().find((s) => s.active);
    const here = space ? this.root(place(space, panes()), cx) : null;
    const rows = this.matches();
    const mine = here ? rows.filter((r) => insidePath(r.session.project, here)) : [];
    const shown = this.scope === "space" ? mine : rows;
    const searching = Boolean(this.search.value().trim());

    let body;
    if (this.error) body = v_flex().flex_1().p(6).child(Message(this.error, true));
    else if (!this.loaded) body = Busy("Loading sessions…");
    else {
      body = v_flex().id(`threads-list-${this.scope}`).flex_1().min_h(0).overflow_y_scrollbar().p(6).gap(2);
      if (!shown.length)
        body.child(Message(searching ? "No matching sessions." : this.scope === "space" ? `No sessions in ${here ? baseName(here) : "this space"} yet.` : "No sessions yet."));
      else body.children(shown.map((r) => this.row(t, r)));
    }

    return Panel({ id: "threads-root" })
      .child(this.header(here, mine.length, rows.length))
      .child(
        v_flex()
          .id("threads-search-row")
          .flex_none()
          .px(14)
          .py(10)
          .border_b(1)
          .border_color(t.border)
          .child(h_flex().child(TextField({ id: "threads-search", state: this.search }))),
      )
      .child(body)
      .child(
        Footer({
          left: "Click resumes · Shift-click forks",
          right: this.loading ? "Updating…" : this.updated ? `Updated ${ago(this.updated, Date.now())}` : "",
          tooltip: this.updated ? new Date(this.updated).toLocaleString() : "",
        }),
      );
  }
}
