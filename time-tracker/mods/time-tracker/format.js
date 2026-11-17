// Pure helpers: no imports, so tests load this file as it is.

export function formatDuration(ms) {
  const m = Math.floor(Math.max(0, ms) / 60000);
  const h = Math.floor(m / 60);
  if (!m) return "—";
  if (!h) return `${m}м`;
  return m % 60 ? `${h}ч ${m % 60}м` : `${h}ч`;
}

// "7 переключений": the form for n, after n.
export function plural(n, one, few, many) {
  const d = n % 10, dd = n % 100;
  return `${n} ${d === 1 && dd !== 11 ? one : d >= 2 && d <= 4 && (dd < 12 || dd > 14) ? few : many}`;
}

const WEEKDAYS = ["вс", "пн", "вт", "ср", "чт", "пт", "сб"];
export const MONTHS = ["января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"];

// "2026-11-16" → "пн, 16 ноября".
export function dayTitle(day) {
  const [y, m, d] = day.split("-").map(Number);
  return `${WEEKDAYS[new Date(y, m - 1, d).getDay()]}, ${d} ${MONTHS[m - 1]}`;
}

// The other projects in a stretch: "+ Учёба 4м, другое 7м"; under a minute is left out.
export function shareText(shares, name) {
  const parts = shares.filter((s) => s.me >= 60000).map((s) => `${s.project ? name(s.project) : "другое"} ${formatDuration(s.me)}`);
  return parts.length ? `+ ${parts.join(", ")}` : "";
}

// The built-in kinds of activity, for a collector that doesn't list its own.
export const ACTIVITIES = [
  { id: "development", label: "Разработка" },
  { id: "architecture", label: "Проектирование" },
  { id: "design", label: "Дизайн" },
  { id: "documentation", label: "Документация" },
  { id: "study", label: "Учёба" },
];
const byId = (list, id) => list.find((a) => a.id === id);
export const activityLabel = (id, list = ACTIVITIES) => (id === "" ? "Без вида" : (byId(list, id) ?? byId(ACTIVITIES, id))?.label ?? id ?? "");

// What `projects` gave: the projects and the kinds of activity, the built-in ones when it lists none.
export function catalog(r) {
  if (!Array.isArray(r?.projects)) return null;
  return { projects: r.projects, activities: Array.isArray(r.activities) && r.activities.length ? r.activities : ACTIVITIES };
}

// What `herdr-time ... --json` printed: exit code 3 means the collector isn't running. An error keeps
// its other fields (a closed period's id).
export function parseResult(r) {
  if (r.code === 3) return { down: true };
  try {
    const out = JSON.parse(r.stdout);
    if (r.code === 0) return out;
    return { ...out, error: out.error ?? `exit ${r.code}` };
  } catch {
    return { error: (r.stderr || r.stdout || `exit ${r.code}`).trim() };
  }
}

const pad2 = (n) => String(n).padStart(2, "0");
export const hhmm = (ms) => `${pad2(new Date(ms).getHours())}:${pad2(new Date(ms).getMinutes())}`;
export const dayString = (ms) => {
  const d = new Date(ms);
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
};

// The local midnight of a "YYYY-MM-DD" day (calendar steps, so a DST change doesn't shift it).
export const midnight = (day, plusDays = 0) => {
  const [y, m, d] = day.split("-").map(Number);
  return new Date(y, m - 1, d + plusDays).getTime();
};

// "9:05" → 545 minutes after midnight; anything else than H:MM / HH:MM within the day is null.
export function parseClock(s) {
  const m = /^(\d{1,2}):(\d{2})$/.exec(s.trim());
  return m && +m[1] < 24 && +m[2] < 60 ? +m[1] * 60 + +m[2] : null;
}

// Two clocks on a day that starts dayStartMs after midnight, as ms; a clock before that start is the
// night after (the day runs 06:00 → 06:00). null when the end doesn't come after the start.
export function manualRange(day, dayStartMs, fromMin, toMin) {
  const at = (min) => midnight(day, min * 60_000 < dayStartMs ? 1 : 0) + min * 60_000;
  const start = at(fromMin), end = at(toMin);
  return end > start ? { start, end } : null;
}

