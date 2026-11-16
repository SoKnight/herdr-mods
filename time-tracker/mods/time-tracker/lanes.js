// The day as lanes of bars, and what a drag over them means. Pure: no imports, tests load it as it is.

const ACTIVITY_IDS = ["development", "architecture", "design", "documentation", "study"];

// Projects take ansi1..ansi6 in config order; no project is dim, an unknown id muted.
export function projectColor(id, order) {
  if (!id) return "dim";
  const i = (order ?? []).indexOf(id);
  return i < 0 ? "muted" : `ansi${(i % 6) + 1}`;
}

// Pieces closer than a pixel join when they show the same thing, or when one of them is too thin
// to see; the joined bar shows whatever is widest in it.
function toBars(items, from, to, width, colorOf) {
  const px = (ms) => ((ms - from) / (to - from)) * width;
  const bars = [];
  for (const it of [...items].sort((a, b) => a.start - b.start)) {
    const s = Math.max(it.start, from), e = Math.min(it.end, to);
    if (s >= e) continue;
    const x = px(s), w = px(e) - x;
    const last = bars[bars.length - 1];
    if (last && x - (last.x + last.w) < 1 && (last.value === it.value || w < 1 || last.w < 1)) {
      last.w = Math.max(last.x + last.w, x + w) - last.x;
      last.segs.push(it.seg);
      if (w > last.widest) Object.assign(last, { widest: w, value: it.value, seg: it.seg });
      continue;
    }
    bars.push({ x, w, value: it.value, seg: it.seg, segs: [it.seg], widest: w });
  }
  return bars.map(({ widest, ...b }) => ({ ...b, w: Math.max(b.w, 1), color: colorOf(b.value) }));
}

export function lanes(timeline, from, to, width, order = []) {
  const me = timeline?.me ?? [];
  const apps = [...new Set(me.map((s) => s.app))];
  const lane = (id, list, valueOf, colorOf) =>
    ({ id, bars: toBars(list.map((seg) => ({ start: seg.start, end: seg.end, value: valueOf(seg), seg })), from, to, width, colorOf) });
  return [
    lane("power", timeline?.power ?? [], () => "power", () => "muted"),
    lane("me", me, () => "me", () => "accent"),
    lane("app", me, (s) => s.app, (v) => projectColor(v, apps)),
    lane("project", me, (s) => s.project, (v) => projectColor(v, order)),
    lane("activity", me, (s) => s.activity, (v) => projectColor(v, ACTIVITY_IDS)),
    lane("agents", timeline?.agents ?? [], (s) => s.project, (v) => projectColor(v, order)),
  ];
}

// Whose time a selection over these lanes corrects; the power lane belongs to no one.
export function targetFor(laneIds) {
  const ids = laneIds.filter((id) => id !== "power");
  if (!ids.includes("agents")) return "me";
  return ids.every((id) => id === "agents") ? "agents" : "both";
}

// A drag between two x positions as a range of whole minutes, start before end.
export function rangeFromDrag(x0, x1, from, to, width) {
  const at = (x) => {
    const ms = from + (Math.min(Math.max(x, 0), width) / width) * (to - from);
    return Math.round(ms / 60_000) * 60_000;
  };
  let start = at(Math.min(x0, x1));
  let end = at(Math.max(x0, x1));
  if (end <= start) end = start + 60_000;
  return { start: Math.max(start, from), end: Math.min(end, to) };
}

// The corrections a segment's why names ("правка #12: ..."), each once, for undo.
export function correctionIds(why) {
  return [...new Set([...(why ?? "").matchAll(/правка #(\d+)/g)].map((m) => Number(m[1])))];
}

// The day n days from a YYYY-MM-DD day.
export function shiftDay(day, n) {
  const [y, m, d] = day.split("-").map(Number);
  return new Date(Date.UTC(y, m - 1, d + n)).toISOString().slice(0, 10);
}

// A rule for one "no project" item: titles and Figma files are regexps, so a title matches as written.
export function ruleFor(item, project) {
  const exact = (s) => `^${s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}$`;
  return { project, kind: item.kind, value: item.kind === "path" ? item.value : exact(item.value) };
}
