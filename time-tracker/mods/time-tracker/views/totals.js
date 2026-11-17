// The day's totals by project — in the panel under the lanes and in the chip's card — and "no project"
// opened up to assign. Rows are drawn as Usage Limits draws its own: name, bar, numbers in columns.
import { div } from "gpui-kit";
import { h_flex, v_flex } from "gpui-base";
import { formatDuration } from "../format.js";
import { projectColor } from "../lanes.js";
import { Message, TEXT } from "../ui.js";
import { Link } from "./menu.js";

const BAR_W = 110;
const ME_W = 56;
const AGENTS_W = 64;

export function ShareBar(fill, color, t, width = BAR_W) {
  return div().relative().w(width).h(4).rounded(2).bg(t.border).flex_none()
    .child(div().absolute().left(0).top(0).h(4).rounded(2).w(Math.round(width * Math.min(1, Math.max(0, fill)))).bg(color));
}

const num = (w, text, color) => div().w(w).flex_none().flex().justify_end().whitespace_nowrap().text_color(color).child(text);

// The column heads over the rows.
export function TotalsHead(t) {
  return h_flex().gap(8).items_center().text_size(TEXT.caption).text_color(t.dim)
    .child(div().flex_1())
    .child(num(ME_W, "моё", t.dim))
    .child(num(AGENTS_W, "агенты", t.dim));
}

// p: a report project; total: my time across all of them, for the bar; extra: what goes after the name.
export function ProjectRow(p, name, order, total, t, extra = null) {
  const color = t[projectColor(p.project, order)] ?? t.muted;
  return h_flex().gap(8).items_center()
    .child(div().size(8).rounded(4).flex_none().bg(color))
    .child(div().flex_1().min_w(0).truncate().text_color(p.project ? t.foreground : t.muted).child(name))
    .when(Boolean(extra), (el) => el.child(extra))
    .child(ShareBar(total > 0 ? p.me / total : 0, color, t))
    .child(num(ME_W, formatDuration(p.me), p.me > 0 ? t.foreground : t.dim))
    .child(num(AGENTS_W, p.agents > 0 ? formatDuration(p.agents) : "—", t.dim));
}

// day: the Day view, which holds the data and the actions.
export function Totals(day, t) {
  const order = day.projects.map((p) => p.id);
  const report = day.data.report;
  const list = v_flex().mx(14).mt(14).pt(12).gap(8).border_t(1).border_color(t.border).text_size(TEXT.small).child(TotalsHead(t));
  if (!report.projects.length) return list.child(Message("За этот день ничего не насчитано."));
  for (const p of report.projects) {
    const none = p.project === "";
    const assign = none && p.me > 0
      ? Link({ id: "day-assign", label: day.unassignedOpen ? "скрыть" : "назначить", color: t.dim, onClick: (cx) => day.toggleUnassigned(cx) }).text_size(TEXT.caption)
      : null;
    list.child(ProjectRow(p, day.name(p.project), order, report.me, t, assign));
    if (none && day.unassignedOpen) list.child(Unassigned(day, t));
  }
  return list;
}

// My time without a project by what a rule could match; picking a project writes that rule.
function Unassigned(day, t) {
  const box = v_flex().pl(16).pb(4).gap(2).text_size(TEXT.small);
  if (!day.unassigned) return box.child(Message("Загрузка…"));
  if (!day.unassigned.length) return box.child(Message("Нечего назначить: правилу не за что зацепиться."));
  day.unassigned.forEach((u, i) => {
    const kind = { path: "папка", title: "заголовок", figma: "Figma" }[u.kind] ?? u.kind;
    box.child(
      h_flex()
        .id(`un-${i}`)
        .h(24)
        .px(6)
        .gap(10)
        .items_center()
        .rounded(4)
        .cursor_pointer()
        .hover((s) => s.bg(t.hover))
        .on_click((_e, cx) => ((day.assigning = day.assigning === i ? null : i), cx.notify()))
        .child(div().w(64).flex_none().text_color(t.dim).child(kind))
        .child(div().flex_1().min_w(0).truncate().text_color(t.muted).child(u.exe ? `${u.value} (${u.exe})` : u.value))
        .child(num(ME_W, formatDuration(u.ms), t.foreground)),
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
