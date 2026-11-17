// A stretch of time as a strip: the computer on, my time by project, now; a drag over it selects time
// for the menu. Under it the agents, the hours and the legend.
import { div } from "gpui-kit";
import { h_flex, v_flex, Popup } from "gpui-base";
import { hhmm } from "../format.js";
import { bars, projectColor, ticks } from "../lanes.js";
import { TEXT } from "../ui.js";
import { MarkupMenu } from "./menu.js";

const items = (segs, valueOf) => (segs ?? []).map((seg) => ({ start: seg.start, end: seg.end, value: valueOf(seg), seg }));

// Bars as absolute blocks, full height, placed in percent of the strip.
const blocks = (view, list, h, color) =>
  list.map((bar) => div().absolute().top(0).h(h).left(view.pct(bar.x)).w(view.pct(bar.w)).bg(color(bar)));

export function Strip(view, t, { from, to, h, radius }) {
  const tl = view.data.timeline;
  const order = view.order();
  // The bars clip to the rounded track; the selection and its menu sit outside the clip.
  const track = div().absolute().top(0).left(0).size_full().rounded(radius).overflow_hidden().bg(t.chrome);
  if (view.width > 0) {
    track.children(blocks(view, bars(items(tl.power, () => "on"), from, to, view.width, () => "raised"), h, () => t.raised));
    bars(items(tl.me, (s) => s.project), from, to, view.width, (v) => projectColor(v, order)).forEach((bar, i) =>
      track.child(div().id(`me-${i}`).absolute().top(0).h(h).left(view.pct(bar.x)).w(view.pct(bar.w)).bg(t[bar.color] ?? t.muted).tooltip(view.tip(bar))),
    );
  }
  const now = Date.now();
  if (from <= now && now < to) track.child(div().absolute().top(0).h(h).w(2).left(`${((now - from) / (to - from)) * 100}%`).bg(t.foreground));

  const strip = div()
    .id("day-strip")
    .relative()
    .h(h)
    .w_full()
    .flex_none()
    .on_size_change((size, cx) => {
      if (size.width !== view.width) {
        view.width = size.width;
        cx.notify();
      }
    })
    .child(track);

  const g = view.drag;
  const range = (el, x, w) => el.absolute().top(0).h(h).left(view.pct(x)).w(view.pct(w)).bg(t.selection).border_l(2).border_r(2).border_color(t.foreground);
  if (g) strip.child(range(div(), Math.min(g.x0, g.x1), Math.abs(g.x1 - g.x0)));
  else if (view.sel) strip.child(range(Popup.new("day-sel", div().size_full()), view.sel.x, view.sel.w).anchor("top_left").content(MarkupMenu(view)));
  return strip;
}

// The strip with what goes under it, as wide as the strip: a drag over the strip lives here, so the
// pointer can drift off the 20 px strip without ending it.
export function DragArea(view) {
  return v_flex()
    .id("day-timeline")
    .cursor_crosshair()
    .on_mouse_down("left", (e, cx) => {
      const p = view.point(e);
      view.sel = null;
      view.drag = { x0: p.x, x1: p.x };
      cx.notify();
    })
    .on_mouse_move((e, cx) => {
      if (!view.drag) return;
      view.drag.x1 = view.point(e).x;
      cx.notify();
    })
    .on_mouse_up("left", (_e, cx) => view.finishDrag(cx))
    .on_hover((hovered, cx) => !hovered && view.drag && view.finishDrag(cx));
}

export function AgentsStrip(view, t, { from, to }) {
  const order = view.order();
  const strip = div().relative().h(4).w_full().flex_none().rounded(2).overflow_hidden().bg(t.chrome);
  if (view.width > 0)
    strip.children(blocks(view, bars(items(view.data.timeline.agents, (s) => s.project), from, to, view.width, (v) => projectColor(v, order)), 4, (bar) => t[bar.color] ?? t.muted).map((el) => el.opacity(0.55)));
  return strip;
}

export function HourLabels(t, from, to) {
  return h_flex().h(16).flex_none().justify_between().text_size(TEXT.caption).text_color(t.dim).children(ticks(from, to).map((ms) => div().child(hhmm(ms))));
}

export function Legend(t) {
  const item = (swatch, label) => h_flex().gap(6).items_center().child(swatch.w(14).flex_none().rounded(2)).child(label);
  return h_flex()
    .gap(16)
    .flex_wrap()
    .items_center()
    .text_size(TEXT.caption)
    .text_color(t.dim)
    .child(item(div().h(10).bg(t.muted), "я работал"))
    .child(item(div().h(10).bg(t.raised), "включён, без активности"))
    .child(item(div().h(10).bg(t.chrome).border_1().border_color(t.border), "выключен"))
    .child(item(div().h(4).bg(t.muted), "агенты"));
}
