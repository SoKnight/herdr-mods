// The Проекты screen: the projects, Досуг, Не работа and Другое on the left; on the right the chosen
// one's name, colour and rules.
import { div } from "gpui-kit";
import { h_flex, v_flex, Input, InputState, Popover } from "gpui-base";
import { theme, props, layout } from "herdr";
import { call } from "../client.js";
import { formatDuration } from "../format.js";
import { projectColor } from "../lanes.js";
import { RULE_KINDS, COLORS, ruleText, targetName } from "../rules.js";
import { Button, Icon, Message, TEXT } from "../ui.js";
import { Item, MenuPanel } from "./menu.js";
import { panelShown } from "./day.js";
import { RuleEdit, confirmDelete } from "./ruleedit.js";
import { NewProject } from "./newproject.js";

const NONE = "#00000000";

// Six colour circles; one another project in use has is dimmed and says whose it is.
// self: the project picking, whose own colour isn't taken.
export function Swatches({ id, size, value, projects, self, onPick }) {
  const t = theme();
  return h_flex()
    .id(id)
    .flex_none()
    .gap(6)
    .items_center()
    .role("group")
    .accessibility_label("Цвет проекта")
    .children(
      COLORS.map((c) => {
        const owner = projects.find((p) => p.id !== self && !p.archived && projectColor(p.id, projects) === c);
        return div()
          .id(`${id}-${c}`)
          .relative()
          .flex_none()
          .size(size)
          .rounded_full()
          .bg(t[c])
          .accessibility_label(`Цвет ${c}`)
          .cursor_pointer()
          .on_click((_e, cx) => onPick(c, cx))
          .when(Boolean(owner), (el) => el.tooltip(`${c}, у ${owner.name || owner.id}`))
          .when(Boolean(owner) && c !== value, (el) => el.opacity(0.45))
          .when(c === value, (el) => el.child(div().absolute().top(-3).left(-3).size(size + 6).rounded_full().border_1().border_color(t.foreground)));
      }),
    );
}

export class Projects {
  // open(tab, cx): shows another tab of the panel; showDialog(d | null, cx): a dialog over it.
  constructor(open, showDialog) {
    this.open = open;
    this.showDialog = showDialog;
    this.projects = []; // [{id, name, color, archived?}] in config order
    this.rules = []; // [{id, kind, value, title?, project, from?, ms, windows}] in priority order
    this.suggestions = []; // [{kind, value, ms}] Другое of the last 3 days, read while there are no projects
    this.other = 0; // today's Другое, ms
    this.selected = null; // a project id, "exclude" or "leisure"
    this.menu = null; // the rule whose menu is open
    this.error = null;
    this.message = null; // { text, error? }
    this.loading = false;
    this.loaded = false; // read once: the empty state waits for it
    this.wasShown = false;
    this.seq = 0;
    this.nameInput = InputState.new({ placeholder: "Название проекта" });
    this.nameInput.on("submit", (_e, cx) => this.rename(cx));
    this.nameInput.on("blur", (_e, cx) => this.rename(cx));
  }

  // Read again on showing; a hidden panel reads nothing.
  tick(cx) {
    const shown = Boolean(props()?.overlay) || panelShown(layout());
    if (shown && !this.wasShown) this.load(cx);
    this.wasShown = shown;
  }