// My recorded time ([{start, end}]) inside [start, end].
export const overlapMs = (me, start, end) => me.reduce((n, s) => n + Math.max(0, Math.min(s.end, end) - Math.max(s.start, start)), 0);

// Monday to the next Monday, the day starting at dayStartMs after midnight; offset counts weeks back (-) or on.
export function weekRange(day, dayStartMs, offset = 0) {
  const back = ((new Date(midnight(day)).getDay() + 6) % 7) - 7 * offset;
  return { from: midnight(day, -back) + dayStartMs, to: midnight(day, 7 - back) + dayStartMs };
}

export function monthRange(day, dayStartMs = 4 * 3600_000, offset = 0) {
  const [y, m] = day.split("-").map(Number);
  return { from: new Date(y, m - 1 + offset, 1).getTime() + dayStartMs, to: new Date(y, m + offset, 1).getTime() + dayStartMs };
}

// The line under a session's title: how it began and when it ends.
export const sessionNote = (manual, minutes) => `${manual ? "начата вручную" : "началась сама после простоя"} · закончится после ${minutes} мин без активности`;

export const MONTH_NAMES = ["январь", "февраль", "март", "апрель", "май", "июнь", "июль", "август", "сентябрь", "октябрь", "ноябрь", "декабрь"];

// Goals and balance: minutes in, wording out.
export const fmtMin = (min) => formatDuration(min * 60000);

// "+3ч 29м" / "−3ч 29м" (U+2212); under a minute "0".
export const signed = (min) => (Math.abs(min) < 1 ? "0" : `${min < 0 ? "−" : "+"}${fmtMin(Math.abs(min))}`);

// "6,5" / "6.5" hours → whole minutes; not a positive number → null.
export function parseHours(s) {
  if (!/^\d+([.,]\d+)?$/.test(String(s).trim())) return null;
  return Math.round(parseFloat(String(s).trim().replace(",", ".")) * 60) || null;
}

// 390 → "6,5", 480 → "8", 375 → "6,25".
export const hoursText = (min) => String(Number((min / 60).toFixed(2))).replace(".", ",");

// The version in force on a day: the greatest `from` ≤ day, whatever the order; an `off` one is none.
export function goalOn(goals, day) {
  let g = null;
  for (const v of goals) if (v.from <= day && (!g || v.from >= g.from)) g = v;
  return g && !g.off ? g : null;
}

const PERIOD_SHORT = { day: "д", week: "нед", month: "мес" };
const PERIOD_LINE = { day: "день", week: "нед.", month: "мес." };
const PERIOD_IN = { day: "в день", week: "в неделю", month: "в месяц" };

// "6,5ч / д", "≤ 40ч / мес".
export const goalShort = (g) => `${g.dir === "max" ? "≤ " : ""}${hoursText(g.minutes)}ч / ${PERIOD_SHORT[g.period]}`;

// "≥ 6ч 30м / раб. день".
export const goalLine = (g) => `${g.dir === "max" ? "≤" : "≥"} ${fmtMin(g.minutes)} / ${g.period === "day" && g.calendar ? "раб. день" : PERIOD_LINE[g.period]}`;

// "с 1 ноября 2026 — не меньше 6,5ч в день".
export function goalVersion(g) {
  const [y, m, d] = g.from.split("-").map(Number);
  const from = `с ${d} ${MONTHS[m - 1]} ${y}`;
  return g.off ? `${from} — цели нет` : `${from} — ${g.dir === "max" ? "не больше" : "не меньше"} ${hoursText(g.minutes)}ч ${PERIOD_IN[g.period]}`;
}

// A week's worth of goals, minutes: month goals as 12 / 52 of a month.
export const weeklyTotal = (goals) => goals.reduce((n, g) => n + (g.period === "day" ? g.minutes * (g.weekdays ?? []).length : g.period === "week" ? g.minutes : Math.round((g.minutes * 12) / 52)), 0);

