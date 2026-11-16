// The panel's Day tab: the day as lanes, totals by project, and corrections made by selecting time.
import { div } from "gpui-kit";
import { h_flex, v_flex, InputState, Popup } from "gpui-base";
import { theme, props, layout } from "herdr";
import { call } from "../client.js";
import { formatDuration, activityLabel } from "../format.js";
import { lanes, targetFor, rangeFromDrag, correctionIds, stepDay, laneSpan, ruleFor } from "../lanes.js";
import { IconButton, Message, TEXT } from "../ui.js";
import { SelectionMenu, Link } from "./menu.js";
import { Totals } from "./totals.js";

const ROW = 22;
const REFRESH_MS = 60_000;
const LABELS = { power: "Питание", me: "Я", app: "Приложение", project: "Проект", activity: "Вид", agents: "Агенты" };
const WEEKDAYS = ["вс", "пн", "вт", "ср", "чт", "пт", "сб"];

const pad = (n) => String(n).padStart(2, "0");
const hhmm = (ms) => `${pad(new Date(ms).getHours())}:${pad(new Date(ms).getMinutes())}`;
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
  constructor() {
    this.data = null; // what `timeline` gave
    this.projects = []; // [{id, name}] in config order
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
    this.appOpen = false;
    this.unassigned = null;
    this.unassignedOpen = false;
    this.assigning = null;
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
    const { want, fresh } = this;
    this.loading = true;
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        const args = ["timeline", ...(want ? ["--day", want] : []), ...(fresh ? ["--fresh"] : [])];
        const [tl, projects] = await Promise.all([call(args), call(["projects"])]);
        if (seq !== this.seq) return;
        if (Array.isArray(projects)) this.projects = projects;
        this.error = tl.down ? "Сборщик не запущен: go -C helper run ./deploy" : (tl.error ?? null);
        if (!this.error) {
          this.data = tl;
          if (!want) this.today = tl.day;
          if (!fresh) this.divergences = tl.divergences;
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

  async loadUnassigned(seq = this.seq) {
    const r = await call(["unassigned", "--from", String(this.data.from), "--to", String(this.data.to)]);
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
    if (!id) return "Без проекта";
    return this.projects.find((p) => p.id === id)?.name || id;
  }

  shownLanes() {
    const d = this.data;
    const all = d && this.width > 0 ? lanes(d.timeline, d.from, d.to, this.width, this.projects.map((p) => p.id)) : [];
    return all.filter((l) => l.id !== "app" || this.appOpen);
  }

  // Pointer position in the tracks, in the px the bars are drawn in.
  point(e) {
    const p = e.local_position ?? { x: 0, y: 0 };
    return { x: e.bounds?.width ? (p.x / e.bounds.width) * this.width : p.x, y: p.y };
  }

  finishDrag(ids, cx) {
    const g = this.drag;
    this.drag = null;
    if (!g || Math.abs(g.x1 - g.x0) < 3) return cx.notify();
    const { top, bottom } = laneSpan(g.y0, g.y1, ids.length, ROW);
    const picked = ids.slice(top, bottom + 1);
    const x = Math.max(0, Math.min(g.x0, g.x1));
    const w = Math.min(this.width, Math.max(g.x0, g.x1)) - x;
    this.sel = { ...rangeFromDrag(g.x0, g.x1, this.data.from, this.data.to, this.width), target: targetFor(picked), x, w, top: top * ROW, h: picked.length * ROW };
    this.step = "root";
    cx.notify();
  }

  render() {
    const t = theme();
    const d = this.data;
    const root = v_flex().id("day").flex_1().min_h(0).overflow_y_scroll().pb(14).child(this.dayBar(t));
    if (this.error) root.child(Message(this.error, true));
    if (this.pending) root.child(this.confirmView(t));
    root.children(this.divergences.map((v) => this.divergenceView(v, t)));
    if (this.message) root.child(this.messageView(t));
    if (!d) return root.child(Message(this.loading ? "Загрузка…" : ""));
    return root.child(this.lanesView(t)).child(Totals(this, t));
  }

  dayBar(t) {
    const d = this.data;
    const r = d?.report;
    const day = d?.day; // the day drawn, not the one being asked for: a failed load keeps both as they were
    const date = day ? new Date(`${day}T12:00:00`) : null;
    const label = date ? `${WEEKDAYS[date.getDay()]} ${dateLabel(date.getTime())}` : "…";
    return h_flex()
      .h(40)
      .flex_none()
      .px(14)
      .gap(10)
      .items_center()
      .child(Link({ id: "day-prev", label: "‹", onClick: (cx) => this.go(-1, cx) }))
      .child(div().text_size(TEXT.body).child(day && day === this.today ? `Сегодня, ${label}` : label))
      .when(Boolean(day) && day !== this.today, (el) => el.child(Link({ id: "day-next", label: "›", onClick: (cx) => this.go(1, cx) })))
      .child(div().flex_1())
      .when(Boolean(r), (el) =>
        el.child(div().text_size(TEXT.small).text_color(t.muted).child(`Аптайм ${formatDuration(r.uptime)} · Моё ${formatDuration(r.me)} · Агенты ${formatDuration(r.agents)}`)),
      )
      .child(IconButton({ id: "day-refresh", icon: "refresh", label: "Обновить", spinning: this.loading, onClick: (cx) => this.load(cx) }));
  }

  strip(id, t, text, links) {
    return h_flex()
      .id(id)
      .flex_none()
      .mx(14)
      .mb(8)
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
      .px(14)
      .pb(8)
      .gap(10)
      .text_size(TEXT.small)
      .text_color(m.error ? t.ansi1 : t.muted)
      .child(div().min_w(0).whitespace_normal().child(m.text))
      .when(Boolean(m.undo), (el) => el.child(Link({ id: "day-undo-last", label: "Отменить", onClick: (cx) => this.undo(m.undo, cx) })));
  }

  // Hover: when, what, which project and kind, and why it was counted so.
  tip(id, bar) {
    const s = bar.seg;
    const more = bar.segs.length > 1 ? `\n(и ещё ${bar.segs.length - 1} коротких рядом)` : "";
    const when = `${hhmm(s.start)}–${hhmm(s.end)} · ${formatDuration(s.end - s.start)}`;
    if (id === "power") return `Компьютер включён ${when}`;
    if (id === "agents") return `${when}\nПроект: ${this.name(s.project)}\nПочему: ${s.why}${more}`;
    const what = [s.app, s.title].filter(Boolean).join(" — ");
    return `${when}\n${what}\nПроект: ${this.name(s.project)}\nВид: ${activityLabel(s.activity) || "—"}${s.note ? `\nЗаметка: ${s.note}` : ""}\nПочему: ${s.why}${more}`;
  }

  laneRow(lane, i, t) {
    const row = div().relative().h(ROW).w_full();
    lane.bars.forEach((bar, j) => {
      row.child(div().id(`bar-${lane.id}-${j}`).absolute().left(bar.x).top(4).w(bar.w).h(ROW - 8).rounded(2).bg(t[bar.color] ?? t.muted).tooltip(this.tip(lane.id, bar)));
      const ids = [...new Set(bar.segs.filter((s) => s.corrected).flatMap((s) => correctionIds(s.why)))];
      if (ids.length)
        row.child(
          div()
            .id(`mark-${lane.id}-${j}`)
            .absolute()
            .left(bar.x)
            .top(0)
            .size(7)
            .rounded(4)
            .bg(t.foreground)
            .cursor_pointer()
            .tooltip(`Правка ${ids.map((n) => `#${n}`).join(", ")}`)
            .on_mouse_down("left", (_e, cx) => {
              cx.stop_propagation();
              this.sel = { undo: ids, x: bar.x, w: 7, top: i * ROW, h: ROW };
              this.step = "undo";
              cx.notify();
            }),
        );
    });
    return row;
  }

  lanesView(t) {
    const d = this.data;
    const shown = this.shownLanes();
    const ids = shown.map((l) => l.id);
    const labels = (this.appOpen ? Object.keys(LABELS) : Object.keys(LABELS).filter((id) => id !== "app")).map((id) =>
      h_flex()
        .h(ROW)
        .items_center()
        .text_size(TEXT.small)
        .text_color(t.muted)
        .child(LABELS[id]),
    );
    // The App lane folds; its toggle sits under the labels.
    labels.push(Link({ id: "day-app-toggle", label: this.appOpen ? "▾ Приложение" : "▸ Приложение", color: t.dim, onClick: (cx) => ((this.appOpen = !this.appOpen), cx.notify()) }).text_size(TEXT.caption));

    const g = this.drag;
    const tracks = v_flex()
      .id("day-tracks")
      .relative()
      .flex_1()
      .min_w(0)
      .cursor_crosshair()
      .on_size_change((size, cx) => {
        if (size.width !== this.width) {
          this.width = size.width;
          cx.notify();
        }
      })
      .on_mouse_down("left", (e, cx) => {
        const p = this.point(e);
        if (p.y >= ids.length * ROW) return; // the hours under the lanes
        this.sel = null;
        this.drag = { x0: p.x, y0: p.y, x1: p.x, y1: p.y };
        cx.notify();
      })
      .on_mouse_move((e, cx) => {
        if (!this.drag) return;
        const p = this.point(e);
        Object.assign(this.drag, { x1: p.x, y1: p.y });
        cx.notify();
      })
      .on_mouse_up("left", (_e, cx) => this.finishDrag(ids, cx))
      .on_hover((hovered, cx) => !hovered && this.drag && this.finishDrag(ids, cx))
      .children(shown.map((lane, i) => this.laneRow(lane, i, t)))
      .child(this.axis(d, t));

    if (g) {
      const { top, bottom } = laneSpan(g.y0, g.y1, ids.length, ROW);
      tracks.child(div().absolute().left(Math.min(g.x0, g.x1)).top(top * ROW).w(Math.abs(g.x1 - g.x0)).h((bottom - top + 1) * ROW).bg(t.selection));
    } else if (this.sel) {
      const s = this.sel;
      tracks.child(
        Popup.new("day-sel", div().size_full())
          .absolute()
          .left(s.x)
          .top(s.top)
          .w(s.w)
          .h(s.h)
          .bg(s.undo ? t.background : t.selection)
          .anchor("top_left")
          .content(SelectionMenu(this)),
      );
    }
    return h_flex().px(14).pt(4).gap(10).items_start().child(v_flex().w(96).flex_none().children(labels)).child(tracks);
  }

  // Hours under the lanes, every three from the day's start.
  axis(d, t) {
    const row = div().relative().h(16).w_full().text_size(TEXT.badge).text_color(t.dim);
    if (!this.width) return row;
    for (let ms = d.from; ms < d.to; ms += 3 * 3600_000)
      row.child(div().absolute().top(2).left(((ms - d.from) / (d.to - d.from)) * this.width).child(hhmm(ms)));
    return row;
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
