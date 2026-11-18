// A session's stretches as rows, with the breaks between them, under a header for the session.
import { div } from "gpui-kit";
import { h_flex, v_flex } from "gpui-base";
import { formatDuration, hhmm, shareText } from "../format.js";
import { projectColor } from "../lanes.js";
import { Icon, TEXT } from "../ui.js";
import { Link } from "./menu.js";

export function StretchList(view, t, stretches, { row, gap }) {
  const list = v_flex();
  for (const s of stretches) {
    if (s.gap_before > 0)
      list.child(
        h_flex()
          .h(gap)
          .pl(96)
          .gap(10)
          .items_center()
          .text_size(TEXT.caption)
          .text_color(t.dim)
          .child(div().w(1).h(gap).bg(t.border))
          .child(`перерыв ${formatDuration(s.gap_before)}`),
      );
    const share = shareText(s.shares, (id) => view.name(id));
    list.child(
      h_flex()
        .h(row)
        .px(8)
        .gap(12)
        .items_center()
        .rounded(6)
        .when(s.current, (el) => el.bg(t.chrome))
        .child(div().w(84).flex_none().text_color(t.dim).child(`${hhmm(s.start)}–${hhmm(s.end)}`))
        .child(div().size(8).flex_none().rounded(4).bg(t[projectColor(s.project, view.projects)] ?? t.muted))
        .child(div().flex_none().font_medium().child(view.name(s.project)))
        .when(Boolean(share), (el) => el.child(div().min_w(0).truncate().text_size(TEXT.small).text_color(t.dim).child(share)))
        .child(div().flex_1())
        .when(s.commits > 0, (el) =>
          el.child(h_flex().flex_none().gap(4).items_center().text_size(TEXT.small).text_color(t.dim).accessibility_label("Коммиты").child(Icon("git-commit-horizontal", 14)).child(String(s.commits))))
        .when(s.current, (el) => el.child(div().flex_none().px(6).py(1).rounded(4).bg(t.ansi2).text_color(t.background).text_size(TEXT.caption).child("сейчас")))
        .child(div().w(60).flex_none().flex().justify_end().font_medium().child(formatDuration(s.me))),
    );
  }
  return list;
}

export function SessionHeader(view, t, session) {
  return h_flex()
    .h(28)
    .px(8)
    .gap(10)
    .items_center()
    .border_b(1)
    .border_color(t.border)
    .text_size(TEXT.small)
    .text_color(t.muted)
    .child(div().font_semibold().text_color(t.foreground).child("Сессия"))
    .child(`${hhmm(session.start)} – ${session.open ? "сейчас" : hhmm(session.end)}`)
    .child(div().flex_1())
    .child(`${formatDuration(session.me)}${session.breaks > 0 ? ` · перерывы ${formatDuration(session.breaks)}` : ""}`)
    .when(session.open, (el) => el.child(Link({ id: `session-open-${session.start}`, label: "открыть →", color: t.muted, onClick: (cx) => view.open("session", cx) })));
}
