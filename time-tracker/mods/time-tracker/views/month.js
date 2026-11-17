// The Месяц tab: the month as a calendar, each day with my time and its bar by project, the goal tick and
// the forecast; days selected on it are marked off. On the right the month's totals and goals.
import { div } from "gpui-kit";
import { h_flex, v_flex, Popup } from "gpui-base";
import { theme, props, layout } from "herdr";
import { call } from "../client.js";
import { fmtMin, signed, progressText, catalog, mainGoal, dayString, sumOf, monthGrid, cellBars, OFF_KINDS, offLabel, offTitle, workdaysText, offReduce, MONTH_NAMES } from "../format.js";
import { projectColor } from "../lanes.js";
import { Button, Check, Message, Segmented, TEXT } from "../ui.js";
import { PlanSide } from "./side.js";
import { panelShown } from "./day.js";

const REFRESH_MS = 60_000;
const WEEKDAYS = ["пн", "вт", "ср", "чт", "пт", "сб", "вс"];

export class Month {
  // openDay(date, cx) switches to the День tab; open(tab, cx, arg) shows another tab.
  constructor(openDay, open) {
    this.openDay = openDay;
    this.open = open;
    this.anchor = null; // the first day of the month shown; null is this month
    this.today = null;
    this.plan = null; // what `plan` gave for the month
    this.projects = [];
    this.error = null;
    this.message = null; // { text, error? }
    this.loading = false;
    this.busy = false;
    this.seq = 0;
    this.loadedAt = 0;
    this.wasShown = false;
    this.drag = null; // { from, to }: the day pressed and the day under the pointer
    this.closedAt = null; // where the press that closed the popup was
    this.sel = null; // { from, to }, ordered: the days the popup marks
    this.offKind = "otgul";
    this.offProjects = new Set();
  }

  tick(cx) {
    const shown = Boolean(props()?.overlay) || panelShown(layout());
    if (shown && (!this.wasShown || Date.now() - this.loadedAt >= REFRESH_MS)) this.load(cx);
    this.wasShown = shown;
  }

