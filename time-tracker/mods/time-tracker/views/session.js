// The panel's Сессия tab: the open session as a strip and stretches, or the empty state when none is open.
import { div } from "gpui-kit";
import { h_flex, v_flex } from "gpui-base";
import { call } from "../client.js";
import { formatDuration, hhmm, plural, sessionNote } from "../format.js";
import { sessionWindow } from "../lanes.js";
import { Button, Icon, Message, TEXT } from "../ui.js";
import { theme } from "herdr";
import { Day } from "./day.js";
import { Strip, AgentsStrip, HourLabels, DragArea } from "./strip.js";
import { StretchList } from "./stretches.js";
import { Side } from "./side.js";

export class SessionView extends Day {
  args() {
    return ["session"];
  }

  // Today's goals, for the line under each project: a session has none of its own.
  goalArgs() {
    return ["goals"];
  }

  // A session isn't a day: no today, and the divergences are the day view's.
  took(r) {
    this.data = r;
    this.lastSample = r.last_sample ?? 0;
  }

  range() {
    const s = this.data.session;
    return sessionWindow(s.start, s.end, Date.now(), s.open);
  }

  startSession(cx) {
    this.perform(cx, () => call(["session", "start"]), () => null);
  }

  endSession(cx) {
    this.perform(cx, () => call(["session", "end"]), () => null);
  }

  // No side column without an open session.
  render() {
    if (this.data && !this.data.session?.open) return h_flex().flex_1().min_h(0).items_stretch().child(this.left(theme()));
    return super.render();
  }

  left(t) {
    const d = this.data;
    const s = d?.session;
    if (!d || !s?.open) {
      const col = v_flex().id("session").flex_1().min_w(0).min_h(0).overflow_y_scroll().pt(18).px(22).pb(18).gap(14).children(this.notices(t));
      return col.child(d ? this.empty(t, s) : Message(this.loading ? "Загрузка…" : ""));
    }
    const { from, to } = this.range();
    const top = [
      ...this.notices(t),
      v_flex()
        .flex_none()
        .gap(4)
        .child(
          h_flex()
            .gap(10)
            .items_center()
            .child(div().text_size(15).font_semibold().child("Идёт сессия"))
            .child(div().text_color(t.dim).child(`с ${hhmm(s.start)} · ${formatDuration(Date.now() - s.start)}`))
            .child(div().flex_1())
            .child(Button({ id: "session-end", label: "Завершить", onClick: (cx) => this.endSession(cx) })),
        )
        .child(div().text_size(TEXT.small).text_color(t.dim).child(sessionNote(s.manual, d.session_idle_minutes))),
      v_flex()
        .flex_none()
        .gap(4)
        .child(DragArea(this).gap(4).child(Strip(this, t, { from, to, h: 26, radius: 5 })).child(AgentsStrip(this, t, { from, to })).child(HourLabels(t, from, to))),
    ];
    const rows = [StretchList(this, t, s.stretches, { row: 32, gap: 18 })];
    return this.column("session", top, rows, `${s.start}|${s.stretches.length}`);
  }

  empty(t, last) {
    return v_flex()
      .flex_1()
      .items_center()
      .justify_center()
      .gap(8)
      .child(div().text_color(t.dim).child(Icon("clock", 26)))
      .child(div().text_size(15).font_semibold().child("Сессии нет"))
      .when(Boolean(last), (el) => el.child(div().text_size(TEXT.small).text_color(t.dim).child(`последняя: ${hhmm(last.start)} – ${hhmm(last.end)} · ${formatDuration(last.me)}`)))
      .child(Button({ id: "session-start", label: "Начать сессию", icon: "play", primary: true, onClick: (cx) => this.startSession(cx) }))
      .child(div().text_size(TEXT.caption).text_color(t.dim).child("или начнётся сама при активности"));
  }

  sideView(t) {
    const { report } = this.data;
    const s = this.data.session;
    return Side(this, t, {
      caption: "Моё время в сессии",
      session: true,
      lines: [
        `агенты ${formatDuration(report.agents)} · перерывы ${formatDuration(s.breaks)}`,
        `${plural(s.switches, "переключение", "переключения", "переключений")} · фокус до ${formatDuration(s.focus)}`,
      ],
    });
  }
}
