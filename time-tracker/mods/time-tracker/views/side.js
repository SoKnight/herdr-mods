// The right panel: my time in big, a few lines about it, and the projects.
import { div } from "gpui-kit";
import { h_flex, v_flex } from "gpui-base";
import { formatDuration, goalBlock, roundRows } from "../format.js";
import { projectColor } from "../lanes.js";
import { TEXT } from "../ui.js";
import { Link } from "./menu.js";

// lines: strings. The total is the rows' sum, each rounded half up to the minute as Неделя counts them.
// A project with a goal in view.goals shows it; the rest a share of the longest.
export function Side(view, t, { caption, lines }) {
  const { rows, total } = roundRows(view.data.report.projects);
  const projects = rows.filter((p) => p.me >= 60_000); // rounds to nothing: "—"
  const max = Math.max(...projects.map((p) => p.me));
  const block = (p) => {
    const base = { name: view.name(p.project), color: t[projectColor(p.project, view.projects)] ?? t.muted, value: formatDuration(p.me), fill: (p.me / max) * 100, other: !p.project };
    const info = p.project && view.goals?.find((g) => g.project === p.project);
    if (!info) return ProjectBlock(t, base);
    const b = goalBlock(info);
    return ProjectBlock(t, { ...base, value: b.value, goal: b.goal, fill: b.fill ?? base.fill, sub: b.sub });
  };
  return SideFrame(t, "day-side")
    .pb(18)
    .child(
      v_flex()
        .gap(2)
        .child(div().text_size(TEXT.small).text_color(t.dim).child(caption))
        .child(div().text_size(26).font_semibold().child(formatDuration(total)))
        .children(lines.map((line) => div().text_size(TEXT.small).text_color(t.dim).child(line))),
    )
    .child(div().h(1).flex_none().bg(t.border))
    .child(
      v_flex()
        .gap(12)
        .children(projects.map(block))
        .when(projects.some((p) => !p.project), (el) =>
          el.child(Link({ id: "day-inbox", label: "разобрать «Другое» →", color: t.muted, onClick: (cx) => view.open("inbox", cx, view.want ?? null) }).text_size(TEXT.small)),
        ),
    );
}

// The right column's look, shared with Разбор.
export function SideFrame(t, id) {
  return v_flex().id(id).w(270).flex_none().overflow_y_scroll().bg(t.chrome).border_l(1).border_color(t.border).pt(18).px(18).gap(16);
}

// goal: "6ч 30м" after the value, the bar then filled at full opacity; sub: a line under the bar.
export function ProjectBlock(t, { name, color, value, fill, other, goal, sub }) {
  return v_flex()
    .gap(6)
    .child(
      h_flex()
        .gap(8)
        .items_center()
        .child(div().size(8).flex_none().rounded(4).bg(color))
        .child(div().min_w(0).truncate().text_color(other ? t.muted : t.foreground).child(name))
        .child(div().flex_1())
        .child(
          h_flex()
            .flex_none()
            .gap(4)
            .child(div().font_medium().child(value))
            .when(Boolean(goal), (el) => el.child(div().text_color(t.dim).child(`/ ${goal}`))),
        ),
    )
    .child(div().relative().h(4).rounded(2).overflow_hidden().bg(t.raised).child(div().absolute().top(0).left(0).h(4).w(`${fill}%`).bg(color).opacity(goal ? 1 : 0.5)))
    .when(Boolean(sub), (el) => el.child(div().text_size(TEXT.caption).text_color(t.dim).child(sub)));
}