  load(cx) {
    const seq = ++this.seq;
    this.loading = true;
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        const answers = await Promise.all([call(["projects"]), call(["rules"]), call(["timeline"])]);
        if (seq !== this.seq) return;
        const [p, r, day] = answers;
        const bad = answers.find((a) => a.down || a.error);
        this.error = bad ? (bad.down ? "Сборщик не запущен" : bad.error) : null;
        if (Array.isArray(p.projects)) this.projects = p.projects;
        if (Array.isArray(r.rules)) this.rules = r.rules;
        if (Array.isArray(day.report?.projects)) this.other = day.report.projects.find((x) => !x.project)?.me ?? 0;
        const known = ["exclude", "leisure"].includes(this.selected) || this.projects.some((x) => x.id === this.selected);
        if (!known) this.selected = this.list()[0]?.id ?? "leisure";
        this.syncName();
        if (!this.projects.length) {
          const h = await call(["hints", "--days", "3"]);
          if (seq === this.seq && Array.isArray(h)) this.suggestions = h;
        }
      } finally {
        if (seq === this.seq) (this.loading = false), (this.loaded = true);
        cx.notify();
      }
    });
  }

  // Archived projects last.
  list() {
    return [...this.projects.filter((p) => !p.archived), ...this.projects.filter((p) => p.archived)];
  }

  project() {
    return this.projects.find((p) => p.id === this.selected);
  }

  select(id, cx) {
    this.selected = id;
    this.menu = null;
    this.syncName();
    cx.notify();
  }

  syncName() {
    const p = this.project();
    if (p) this.nameInput.set_value(p.name || p.id);
  }

  // Writes, then reads it all again; the change shows at once and a failure reads it back.
  change(args, cx, done = null) {
    this.menu = null;
    cx.spawn(async (cx) => {
      const r = await call(args);
      this.message = r.down || r.error ? { text: r.down ? "Сборщик не запущен" : r.error, error: true } : done && { text: done };
      this.load(cx);
    });
    cx.notify();
  }

  setProject(p, fields, cx) {
    Object.assign(p, fields);
    this.change(["project", "set", p.id, JSON.stringify(fields)], cx);
  }

  rename(cx) {
    const p = this.project();
    const name = this.nameInput.value().trim();
    if (!p || name === (p.name || p.id)) return;
    if (!name) return this.syncName();
    this.setProject(p, { name }, cx);
  }

  setColor(color, cx) {
    const p = this.project();
    if (p && p.color !== color) this.setProject(p, { color }, cx);
  }

  toggleArchive(cx) {
    const p = this.project();
    if (p) this.setProject(p, { archived: !p.archived }, cx);
  }

  // rule: one of this.rules, or a new one's { kind, value, project }.
  editRule(rule, cx) {
    this.menu = null;
    const onSaved = (text, cx) => {
      this.message = { text };
      this.load(cx);
    };
    RuleEdit.open(this.showDialog, { rule, projects: this.projects, onSaved }, cx);
  }

  newProject(cx, hint) {
    this.menu = null;
    const onCreated = (id, cx) => {
      this.selected = id;
      this.load(cx);
    };
    NewProject.open(this.showDialog, { projects: this.projects, hint, onCreated, onSaved: (_text, cx) => this.load(cx) }, cx);
  }

  deleteRule(rule, cx) {
    this.menu = null;
    confirmDelete(rule, this.showDialog, (cx) => {
      this.rules = this.rules.filter((r) => r.id !== rule.id);
      this.change(["rule", "rm", String(rule.id)], cx, "Правило удалено.");
    }, cx);
  }

  render() {
    const t = theme();
    return h_flex().flex_1().min_h(0).items_stretch().child(this.sidebar(t)).child(this.detail(t));
  }

  sidebar(t) {
    const count = (target) => div().flex_none().text_size(TEXT.small).text_color(t.dim).child(String(this.rules.filter((r) => r.project === target).length));
    const icon = (name) => div().flex_none().text_color(t.muted).child(Icon(name, 14));
    const other = this.other >= 60_000 && div().flex_none().px(6).py(1).rounded(4).bg(t.muted).text_size(TEXT.caption).text_color(t.background).child(formatDuration(this.other));
    return v_flex()
      .id("projects-side")
      .w(260)
      .flex_none()
      .overflow_y_scroll()
      .bg(t.chrome)
      .border_r(1)
      .border_color(t.border)
      .pt(14)
      .px(10)
      .gap(10)
      .child(h_flex().flex_none().px(6).items_center().child(div().text_size(15).font_semibold().child("Проекты")).child(div().flex_1()).child(
        div()
          .id("projects-new")
          .flex_none()
          .size(28)
          .flex()
          .items_center()
          .justify_center()
          .rounded(6)
          .text_color(t.muted)
          .accessibility_label("Новый проект")
          .tooltip("Новый проект")
          .cursor_pointer()
          .hover((s) => s.bg(t.hover).text_color(t.foreground))
          .on_click((_e, cx) => this.newProject(cx))
          .child(Icon("plus", 15)),
      ))
      .when(this.projects.length > 0, (el) => el.child(
        v_flex()
          .flex_none()
          .gap(2)
          .children(
            this.list().map((p) =>
              this.row(t, {
                id: p.id,
                lead: div().flex_none().size(8).rounded(4).bg(t[projectColor(p.id, this.projects)] ?? t.muted).when(Boolean(p.archived), (el) => el.opacity(0.45)),
                label: div().min_w(0).truncate().font_medium().child(p.name || p.id),
                dim: p.archived,
              }),
            ),
          ),
      ))
      .child(div().flex_none().h(1).bg(t.border))
      .child(
        v_flex()
          .flex_none()
          .gap(2)
          .child(this.row(t, { id: "leisure", lead: icon("coffee"), label: "Досуг", end: count("leisure") }))
          .child(this.row(t, { id: "exclude", lead: icon("ban"), label: "Не работа", end: count("exclude") }))
          .child(this.row(t, { id: "other", lead: icon("inbox"), label: "Другое", end: other, onClick: (cx) => this.open("inbox", cx, null) })),
      );
  }

  row(t, { id, lead, label, end, dim, onClick }) {
    const on = this.selected === id;
    return h_flex()
      .id(`projects-row-${id}`)
      .flex_none()
      .p(8)
      .gap(10)
      .items_center()
      .rounded(6)
      .text_color(dim ? t.dim : t.foreground)
      .cursor_pointer()
      .when(on, (el) => el.bg(t.raised))
      .when(!on, (el) => el.hover((s) => s.bg(t.hover)))
      .on_click((_e, cx) => (onClick ? onClick(cx) : this.select(id, cx)))
      .child(lead)
      .child(label)
      .child(div().flex_1())
      .when(Boolean(end), (el) => el.child(end));
  }

  detail(t) {
    const p = this.project();
    const col = v_flex().id("projects-detail").flex_1().min_w(0).overflow_y_scroll().pt(14).px(22).gap(18);
    if (this.error) col.child(Message(this.error, true));
    if (this.message) col.child(div().flex_none().text_size(TEXT.small).whitespace_normal().text_color(this.message.error ? t.ansi1 : t.muted).child(this.message.text));
    if (!this.projects.length && this.loaded && !this.loading && !this.error) return col.child(this.empty(t));
    if (!this.selected) return col.child(Message(this.loading ? "Загрузка…" : ""));
    col.child(p ? this.title(t, p) : div().flex_none().text_size(17).font_semibold().child(targetName(this.selected, this.projects)));
    col.child(this.ruleList(t));
    if (p)
      col.child(
        h_flex()
          .flex_none()
          .h(40)
          .gap(10)
          .items_center()
          .border_t(1)
          .border_color(t.border)
          .text_size(TEXT.small)
          .text_color(t.muted)
          .child(Icon("bot", 14))
          .child("агенты по папке проекта"),
      );
    return col;
  }

  // The first run: nothing to show but what Другое could become.
  empty(t) {
    return v_flex()
      .flex_none()
      .pt(40)
      .gap(10)
      .items_center()
      .child(div().text_color(t.dim).child(Icon("sparkles", 26)))
      .child(div().text_size(15).font_semibold().child("Проектов пока нет"))
      .when(this.suggestions.length > 0, (el) =>
        el
          .child(div().text_size(TEXT.small).text_color(t.dim).child("Всё копится в «Другом». Чаще всего за 3 дня:"))
          .child(
            v_flex()
              .flex_none()
              .w(460)
              .gap(4)
              .children(
                this.suggestions.map((h, i) =>
                  h_flex()
                    .flex_none()
                    .h(34)
                    .pl(12)
                    .pr(6)
                    .gap(10)
                    .items_center()
                    .rounded(6)
                    .bg(t.chrome)
                    .child(div().flex_1().min_w(0).truncate().font_family("Consolas").text_size(12).child(ruleText(h)))
                    .child(div().flex_none().text_size(TEXT.small).text_color(t.dim).child(formatDuration(h.ms)))
                    .child(Button({ id: `projects-suggest-${i}`, label: "Создать", onClick: (cx) => this.newProject(cx, h) })),
                ),
              ),
          ),
      );
  }

  title(t, p) {
    const archive = p.archived ? "Вернуть из архива" : "Архивировать проект";
    return h_flex()
      .flex_none()
      .gap(10)
      .items_center()
      .child(div().flex_none().size(12).rounded(6).bg(t[projectColor(p.id, this.projects)] ?? t.muted))
      .child(
        Input.new(this.nameInput)
          .id("projects-name")
          .w(240)
          .h(32)
          .px(6)
          .ml(-6)
          .rounded(6)
          .border_1()
          .border_color(NONE)
          .hover((s) => s.border_color(t.border))
          .focus((s) => s.border_color(t.border))
          .text_size(17)
          .font_semibold()
          .text_color(t.foreground),
      )
      .child(div().flex_1())
      .child(Swatches({ id: "projects-color", size: 16, value: projectColor(p.id, this.projects), projects: this.projects, self: p.id, onPick: (c, cx) => this.setColor(c, cx) }))
      .child(
        div()
          .id("projects-archive")
          .flex_none()
          .size(30)
          .flex()
          .items_center()
          .justify_center()
          .rounded(6)
          .border_1()
          .border_color(t.border)
          .text_color(t.muted)
          .accessibility_label(archive)
          .tooltip(archive)
          .cursor_pointer()
          .hover((s) => s.bg(t.hover).text_color(t.foreground))
          .on_click((_e, cx) => this.toggleArchive(cx))
          .child(Icon("archive", 15)),
      );
  }

  ruleList(t) {
    const rules = this.rules.filter((r) => r.project === this.selected);
    return v_flex()
      .flex_none()
      .gap(6)
      .child(
        h_flex()
          .gap(8)
          .items_center()
          .child(div().text_size(TEXT.body).font_semibold().child("Правила"))
          .child(div().flex_1())
          .child(div().text_size(TEXT.small).text_color(t.dim).child("за 7 дней")),
      )
      .when(!rules.length, (el) => el.child(div().text_size(TEXT.small).text_color(t.dim).child("Правил пока нет.")))
      .children(rules.map((r) => this.ruleRow(t, r)))
      .child(
        h_flex()
          .flex_none()
          .pt(4)
          .gap(8)
          .items_center()
          .flex_wrap()
          .child(div().flex_none().text_color(t.dim).child(Icon("plus", 14)))
          .children(
            RULE_KINDS.map((k) =>
              div()
                .id(`projects-add-${k.id}`)
                .flex_none()
                .px(10)
                .py(4)
                .rounded(6)
                .border_1()
                .border_dashed()
                .border_color(t.border)
                .text_size(12)
                .text_color(t.muted)
                .accessibility_label(`Новое правило: ${k.label}`)
                .cursor_pointer()
                .hover((s) => s.bg(t.hover).text_color(t.foreground))
                .on_click((_e, cx) => this.editRule({ kind: k.id, value: "", project: this.selected }, cx))
                .child(k.label),
            ),
          ),
      );
  }

  ruleRow(t, r) {
    const kind = RULE_KINDS.find((k) => k.id === r.kind);
    return h_flex()
      .id(`projects-rule-${r.id}`)
      .flex_none()
      .h(38)
      .pl(12)
      .pr(6)
      .gap(10)
      .items_center()
      .rounded(6)
      .bg(t.chrome)
      .child(div().flex_none().text_color(t.dim).accessibility_label(kind?.label ?? r.kind).child(Icon(kind?.icon ?? "type", 14)))
      .child(div().flex_1().min_w(0).truncate().font_family("Consolas").text_size(12).child(ruleText(r)))
      .child(div().flex_none().text_size(TEXT.small).text_color(t.dim).child(formatDuration(r.ms)))
      .child(this.ruleMenu(t, r));
  }

  ruleMenu(t, r) {
    const open = this.menu === r.id;
    const trigger = div()
      .id(`projects-rule-more-${r.id}`)
      .flex_none()
      .size(26)
      .flex()
      .items_center()
      .justify_center()
      .rounded(5)
      .text_color(open ? t.foreground : t.dim)
      .when(open, (el) => el.bg(t.hover))
      .accessibility_label("Действия с правилом")
      .cursor_pointer()
      .hover((s) => s.bg(t.hover).text_color(t.foreground))
      .child(Icon("ellipsis", 15));
    return Popover.new(`projects-rule-actions-${r.id}`)
      .open(open)
      .anchor("top_right")
      .on_open_change((value, cx) => {
        this.menu = value ? r.id : null;
        cx.notify();
      })
      .trigger(trigger)
      .when(open, (el) =>
        el.content(
          MenuPanel(`projects-rule-menu-${r.id}`, 160)
            .child(Item(`projects-rule-edit-${r.id}`, (cx) => this.editRule(r, cx)).child("Изменить"))
            .child(Item(`projects-rule-delete-${r.id}`, (cx) => this.deleteRule(r, cx)).child("Удалить")),
        ),
      );
  }
}
