// Reading the folder tree, and Git's view of it, for the focused pane's folder.
import { spaces, panes, focused, exec, watchFolder, window } from "herdr";
import { PER_FOLDER, parseStatus, sortEntries } from "./model.js";
import { powershell } from "../actions/outside.js";
import { insidePath, isWindowsPath, joinPath, nativePath, normPath } from "../paths.js";

const windowsHere = (on) => !on && window()?.platform === "windows";
const WATCH_SETTLE_MS = 500;

// Each read of a folder is bounded, and the names come back null-separated, so no file name
// can break the list. Adapted from Herdr's shipped Files mod.
async function directory(path, on) {
  const script = `n=0; for p in "$1"/* "$1"/.[!.]* "$1"/..?*; do [ -e "$p" ] || [ -L "$p" ] || continue; name=\${p##*/}; [ "$name" = .git ] && continue; if [ -d "$p" ]; then kind=d; else kind=f; fi; printf "%s\\0%s\\0" "$kind" "$name"; n=$((n+1)); [ "$n" -gt ${PER_FOLDER} ] && break; done; test -d "$1"`;
  const r = await exec("sh", ["-c", script, "files", path], { on });
  if (r.code !== 0) throw new Error(r.stderr || "Cannot list this folder.");
  const fields = r.stdout.split("\0");
  const out = [];
  for (let i = 0; i + 1 < fields.length; i += 2)
    out.push({ name: fields[i + 1], dir: fields[i] === "d" });
  return out;
}

// Windows has no `sh`. Starting PowerShell takes a third of a second, so one start lists every
// folder asked for. As in Herdr's Files mod: a link is a folder when its target is, any other
// reparse point (OneDrive's folders) is a folder, and .git is left out.
const LIST = `ConvertTo-Json -Compress -Depth 4 -InputObject @(foreach ($p in $args) { try { if (-not (Test-Path -LiteralPath $p -PathType Container)) { throw ('Not a folder: ' + $p) }; $e = @(Get-ChildItem -LiteralPath $p -Force | Where-Object Name -ne '.git' | Select-Object -First ${PER_FOLDER + 1} | ForEach-Object { @{ name = $_.Name; dir = [bool]($_.PSIsContainer -and (-not ($_.Attributes -band [IO.FileAttributes]::ReparsePoint) -or -not $_.LinkType -or ($_.Target -and [IO.Directory]::Exists(([IO.Path]::Combine([IO.Path]::GetDirectoryName($_.FullName), $_.Target[0])))))) } }); @{ entries = $e } } catch { @{ error = $_.Exception.Message } } })`;

async function windowsDirectories(paths, on) {
  const r = await powershell(LIST, paths.map(nativePath), { on });
  if (r.code !== 0) throw new Error(r.stderr || "Cannot list this folder.");
  return JSON.parse(r.stdout).map((result) => {
    if (result.error) throw new Error(result.error);
    return result.entries ?? [];
  });
}

// Each folder's entries, in order. Another machine without `sh` is a Windows one.
async function directories(paths, on) {
  if (windowsHere(on)) return windowsDirectories(paths, on);
  const lists = [];
  for (const path of paths) {
    try {
      lists.push(await directory(path, on));
    } catch (e) {
      if (on && String(e).includes("cannot start sh: program not found")) return windowsDirectories(paths, on);
      throw e;
    }
  }
  return lists;
}

// Git spells a Windows repository C:/x where Herdr says C:\x, maybe in another case. Marks
// are keyed by the tree's own spelling, so the top is taken from the folder on show.
function repositoryTop(root, gitTop) {
  const top = normPath(gitTop);
  return isWindowsPath(root) && insidePath(root, top) ? root.slice(0, top.length) : top;
}

