// The panel's День tab: the day as a strip, its sessions as stretches, my time by project on the
// right, and corrections made by selecting time. The Сессия tab is the same view over a session.
import { div } from "gpui-kit";
import { h_flex, v_flex, InputState } from "gpui-base";
import { theme, props, layout } from "herdr";
import { call } from "../client.js";
import { formatDuration, activityLabel, catalog, hhmm, dayTitle, plural, ACTIVITIES } from "../format.js";
import { rangeFromDrag, stepDay, ruleFor } from "../lanes.js";
import { Message, TEXT } from "../ui.js";
import { Link } from "./menu.js";
import { Unassigned } from "./totals.js";
import { Strip, AgentsStrip, HourLabels, Legend, DragArea } from "./strip.js";
import { StretchList, SessionHeader } from "./stretches.js";
import { Side } from "./side.js";

const REFRESH_MS = 60_000;

const pad = (n) => String(n).padStart(2, "0");
const dateLabel = (ms) => {
  const d = new Date(ms);
  return `${pad(d.getDate())}.${pad(d.getMonth() + 1)}.${d.getFullYear()}`;
};
const minutes = (ms) => `${ms > 0 ? "+" : ""}${Math.round(ms / 60_000)} мин`;

// Whether a panel copy is on screen: in a window of its own, or a shown block of the layout (the
// status chip doesn't count). As in Branch PR: every parent shown, and the chosen child of a stack.
export function panelShown(node) {
  if (!node || node.hidden) return false;
  if (node.content?.mod === "time-tracker" && node.content.props?.view !== "status") return true;
  const children = node.children ?? [];
  if (node.type !== "stack") return children.some(panelShown);
  return panelShown(children.find((c) => c.id === node.active) ?? children[0]);
}

export class Day {
  // open(tab, cx): shows another tab of the panel.
  constructor(open) {
    this.open = open;
    this.data = null; // what `timeline` gave
    this.projects = []; // [{id, name}] in config order
    this.activities = ACTIVITIES; // [{id, label}], as the collector lists them
    this.want = null; // the day asked for; null is today, which only the collector knows
    this.today = null; // from the last answer for today
    this.seq = 0; // the latest load: older answers are dropped
    this.visit = 0; // counts day switches: a change that finishes after one shows no message
    this.fresh = false; // closed periods shown as the rules give them now
    this.divergences = [];
    this.error = null;
    this.message = null; // { text, error?, undo? }
    this.pending = null; // a correction a closed period stopped: { period, date, retry, done }
    this.loading = false;
    this.loadedAt = 0;
    this.wasShown = false;
    this.width = 0;
    this.drag = null;
    this.sel = null; // the selection the menu acts on
    this.step = "root";
    this.unassigned = null;
    this.unassignedOpen = false;
    this.assigning = null;
    this.silentSince = null;
    this.lastSample = 0; // when the collector last heard from the computer
  }

  // Read again on showing, then once a minute while shown; a hidden panel reads nothing.
  tick(cx) {
    const shown = Boolean(props()?.overlay) || panelShown(layout());
    if (shown && (!this.wasShown || Date.now() - this.loadedAt >= REFRESH_MS)) this.load(cx);
    this.wasShown = shown;
  }

