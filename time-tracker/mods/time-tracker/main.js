// Time Tracker: the status-bar chip (props.view "status") and the panel (default).
// Herdr's signals go to the collector from the chip, which a layout shows all the time.
import { View, div } from "gpui-kit";
import { props, checking } from "herdr";
import { Status } from "./views/status.js";
import { Day } from "./views/day.js";
import { SessionView } from "./views/session.js";
import { Table } from "./views/table.js";
import { Report } from "./views/report.js";
import { theme } from "herdr";
import { Panel, PanelHeader, HeaderTab, Icon, Placeholder } from "./ui.js";

const TABS = [["session", "Сессия"], ["day", "День"], ["week", "Неделя"], ["month", "Месяц"], ["report", "Отчёт"]];
const PROJECTS = ["Проекты", "Список проектов, правила и цвета появятся здесь в следующем обновлении. Пока проекты и правила задаются в projects.json (%LOCALAPPDATA%\\herdr-time\\projects.json), сборщик перечитывает его сам."];
const SETTINGS = ["Настройки", "Экран настроек появится позже. Пока они в settings.json рядом с projects.json: session_idle_minutes (45), pause_minutes (15), day_start (\"06:00\")."];
const placeholder = ([title, text]) => ({ tick() {}, render: () => Placeholder(title, text) });

export default class TimeTracker extends View {
  init(given, cx) {
    // `herdr-app check` runs init for real: start nothing then.
    if (props()?.view === "status") {
      this.status = new Status();
      if (!checking()) this.status.init(given, cx);
      return;
    }
    this.tab = "session";
    const openDay = (date, cx) => {
      this.tab = "day";
      this.parts.day.show(date === this.parts.day.today ? null : date, cx); // loads once; the tick finds it fresh
      cx.notify();
    };
    const open = (tab, cx) => this.open(tab, cx);
    this.parts = { session: new SessionView(open), projects: placeholder(PROJECTS), settings: placeholder(SETTINGS), day: new Day(open), week: new Table("week", openDay), month: new Table("month", openDay), report: new Report() };
    if (checking()) return;
    this.tick(cx);
    cx.timer.every(5000, (cx) => this.tick(cx));
  }

  // Only the tab on screen reads anything.
  tick(cx) {
    this.parts[this.tab].tick(cx);
  }

  open(tab, cx) {
    this.tab = tab;
    this.parts[tab].wasShown = false; // read again on coming back
    this.tick(cx);
    cx.notify();
  }

  render(cx) {
    if (this.status) return this.status.render(cx);
    const t = theme();
    const button = (id, label) => {
      const on = this.tab === id;
      return div()
        .id(`time-tab-${id}`)
        .flex_none()
        .flex()
        .items_center()
        .rounded(7)
        .border(1)
        .border_color(on ? t.border : t.background)
        .bg(on ? t.raised : t.background)
        .text_color(on ? t.foreground : t.muted)
        .accessibility_label(label)
        .cursor_pointer()
        .hover((s) => s.bg(t.hover))
        .on_click((_e, cx) => this.open(id, cx));
    };
    return Panel({ id: "time-root" })
      .child(
        PanelHeader()
          .children(TABS.map(([id, label]) => HeaderTab({ id: `time-tab-${id}`, label, active: id === this.tab, dot: id === "session" ? t.ansi2 : undefined, onClick: (cx) => this.open(id, cx) })))
          .child(div().flex_1())
          .child(button("projects", "Проекты").px(10).py(6).gap(6).text_size(13).child(Icon("settings-2", 15)).child("Проекты"))
          .child(button("settings", "Настройки").w(32).h(30).justify_center().child(Icon("settings", 15)).tooltip("Настройки")),
      )
      .child(this.parts[this.tab].render(cx));
  }
}
