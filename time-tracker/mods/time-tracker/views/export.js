// Saving a project's month: the CSV goes to Downloads as the collector writes it.
import { home } from "herdr";
import { call } from "../client.js";
import { exportFile } from "../format.js";

// Writes {home}/Downloads/{project}_{month}.csv with the rows as shown; the answer is the panel's message.
export async function saveCSV({ project, month, rows, agents }) {
  const path = `${home().replace(/\\/g, "/")}/Downloads/${exportFile(project, month)}`;
  const args = ["export", "--project", project, "--month", month, "--rows", rows, "--format", "csv", "--notes", "--pay", "--goal", "--out", path];
  if (agents) args.push("--agents");
  const x = await call(args);
  if (x.down) return { text: "Сборщик не запущен", error: true };
  if (x.error) return { text: x.error, error: true };
  return { text: `Сохранено: ${path}` };
}