export function progressText(goal, fact, plan) {
  if (goal.dir === "max") return fact <= plan ? "в пределах цели" : `сверх цели на ${fmtMin(fact - plan)}`;
  return fact >= plan ? "цель выполнена" : `не хватает ${fmtMin(plan - fact)}`;
}

// One `goals` entry as a right-panel or card row.
export function goalBlock(info) {
  const { goal, day, agents } = info;
  const ag = agents >= 1 ? `агенты ${fmtMin(agents)}` : null;
  const fill = (s) => (s.plan ? Math.min(100, (s.fact / s.plan) * 100) : null);
  if (goal.period === "day") {
    return { value: fmtMin(day.me), goal: day.plan ? fmtMin(day.plan) : null, fill: fill(day), sub: day.plan ? [signed(day.balance), ag].filter(Boolean).join(" · ") : ag };
  }
  const [s, short, word] = goal.period === "week" ? [info.week, "нед.", "неделю"] : [info.month, "мес.", "месяц"];
  return { value: fmtMin(day.me), goal: `${fmtMin(s.plan)} в ${short}`, fill: fill(s), sub: `за ${word} ${fmtMin(s.fact)}` };
}

// "прогноз ноября по Universe ≈ 81ч 41м из 130ч".
export const forecastLine = (info, name, day) => `прогноз ${MONTHS[Number(day.slice(5, 7)) - 1]} по ${name} ≈ ${fmtMin(info.month.forecast)} из ${fmtMin(info.month.plan)}`;

// Whole rubles as "100 000 ₽", plain spaces so it pastes cleanly.
export const money = (rub) => `${String(rub).replace(/\B(?=(\d{3})+$)/g, " ")} ₽`;

// "100 000" / "15000" (a no-break or narrow space between groups too) → whole rubles 1..100 000 000, as the
// collector takes them; anything else → null.
export function parseMoney(s) {
  const t = String(s).replace(/[\u00a0\u202f]/g, " ").trim();
  const n = /^\d[\d ]*$/.test(t) ? Number(t.replace(/ /g, "")) : 0;
  return n >= 1 && n <= 100_000_000 ? n : null;
}

// The pay version in force in a month ("YYYY-MM"); a `none` one is none.
export const payOn = (pays, month) => goalOn((pays ?? []).map((p) => ({ ...p, off: p.kind === "none" })), month);

// Проекты's pay line: "100 000 ₽ / мес + премии", "2 000 ₽ / ч"; no pay → null.
export const payShort = (p) => (p?.kind === "fixed" ? `${money(p.amount)} / мес + премии` : p?.kind === "hourly" ? `${money(p.amount)} / ч` : null);

// The goal dialog's writes: `goal set` when its goal controls differ from when it opened, `pay set` when its
// pay controls do. opened, now: { goal: [dir, hours, period, weekdays, calendar, weekends, from], pay: [kind, sum] }.
export function dialogWrites(opened, now) {
  const differ = (k) => JSON.stringify(opened[k]) !== JSON.stringify(now[k]);
  return { goal: differ("goal"), pay: differ("pay") };
}

// "2026-11" → "ноября 2026".
export const monthOf = (month) => `${MONTHS[Number(month.slice(5, 7)) - 1]} ${month.slice(0, 4)}`;

const monthTitle = (i) => MONTH_NAMES[i][0].toUpperCase() + MONTH_NAMES[i].slice(1);

// The n months up to today's, newest first: [{ id: "2026-11", label: "Ноябрь 2026" }, …].
export function monthOptions(today, n = 12) {
  const [y, m] = today.split("-").map(Number);
  return Array.from({ length: n }, (_, i) => {
    const d = new Date(y, m - 1 - i, 1);
    return { id: `${d.getFullYear()}-${pad2(d.getMonth() + 1)}`, label: `${monthTitle(d.getMonth())} ${d.getFullYear()}` };
  });
}

// Отчёт's goal strip: "6ч 30м в рабочий день" and what counts toward it.
export function goalStrip(g) {
  const per = g.period === "day" && g.calendar ? "в рабочий день" : PERIOD_IN[g.period];
  const weekends = g.period === "day" && g.weekends;
  return { text: `${g.dir === "max" ? "не больше " : ""}${fmtMin(g.minutes)} ${per}`, sub: weekends ? "активное время · выходные в зачёт" : "активное время" };
}

