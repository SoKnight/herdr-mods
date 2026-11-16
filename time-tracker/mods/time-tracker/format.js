// Pure helpers: no imports, so tests load this file as it is.

export function formatDuration(ms) {
  const m = Math.floor(Math.max(0, ms) / 60000);
  const h = Math.floor(m / 60);
  return h ? `${h}ч ${m % 60}м` : `${m}м`;
}

const ACTIVITIES = { development: "Разработка", architecture: "Проектирование", design: "Дизайн", documentation: "Документация", study: "Учёба" };
export const activityLabel = (id) => ACTIVITIES[id] ?? id ?? "";

// What `herdr-time ... --json` printed: exit code 3 means the collector isn't running.
export function parseResult(r) {
  if (r.code === 3) return { down: true };
  try {
    const out = JSON.parse(r.stdout);
    if (r.code === 0) return out;
    return { error: out.error ?? `exit ${r.code}` };
  } catch {
    return { error: (r.stderr || r.stdout || `exit ${r.code}`).trim() };
  }
}
