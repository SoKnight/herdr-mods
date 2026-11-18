// Taking a project's month out: the CSV goes to Downloads as the collector writes it, and «Отчёт текстом»
// shows the collector's text for the clipboard. Neither renders the month itself.
import { div } from "gpui-kit";
import { h_flex, v_flex } from "gpui-base";
import { theme, home } from "herdr";
import { call } from "../client.js";
import { exportFile, MONTH_NAMES } from "../format.js";
import { Dialog, Segmented, Check, Button, Message, TEXT } from "../ui.js";

const DOWN = "Сборщик не запущен";
const ROWS = [{ id: "week", label: "по неделям" }, { id: "day", label: "по дням" }];

// The export's switches as CLI flags.
const flags = ({ commits, notes, pay, goal, agents }) => [commits && "--commits", notes && "--notes", pay && "--pay", goal && "--goal", agents && "--agents"].filter(Boolean);

// Writes {home}/Downloads/{project}_{month}.csv with the rows and parts asked for (Отчёт: notes, pay and goal,
// agents as its column shows them; commits only when asked); the answer is the panel's message.
export async function saveCSV({ project, month, rows, commits, notes = true, pay = true, goal = true, agents }) {
  const path = `${home().replace(/\\/g, "/")}/Downloads/${exportFile(project, month)}`;
  const x = await call(["export", "--project", project, "--month", month, "--rows", rows, "--format", "csv", ...flags({ commits, notes, pay, goal, agents }), "--out", path]);
  if (x.down) return { text: DOWN, error: true };
  if (x.error) return { text: x.error, error: true };
  return { text: `Сохранено: ${path}` };
}

export class ExportText {
  // Shows the dialog with showDialog(d | null, cx), which main.js gives the views, and reads the text.
  static open(showDialog, opts, cx) {
    const d = new ExportText({ ...opts, close: (cx) => showDialog(null, cx) });
    showDialog(d, cx);
    d.load(cx);
  }

  // project: its id; name: its name; month: "YYYY-MM"; rows: "week" | "day"; onDone(text, cx) after copying;
  // close(cx) takes the dialog away.
  constructor({ project, name, month, rows, onDone, close }) {
    Object.assign(this, { project, name, month, rows, onDone, closeDialog: close });
    this.commits = true;
    this.notes = true;
    this.pay = true;
    this.agents = false;
    this.goal = false;
    this.text = null; // the collector's text for these options
    this.error = null;
    this.message = null; // the CSV's answer: { text, error? }
    this.busy = false; // a CSV is being saved
    this.seq = 0;
    this.closed = false;
  }

  // The text again for the options as they are now; an answer to older options is dropped.
  load(cx) {
    const seq = ++this.seq;
    cx.spawn(async (cx) => {
      const x = await call(["export", "--project", this.project, "--month", this.month, "--rows", this.rows, "--format", "text", ...flags(this)]);
      if (seq !== this.seq || this.closed) return;
      this.error = x.down ? DOWN : (x.error ?? null);
      this.text = this.error ? null : (x.content ?? "");
      cx.notify();
    });
  }

  set(change, cx) {
    Object.assign(this, change, { message: null, text: null, error: null }); // Скопировать waits for the new text
    this.load(cx);
    cx.notify();
  }

  saveCSV(cx) {
    if (this.busy) return;
    this.busy = true;
    const opts = { project: this.project, month: this.month, rows: this.rows, commits: this.commits, notes: this.notes, pay: this.pay, goal: this.goal, agents: this.agents };
    cx.spawn(async (cx) => {
      const m = await saveCSV(opts);
      this.busy = false;
      if (this.closed) return;
      this.message = m;
      cx.notify();
    });
  }

  copy(cx) {
    if (this.text === null || this.closed) return;
    cx.write_to_clipboard(this.text);
    this.close(cx);
    this.onDone("Скопировано.", cx);
  }

  close(cx) {
    if (this.closed) return;
    this.closed = true;
    this.closeDialog(cx);
  }

  render() {
    const t = theme();
    const caption = (text) => div().flex_none().text_size(12).text_color(t.dim).child(text);
    const check = (key, label) => Check({ id: `export-${key}`, checked: this[key], label, onClick: (cx) => this.set({ [key]: !this[key] }, cx) }).text_size(TEXT.small);
    const options = v_flex()
      .w(170)
      .flex_none()
      .gap(8)
      .child(caption("Строки"))
      .child(Segmented({ id: "export-rows", options: ROWS, value: this.rows, onChange: (rows, cx) => this.set({ rows }, cx) }))
      .child(caption("Включить").mt(8))
      .child(check("commits", "коммиты"))
      .child(check("notes", "мои заметки"))
      .child(check("pay", "оплату"))
      .child(check("agents", "агентов"))
      .child(check("goal", "цель и разницу"));
    const preview = v_flex()
      .id("export-preview")
      .h(300)
      .overflow_y_scroll()
      .p(12)
      .rounded(8)
      .border_1()
      .border_color(t.border)
      .bg(t.background)
      .font_family("Consolas")
      .text_size(12)
      .line_height(19.2)
      .when(this.text === null, (el) => el.text_color(t.dim).child(this.error ? "" : "Загрузка…"))
      .when(this.text !== null, (el) => el.children(this.text.replace(/\n$/, "").split("\n").map((l) => div().whitespace_normal().child(l || " "))));
    const body = v_flex()
      .p(18)
      .gap(10)
      .child(
        h_flex()
          .gap(18)
          .items_start()
          .child(options)
          .child(v_flex().flex_1().min_w(0).child(preview)),
      )
      .when(Boolean(this.error), (el) => el.child(Message(this.error, true)))
      .when(Boolean(this.message), (el) => el.child(Message(this.message.text, this.message.error)));
    return Dialog({
      id: "export",
      width: 700,
      title: "Отчёт текстом",
      sub: div().min_w(0).truncate().text_color(t.muted).child(`${this.name} · ${MONTH_NAMES[Number(this.month.slice(5, 7)) - 1]}`),
      onClose: (cx) => this.close(cx),
      body,
      footer: [
        Button({ id: "export-csv", label: "CSV", icon: "download", onClick: (cx) => this.saveCSV(cx) }).px(14).py(6).text_size(TEXT.body),
        div().flex_1(),
        Button({ id: "export-cancel", label: "Отмена", onClick: (cx) => this.close(cx) }).px(14).py(6).text_size(TEXT.body),
        Button({ id: "export-copy", label: "Скопировать", icon: "copy", primary: true, onClick: (cx) => this.copy(cx) }).px(14).py(6).text_size(TEXT.body),
      ],
    });
  }
}
