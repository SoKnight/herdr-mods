// Time Tracker: the status-bar chip (props.view "status") and the panel (default).
// Herdr's signals go to the collector from the chip, which a layout shows all the time.
import { View } from "gpui-kit";
import { props, checking } from "herdr";
import { Status } from "./views/status.js";
import { Panel } from "./ui.js";

export default class TimeTracker extends View {
  init(given, cx) {
    // `herdr-app check` runs init for real: start nothing then.
    this.part = props()?.view === "status" ? new Status() : null;
    if (!checking()) this.part?.init(given, cx);
  }

  render(cx) {
    // ponytail: the panel comes in a later task; an empty frame until then.
    return this.part ? this.part.render(cx) : Panel({ id: "time-root" });
  }
}