// The note under Отчёт's table: the calendar and weekends for a day goal, the agents with their column.
export function reportFootnote(rep, agents) {
  const g = rep.goal, parts = [];
  if (g?.period === "day" && g.calendar) {
    const days = (rep.holidays ?? []).map((d) => Number(d.slice(8)));
    const month = rep.holidays?.length ? MONTHS[Number(rep.holidays[0].slice(5, 7)) - 1] : "";
    const list = days.length > 1 ? `${days.slice(0, -1).join(", ")} и ${days[days.length - 1]}` : String(days[0]);
    parts.push(`Рабочие дни — по производственному календарю${days.length ? ` (${list} ${month} — ${days.length > 1 ? "праздники" : "праздник"})` : ""}.`);
  }
  if (g?.period === "day") parts.push(g.weekends ? "Время в выходные идёт в зачёт." : "Время в выходные в цель не идёт.");
  if (agents) parts.push("Агенты в разницу не входят.");
  return parts.join(" ");
}

// The CSV's file name from the project id, without what Windows forbids in a name.
export const exportFile = (project, month) => `${project.replace(/[<>:"/\\|?*]/g, "")}_${month}.csv`;

// Each row's `me` (ms) rounded half up to whole minutes, the total their sum.
export function roundRows(rows) {
  const out = rows.map((r) => ({ ...r, me: Math.floor(r.me / 60000 + 0.5) * 60000 }));
  return { rows: out, total: out.reduce((n, r) => n + r.me, 0) };
}

