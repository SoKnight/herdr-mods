// Reads through the helper, which holds the `gh` calls. Authentication lives on
// this computer, including for remote spaces.
import { exec, appHome, window, writeTextFile } from "herdr";
import { duration, size } from "../settings.js";
import { runIdOf, searchQuery } from "./model.js";

const SEARCH_DEBOUNCE_MS = 450;
const LOG_TAIL_BYTES = 60000;
// Windows caps a command line at 32,767 characters, each quote and backslash escaped.
const COMMAND_LINE = 30000;

// The helper is always this computer's: an .exe on Windows.
const windows = () => window()?.platform === "windows";
const helperPath = () => `${appHome()}/mods/branch-pr/bin/herdr-pr${windows() ? ".exe" : ""}`;

// A request too long for the command line goes in a file beside the helper, which reads
// and removes it.
async function helperArgument(json) {
  if (!windows() || json.length + (json.match(/["\\]/g)?.length ?? 0) <= COMMAND_LINE) return json;
  const file = `${appHome()}/mods/branch-pr/bin/request-${Date.now().toString(36)}${Math.random().toString(36).slice(2, 8)}.json`;
  await writeTextFile(file, json, null);
  return `@${file}`;
}

export const readMethods = {
  searchChanged(cx) {
    const token = ++this.searchToken;
    this.rememberView();
    this.searchReloadNeeded = Boolean(this.searchReloadNeeded || this.loading);
    this.searchPending = true;
    // Invalidate older search responses immediately, including while waiting to debounce.
    this.listToken++;
    this.loading = false;
    cx.timer.after(duration(this.settings.refresh.search_debounce, SEARCH_DEBOUNCE_MS), (cx) => {
      if (token !== this.searchToken) return;
      const query = searchQuery(this.search.value());
      this.searchPending = false;
      if (this.searchReloadNeeded || query !== (this.serverSearch ?? "")) {
        this.serverSearch = query;
        this.loadList(cx);
      }
      cx.notify();
    });
    cx.notify();
  },

  async api(request, sp = this.sp) {
    request = { tools: this.settings.tools, ...request };
    // GitHub auth and the helper live on this computer, including for remote spaces.
    const result = await exec(
      helperPath(),
      [await helperArgument(JSON.stringify(request))],
      { cwd: sp?.machine ? undefined : sp?.folder },
    );
    let data;
    try {
      data = JSON.parse(result.stdout);
    } catch {
      throw new Error(result.stderr || "PR helper returned invalid JSON");
    }
    if (result.code !== 0 || data.error)
      throw new Error(data.error || result.stderr || `GitHub request failed`);
    return data;
  },

  listKey() {
    return `${this.spaceKey}:${this.repo}:${this.tab}:${this.state}:${this.serverSearch ?? ""}`;
  },

  loadList(cx, more = false) {
    if (!this.repo) return;
    this.searchReloadNeeded = false;
    this.rememberView();
    const token = ++this.listToken;
    const key = this.listKey();
    this.loading = true;
    this.error = null;
    // Reloading the list on screen re-reads page one and keeps further pages.
    // shortcut: an item that left the filter stays until the filter changes.
    const keep =
      !more &&
      this.cache[key]?.items === this.items &&
      this.items.length > this.settings.list.page_size;
    if (!more && !keep) {
      const cached = this.cache[key];
      this.items = cached?.items ?? [];
      this.cursors = cached?.cursors ?? {};
      this.total = cached?.total ?? null;
    }
    const request = {
      action: "list",
      repo: this.repo,
      tab: this.tab,
      state: this.state,
      search: this.serverSearch ?? "",
      cursors: more ? this.cursors : {},
      pageSize: this.settings.list.page_size,
    };
    cx.spawn(async (cx) => {
      try {
        const data = await this.api(request);
        if (token !== this.listToken) return;
        const merged = new Map((more || keep ? this.items : []).map((p) => [p.id, p]));
        for (const p of data.items) merged.set(p.id, p);
        this.items = [...merged.values()].sort((a, b) => b.updatedAt.localeCompare(a.updatedAt));
        if (!keep) this.cursors = data.cursors;
        this.total = data.total ?? null;
        this.cache[key] = { items: this.items, cursors: this.cursors, total: this.total };
        this.refreshed = Date.now();
        this.lastUpdated = Date.now();
        // Only the first time: a branch change should show the list, not an old PR.
        const restore = this.reopenSaved ? this.restoreSelection : null;
        this.reopenSaved = false;
        this.restoreSelection = null;
        if (restore && !this.pr) this.openPR(restore.number, cx, restore.repository ?? this.repo);
        else this.rememberView();
      } catch (e) {
        if (token === this.listToken) {
          this.error = e.message;
          this.refreshed = Date.now();
        }
      } finally {
        if (token === this.listToken) {
          this.loading = false;
          this.listLoaded = true;
          cx.notify();
        }
      }
    });
  },

  // A PR's repository can differ from the list's, for one from another remote.
  openPR(number, cx, repo = this.repo) {
    this.pendingPR = { number };
    const token = ++this.detailToken;
    const key = this.spaceKey;
    this.detailLoading = true;
    this.error = null;
    cx.spawn(async (cx) => {
      try {
        const pr = await this.api({ action: "detail", repo, number });
        if (token !== this.detailToken || key !== this.spaceKey) return;
        this.pendingPR = null;
        this.adoptPR(pr, repo);
        this.refreshed = Date.now();
        this.loadWorktrees(cx);
        if (this.section === "files") this.loadFiles(cx);
      } catch (e) {
        if (token === this.detailToken) {
          this.error = e.message;
          this.refreshed = Date.now();
        }
      } finally {
        if (token === this.detailToken) {
          this.detailLoading = false;
          this.opening = null;
          cx.notify();
        }
      }
    });
  },

  refresh(cx) {
    this.loadBranchPRs(cx);
    this.resolveRepo(cx);
    if (this.pr) this.openPR(this.pr.number, cx, this.pr.repository);
  },

  loadJobs(check, cx) {
    const id = runIdOf(check);
    if (!id) return;
    const key = this.spaceKey;
    this.jobs[id] = { loading: true };
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        const pages = await this.api({ action: "jobs", repo: this.pr.repository, run: Number(id) });
        if (key === this.spaceKey) this.jobs[id] = { jobs: pages.flatMap((p) => p.jobs ?? []) };
      } catch (e) {
        if (key === this.spaceKey) this.jobs[id] = { error: e.message };
      }
      cx.notify();
    });
  },

  loadLogs(job, cx) {
    const key = this.spaceKey;
    this.logs[job.id] = { loading: true };
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        const data = await this.api({
          action: "logs",
          repo: this.pr.repository,
          job: job.id,
          tail: size(this.settings.logs.keep_last, LOG_TAIL_BYTES),
        });
        if (key === this.spaceKey) this.logs[job.id] = data;
      } catch (e) {
        if (key === this.spaceKey) this.logs[job.id] = { error: e.message };
      }
      cx.notify();
    });
  },
};
