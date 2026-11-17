// The «Цель» dialog: how much time a project wants per day, week or month, and from when. Drawn over the panel.
import { div } from "gpui-kit";
import { h_flex, v_flex, Input, InputState } from "gpui-base";
import { theme } from "herdr";
import { call } from "../client.js";
import { parseHours, hoursText, goalOn, goalVersion, dayTitle, dayString } from "../format.js";
import { projectColor } from "../lanes.js";
import { monthStartTitle } from "../rules.js";
import { Dialog, Dropdown, Check, Button, Icon, Message, TEXT } from "../ui.js";
import { Link } from "./menu.js";

const DIRS = [{ id: "min", label: "не меньше" }, { id: "max", label: "не больше" }];
const PERIODS = [{ id: "day", label: "день" }, { id: "week", label: "неделю" }, { id: "month", label: "месяц" }];
const WEEKDAYS = ["пн", "вт", "ср", "чт", "пт", "сб", "вс"]; // 1 = Monday … 7 = Sunday

export class GoalEdit {
  // Shows the dialog with showDialog(d | null, cx), which main.js gives the views.
  static open(showDialog, opts, cx) {
    showDialog(new GoalEdit({ ...opts, close: (cx) => showDialog(null, cx) }), cx);
  }

  // project: { id, name, goals } from `projects`; today: the collector's day; onSaved(text, cx) after a
  // save or a removal; close(cx) takes the dialog away.
  constructor({ project, today, onSaved, close }) {
    Object.assign(this, { project, today, onSaved, closeDialog: close });
    this.current = goalOn(project.goals ?? [], today);
    const g = this.current;
    this.dir = g?.dir ?? "min";
    this.period = g?.period ?? "day";
    this.weekdays = new Set(g?.weekdays ?? [1, 2, 3, 4, 5]);
    this.calendar = g ? Boolean(g.calendar) : true;
    this.weekends = g ? Boolean(g.weekends) : true;
    this.hours = InputState.new({ placeholder: "6,5", value: g ? hoursText(g.minutes) : "" });
    this.hours.on("submit", (_e, cx) => this.save(cx));
    const [y, m] = today.split("-").map(Number);
    const next = dayString(new Date(y, m, 1).getTime());
    this.froms = [
      { id: "month", day: `${today.slice(0, 8)}01`, label: `${monthStartTitle(today)} ${y}` },
      { id: "today", day: today, label: `сегодня, ${dayTitle(today).split(", ")[1]}` },
      { id: "next", day: next, label: `${monthStartTitle(next)} ${next.slice(0, 4)}` },
    ];
    this.from = this.froms[0].day;
    this.openList = null; // the open dropdown: "dir" | "period" | "from"
    this.showVersions = false;
    this.error = null;
    this.busy = false;
    this.closed = false; // close() ran: an answer still on its way must not close again
  }

  save(cx) {
    const minutes = parseHours(this.hours.value());
    if (minutes === null) {
      this.error = "Часы: число больше нуля, например 6,5";
      return cx.notify();
    }
    const weekdays = [...this.weekdays].sort((a, b) => a - b);
    this.finish(cx, { from: this.from, dir: this.dir, minutes, period: this.period, weekdays, calendar: this.calendar, weekends: this.weekends }, "Цель сохранена.");
  }

  remove(cx) {
    this.finish(cx, { from: this.from, off: true }, "Цель убрана.");
  }

  // Writes; a refusal stays open with its reason. Closed meanwhile, it only lets the panel read again.
  finish(cx, body, text) {
    if (this.busy || this.closed) return;
    this.busy = true;
    cx.spawn(async (cx) => {
      const r = await call(["goal", "set", this.project.id, JSON.stringify(body)]);
      this.busy = false;
      const ok = !r.down && !r.error;
      if (this.closed) {
        if (ok) this.onSaved(text, cx);
        return;
      }
      if (!ok) {
        this.error = r.down ? "Сборщик не запущен" : r.error;
        return cx.notify();
      }
      this.close(cx);
      this.onSaved(text, cx);
    });
  }

  close(cx) {
    if (this.closed) return;
    this.closed = true;
    this.hours.release();
    this.closeDialog(cx);
  }

  toggleDay(n, cx) {
    if (!this.weekdays.delete(n)) this.weekdays.add(n);
    cx.notify();
  }

