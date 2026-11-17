// Разбор: a day's Другое by window, each given a project (and a rule, so it counts itself next time).
import { div } from "gpui-kit";
import { h_flex, v_flex } from "gpui-base";
import { theme, props, layout } from "herdr";
import { call } from "../client.js";
import { formatDuration, dayTitle, dayString, plural, catalog } from "../format.js";
import { projectColor, ruleItem } from "../lanes.js";
import { ruleText, targetName, monthStartTitle, timeRange, inboxLeft } from "../rules.js";
import { Button, Check, Dropdown, Icon, Message, TEXT } from "../ui.js";
import { Link } from "./menu.js";
import { panelShown } from "./day.js";
import { SideFrame } from "./side.js";

const REFRESH_MS = 60_000;
const windowKey = (it) => JSON.stringify([it.app, it.title, it.path, it.server]);

export class Inbox {
  // open(tab, cx): shows another tab of the panel.
  constructor(open) {
    this.open = open;
    this.data = null; // what `inbox` gave
    this.projects = [];
    this.want = null; // the day asked for; null is today
    this.decisions = {}; // item index → { project, rule, keep }
    this.past = true; // rules from this month's start
    this.menu = null; // the item whose dropdown is open
    this.seq = 0;
    this.error = null;
    this.message = null; // { text, error? }
    this.loading = false;
    this.applying = false;
    this.loadedAt = 0;
    this.wasShown = false;
  }

  // As Day's: read again on showing, then once a minute while shown.
  tick(cx) {
    const shown = Boolean(props()?.overlay) || panelShown(layout());
    if (shown && (!this.wasShown || Date.now() - this.loadedAt >= REFRESH_MS)) this.load(cx);
    this.wasShown = shown;
  }

  // Shows a day's Другое: null is today.
  show(want, cx) {
    Object.assign(this, { want, data: null, decisions: {}, menu: null, message: null, error: null });
    this.load(cx);
  }

