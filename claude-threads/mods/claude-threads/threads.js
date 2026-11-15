// Claude Code's sessions as this panel sees them. ~/.claude/history.jsonl holds one line per
// prompt the person typed ({display, timestamp, project, sessionId}), so it alone gives every
// session's folder, first and last prompt and when it was last used, without opening the
// session files themselves (some run to 100 MB).

// One line of text: prompts may hold newlines and control characters.
export const oneLine = (s) => (s ?? "").replace(/[\u0000-\u001f\u007f]+/g, " ").trim();

const command = (s) => s.trimStart().startsWith("/");

export function sessionsFromHistory(text) {
  const byId = new Map();
  for (const raw of text.split("\n")) {
    let entry;
    try {
      entry = JSON.parse(raw);
    } catch {
      continue; // blank, or the line Claude is writing right now
    }
    if (!entry?.sessionId || typeof entry.display !== "string") continue;
    let s = byId.get(entry.sessionId);
    if (!s) byId.set(entry.sessionId, (s = { id: entry.sessionId, project: entry.project, first: null, last: null, count: 0, updated: 0, prompts: [] }));
    s.count += 1;
    s.updated = Math.max(s.updated, entry.timestamp ?? 0);
    s.prompts.push(entry.display);
    if (!command(entry.display)) {
      s.first ??= entry.display;
      s.last = entry.display;
    }
  }
  return [...byId.values()].sort((a, b) => b.updated - a.updated);
}

// The folder under ~/.claude/projects that holds a session started in cwd.
export const projectDir = (cwd) => cwd.replace(/[^A-Za-z0-9]/g, "-");

// Sessions with a prompt containing query (any case), each with the last such prompt.
export function searchSessions(sessions, query) {
  const q = query.trim().toLowerCase();
  if (!q) return sessions.map((session) => ({ session, hit: null }));
  const found = [];
  for (const session of sessions) {
    const hit = session.prompts.findLast((p) => p.toLowerCase().includes(q));
    if (hit !== undefined) found.push({ session, hit });
  }
  return found;
}

// Session id -> the terminal running it. Claude keeps ~/.claude/sessions/<pid>.json while it
// runs, but a crashed one leaves its file behind, so only a live terminal's process counts.
export function openSessions(registry, terminals) {
  const byPid = new Map(terminals.filter((t) => t.pid != null).map((t) => [t.pid, t.address]));
  const open = new Map();
  for (const r of registry) if (byPid.has(r.pid)) open.set(r.sessionId, byPid.get(r.pid));
  return open;
}

export function ago(ms, now) {
  const m = Math.floor((now - ms) / 60000);
  if (!(m >= 1)) return "just now";
  if (m < 60) return `${m}m ago`;
  return m < 1440 ? `${Math.floor(m / 60)}h ago` : `${Math.floor(m / 1440)}d ago`;
}
