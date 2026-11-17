// The status-bar chip: today's time and the project in focus, with the day's summary in a card on
// hover (drawn as Usage Limits draws its own). Also what Herdr tells the collector.
import { div } from "gpui-kit";
import { h_flex, v_flex, HoverCard } from "gpui-base";
import { theme, run, focused, spaces, notify, layout, settings } from "herdr";
import { call } from "../client.js";
import { formatDuration, activityLabel, catalog } from "../format.js";
import { projectColor } from "../lanes.js";
import { beatFor, shouldSend } from "../signals.js";
import { Icon, IconButton, TEXT } from "../ui.js";
import { ProjectRow, TotalsHead } from "./totals.js";

const CARD_W = 360;
const CARD_EVERY = 30_000; // ms, at most this often while hovered
const OPEN_DELAY = 350; // ms, so passing over the bar doesn't flash it
const CLOSE_DELAY = 300; // ms, lets the pointer cross from the chip to the card
const PAD = 12; // px each side of the chip

// The layout block that holds the chip.
function findBlock(node) {
  if (!node) return null;
  if (node.content?.mod === "time-tracker" && node.content.props?.view === "status") return node;
  for (const c of node.children ?? []) {
    const b = findBlock(c);
    if (b) return b;
  }
  return null;
}

export class Status {
  init(_given, cx) {
    this.status = null;
    this.day = null; // `timeline` for today, read when the card opens
    this.catalog = null; // { projects, activities }
    this.dayAt = 0;
    this.dayBusy = false;
    this.last = null;
    this.sending = false;
    this.polling = false;
    this.poll(cx);
    cx.timer.every(5000, (cx) => this.signal(cx));
    cx.timer.every(30000, (cx) => this.poll(cx));
  }

  poll(cx) {
    if (this.polling) return;
    this.polling = true;
    cx.spawn(async (cx) => {
      try {
        this.status = await call(["status"]);
        if (!this.catalog && !this.status.down && !this.status.error) this.catalog = catalog(await call(["projects"])); // the project's colour
      } finally {
        this.polling = false;
      }
      cx.notify();
    });
  }

