// What Herdr tells the collector: the folder you're working in, from the focused terminal or the shown space.

const RESEND = 30000;

export function beatFor(focused, spaces, now) {
  const project_path = focused?.cwd || spaces.find((s) => s.active)?.folder;
  if (!project_path) return null;
  return { source: "herdr", time: now, app: "herdr", project_path, focused: true };
}

export function shouldSend(last, next, now) {
  return !last || last.beat.project_path !== next.project_path || now - last.at >= RESEND;
}
