// The world outside Herdr: code editors, Finder or Explorer, and the clipboard. This computer
// only: a path on another machine means nothing to an app here.
import { appHome, exec, knownAgents, readTextFile, setShared, shared, window } from "herdr";
import { EDITORS, WINDOWS_EDITORS } from "../files/model.js";
import { nativePath, psQuote } from "../paths.js";

const FOUND = "better-files.editors";
const ERROR = "better-files.error";

export const foundEditors = () => shared(FOUND) ?? [];
// macOS knows an editor by its app ID, Windows by the program that starts it.
export const editorId = (editor) => editor.id ?? editor.bundle;
export const fileManager = () =>
  ({ macos: "Finder", windows: "File Explorer" })[window()?.platform] ?? "file manager";

// A PowerShell script, its arguments as $args. Each is a quoted string after the script, so no
// name can become code; the script itself has no double quotes, which Windows would escape.
// UTF-8 out, since exec reads UTF-8 and Windows PowerShell writes the console's code page.
export function powershell(script, args = [], options = {}) {
  const command = `[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false); $ErrorActionPreference = 'Stop'; & { ${script} } ${args.map(psQuote).join(" ")}`;
  return exec("powershell", ["-NoProfile", "-NonInteractive", "-Command", command], options);
}

// Each editor's first file that exists under the install folders (newest last, so the last
// match of a wildcard), else its command on PATH. Prints {name: program}.
const FIND_EDITORS = `$roots = @((Join-Path $env:LOCALAPPDATA 'Programs'), $env:ProgramFiles, (Join-Path $env:ProgramFiles 'JetBrains'), (Join-Path $env:LOCALAPPDATA 'JetBrains\\Toolbox\\scripts')) | Where-Object { $_ }; $out = [ordered]@{}; foreach ($e in $args) { $f = $e -split '\\|'; $hit = $null; foreach ($rel in ($f[2] -split ';' | Where-Object { $_ })) { foreach ($r in $roots) { if (-not $hit) { $hit = Get-Item -Path (Join-Path $r $rel) -ErrorAction SilentlyContinue | Select-Object -Last 1 -ExpandProperty FullName } } }; if (-not $hit -and $f[1]) { $hit = Get-Command $f[1] -CommandType Application -ErrorAction SilentlyContinue | Select-Object -First 1 -ExpandProperty Source }; if ($hit) { $out[$f[0]] = $hit } }; ConvertTo-Json -Compress -InputObject $out`;

// A script (.cmd, as code.cmd and Toolbox's) starts hidden, or a console flashes; an app
// starts as it is. Start-Process returns at once: exec would wait for the editor to quit.
const LAUNCH = `$p = [char]34 + $args[1] + [char]34; if ($args[0] -match '\\.(cmd|bat)$') { Start-Process -FilePath $args[0] -ArgumentList $p -WindowStyle Hidden } else { Start-Process -FilePath $args[0] -ArgumentList $p }`;

// Quoted for a command line, where a backslash before the closing quote would escape it.
const launchPath = (path) => {
  const native = nativePath(path);
  return native.endsWith("\\") ? `${native}.` : native;
};

export const outsideMethods = {
  // One lookup per app ID, done once: Spotlight knows apps wherever they're installed.
  findEditors(cx, extra = []) {
    if (window()?.platform === "windows") return this.findWindowsEditors(cx, extra);
    cx.spawn(async (cx) => {
      const found = [];
      for (const editor of [...EDITORS, ...extra]) {
        const r = await exec(
          "mdfind",
          [`kMDItemCFBundleIdentifier == '${String(editor.bundle).replace(/'/g, "")}'`],
          {},
        ).catch(() => null);
        if (r?.code === 0 && r.stdout.trim()) found.push(editor);
      }
      setShared(FOUND, found);
      cx.notify();
    });
  },

  // One PowerShell for every editor. Editors from settings need a `command` here.
  findWindowsEditors(cx, extra) {
    const wanted = [...WINDOWS_EDITORS, ...extra.filter((e) => e?.name && e.command)];
    cx.spawn(async (cx) => {
      const specs = wanted.map((e) => `${e.name}|${e.command}|${(e.paths ?? []).join(";")}`);
      const r = await powershell(FIND_EDITORS, specs).catch(() => null);
      let programs = {};
      try {
        if (r?.code === 0) programs = JSON.parse(r.stdout || "{}") ?? {};
      } catch {}
      const found = wanted
        .filter((e) => programs[e.name])
        .map((e) => ({ name: e.name, id: e.name, command: programs[e.name] }));
      setShared(FOUND, found);
      cx.notify();
    });
  },

  // Show diff is only offered where Better Changes can answer it.
  findBetterChanges(cx) {
    cx.spawn(async (cx) => {
      const manifest = await readTextFile(`${appHome()}/mods/better-changes/mod.json`).catch(
        () => null,
      );
      setShared("better-files.has-changes", Boolean(manifest));
      cx.notify();
    });
  },

  findNewAgents(on, cx) {
    cx.spawn(async (cx) => {
      try {
        const known = await knownAgents({ on: on ?? null });
        setShared(
          "better-files.new-agents",
          known.filter((a) => a.installed).map((a) => ({ key: a.key, name: a.name, run: a.run })),
        );
      } catch {
        setShared("better-files.new-agents", []);
      }
      cx.notify();
    });
  },

  async report(running, what) {
    const r = await running.catch((e) => ({ code: 1, stderr: String(e) }));
    if (r.code !== 0) setShared(ERROR, `Could not ${what}: ${r.stderr?.trim() || "it failed"}`);
    else setShared(ERROR, null);
  },

  outside(args, what) {
    return this.report(exec("open", args, {}), what);
  },

  openInEditor(editor, path) {
    const what = `open it in ${editor.name}`;
    if (window()?.platform === "windows") return this.report(powershell(LAUNCH, [editor.command, launchPath(path)]), what);
    return this.outside(["-b", editor.bundle, path], what);
  },

  // Invoke-Item opens a file with its app and a folder in File Explorer.
  openWithDefault(path) {
    if (window()?.platform === "windows")
      return this.report(powershell("Invoke-Item -LiteralPath $args[0]", [nativePath(path)]), "open it");
    return this.outside([path], "open it");
  },

  // Explorer exits 1 even when it worked, so only a failure to start counts.
  revealInFinder(path) {
    if (window()?.platform === "windows")
      return this.report(
        exec("explorer.exe", ["/select,", nativePath(path)], {}).then(() => ({ code: 0 })),
        "show it in File Explorer",
      );
    return this.outside(["-R", path], "show it in Finder");
  },
};
