// Pure helpers: no imports, so tests load this file as it is.

export function formatDuration(ms) {
  const m = Math.floor(Math.max(0, ms) / 60000);
  const h = Math.floor(m / 60);
  return h ? `${h}ч ${m % 60}м` : `${m}м`;
}

const ACTIVITIES = { development: "Разработка", architecture: "Проектирование", design: "Дизайн", documentation: "Документация", study: "Учёба" };
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

// `to` is exclusive: the file names the last day inside.
export function exportName(project, from, to, format) {
  return `${project}_${dayString(from)}_${dayString(to - 24 * 3600_000)}.${format}`;
}

// What the export file will say, from a report's project total (notes are added only in the file).
export function previewMarkdown(name, from, to, p, step, agents) {
  const round = (ms) => Math.floor((ms + (step * 30_000)) / (step * 60_000)) * step;
  const out = [`# ${name}: ${dayString(from)} – ${dayString(to - 24 * 3600_000)}`];
  let total = 0;
  for (const day of Object.keys(p.days).sort()) {
    const rows = Object.entries(p.days[day].activities).filter(([, ms]) => ms > 0);
    if (!rows.length) continue;
    out.push("", `## ${day}`, "", "| Вид | Минуты |", "|---|---|");
    for (const [a, ms] of rows) {
      total += round(ms);
      out.push(`| ${activityLabel(a) || "Без вида"} | ${round(ms)} |`);
    }
  }
  out.push("", `**Итого: ${total} мин**`);
  if (agents) out.push("", `Время агентов (для справки): ${Object.values(p.days).reduce((s, d) => s + round(d.agents), 0)} мин`);
  return out.join("\n");
}
