// The status-bar chip: today's time and the project in focus, with the day's summary in a card on
// hover (drawn as Usage Limits draws its own). Also what Herdr tells the collector.
import { div } from "gpui-kit";
import { h_flex, v_flex, HoverCard } from "gpui-base";
import { theme, run, focused, spaces, notify, layout, settings } from "herdr";
import { call } from "../client.js";
import { formatDuration, hhmm, catalog } from "../format.js";
import { chipState } from "../chip.js";
import { projectColor } from "../lanes.js";
import { beatFor, shouldSend } from "../signals.js";
import { Icon, TEXT } from "../ui.js";
import { ProjectBlock } from "./side.js";

const CARD_W = 340;
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
    this.lastSample = 0; // from the latest good status
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
        if (!this.status.down && !this.status.error) this.lastSample = this.status.last_sample ?? 0;
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
    if (!id) return "Другое";
    return this.catalog?.projects.find((p) => p.id === id)?.name || id;
  }

  chip(t) {
    const c = chipState(this.status, this.lastSample, Date.now());
    const chip = h_flex().id("time-chip").gap(6).items_center().flex_none().cursor_pointer().on_click(() => this.openPanel())
      .on_size_change((size) => this.fit(size.width));
    const text = (color, s) => div().whitespace_nowrap().text_color(color).child(s);
    const clock = (color = t.muted) => div().text_color(color).child(Icon("clock", 13));
    switch (c.kind) {
      case "wait":
        return chip.child(clock()).child(text(t.dim, "…"));
      case "silent":
        return chip.child(div().text_color(t.ansi3).child(Icon("triangle-alert", 13)))
          .child(text(t.ansi3, c.since ? `нет данных ${formatDuration(Date.now() - c.since)}` : "нет данных"));
      case "error":
        return chip.tooltip(c.text).child(clock()).child(text(t.ansi1 ?? t.dim, "ошибка"));
      case "none":
        return chip.child(div().text_color(t.muted).child(Icon("play", 12))).child(text(t.muted, formatDuration(c.me)));
      case "away":
        return chip.child(clock(t.dim)).child(text(t.dim, `${formatDuration(c.me)} · отошёл ${formatDuration(c.away)}`));
      default: {
        const order = this.catalog?.projects ?? [];
        return chip.child(clock()).child(text(t.foreground, formatDuration(c.me)))
          .when(Boolean(c.project), (el) =>
            el.child(text(t.dim, "·")).child(div().size(7).rounded(3.5).flex_none().bg(t[projectColor(c.project, order)] ?? t.muted))
              .child(text(t.muted, c.name || c.project)));
      }
    }
  }

  card(t) {
    const s = this.status;
    const r = this.day?.report;
    const rule = () => div().h(1).bg(t.border).flex_none();
    const body = v_flex().id("time-card").w(CARD_W).p(12).gap(10).rounded(8).border(1).border_color(t.border).bg(t.popover).shadow_lg()
      .text_size(12).text_color(t.foreground);
    if (this.day?.down || this.day?.error) return body.child(div().text_color(t.ansi3).whitespace_normal().child(this.day.down ? "Сборщик не отвечает" : this.day.error));
    if (!r) return body.child(div().text_color(t.dim).child(this.dayBusy ? "Загрузка…" : "Нет данных"));
    const order = this.catalog?.projects ?? [];
    const projects = r.projects.filter((p) => p.me >= 60_000);
    const max = Math.max(...projects.map((p) => p.me));
    const session = s?.session_start > 0 ? `сессия с ${hhmm(s.session_start)}` : "сессии нет";
    return body
      .child(h_flex().items_center().child(div().font_semibold().child("Сегодня")).child(div().flex_1()).child(div().font_semibold().child(formatDuration(r.me))))
      .child(div().text_size(TEXT.small).text_color(t.dim).child(`${session} · агенты ${formatDuration(r.agents)}`))
      .child(rule())
      .child(v_flex().gap(12).children(projects.map((p) =>
        ProjectBlock(t, { name: this.name(p.project), color: t[projectColor(p.project, order)] ?? t.muted, value: formatDuration(p.me), fill: (p.me / max) * 100, other: !p.project }))))
      .child(rule())
      .child(div().id("time-card-open").text_size(TEXT.small).text_color(t.muted).cursor_pointer().hover((e) => e.text_color(t.foreground)).on_click(() => this.openPanel()).child("Открыть панель ›"));
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
