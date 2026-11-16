// The status-bar chip: today's time and the project in focus. Also what Herdr tells the collector.
import { h_flex } from "gpui-base";
import { theme, run, focused, spaces, notify } from "herdr";
import { call } from "../client.js";
import { formatDuration } from "../format.js";
import { beatFor, shouldSend } from "../signals.js";
import { Icon, TEXT } from "../ui.js";

export class Status {
  init(_given, cx) {
    this.status = null;
    this.last = null;
    this.sending = false;
    this.poll(cx);
    cx.timer.every(5000, (cx) => this.signal(cx));
    cx.timer.every(30000, (cx) => this.poll(cx));
  }

  poll(cx) {
    if (this.polling) return;
    this.polling = true;
    cx.spawn(async (cx) => {
      this.status = await call(["status"]);
      this.polling = false;
      cx.notify();
    });
  }

  signal(cx) {
    const beat = beatFor(focused(), spaces(), Date.now());
    if (!beat || this.sending || !shouldSend(this.last, beat, beat.time)) return;
    this.sending = true;
    cx.spawn(async (cx) => {
      const r = await call(["beat", JSON.stringify(beat)]);
      if (!r.down && !r.error) this.last = { beat, at: beat.time };
      this.sending = false;
    });
  }

  label() {
    const s = this.status;
    if (!s) return "…";
    if (s.down) return "сборщик не запущен";
    if (s.error) return "ошибка";
    return `${formatDuration(s.today_me_ms)} · ${s.project_name ?? s.project ?? "—"}`;
  }

  click() {
    if (this.status?.down)
      return notify({ title: "Time Tracker: сборщик не запущен", body: "Запустите: go -C helper run ./deploy", level: "warning" });
    run("mod.open", { name: "time-tracker" });
  }

  render() {
    const t = theme();
    return h_flex()
      .id("time-chip")
      .h_full()
      .px(8)
      .gap(5)
      .items_center()
      .text_size(TEXT.small)
      .text_color(this.status?.down || this.status?.error ? t.dim : t.foreground)
      .cursor_pointer()
      .hover((s) => s.text_color(t.accent))
      .accessibility_label("Time Tracker")
      .tooltip(this.status?.error ?? "Учёт времени")
      .on_click(() => this.click())
      .child(Icon("clock", 13))
      .child(this.label());
  }
}
