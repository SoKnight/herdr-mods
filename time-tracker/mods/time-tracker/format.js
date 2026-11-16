// Pure helpers: no imports, so tests load this file as it is.

export function formatDuration(ms) {
  const m = Math.floor(Math.max(0, ms) / 60000);
  const h = Math.floor(m / 60);
  return h ? `${h}ч ${m % 60}м` : `${m}м`;
}

const ACTIVITIES = { development: "Разработка", architecture: "Проектирование", design: "Дизайн", documentation: "Документация", study: "Учёба", "": "Без вида" };
export const activityLabel = (id) => ACTIVITIES[id] ?? id ?? "";

// What `herdr-time ... --json` printed: exit code 3 means the collector isn't running. An error keeps
// its other fields (a closed period's id).
export function parseResult(r) {
  if (r.code === 3) return { down: true };
  try {
    const out = JSON.parse(r.stdout);
    if (r.code === 0) return out;
    return { ...out, error: out.error ?? `exit ${r.code}` };
  } catch {
    return { error: (r.stderr || r.stdout || `exit ${r.code}`).trim() };
  }
}

const pad2 = (n) => String(n).padStart(2, "0");
export const dayString = (ms) => {
  const d = new Date(ms);
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
};

// The local midnight of a "YYYY-MM-DD" day (calendar steps, so a DST change doesn't shift it).
const midnight = (day, plusDays = 0) => {
  const [y, m, d] = day.split("-").map(Number);
  return new Date(y, m - 1, d + plusDays).getTime();
};

// Monday to the next Monday, the day starting at dayStartMs after midnight.
export function weekRange(day, dayStartMs) {
  const back = (new Date(midnight(day)).getDay() + 6) % 7;
  return { from: midnight(day, -back) + dayStartMs, to: midnight(day, 7 - back) + dayStartMs };
}

export function monthRange(day, dayStartMs = 4 * 3600_000) {
  const [y, m] = day.split("-").map(Number);
  return { from: new Date(y, m - 1, 1).getTime() + dayStartMs, to: new Date(y, m, 1).getTime() + dayStartMs };
}

// The file's name: the days it covers, the last one as the collector counts it (the end cut at now, `to`
// being exclusive) and the day starting dayStartMs after midnight.
export function exportName(project, from, to, format, dayStartMs = 0, now = Date.now()) {
  return `${project}_${dayString(from - dayStartMs)}_${dayString(Math.min(to, now) - 1 - dayStartMs)}.${format}`;
}
