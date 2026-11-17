// "Другое" opened up to assign.
import { div } from "gpui-kit";
import { h_flex, v_flex } from "gpui-base";
import { formatDuration } from "../format.js";
import { Message, TEXT } from "../ui.js";
import { Link } from "./menu.js";

const ME_W = 56;

const num = (w, text, color) => div().w(w).flex_none().flex().justify_end().whitespace_nowrap().text_color(color).child(text);

// My time without a project by what a rule could match; picking a project writes that rule.
export function Unassigned(day, t) {
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
