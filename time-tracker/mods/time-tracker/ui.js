// The look of this repo's panels (Branch PR, Branch Issue), with only what this mod uses:
// the same frame, header pills, icon button and message.
import { div, svg } from "gpui-kit";
import { h_flex, v_flex, Popup } from "gpui-base";
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
  return h_flex().flex_none().px(14).py(10).gap(4).items_center().border_b(1).border_color(t.border);
}

export function Badge(text, color) {
  const t = theme();
  return div().flex_none().px(5).py(1).rounded(4).border_1().border_color(color ?? t.border).text_size(TEXT.badge).text_color(color ?? t.muted).child(String(text));
}

// A panel title that doubles as a tab, the same pill as Herdr's tab bar; `dot` is a colour.
export function HeaderTab({ id, label, active, onClick, dot }) {
  const t = theme();
  return h_flex()
    .id(id)
    .min_w(0)
    .px(12)
    .py(6)
    .gap(6)
    .items_center()
    .rounded(7)
    .border(1)
    .border_color(active ? t.border : t.background)
    .bg(active ? t.raised : t.background)
    .text_size(13)
    .text_color(active ? t.foreground : t.muted)
    .role("tab")
    .aria_selected(Boolean(active))
    .cursor_pointer()
    .hover((s) => s.bg(t.hover))
    .on_click((_e, cx) => onClick(cx))
    .when(dot, (el) => el.child(div().size(6).flex_none().rounded(3).bg(dot)))
    .child(div().min_w(0).truncate().child(label));
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

export function Button({ id, label, icon, primary, ghost, onClick }) {
  const t = theme();
  return h_flex()
    .id(id)
    .flex_none()
    .px(10)
    .py(4)
    .gap(6)
    .items_center()
    .rounded(6)
    .text_size(12)
    .cursor_pointer()
    .on_click((_e, cx) => onClick(cx))
    .when(primary, (el) => el.bg(t.solid).text_color(t.solid_foreground).font_medium())
    .when(ghost, (el) => el.text_color(t.dim).hover((s) => s.text_color(t.foreground)))
    .when(!primary && !ghost, (el) => el.bg(t.raised).text_color(t.foreground).border_1().border_color(t.border))
    .when(icon, (el) => el.child(Icon(icon, 12)))
    .child(label);
}

export function Placeholder(title, text) {
  const t = theme();
  return v_flex()
    .flex_1()
    .min_h(0)
    .items_center()
    .justify_center()
    .gap(6)
    .p(24)
    .child(div().text_size(15).font_semibold().child(title))
    .child(div().max_w(420).text_size(12).text_center().whitespace_normal().text_color(t.dim).child(text));
}

// A dialog over the whole panel (which is `relative`): a dimmed backdrop that keeps presses from what's
// under it, and the dialog centred on it. `icon` sits before the title, `sub` after it; `footer` is the row's children.
export function Dialog({ id, width, icon, title, sub, onClose, body, footer }) {
  const t = theme();
  const stop = (_e, cx) => cx.stop_propagation();
  return div()
    .id(id)
    .absolute()
    .inset_0()
    .flex()
    .items_center()
    .justify_center()
    .on_mouse_down("left", stop)
    .on_mouse_down("right", stop)
    .on_scroll_wheel(stop)
    .child(div().absolute().inset_0().bg(t.background).opacity(0.8))
    .child(
      v_flex()
        .w(width)
        .role("dialog")
        .accessibility_label(title)
        .rounded(10)
        .border_1()
        .border_color(t.border)
        .bg(t.chrome)
        .shadow_lg()
        .child(
          h_flex()
            .flex_none()
            .px(18)
            .py(12)
            .gap(10)
            .items_center()
            .border_b(1)
            .border_color(t.border)
            .when(Boolean(icon), (el) => el.child(Icon(icon, 16)))
            .child(div().flex_none().text_size(15).font_semibold().child(title))
            .when(Boolean(sub), (el) => el.child(sub))
            .child(div().flex_1())
            .child(IconButton({ id: `${id}-close`, icon: "x", label: "Закрыть", onClick: onClose }).mr(-6)),
        )
        .child(body)
        .child(h_flex().flex_none().px(18).py(12).gap(8).items_center().border_t(1).border_color(t.border).children(footer)),
    );
}

// One of a few choices, side by side; options: [{ id, label, icon }].
export function Segmented({ id, options, value, onChange }) {
  const t = theme();
  return h_flex()
    .id(id)
    .flex_none()
    .p(2)
    .rounded(7)
    .border_1()
    .border_color(t.border)
    .bg(t.background)
    .role("radio_group")
    .children(
      options.map((o) => {
        const on = o.id === value;
        return h_flex()
          .id(`${id}-${o.id}`)
          .flex_1()
          .h(28)
          .gap(6)
          .items_center()
          .justify_center()
          .rounded(5)
          .text_size(12)
          .text_color(on ? t.foreground : t.dim)
          .role("radio_button")
          .aria_selected(on)
          .cursor_pointer()
          .when(on, (el) => el.bg(t.raised))
          .when(!on, (el) => el.hover((s) => s.text_color(t.foreground)))
          .on_click((_e, cx) => onChange(o.id, cx))
          .when(Boolean(o.icon), (el) => el.child(Icon(o.icon, 13)))
          .child(o.label);
      }),
    );
}

// A checkbox, or a radio button with `radio`, and its label.
export function Check({ id, checked, label, radio, onClick }) {
  const t = theme();
  const box = div()
    .size(14)
    .flex_none()
    .flex()
    .items_center()
    .justify_center()
    .rounded(radio ? 7 : 4)
    .border_1()
    .border_color(checked ? t.foreground : t.dim)
    .bg(checked ? t.foreground : t.background)
    .text_color(t.background)
    .when(checked && radio, (el) => el.child(div().size(6).rounded(3).bg(t.background)))
    .when(checked && !radio, (el) => el.child(Icon("check", 10)));
  return h_flex()
    .id(id)
    .flex_none()
    .gap(8)
    .items_center()
    .text_color(checked ? t.foreground : t.muted)
    .role(radio ? "radio_button" : "check_box")
    .aria_selected(Boolean(checked))
    .cursor_pointer()
    .on_click((_e, cx) => onClick(cx))
    .child(box)
    .child(label);
}

// A button that opens a list under it; items: [{ id, label, color }], a null item is a divider.
// The trigger toggles on press, after the list's press-outside (capture phase) has closed it.
export function Dropdown({ id, label, open, onToggle, items, onPick }) {
  const t = theme();
  const trigger = h_flex()
    .id(id)
    .flex_none()
    .px(10)
    .py(4)
    .gap(6)
    .items_center()
    .rounded(6)
    .border_1()
    .border_color(t.border)
    .bg(t.background)
    .text_size(12)
    .cursor_pointer()
    .on_mouse_down("left", (_e, cx) => onToggle(!open, cx))
    .child(label)
    .child(div().text_color(t.dim).child(Icon("chevron-down", 12)));
  const list = v_flex()
    .w(200)
    .p(6)
    .rounded(8)
    .border_1()
    .border_color(t.border)
    .bg(t.popover)
    .shadow_lg()
    .text_color(t.foreground)
    .on_mouse_down_out((_e, cx) => onToggle(false, cx))
    .children(
      items.map((it) =>
        it
          ? h_flex()
              .id(`${id}-${it.id}`)
              .px(8)
              .py(7)
              .gap(8)
              .items_center()
              .rounded(5)
              .cursor_pointer()
              .hover((s) => s.bg(t.hover))
              .on_click((_e, cx) => onPick(it.id, cx))
              .when(Boolean(it.color), (el) => el.child(div().flex_none().size(8).rounded(4).bg(it.color)))
              .child(div().min_w(0).truncate().child(it.label))
          : div().flex_none().h(1).my(4).bg(t.border),
      ),
    );
  return Popup.new(`${id}-popup`, trigger).anchor("top_right").when(open, (el) => el.content(list));
}
