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
  if (status.state === "working") return { kind: "working", me, project: status.project, name: status.project_name };
  if (status.state === "away") return { kind: "away", me, away: Math.max(0, now - status.last_active) };
  return { kind: "none", me };
}