  render() {
    const t = theme();
    const caption = (text) => div().flex_none().text_size(TEXT.small).text_color(t.dim).child(text);
    const dropdown = (id, label, items, pick) =>
      Dropdown({ id: `goal-${id}`, label, open: this.openList === id, items, onToggle: (open, cx) => ((this.openList = open ? id : null), cx.notify()), onPick: (v, cx) => (pick(v), (this.openList = null), cx.notify()) });
    const labelOf = (items, id) => items.find((it) => it.id === id)?.label ?? "";
    const from = this.froms.find((f) => f.day === this.from);
    const since = `с ${from?.label ?? this.from}`; // the buttons name the day they act from
    const versions = [...(this.project.goals ?? [])].sort((a, b) => b.from.localeCompare(a.from));
    const body = v_flex()
      .p(18)
      .gap(12)
      .child(caption("Цель"))
      .child(
        h_flex()
          .flex_none()
          .gap(8)
          .items_center()
          .child(dropdown("dir", labelOf(DIRS, this.dir), DIRS, (v) => (this.dir = v)))
          .child(Input.new(this.hours).id("goal-hours").flex_none().w(52).h(28).px(6).rounded(6).border_1().border_color(t.border).bg(t.background).text_size(TEXT.small).text_right())
          .child(div().flex_none().text_color(t.muted).child("ч в"))
          .child(dropdown("period", labelOf(PERIODS, this.period), PERIODS, (v) => (this.period = v))),
      )
      .when(this.period === "day", (el) =>
        el
          .child(
            h_flex()
              .flex_none()
              .gap(4)
              .role("group")
              .accessibility_label("Дни недели")
              .children(
                WEEKDAYS.map((label, i) => {
                  const on = this.weekdays.has(i + 1);
                  return div()
                    .id(`goal-weekday-${i + 1}`)
                    .flex_none()
                    .w(34)
                    .h(30)
                    .flex()
                    .items_center()
                    .justify_center()
                    .rounded(6)
                    .text_size(TEXT.small)
                    .role("check_box")
                    .aria_selected(on)
                    .cursor_pointer()
                    .when(on, (el) => el.bg(t.raised).border_1().border_color(t.border).text_color(t.foreground))
                    .when(!on, (el) => el.text_color(t.dim).hover((s) => s.bg(t.hover)))
                    .on_click((_e, cx) => this.toggleDay(i + 1, cx))
                    .child(label);
                }),
              ),
          )
          .child(Check({ id: "goal-calendar", checked: this.calendar, label: "производственный календарь РФ", onClick: (cx) => ((this.calendar = !this.calendar), cx.notify()) }))
          .child(Check({ id: "goal-weekends", checked: this.weekends, label: "время в выходные идёт в зачёт", onClick: (cx) => ((this.weekends = !this.weekends), cx.notify()) })),
      )
      .child(
        h_flex()
          .flex_none()
          .gap(8)
          .items_center()
          .child(caption("Действует с"))
          .child(
            dropdown(
              "from",
              h_flex().gap(6).items_center().child(div().text_color(t.dim).child(Icon("calendar", 12))).child(from?.label ?? this.from),
              this.froms.map((f) => ({ id: f.id, label: f.label })),
              (v) => (this.from = this.froms.find((f) => f.id === v).day),
            ),
          )
          .child(Link({ id: "goal-history", label: "история", onClick: (cx) => ((this.showVersions = !this.showVersions), cx.notify()) }).text_size(TEXT.small)),
      )
      .when(this.showVersions, (el) =>
        el.child(
          v_flex()
            .flex_none()
            .gap(4)
            .text_size(TEXT.small)
            .text_color(t.dim)
            .when(!versions.length, (el) => el.child("Версий пока нет."))
            .children(versions.map((g) => div().child(goalVersion(g)))),
        ),
      )
      .when(Boolean(this.error), (el) => el.child(Message(this.error, true)));
    return Dialog({
      id: "goal",
      width: 560, // room for «Убрать с сегодня, 30 сентября» and «Сохранить с …» side by side
      title: "Цель",
      sub: h_flex()
        .flex_none()
        .min_w(0)
        .gap(10)
        .items_center()
        .text_color(t.muted)
        .child(div().flex_none().size(8).rounded(4).bg(t[projectColor(this.project.id, [this.project])] ?? t.muted))
        .child(div().min_w(0).truncate().child(this.project.name || this.project.id)),
      onClose: (cx) => this.close(cx),
      body,
      footer: [
        this.current && Button({ id: "goal-remove", label: `Убрать ${since}`, ghost: true, onClick: (cx) => this.remove(cx) }).px(0),
        div().flex_1(),
        Button({ id: "goal-cancel", label: "Отмена", onClick: (cx) => this.close(cx) }).px(14).py(6).text_size(TEXT.body),
        Button({ id: "goal-save", label: `Сохранить ${since}`, primary: true, onClick: (cx) => this.save(cx) }).px(14).py(6).text_size(TEXT.body),
      ].filter(Boolean),
    });
  }
}
