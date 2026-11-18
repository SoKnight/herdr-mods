// The «Цель и оплата» dialog: how much time a project wants per day, week or month, and from when; and its pay
// from a month on. Drawn over the panel.
import { div } from "gpui-kit";
import { h_flex, v_flex, Input, InputState } from "gpui-base";
import { theme } from "herdr";
import { call } from "../client.js";
import { parseHours, parseDay, goalFromNote, goalNow, goalReplaceText, goalRemoveText, hoursText, goalOn, goalVersion, dayTitle, dayString, fmtMin, money, parseMoney, payOn, monthOf, dialogWrites, MONTH_NAMES } from "../format.js";
import { projectColor } from "../lanes.js";
import { monthStartTitle } from "../rules.js";
import { Dialog, Dropdown, Check, Button, Icon, Message, Segmented, TEXT } from "../ui.js";
import { Link } from "./menu.js";

const DIRS = [{ id: "min", label: "не меньше" }, { id: "max", label: "не больше" }];
const PERIODS = [{ id: "day", label: "день" }, { id: "week", label: "неделю" }, { id: "month", label: "месяц" }];
const WEEKDAYS = ["пн", "вт", "ср", "чт", "пт", "сб", "вс"]; // 1 = Monday … 7 = Sunday
const PAY_KINDS = [{ id: "fixed", label: "фикс в месяц" }, { id: "hourly", label: "почасовая" }, { id: "none", label: "нет" }];

export class GoalEdit {
  // Shows the dialog with showDialog(d | null, cx), which main.js gives the views.
  static open(showDialog, opts, cx) {
    showDialog(new GoalEdit({ ...opts, close: (cx) => showDialog(null, cx) }), cx);
  }

  // project: { id, name, goals, pay } from `projects`; today: the collector's day; month ("YYYY-MM"): the month
  // the pay applies from, today's when not given; plan, planDays: that month's plan minutes and planned days
  // from its report (null: not known); onSaved(text, cx) after a save or a removal; close(cx) takes the dialog away.
  constructor({ project, today, month, plan = null, planDays = 0, onSaved, close }) {
    Object.assign(this, { project, today, plan, planDays, onSaved, closeDialog: close });
    this.month = month ?? today.slice(0, 7);
    const pay = payOn(project.pay, this.month);
    this.payKind = pay?.kind ?? "none";
    this.sum = InputState.new({ placeholder: "100 000", value: pay ? money(pay.amount).slice(0, -2) : "" });
    this.sum.on("change", (_e, cx) => cx.notify());
    this.sum.on("submit", (_e, cx) => this.save(cx));
    this.current = goalOn(project.goals ?? [], today);
    const g = this.current;
    this.dir = g?.dir ?? "min";
    this.period = g?.period ?? "day";
    this.weekdays = new Set(g?.weekdays ?? [1, 2, 3, 4, 5]);
    this.calendar = g ? Boolean(g.calendar) : true;
    this.weekends = g ? Boolean(g.weekends) : true;
    this.hours = InputState.new({ placeholder: "часы", value: g ? hoursText(g.minutes) : "" });
    this.hours.on("submit", (_e, cx) => this.save(cx));
    const [y, m] = today.split("-").map(Number);
    const next = dayString(new Date(y, m, 1).getTime());
    this.froms = [
      { id: "month", day: `${today.slice(0, 8)}01`, label: `${monthStartTitle(today)} ${y}` },
      { id: "today", day: today, label: `сегодня, ${dayTitle(today).split(", ")[1]}` },
      { id: "next", day: next, label: `${monthStartTitle(next)} ${next.slice(0, 4)}` },
    ];
    // A goal in force: changes start where it does, so a save edits it rather than adding a version.
    if (g && !this.froms.some((f) => f.day === g.from)) this.froms.unshift({ id: "current", day: g.from, label: `${dayTitle(g.from).split(", ")[1]} ${g.from.slice(0, 4)}, как сейчас` });
    this.from = g?.from ?? this.froms[0].day;
    this.fromOther = false; // «другая дата…»: the day is typed into fromText
    this.fromText = InputState.new({ placeholder: "ДД.ММ" });
    this.fromText.on("submit", (_e, cx) => this.save(cx));
    this.openList = null; // the open dropdown: "dir" | "period" | "from"
    this.showVersions = false;
    this.error = null;
    this.confirm = null; // { text, label, run(cx) }: asked before replacing or removing versions
    this.busy = false;
    this.closed = false; // close() ran: an answer still on its way must not close again
    this.opened = this.controls(); // what the controls showed: a side left as it was is not sent
  }

  controls() {
    return {
      goal: [this.dir, this.hours.value().trim(), this.period, [...this.weekdays].sort((a, b) => a - b), this.calendar, this.weekends, this.fromOther ? `other:${this.fromText.value().trim()}` : this.from],
      pay: [this.payKind, this.sum.value().trim()],
    };
  }

