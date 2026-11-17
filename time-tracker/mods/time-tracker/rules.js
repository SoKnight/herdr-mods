// Rules and projects as the Проекты screen shows them. Pure: no imports, tests load it as it is.

export const RULE_KINDS = [
  { id: "folder", label: "Папка", icon: "folder" },
  { id: "process", label: "Процесс", icon: "app-window" },
  { id: "title", label: "Заголовок", icon: "type" },
  { id: "figma", label: "Figma", icon: "pen-tool" },
  { id: "minecraft", label: "Minecraft", icon: "gamepad-2" },
];

export const COLORS = ["ansi1", "ansi2", "ansi3", "ansi4", "ansi5", "ansi6"];

const MONTHS = ["января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"];
const trimSlash = (path) => path.replace(/[\\/]+$/, "");

// "D:\Work\Universe\**", "idea64.exe · «universe-»"; long spells the kind out: "папка D:\Work\Universe\**".
export function ruleText(rule, long = false) {
  const v = rule.value;
  switch (rule.kind) {
    case "folder":
      return `${long ? "папка " : ""}${trimSlash(v)}\\**`;
    case "process":
      return `${long ? "процесс " : ""}${v}${rule.title ? ` · ${long ? "заголовок содержит " : ""}«${rule.title}»` : ""}`;
    case "title":
      return `${long ? "заголовок содержит " : ""}«${v}»`;
    case "figma":
      return `${long ? "Figma " : ""}«${v}»`;
    case "minecraft":
      return `Minecraft · ${v}`;
    default:
      return v;
  }
}

// The first colour no project in use has; all taken, the first one.
export function freeColor(projects) {
  const taken = new Set(projects.filter((p) => !p.archived).map((p) => p.color));
  return COLORS.find((c) => !taken.has(c)) ?? COLORS[0];
}

// Where a rule sends time: a project's name, Не работа or Досуг.
export function targetName(id, projects) {
  if (id === "exclude") return "Не работа";
  if (id === "leisure") return "Досуг";
  return projects.find((p) => p.id === id)?.name || id;
}

// A name for a new project from what a rule would match: a folder's last part, a server's host.
export function guessName(hint) {
  if (hint.kind === "folder") return trimSlash(hint.value).split(/[\\/]/).pop();
  if (hint.kind === "minecraft") return hint.value.replace(/:\d+$/, "");
  return hint.value;
}

// Whether deleting the rule moves time it decided this month (from the day "2026-11-16"), so it asks first.
export function deleteAsks(rule, day) {
  const [y, m] = day.split("-").map(Number);
  return rule.ms > 0 || !rule.from || rule.from < new Date(y, m - 1, 1).getTime();
}

// "2026-11-16" → "1 ноября".
export function monthStartTitle(day) {
  return `1 ${MONTHS[Number(day.split("-")[1]) - 1]}`;
}
