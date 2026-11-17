// The day as lanes of bars, and what a drag over them means. Pure: no imports, tests load it as it is.

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

// order: project ids in config order; activities: the kinds' ids, coloured the same way.
export function lanes(timeline, from, to, width, order = [], activities = []) {
  const me = timeline?.me ?? [];
  const apps = [...new Set(me.map((s) => s.app))];
  const lane = (id, list, valueOf, colorOf) =>
    ({ id, bars: toBars(list.map((seg) => ({ start: seg.start, end: seg.end, value: valueOf(seg), seg })), from, to, width, colorOf) });
  return [
    lane("power", timeline?.power ?? [], () => "power", () => "muted"),
    lane("me", me, () => "me", () => "accent"),
    lane("app", me, (s) => s.app, (v) => projectColor(v, apps)),
    lane("project", me, (s) => s.project, (v) => projectColor(v, order)),
    lane("activity", me, (s) => s.activity, (v) => projectColor(v, activities)),
    lane("agents", timeline?.agents ?? [], (s) => s.project, (v) => projectColor(v, order)),
  ];
}

// The part of the day worth drawing: whole hours around my and the agents' time (and now, for today),
// at least three hours wide. With nothing counted yet, the whole day.
export function viewRange(timeline, from, to, now) {
  const H = 3600_000;
  const segs = [...(timeline?.me ?? []), ...(timeline?.agents ?? [])];
  if (!segs.length) return { from, to };
  const first = Math.min(...segs.map((s) => s.start));
  const last = Math.max(Math.min(now, to), ...segs.map((s) => s.end));
  let a = Math.max(from, from + Math.floor((first - from) / H) * H);
  let b = Math.min(to, from + Math.ceil((last - from) / H) * H);
  if (b - a < 3 * H) a = Math.max(from, b - 3 * H);
  if (b - a < 3 * H) b = Math.min(to, a + 3 * H);
  return { from: a, to: b };
}

// Hours between axis labels for a range: about six labels at most.
export const axisStep = (span) => (span <= 6 * 3600_000 ? 1 : span <= 12 * 3600_000 ? 2 : 3) * 3600_000;

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

// The day to ask for after moving n days from want (null: today). Today and later are null, so the
// collector names today itself and a panel left open past the day's start moves on to the new day.
export function stepDay(want, today, n) {
  const next = shiftDay(want ?? today, n);
  return next >= today ? null : next;
}

// The lanes a drag between y0 and y1 covers, as first and last index; y outside the lanes clamps.
export function laneSpan(y0, y1, count, row) {
  const at = (y) => Math.max(0, Math.min(count - 1, Math.floor(y / row)));
  return { top: at(Math.min(y0, y1)), bottom: at(Math.max(y0, y1)) };
}

// A rule for one "no project" item: titles and Figma files are regexps, so a title matches as written.
export function ruleFor(item, project) {
  const exact = (s) => `^${s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")}$`;
  return { project, kind: item.kind, value: item.kind === "path" ? item.value : exact(item.value) };
}
