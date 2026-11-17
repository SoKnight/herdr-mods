// История правок: corrections and rule changes of a month or a day, each with Отменить.
import { div } from "gpui-kit";
import { h_flex, v_flex } from "gpui-base";
import { theme } from "herdr";
import { call } from "../client.js";
import { monthRange, midnight, dayTitle, activityLabel, catalog, stamp, historyEntries, ACTIVITIES } from "../format.js";
import { ruleText, targetName } from "../rules.js";
import { Button, Icon, Message, Segmented, TEXT } from "../ui.js";
import { MONTHS } from "./table.js";

const ICONS = { assign: "arrow-right", excl: "ban", activity: "tag", note: "sticky-note", manual: "plus", rule: "funnel" };

export class History {
  // open(tab, cx): shows another tab of the panel.
  constructor(open) {
    this.open = open;
    this.day = null; // "YYYY-MM-DD" from День
    this.dayStartMs = 0;
    this.period = "month"; // or "day"
    this.entries = null;
    this.catalog = { projects: [], activities: ACTIVITIES };
    this.seq = 0;
    this.error = null;
    this.message = null; // { text, error? }
    this.busy = false;
  }

  // Reads on show and after an undo only.
  tick() {}

  show({ day, dayStartMs }, cx) {
    Object.assign(this, { day, dayStartMs, entries: null, message: null, error: null });
    this.load(cx);
  }

  range() {
    if (this.period === "month") return monthRange(this.day, this.dayStartMs);
    return { from: midnight(this.day) + this.dayStartMs, to: midnight(this.day, 1) + this.dayStartMs };
  }

  load(cx) {
    const seq = ++this.seq;
    const { from, to } = this.range();
    cx.notify();
    cx.spawn(async (cx) => {
      const [r, projects] = await Promise.all([call(["history", "--from", String(from), "--to", String(to)]), call(["projects"])]);
      if (seq !== this.seq) return;
      this.catalog = catalog(projects) ?? this.catalog;
      this.error = r.down ? "Сборщик не запущен" : (r.error ?? null);
      if (!this.error) {
        const { projects: ps, activities } = this.catalog;
        this.entries = historyEntries(r, { name: (id) => targetName(id, ps), activity: (id) => activityLabel(id, activities), ruleText, dayStartMs: this.dayStartMs });
      }
      cx.notify();
    });
  }

  setPeriod(period, cx) {
    if (period === this.period) return;
    Object.assign(this, { period, entries: null, message: null });
    this.load(cx);
  }

  undo(entry, cx) {
    if (this.busy) return;
    this.busy = true;
    this.message = null;
    cx.spawn(async (cx) => {
      try {
        const r = await call(entry.undo);
        if (r.down || r.error) {
          this.message = { text: r.down ? "Сборщик не запущен" : r.error === "closed" ? "Период закрыт выгрузкой: ничего не записано." : r.error, error: true };
        } else {
          this.message = { text: "Отменено." };
          this.load(cx);
        }
      } finally {
        this.busy = false;
        cx.notify();
      }
    });
    cx.notify();
  }

  render() {
    const t = theme();
    const col = v_flex().id("history").flex_1().min_w(0).min_h(0).overflow_y_scroll().pt(18).px(22).gap(14);
    const n = this.entries?.length;
    const when = this.day && (this.period === "month" ? MONTHS[Number(this.day.slice(5, 7)) - 1] : dayTitle(this.day).split(", ")[1]);
    col.child(
      h_flex()
        .flex_none()
        .gap(10)
        .items_center()
        .child(
          div()
            .id("history-back")
            .flex_none()
            .px(4)
            .text_size(16)
            .text_color(t.muted)
            .accessibility_label("Назад ко дню")
            .cursor_pointer()
            .hover((s) => s.text_color(t.foreground))
            .on_click((_e, cx) => this.open("day", cx))
            .child("‹"),
        )
        .child(div().flex_none().text_size(15).font_semibold().child("История правок"))
        .when(Boolean(when), (el) => el.child(div().text_color(t.dim).child(n === undefined ? when : `${when} · ${n}`)))
        .child(div().flex_1())
        .child(Segmented({ id: "history-period", options: [{ id: "day", label: "день" }, { id: "month", label: "месяц" }], value: this.period, onChange: (id, cx) => this.setPeriod(id, cx) }).w(140)),
    );
    if (this.error) col.child(Message(this.error, true));
    if (this.message) col.child(div().flex_none().text_size(TEXT.small).whitespace_normal().text_color(this.message.error ? t.ansi1 : t.muted).child(this.message.text));
    if (!this.entries) return this.error ? col : col.child(Message("Загрузка…"));
    if (!this.entries.length) return col.child(div().flex_none().text_size(TEXT.small).text_color(t.dim).child("Правок за этот период нет."));
    return col.child(v_flex().flex_none().gap(8).children(this.entries.map((e) => this.row(t, e))));
  }

  row(t, e) {
    const id = e.key.replace(":", "-");
    return h_flex()
      .id(`history-${id}`)
      .flex_none()
      .px(12)
      .py(10)
      .gap(12)
      .items_center()
      .rounded(8)
      .bg(t.chrome)
      .child(div().flex_none().size(30).flex().items_center().justify_center().rounded(6).bg(t.raised).child(Icon(ICONS[e.kind], 15)))
      .child(
        v_flex()
          .flex_1()
          .min_w(0)
          .gap(2)
          .child(div().truncate().child(e.title))
          .child(div().truncate().text_size(TEXT.small).text_color(t.dim).child(e.sub)),
      )
      .child(div().flex_none().text_size(TEXT.small).text_color(t.dim).child(stamp(e.at)))
      .child(Button({ id: `history-undo-${id}`, label: "Отменить", icon: "undo-2", onClick: (cx) => this.undo(e, cx) }));
  }
}
