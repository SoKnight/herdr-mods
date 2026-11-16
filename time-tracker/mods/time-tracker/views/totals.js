// Below the lanes: the day's totals by project, and "no project" opened up to assign.
import { div } from "gpui-kit";
import { h_flex, v_flex } from "gpui-base";
import { formatDuration } from "../format.js";
import { projectColor } from "../lanes.js";
import { Message, TEXT } from "../ui.js";
import { Link } from "./menu.js";

// day: the Day view, which holds the data and the actions.
export function Totals(day, t) {
  const order = day.projects.map((p) => p.id);
  const list = v_flex().px(14).pt(12).gap(2);
  for (const p of day.data.report.projects) {
    const none = p.project === "";
    list.child(
      h_flex()
        .h(26)
        .gap(10)
        .items_center()
        .text_size(TEXT.small)
        .child(div().size(9).rounded(5).flex_none().bg(t[projectColor(p.project, order)] ?? t.muted))
        .child(div().flex_1().min_w(0).truncate().child(day.name(p.project)))
        .when(none && p.me > 0, (el) =>
          el.child(Link({ id: "day-assign", label: day.unassignedOpen ? "скрыть" : "назначить", onClick: (cx) => day.toggleUnassigned(cx) })),
        )
        .child(div().w(90).flex_none().child(`моё ${formatDuration(p.me)}`))
        .child(div().w(110).flex_none().text_color(t.muted).child(`агенты ${formatDuration(p.agents)}`)),
    );
    if (none && day.unassignedOpen) list.child(Unassigned(day, t));
  }
  return list;
}

// My time without a project by what a rule could match; picking a project writes that rule.
function Unassigned(day, t) {
  const box = v_flex().pl(19).pb(6).gap(2).text_size(TEXT.small);
  if (!day.unassigned) return box.child(Message("Загрузка…"));
  if (!day.unassigned.length) return box.child(Message("Нечего назначить: правилу не за что зацепиться."));
  day.unassigned.forEach((u, i) => {
    const kind = { path: "папка", title: "заголовок", figma: "Figma" }[u.kind] ?? u.kind;
    box.child(
      h_flex()
        .id(`un-${i}`)
        .h(24)
        .gap(10)
        .items_center()
        .cursor_pointer()
        .hover((s) => s.text_color(t.accent))
        .on_click((_e, cx) => ((day.assigning = day.assigning === i ? null : i), cx.notify()))
        .child(div().w(70).flex_none().text_color(t.dim).child(kind))
        .child(div().flex_1().min_w(0).truncate().child(u.exe ? `${u.value} (${u.exe})` : u.value))
        .child(div().w(90).flex_none().child(formatDuration(u.ms))),
    );
    if (day.assigning === i)
      box.child(
        h_flex()
          .pl(80)
          .pb(4)
          .gap(12)
          .flex_wrap()
          .child(div().text_color(t.dim).child("Проект →"))
          .children(day.projects.map((p) => Link({ id: `un-${i}-${p.id}`, label: p.name || p.id, onClick: (cx) => day.assign(u, p.id, cx) }))),
      );
  });
  return box;
}
