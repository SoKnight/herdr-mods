// The «Время вручную» dialog: time added by hand on the day open on День. Drawn over the panel.
import { div } from "gpui-kit";
import { h_flex, v_flex, Input, InputState, Textarea, TextareaState } from "gpui-base";
import { theme } from "herdr";
import { formatDuration, dayTitle, parseClock, manualRange, overlapMs } from "../format.js";
import { projectColor } from "../lanes.js";
import { Dialog, Dropdown, Button, Icon, TEXT } from "../ui.js";

export class ManualEntry {
  // day: "YYYY-MM-DD" starting dayStartMs after midnight; me: my recorded time that day;
  // onAdd(body, cx) with the `correct add` body; close(cx) takes the dialog away.
  constructor({ day, dayStartMs, projects, activities, me, onAdd, close }) {
    Object.assign(this, { day, dayStartMs, activities, me, onAdd, onClose: close });
    this.projects = projects.filter((p) => !p.archived);
    this.allProjects = projects; // colours are given by the whole list
    this.project = this.projects[0]?.id ?? "";
    this.activity = "";
    this.menu = null; // the open dropdown: "project" | "activity"
    this.from = InputState.new({ placeholder: "14:00" });
    this.to = InputState.new({ placeholder: "15:30" });
    this.note = TextareaState.new({ placeholder: "Заметка", rows: 2 });
    for (const s of [this.from, this.to]) s.on("change", (_e, cx) => cx.notify());
    this.closed = false; // close() ran: nothing may close again
  }

  // The range to add (its end cut to the current minute), and what the status line says.
  state(now = Date.now()) {
    const a = parseClock(this.from.value()), b = parseClock(this.to.value());
    const r = a !== null && b !== null ? manualRange(this.day, this.dayStartMs, a, b) : null;
    const minute = now - (now % 60_000);
    const duration = formatDuration(r ? Math.min(r.end, minute) - r.start : 0);
    if (!r) return { duration, error: "время: ЧЧ:ММ" };
    if (r.start >= minute) return { duration, error: "это время ещё не наступило" };
    const range = { start: r.start, end: Math.min(r.end, minute) };
    const over = overlapMs(this.me, range.start, range.end);
    // ponytail: under a minute of overlap isn't mentioned, as formatDuration would show it as «—».
    return { duration, range, warn: over >= 60_000 ? `пересекается с записанным: ${formatDuration(over)} — вручную заменит его` : null };
  }

  add(cx) {
    if (this.closed) return; // its inputs are released
    const { range } = this.state();
    if (!range) return;
    const body = { kind: "add", ...range, project: this.project, activity: this.activity, note: this.note.value().trim() };
    this.close(cx);
    this.onAdd(body, cx);
  }

  close(cx) {
    if (this.closed) return;
    this.closed = true;
    for (const s of [this.from, this.to, this.note]) s.release();
    this.onClose(cx);
  }

  render() {
    const t = theme();
    const s = this.state();
    const row = (label, ...controls) =>
      h_flex().flex_none().min_h(28).items_center().child(div().flex_none().w(96).text_size(TEXT.small).text_color(t.dim).child(label)).children(controls);
    const box = (el) => el.rounded(6).border_1().border_color(t.border).bg(t.background);
    const clock = (id, state) => box(Input.new(state).id(id).flex_none().w(64).h(28).px(6)).text_size(TEXT.small).text_center();
    const color = (id) => t[projectColor(id, this.allProjects)] ?? t.muted;
    const dropdown = (id, label, items, pick) =>
      Dropdown({ id: `manual-${id}`, label, open: this.menu === id, items, onToggle: (open, cx) => ((this.menu = open ? id : null), cx.notify()), onPick: (v, cx) => (pick(v), (this.menu = null), cx.notify()) });
    const current = this.projects.find((p) => p.id === this.project);
    const activity = this.activities.find((a) => a.id === this.activity);
    const [icon, tone, text] = s.error ? ["triangle-alert", t.ansi3, s.error] : s.warn ? ["triangle-alert", t.ansi3, s.warn] : ["circle-check", t.ansi2, "не пересекается с записанным временем"];
    const body = v_flex()
      .p(18)
      .gap(12)
      .child(row("Дата", box(h_flex().flex_none().px(10).py(4).gap(6).items_center().text_size(TEXT.small)).child(div().text_color(t.dim).child(Icon("calendar", 12))).child(dayTitle(this.day))))
      .child(
        row(
          "Время",
          h_flex()
            .flex_none()
            .gap(8)
            .items_center()
            .child(clock("manual-from", this.from))
            .child(div().text_color(t.dim).child("–"))
            .child(clock("manual-to", this.to))
            .child(div().ml(4).font_medium().child(s.duration)),
        ),
      )
      .child(
        row(
          "Проект",
          dropdown(
            "project",
            h_flex().gap(8).items_center().when(Boolean(current), (el) => el.child(div().flex_none().size(8).rounded(4).bg(color(current.id)))).child(current?.name || current?.id || "Другое"),
            this.projects.map((p) => ({ id: p.id, label: p.name || p.id, color: color(p.id) })),
            (v) => (this.project = v),
          ),
        ),
      )
      .child(row("Вид работы", dropdown("activity", activity?.label ?? "Без вида", [{ id: "", label: "Без вида" }, ...this.activities], (v) => (this.activity = v))))
      .child(row("Заметка", box(Textarea.new(this.note).id("manual-note").flex_1().min_w(0).px(8).py(4)).text_size(TEXT.small)))
      .child(h_flex().flex_none().gap(8).items_center().text_size(TEXT.small).text_color(t.dim).child(div().text_color(tone).child(Icon(icon, 14))).child(div().min_w(0).whitespace_normal().child(text)));
    return Dialog({
      id: "manual",
      width: 460,
      title: "Время вручную",
      onClose: (cx) => this.close(cx),
      body,
      footer: [
        div().flex_1(),
        Button({ id: "manual-cancel", label: "Отмена", onClick: (cx) => this.close(cx) }).px(14).py(6).text_size(TEXT.body),
        Button({ id: "manual-add", label: "Добавить", primary: true, onClick: (cx) => this.add(cx) }).px(14).py(6).text_size(TEXT.body),
      ],
    });
  }
}
