// Pure helpers: no imports, so tests load this file as it is.

export function formatDuration(ms) {
  const m = Math.floor(Math.max(0, ms) / 60000);
  const h = Math.floor(m / 60);
  if (!m) return "—";
  if (!h) return `${m}м`;
  return m % 60 ? `${h}ч ${m % 60}м` : `${h}ч`;
}

// "7 переключений": the form for n, after n.
export function plural(n, one, few, many) {
  const d = n % 10, dd = n % 100;
  return `${n} ${d === 1 && dd !== 11 ? one : d >= 2 && d <= 4 && (dd < 12 || dd > 14) ? few : many}`;
}

const WEEKDAYS = ["вс", "пн", "вт", "ср", "чт", "пт", "сб"];
const MONTHS = ["января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"];

// "2026-11-16" → "пн, 16 ноября".
export function dayTitle(day) {
  const [y, m, d] = day.split("-").map(Number);
  return `${WEEKDAYS[new Date(y, m - 1, d).getDay()]}, ${d} ${MONTHS[m - 1]}`;
}

// The other projects in a stretch: "+ Учёба 4м, другое 7м"; under a minute is left out.
export function shareText(shares, name) {
  const parts = shares.filter((s) => s.me >= 60000).map((s) => `${s.project ? name(s.project) : "другое"} ${formatDuration(s.me)}`);
  return parts.length ? `+ ${parts.join(", ")}` : "";
}

// The built-in kinds of activity, for a collector that doesn't list its own.
export const ACTIVITIES = [
  { id: "development", label: "Разработка" },
  { id: "architecture", label: "Проектирование" },
  { id: "design", label: "Дизайн" },
  { id: "documentation", label: "Документация" },
  { id: "study", label: "Учёба" },
];
const byId = (list, id) => list.find((a) => a.id === id);
export const activityLabel = (id, list = ACTIVITIES) => (id === "" ? "Без вида" : (byId(list, id) ?? byId(ACTIVITIES, id))?.label ?? id ?? "");

// What `projects` gave: the projects and the kinds of activity, the built-in ones when it lists none.
export function catalog(r) {
  if (!Array.isArray(r?.projects)) return null;
  return { projects: r.projects, activities: Array.isArray(r.activities) && r.activities.length ? r.activities : ACTIVITIES };
}

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
export const hhmm = (ms) => `${pad2(new Date(ms).getHours())}:${pad2(new Date(ms).getMinutes())}`;
export const dayString = (ms) => {
  const d = new Date(ms);
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
};

// The local midnight of a "YYYY-MM-DD" day (calendar steps, so a DST change doesn't shift it).
const midnight = (day, plusDays = 0) => {
  const [y, m, d] = day.split("-").map(Number);
  return new Date(y, m - 1, d + plusDays).getTime();
};

// Monday to the next Monday, the day starting at dayStartMs after midnight; offset counts weeks back (-) or on.
export function weekRange(day, dayStartMs, offset = 0) {
  const back = ((new Date(midnight(day)).getDay() + 6) % 7) - 7 * offset;
  return { from: midnight(day, -back) + dayStartMs, to: midnight(day, 7 - back) + dayStartMs };
}

export function monthRange(day, dayStartMs = 4 * 3600_000, offset = 0) {
  const [y, m] = day.split("-").map(Number);
  return { from: new Date(y, m - 1 + offset, 1).getTime() + dayStartMs, to: new Date(y, m + offset, 1).getTime() + dayStartMs };
}

// The file's name: the days it covers, the last one as the collector counts it (the end cut at now, `to`
// being exclusive) and the day starting dayStartMs after midnight.
export function exportName(project, from, to, format, dayStartMs = 0, now = Date.now()) {
  return `${project}_${dayString(from - dayStartMs)}_${dayString(Math.min(to, now) - 1 - dayStartMs)}.${format}`;
}

// The line under a session's title: how it began and when it ends.
export const sessionNote = (manual, minutes) => `${manual ? "начата вручную" : "началась сама после простоя"} · закончится после ${minutes} мин без активности`;
