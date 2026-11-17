// The Отчёт tab: a project's month as the collector's month report gives it, by week or day, with the goal,
// my time, the difference and the agents; on the right the month's kinds of work and pay. The month card
// says whether the month is closed. Nothing here adds minutes or money up: it all comes from the report.
import { div } from "gpui-kit";
import { h_flex, v_flex } from "gpui-base";
import { theme, props, layout } from "herdr";
import { call } from "../client.js";
import { fmtMin, signed, catalog, dayTitle, dayString, activityLabel, ACTIVITIES, MONTH_NAMES, SHORT_MONTHS, money, monthOptions, goalStrip, reportFootnote } from "../format.js";
import { projectColor } from "../lanes.js";
import { Button, Check, Dropdown, Icon, IconButton, Message, TEXT } from "../ui.js";
import { Link } from "./menu.js";
import { SideFrame } from "./side.js";
import { GoalEdit } from "./goal.js";
import { BonusAdd } from "./bonus.js";
import { saveCSV, ExportText } from "./export.js";
import { panelShown } from "./day.js";

const REFRESH_MS = 60_000;
const ROWS = [{ id: "week", label: "по неделям" }, { id: "day", label: "по дням" }];
const DOWN = "Сборщик не запущен";

const monthName = (month) => MONTH_NAMES[Number(month.slice(5, 7)) - 1]; // "ноябрь"
const title = (month) => monthName(month)[0].toUpperCase() + monthName(month).slice(1); // "Ноябрь"
const dayOfMonth = (day) => dayTitle(day).split(", ")[1]; // "2026-12-01" → "1 декабря"
const problem = (x) => (x?.down ? DOWN : (x?.error ?? null));

export class Report {
  // open(tab, cx, arg) shows another tab; showDialog(d | null, cx) draws a dialog over the panel.
  constructor(open, showDialog) {
    this.open = open;
    this.showDialog = showDialog;
    this.project = null; // chosen on the first load
    this.month = null; // "YYYY-MM"; null is this month
    this.rows = "week";
    this.agents = true;
    this.today = null;
    this.projects = [];
    this.activities = ACTIVITIES;
    this.rep = null; // what `month report` gave
    this.state = null; // what `month state` gave
    this.menu = null; // the open dropdown: "project" | "month" | "rows"
    this.error = null;
    this.message = null; // { text, error? }
    this.busy = false;
    this.loading = false;
    this.seq = 0;
    this.loadedAt = 0;
    this.wasShown = false;
  }

  tick(cx) {
    const shown = Boolean(props()?.overlay) || panelShown(layout());
    if (shown && (!this.wasShown || Date.now() - this.loadedAt >= REFRESH_MS)) this.load(cx);
    this.wasShown = shown;
  }

  shownMonth() {
    return this.month ?? this.today?.slice(0, 7);
  }

