// The menu over a selection on the day's lanes: what to do with that time, step by step.
import { div } from "gpui-kit";
import { h_flex, v_flex, Input } from "gpui-base";
import { theme } from "herdr";
import { hhmm, formatDuration } from "../format.js";
import { composition, topWindow, ruleItem, projectColor } from "../lanes.js";
import { TEXT } from "../ui.js";

// A word that acts: for confirmations, strips and list rows.
export function Link({ id, label, onClick, color }) {
  const t = theme();
  return div()
    .id(id)
    .flex_none()
    .text_color(color ?? t.accent)
    .cursor_pointer()
    .hover((s) => s.underline())
    .on_click((_e, cx) => onClick(cx))
    .child(label);
}

export function Item(id, onClick, open) {
  const t = theme();
  return h_flex()
    .id(id)
    .flex_none()
    .px(8)
    .py(7)
    .gap(8)
    .items_center()
    .rounded(5)
    .text_size(TEXT.body)
    .cursor_pointer()
    .when(Boolean(open), (el) => el.bg(t.hover))
    .hover((s) => s.bg(t.hover))
    .on_click((_e, cx) => onClick(cx));
}

const DIVIDER = 9; // its height with margins
const Divider = () => div().flex_none().h(1).my(4).bg(theme().border);

// The popover look of the menu and its submenus.
export function MenuPanel(id, width) {
  const t = theme();
  return v_flex()
    .id(id)
    .flex_none()
    .w(width)
    .p(6)
    .rounded(8)
    .border_1()
    .border_color(t.border)
    .bg(t.popover)
    .shadow_lg()
    .text_color(t.foreground)
    .on_mouse_down("left", (_e, cx) => cx.stop_propagation());
}

// What sits above the item a submenu opens from: the header, the items before it, the divider.
const HEADER = 56;
const ROW = 31;
const SUB_TOP = { project: 0, activity: ROW, note: 3 * ROW, rule: 4 * ROW + DIVIDER };

// view: the Day view (or Session view), which holds the selection (sel), the step and the actions.
export function MarkupMenu(view) {
  const t = theme();
  const { sel, step } = view;
  const comp = composition(view.data.timeline.me, sel.start, sel.end);
  const top = comp[0]?.project;
  const win = topWindow(view.data.timeline.me, sel.start, sel.end);
  const rule = win ? ruleItem(win) : null;
  const go = (next) => (cx) => {
    if (next === "note") view.openNote(cx);
    view.step = next;
    cx.notify();
  };
  const chevron = () => div().flex_1().flex().justify_end().text_color(t.dim).child("›");
  const sub = (id, label, to) => Item(id, go(to), step === to).child(label).child(chevron());

  const total = comp.reduce((n, c) => n + c.ms, 0);
  const summary = comp.map((c) => `${c.project ? view.name(c.project) : "другое"} ${formatDuration(c.ms)}`).join(" · ");
  const menu = MenuPanel("day-menu", 264)
    .child(
      v_flex().flex_none().gap(2).px(8).pt(6).pb(8).mb(4).border_b_1().border_color(t.border)
        .child(div().font_semibold().child(`${hhmm(sel.start)} – ${hhmm(sel.end)} · ${formatDuration(total)}`))
        .child(div().text_size(TEXT.small).text_color(t.dim).child(summary)),
    )
    .child(sub("menu-project", "Назначить проект", "project"))
    .child(sub("menu-activity", "Вид работы", "activity"))
    .child(Item("menu-exclude", (cx) => view.correct({ kind: "exclude" }, cx)).child("Исключить — не работа"))
    .child(Item("menu-note", go("note"), step === "note").child("Заметка…"));
  if (rule)
    menu
      .child(Divider())
      .child(
        Item("menu-rule", go("rule"), step === "rule")
          .items_start()
          .child(
            v_flex().gap(2).child(div().child("Создать правило из окна…")).child(div().text_size(TEXT.caption).text_color(t.dim).child(`${win.app.replace(/\.exe$/i, "")} · «${win.title}»`)),
          ),
      );

  const projects = (prefix, pick) =>
    view.projects.filter((p) => !p.archived).map((p) =>
      Item(`${prefix}-${p.id}`, (cx) => pick(p.id, cx), p.id === top)
        .child(div().flex_none().size(8).rounded(4).bg(t[projectColor(p.id, view.projects)] ?? t.muted))
        .child(div().min_w(0).truncate().child(p.name || p.id)),
    );
  let submenu = null;
  if (step === "project") {
    const pick = (project, cx) => view.correct({ kind: "project", project }, cx);
    submenu = [
      ...projects("menu-p", pick),
      Divider(),
      Item("menu-p-none", (cx) => pick("", cx), top === "").child(div().flex_none().size(8).rounded(4).bg(t.dim)).child("Без проекта"),
      Divider(),
      Item("menu-p-new", (cx) => view.newProject(cx)).text_color(t.muted).child("+ Новый проект…"),
    ];
  } else if (step === "rule") submenu = projects("menu-r", (project, cx) => view.editRule({ ...rule, project }, cx));
  else if (step === "activity") submenu = view.activities.map((a) => Item(`menu-a-${a.id}`, (cx) => view.correct({ kind: "activity", activity: a.id }, cx)).child(a.label || a.id));
  else if (step === "note")
    submenu = [
      h_flex().p(2).gap(6).items_center()
        .child(Input.new(view.note).id("menu-note-input").flex_1().min_w(0).h(28).px(8).rounded(6).border_1().border_color(t.border).bg(t.input).text_size(TEXT.small))
        .child(Link({ id: "menu-note-save", label: "Сохранить", onClick: (cx) => view.saveNote(cx) })),
    ];

  // Outside both panels closes the menu: on one of them, a press in the other would count as outside.
  return h_flex()
    .items_start()
    .gap(4)
    .on_mouse_down_out((_e, cx) => view.closeMenu(cx))
    .child(menu)
    .when(Boolean(submenu), (el) => el.child(MenuPanel("day-submenu", 200).mt(HEADER + (SUB_TOP[step] ?? 0)).children(submenu)));
}
