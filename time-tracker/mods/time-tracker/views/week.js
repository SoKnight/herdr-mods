// The Неделя tab: the seven days as strips with my time, the agents' and the day's goal difference;
// on the right the week's totals and goals. A row opens that day in the День tab.
import { div } from "gpui-kit";
import { h_flex, v_flex } from "gpui-base";
import { theme, props, layout } from "herdr";
import { call } from "../client.js";
import { formatDuration, fmtMin, signed, progressText, hhmm, catalog, weekDays, weekTitle, weekRelative, mainGoal, weekRows } from "../format.js";
import { bars, projectColor, shiftDay } from "../lanes.js";
import { Icon, Message, TEXT } from "../ui.js";
import { Legend } from "./strip.js";
import { PlanSide } from "./side.js";
import { panelShown } from "./day.js";

const REFRESH_MS = 60_000;
const W = 600; // the px bars are laid out in, drawn in percent of the strip

const items = (segs, valueOf) => (segs ?? []).map((seg) => ({ start: seg.start, end: seg.end, value: valueOf(seg), seg }));
const pct = (px) => `${(px / W) * 100}%`;
const roundMs = (ms) => Math.floor(ms / 60000 + 0.5); // half up, as the collector rounds a day

export class Week {
  // openDay(date, cx) switches to the День tab; open(tab, cx, arg) shows another tab.
  constructor(openDay, open) {
    this.openDay = openDay;
    this.open = open;
    this.anchor = null; // a day of the week shown; null is this week
    this.today = null;
    this.from = 0; // today's start, ms: the day start the axis is labelled with
    this.plan = null; // what `plan` gave for the week
    this.timelines = {}; // day → what `timeline --day` gave, days up to today
    this.projects = [];
    this.error = null;
    this.loading = false;
    this.seq = 0;
    this.loadedAt = 0;
    this.wasShown = false;
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
        this.from = tl.from;
        const days = weekDays(this.anchor ?? tl.day);
        const past = days.filter((d) => d <= tl.day);
        // ponytail: seven timeline calls for the strips; one range call if Неделя feels slow
        const [plan, projects, ...tls] = await Promise.all([
          call(["plan", "--from", days[0], "--to", days[6]]),
          call(["projects"]),
          ...past.map((d) => (d === tl.day ? tl : call(["timeline", "--day", d]))),
        ]);
        if (seq !== this.seq) return;
        Object.assign(this, catalog(projects));
        this.error = plan.down ? "Сборщик не запущен" : (plan.error ?? null);
        if (this.error) return;
        this.plan = plan;
        this.timelines = Object.fromEntries(past.map((d, i) => [d, tls[i]?.timeline ? tls[i] : null]));
      } finally {
        if (seq === this.seq) {
          this.loading = false;
          this.loadedAt = Date.now();
        }
        cx.notify();
      }
    });
  }

  step(n, cx) {
    const base = this.anchor ?? this.today;
    if (!base) return;
    const next = shiftDay(base, 7 * n);
    this.anchor = weekDays(next)[0] >= weekDays(this.today)[0] ? null : next;
    this.load(cx);
  }

  name(id) {
    if (!id) return "Другое";
    return this.projects.find((p) => p.id === id)?.name || id;
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
    const col = v_flex().id("week").flex_1().min_w(0).min_h(0).overflow_y_scroll().pt(18).px(22).pb(18).gap(14).child(this.header(t));
    if (this.error) col.child(Message(this.error, true));
    const plan = this.plan;
    if (!plan) return col.child(Message(this.loading ? "Загрузка…" : ""));
    const main = mainGoal(plan.goals, this.projects.map((p) => p.id));
    const { rows, totals } = weekRows(plan, main);
    const cell = (w, text) => div().w(w).flex_none().text_right().child(text);
    const label = (text, color) => div().w(48).flex_none().text_color(color).child(text);
    const table = v_flex().flex_none().child(this.axis(t, main));
    for (const r of rows) {
      const shown = r.day <= plan.today;
      table.child(
        h_flex()
          .id(`week-day-${r.day}`)
          .h(38)
          .gap(12)
          .px(8)
          .items_center()
          .border_t(1)
          .border_color(t.chrome)
          .cursor_pointer()
          .hover((s) => s.bg(t.hover))
          .on_click((_e, cx) => this.openDay(r.day, cx))
          .child(label(r.label, r.weekend ? t.dim : t.foreground))
          .child(this.strip(t, shown ? this.timelines[r.day] : null))
          .child(cell(56, shown ? fmtMin(r.agents) : "").text_color(t.dim))
          .child(cell(60, shown ? fmtMin(r.me) : "").font_medium())
          .when(Boolean(main), (el) => el.child(cell(64, r.diff.text).text_color(r.diff.low ? t.foreground : t.dim))),
      );
    }
    table.child(
      h_flex()
        .h(38)
        .gap(12)
        .px(8)
        .items_center()
        .border_t(1)
        .border_color(t.border)
        .child(label("Итого", t.foreground))
        .child(div().flex_1())
        .child(cell(56, fmtMin(totals.agents)).text_color(t.dim))
        .child(cell(60, fmtMin(totals.me)).font_semibold())
        .when(Boolean(main), (el) => el.child(cell(64, totals.diff).font_semibold())),
    );
    return col
      .child(table)
      .child(h_flex().flex_none().gap(12).items_center().child(Legend(t, true, false)).child(div().flex_1()).child(div().flex_none().text_size(TEXT.caption).text_color(t.dim).child("строка → открыть день")));
  }

  header(t) {
    const days = this.today && weekDays(this.anchor ?? this.today);
    const current = !this.anchor;
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
    return h_flex()
      .flex_none()
      .gap(10)
      .items_center()
      .child(chevron("week-prev", "Предыдущая неделя", "‹", -1))
      .child(div().flex_none().text_size(15).font_semibold().child(days ? weekTitle(days) : "…"))
      .when(Boolean(days), (el) => el.child(div().text_color(t.dim).child(weekRelative(days, this.today))))
      .when(Boolean(days) && !current, (el) => el.child(chevron("week-next", "Следующая неделя", "›", 1)))
      .child(div().flex_1())
      .when(this.from > 0, (el) => el.child(div().text_size(TEXT.small).text_color(t.dim).child(`сутки ${hhmm(this.from)} → ${hhmm(this.from)}`)));
  }

  // The column titles, the strip's hours at 0 / 25 / 50 / 75 / 100 %.
  axis(t, main) {
    const hour = (i) => hhmm(this.from + i * 6 * 3600_000);
    const mark = (i) => {
      if (i === 0) return div().absolute().top(0).left(0).child(hour(i));
      if (i === 4) return div().absolute().top(0).right(0).child(hour(i));
      return div().absolute().top(0).left(`${i * 25 - 10}%`).w("20%").text_center().child(hour(i));
    };
    const col = (w, text) => div().w(w).flex_none().text_right().child(text);
    return h_flex()
      .h(18)
      .gap(12)
      .px(8)
      .items_center()
      .text_size(TEXT.caption)
      .text_color(t.dim)
      .child(div().w(48).flex_none())
      .child(div().relative().flex_1().min_w(0).h(18).children([0, 1, 2, 3, 4].map(mark)))
      .child(col(56, "агенты"))
      .child(col(60, "моё"))
      .when(Boolean(main), (el) => el.child(col(64, "цель дня")));
  }

  // A day's strip: the computer on, Досуг, my time by project; empty for days to come.
  strip(t, d) {
    const track = div().relative().flex_1().min_w(0).h(12).rounded(3).overflow_hidden().bg(t.chrome);
    if (!d) return track;
    const { from, to, timeline: tl } = d;
    const block = (bar) => div().absolute().top(0).h(12).left(pct(bar.x)).w(pct(bar.w));
    bars(items(tl.power, () => "on"), from, to, W, () => "raised").forEach((bar) => track.child(block(bar).bg(t.raised)));
    bars(items(tl.leisure, () => "leisure"), from, to, W, () => "raised").forEach((bar) => track.child(block(bar).bg(t.raised).border_1().border_color(t.muted)));
    bars(items(tl.me, (s) => s.project), from, to, W, (v) => projectColor(v, this.projects)).forEach((bar) => track.child(block(bar).bg(t[bar.color] ?? t.muted)));
    return track;
  }

  side(t) {
    const plan = this.plan;
    const past = plan.days.filter((d) => d.day <= plan.today);
    const { totals } = weekRows(plan, null);
    const tls = past.map((d) => this.timelines[d.day]).filter(Boolean);
    const worked = past.filter((d) => Object.values(d.me ?? {}).some((m) => m > 0)).map((d) => this.timelines[d.day]?.stats).filter(Boolean);
    const lines = [["bot", "агенты", fmtMin(totals.agents)], ["gauge", "в среднем в день", fmtMin(Math.floor(totals.me / Math.max(1, past.length)))]];
    if (totals.me > 0 && worked.length) {
      const n = Math.round(worked.reduce((s, x) => s + x.switches, 0) / worked.length);
      lines.push(["arrow-left-right", "переключений в день", String(n)], ["focus", "фокус до", formatDuration(Math.max(...worked.map((x) => x.focus)))]);
    }
    const goal = (g, s, id, sum) => {
      const agents = sum("agents", id);
      const sub = g.period === "day" ? [signed(s.balance), agents >= 1 && `агенты ${fmtMin(agents)}`].filter(Boolean).join(" · ") : progressText(g, s.fact, s.plan_to_date);
      return { goal: fmtMin(s.plan), fill: Math.min(100, (s.fact / s.plan) * 100), sub };
    };
    const leisure = tls.reduce((n, d) => n + roundMs(d.report?.leisure ?? 0), 0);
    const extra = leisure > 0 ? [h_flex().pt(10).gap(8).items_center().border_t(1).border_color(t.border).child(div().text_color(t.muted).child(Icon("gamepad-2", 14))).child("Досуг").child(div().flex_1()).child(div().font_medium().child(fmtMin(leisure)))] : [];
    return PlanSide(this, t, { id: "week", caption: "Моё время за неделю", total: totals.me, lines, goal, extra });
  }
}