  // The month report, its state and the projects at once; the first time (or when the project is gone) the
  // month's report picks the first project not archived with time in it, else the first.
  load(cx) {
    const seq = ++this.seq;
    this.loading = true;
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        const tl = await call(["timeline"]);
        if (seq !== this.seq) return;
        if (tl.down || tl.error) {
          this.error = problem(tl);
          return;
        }
        this.today = tl.day;
        const month = this.shownMonth();
        const report = (p) => call(["month", "report", month, "--project", p, "--rows", this.rows]);
        const known = this.project;
        const [projects, state, rep, used] = await Promise.all([
          call(["projects"]),
          call(["month", "state", month]),
          known ? report(known) : null,
          known ? null : call(["report", "--from", `${month}-01`, "--to", tl.day]),
        ]);
        if (seq !== this.seq) return;
        const cat = catalog(projects);
        if (!cat) {
          this.error = problem(projects) ?? "projects: нет ответа";
          return;
        }
        Object.assign(this, cat);
        let r = rep;
        if (!this.projects.some((p) => p.id === this.project)) {
          const withTime = (used?.projects ?? []).filter((x) => x.me > 0).map((x) => x.project);
          this.project = (this.projects.find((p) => !p.archived && withTime.includes(p.id)) ?? this.projects[0])?.id ?? null;
          r = this.project ? await report(this.project) : null;
          if (seq !== this.seq) return;
        }
        this.error = problem(r) ?? problem(state);
        this.rep = r && !problem(r) ? r : null;
        this.state = problem(state) ? null : state;
      } finally {
        if (seq === this.seq) {
          this.loading = false;
          this.loadedAt = Date.now();
        }
        cx.notify();
      }
    });
  }

  // A choice changed: the old numbers go at once and the month is read again.
  choose(change, cx) {
    Object.assign(this, change, { rep: null, message: null });
    this.load(cx);
  }

  name(id) {
    return this.projects.find((p) => p.id === id)?.name || id;
  }

  editGoal(cx) {
    const project = this.projects.find((p) => p.id === this.project);
    if (!project) return;
    const rep = this.rep;
    GoalEdit.open(this.showDialog, { project, today: this.today, month: rep.month, plan: rep.plan, planDays: rep.plan_days, onSaved: (text, cx) => this.saved(text, cx) }, cx);
  }

  addBonus(cx) {
    const rep = this.rep;
    BonusAdd.open(this.showDialog, { project: rep.project, name: this.name(rep.project), month: rep.month, pay: rep.pay, onSaved: (text, cx) => this.saved(text, cx) }, cx);
  }

  saved(text, cx) {
    if (text) this.message = { text };
    this.load(cx);
  }

  // A write's answer: a refusal is the message, otherwise the text (if any) and everything read again.
  async write(args, text, cx) {
    const x = await call(args);
    if (x.down) this.message = { text: DOWN, error: true };
    else if (x.error === "closed") this.message = { text: "Месяц закрыт: ничего не записано.", error: true };
    else if (x.error) this.message = { text: x.error, error: true };
    else {
      this.message = text ? { text } : null;
      this.load(cx);
    }
    cx.notify();
  }

  reopen(cx) {
    const month = this.shownMonth();
    cx.spawn((cx) => this.write(["month", "reopen", month], `${title(month)} открыт заново.`, cx));
  }

  removeBonus(id, cx) {
    cx.spawn((cx) => this.write(["bonus", "rm", String(id)], null, cx));
  }

  exportText(cx) {
    const rep = this.rep;
    const onDone = (text, cx) => ((this.message = { text }), cx.notify());
    ExportText.open(this.showDialog, { project: rep.project, name: this.name(rep.project), month: rep.month, rows: this.rows, onDone }, cx);
  }

  saveCSV(cx) {
    if (this.busy || !this.project) return;
    this.busy = true;
    const opts = { project: this.project, month: this.shownMonth(), rows: this.rows, agents: this.agents };
    cx.spawn(async (cx) => {
      this.message = await saveCSV(opts);
      this.busy = false;
      cx.notify();
    });
  }

  render() {
    const t = theme();
    return h_flex()
      .flex_1()
      .min_h(0)
      .items_stretch()
      .child(this.left(t))
      .when(Boolean(this.rep), (el) => el.child(this.side(t)));
  }

  left(t) {
    const col = v_flex().id("report").flex_1().min_w(0).min_h(0).overflow_y_scroll().pt(18).px(22).pb(18).gap(14);
    if (this.today && this.projects.length) col.child(this.filters(t));
    if (this.error) col.child(Message(this.error, true));
    if (!this.projects.length) return col.child(Message(this.loading ? "Загрузка…" : this.error ? "" : "Нет проектов в projects.json."));
    const rep = this.rep;
    if (!rep) return col.child(Message(this.loading ? "Загрузка…" : ""));
    const footnote = reportFootnote(rep, this.agents);
    return col
      .child(this.goalStrip(t))
      .child(this.table(t))
      .when(Boolean(footnote), (el) => el.child(div().flex_none().text_size(TEXT.caption).text_color(t.dim).whitespace_normal().child(footnote)))
      .when(Boolean(this.state), (el) => el.child(this.card(t)))
      .when(Boolean(this.message), (el) => el.child(Message(this.message.text, this.message.error)));
  }

  filters(t) {
    const dropdown = (id, label, items, pick) =>
      Dropdown({ id: `report-${id}`, label, open: this.menu === id, items, onToggle: (open, cx) => ((this.menu = open ? id : null), cx.notify()), onPick: (v, cx) => ((this.menu = null), pick(v, cx)) });
    const caption = (text) => div().text_color(t.dim).child(text);
    const color = (id) => t[projectColor(id, this.projects)] ?? t.muted;
    const months = monthOptions(this.today);
    const month = this.shownMonth();
    return h_flex()
      .flex_none()
      .flex_wrap()
      .gap(8)
      .items_center()
      .child(
        dropdown(
          "project",
          h_flex().gap(6).items_center().child(caption("Проект")).child(div().size(8).flex_none().rounded(4).bg(color(this.project))).child(this.name(this.project) ?? ""),
          this.projects.map((p) => ({ id: p.id, label: p.name || p.id, color: color(p.id) })),
          (id, cx) => this.choose({ project: id }, cx),
        ),
      )
      .child(
        dropdown(
          "month",
          h_flex().gap(6).items_center().child(caption("Период")).child(months.find((m) => m.id === month)?.label ?? month),
          months,
          (id, cx) => this.choose({ month: id === this.today.slice(0, 7) ? null : id }, cx),
        ),
      )
      .child(dropdown("rows", h_flex().gap(6).items_center().child(caption("Строки")).child(ROWS.find((r) => r.id === this.rows).label), ROWS, (id, cx) => this.choose({ rows: id }, cx)))
      .child(div().flex_1())
      .child(Check({ id: "report-agents", checked: this.agents, label: "колонка агентов", onClick: (cx) => ((this.agents = !this.agents), cx.notify()) }).text_size(TEXT.small));
  }

  goalStrip(t) {
    const g = this.rep.goal;
    const closed = Boolean(this.state?.closed);
    const strip = h_flex().flex_none().px(12).py(9).gap(8).items_center().rounded(8).bg(t.chrome);
    if (!g) {
      return strip.child("Цели нет").when(!closed, (el) => el.child(Link({ id: "report-goal-set", label: "задать", onClick: (cx) => this.editGoal(cx) })));
    }
    const s = goalStrip(g);
    return strip
      .child(div().flex_none().text_color(t.dim).child("Цель"))
      .child(div().flex_none().child(s.text))
      .child(div().min_w(0).truncate().text_size(TEXT.small).text_color(t.dim).child(s.sub))
      .child(div().flex_1())
      .when(!closed, (el) => el.child(Link({ id: "report-goal-edit", label: "изменить", onClick: (cx) => this.editGoal(cx) })));
  }

  table(t) {
    const rep = this.rep;
    const goal = Boolean(rep.goal);
    const cell = (w, text) => div().w(w).flex_none().text_right().child(text);
    // the columns after the label: Раб. дн., Цель, Моё, Разница, Агенты, as the goal and the check show them
    const cells = (el, r, head) =>
      el
        .when(goal, (el) => el.child(cell(52, head ? "Раб. дн." : r.workdays ? String(r.workdays) : "—")).child(cell(60, head ? "Цель" : fmtMin(r.plan))))
        .child(cell(60, head ? "Моё" : fmtMin(r.me)))
        .when(goal, (el) => el.child(head ? cell(72, "Разница") : cell(72, signed(r.fact - r.plan)).text_color(r.fact - r.plan <= -1 ? t.foreground : t.dim)))
        .when(this.agents, (el) => el.child(cell(60, head ? "Агенты" : fmtMin(r.agents)).when(!head, (c) => c.text_color(t.dim))));
    const row = (h) => h_flex().h(h).gap(12).px(8).items_center();
    const table = v_flex().flex_none().child(cells(row(24).text_size(TEXT.caption).text_color(t.dim).child(div().flex_1().child(this.rows === "day" ? "День" : "Неделя")), null, true));
    for (const r of rep.rows) {
      table.child(
        cells(
          row(34)
            .border_t(1)
            .border_color(t.chrome)
            .child(h_flex().flex_1().min_w(0).gap(8).items_center().child(r.label).when(Boolean(r.current), (el) => el.child(div().text_size(TEXT.caption).text_color(t.ansi2).child("идёт")))),
          r,
          false,
        ),
      );
    }
    return table.child(cells(row(38).border_t(1).border_color(t.border).font_semibold().child(div().flex_1().child(rep.running ? "Итого на сегодня" : "Итого")), rep.total, false));
  }

  card(t) {
    const st = this.state;
    const month = st.month;
    const box = h_flex().flex_none().px(12).py(10).gap(10).items_center().rounded(8).border_1().border_color(t.border);
    const note = (text) => div().min_w(0).truncate().text_size(TEXT.small).text_color(t.dim).child(text);
    if (st.closed) {
      const at = [dayOfMonth(dayString(st.closed_at)), st.sent && "отчёт отправлен"].filter(Boolean).join(" · ");
      return box
        .child(div().text_color(t.muted).child(Icon("lock", 14)))
        .child(div().flex_none().child(`${title(month)} закрыт`))
        .child(note(at))
        .child(div().flex_1())
        .child(Link({ id: "report-reopen", label: "открыть заново", onClick: (cx) => this.reopen(cx) }));
    }
    const prev = st.prev ? ` · ${monthName(st.prev.month)} ${st.prev.closed ? "закрыт" : "открыт"}` : "";
    const [y, m] = month.split("-").map(Number);
    return box
      .child(div().text_color(t.muted).child(Icon("lock-open", 14)))
      .child(div().flex_none().child(`${title(month)} открыт`))
      .child(note(`правила и правки ещё меняют цифры${prev}`))
      .child(div().flex_1())
      .when(!st.over, (el) => el.child(div().flex_none().text_size(TEXT.small).text_color(t.dim).child(`закрыть можно с ${dayOfMonth(dayString(new Date(y, m, 1).getTime()))}`)));
  }

  side(t) {
    const rep = this.rep;
    const total = rep.total;
    const caption = (text) => div().flex_none().text_size(TEXT.small).text_color(t.dim).child(text);
    const divider = () => div().h(1).flex_none().bg(t.border);
    const line = (left, right) => h_flex().gap(8).items_center().child(left).child(div().flex_1()).child(div().flex_none().child(right));
    const frame = SideFrame(t, "report-side")
      .pb(18)
      .child(
        v_flex()
          .gap(2)
          .child(caption(`${this.name(this.project)} · ${monthName(rep.month)}`))
          .child(div().text_size(26).font_semibold().child(fmtMin(total.me)))
          .when(Boolean(rep.goal), (el) => el.child(caption(`${rep.running ? "цель на сегодня" : "цель"} ${fmtMin(total.plan)} · ${signed(total.fact - total.plan)}`))),
      );

    const acts = rep.activities ?? {};
    const known = this.activities.map((a) => a.id);
    const kinds = [...known, ...Object.keys(acts).filter((k) => !known.includes(k))].filter((k) => acts[k] > 0);
    if (kinds.length) {
      const max = Math.max(...kinds.map((k) => acts[k]));
      frame.child(divider()).child(
        v_flex()
          .gap(8)
          .child(caption("Виды работы"))
          .children(
            kinds.map((k) =>
              v_flex()
                .gap(4)
                .child(line(div().min_w(0).truncate().child(activityLabel(k, this.activities)), fmtMin(acts[k])))
                .child(div().relative().h(4).rounded(2).overflow_hidden().bg(t.raised).child(div().absolute().top(0).left(0).h(4).w(`${(acts[k] / max) * 100}%`).bg(t.muted))),
            ),
          ),
      );
    }
    return frame.child(divider()).child(this.pay(t, caption, line)).child(div().flex_1()).child(
      h_flex()
        .flex_none()
        .gap(8)
        .child(Button({ id: "report-text", label: "Скопировать текстом", icon: "copy", primary: true, onClick: (cx) => this.exportText(cx) }))
        .child(Button({ id: "report-csv", label: "CSV", icon: "download", onClick: (cx) => this.saveCSV(cx) })),
    );
  }

  pay(t, caption, line) {
    const pay = this.rep.pay;
    const closed = Boolean(this.state?.closed);
    const box = v_flex().flex_none().gap(8).child(caption("Оплата"));
    const unset = () => h_flex().gap(8).items_center().child(div().text_color(t.muted).child("Оплата не задана")).when(!closed, (el) => el.child(Link({ id: "report-pay-set", label: "задать", onClick: (cx) => this.editGoal(cx) })));
    const bonuses = h_flex()
      .gap(8)
      .items_center()
      .child(caption("Премии за сроки"))
      .when(!closed, (el) => el.child(Link({ id: "report-bonus-add", label: "+ добавить", onClick: (cx) => this.addBonus(cx) }).text_size(TEXT.small)));
    if (!pay) return box.child(unset()).child(bonuses);
    const month = this.rep.month;
    if (pay.kind === "fixed") box.child(line(`Фикс за ${monthName(month)}`, money(pay.base)));
    else if (pay.kind === "hourly") box.child(line(`Почасовая · ${money(pay.amount)}/ч`, money(pay.base)));
    else box.child(unset());
    box.child(bonuses);
    for (const b of pay.bonuses ?? []) {
      box.child(
        h_flex()
          .gap(6)
          .items_center()
          .child(div().flex_1().min_w(0).truncate().text_size(TEXT.small).text_color(t.dim).child(`${b.title} · ${b.day} ${SHORT_MONTHS[Number(month.slice(5, 7)) - 1]}`))
          .child(div().flex_none().child(money(b.amount)))
          .when(!closed, (el) => el.child(IconButton({ id: `report-bonus-rm-${b.id}`, icon: "x", label: "Убрать премию", onClick: (cx) => this.removeBonus(b.id, cx) }).size(16))),
      );
    }
    box.child(line(div().font_semibold().child("Итого"), money(pay.total)).pt(8).border_t(1).border_color(t.border).font_semibold());
    if (pay.kind === "fixed" && pay.plan > 0) box.child(caption(`по плану ${fmtMin(pay.plan)} → ≈ ${money(pay.rate)}/ч`));
    return box;
  }
}
