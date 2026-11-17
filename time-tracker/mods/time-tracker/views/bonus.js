// The «Премия» dialog: a sum paid on top of a project's pay for a month, on a day of it. Drawn over the panel.
import { div } from "gpui-kit";
import { h_flex, v_flex, Input, InputState } from "gpui-base";
import { theme } from "herdr";
import { call } from "../client.js";
import { money, parseMoney, MONTHS, MONTH_NAMES } from "../format.js";
import { Dialog, Button, Message, TEXT } from "../ui.js";

export class BonusAdd {
  // Shows the dialog with showDialog(d | null, cx), which main.js gives the views.
  static open(showDialog, opts, cx) {
    showDialog(new BonusAdd({ ...opts, close: (cx) => showDialog(null, cx) }), cx);
  }

  // project: its id; name: its name; month: "YYYY-MM"; pay: the month report's `pay` (null: none);
  // onSaved(text, cx) after adding; close(cx) takes the dialog away.
  constructor({ project, name, month, pay, onSaved, close }) {
    Object.assign(this, { project, name, month, pay, onSaved, closeDialog: close });
    const [y, m] = month.split("-").map(Number);
    this.days = new Date(y, m, 0).getDate();
    this.title = InputState.new({ placeholder: "Релиз в срок" });
    this.sum = InputState.new({ placeholder: "15 000" });
    this.day = InputState.new({ placeholder: "1" });
    for (const s of [this.title, this.sum, this.day]) {
      s.on("change", (_e, cx) => cx.notify());
      s.on("submit", (_e, cx) => this.add(cx));
    }
    this.error = null;
    this.busy = false;
    this.closed = false; // close() ran: an answer still on its way must not close again
  }

  add(cx) {
    if (this.busy || this.closed) return;
    const title = this.title.value().trim();
    const amount = parseMoney(this.sum.value());
    const day = /^\d{1,2}$/.test(this.day.value().trim()) ? Number(this.day.value().trim()) : 0;
    this.error = !title ? "За что: напишите, за что премия" : [...title].length > 200 ? "За что: не длиннее 200 символов" : amount === null ? "Сумма: целое число рублей" : day < 1 || day > this.days ? `День: число от 1 до ${this.days}` : null;
    if (this.error) return cx.notify();
    this.busy = true;
    cx.spawn(async (cx) => {
      const r = await call(["bonus", "add", JSON.stringify({ project: this.project, month: this.month, day, title, amount })]);
      this.busy = false;
      const ok = !r.down && !r.error;
      if (this.closed) {
        if (ok) this.onSaved("Премия добавлена.", cx);
        return;
      }
      if (!ok) {
        this.error = r.down ? "Сборщик не запущен" : r.error === "closed" ? "Месяц закрыт: ничего не записано." : r.error;
        return cx.notify();
      }
      this.close(cx);
      this.onSaved("Премия добавлена.", cx);
    });
  }

  close(cx) {
    if (this.closed) return;
    this.closed = true;
    for (const s of [this.title, this.sum, this.day]) s.release();
    this.closeDialog(cx);
  }

  render() {
    const t = theme();
    const i = Number(this.month.slice(5, 7)) - 1;
    const row = (label, ...controls) =>
      h_flex().flex_none().min_h(28).gap(8).items_center().child(div().flex_none().w(96).text_size(TEXT.small).text_color(t.dim).child(label)).children(controls);
    const input = (id, state) => Input.new(state).id(id).flex_none().h(28).px(6).rounded(6).border_1().border_color(t.border).bg(t.background).text_size(TEXT.small);
    const unit = (text) => div().flex_none().text_color(t.muted).child(text);
    const line = (left, right) => h_flex().gap(8).items_center().child(div().flex_1().min_w(0).child(left)).child(div().flex_none().child(right));
    const pay = this.pay;
    const amount = parseMoney(this.sum.value()) ?? 0;
    const base = pay?.kind === "fixed" ? `Фикс за ${MONTH_NAMES[i]}` : pay?.kind === "hourly" ? "Почасовая" : null;
    const preview = v_flex()
      .flex_none()
      .px(12)
      .py(10)
      .gap(6)
      .rounded(8)
      .bg(t.background)
      .text_size(TEXT.small)
      .when(Boolean(base), (el) => el.child(line(div().text_color(t.dim).child(base), money(pay.base))))
      .child(line(div().text_color(t.dim).child("Премии"), `+ ${money((pay?.bonus_sum ?? 0) + amount)}`))
      .child(line("Итого", money((pay?.total ?? 0) + amount)).font_semibold());
    const body = v_flex()
      .p(18)
      .gap(12)
      .child(row("За что", input("bonus-title", this.title).flex_1().min_w(0)))
      .child(row("Сумма", input("bonus-sum", this.sum).w(96).text_right(), unit("₽")))
      .child(row("День", input("bonus-day", this.day).w(44).text_center(), unit(MONTHS[i])))
      .child(preview)
      .when(Boolean(this.error), (el) => el.child(Message(this.error, true)));
    return Dialog({
      id: "bonus",
      width: 460,
      title: "Премия",
      sub: div().min_w(0).truncate().text_color(t.muted).child(`${this.name} · ${MONTH_NAMES[i]}`),
      onClose: (cx) => this.close(cx),
      body,
      footer: [
        div().flex_1(),
        Button({ id: "bonus-cancel", label: "Отмена", onClick: (cx) => this.close(cx) }).px(14).py(6).text_size(TEXT.body),
        Button({ id: "bonus-add", label: "Добавить", primary: true, onClick: (cx) => this.add(cx) }).px(14).py(6).text_size(TEXT.body),
      ],
    });
  }
}
