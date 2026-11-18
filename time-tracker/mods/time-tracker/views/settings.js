// The Настройки tab: the collector's settings.json as controls. Each control sends only the key it changed;
// a refusal shows under its section and puts the stored value back.
import { div } from "gpui-kit";
import { h_flex, v_flex, Input, InputState } from "gpui-base";
import { theme } from "herdr";
import { call } from "../client.js";
import { hhmm } from "../format.js";
import { Check, Icon, Message, TEXT } from "../ui.js";

const LIVE_MS = 60000; // a sample this fresh: the collector is working

export class SettingsView {
  constructor() {
    this.s = null; // the stored settings
    this.lastSample = 0;
    this.down = false;
    this.wasShown = false;
    this.errors = {}; // section → text
    this.adding = false;
    this.inputs = {
      session_idle_minutes: InputState.new({ value: "" }),
      pause_minutes: InputState.new({ value: "" }),
      day_start: InputState.new({ value: "" }),
    };
    for (const [key, state] of Object.entries(this.inputs)) {
      const save = (_e, cx) => this.saveInput(key, cx);
      state.on("submit", save);
      state.on("blur", save);
    }
    this.add = InputState.new({ placeholder: "KeePassXC.exe" });
    this.add.on("submit", (_e, cx) => this.addPattern(cx));
    this.add.on("blur", (_e, cx) => ((this.adding = false), this.add.set_value(""), cx.notify()));
  }

  // Reads the settings on coming to the tab; after that only the last sample, so typing is never overwritten.
  tick(cx) {
    const first = !this.wasShown || !this.s || this.down;
    cx.spawn(async (cx) => {
      const [st, set] = await Promise.all([call(["status"]), first ? call(["settings"]) : null]);
      this.lastSample = st.last_sample ?? this.lastSample;
      if (st.down) this.down = true;
      if (set) {
        this.down = Boolean(set.down || st.down);
        if (!this.down && !set.error) (this.take(set), (this.wasShown = true), (this.errors.time = null));
        else if (set.error) this.errors.time = set.error;
      }
      cx.notify();
    });
  }

  take(s) {
    this.s = s;
    this.syncInputs();
  }

  syncInputs() {
    for (const [key, state] of Object.entries(this.inputs)) state.set_value(String(this.s[key]));
  }

  // Sends one changed key; section names where a refusal shows.
  save(key, value, section, cx) {
    this.errors[section] = null;
    cx.notify();
    cx.spawn(async (cx) => {
      const r = await call(["settings", "set", JSON.stringify({ [key]: value })]);
      if (r.down) this.down = true;
      else if (r.error) {
        this.errors[section] = r.error;
        this.syncInputs();
      } else this.take(r);
      cx.notify();
    });
  }

  saveInput(key, cx) {
    if (!this.s) return;
    const text = this.inputs[key].value().trim();
    if (text === String(this.s[key])) return;
    if (key === "day_start") return this.save(key, text, "time", cx);
    if (!/^\d+$/.test(text)) {
      this.errors.time = "Минуты: целое число";
      this.syncInputs();
      return cx.notify();
    }
    this.save(key, Number(text), "time", cx);
  }

  addPattern(cx) {
    const text = this.add.value().trim();
    this.adding = false;
    this.add.set_value("");
    if (text && !this.s.hidden_titles.includes(text)) this.save("hidden_titles", [...this.s.hidden_titles, text], "hidden", cx);
    else cx.notify();
  }

  startAdding(cx) {
    this.adding = true;
    cx.notify();
    cx.timer.after(0, () => this.adding && this.add.focus()); // once the input is drawn
  }

