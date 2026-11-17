// The rule editor: what a rule matches, what it matched over the last 7 days, an earlier rule that keeps
// some of that time, and from when it applies. Drawn as a dialog over the panel.
import { div } from "gpui-kit";
import { h_flex, v_flex, Input, InputState } from "gpui-base";
import { theme } from "herdr";
import { call } from "../client.js";
import { formatDuration, plural, dayString } from "../format.js";
import { projectColor } from "../lanes.js";
import { RULE_KINDS, ruleText, targetName, monthStartTitle } from "../rules.js";
import { Dialog, Segmented, Check, Button, Icon, Message, TEXT } from "../ui.js";

const PREVIEW_MS = 400;
const NOTHING = { ms: 0, windows: 0, top: [], overlap: null };

// The value field of each kind: its label, its placeholder, and whether a «содержит» box goes before it.
const FIELDS = {
  folder: { label: "Папка", placeholder: "D:\\Work\\Universe\\**" },
  process: { label: "Процесс", placeholder: "idea64.exe" },
  title: { label: "Заголовок окна", contains: true },
  figma: { label: "Файл Figma", contains: true },
  minecraft: { label: "Сервер", placeholder: "localhost:25565" },
};

export class RuleEdit {
  // Shows an editor with showDialog(d | null, cx), which main.js gives the views, and previews at once.
  static open(showDialog, { rule, projects, onSaved }, cx) {
    const edit = new RuleEdit({ rule, projects, onSaved, close: (cx) => showDialog(null, cx) });
    showDialog(edit, cx);
    edit.refresh(cx);
  }

  // rule: { id?, kind, value, title?, project }, no id for a new one; onSaved(text, cx) after a save or
  // delete; close(cx) takes the dialog away.
  constructor({ rule, projects, onSaved, close }) {
    this.rule = rule;
    this.projects = projects;
    this.onSaved = onSaved;
    this.closeDialog = close;
    this.kind = rule.kind;
    this.value = this.input(FIELDS[rule.kind].placeholder, rule.value);
    this.title = this.input("необязательно", rule.title);
    this.scope = "month";
    this.before = 0; // the rule to go above, 0: where its kind goes
    this.preview = null; // the last answer of `rule preview`; null until the first
    this.error = null;
    this.seq = 0; // the latest change: older previews are dropped
    this.busy = false;
  }

  input(placeholder, value) {
    const state = InputState.new({ placeholder: placeholder ?? "", value: value ?? "" });
    state.on("change", (_e, cx) => this.changed(cx));
    return state;
  }

  // Previews once typing stops.
  changed(cx) {
    const n = ++this.seq;
    cx.timer.after(PREVIEW_MS, (cx) => n === this.seq && this.refresh(cx));
  }

  body() {
    return { kind: this.kind, value: this.value.value().trim(), title: this.kind === "process" ? this.title.value().trim() : "", project: this.rule.project };
  }

  refresh(cx) {
    const n = ++this.seq;
    const body = this.body();
    cx.notify();
    if (!body.value) {
      this.preview = NOTHING;
      return;
    }
    cx.spawn(async (cx) => {
      const r = await call(["rule", "preview", JSON.stringify({ id: this.rule.id ?? 0, ...body, before: this.before })]);
      if (n !== this.seq) return;
      this.preview = r.down || r.error ? null : r;
      this.error = r.down ? "Сборщик не запущен" : (r.error ?? null);
      cx.notify();
    });
  }

  // The placeholder belongs to the input, so another kind gets a new one with the text carried over.
  setKind(kind, cx) {
    if (kind === this.kind) return;
    const old = this.value;
    this.kind = kind;
    this.value = this.input(FIELDS[kind].placeholder, old.value());
    old.release();
    this.refresh(cx);
  }

  raise(cx) {
    this.before = this.preview.overlap.rule.id;
    this.refresh(cx);
  }

  save(cx) {
    const body = JSON.stringify({ ...this.body(), scope: this.scope, before: this.before });
    this.finish(cx, this.rule.id ? ["rule", "set", String(this.rule.id), body] : ["rule", "add", body], "Правило сохранено.");
  }

  remove(cx) {
    this.finish(cx, ["rule", "rm", String(this.rule.id)], "Правило удалено.");
  }

  // Writes; a refusal stays open with its reason.
  finish(cx, args, text) {
    if (this.busy) return;
    this.busy = true;
    cx.spawn(async (cx) => {
      const r = await call(args);
      this.busy = false;
      if (r.down || r.error) {
        this.error = r.down ? "Сборщик не запущен" : r.error;
        return cx.notify();
      }
      this.close(cx);
      this.onSaved(text, cx);
    });
  }

  close(cx) {
    this.seq++; // a preview still on its way lands nowhere
    this.value.release();
    this.title.release();
    this.closeDialog(cx);
  }

