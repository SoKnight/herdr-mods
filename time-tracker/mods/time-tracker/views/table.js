// The Week and Month tabs: projects as rows, kinds of activity as columns, then my time and the agents'.
// Below, every day of the period; a click opens that day in the Day tab.
import { div } from "gpui-kit";
import { h_flex, v_flex } from "gpui-base";
import { theme, props, layout } from "herdr";
import { call } from "../client.js";
import { formatDuration, activityLabel, weekRange, monthRange, dayString, catalog, ACTIVITIES } from "../format.js";
import { shiftDay } from "../lanes.js";
import { IconButton, Message, TEXT } from "../ui.js";
import { Link } from "./menu.js";
import { panelShown } from "./day.js";

const REFRESH_MS = 60_000;
const WEEKDAYS = ["вс", "пн", "вт", "ср", "чт", "пт", "сб"];
const MONTHS = ["январь", "февраль", "март", "апрель", "май", "июнь", "июль", "август", "сентябрь", "октябрь", "ноябрь", "декабрь"];

const label = (day) => `${day.slice(8)}.${day.slice(5, 7)}`;

// Every day label of [from, to), the way the collector names them.
function daysOf(from, to) {
  const out = [];
  for (const c = new Date(from); c.getTime() < to; c.setDate(c.getDate() + 1)) out.push(dayString(c.getTime()));
  return out;
}

export class Table {
  // mode: "week" | "month"; openDay(date, cx) switches to the Day tab.
  constructor(mode, openDay) {
    this.mode = mode;
    this.openDay = openDay;
    this.anchor = null; // a day of the period shown; null is today's
    this.today = null;
    this.dayStart = 4 * 3600_000;
    this.report = null;
    this.range = null;
    this.projects = [];
    this.activities = ACTIVITIES;
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
          this.error = tl.down ? "Сборщик не запущен: go -C helper run ./deploy" : tl.error;
          return;
        }
        this.today = tl.day;
        this.dayStart = tl.from - new Date(`${tl.day}T00:00:00`).getTime();
        const day = this.anchor ?? tl.day;
        const range = (this.mode === "week" ? weekRange : monthRange)(day, this.dayStart);
        const [rep, projects] = await Promise.all([call(["report", "--from", String(range.from), "--to", String(range.to)]), call(["projects"])]);
        if (seq !== this.seq) return;
        Object.assign(this, catalog(projects));
        this.error = rep.down ? "Сборщик не запущен" : (rep.error ?? null);
        if (!this.error) Object.assign(this, { report: rep, range });
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
    const [y, m, d] = base.split("-").map(Number);
    const next = this.mode === "week" ? shiftDay(base, 7 * n) : dayString(new Date(y, m - 1 + n, 1).getTime());
    this.anchor = next;
    this.load(cx);
  }

  name(id) {
    return id ? (this.projects.find((p) => p.id === id)?.name || id) : "Без проекта";
  }

  title() {
    const r = this.range;
    if (!r) return "…";
    if (this.mode === "month") {
      const d = new Date(r.from);
      return `${MONTHS[d.getMonth()]} ${d.getFullYear()}`;
    }
    return `${label(dayString(r.from))} – ${label(dayString(r.to - 24 * 3600_000))}`;
  }

  render() {
    const t = theme();
    const rep = this.report;
    const root = v_flex().id(`table-${this.mode}`).flex_1().min_h(0).overflow_y_scroll().pb(14).child(this.bar(t));
    if (this.error) root.child(Message(this.error, true));
    if (!rep) return root.child(Message(this.loading ? "Загрузка…" : ""));
    const listed = this.activities.map((a) => a.id);
    const kinds = [...listed, ...new Set(rep.projects.flatMap((p) => Object.keys(p.activities)).filter((a) => !listed.includes(a)))].filter((a) =>
      rep.projects.some((p) => p.activities[a] > 0),
    );
    return root.child(this.grid(rep, kinds, t)).child(this.days(rep, t));
  }

  bar(t) {
    const current = this.range && this.today && this.range.from <= Date.now() && Date.now() < this.range.to;
    return h_flex()
      .h(40)
      .flex_none()
      .px(14)
      .gap(10)
      .items_center()
      .child(Link({ id: `table-${this.mode}-prev`, label: "‹", onClick: (cx) => this.step(-1, cx) }))
      .child(div().text_size(TEXT.body).child(this.title()))
      .when(!current, (el) => el.child(Link({ id: `table-${this.mode}-next`, label: "›", onClick: (cx) => this.step(1, cx) })))
      .child(div().flex_1())
      .child(IconButton({ id: `table-${this.mode}-refresh`, icon: "refresh", label: "Обновить", spinning: this.loading, onClick: (cx) => this.load(cx) }));
  }

  grid(rep, kinds, t) {
    const cell = (text, w, head = false) => div().w(w).flex_none().text_size(TEXT.small).text_color(head ? t.muted : t.foreground).child(text);
    const head = h_flex().h(26).gap(8).items_center().child(div().flex_1().min_w(0)).children(kinds.map((a) => cell(activityLabel(a, this.activities), 96, true))).child(cell("Моё", 70, true)).child(cell("Агенты", 70, true));
    const box = v_flex().px(14).pt(4).child(head);
    for (const p of rep.projects) {
      box.child(
        h_flex()
          .h(26)
          .gap(8)
          .items_center()
          .child(div().flex_1().min_w(0).truncate().text_size(TEXT.small).child(this.name(p.project)))
          .children(kinds.map((a) => cell(p.activities[a] ? formatDuration(p.activities[a]) : "", 96)))
          .child(cell(formatDuration(p.me), 70))
          .child(cell(formatDuration(p.agents), 70)),
      );
    }
    return box.when(!rep.projects.length, (el) => el.child(Message("За период времени нет.")));
  }

  days(rep, t) {
    const sum = (day, key) => rep.projects.reduce((s, p) => s + (p.days[day]?.[key] ?? 0), 0);
    const box = v_flex().px(14).pt(14).gap(2).child(div().h(22).text_size(TEXT.small).text_color(t.muted).child("По дням"));
    for (const day of daysOf(this.range.from, this.range.to)) {
      const me = sum(day, "me");
      const agents = sum(day, "agents");
      if (!me && !agents) continue;
      box.child(
        h_flex()
          .id(`table-day-${day}`)
          .h(24)
          .gap(10)
          .items_center()
          .text_size(TEXT.small)
          .cursor_pointer()
          .hover((s) => s.text_color(t.accent))
          .on_click((_e, cx) => this.openDay(day, cx))
          .child(div().w(70).flex_none().child(`${WEEKDAYS[new Date(`${day}T12:00:00`).getDay()]} ${label(day)}`))
          .child(div().w(90).flex_none().child(`моё ${formatDuration(me)}`))
          .child(div().text_color(t.muted).child(`агенты ${formatDuration(agents)}`)),
      );
    }
    return box;
  }
}
