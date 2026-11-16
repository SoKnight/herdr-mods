// The Report tab: one project over a period, rounded, previewed as Markdown and saved as a file.
// Saving closes the period for that project, so it asks first; closed periods are listed below.
import { div } from "gpui-kit";
import { h_flex, v_flex, InputState, Input, TextView } from "gpui-base";
import { theme, props, layout, home, pickFolder } from "herdr";
import { call } from "../client.js";
import { weekRange, monthRange, exportName, dayString, catalog } from "../format.js";
import { Message, TEXT } from "../ui.js";
import { Link } from "./menu.js";
import { panelShown } from "./day.js";

const REFRESH_MS = 60_000;
const DAY = 24 * 3600_000;
const DATE = /^\d{4}-\d{2}-\d{2}$/;
const label = (ms) => dayString(ms).split("-").reverse().join(".");
const minutes = (ms) => `${ms > 0 ? "+" : ""}${Math.round(ms / 60_000)} мин`;

export class Report {
  constructor() {
    this.projects = [];
    this.project = null;
    this.period = "week"; // week | month | custom
    this.offset = 0; // weeks or months back (-) from the current one
    this.round = 15;
    this.agents = false;
    this.folder = `${home().replace(/\\/g, "/")}/Downloads`;
    this.confirm = null; // "md" | "csv": waiting for the closing warning to be accepted
    this.custom = { from: null, to: null }; // InputStates, made when "custom" is first picked
    this.today = null;
    this.dayStart = 4 * 3600_000;
    this.preview = null; // the file as the collector would write it now
    this.periods = [];
    this.error = null;
    this.readError = null; // the preview or the periods could not be read
    this.message = null; // { text, error? }
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

  // [from, to) of the chosen period, or null while it can't be told (no today yet, or custom dates that aren't).
  range() {
    if (!this.today) return null;
    if (this.period === "week") return weekRange(this.today, this.dayStart, this.offset);
    if (this.period === "month") return monthRange(this.today, this.dayStart, this.offset);
    const [a, b] = [this.custom.from?.value().trim() ?? "", this.custom.to?.value().trim() ?? ""];
    if (!DATE.test(a) || !DATE.test(b)) return null;
    const from = new Date(`${a}T00:00:00`).getTime() + this.dayStart;
    const to = new Date(`${b}T00:00:00`).getTime() + this.dayStart + DAY; // the last day is inside
    return from < to ? { from, to } : null;
  }

  load(cx) {
    const seq = ++this.seq;
    this.loading = true;
    cx.notify();
    cx.spawn(async (cx) => {
      try {
        const [tl, projects] = await Promise.all([call(["timeline"]), call(["projects"])]);
        if (seq !== this.seq) return;
        if (tl.down || tl.error) {
          this.error = tl.down ? "Сборщик не запущен: go -C helper run ./deploy" : tl.error;
          return;
        }
        this.today = tl.day;
        this.dayStart = tl.from - new Date(`${tl.day}T00:00:00`).getTime();
        Object.assign(this, catalog(projects));
        if (!this.projects.some((p) => p.id === this.project)) this.project = this.projects[0]?.id ?? null;
        this.error = null;
        await this.loadData(seq);
      } finally {
        if (seq === this.seq) {
          this.loading = false;
          this.loadedAt = Date.now();
        }
        cx.notify();
      }
    });
  }

  // The preview is the collector's own dry run, so it is what the file will say; the periods come with it.
  async loadData(seq = this.seq) {
    if (!this.project) {
      this.preview = this.readError = null;
      return;
    }
    const r = this.range();
    const args = ["export", "--from", String(r?.from), "--to", String(r?.to), "--project", this.project, "--format", "md", "--round", String(this.round), "--dry-run"];
    if (this.agents) args.push("--agents");
    const [dry, periods] = await Promise.all([r ? call(args) : null, call(["periods", "--project", this.project])]);
    if (seq !== this.seq) return;
    const problem = (x) => (x.down ? "Сборщик не запущен" : x.error);
    this.preview = dry && !problem(dry) ? dry.content : null;
    this.readError = dry?.error === "closed" ? "Период уже закрыт выгрузкой: переоткройте его в списке ниже." : (dry && problem(dry)) || (!Array.isArray(periods) && problem(periods)) || null;
    if (Array.isArray(periods)) this.periods = periods;
  }

  // A choice changed: read the numbers again, nothing else.
  refresh(cx) {
    this.confirm = null;
    this.message = null;
    const seq = ++this.seq;
    cx.spawn(async (cx) => {
      await this.loadData(seq);
      cx.notify();
    });
    cx.notify();
  }

  pick(period, cx) {
    this.period = period;
    this.offset = 0;
    if (period === "custom" && !this.custom.from) {
      for (const k of ["from", "to"]) {
        this.custom[k] = InputState.new({ placeholder: "ГГГГ-ММ-ДД" });
        this.custom[k].on("submit", (_e, cx) => this.refresh(cx));
      }
    }
    this.refresh(cx);
  }

  // The previous (-1) or next week or month.
  shift(n, cx) {
    this.offset += n;
    this.refresh(cx);
  }

  async chooseFolder(cx) {
    const dir = await pickFolder({ prompt: "Выбрать" });
    if (dir) this.folder = dir.replace(/\\/g, "/");
    cx.notify();
  }

  // The warning comes after the preview is read again, so what is closed is what was shown.
  ask(format, cx) {
    const seq = ++this.seq;
    cx.spawn(async (cx) => {
      await this.loadData(seq);
      if (seq === this.seq && this.preview) this.confirm = format;
      cx.notify();
    });
  }

  save(format, cx) {
    const r = this.range();
    this.confirm = null;
    if (!r || !this.project) return;
    const path = `${this.folder}/${exportName(this.project, r.from, r.to, format, this.dayStart)}`;
    const args = ["export", "--from", String(r.from), "--to", String(r.to), "--project", this.project, "--format", format, "--round", String(this.round), "--out", path];
    if (this.agents) args.push("--agents");
    cx.spawn(async (cx) => {
      const x = await call(args);
      if (x.down) this.message = { text: "Сборщик не запущен", error: true };
      else if (x.error === "closed") this.message = { text: "Период уже закрыт выгрузкой: переоткройте его в списке ниже.", error: true };
      else if (x.error) this.message = { text: x.error, error: true };
      else {
        this.message = { text: `Сохранено: ${path}` };
        await this.loadData();
      }
      cx.notify();
    });
    cx.notify();
  }

  reopen(id, cx) {
    cx.spawn(async (cx) => {
      const x = await call(["reopen", String(id)]);
      this.message = x.down || x.error ? { text: x.error ?? "Сборщик не запущен", error: true } : { text: "Период переоткрыт." };
      if (!x.down && !x.error) await this.loadData();
      cx.notify();
    });
  }

  name(id) {
    return this.projects.find((p) => p.id === id)?.name || id;
  }

  render() {
    const t = theme();
    const root = v_flex().id("report").flex_1().min_h(0).overflow_y_scroll().pb(14);
    if (this.error) return root.child(Message(this.error, true));
    if (!this.projects.length) return root.child(Message(this.loading ? "Загрузка…" : "Нет проектов в projects.json."));
    const choice = (id, text, on, onClick) => Link({ id, label: text, color: on ? t.foreground : t.muted, onClick });
    const row = (title) => h_flex().px(14).py(4).gap(12).items_center().flex_wrap().child(div().w(90).flex_none().text_size(TEXT.small).text_color(t.dim).child(title));
    const r = this.range();
    root
      .child(row("Проект").children(this.projects.map((p) => choice(`rep-p-${p.id}`, p.name || p.id, p.id === this.project, (cx) => ((this.project = p.id), this.refresh(cx))))))
      .child(row("Период").children([["week", "Неделя"], ["month", "Месяц"], ["custom", "Свой"]].map(([id, text]) => choice(`rep-per-${id}`, text, this.period === id, (cx) => this.pick(id, cx)))));
    if (this.period !== "custom" && r) {
      root.child(
        row("")
          .child(Link({ id: "rep-prev", label: "‹", onClick: (cx) => this.shift(-1, cx) }))
          .child(div().text_size(TEXT.small).child(`${label(r.from - this.dayStart)} – ${label(r.to - 1 - this.dayStart)}`))
          .when(this.offset < 0, (el) => el.child(Link({ id: "rep-next", label: "›", onClick: (cx) => this.shift(1, cx) }))),
      );
    }
    if (this.period === "custom") {
      const field = (state, id) => Input.new(state).id(id).w(120).h(28).px(8).rounded(6).border_1().border_color(t.border).bg(t.input).text_size(TEXT.small);
      root.child(row("").child(field(this.custom.from, "rep-from")).child(div().text_color(t.dim).child("–")).child(field(this.custom.to, "rep-to")).child(Link({ id: "rep-custom-apply", label: "Показать", onClick: (cx) => this.refresh(cx) })));
    }
    root
      .child(row("Округление").children([1, 5, 15].map((n) => choice(`rep-round-${n}`, `${n} мин`, this.round === n, (cx) => ((this.round = n), this.refresh(cx))))))
      .child(row("").child(choice("rep-agents", `${this.agents ? "☑" : "☐"} Показать время агентов (для справки)`, this.agents, (cx) => ((this.agents = !this.agents), this.refresh(cx)))))
      .child(row("Папка").child(div().min_w(0).truncate().text_size(TEXT.small).child(this.folder)).child(Link({ id: "rep-folder", label: "Выбрать…", onClick: (cx) => this.chooseFolder(cx) })));

    const preview = this.preview;
    root.child(
      div()
        .id("rep-preview")
        .mx(14)
        .my(6)
        .p(10)
        .rounded(8)
        .border_1()
        .border_color(t.border)
        .bg(t.raised)
        .text_size(TEXT.small)
        .whitespace_normal()
        .child(preview ? TextView.markdown("rep-preview-text", preview).text_size(TEXT.small).w_full().min_w(0).scrollable(false).selectable(true) : div().text_color(this.readError ? t.ansi1 : t.dim).child(this.readError ?? (r ? "Загрузка…" : "Введите даты в виде ГГГГ-ММ-ДД, начало не позже конца."))),
    );

    if (this.confirm) {
      root.child(
        h_flex()
          .px(14)
          .pb(8)
          .gap(12)
          .items_center()
          .text_size(TEXT.small)
          .child(div().flex_1().min_w(0).whitespace_normal().child("Период по проекту закроется: правила и правки потом не изменят выгрузку, пока его не переоткроют."))
          .child(Link({ id: "rep-confirm-no", label: "Отмена", color: t.muted, onClick: (cx) => ((this.confirm = null), cx.notify()) }))
          .child(Link({ id: "rep-confirm-yes", label: "Сохранить", onClick: (cx) => this.save(this.confirm, cx) })),
      );
    } else if (preview) {
      root.child(
        h_flex()
          .px(14)
          .pb(8)
          .gap(16)
          .child(Link({ id: "rep-save-csv", label: "Сохранить CSV", onClick: (cx) => this.ask("csv", cx) }))
          .child(Link({ id: "rep-save-md", label: "Сохранить Markdown", onClick: (cx) => this.ask("md", cx) })),
      );
    }
    if (this.message) root.child(div().px(14).pb(8).text_size(TEXT.small).text_color(this.message.error ? t.ansi1 : t.muted).whitespace_normal().child(this.message.text));
    return root.child(this.closed(t));
  }

  closed(t) {
    const box = v_flex().px(14).pt(10).gap(2).child(div().h(22).text_size(TEXT.small).text_color(t.muted).child("Закрытые периоды"));
    if (!this.periods.length) return box.child(div().text_size(TEXT.small).text_color(t.dim).child("Выгрузок пока не было."));
    for (const p of this.periods) {
      box.child(
        h_flex()
          .h(24)
          .gap(12)
          .items_center()
          .text_size(TEXT.small)
          .child(div().w(150).flex_none().child(`${label(p.from - this.dayStart)} – ${label(p.to - 1 - this.dayStart)}`))
          .child(div().flex_1().min_w(0).text_color(p.delta_ms ? t.ansi3 : t.dim).child(p.delta_ms ? `правила изменили бы ${minutes(p.delta_ms)}` : "без расхождений"))
          .child(Link({ id: `rep-reopen-${p.id}`, label: "Переоткрыть", onClick: (cx) => this.reopen(p.id, cx) })),
      );
    }
    return box;
  }
}
