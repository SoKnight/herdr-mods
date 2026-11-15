// What the folder, Git and editors say, as plain data. No drawing and no side effects.
import { baseName, isWindowsPath, joinPath, normPath } from "../paths.js";

export const PER_FOLDER = 500;
export const TOTAL_ROWS = 5000;
export const MAX_CHARS = 400000;

// Editors found by app ID: a Nix or per-user install isn't in /Applications.
// More can be added under `settings.editors` in mod.json.
export const EDITORS = [
  { name: "Zed", bundle: "dev.zed.Zed" },
  { name: "VS Code", bundle: "com.microsoft.VSCode" },
  { name: "Cursor", bundle: "com.todesktop.230313mzl4w4u92" },
  { name: "Windsurf", bundle: "com.exafunction.windsurf" },
  { name: "Sublime Text", bundle: "com.sublimetext.4" },
  { name: "Nova", bundle: "com.panic.Nova" },
];

// Windows has no app IDs: each editor is the first of its `paths` (wildcards allowed) under
// the usual install folders, else its `command` on PATH. JetBrains IDEs, as Toolbox and the
// installer put them, and Toolbox's own scripts. More go under `settings.editors` with a
// `command`: a program on PATH or a full path.
export const WINDOWS_EDITORS = [
  { name: "VS Code", command: "code", paths: ["Microsoft VS Code\\Code.exe"] },
  { name: "Cursor", command: "cursor", paths: ["cursor\\Cursor.exe"] },
  { name: "Windsurf", command: "windsurf", paths: ["Windsurf\\Windsurf.exe"] },
  { name: "Zed", command: "zed", paths: ["Zed\\bin\\zed.exe", "Zed\\Zed.exe"] },
  { name: "Sublime Text", command: "subl", paths: ["Sublime Text\\subl.exe", "Sublime Text 3\\subl.exe"] },
  { name: "IntelliJ IDEA", command: "idea", paths: ["IntelliJ IDEA*\\bin\\idea64.exe", "idea.cmd"] },
  { name: "PyCharm", command: "pycharm", paths: ["PyCharm*\\bin\\pycharm64.exe", "pycharm.cmd"] },
  { name: "WebStorm", command: "webstorm", paths: ["WebStorm*\\bin\\webstorm64.exe", "webstorm.cmd"] },
  { name: "GoLand", command: "goland", paths: ["GoLand*\\bin\\goland64.exe", "goland.cmd"] },
  { name: "CLion", command: "clion", paths: ["CLion*\\bin\\clion64.exe", "clion.cmd"] },
  { name: "Rider", command: "rider", paths: ["*Rider*\\bin\\rider64.exe", "rider.cmd"] },
  { name: "RustRover", command: "rustrover", paths: ["RustRover*\\bin\\rustrover64.exe", "rustrover.cmd"] },
];

const LANGUAGES = {
  ts: "js",
  tsx: "js",
  mts: "js",
  cts: "js",
  jsx: "js",
  mjs: "js",
  cjs: "js",
  toml: "yaml",
  jsonc: "json",
  svelte: "html",
  vue: "html",
};

// A drive's own folder keeps its slash: C:/a.md is in C:/.
export const parentOf = (path) => {
  const parent = path.slice(0, path.lastIndexOf("/"));
  return /^[A-Za-z]:$/.test(parent) ? `${parent}/` : parent || "/";
};

const extOf = (path) => {
  const name = baseName(path);
  return name.includes(".") ? name.split(".").pop().toLowerCase() : "";
};
const IMAGES = new Set(["png", "jpg", "jpeg", "gif", "webp", "bmp", "svg"]);
export const isImage = (path) => IMAGES.has(extOf(path));
export const isMarkdown = (path) => ["md", "markdown"].includes(extOf(path));