  // Every load wins over the ones before it: a day switch or a change mid-load is never lost.
  load(cx) {
    const seq = ++this.seq;
    const args = this.args();
    this.loading = true;
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        const [r, projects] = await Promise.all([call(args), call(["projects"])]);
        if (seq !== this.seq) return;
        Object.assign(this, catalog(projects));
        this.error = r.down ? "Сборщик не запущен: go -C helper run ./deploy" : (r.error ?? null);
        if (!this.error) {
          this.took(r);
          if (this.unassignedOpen) await this.loadUnassigned(seq);
        }
      } finally {
        // Even after a throw: no spinner left on, and the tick waits its minute before trying again.
        if (seq === this.seq) {
          this.loading = false;
          this.loadedAt = Date.now();
        }
        cx.notify();
      }
    });
  }

  // What the load asks the collector.
  args() {
    return ["timeline", ...(this.want ? ["--day", this.want] : []), ...(this.fresh ? ["--fresh"] : [])];
  }

  // Keeps a good answer. A day switch starts a newer load, which drops this one, so want is the one asked.
  took(r) {
    this.data = r;
    this.lastSample = r.last_sample ?? 0;
    if (!this.want) this.today = r.day;
    if (!this.fresh) this.divergences = r.divergences;
  }

  async loadUnassigned(seq = this.seq) {
    const { from, to } = this.otherRange();
    const r = await call(["unassigned", "--from", String(from), "--to", String(to)]);
    if (seq === this.seq) this.unassigned = Array.isArray(r) ? r : [];
  }

  go(n, cx) {
    if (this.today) this.show(stepDay(this.want, this.today, n), cx);
  }

  // Shows a day: null is today.
  show(want, cx) {
    this.dropNote();
    Object.assign(this, { visit: this.visit + 1, want, fresh: false, divergences: [], sel: null, message: null, pending: null, unassigned: null, unassignedOpen: false });
    this.load(cx);
  }

  // Runs one change; a closed period asks first and runs it again once reopened.
  perform(cx, step, done) {
    this.sel = null;
    this.dropNote();
    const visit = this.visit;
    cx.spawn(async (cx) => {
      const r = await step();
      if (visit !== this.visit) {
        // Another day is shown now: its own load is under way, and this result is about the old one.
        if (!r.down && !r.error) this.load(cx);
        return;
      }
      if (r.error === "closed" && r.period) {
        const periods = await call(["periods"]);
        if (visit !== this.visit) return;
        const p = Array.isArray(periods) ? periods.find((x) => x.id === r.period) : null;
        this.pending = { period: r.period, date: p ? dateLabel(p.exported_at) : null, retry: step, done };
      } else if (r.down || r.error) {
        this.message = { text: r.down ? "Сборщик не запущен" : r.error, error: true };
      } else {
        this.message = done(r);
        this.load(cx);
      }
      cx.notify();
    });
    cx.notify();
  }

  correct(c, cx) {
    const { start, end, target } = this.sel;
    const body = { project: "", activity: "", note: "", ...c, start, end, target: c.kind === "project" || c.kind === "exclude" ? target : "me" };
    this.perform(cx, () => call(["correct", "add", JSON.stringify(body)]), (r) => ({ text: `Правка #${r.id} сохранена.`, undo: r.id }));
  }

  undo(id, cx) {
    this.perform(cx, () => call(["correct", "rm", String(id)]), () => ({ text: `Правка #${id} отменена.` }));
  }

  reopen(period, cx, then = () => ({ text: "Период переоткрыт." })) {
    this.fresh = false;
    this.perform(cx, () => call(["reopen", String(period)]), then);
  }

  confirmReopen(cx) {
    const { period, retry, done } = this.pending;
    this.pending = null;
    this.perform(cx, async () => {
      const r = await call(["reopen", String(period)]);
      return r.down || r.error ? r : retry();
    }, done);
  }

  assign(item, project, cx) {
    this.assigning = null;
    this.perform(cx, () => call(["rule", "add", JSON.stringify(ruleFor(item, project))]), () => ({ text: `Правило добавлено: ${this.name(project)}.` }));
  }

  openNote(cx) {
    this.dropNote();
    this.note = InputState.new({ placeholder: "Заметка" });
    this.note.on("submit", (_e, cx) => this.saveNote(cx));
    cx.timer.after(0, () => this.note?.focus()); // once the input is drawn
  }

  saveNote(cx) {
    const note = this.note?.value().trim();
    if (note) this.correct({ kind: "note", note }, cx);
  }

  dropNote() {
    this.note?.release();
    this.note = null;
  }

  closeMenu(cx) {
    if (!this.sel) return;
    this.sel = null;
    this.dropNote();
    cx.notify();
  }

  name(id) {
    if (!id) return "Другое";
    return this.projects.find((p) => p.id === id)?.name || id;
  }

  // Project ids in config order, for their colours.
  order() {
    return this.projects.map((p) => p.id);
  }

  // The time the strip draws.
  range() {
    return { from: this.data.from, to: this.data.to };
  }

  // The time "Другое" is taken apart over.
  otherRange() {
    return { from: this.data.from, to: this.data.to };
  }

  // Pointer position in the strip, in the px the bars are drawn in.
  point(e) {
    const p = e.local_position ?? { x: 0, y: 0 };
    return { x: e.bounds?.width ? (p.x / e.bounds.width) * this.width : p.x, y: p.y };
  }

  // An x in those px as a share of the strip: sizes reach us scaled by zoom, so pixels would overshoot.
  pct(x) {
    return `${(x / this.width) * 100}%`;
  }

  finishDrag(cx) {
    const g = this.drag;
    this.drag = null;
    if (!g || Math.abs(g.x1 - g.x0) < 3) return cx.notify();
    const x = Math.max(0, Math.min(g.x0, g.x1));
    const w = Math.min(this.width, Math.max(g.x0, g.x1)) - x;
    const r = this.range();
    this.sel = { ...rangeFromDrag(g.x0, g.x1, r.from, r.to, this.width), target: "me", x, w };
    this.step = "root";
    cx.notify();
  }

  // Hover on my time: when, what, which project and kind, and why it was counted so.
  tip(bar) {
    const s = bar.seg;
    const more = bar.segs.length > 1 ? `\n(и ещё ${bar.segs.length - 1} коротких рядом)` : "";
    const what = [s.app, s.title].filter(Boolean).join(" — ");
    return `${hhmm(s.start)}–${hhmm(s.end)} · ${formatDuration(s.end - s.start)}\n${what}\nПроект: ${this.name(s.project)}\nВид: ${activityLabel(s.activity, this.activities) || "—"}${s.note ? `\nЗаметка: ${s.note}` : ""}\nПочему: ${s.why}${more}`;
  }

  render() {
    const t = theme();
    return h_flex()
      .flex_1()
      .min_h(0)
      .items_stretch()
      .child(this.left(t))
      .when(Boolean(this.data), (el) => el.child(this.sideView(t)));
  }

  left(t) {
    const d = this.data;
    const col = v_flex().id("day").flex_1().min_w(0).min_h(0).overflow_y_scroll().pt(18).px(22).pb(18).gap(14).child(this.header(t)).children(this.notices(t));
    if (!d) return col.child(Message(this.loading ? "Загрузка…" : ""));
    const { from, to } = this.range();
    col.child(
      v_flex()
        .flex_none()
        .gap(4)
        .child(DragArea(this).gap(4).child(Strip(this, t, { from, to, h: 20, radius: 4 })).child(AgentsStrip(this, t, { from, to })).child(HourLabels(t, from, to)))
        .child(Legend(t)),
    );
    const sessions = d.sessions ?? [];
    if (!sessions.length) col.child(Message("За этот день ничего не насчитано."));
    for (const s of sessions) col.child(v_flex().flex_none().child(SessionHeader(this, t, s)).child(StretchList(this, t, s.stretches, { row: 30, gap: 16 })));
    if (this.unassignedOpen) col.child(Unassigned(this, t));
    return col;
  }

  sideView(t) {
    const { report, stats } = this.data;
    const lines = [`агенты ${formatDuration(report.agents)} · включён ${formatDuration(report.uptime)}`];
    if (report.me > 0 && stats) lines.push(`${plural(stats.switches, "переключение", "переключения", "переключений")} · фокус до ${formatDuration(stats.focus)}`);
    return Side(this, t, { caption: "Моё время", total: report.me, lines });
  }

  header(t) {
    const d = this.data;
    const day = d?.day; // the day drawn, not the one being asked for: a failed load keeps both as they were
    const today = Boolean(day) && day === this.today;
    const chevron = (id, label, glyph, n) =>
      div()
        .id(id)
        .flex_none()
        .px(4)
        .text_size(16)
        .text_color(t.muted)
        .accessibility_label(label)
        .cursor_pointer()
        .hover((s) => s.text_color(t.foreground))
        .on_click((_e, cx) => this.go(n, cx))
        .child(glyph);
    const title = (text) => div().flex_none().text_size(15).font_semibold().child(text);
    return h_flex()
      .flex_none()
      .gap(10)
      .items_center()
      .child(chevron("day-prev", "Предыдущий день", "‹", -1))
      .when(!day, (el) => el.child(title("…")))
      .when(today, (el) => el.child(title("Сегодня")).child(div().text_color(t.dim).child(dayTitle(day))))
      .when(Boolean(day) && !today, (el) => el.child(title(dayTitle(day))).child(chevron("day-next", "Следующий день", "›", 1)))
      .child(div().flex_1())
      .when(Boolean(d), (el) => el.child(div().text_size(TEXT.small).text_color(t.dim).child(`сутки ${hhmm(d.from)} → ${hhmm(d.from)}`)));
  }

  // Above the strip: an error, a reopen to confirm, closed exports the rules would change, the last message.
  notices(t) {
    return [
      this.error && Message(this.error, true),
      this.pending && this.confirmView(t),
      ...this.divergences.map((v) => this.divergenceView(v, t)),
      this.message && this.messageView(t),
    ].filter(Boolean);
  }

  strip(id, t, text, links) {
    return h_flex()
      .id(id)
      .flex_none()
      .px(10)
      .py(7)
      .gap(12)
      .items_center()
      .rounded(8)
      .border_1()
      .border_color(t.border)
      .bg(t.raised)
      .text_size(TEXT.small)
      .child(div().flex_1().min_w(0).whitespace_normal().child(text))
      .children(links);
  }

  confirmView(t) {
    const p = this.pending;
    return this.strip("day-confirm", t, `Период закрыт выгрузкой${p.date ? ` от ${p.date}` : ""}. Переоткрыть?`, [
      Link({ id: "day-confirm-no", label: "Отмена", color: t.muted, onClick: (cx) => ((this.pending = null), cx.notify()) }),
      Link({ id: "day-confirm-yes", label: "Переоткрыть", onClick: (cx) => this.confirmReopen(cx) }),
    ]);
  }

  divergenceView(v, t) {
    return this.strip(`day-div-${v.period}`, t, `Выгрузка «${this.name(v.project)}» закрыта: правило изменило бы ${minutes(v.delta_ms)}.`, [
      Link({
        id: `day-div-fresh-${v.period}`,
        label: this.fresh ? "Показать выгрузку" : "Посмотреть разницу",
        onClick: (cx) => {
          this.fresh = !this.fresh;
          this.load(cx);
        },
      }),
      Link({ id: `day-div-reopen-${v.period}`, label: "Переоткрыть", onClick: (cx) => this.reopen(v.period, cx) }),
    ]);
  }

  messageView(t) {
    const m = this.message;
    return h_flex()
      .flex_none()
      .gap(10)
      .text_size(TEXT.small)
      .text_color(m.error ? t.ansi1 : t.muted)
      .child(div().min_w(0).whitespace_normal().child(m.text))
      .when(Boolean(m.undo), (el) => el.child(Link({ id: "day-undo-last", label: "Отменить", onClick: (cx) => this.undo(m.undo, cx) })));
  }

  toggleUnassigned(cx) {
    this.unassignedOpen = !this.unassignedOpen;
    this.assigning = null;
    if (!this.unassignedOpen) return cx.notify();
    cx.spawn(async (cx) => {
      await this.loadUnassigned();
      cx.notify();
    });
  }
}
