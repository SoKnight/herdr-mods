// What the status chip shows, from the collector's `status` and when it last heard from the computer.
export const SILENT_MS = 60_000;

// null while the collector is answering and sampling; else { since }: when it last sampled (0 unknown).
export function silence(down, lastSample, now) {
  if (down || (lastSample > 0 && now - lastSample > SILENT_MS)) return { since: lastSample };
  return null;
}

// status: what `status` gave (null before the first answer); lastSample: the latest last_sample seen in a good answer.
export function chipState(status, lastSample, now) {
  if (!status) return { kind: "wait" };
  const good = !status.down && !status.error;
  const quiet = silence(Boolean(status.down), good ? status.last_sample ?? 0 : lastSample, now);
  if (quiet) return { kind: "silent", since: quiet.since };
  if (status.error) return { kind: "error", text: status.error };
  const me = status.today_me_ms;
  if (status.state === "away" && status.last_active > 0) return { kind: "away", me, away: Math.max(0, now - status.last_active) };
  // An open session with no activity to be away from yet reads as working.
  if (status.state === "working" || status.state === "away") return { kind: "working", me, project: status.project, name: status.project_name };
  return { kind: "none", me };
}

export const SILENT_REMIND_MS = 600_000;
const MONTHS_NOM = ["январь", "февраль", "март", "апрель", "май", "июнь", "июль", "август", "сентябрь", "октябрь", "ноябрь", "декабрь"];

// What to tell me now, as [{ key, title, body }]: c the chip state, remind the last good status.remind (null: defaults),
// sent the keys already shown, fmt the duration formatter (this file imports nothing).
export function reminders(c, remind, now, sent, fmt) {
  const r = remind ?? { silent: true, close: "" };
  const out = [];
  if (r.silent && c.kind === "silent" && c.since > 0 && now - c.since >= SILENT_REMIND_MS)
    out.push({ key: `silent:${c.since}`, title: "Time Tracker: сборщик молчит", body: `Нет данных ${fmt(now - c.since)} — время сейчас не считается.` });
  if (r.close) {
    const d = new Date(now), p = (n) => String(n).padStart(2, "0");
    out.push({ key: `close:${r.close}:${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`, title: `Time Tracker: ${MONTHS_NOM[Number(r.close.slice(5)) - 1]} не закрыт`, body: "Проверьте и закройте месяц в Отчёте." });
  }
  return out.filter((n) => !sent.has(n.key));
}
