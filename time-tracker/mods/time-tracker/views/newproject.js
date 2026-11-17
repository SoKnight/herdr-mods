// The new-project dialog: a name, a colour and the rules Другое suggests for it. Drawn over the panel.
import { div } from "gpui-kit";
import { h_flex, v_flex, Input, InputState } from "gpui-base";
import { theme } from "herdr";
import { call } from "../client.js";
import { formatDuration } from "../format.js";
import { RULE_KINDS, ruleText, freeColor, guessName } from "../rules.js";
import { Dialog, Check, Button, Icon, Message, TEXT } from "../ui.js";
import { Link } from "./menu.js";
import { Swatches } from "./projects.js";
import { RuleEdit } from "./ruleedit.js";

const HINTS_MS = 600;
const key = (h) => `${h.kind}\n${h.value}\n${h.title ?? ""}`;

export class NewProject {
  // Shows the dialog with showDialog(d | null, cx), which main.js gives the views.
  static open(showDialog, opts, cx) {
    const d = new NewProject({ ...opts, showDialog });
    showDialog(d, cx);
    d.fetch(cx);
  }

  // projects: the existing ones; hint: { kind, value, ms } to start from; onCreated(id, cx) after a create.
  constructor({ projects, hint, onCreated, onSaved, showDialog }) {
    this.projects = projects;
    this.onCreated = onCreated;
    this.onSaved = onSaved; // (text, cx) after «+ своё правило» is saved
    this.id = null; // the project once made: a retry only sends the rules left
    this.added = new Set(); // keys of the rules already sent
    this.showDialog = showDialog;
    this.name = InputState.new({ placeholder: "Название", value: hint ? guessName(hint) : "" });
    this.name.on("change", (_e, cx) => this.changed(cx));
    this.name.on("submit", (_e, cx) => this.create(cx));
    this.color = freeColor(projects);
    this.hints = hint ? [hint] : [];
    this.checked = new Set(hint ? [key(hint)] : []);
    this.error = null;
    this.seq = 0; // the latest change: older answers are dropped
    this.busy = false;
  }

  changed(cx) {
    const n = ++this.seq;
    cx.timer.after(HINTS_MS, (cx) => n === this.seq && this.fetch(cx));
    cx.notify();
  }

  fetch(cx) {
    const n = ++this.seq;
    const name = this.name.value().trim();
    if (name.length < 2) return;
    cx.spawn(async (cx) => {
      const r = await call(["hints", "--name", name]);
      if (n !== this.seq || !Array.isArray(r)) return;
      const have = new Set(this.hints.map(key));
      for (const h of r) {
        if (have.has(key(h))) continue;
        this.hints.push(h);
        if (h.kind !== "title") this.checked.add(key(h));
      }
      cx.notify();
    });
  }

  toggle(h, cx) {
    const k = key(h);
    if (!this.checked.delete(k)) this.checked.add(k);
    cx.notify();
  }

  fail(r, cx) {
    this.error = r.down ? "Сборщик не запущен" : r.error;
    this.busy = false;
    cx.notify();
  }

  // Makes the project and the checked rules; then(id, cx) runs after onCreated.
  create(cx, then) {
    if (this.busy) return;
    const name = this.name.value().trim();
    if (!name) {
      this.error = "Нужно название";
      return cx.notify();
    }
    this.busy = true;
    this.error = null;
    cx.spawn(async (cx) => {
      if (!this.id) {
        const p = await call(["project", "add", JSON.stringify({ name, color: this.color })]);
        if (p.down || p.error) return this.fail(p, cx);
        this.id = p.id;
        this.onCreated(p.id, cx); // it exists now, even if a rule fails below
      }
      for (const h of this.hints.filter((h) => this.checked.has(key(h)) && !this.added.has(key(h)))) {
        const rule = { kind: h.kind, value: h.value, project: this.id, scope: "month" };
        if (h.title) rule.title = h.title;
        const r = await call(["rule", "add", JSON.stringify(rule)]);
        if (r.down || r.error) return this.fail(r, cx);
        this.added.add(key(h));
      }
      this.busy = false;
      this.close(cx);
      then?.(this.id, cx);
    });
  }

  close(cx) {
    this.seq++;
    this.name.release();
    this.showDialog(null, cx);
  }

  own(cx) {
    this.create(cx, (id, cx) => RuleEdit.open(this.showDialog, { rule: { kind: "folder", value: "", project: id }, projects: this.projects, onSaved: this.onSaved }, cx));
  }

  render() {
    const t = theme();
    const body = v_flex()
      .p(18)
      .gap(14)
      .child(
        h_flex()
          .flex_none()
          .gap(10)
          .items_center()
          .child(div().flex_none().size(12).rounded(6).bg(t[this.color]))
          .child(Input.new(this.name).id("new-project-name").flex_1().min_w(0).h(32).px(8).rounded(6).border_1().border_color(t.border).bg(t.background).text_size(15).font_semibold()),
      )
      .child(
        h_flex()
          .flex_none()
          .gap(10)
          .items_center()
          .child(div().text_size(TEXT.small).text_color(t.dim).child("Цвет"))
          .child(Swatches({ id: "new-project-color", size: 18, value: this.color, projects: this.projects, onPick: (c, cx) => ((this.color = c), cx.notify()) }))
          .child(div().text_size(TEXT.caption).text_color(t.dim).child("тусклые заняты")),
      )
      .when(this.hints.length > 0, (el) => el.child(this.hintList(t)))
      .child(Link({ id: "new-project-rule", label: "+ своё правило", onClick: (cx) => this.own(cx) }).text_size(TEXT.small))
      .when(Boolean(this.error), (el) => el.child(Message(this.error, true)));
    return Dialog({
      id: "new-project",
      width: 520,
      title: "Новый проект",
      onClose: (cx) => this.close(cx),
      body,
      footer: [
        div().flex_1(),
        Button({ id: "new-project-cancel", label: "Отмена", onClick: (cx) => this.close(cx) }).px(14).py(6).text_size(TEXT.body),
        Button({ id: "new-project-create", label: "Создать", primary: true, onClick: (cx) => this.create(cx) }).px(14).py(6).text_size(TEXT.body),
      ],
    });
  }

  hintList(t) {
    return v_flex()
      .flex_none()
      .gap(4)
      .child(
        h_flex()
          .flex_none()
          .gap(8)
          .items_center()
          .child(div().flex_none().text_color(t.muted).child(Icon("sparkles", 14)))
          .child(div().text_size(TEXT.small).text_color(t.muted).child("Похоже на этот проект"))
          .child(div().flex_1())
          .child(div().text_size(TEXT.small).text_color(t.dim).child("за 30 дней")),
      )
      .children(
        this.hints.map((h, i) => {
          const on = this.checked.has(key(h));
          const icon = RULE_KINDS.find((k) => k.id === h.kind)?.icon ?? "type";
          return h_flex()
            .flex_none()
            .h(30)
            .gap(10)
            .items_center()
            .child(Check({ id: `new-project-hint-${i}`, checked: on, label: "", onClick: (cx) => this.toggle(h, cx) }))
            .child(div().flex_none().text_color(t.dim).child(Icon(icon, 14)))
            .child(div().flex_1().min_w(0).truncate().font_family("Consolas").text_size(12).text_color(on ? t.foreground : t.muted).child(ruleText(h)))
            .child(div().flex_none().text_size(TEXT.small).text_color(t.dim).child(formatDuration(h.ms)));
        }),
      );
  }
}