  // The day the goal acts from: a listed one, or the typed one (null when it doesn't read as a day).
  fromDay() {
    return this.fromOther ? parseDay(this.fromText.value(), this.today) : this.from;
  }

  // Sends `goal set` when the goal's controls were changed and `pay set` when the pay's were.
  save(cx, sure = false) {
    this.confirm = null;
    const now = this.controls(), writes = dialogWrites(this.opened, now), steps = [];
    const from = this.fromDay();
    if (writes.goal && !from) {
      this.error = "Дата: ДД.ММ, например 16.11";
      return cx.notify();
    }
    if (writes.goal) {
      const minutes = parseHours(this.hours.value());
      if (minutes === null) {
        this.error = "Часы: число больше нуля, например 6,5";
        return cx.notify();
      }
      const ask = !sure && goalReplaceText(this.project.goals, from);
      if (ask) {
        this.confirm = { text: ask, label: "Сохранить", run: (cx) => this.save(cx, true) };
        return cx.notify();
      }
      const goal = { from, dir: this.dir, minutes, period: this.period, weekdays: [...this.weekdays].sort((a, b) => a - b), calendar: this.calendar, weekends: this.weekends };
      steps.push({ args: ["goal", "set", this.project.id, JSON.stringify(goal)], done: () => (this.opened.goal = now.goal) });
    }
    if (writes.pay) {
      const pay = { from: this.month, kind: this.payKind };
      if (pay.kind !== "none") {
        pay.amount = parseMoney(this.sum.value());
        if (pay.amount === null) {
          this.error = "Сумма: целое число рублей, например 100 000";
          return cx.notify();
        }
      }
      steps.push({ args: ["pay", "set", this.project.id, JSON.stringify(pay)], done: () => (this.opened.pay = now.pay) });
    }
    // No goal yet and no hours typed: say so rather than close as if the goal were saved.
    const noGoal = !this.current && !this.hours.value().trim();
    if (!steps.length && noGoal) {
      this.error = "Цель не задана: впишите часы, например 6,5";
      return cx.notify();
    }
    const text = steps.length === 2 ? "Цель и оплата сохранены." : steps[0]?.args[0] === "pay" ? (noGoal ? "Оплата сохранена. Цель не задана: впишите часы." : "Оплата сохранена.") : "Цель сохранена.";
    if (!steps.length) return this.close(cx);
    this.finish(cx, steps, text);
  }

  remove(cx, sure = false) {
    this.confirm = null;
    const from = this.fromDay();
    if (!from) {
      this.error = "Дата: ДД.ММ, например 16.11";
      return cx.notify();
    }
    if (!sure) {
      this.confirm = { text: goalRemoveText(this.project.goals, from), label: "Убрать", run: (cx) => this.remove(cx, true) };
      return cx.notify();
    }
    this.finish(cx, [{ args: ["goal", "set", this.project.id, JSON.stringify({ from, off: true })] }], "Цель убрана.");
  }

