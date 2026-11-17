// «Закрыть месяц»: what is left to do in a month that is over, then a close for every project. Drawn over the panel.
import { div } from "gpui-kit";
import { h_flex, v_flex } from "gpui-base";
import { theme } from "herdr";
import { call } from "../client.js";
import { fmtMin, midnight, monthTitle, offDaysText, payOn, catalog, MONTH_NAMES } from "../format.js";
import { Dialog, Button, Check, Icon, Message, TEXT } from "../ui.js";
import { Link } from "./menu.js";

const DOWN = "Сборщик не запущен";

export class CloseMonth {
  // Shows the dialog with showDialog(d | null, cx), which main.js gives the views, and reads the month.
  static open(showDialog, opts, cx) {
    const d = new CloseMonth({ ...opts, close: (cx) => showDialog(null, cx) });
    showDialog(d, cx);
    d.load(cx);
  }

  // month: "YYYY-MM"; name: the project Отчёт shows, for the check; open(tab, cx, arg) switches tabs; onDone(text, cx)
  // after closing; close(cx) takes the dialog away.
  constructor({ month, name, open, onDone, close }) {
    Object.assign(this, { month, name, open, onDone, closeDialog: close });
    this.state = null; // what `month state` gave
    this.projects = [];
    this.sent = false;
    this.error = null;
    this.busy = false;
    this.closed = false; // close() ran: an answer still on its way must not close again
  }

  load(cx) {
    cx.spawn(async (cx) => {
      const [st, projects] = await Promise.all([call(["month", "state", this.month]), call(["projects"])]);
      if (this.closed) return;
      const cat = catalog(projects);
      this.error = st.down || projects.down ? DOWN : (st.error ?? (cat ? null : (projects.error ?? "projects: нет ответа")));
      if (!this.error) Object.assign(this, { state: st, projects: cat.projects });
      cx.notify();
    });
  }

  submit(cx) {
    if (this.busy || this.closed) return;
    this.busy = true;
    this.error = null;
    cx.spawn(async (cx) => {
      const r = await call(["month", "close", this.month, ...(this.sent ? ["--sent"] : [])]);
      this.busy = false;
      const ok = !r.down && !r.error;
      const text = `${monthTitle(this.month)} закрыт.`;
      if (this.closed) {
        if (ok) this.onDone(text, cx);
        return;
      }
      if (!ok) {
        this.error = r.down ? DOWN : r.error;
        return cx.notify();
      }
      this.close(cx);
      this.onDone(text, cx);
    });
  }

  // Closes the dialog and shows another tab.
  go(tab, arg, cx) {
    this.close(cx);
    this.open(tab, cx, arg);
  }

  close(cx) {
    if (this.closed) return;
    this.closed = true;
    this.closeDialog(cx);
  }

  render() {
    const t = theme();
    const name = MONTH_NAMES[Number(this.month.slice(5, 7)) - 1]; // "ноябрь"
    const st = this.state;
    const body = v_flex().p(18).gap(12);
    if (st) {
      // ponytail: the mock's row border #25262b is `raised`; `chrome` is the dialog's own background
      const row = (icon, color, text, right) =>
        h_flex()
          .flex_none()
          .h(38)
          .gap(10)
          .items_center()
          .border_t(1)
          .border_color(t.raised)
          .child(div().flex_none().text_color(color).child(Icon(icon, 16)))
          .child(div().flex_1().min_w(0).truncate().child(text))
          .when(Boolean(right), (el) => el.child(right));
      const link = (id, label, tab, arg) => Link({ id: `close-month-${id}`, label, onClick: (cx) => this.go(tab, arg, cx) });
      const rows = [
        st.other > 0
          ? row("circle-alert", t.ansi3, `«Другое» — ${fmtMin(st.other)} не разобрано`, link("inbox", "разобрать", "inbox", st.other_day))
          : row("circle-check", t.ansi2, "«Другое» разобрано"),
      ];
      const pname = (id) => this.projects.find((p) => p.id === id)?.name || id;
      for (const id of st.no_bonus) rows.push(row("circle", t.dim, `Премии ${pname(id)} не внесены`, link(`bonus-${id}`, "добавить", "report", { project: id, month: this.month })));
      if (!st.no_bonus.length && this.projects.some((p) => payOn(p.pay, this.month))) rows.push(row("circle-check", t.ansi2, "Премии внесены"));
      const first = `${this.month}-01`;
      rows.push(row("circle-check", t.ansi2, "Отгулы и праздники", div().flex_none().text_size(TEXT.small).text_color(t.dim).child(offDaysText(st))));
      rows.push(row("circle-check", t.ansi2, "Ручные правки", link("history", `${st.edits} · история`, "history", { day: first, dayStartMs: st.from - midnight(first), period: "month" })));
      body
        .child(v_flex().gap(6).child(div().text_size(TEXT.small).text_color(t.dim).child("Перед закрытием")).child(v_flex().children(rows)))
        .child(div().text_size(TEXT.small).text_color(t.muted).whitespace_normal().child(`После закрытия правила, разбор «Другого» и правки задним числом ${name} не трогают. Открыть заново можно — это попадёт в историю.`))
        .child(Check({ id: "close-month-sent", checked: this.sent, label: `отчёт ${this.name} отправлен заказчику`, onClick: (cx) => ((this.sent = !this.sent), cx.notify()) }));
    } else if (!this.error) body.child(Message("Загрузка…"));
    if (this.error) body.child(Message(this.error, true));
    return Dialog({
      id: "close-month",
      width: 500,
      icon: "lock",
      title: `Закрыть ${name}`,
      sub: div().flex_none().text_color(t.dim).child("все проекты"),
      onClose: (cx) => this.close(cx),
      body,
      footer: [
        div().flex_1(),
        Button({ id: "close-month-cancel", label: "Отмена", onClick: (cx) => this.close(cx) }).px(14).py(6).text_size(TEXT.body),
        Button({ id: "close-month-submit", label: "Закрыть месяц", icon: "lock", primary: true, onClick: (cx) => st && this.submit(cx) }).px(14).py(6).text_size(TEXT.body),
      ],
    });
  }
}
