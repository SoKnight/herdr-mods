// Time Tracker: the status-bar chip (props.view "status") and the panel (default).
// Herdr's signals go to the collector from the chip, which a layout shows all the time.
import { View } from "gpui-kit";
import { props, checking } from "herdr";
import { Status } from "./views/status.js";
import { Day } from "./views/day.js";
import { Table } from "./views/table.js";
import { Report } from "./views/report.js";
import { Panel, PanelHeader, HeaderTab } from "./ui.js";

const TABS = [["day", "День"], ["week", "Неделя"], ["month", "Месяц"], ["report", "Отчёт"]];

export default class TimeTracker extends View {
  init(given, cx) {
    // `herdr-app check` runs init for real: start nothing then.
    if (props()?.view === "status") {
      this.status = new Status();
      if (!checking()) this.status.init(given, cx);
      return;
    }
    this.tab = "day";
    const openDay = (date, cx) => {
      this.parts.day.show(date === this.parts.day.today ? null : date, cx);
      this.open("day", cx);
    };
    this.parts = { day: new Day(), week: new Table("week", openDay), month: new Table("month", openDay), report: new Report() };
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
    return Panel({ id: "time-root" })
      .child(PanelHeader().children(TABS.map(([id, label]) => HeaderTab({ id: `time-tab-${id}`, label, active: id === this.tab, onClick: (cx) => this.open(id, cx) }))))
      .child(this.parts[this.tab].render(cx));
  }
}