export const readMethods = {
  // The focused pane's folder. A file or diff tab has no folder of its own, so looking at
  // one keeps the tree where it was instead of jumping back to the space's.
  location() {
    const space = spaces().find((s) => s.active);
    const f = focused();
    const pane = f?.pane ? panes().find((p) => p.pane === f.pane) : null;
    const on =
      f?.pane === null
        ? f.terminal?.includes("/")
          ? f.terminal.split("/")[0]
          : null
        : (space?.machine ?? null);
    // One spelling for a Windows folder, so C:\x and Git's C:/x are the same place.
    const cwd = normPath(pane?.cwd || f?.cwd);
    if (!cwd && this.place?.space === space?.id && this.place?.cwd) return this.place;
    return (this.place = { cwd: cwd || normPath(space?.folder) || space?.folder, on, space: space?.id });
  },

  placeKey() {
    return JSON.stringify(this.location());
  },

  // A folder's own state is kept, so coming back to it is where you left it.
  refresh(cx) {
    const { cwd, on } = this.location();
    const key = this.placeKey();
    if (key !== this.key) {
      if (this.key) this.saved.set(this.key, { folders: this.folders });
      const kept = this.saved.get(key);
      this.key = key;
      this.root = cwd;
      this.on = on;
      this.entries = new Map();
      this.folders = kept?.folders ?? new Set();
      this.marks = {};
      this.top = null;
      this.treeError = null;
      this.gitProblem = null;
      this.watch(cx);
    }
    if (!cwd || this.loading === key) return;
    const token = key;
    this.loading = key;
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        const paths = ["", ...this.folders];
        // One PowerShell for every folder here on Windows; elsewhere each folder in turn, so
        // the ones before a folder that can't be read still refresh.
        if (windowsHere(on)) {
          const lists = await windowsDirectories(paths.map((path) => joinPath(cwd, path)), on);
          if (this.key !== token) return;
          paths.forEach((path, i) => this.entries.set(path, sortEntries(lists[i])));
        } else
          for (const path of paths) {
            if (this.key !== token) return;
            await this.loadFolder(path, cwd, on, token);
          }
        await this.loadMarks(cwd, on, token);
        if (this.key === token) this.treeError = null;
      } catch (e) {
        if (this.key === token) this.treeError = e.message ?? String(e);
      } finally {
        if (this.loading === token) this.loading = null;
        cx.notify();
      }
    });
  },

  async loadFolder(path, root, on, token) {
    const [list] = await directories([joinPath(root, path)], on);
    if (this.key !== token) return;
    this.entries.set(path, sortEntries(list));
  },

  // Writes nothing under .git, or the folder watch would see its own read and refresh again.
  async loadMarks(root, on, token) {
    const top = await exec("git", ["rev-parse", "--show-toplevel"], { cwd: root, on }).catch(
      () => null,
    );
    if (this.key !== token) return;
    // Git refuses a repository it thinks another user owns, common on Windows for one on
    // another drive or share: its own words say how to allow it.
    this.gitProblem = /dubious ownership/.test(top?.stderr ?? "") ? top.stderr.trim() : null;
    if (top?.code !== 0) {
      this.top = null;
      this.marks = {};
      return;
    }
    const status = await exec(
      "git",
      [
        "--no-optional-locks",
        "-c",
        "core.fsmonitor=false",
        "status",
        "--porcelain=v1",
        "-z",
        "--no-renames",
        "--untracked-files=all",
      ],
      { cwd: root, on },
    ).catch(() => null);
    if (this.key !== token) return;
    this.top = repositoryTop(root, top.stdout.trim());
    if (status?.code === 0) this.marks = parseStatus(status.stdout, this.top);
  },

  // A local folder says when it changes; another machine's is read on the timer alone.
  watch(cx) {
    const { root, on, key } = this;
    if (on || !root || this.watched === key) return;
    this.watched = key;
    cx.spawn(async (cx) => {
      try {
        while (this.key === key) {
          await watchFolder(root);
          // Windows watches every folder below, node_modules too: let a burst settle first.
          if (window()?.platform === "windows") await new Promise((resolve) => cx.timer.after(WATCH_SETTLE_MS, resolve));
          if (this.key === key) this.refresh(cx);
        }
      } catch {
        // No watcher here: the timer still refreshes.
      }
      if (this.watched === key) this.watched = null;
    });
  },

  toggleFolder(path, cx) {
    if (!this.folders.delete(path)) {
      this.folders.add(path);
      const { root, on, key } = this;
      cx.spawn(async (cx) => {
        try {
          await this.loadFolder(path, root, on, key);
        } catch (e) {
          if (this.key === key) this.treeError = e.message ?? String(e);
        }
        cx.notify();
      });
    }
    cx.notify();
  },

};