  // Writes the steps one after the other; a refusal stops there and stays open with its reason, the panel
  // reading again when the goal before it went through. Closed meanwhile, it only lets the panel read again.
  finish(cx, steps, text) {
    if (this.busy || this.closed) return;
    this.busy = true;
    cx.spawn(async (cx) => {
      let r = {}, wrote = 0;
      for (const step of steps) {
        r = await call(step.args);
        if (r.down || r.error) break;
        step.done?.();
        wrote++;
      }
      this.busy = false;
      const ok = !r.down && !r.error;
      if (this.closed) {
        if (wrote) this.onSaved(ok ? text : null, cx);
        return;
      }
      if (!ok) {
        const why = r.down ? "Сборщик не запущен" : r.error;
        this.error = wrote ? `Цель сохранена, оплата — нет: ${why}` : why; // only goal then pay is two steps
        if (wrote) this.onSaved(null, cx);
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
    this.sum.release();
    this.fromText.release();
    this.closeDialog(cx);
  }

  toggleDay(n, cx) {
    if (!this.weekdays.delete(n)) this.weekdays.add(n);
    this.confirm = null;
    cx.notify();
  }

  render() {
    const t = theme();
    const caption = (text) => div().flex_none().text_size(TEXT.small).text_color(t.dim).child(text);
    const dropdown = (id, label, items, pick) =>
      Dropdown({ id: `goal-${id}`, label, open: this.openList === id, items, onToggle: (open, cx) => ((this.openList = open ? id : null), cx.notify()), onPick: (v, cx) => (pick(v, cx), (this.openList = null), (this.confirm = null), cx.notify()) });
    const labelOf = (items, id) => items.find((it) => it.id === id)?.label ?? "";
    const from = this.froms.find((f) => f.day === this.from);
    const fromNote = this.fromDay() ? goalFromNote(this.project.goals, this.fromDay()) : null;
    // the buttons name the day they act from
    const day = this.fromDay();
    const since = day ? `с ${dayTitle(day).split(", ")[1]}` : "с даты";
    const versions = [...(this.project.goals ?? [])].sort((a, b) => b.from.localeCompare(a.from));
    const goal = v_flex()
      .flex_1()
      .min_w(0)
      .gap(12)
      .child(caption("Цель"))
      .child(div().flex_none().whitespace_normal().text_size(TEXT.small).child(goalNow(this.current)))
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
          .flex_wrap() // a half-width column: «история» may go under the date
          .gap(8)
          .items_center()
          .child(caption("Действует с"))
          .child(
            dropdown(
              "from",
              h_flex().gap(6).items_center().child(div().text_color(t.dim).child(Icon("calendar", 12))).child(this.fromOther ? "другая дата" : (from?.label ?? this.from)),
              [...this.froms.map((f) => ({ id: f.id, label: f.label })), { id: "other", label: "другая дата…" }],
              (v, cx) => {
                this.fromOther = v === "other";
                if (!this.fromOther) this.from = this.froms.find((f) => f.id === v).day;
                else cx.timer.after(0, () => this.fromOther && this.fromText.focus()); // once the input is drawn
              },
            ),
          )
          .when(this.fromOther, (el) =>
            el.child(Input.new(this.fromText).id("goal-from-day").flex_none().w(64).h(28).px(6).rounded(6).border_1().border_color(t.border).bg(t.background).text_size(TEXT.small)),
          )
          .child(Link({ id: "goal-history", label: "история", onClick: (cx) => ((this.showVersions = !this.showVersions), cx.notify()) }).text_size(TEXT.small)),
      )
      .when(Boolean(fromNote), (el) => el.child(div().flex_none().whitespace_normal().text_size(TEXT.small).text_color(t.ansi3).child(fromNote)))
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
      );
    const body = v_flex()
      .p(18)
      .gap(12)
      .child(h_flex().items_start().gap(28).child(goal).child(this.payColumn(t, caption)))
      .when(Boolean(this.error), (el) => el.child(Message(this.error, true)))
      .when(Boolean(this.confirm), (el) =>
        el.child(
          h_flex()
            .flex_none()
            .gap(12)
            .items_center()
            .p(8)
            .rounded(6)
            .bg(t.raised)
            .text_size(TEXT.small)
            .child(div().flex_1().min_w(0).whitespace_normal().child(this.confirm.text))
            .child(Link({ id: "goal-confirm-no", label: "Отмена", color: t.muted, onClick: (cx) => ((this.confirm = null), cx.notify()) }))
            .child(Link({ id: "goal-confirm-yes", label: this.confirm.label, onClick: (cx) => this.confirm.run(cx) })),
        ),
      );
    return Dialog({
      id: "goal",
      width: 640,
      title: "Цель и оплата",
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

  // Оплата from this.month: the kind, the sum and what it comes to by the month's plan.
  payColumn(t, caption) {
    const kind = this.payKind;
    const name = MONTH_NAMES[Number(this.month.slice(5, 7)) - 1]; // "ноябрь"
    const sum = parseMoney(this.sum.value());
    const line = (left, right) => h_flex().gap(8).items_center().child(div().flex_1().min_w(0).text_color(t.dim).child(left)).child(div().flex_none().child(right));
    const plan = this.plan > 0 ? this.plan : null;
    const card = v_flex()
      .flex_none()
      .px(12)
      .py(10)
      .gap(6)
      .rounded(8)
      .bg(t.background) // as RuleEdit's matches card: chrome would vanish on the dialog's chrome
      .text_size(TEXT.small)
      .child(line(`План на ${name}`, plan ? `${fmtMin(plan)} · ${this.planDays} дн.` : "—"))
      .when(Boolean(plan && sum && kind === "fixed"), (el) => el.child(line("Ставка по плану", `≈ ${money(Math.round((sum * 60) / plan))}/ч`)))
      .when(Boolean(plan && sum && kind === "hourly"), (el) => el.child(line(`За ${name} по плану`, `≈ ${money(Math.round((plan * sum) / 60))}`)));
    return v_flex()
      .flex_1()
      .min_w(0)
      .gap(12)
      .child(caption(`Оплата с ${monthOf(this.month)}`))
      .child(Segmented({ id: "goal-pay-kind", options: PAY_KINDS, value: kind, onChange: (v, cx) => ((this.payKind = v), cx.notify()) }))
      .when(kind !== "none", (el) =>
        el.child(
          h_flex()
            .flex_none()
            .gap(8)
            .items_center()
            .child(div().flex_none().text_color(t.muted).child("Сумма"))
            .child(Input.new(this.sum).id("goal-sum").flex_none().w(96).h(28).px(6).rounded(6).border_1().border_color(t.border).bg(t.background).text_size(TEXT.small).text_right())
            .child(div().flex_none().text_color(t.muted).child(kind === "fixed" ? "₽ / мес" : "₽ / ч")),
        ),
      )
      .child(card)
      .child(div().flex_none().text_size(TEXT.small).text_color(t.dim).whitespace_normal().child("Премии за сроки трекер сам не узнает — они вносятся вручную в отчёте за месяц."));
  }
}
