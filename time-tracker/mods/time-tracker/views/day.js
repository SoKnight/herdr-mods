// The panel's День tab: the day as a strip, its sessions as stretches, my time by project on the
// right, and corrections made by selecting time. The Сессия tab is the same view over a session.
import { div } from "gpui-kit";
import { h_flex, v_flex, InputState, ScrollHandle } from "gpui-base";
import { theme, props, layout } from "herdr";
import { call } from "../client.js";
import { formatDuration, activityLabel, catalog, hhmm, dayTitle, plural, midnight, ACTIVITIES } from "../format.js";
import { rangeFromDrag, stepDay } from "../lanes.js";
import { silence } from "../chip.js";
import { Button, Icon, IconButton, Message, TEXT } from "../ui.js";
import { Link } from "./menu.js";
import { Strip, AgentsStrip, HourLabels, Legend, DragArea } from "./strip.js";
import { StretchList, SessionHeader } from "./stretches.js";
import { Side } from "./side.js";
import { RuleEdit } from "./ruleedit.js";
import { NewProject } from "./newproject.js";
import { ManualEntry } from "./manual.js";

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
  // open(tab, cx): shows another tab of the panel; showDialog(d | null, cx): a dialog over it.
  constructor(open, showDialog) {
    this.open = open;
    this.showDialog = showDialog;
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
    this.silentSince = null;
    this.lastSample = 0; // when the collector last heard from the computer
    this.scroll = ScrollHandle.new(); // the rows under the strip
    this.scrolledTo = null; // the rows last scrolled to their end
  }

  // Read again on showing, then once a minute while shown; a hidden panel reads nothing.
  // Back from another tab or a hidden panel, the last «Отменить» goes: History or Разбор may have removed it since.
  tick(cx) {
    const shown = Boolean(props()?.overlay) || panelShown(layout());
    if (shown && !this.wasShown && this.message?.undo) this.message = { ...this.message, undo: null };
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
        this.error = r.down ? null : (r.error ?? null); // a collector that is down is the notice's business; the last data stays
        if (!r.down && !r.error) this.took(r);
        this.silentSince = this.want ? null : (silence(Boolean(r.down), this.lastSample, Date.now())?.since ?? null);
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

  // Starts the collector again, then reads.
  restart(cx) {
    this.message = null;
    cx.spawn(async (cx) => {
      const r = await call(["restart"], 20000);
      if (r.error) this.message = { text: r.error, error: true };
      this.load(cx);
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

  go(n, cx) {
    if (this.today) this.show(stepDay(this.want, this.today, n), cx);
  }

  // Shows a day: null is today.
  show(want, cx) {
    this.dropNote();
    Object.assign(this, { visit: this.visit + 1, want, fresh: false, divergences: [], sel: null, message: null, pending: null });
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
    if (!this.sel) return; // the menu closed under the click
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

  editRule(rule, cx) {
    this.sel = null;
    this.dropNote();
    const onSaved = (text, cx) => {
      this.message = { text };
      this.load(cx);
    };
    RuleEdit.open(this.showDialog, { rule, projects: this.projects, onSaved }, cx);
  }

  // The menu closes under the dialog, so the range is kept to assign it to the new project.
  newProject(cx) {
    const sel = this.sel && { ...this.sel };
    this.closeMenu(cx);
    const onCreated = (id, cx) => {
      this.sel = sel;
      this.correct({ kind: "project", project: id }, cx);
    };
    NewProject.open(this.showDialog, { projects: this.projects, today: this.today, onCreated, onSaved: (_text, cx) => this.load(cx) }, cx);
  }

  // Time added by hand on the day shown.
  addManual(cx) {
    const d = this.data;
    this.closeMenu(cx);
    const onAdd = (body, cx) =>
      this.perform(cx, () => call(["correct", "add", JSON.stringify(body)]), (r) => ({ text: `Добавлено: ${hhmm(body.start)}–${hhmm(body.end)} · ${this.name(body.project)}.`, undo: r.id }));
    const dialog = new ManualEntry({ day: d.day, dayStartMs: d.from - midnight(d.day), projects: this.projects, activities: this.activities, me: d.timeline?.me ?? [], onAdd, close: (cx) => this.showDialog(null, cx) });
    this.showDialog(dialog, cx);
    cx.timer.after(0, () => dialog.closed || dialog.from.focus()); // once the input is drawn
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

  // The time the strip draws.
  range() {
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
    const r = this.range();
    const sel = rangeFromDrag(g.x0, g.x1, r.from, r.to, this.width, Date.now()); // never past now
    if (!sel) return cx.notify();
    const px = (ms) => ((ms - r.from) / (r.to - r.from)) * this.width;
    this.sel = { ...sel, target: "me", x: px(sel.start), w: px(sel.end) - px(sel.start) };
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

  // The header and strip stay put while the rows scroll under them. The rows open at their end,
  // and go back there when `key` changes (another day, a new stretch), so the latest one shows.
  column(id, top, rows, key) {
    if (key !== this.scrolledTo) {
      this.scrolledTo = key;
      this.scroll.scroll_to_item(rows.length); // the marker after the rows
    }
    return v_flex()
      .flex_1()
      .min_w(0)
      .min_h(0)
      .child(v_flex().flex_none().pt(18).px(22).pb(14).gap(14).children(top))
      .child(
        v_flex().id(id).flex_1().min_h(0).overflow_y_scroll().track_scroll(this.scroll).px(22).pb(18).gap(14)
          .children(rows).child(div().h(1).flex_none()),
      );
  }

  left(t) {
    const d = this.data;
    const top = [this.header(t), ...this.notices(t)];
    if (!d) return this.column("day", [...top, Message(this.loading ? "Загрузка…" : "")], [], null);
    const { from, to } = this.range();
    top.push(
      v_flex()
        .flex_none()
        .gap(4)
        .child(DragArea(this).gap(4).child(Strip(this, t, { from, to, h: 20, radius: 4 })).child(AgentsStrip(this, t, { from, to })).child(HourLabels(t, from, to)))
        .child(Legend(t, d.timeline.leisure?.length > 0)),
    );
    const sessions = d.sessions ?? [];
    const rows = sessions.length
      ? sessions.map((s) => v_flex().flex_none().child(SessionHeader(this, t, s)).child(StretchList(this, t, s.stretches, { row: 30, gap: 16 })))
      : [Message("За этот день ничего не насчитано.")];
    const key = `${from}|${sessions.map((s) => s.stretches.length).join(",")}`;
    return this.column("day", top, rows, key);
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
      .when(Boolean(d), (el) =>
        el
          .child(div().text_size(TEXT.small).text_color(t.dim).child(`сутки ${hhmm(d.from)} → ${hhmm(d.from)}`))
          .child(Button({ id: "day-manual", label: "+ вручную", onClick: (cx) => this.addManual(cx) }))
          .child(IconButton({ id: "day-history", icon: "history", label: "История правок", onClick: (cx) => this.open("history", cx, { day: d.day, dayStartMs: d.from - midnight(d.day) }) }).w(28)),
      );
  }

  // Above the strip: an error, a reopen to confirm, closed exports the rules would change, the last message.
  notices(t) {
    return [
      this.silentSince !== null && this.silentView(t),
      this.error && Message(this.error, true),
      this.pending && this.confirmView(t),
      ...this.divergences.map((v) => this.divergenceView(v, t)),
      this.message && this.messageView(t),
    ].filter(Boolean);
  }

  // The collector isn't answering or sampling: nothing is being counted.
  silentView(t) {
    return v_flex()
      .id("day-silent")
      .flex_none()
      .px(12)
      .py(10)
      .gap(8)
      .rounded(8)
      .bg(t.chrome)
      .child(
        h_flex()
          .gap(8)
          .items_center()
          .child(div().text_color(t.ansi3).child(Icon("triangle-alert", 14)))
          .child(div().font_medium().child(this.silentSince ? `Нет данных с ${hhmm(this.silentSince)}` : "Нет данных")),
      )
      .child(div().text_size(TEXT.small).text_color(t.muted).child("Сборщик не отвечает — время сейчас не считается."))
      .child(
        h_flex()
          .gap(12)
          .items_center()
          .child(Button({ id: "day-restart", label: "Перезапустить", primary: true, onClick: (cx) => this.restart(cx) }))
          .child(Link({ id: "day-silent-settings", label: "Настройки", onClick: (cx) => this.open("settings", cx) })),
      );
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
}