  render() {
    const t = theme();
    const { project, id } = this.rule;
    const dot = !["exclude", "leisure"].includes(project) && (t[projectColor(project, this.projects)] ?? t.muted);
    const body = v_flex()
      .p(18)
      .gap(14)
      .child(Segmented({ id: "rule-kind", options: RULE_KINDS, value: this.kind, onChange: (kind, cx) => this.setKind(kind, cx) }))
      .child(this.fields(t))
      .child(this.matches(t))
      .when(Boolean(this.preview?.overlap), (el) => el.child(this.overlap(t)))
      .child(
        h_flex()
          .flex_none()
          .gap(18)
          .items_center()
          .role("radio_group")
          .child(div().flex_none().mr(-4).text_size(TEXT.small).text_color(t.dim).child("Применить"))
          .child(Check({ id: "rule-scope-today", radio: true, checked: this.scope === "today", label: "с сегодня", onClick: (cx) => this.setScope("today", cx) }))
          .child(Check({ id: "rule-scope-month", radio: true, checked: this.scope === "month", label: `и к прошлым дням с ${monthStartTitle(dayString(Date.now()))}`, onClick: (cx) => this.setScope("month", cx) })),
      )
      .when(Boolean(this.error), (el) => el.child(Message(this.error, true)));
    return Dialog({
      id: "rule-edit",
      width: 620,
      title: "Правило",
      sub: h_flex()
        .flex_none()
        .min_w(0)
        .gap(10)
        .items_center()
        .text_color(t.muted)
        .when(Boolean(dot), (el) => el.child(div().flex_none().size(8).rounded(4).bg(dot)))
        .child(div().min_w(0).truncate().child(targetName(project, this.projects))),
      onClose: (cx) => this.close(cx),
      body,
      footer: [
        id && Button({ id: "rule-delete", label: "Удалить правило", ghost: true, onClick: (cx) => this.remove(cx) }).px(0),
        div().flex_1(),
        Button({ id: "rule-cancel", label: "Отмена", onClick: (cx) => this.close(cx) }).px(14).py(6).text_size(TEXT.body),
        Button({ id: "rule-save", label: "Сохранить", primary: true, onClick: (cx) => this.save(cx) }).px(14).py(6).text_size(TEXT.body),
      ].filter(Boolean),
    });
  }

  setScope(scope, cx) {
    this.scope = scope;
    cx.notify();
  }

  fields(t) {
    const label = (text) => div().w(110).flex_none().text_size(TEXT.small).text_color(t.dim).child(text);
    const box = () => div().flex_none().h(30).px(8).flex().items_center().rounded(6).border_1().border_color(t.border).bg(t.background).text_size(TEXT.small).text_color(t.dim).child("содержит");
    const input = (state, id) => Input.new(state).id(id).flex_1().min_w(0).h(30).px(8).rounded(6).border_1().border_color(t.border).bg(t.background).font_family("Consolas").text_size(12);
    const row = (text, contains, state, id) => h_flex().gap(10).items_center().child(label(text)).when(contains, (el) => el.child(box())).child(input(state, id));
    const f = FIELDS[this.kind];
    return v_flex()
      .flex_none()
      .gap(8)
      .child(row(f.label, Boolean(f.contains), this.value, "rule-value"))
      .when(this.kind === "process", (el) => el.child(row("Заголовок окна", true, this.title, "rule-title")));
  }

  matches(t) {
    const p = this.preview;
    const caption = (text) => div().text_size(TEXT.small).text_color(t.dim).child(text);
    const card = v_flex().flex_none().px(12).py(8).rounded(8).bg(t.background);
    const head = h_flex().h(26).gap(8).items_center();
    if (p && !p.ms) return card.child(head.child(caption("за 7 дней совпадений нет")));
    head.child(caption("Совпадения за 7 дней")).child(div().flex_1());
    if (!p) return card.child(head.child(caption("…")));
    return card
      .child(head.child(div().font_medium().child(formatDuration(p.ms))).child(caption(`· ${plural(p.windows, "окно", "окна", "окон")}`)))
      .children(
        p.top.map((w) =>
          h_flex()
            .h(28)
            .gap(12)
            .items_center()
            .border_t(1)
            .border_color(t.border)
            .child(div().flex_1().min_w(0).truncate().font_family("Consolas").text_size(12).text_color(t.muted).child(w.title || "—"))
            .child(div().flex_none().text_size(TEXT.small).child(formatDuration(w.ms))),
        ),
      );
  }

  // An earlier rule keeps some of this time: first match wins, so this one can go above it.
  overlap(t) {
    const o = this.preview.overlap;
    return h_flex()
      .flex_none()
      .px(12)
      .py(8)
      .gap(10)
      .items_center()
      .rounded(8)
      .border_1()
      .border_color(t.border)
      .child(div().flex_none().text_color(t.ansi3).accessibility_label("Пересечение").child(Icon("triangle-alert", 14)))
      .child(
        v_flex()
          .min_w(0)
          .gap(2)
          .child(h_flex().gap(4).child(`${formatDuration(o.ms)} уже у`).child(div().text_color(t.ansi3).child(targetName(o.project, this.projects))))
          .child(div().truncate().font_family("Consolas").text_size(12).text_color(t.dim).child(`${ruleText(o.rule, true)} — выше в списке`)),
      )
      .child(div().flex_1())
      .child(Button({ id: "rule-raise", label: "Поднять выше", onClick: (cx) => this.raise(cx) }));
  }
}