// A link or image source in a Markdown file as an absolute path, from the file's folder.
// Web addresses and other schemes aren't paths, so they come back null.
// A Windows file's links may use either separator or start with a drive (C:\x.png,
// file:///C:/x.png); a drive is a path, not a scheme.
export function resolvePath(folder, link) {
  let rel = String(link ?? "")
    .replace(/^file:\/\/\/(?=[A-Za-z]:)/i, "")
    .replace(/^file:\/\//, "");
  const drive = isWindowsPath(rel);
  if (!drive && /^[a-z][a-z0-9+.-]*:/i.test(rel)) return null;
  rel = rel.replace(/[?#].*$/, "");
  try {
    rel = decodeURIComponent(rel);
  } catch {}
  if (!rel) return null;
  const windows = drive || isWindowsPath(folder);
  if (windows) rel = rel.replace(/\\/g, "/");
  const parts = drive ? [] : rel.startsWith("/") ? [""] : normPath(folder).split("/");
  for (const part of rel.split("/")) {
    if (part === "..") parts.length > 1 && parts.pop();
    else if (part && part !== ".") parts.push(part);
  }
  const path = parts.join("/") || "/";
  return windows ? normPath(path) : path;
}

// A line that is only an image, not indented under a list item: `![alt](src)` or `![alt](<src> "title")`.
const IMAGE_LINE = /^!\[([^\]]*)\]\(\s*(?:<([^>]*)>|([^)\s]+))(?:\s+"[^"]*")?\s*\)\s*$/;
const FENCE = /^ {0,3}(`{3,}|~{3,})/;

// Markdown as blocks for the preview: Herdr's TextView draws the text, and an image on a
// line of its own, from this computer, becomes an image block so it can be shown through
// the cache like the image viewer. Code fences are kept whole, so an image in one stays text.
// shortcut: an image inside a paragraph, list or table, or from the web, is left to TextView;
// split those out too if they need to show.
export function markdownBlocks(text, folder) {
  const blocks = [];
  let lines = [];
  let fence = null;
  const flush = () => {
    if (lines.some((line) => line.trim())) blocks.push({ type: "text", text: lines.join("\n") });
    lines = [];
  };
  for (const line of String(text ?? "").split("\n")) {
    const opens = FENCE.exec(line);
    if (fence) {
      if (opens && opens[1][0] === fence[0] && opens[1].length >= fence.length && !line.slice(opens[0].length).trim()) fence = null;
    } else if (opens) fence = opens[1];
    else {
      const image = IMAGE_LINE.exec(line);
      const path = image && resolvePath(folder, image[2] ?? image[3]);
      if (path && isImage(path)) {
        flush();
        blocks.push({ type: "image", alt: image[1], path });
        continue;
      }
    }
    lines.push(line);
  }
  flush();
  return blocks;
}

export function languageOf(path) {
  const name = baseName(path);
  const ext = name.includes(".") ? name.split(".").pop() : "txt";
  return LANGUAGES[ext] ?? ext;
}

// Directories first, each kind by name, the way a file browser reads.
export function sortEntries(entries) {
  return [...entries].sort(
    (a, b) => Number(b.dir) - Number(a.dir) || a.name.localeCompare(b.name),
  );
}

// `git status --porcelain=v1 -z`, keyed by absolute path. `staged` is true when the only
// change is in the index, which is where a diff of it has to look.
export function parseStatus(stdout, top) {
  const marks = {};
  for (const entry of stdout.split("\0").filter(Boolean).slice(0, TOTAL_ROWS)) {
    const xy = entry.slice(0, 2);
    const untracked = xy === "??";
    marks[joinPath(top, entry.slice(3))] = {
      mark: untracked || xy.includes("A") ? "A" : xy.includes("U") ? "U" : xy.includes("D") ? "D" : "M",
      staged: !untracked && xy[0] !== " " && xy[1] === " ",
    };
  }
  return marks;
}

// `git diff -U0` as changes, by 0-based line in the file as it is now, each with the old
// lines it replaced. A hunk with new lines and none removed is added; with both, changed;
// with only removed lines, there's no line to colour, so it marks the place they were.
export function parseHunks(diff) {
  const hunks = [];
  let current = null;
  for (const text of diff.split("\n")) {
    const header = /^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@/.exec(text);
    if (header) {
      const [, oldStart, removed, start, added] = header;
      const gone = removed === undefined ? 1 : Number(removed);
      const fresh = added === undefined ? 1 : Number(added);
      current = {
        line: fresh ? Number(start) - 1 : Number(start),
        count: fresh,
        kind: !fresh ? "removed" : gone ? "changed" : "added",
        oldStart: Number(oldStart),
        old: [],
      };
      hunks.push(current);
    } else if (current && text.startsWith("-")) current.old.push(text.slice(1));
  }
  return hunks;
}

const cleanLine = (line) => line.replace(/\r$/, "").replace(/\t/g, "    ");
export const fileLinesOf = (text) => text.split("\n").map(cleanLine);

// What to draw, top to bottom: the file's lines, and where a change is open, the lines it
// replaced. A closed removal is a thin row of its own, so it can be clicked without a line.
export function buildRows(lines, hunks, open) {
  const covering = new Map();
  const startingAt = new Map();
  const removedAt = new Map();
  for (const hunk of hunks) {
    if (hunk.kind === "removed") removedAt.set(Math.min(hunk.line, lines.length), hunk);
    else {
      startingAt.set(hunk.line, hunk);
      for (let i = hunk.line; i < Math.min(hunk.line + (hunk.count ?? lines.length), lines.length); i++)
        covering.set(i, hunk);
    }
  }
  const rows = [];
  const oldRows = (hunk) =>
    hunk.old.forEach((text, n) =>
      rows.push({
        key: `old-${hunk.oldStart + n}`,
        type: "old",
        number: hunk.oldStart + n,
        text: cleanLine(text),
        hunk,
      }),
    );
  for (let i = 0; i <= lines.length; i++) {
    const removal = removedAt.get(i);
    if (removal) {
      if (open.has(hunkKey(removal))) oldRows(removal);
      else rows.push({ key: `removed-${i}`, type: "removed", hunk: removal });
    }
    const starting = startingAt.get(i);
    if (starting && open.has(hunkKey(starting))) oldRows(starting);
    if (i < lines.length)
      rows.push({ key: `line-${i}`, type: "line", number: i + 1, text: lines[i], hunk: covering.get(i) });
  }
  return rows;
}

export const hunkKey = (hunk) => `${hunk.kind}-${hunk.oldStart}`;

// A folder holds a change if any changed file is inside it.
export const holdsChange = (marks, folder) =>
  Object.keys(marks).some((path) => path.startsWith(`${folder}/`));

// Read-only view text: what can't be shown says why.
export function viewable(text) {
  if (text === null) return { problem: "This file is no longer available." };
  if (text.includes("\0")) return { problem: "This is a binary file, so there's no text to show." };
  return text.length > MAX_CHARS
    ? { text: `${text.slice(0, MAX_CHARS)}\n… (file truncated)` }
    : { text };
}
