// Paths the same way on every machine. Herdr gives a Windows folder as C:\x\y, Git as C:/x/y,
// and Windows doesn't care about case, so a Windows path is told by its shape (a drive letter,
// or \\ for a share), never by this computer's platform: a space can be on another machine.
// Shared between this repo's mods: each keeps an identical copy, because a mod can't import
// from another's folder. POSIX paths pass through every function unchanged.

const WINDOWS = /^(?:[A-Za-z]:(?:[\\/]|$)|[\\/]{2}[^\\/])/;

export const isWindowsPath = (path) => WINDOWS.test(path ?? "");

// One spelling for a path: "/" between parts, a capital drive letter, no trailing "/" but a
// drive's own (C:/). Leaves a POSIX path as it is.
export function normPath(path) {
  if (!isWindowsPath(path)) return path ?? "";
  const p = path.replace(/\\/g, "/").replace(/^([a-z]):/, (_m, d) => `${d.toUpperCase()}:`);
  if (/^[A-Z]:\/?$/.test(p)) return `${p.slice(0, 2)}/`;
  return p.replace(/\/+$/, "");
}

const key = (path) => {
  const p = normPath(path);
  return isWindowsPath(p) ? p.toLowerCase() : p;
};

export const samePath = (a, b) => key(a) === key(b);

// Whether path is root itself or anywhere under it.
export function insidePath(path, root) {
  const p = key(path);
  const r = key(root);
  return p === r || p.startsWith(r.endsWith("/") ? r : `${r}/`);
}

// path below root, "/"-separated, or null when it isn't under it ("" for root itself).
export function relativePath(path, root) {
  if (!insidePath(path, root)) return null;
  const r = normPath(root);
  return normPath(path).slice(r.endsWith("/") ? r.length : r.length + 1);
}

// child below folder, with one "/" between them.
export const joinPath = (folder, child) => {
  const f = normPath(folder);
  return child ? `${f.endsWith("/") ? f : `${f}/`}${child}` : f;
};

// The last part of a path, whichever separator it uses.
export const baseName = (path) => (path ?? "").split(/[\\/]/).filter(Boolean).pop() ?? path ?? "";

// How the person and Windows programs write it: backslashes for a Windows path.
export const nativePath = (path) => (isWindowsPath(path) ? normPath(path).replace(/\//g, "\\") : path ?? "");

// A path as one PowerShell single-quoted string: quotes, curly ones too, are doubled.
export const psQuote = (text) => `'${String(text).replace(/['‘’‚‛]/g, "$&$&")}'`;