  render() {
    const t = theme();
    const col = v_flex().id("settings").flex_1().min_w(0).min_h(0).overflow_y_scroll().pt(18).px(22).pb(18).gap(14);
    col.child(div().flex_none().text_size(15).font_semibold().child("Настройки"));
    if (this.down) return col.child(Message("Сборщик не отвечает"));
    if (!this.s) return col.child(Message(this.errors.time ?? "Загрузка…", Boolean(this.errors.time)));
    const head = (text, icon) =>
      h_flex().flex_none().gap(6).items_center().text_size(TEXT.small).text_color(t.dim).when(Boolean(icon), (el) => el.child(Icon(icon, 13))).child(text);
    const row = (label, ...right) =>
      h_flex().flex_none().h(42).gap(10).items_center().border_t(1).border_color(t.raised).child(div().flex_1().min_w(0).truncate().text_color(t.muted).child(label)).children(right);
    const box = (key, id, w) => Input.new(this.inputs[key]).id(id).flex_none().w(w).h(28).px(6).rounded(6).border_1().border_color(t.border).bg(t.background).text_size(TEXT.small).text_right();
    const unit = div().flex_none().text_color(t.dim).child("мин");
    const check = (key, label) => Check({ id: `settings-${key}`, checked: this.s[key], label, onClick: (cx) => this.save(key, !this.s[key], "time", cx) });
    const checks = (keys, section) =>
      keys.map(([key, label]) => Check({ id: `settings-${key}`, checked: this.s[key], label, onClick: (cx) => this.save(key, !this.s[key], section, cx) }));
    const err = (section) => this.errors[section] && Message(this.errors[section], true);

    const left = v_flex()
      .flex_1()
      .min_w(0)
      .gap(10)
      .child(head("Учёт времени"))
      .child(
        v_flex()
          .flex_none()
          .child(row("Сессия заканчивается после простоя", box("session_idle_minutes", "settings-idle", 52), unit))
          .child(row("Паузы в зачёт сессии до", box("pause_minutes", "settings-pause", 52), unit))
          .child(row("Сутки начинаются в", box("day_start", "settings-day-start", 82)))
          .child(row(check("video_activity", "видео со звуком на переднем плане — активность"))),
      )
      .when(Boolean(this.errors.time), (el) => el.child(err("time")))
      .child(head("Напоминания"))
      .child(
        v_flex()
          .flex_none()
          .gap(10)
          .children(checks([["autostart", "сборщик стартует вместе с Windows"], ["silent_warning", "сказать, если сборщик молчит 10 мин"], ["close_reminder", "в конце месяца напомнить закрыть его"]], "remind")),
      )
      .when(Boolean(this.errors.remind), (el) => el.child(err("remind")));

    const chip = (p) =>
      h_flex()
        .flex_none()
        .h(26)
        .pl(8)
        .pr(4)
        .gap(4)
        .items_center()
        .rounded(6)
        .bg(t.raised)
        .font_family("Consolas")
        .text_size(12)
        .child(p)
        .child(
          div()
            .id(`settings-hidden-${p}`)
            .size(18)
            .flex_none()
            .flex()
            .items_center()
            .justify_center()
            .rounded(4)
            .text_color(t.dim)
            .accessibility_label("Убрать")
            .cursor_pointer()
            .hover((s) => s.bg(t.hover).text_color(t.foreground))
            .on_click((_e, cx) => this.save("hidden_titles", this.s.hidden_titles.filter((x) => x !== p), "hidden", cx))
            .child(Icon("x", 12)),
        );
    const adder = this.adding
      ? Input.new(this.add).id("settings-hidden-add").flex_none().w(140).h(26).px(8).rounded(6).border_1().border_color(t.border).bg(t.background).font_family("Consolas").text_size(12)
      : div()
          .id("settings-hidden-new")
          .flex_none()
          .h(26)
          .px(8)
          .flex()
          .items_center()
          .rounded(6)
          .border_1()
          .border_dashed()
          .border_color(t.border)
          .text_size(TEXT.small)
          .text_color(t.dim)
          .cursor_pointer()
          .hover((s) => s.text_color(t.foreground))
          .on_click((_e, cx) => this.startAdding(cx))
          .child("+ добавить");

    const live = Date.now() - this.lastSample <= LIVE_MS;
    const dot = live ? t.ansi2 : t.ansi3;
    const right = v_flex()
      .flex_1()
      .min_w(0)
      .gap(10)
      .child(head("Не записывать заголовки окон", "eye-off"))
      .child(h_flex().flex_none().flex_wrap().gap(6).children([...this.s.hidden_titles.map(chip), adder]))
      .child(div().flex_none().text_size(TEXT.small).text_color(t.dim).child("время считается, текст заголовка не сохраняется"))
      .when(Boolean(this.errors.hidden), (el) => el.child(err("hidden")))
      .child(div().flex_none().h(8))
      .child(head("Устройства"))
      .child(
        h_flex()
          .flex_none()
          .h(46)
          .gap(10)
          .items_center()
          .border_t(1)
          .border_color(t.raised)
          .child(
            v_flex()
              .flex_1()
              .min_w(0)
              .child(div().truncate().child("Этот ПК · Windows"))
              .child(div().truncate().text_size(TEXT.small).text_color(t.dim).child(this.lastSample ? `данные до ${hhmm(this.lastSample)}` : "данных нет")),
          )
          .child(h_flex().flex_none().gap(6).items_center().text_size(TEXT.small).text_color(dot).child(div().size(6).flex_none().rounded(3).bg(dot)).child(live ? "работает" : "нет данных")),
      );

    return col.child(h_flex().flex_none().gap(32).items_start().child(left).child(right));
  }
}
