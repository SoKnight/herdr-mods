// The look of this repo's panels (Branch PR, Branch Issue), with only what this mod uses:
// the same frame, header pills, icon button, search field, rows and footer.
import { div, svg, image } from "gpui-kit";
import { h_flex, v_flex, Input } from "gpui-base";
import { theme, settings } from "herdr";

export const TEXT = { badge: 10, caption: 11, small: 12, tab: 12.8, body: 13 };
const SPIN_MS = 900;

export const codeFont = () => settings().font_family ?? "JetBrains Mono";

export const Icon = (name, size = 14) => svg(`mods/claude-threads/icons/${name}.svg`).size(size).flex_none();

// Claude Code's own icon, from the ones Herdr ships.
export const ClaudeIcon = () => image("icons/claude.svg").size(14).flex_none();

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

export function TextField({ id, state }) {
  const t = theme();
  return Input.new(state)
    .id(id)
    .flex_1()
    .min_w(0)
    .h(32)
    .px(10)
    .rounded(8)
    .border_1()
    .border_color(t.border)
    .bg(t.input)
    .text_size(TEXT.small)
    .text_color(t.foreground);
}

// A list item: a title with an optional badge on the right, then details with a code on the
// right, so titles, details and codes each line up down the list.
export function ItemRow({ id, icon, title, badge, meta, code, onClick }) {
  const t = theme();
  return v_flex()
    .id(id)
    .flex_none()
    .px(8)
    .py(7)
    .gap(3)
    .rounded(7)
    .cursor_pointer()
    .hover((s) => s.bg(t.hover))
    .on_click((e, cx) => onClick(e, cx))
    .child(h_flex().gap(8).items_center().child(icon).child(div().flex_1().min_w(0).truncate().child(title)).when(Boolean(badge), (el) => el.child(badge)))
    .child(
      h_flex()
        .pl(22)
        .gap(8)
        .items_center()
        .text_size(TEXT.caption)
        .text_color(t.dim)
        .child(div().flex_1().min_w(0).truncate().child(meta))
        .child(div().flex_none().font_family(codeFont()).child(code)),
    );
}

// Waiting for the whole list: fills the content below the header.
export function Busy(label) {
  const t = theme();
  return v_flex()
    .id("threads-busy")
    .flex_1()
    .min_h(0)
    .gap(10)
    .items_center()
    .justify_center()
    .text_size(TEXT.small)
    .text_color(t.dim)
    .child(Icon("refresh", 20).spin(SPIN_MS))
    .child(label);
}

export function Message(text, error = false) {
  const t = theme();
  return div().flex_none().p(8).text_size(TEXT.small).whitespace_normal().text_color(error ? t.ansi1 : t.dim).child(text);
}

export function Footer({ left, right, tooltip }) {
  const t = theme();
  return h_flex()
    .id("threads-footer")
    .h(34)
    .flex_none()
    .px(14)
    .gap(8)
    .items_center()
    .border_t(1)
    .border_color(t.border)
    .text_size(TEXT.caption)
    .text_color(t.dim)
    .child(div().flex_1().min_w(0).truncate().child(left))
    .child(div().flex_none().tooltip(tooltip).child(right));
}
