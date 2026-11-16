// Time Tracker: the status-bar chip (props.view "status") and the panel (default).
// Herdr's signals go to the collector from the chip, which a layout shows all the time.
import { View } from "gpui-kit";
import { props, checking } from "herdr";
import { Status } from "./views/status.js";
import { Day } from "./views/day.js";
import { Panel, PanelHeader, HeaderTab } from "./ui.js";

// ponytail: only Day works so far; the other tabs come with their own tasks.
const TABS = [["day", "День"], ["week", "Неделя"], ["month", "Месяц"], ["report", "Отчёт"]];

export default class TimeTracker extends View {
  init(given, cx) {
    // `herdr-app check` runs init for real: start nothing then.
    this.part = props()?.view === "status" ? new Status() : new Day();
    if (!checking()) this.part.init(given, cx);
  }

  render(cx) {
    if (this.part instanceof Status) return this.part.render(cx);
    return Panel({ id: "time-root" })
      .child(PanelHeader().children(TABS.map(([id, label]) => HeaderTab({ id: `time-tab-${id}`, label, active: id === "day", onClick: () => {} }))))
      .child(this.part.render(cx));
  }
}