  load(cx) {
    const seq = ++this.seq;
    this.loading = true;
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        const tl = await call(["timeline"]);
        if (seq !== this.seq) return;
        if (tl.down || tl.error) {
          this.error = tl.down ? "Сборщик не запущен" : tl.error;
          return;
        }
        this.today = tl.day;
        const days = monthGrid(this.anchor ?? tl.day).filter(Boolean);
        const [plan, projects] = await Promise.all([call(["plan", "--from", days[0], "--to", days[days.length - 1]]), call(["projects"])]);
        if (seq !== this.seq) return;
        Object.assign(this, catalog(projects));
        this.error = plan.down ? "Сборщик не запущен" : (plan.error ?? null);
        if (!this.error) this.plan = plan;
      } finally {
        if (seq === this.seq) {
          this.loading = false;
          this.loadedAt = Date.now();
        }
        cx.notify();
      }
    });
  }

  // The month n months on; the old month's data goes at once.
  step(n, cx) {
    const base = this.anchor ?? this.today;
    if (!base) return;
    const [y, m] = base.split("-").map(Number);
    const next = dayString(new Date(y, m - 1 + n, 1).getTime());
    this.anchor = next.slice(0, 7) >= this.today.slice(0, 7) ? null : next;
    Object.assign(this, { plan: null, error: null, message: null, drag: null, sel: null });
    this.load(cx);
  }

  name(id) {
    if (!id) return "Другое";
    return this.projects.find((p) => p.id === id)?.name || id;
  }

  // Projects with a goal, in config order.
  goalIds() {
    return this.projects.map((p) => p.id).filter((id) => this.plan?.goals?.[id]);
  }

  // Selection (rulings): press on a day and release on another selects the days between; shift-click
  // selects one day; a plain click opens the day. A press that closed the popup does nothing else.
  press(day, e, cx) {
    const closed = this.closedAt;
    this.closedAt = null;
    if (closed && closed.x === e.position?.x && closed.y === e.position?.y) return;
    if (e.modifiers?.shift) return this.select(day, day, cx);
    this.drag = { from: day, to: day };
    cx.notify();
  }

  hover(day, cx) {
    if (!this.drag || this.drag.to === day) return;
    this.drag.to = day;
    cx.notify();
  }

  // day: the day released on; null off the days, which ends a drag on the last day it crossed.
  release(day, cx) {
    const g = this.drag;
    if (!g) return;
    this.drag = null;
    if (day === g.from) this.openDay(day, cx);
    else if (day || g.to !== g.from) this.select(g.from, day ?? g.to, cx);
    cx.notify();
  }

  select(a, b, cx) {
    this.sel = a <= b ? { from: a, to: b } : { from: b, to: a };
    this.offKind = "otgul";
    this.offProjects = new Set(this.goalIds());
    this.message = null;
    cx.notify();
  }

  // e: the press outside the popup that closed it, which the day pressed then has to ignore.
  clearSel(cx, e) {
    this.sel = null;
    this.closedAt = e?.position ?? null;
    cx.notify();
  }

  mark(cx) {
    if (this.busy || !this.sel) return;
    const { from, to } = this.sel;
    const kind = this.offKind;
    const body = { from, to, kind, projects: this.goalIds().filter((id) => this.offProjects.has(id)) };
    this.busy = true;
    cx.spawn(async (cx) => {
      try {
        const r = await call(["dayoff", "add", JSON.stringify(body)]);
        if (r.down || r.error) this.message = { text: r.down ? "Сборщик не запущен" : r.error, error: true };
        else {
          this.message = { text: `Отмечено: ${offTitle(from, to)} · ${offLabel(kind)}.` };
          this.sel = null;
          this.load(cx);
        }
      } finally {
        this.busy = false;
        cx.notify();
      }
    });
  }

  render() {
    const t = theme();
    return h_flex()
      .flex_1()
      .min_h(0)
      .items_stretch()
      .child(this.left(t))
      .when(Boolean(this.plan), (el) => el.child(this.side(t)));
  }

  left(t) {
    const col = v_flex().id("month").flex_1().min_w(0).min_h(0).overflow_y_scroll().pt(18).px(22).gap(14).child(this.header(t));
    if (this.error) col.child(Message(this.error, true));
    if (this.message) col.child(div().flex_none().text_size(TEXT.small).whitespace_normal().text_color(this.message.error ? t.ansi1 : t.muted).child(this.message.text));
    const plan = this.plan;
    if (!plan) return col.child(Message(this.loading ? "Загрузка…" : ""));
    const order = this.projects.map((p) => p.id);
    const main = mainGoal(plan.goals, order);
    const byDay = Object.fromEntries(plan.days.map((d) => [d.day, d]));
    const g = this.drag ?? this.sel;
    const range = g && (g.from <= g.to ? [g.from, g.to] : [g.to, g.from]);
    const ctx = { plan, order, main, sum: main && plan.sums?.[main], byDay, range };
    const cells = monthGrid(this.anchor ?? plan.today);
    const rows = [0, 1, 2, 3, 4, 5].map((r) => h_flex().gap(6).children(cells.slice(r * 7, r * 7 + 7).map((day) => this.cell(t, day, ctx))));
    const grid = v_flex()
      .id("month-grid")
      .gap(6)
      .children(rows)
      .on_mouse_up("left", (_e, cx) => this.release(null, cx))
      .on_hover((hovered, cx) => !hovered && this.release(null, cx));
    return col
      .child(h_flex().flex_none().gap(6).pl(8).text_size(TEXT.caption).text_color(t.dim).children(WEEKDAYS.map((w) => div().flex_1().min_w(0).child(w))))
      .child(Popup.new("month-sel", grid).anchor("top_left").when(Boolean(this.sel), (el) => el.content(this.popup(t))));
  }

  header(t) {
    const day = this.anchor ?? this.today;
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
        .on_click((_e, cx) => this.step(n, cx))
        .child(glyph);
    const name = day && MONTH_NAMES[Number(day.slice(5, 7)) - 1];
    const missing = this.plan?.no_calendar ?? [];
    const note = (text) => div().text_size(TEXT.small).text_color(t.dim).child(text);
    return h_flex()
      .flex_none()
      .gap(10)
      .items_center()
      .child(chevron("month-prev", "Предыдущий месяц", "‹", -1))
      .child(div().flex_none().text_size(15).font_semibold().child(day ? `${name[0].toUpperCase()}${name.slice(1)} ${day.slice(0, 4)}` : "…"))
      .when(Boolean(day) && Boolean(this.anchor), (el) => el.child(chevron("month-next", "Следующий месяц", "›", 1)))
      .child(div().flex_1())
      .child(
        v_flex()
          .flex_none()
          .items_end()
          .child(note("полоска — доля от 8 ч · риска — цель · бледная — прогноз"))
          .when(missing.length > 0, (el) => el.child(note(`календаря на ${missing.join(", ")} нет — считаются пн–пт`))),
      );
  }

  cell(t, day, { plan, order, main, sum, byDay, range }) {
    if (!day) return div().flex_1().min_w(0).h(64);
    const d = byDay[day] ?? { day, me: {}, plan: {} };
    const future = day > plan.today, today = day === plan.today;
    const total = sumOf(d.me);
    const b = cellBars(d, order, main, sum, plan.today);
    const color = (id) => t[projectColor(id, this.projects)] ?? t.muted;
    const track = div().relative().h(4).w_full().flex_none().rounded(2).overflow_hidden().bg(t.raised);
    let left = 0;
    for (const p of b.parts) {
      track.child(div().absolute().top(0).h(4).left(`${left}%`).w(`${p.pct}%`).bg(color(p.project)));
      left += p.pct;
    }
    if (b.forecast !== null) track.child(div().absolute().top(0).left(0).h(4).w(`${b.forecast}%`).bg(color(main)).opacity(0.3));
    if (b.tick !== null) track.child(div().absolute().top(0).h(4).w(2).ml(-1).left(`${Math.min(b.tick, 100)}%`).bg(t.foreground).opacity(0.55));
    const middle = div().font_medium();
    if (d.holiday && !total) middle.text_color(t.dim).child("праздник");
    else if (!future) middle.when(!total, (el) => el.text_color(t.dim)).child(fmtMin(total));
    const corner = today ? div().text_color(t.foreground).child("сегодня") : d.off ? div().text_color(t.dim).child(offLabel(d.off)) : null;
    const selected = range && range[0] <= day && day <= range[1];
    return v_flex()
      .id(`month-day-${day}`)
      .flex_1()
      .min_w(0)
      .h(64)
      .justify_between()
      .py(7)
      .px(8)
      .rounded(6)
      .cursor_pointer()
      .when(!future, (el) => el.bg(t.chrome))
      .when(today, (el) => el.border_1().border_color(t.muted))
      .when(future, (el) => el.border_1().border_color(t.border))
      .when(Boolean(selected), (el) => el.bg(t.hover).border_1().border_color(t.foreground))
      .on_mouse_down("left", (e, cx) => this.press(day, e, cx))
      .on_mouse_up("left", (_e, cx) => this.release(day, cx))
      .on_hover((hovered, cx) => hovered && this.hover(day, cx))
      .child(
        h_flex()
          .text_size(TEXT.caption)
          .child(div().text_color(today ? t.foreground : t.dim).when(future, (el) => el.opacity(0.6)).child(String(Number(day.slice(8)))))
          .child(div().flex_1())
          .when(Boolean(corner), (el) => el.child(corner)),
      )
      .child(middle)
      .child(track);
  }

  // What marking the selected days off does; a press outside closes it.
  popup(t) {
    const { from, to } = this.sel;
    const days = this.plan.days.filter((d) => from <= d.day && d.day <= to);
    const ids = this.goalIds();
    const row = (id) => {
      const on = this.offProjects.has(id);
      const label = h_flex()
        .flex_1()
        .min_w(0)
        .gap(8)
        .items_center()
        .child(div().size(8).flex_none().rounded(4).bg(t[projectColor(id, this.projects)] ?? t.muted))
        .child(div().min_w(0).truncate().child(this.name(id)))
        .child(div().flex_1())
        .child(div().flex_none().when(!on, (el) => el.text_color(t.dim)).child(`−${fmtMin(offReduce(days, id))}`));
      const toggle = (cx) => {
        if (on) this.offProjects.delete(id);
        else this.offProjects.add(id);
        cx.notify();
      };
      return Check({ id: `month-off-${id}`, checked: on, label, onClick: toggle }).w_full();
    };
    return v_flex()
      .id("month-off")
      .w(320)
      .p(12)
      .gap(10)
      .rounded(8)
      .border_1()
      .border_color(t.border)
      .bg(t.popover)
      .shadow_lg()
      .text_color(t.foreground)
      .on_mouse_down_out((e, cx) => this.clearSel(cx, e))
      .child(
        v_flex()
          .gap(2)
          .child(div().font_semibold().child(offTitle(from, to)))
          .child(div().text_size(TEXT.small).text_color(t.dim).child(workdaysText(days.filter((d) => d.work).length))),
      )
      .child(Segmented({ id: "month-off-kind", options: OFF_KINDS, value: this.offKind, onChange: (id, cx) => ((this.offKind = id), cx.notify()) }))
      .when(ids.length > 0, (el) => el.child(div().text_size(TEXT.small).text_color(t.dim).child("Снизить план")).children(ids.map(row)))
      .child(
        h_flex()
          .gap(8)
          .child(div().flex_1())
          .child(Button({ id: "month-off-cancel", label: "Отмена", onClick: (cx) => this.clearSel(cx) }))
          .child(Button({ id: "month-off-mark", label: "Отметить", primary: true, onClick: (cx) => this.mark(cx) })),
      );
  }

  side(t) {
    const plan = this.plan;
    const past = plan.days.filter((d) => d.day <= plan.today);
    const me = past.reduce((n, d) => n + sumOf(d.me), 0);
    const worked = past.filter((d) => sumOf(d.me) > 0).length;
    const lines = [`агенты ${fmtMin(past.reduce((n, d) => n + sumOf(d.agents), 0))} · ${worked} дн. с работой · ср. ${fmtMin(worked ? Math.floor(me / worked) : 0)}`];
    const fc = !this.anchor && this.goalIds().find((id) => plan.goals[id].dir === "min" && plan.sums?.[id]);
    if (fc) lines.push(`прогноз ${this.name(fc)} ≈ ${fmtMin(plan.sums[fc].forecast)} из ${fmtMin(plan.sums[fc].plan)}`);
    const goal = (g, s) => ({
      goal: fmtMin(s.plan_to_date),
      fill: s.plan_to_date > 0 ? Math.min(100, (s.fact / s.plan_to_date) * 100) : 0,
      sub:
        g.period === "day"
          ? `${signed(s.balance)}${s.need > 0 && g.dir !== "max" ? ` · дальше ${fmtMin(s.need)} в раб. день` : ""}`
          : `цель ${fmtMin(g.minutes)} в ${g.period === "week" ? "нед." : "мес."} · ${progressText(g, s.fact, s.plan_to_date).replace(/^цель /, "")}`,
    });
    const month = MONTH_NAMES[Number(plan.days[0].day.slice(5, 7)) - 1];
    return PlanSide(this, t, { id: "month", caption: `Моё время за ${month}`, total: me, lines, goal });
  }
}