export const SHORT_MONTHS = ["янв", "фев", "мар", "апр", "мая", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"];
const dayMonth = (ms) => [new Date(ms).getDate(), SHORT_MONTHS[new Date(ms).getMonth()]];

// "17 ноя, 00:40".
export const stamp = (ms) => `${dayMonth(ms).join(" ")}, ${hhmm(ms)}`;

// The days a rule change covered, from its start to when it was made, days starting dayStartMs after
// midnight: "16 ноя", "1–15 ноя", "28 окт – 3 ноя".
export function appliedRange(from, at, dayStartMs) {
  const [d1, m1] = dayMonth(from - dayStartMs), [d2, m2] = dayMonth(at - dayStartMs);
  if (m1 !== m2) return `${d1} ${m1} – ${d2} ${m2}`;
  return d1 === d2 ? `${d1} ${m1}` : `${d1}–${d2} ${m1}`;
}

// Days from..to (YYYY-MM-DD) with month names: "23–27 ноя" (dash between days of one month), "30 ноя – 2 дек", "23 ноя".
function daySpan(from, to, names, dash) {
  const dm = (day) => [Number(day.slice(8)), names[Number(day.slice(5, 7)) - 1]];
  const [d1, m1] = dm(from), [d2, m2] = dm(to);
  if (m1 !== m2) return `${d1} ${m1} – ${d2} ${m2}`;
  return d1 === d2 ? `${d1} ${m1}` : `${d1}${dash}${d2} ${m2}`;
}

export const OFF_KINDS = [{ id: "otgul", label: "отгул" }, { id: "vacation", label: "отпуск" }, { id: "sick", label: "больничный" }];
export const offLabel = (kind) => OFF_KINDS.find((k) => k.id === kind)?.label ?? kind;

// "23 – 27 ноября", "23 ноября", "30 ноября – 2 декабря".
export const offTitle = (from, to) => daySpan(from, to, MONTHS, " – ");

export const workdaysText = (n) => plural(n, "раб. день", "раб. дня", "раб. дней");

// How much plan marking these `plan` days off takes from a project, whole minutes.
export const offReduce = (days, project) => Math.round(days.reduce((n, d) => n + (d.plan?.[project] ?? 0), 0));

const ENTRY_KIND = { project: "assign", exclude: "excl", activity: "activity", note: "note", add: "manual" };

// История правок: corrections, rule, goal and pay changes and days off as rows, newest first; change kinds it
// doesn't know are left out. opts: name(project id), activity(id), ruleText(rule), dayStartMs.
export function historyEntries({ corrections = [], changes = [], days_off = [] }, { name, activity, ruleText, dayStartMs }) {
  const span = (c) => `${hhmm(c.start)}–${hhmm(c.end)}`;
  const manual = (note) => (note ? `вручную · ${note}` : "вручную");
  const fromCorrection = (c) => {
    const was = (c.was ?? []).filter((s) => s.me >= 60000).map((s) => `${s.project ? name(s.project) : "Другое"} ${formatDuration(s.me)}`);
    const sub = was.length ? `вручную · было: ${was.join(", ")}` : "вручную";
    const project = c.project ? name(c.project) : "Без проекта";
    const title = {
      project: () => `${span(c)} → ${project}`,
      exclude: () => `${span(c)} → Не работа`,
      activity: () => `${span(c)} → ${activity(c.activity)}`,
      note: () => `${span(c)} · заметка`,
      add: () => `+ ${span(c)} · ${project}`,
    }[c.kind]?.() ?? span(c);
    const custom = c.kind === "note" || c.kind === "add";
    return { key: `c:${c.id}`, kind: ENTRY_KIND[c.kind] ?? "note", title, sub: custom ? manual(c.note) : sub, at: c.made || c.start, undo: ["correct", "rm", String(c.id)] };
  };
  const fromGoal = (h) => {
    const g = h.goal, [y, m, d] = g.from.split("-").map(Number);
    const was = goalOn(h.prev_goals ?? [], g.from);
    const title = g.off ? `Цель ${name(h.project)} убрана` : `Цель ${name(h.project)}: ${goalLine(g)}`;
    const from = `с ${d} ${MONTHS[m - 1]}${y === new Date(h.at).getFullYear() ? "" : ` ${y}`}`;
    return { key: `h:${h.id}`, kind: "goal", title, sub: `${from} · ${was ? `было: ${goalLine(was)}` : "новая"}`, at: h.at, undo: ["history", "undo", String(h.id)] };
  };
  const fromPay = (h) => {
    const p = h.pay, was = payOn(h.prev_pays, p.from);
    const title = p.kind === "none" ? `Оплата ${name(h.project)} снята` : `Оплата ${name(h.project)}: ${payShort(p)}`;
    return { key: `h:${h.id}`, kind: "pay", title, sub: `с ${monthOf(p.from)}${was ? ` · было: ${payShort(was)}` : p.kind === "none" ? "" : " · новая"}`, at: h.at, undo: ["history", "undo", String(h.id)] };
  };
  const fromChange = (h) => {
    if (h.kind === "goal-set") return fromGoal(h);
    if (h.kind === "pay-set") return fromPay(h);
    if (!h.kind?.startsWith("rule-")) return null;
    const range = `применено к ${appliedRange(h.from, h.at, dayStartMs)}`;
    const ms = (plus) => (h.ms >= 60000 ? ` · ${plus}${formatDuration(h.ms)}` : "");
    const rule = h.rule ?? h.prev;
    const target = name(rule.project);
    let title = `Правило ${target}: ${ruleText(rule)}`, sub;
    if (h.kind === "rule-delete") sub = "удалено";
    else if (h.source === "inbox") (title = `${ruleText(rule)} → ${target}`), (sub = `правило из разбора · ${range}${ms("")}`);
    else sub = `${h.kind === "rule-edit" ? "изменено" : "новое"} · ${range}${ms("+")}`;
    return { key: `h:${h.id}`, kind: "rule", title, sub, at: h.at, undo: ["history", "undo", String(h.id)] };
  };
  const fromDayOff = (o) => ({
    key: `o:${o.id}`,
    kind: "dayoff",
    title: `${daySpan(o.from, o.to, SHORT_MONTHS, "–")} · ${offLabel(o.kind)}`,
    sub: o.projects?.length ? `план снижен: ${o.projects.map(name).join(", ")}` : "план не менялся",
    at: o.made,
    undo: ["dayoff", "rm", String(o.id)],
  });
  return [...corrections.map(fromCorrection), ...changes.map(fromChange).filter(Boolean), ...days_off.map(fromDayOff)].sort((a, b) => b.at - a.at);
}

// The seven days, Monday to Sunday, of the week around a day.
export function weekDays(day) {
  const back = (new Date(midnight(day)).getDay() + 6) % 7;
  return [0, 1, 2, 3, 4, 5, 6].map((i) => dayString(midnight(day, i - back)));
}

const dayMonthOf = (day) => [Number(day.slice(8)), MONTHS[Number(day.slice(5, 7)) - 1]];

// "9 – 15 ноября", "30 ноября – 6 декабря".
export function weekTitle(days) {
  const [d1, m1] = dayMonthOf(days[0]), [d2, m2] = dayMonthOf(days[6]);
  return m1 === m2 ? `${d1} – ${d2} ${m2}` : `${d1} ${m1} – ${d2} ${m2}`;
}

export function weekRelative(days, today) {
  if (weekDays(today)[0] === days[0]) return "эта неделя";
  return weekDays(dayString(midnight(today, -7)))[0] === days[0] ? "прошлая неделя" : "";
}

// The project whose day goal the Week's «цель дня» shows: the first in order with one.
export const mainGoal = (goals, order) => order.find((id) => goals?.[id]?.period === "day") ?? null;

// One day's «цель дня»: fact against the plan, or time on an unplanned day; nothing after today.
export function dayDiff(day, project, today) {
  if (day.day > today) return { text: "" };
  const plan = day.plan?.[project] ?? 0, fact = day.fact?.[project] ?? 0;
  if (plan > 0) return { text: signed(fact - plan), low: fact - plan <= -1 };
  return { text: fact > 0 ? signed(fact) : "—" };
}

export const sumOf = (m) => Object.values(m ?? {}).reduce((n, v) => n + v, 0);

// The Week's rows from `plan`: each day's minutes as the collector rounded them, and the totals up to today.
export function weekRows(plan, main) {
  const rows = plan.days.map((d) => {
    const [y, m, dd] = d.day.split("-").map(Number);
    return {
      day: d.day,
      label: `${WEEKDAYS[new Date(y, m - 1, dd).getDay()]} ${dd}`,
      weekend: !d.work,
      me: sumOf(d.me),
      agents: sumOf(d.agents),
      diff: main ? dayDiff(d, main, plan.today) : { text: "" },
    };
  });
  const past = rows.filter((r) => r.day <= plan.today);
  const totals = {
    me: past.reduce((n, r) => n + r.me, 0),
    agents: past.reduce((n, r) => n + r.agents, 0),
    diff: main ? signed(plan.sums?.[main]?.balance ?? 0) : "",
  };
  return { rows, totals };
}

// The 42 cells of a month's calendar, Monday first: the month's days, null around them.
export function monthGrid(day) {
  const first = `${day.slice(0, 8)}01`;
  const lead = (new Date(midnight(first)).getDay() + 6) % 7;
  const month = first.slice(0, 7);
  return Array.from({ length: 42 }, (_, i) => {
    const d = dayString(midnight(first, i - lead));
    return d.startsWith(month) ? d : null;
  });
}

// A Month cell's bar on the 8 h scale: my minutes per project (order, then ids it lacks, then Другое), the
// main project's day goal as a tick and, on days to come, its forecast pace.
export function cellBars(day, order, main, sum, today) {
  const me = day.me ?? {};
  const ids = [...order, ...Object.keys(me).filter((id) => id && !order.includes(id)), ""];
  const parts = ids.filter((id) => me[id] > 0).map((project) => ({ project, pct: (me[project] / 480) * 100 }));
  const plan = (main && day.plan?.[main]) || 0;
  const tick = plan > 0 ? (plan / 480) * 100 : null;
  const forecast = day.day > today && plan > 0 && sum?.plan_to_date > 0 ? ((plan * sum.fact) / sum.plan_to_date / 480) * 100 : null;
  return { parts, tick, forecast };
}