  load(cx) {
    const seq = ++this.seq;
    this.loading = true;
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        const [r, projects] = await Promise.all([call(["inbox", ...(this.want ? ["--day", this.want] : [])]), call(["projects"])]);
        if (seq !== this.seq) return;
        this.projects = catalog(projects)?.projects ?? this.projects;
        this.error = r.down ? "Сборщик не запущен" : (r.error ?? null);
        if (!r.down && !r.error) this.took(r);
      } finally {
        if (seq === this.seq) {
          this.loading = false;
          this.loadedAt = Date.now();
        }
        cx.notify();
      }
    });
  }

  // Decisions and the open dropdown follow their windows into the new list, which may have gained or lost
  // some, but not into another day (today's Разбор rolls over at 06:00).
  took(r) {
    if (this.data && this.data.day !== r.day) Object.assign(this, { decisions: {}, menu: null });
    const kept = new Map(Object.entries(this.decisions).map(([i, d]) => [windowKey(this.data.items[i]), d]));
    const shown = this.menu !== null ? this.data?.items[this.menu] : null;
    const open = shown ? windowKey(shown) : null;
    this.decisions = {};
    this.menu = null;
    r.items.forEach((it, i) => {
      if (kept.has(windowKey(it))) this.decisions[i] = kept.get(windowKey(it));
      if (windowKey(it) === open) this.menu = i;
    });
    this.data = r;
  }

  decide(i, project, cx) {
    const it = this.data.items[i];
    const rule = it.suggest && project === it.suggest.project ? it.suggest.rule : ruleItem(it);
    this.decisions = { ...this.decisions, [i]: { project, rule, keep: true } };
    this.menu = null;
    cx.notify();
  }

  undecide(i, cx) {
    const { [i]: _, ...rest } = this.decisions;
    this.decisions = rest;
    cx.notify();
  }

  apply(cx) {
    if (this.applying) return;
    const decisions = Object.entries(this.decisions).map(([i, d]) => {
      const { app, title, path, server } = this.data.items[i];
      return { app, title, path, server, project: d.project, rule: d.keep ? d.rule : null };
    });
    if (!decisions.length) return;
    this.applying = true;
    this.message = null;
    cx.spawn(async (cx) => {
      try {
        const r = await call(["inbox", "apply", JSON.stringify({ day: this.data.day, past: this.past, decisions })]);
        if (r.down || r.error) {
          this.message = { text: r.down ? "Сборщик не запущен" : r.error === "closed" ? "Месяц закрыт: ничего не записано." : r.error, error: true };
        } else {
          this.show(this.want, cx);
          this.message = { text: `Разобрано: правил — ${r.rules}, правок — ${r.corrections}.` };
        }
      } finally {
        this.applying = false;
        cx.notify();
      }
    });
  }

  render() {
    const t = theme();
    return h_flex()
      .flex_1()
      .min_h(0)
      .items_stretch()
      .child(this.left(t))
      .when(Boolean(this.data), (el) => el.child(this.side(t)));
  }

  dot(t, project) {
    const d = div().flex_none().size(8).rounded(4);
    return project === "exclude" ? d.border_1().border_color(t.dim) : d.bg(t[projectColor(project, this.projects)] ?? t.muted);
  }

  left(t) {
    const d = this.data;
    const col = v_flex().id("inbox").flex_1().min_w(0).min_h(0).overflow_y_scroll().pt(18).px(22).gap(14);
    const notices = [this.error && Message(this.error, true), this.message && div().flex_none().text_size(TEXT.small).whitespace_normal().text_color(this.message.error ? t.ansi1 : t.muted).child(this.message.text)].filter(Boolean);
    if (d && !d.items.length && !Object.keys(this.decisions).length) return col.children(notices).child(this.done(t));
    col.child(
      h_flex()
        .flex_none()
        .gap(10)
        .items_center()
        .child(
          div()
            .id("inbox-back")
            .flex_none()
            .px(4)
            .text_size(16)
            .text_color(t.muted)
            .accessibility_label("Назад ко дню")
            .cursor_pointer()
            .hover((s) => s.text_color(t.foreground))
            .on_click((_e, cx) => this.open("day", cx))
            .child("‹"),
        )
        .child(div().flex_none().text_size(15).font_semibold().child("Другое"))
        .when(Boolean(d), (el) => el.child(div().text_color(t.dim).child(`${dayTitle(d.day)} · ${formatDuration(d.ms)}`))),
    );
    col.child(div().flex_none().text_size(TEXT.small).text_color(t.dim).whitespace_normal().child("Окна, которые не попали ни под одно правило. Назначьте проект — и сохраните правило, чтобы дальше это считалось само."));
    col.children(notices);
    if (!d) return col.child(Message(this.loading ? "Загрузка…" : ""));
    return col.child(v_flex().flex_none().gap(8).children(d.items.map((it, i) => this.card(t, it, i))));
  }

  card(t, it, i) {
    const dec = this.decisions[i];
    const s = it.suggest;
    const pick = (label) =>
      Dropdown({
        id: `inbox-pick-${i}`,
        label,
        open: this.menu === i,
        onToggle: (open, cx) => ((this.menu = open ? i : null), cx.notify()),
        items: [...this.projects.filter((p) => !p.archived).map((p) => ({ id: p.id, label: p.name || p.id, color: t[projectColor(p.id, this.projects)] ?? t.muted })), null, { id: "exclude", label: "Не работа" }],
        onPick: (id, cx) => this.decide(i, id, cx),
      });
    const line = h_flex().flex_none().min_h(28).gap(8).items_center().child(div().flex_none().text_size(TEXT.caption).text_color(t.dim).child(timeRange(it.start, it.end))).child(div().flex_1());
    if (dec)
      line
        .child(div().flex_none().text_size(TEXT.small).text_color(t.dim).child("→"))
        .child(this.dot(t, dec.project))
        .child(div().flex_none().text_size(TEXT.small).child(targetName(dec.project, this.projects)))
        .child(Link({ id: `inbox-undo-${i}`, label: "Отменить", color: t.muted, onClick: (cx) => this.undecide(i, cx) }).text_size(TEXT.small));
    else if (s?.project === "exclude")
      line
        .child(div().flex_none().text_size(TEXT.small).text_color(t.dim).child("похоже, не работа"))
        .child(Button({ id: `inbox-accept-${i}`, label: "Исключить", primary: true, onClick: (cx) => this.decide(i, "exclude", cx) }))
        .child(pick("Другой"));
    else if (s)
      line
        .child(div().flex_none().text_size(TEXT.small).text_color(t.dim).child("похоже на"))
        .child(this.dot(t, s.project))
        .child(div().flex_none().text_size(TEXT.small).child(targetName(s.project, this.projects)))
        .child(Button({ id: `inbox-accept-${i}`, label: "Принять", primary: true, onClick: (cx) => this.decide(i, s.project, cx) }))
        .child(pick("Другой"));
    else line.child(pick("Проект"));
    return v_flex()
      .id(`inbox-item-${i}`)
      .flex_none()
      .px(12)
      .py(10)
      .gap(6)
      .rounded(8)
      .bg(t.chrome)
      .child(
        h_flex()
          .gap(8)
          .items_center()
          .child(div().flex_none().font_medium().child(it.app.replace(/\.exe$/i, "")))
          .child(div().min_w(0).truncate().text_color(t.muted).child(it.title))
          .child(div().flex_1())
          .child(div().flex_none().font_medium().child(formatDuration(it.ms))),
      )
      .child(line);
  }

  // Nothing left to decide.
  done(t) {
    return v_flex()
      .flex_1()
      .items_center()
      .justify_center()
      .gap(8)
      .child(div().text_color(t.ansi2).child(Icon("circle-check", 26)))
      .child(div().text_size(15).font_semibold().child("Всё разобрано"))
      .child(div().text_size(TEXT.small).text_color(t.dim).child("новые окна без правила появятся здесь"))
      .child(Link({ id: "inbox-to-day", label: "← к дню", onClick: (cx) => this.open("day", cx) }));
  }

  side(t) {
    const left = inboxLeft(this.data.items, this.decisions);
    const ruled = Object.entries(this.decisions).filter(([, d]) => d.rule);
    const any = Object.keys(this.decisions).length > 0;
    const divider = () => div().h(1).flex_none().bg(t.border);
    // ponytail: today by the calendar, not the collector's day start; only 00:00–06:00 on the 1st differs
    const today = dayString(Date.now());
    const done = Button({ id: "inbox-apply", label: "Готово", primary: true, onClick: (cx) => this.apply(cx) });
    if (!any) done.bg(t.raised).text_color(t.dim).cursor_default();
    return SideFrame(t, "inbox-side")
      .child(
        v_flex()
          .gap(2)
          .child(div().text_size(TEXT.small).text_color(t.dim).child("Осталось разобрать"))
          .child(div().text_size(26).font_semibold().child(formatDuration(left.ms)))
          .child(div().text_size(TEXT.small).text_color(t.dim).child(`${plural(left.windows, "окно", "окна", "окон")} · ${plural(left.hints, "подсказка", "подсказки", "подсказок")}`)),
      )
      .child(divider())
      .child(
        v_flex()
          .gap(10)
          .child(div().text_size(TEXT.small).text_color(t.muted).child("Новые правила"))
          .children(
            ruled.map(([i, d]) =>
              Check({
                id: `inbox-keep-${i}`,
                checked: d.keep,
                onClick: (cx) => ((this.decisions = { ...this.decisions, [i]: { ...d, keep: !d.keep } }), cx.notify()),
                label: v_flex()
                  .min_w(0)
                  .gap(2)
                  .child(h_flex().gap(8).items_center().child(this.dot(t, d.project)).child(div().min_w(0).truncate().child(targetName(d.project, this.projects))))
                  .child(div().font_family("Consolas").text_size(12).text_color(t.dim).truncate().child(ruleText(d.rule, true))),
              }).items_start(),
            ),
          ),
      )
      .child(divider())
      .child(
        v_flex()
          .gap(6)
          .child(Check({ id: "inbox-past", checked: this.past, label: `применить к прошлым дням (с ${monthStartTitle(today)})`, onClick: (cx) => ((this.past = !this.past), cx.notify()) }).text_size(TEXT.small))
          .child(div().text_size(TEXT.caption).text_color(t.dim).whitespace_normal().child("Закрытые месяцы не меняются. Ручная разметка не перезаписывается.")),
      )
      .child(h_flex().child(done));
  }
}
