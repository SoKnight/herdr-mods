// The menu over a selection on the day's lanes: what to do with that time, step by step.
import { div } from "gpui-kit";
import { h_flex, v_flex, Input } from "gpui-base";
import { theme } from "herdr";
import { activityLabel } from "../format.js";
import { TEXT } from "../ui.js";

const ACTIVITIES = ["development", "architecture", "design", "documentation", "study"];
const TARGETS = { me: "моё время", agents: "агенты", both: "оба" };

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

function Item(id, label, onClick) {
  const t = theme();
  return h_flex()
    .id(id)
    .h(28)
    .px(8)
    .items_center()
    .rounded(6)
    .text_size(TEXT.small)
    .cursor_pointer()
    .hover((s) => s.bg(t.hover))
    .on_click((_e, cx) => onClick(cx))
    .child(div().flex_1().min_w(0).truncate().child(label));
}

const hhmm = (ms) => {
  const d = new Date(ms);
  return `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
};

// day: the Day view, which holds the selection (sel), the step and the actions.
export function SelectionMenu(day) {
  const t = theme();
  const { sel, step } = day;
  const go = (next) => (cx) => {
    day.step = next;
    cx.notify();
  };
  const back = Item("menu-back", "‹ Назад", go(sel.undo ? "undo" : "root"));
  const projects = (pick) => [
    ...day.projects.map((p) => Item(`menu-p-${p.id}`, p.name || p.id, (cx) => pick(p.id, cx))),
    Item("menu-p-none", "Без проекта", (cx) => pick("", cx)),
  ];
  const activities = (pick) => ACTIVITIES.map((a) => Item(`menu-a-${a}`, activityLabel(a), (cx) => pick(a, cx)));
  const mine = sel.target !== "agents"; // activity, notes and added time are only ever mine

  let items;
  if (step === "undo")
    items = sel.undo.map((id) => Item(`menu-undo-${id}`, `Отменить правку #${id}`, (cx) => day.undo(id, cx)));
  else if (step === "project") items = [back, ...projects((project, cx) => day.correct({ kind: "project", project }, cx))];
  else if (step === "activity") items = [back, ...activities((activity, cx) => day.correct({ kind: "activity", activity }, cx))];
  else if (step === "add-project")
    items = [back, ...projects((project, cx) => {
      day.addProject = project;
      go("add-activity")(cx);
    })];
  else if (step === "add-activity")
    items = [back, ...activities((activity, cx) => day.correct({ kind: "add", project: day.addProject, activity }, cx))];
  else if (step === "note")
    items = [
      back,
      h_flex()
        .p(4)
        .gap(6)
        .items_center()
        .child(Input.new(day.note).id("menu-note").flex_1().min_w(0).h(28).px(8).rounded(6).border_1().border_color(t.border).bg(t.input).text_size(TEXT.small))
        .child(Link({ id: "menu-note-save", label: "Сохранить", onClick: (cx) => day.correct({ kind: "note", note: day.note.value().trim() }, cx) })),
    ];
  else
    items = [
      Item("menu-project", "Проект →", go("project")),
      mine && Item("menu-activity", "Вид →", go("activity")),
      Item("menu-exclude", "Не считать", (cx) => day.correct({ kind: "exclude" }, cx)),
      mine && Item("menu-add", "Добавить время…", go("add-project")),
      mine && Item("menu-note", "Заметка", (cx) => {
        day.openNote();
        go("note")(cx);
      }),
    ].filter(Boolean);

  const caption = sel.undo ? "Правки этого отрезка" : `${hhmm(sel.start)}–${hhmm(sel.end)} · ${TARGETS[sel.target]}`;
  return v_flex()
    .id("day-menu")
    .w(240)
    .max_h(320)
    .overflow_y_scroll()
    .p(5)
    .gap(1)
    .rounded(7)
    .border_1()
    .border_color(t.border)
    .bg(t.popover)
    .text_color(t.foreground)
    .on_mouse_down("left", (_e, cx) => cx.stop_propagation())
    .on_mouse_down_out((_e, cx) => day.closeMenu(cx))
    .child(div().px(8).py(4).text_size(TEXT.caption).text_color(t.dim).child(caption))
    .children(items);
}