  // The card's numbers: today's report, read on hover and at most every 30 s.
  readDay(cx, force = false) {
    if (this.dayBusy || (!force && Date.now() - this.dayAt < CARD_EVERY)) return;
    this.dayBusy = true;
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        const [tl, projects] = await Promise.all([call(["timeline"]), call(["projects"])]);
        this.day = tl;
        this.catalog = catalog(projects) ?? this.catalog;
        this.dayAt = Date.now();
      } finally {
        this.dayBusy = false;
        cx.notify();
      }
    });
    this.poll(cx);
  }

  signal(cx) {
    const beat = beatFor(focused(), spaces(), Date.now());
    if (!beat || this.sending || !shouldSend(this.last, beat, beat.time)) return;
    this.sending = true;
    cx.spawn(async (cx) => {
      try {
        const r = await call(["beat", JSON.stringify(beat)]);
        if (!r.down && !r.error) this.last = { beat, at: beat.time };
      } finally {
        this.sending = false;
      }
    });
  }

  openPanel() {
    if (this.status?.down)
      return notify({ title: "Time Tracker: сборщик не запущен", body: "Запустите: go -C helper run ./deploy", level: "warning" });
    run("mod.open", { name: "time-tracker" });
  }

  // The layout can't size a block to its content, so the chip asks for its own width. Sizes reach
  // us scaled by zoom, while layout sizes are written as at zoom 1.
  fit(width) {
    const px = Math.ceil(width / (settings().zoom || 1)) + 2 * PAD;
    const block = findBlock(layout());
    if (!block || Math.abs((block.size?.px ?? 0) - px) < 2) return;
    run("layout.resize", { id: block.id, px });
  }

  name(id) {
    if (!id) return "Без проекта";
    return this.catalog?.projects.find((p) => p.id === id)?.name || id;
  }

  chip(t) {
    const s = this.status;
    const chip = h_flex().id("time-chip").gap(6).items_center().flex_none().cursor_pointer().on_click(() => this.openPanel())
      .on_size_change((size) => this.fit(size.width))
      .child(div().text_color(t.muted).child(Icon("clock", 13)));
    if (!s) return chip.child(div().text_color(t.dim).child("…"));
    if (s.down) return chip.child(div().text_color(t.dim).child("сборщик не запущен"));
    if (s.error) return chip.tooltip(s.error).child(div().text_color(t.ansi1 ?? t.dim).child("ошибка"));
    const order = this.catalog?.projects.map((p) => p.id) ?? [];
    return chip
      .child(div().whitespace_nowrap().text_color(t.foreground).child(formatDuration(s.today_me_ms)))
      .when(Boolean(s.project), (el) =>
        el.child(div().size(6).rounded(3).flex_none().bg(t[projectColor(s.project, order)] ?? t.muted))
          .child(div().whitespace_nowrap().text_color(t.muted).child(s.project_name || s.project)));
  }

  card(t) {
    const s = this.status;
    const rule = () => div().h(1).bg(t.border).flex_none();
    const body = v_flex().id("time-card").w(CARD_W).p(12).gap(10).rounded(8).border(1).border_color(t.border).bg(t.popover).shadow_lg()
      .text_size(12).text_color(t.foreground);
    const r = this.day?.report;
    body.child(h_flex().gap(6).items_center()
      .child(div().flex_none().font_semibold().child("Сегодня"))
      .child(div().flex_1())
      .child(div().text_size(TEXT.caption).text_color(t.dim).child(r ? `аптайм ${formatDuration(r.uptime)}` : this.dayBusy ? "загрузка…" : "")));
    if (s && !s.down && !s.error) {
      const order = this.catalog?.projects.map((p) => p.id) ?? [];
      body.child(h_flex().gap(8).items_center()
        .child(div().text_color(t.dim).child("Сейчас"))
        .child(div().size(8).rounded(4).flex_none().bg(s.project ? t[projectColor(s.project, order)] ?? t.muted : t.dim))
        .child(div().min_w(0).truncate().child(s.project ? s.project_name || s.project : "без проекта"))
        .when(Boolean(s.activity), (el) => el.child(div().flex_none().text_color(t.muted).child(`· ${activityLabel(s.activity, this.catalog?.activities)}`))));
    }
    if (this.day?.down || this.day?.error) {
      body.child(rule()).child(div().text_size(TEXT.caption).text_color(t.ansi1 ?? t.dim).whitespace_normal().child(this.day.down ? "Сборщик не запущен" : this.day.error));
    } else if (r) {
      const order = this.catalog?.projects.map((p) => p.id) ?? [];
      const rows = v_flex().gap(7).child(TotalsHead(t));
      if (r.projects.length) rows.children(r.projects.map((p) => ProjectRow(p, this.name(p.project), order, r.me, t)));
      else rows.child(div().text_color(t.dim).child("Пока ничего не насчитано."));
      body.child(rule()).child(rows);
    }
    return body.child(rule())
      .child(h_flex().gap(2).items_center()
        .child(div().flex_1().min_w(0).truncate().text_size(TEXT.caption).text_color(t.dim)
          .child(r ? `моё ${formatDuration(r.me)} · агенты ${formatDuration(r.agents)}` : ""))
        .child(IconButton({ id: "time-card-refresh", icon: "refresh", label: "Обновить", spinning: this.dayBusy, onClick: (cx) => this.readDay(cx, true) }))
        .child(IconButton({ id: "time-card-open", icon: "panel", label: "Открыть панель", onClick: () => this.openPanel() })));
  }

  render() {
    const t = theme();
    const trigger = h_flex().id("time-trigger").h_full().items_center().on_hover((on, cx) => on && this.readDay(cx)).child(this.chip(t));
    return h_flex().id("time-root").size_full().px(PAD).items_center().justify_end().overflow_hidden()
      .bg(t.chrome).border_t(1).border_color(t.border).text_size(11.8)
      .accessibility_label("Time Tracker")
      .child(HoverCard.new("time-panel").anchor("bottom_right").open_delay(OPEN_DELAY).close_delay(CLOSE_DELAY).trigger(trigger).content(this.card(t)));
  }
}
