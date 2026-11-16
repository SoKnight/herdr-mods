// The look of this repo's panels (Branch PR, Branch Issue), with only what this mod uses:
// the same frame, header pills, icon button and message.
import { div, svg } from "gpui-kit";
import { h_flex, v_flex } from "gpui-base";
import { theme } from "herdr";

export const TEXT = { badge: 10, caption: 11, small: 12, tab: 12.8, body: 13 };
const SPIN_MS = 900;

export const Icon = (name, size = 14) => svg(`mods/time-tracker/icons/${name}.svg`).size(size).flex_none();

export function Panel({ id }) {
  const t = theme();
  return v_flex()
    .id(id)
    .size_full()
    .min_w(0)
    .min_h(0)
    .overflow_hidden()
    .bg(t.background)
    .text_color(t.foreground)
    .text_size(TEXT.body)
    .rounded(10)
    .border_1()
    .border_color(t.border);
}

export function PanelHeader() {
  const t = theme();
  return h_flex().h(42).flex_none().px(14).gap(8).items_center().border_b(1).border_color(t.border);
}

export function Badge(text, color) {
  const t = theme();
  return div().flex_none().px(5).py(1).rounded(4).border_1().border_color(color ?? t.border).text_size(TEXT.badge).text_color(color ?? t.muted).child(String(text));
}

// A panel title that doubles as a tab, the same pill as Herdr's tab bar.
export function HeaderTab({ id, label, count, active, onClick }) {
  const t = theme();
  return h_flex()
    .id(id)
    .h(29)
    .min_w(0)
    .px(10)
    .gap(7)
    .items_center()
    .rounded(7)
    .border(1)
    .border_color(active ? t.border : t.background)
    .bg(active ? t.raised : t.background)
    .text_size(TEXT.tab)
    .text_color(active ? t.foreground : t.muted)
    .role("tab")
    .aria_selected(Boolean(active))
    .cursor_pointer()
    .hover((s) => s.bg(t.hover))
    .on_click((_e, cx) => onClick(cx))
    .child(div().min_w(0).truncate().child(label))
    .when(count !== undefined && count !== null, (el) => el.child(Badge(count)));
}

export function IconButton({ id, icon, label, onClick, spinning = false }) {
  const t = theme();
  return div()
    .id(id)
    .size(26)
    .flex_none()
    .flex()
    .items_center()
    .justify_center()
    .rounded(6)
    .text_color(t.dim)
    .accessibility_label(label)
    .tooltip(label)
    .cursor_pointer()
    .hover((s) => s.bg(t.hover).text_color(t.foreground))
    .on_click((_e, cx) => {
      cx.stop_propagation();
      onClick(cx);
    })
    .child(spinning ? Icon(icon, 15).spin(SPIN_MS) : Icon(icon, 15));
}

export function Message(text, error = false) {
  const t = theme();
  return div().flex_none().p(8).text_size(TEXT.small).whitespace_normal().text_color(error ? t.ansi1 : t.dim).child(text);
}
