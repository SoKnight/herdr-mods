const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const code = fs.readFileSync(path.join(__dirname, '../mods/time-tracker/format.js'), 'utf8');
const F = new Function(`${code.replace(/^export /gm, '')}\nreturn { formatDuration, activityLabel, parseResult, weekRange, monthRange, exportName, sessionNote, catalog, plural, dayTitle, shareText, stamp, appliedRange, historyEntries, parseClock, manualRange, overlapMs, fmtMin, signed, roundRows, parseHours, hoursText, goalOn, goalShort, goalLine, goalVersion, weeklyTotal, progressText, goalBlock, forecastLine, weekDays, weekTitle, weekRelative, mainGoal, dayDiff, weekRows, monthGrid, cellBars, offTitle, workdaysText, offReduce };`)();
const rcode = fs.readFileSync(path.join(__dirname, '../mods/time-tracker/rules.js'), 'utf8');
const R = new Function(`${rcode.replace(/^export /gm, '')}
return { ruleText, targetName };`)();

test('durations', () => {
  assert.equal(F.formatDuration(18_720_000), '5ч 12м');
  assert.equal(F.formatDuration(2_700_000), '45м');
  assert.equal(F.formatDuration(7_200_000), '2ч');
  assert.equal(F.formatDuration(0), '—');
  assert.equal(F.formatDuration(59_999), '—');
});

test('plural forms', () => {
  const p = (n) => F.plural(n, 'переключение', 'переключения', 'переключений');
  assert.deepEqual([0, 1, 3, 7, 11, 21, 22].map(p), ['0 переключений', '1 переключение', '3 переключения', '7 переключений', '11 переключений', '21 переключение', '22 переключения']);
});

test('day title', () => {
  assert.equal(F.dayTitle('2026-11-16'), 'пн, 16 ноября');
  assert.equal(F.dayTitle('2026-03-01'), 'вс, 1 марта');
});

test('share text', () => {
  const names = { study: 'Учёба', easy: 'EasyLauncher' };
  const name = (id) => names[id];
  assert.equal(F.shareText([{ project: 'study', me: 4 * 60_000 }, { project: '', me: 7 * 60_000 }, { project: 'easy', me: 30_000 }], name), '+ Учёба 4м, другое 7м');
  assert.equal(F.shareText([], name), '');
});

test('activity labels fall back to the id', () => {
  const labels = { development: 'Разработка', architecture: 'Проектирование', design: 'Дизайн', documentation: 'Документация', study: 'Учёба' };
  for (const [id, label] of Object.entries(labels)) assert.equal(F.activityLabel(id), label);
  assert.equal(F.activityLabel('custom'), 'custom');
  assert.equal(F.activityLabel(''), 'Без вида');
  const listed = [{ id: 'review', label: 'Ревью' }];
  assert.equal(F.activityLabel('review', listed), 'Ревью');
  assert.equal(F.activityLabel('study', listed), 'Учёба'); // dropped from the list, still named
});

test('the catalog falls back to the built-in kinds only when the collector lists none', () => {
  const projects = [{ id: 'easy', name: 'Easy' }];
  assert.deepEqual(F.catalog({ projects, activities: [{ id: 'review', label: 'Ревью' }] }), { projects, activities: [{ id: 'review', label: 'Ревью' }] });
  assert.equal(F.catalog({ projects }).activities.length, 5);
  assert.equal(F.catalog([{ id: 'easy' }]), null);
  assert.equal(F.catalog({ error: 'down' }), null);
});

test('down result', () => {
  assert.deepEqual(F.parseResult({ code: 3, stdout: '{"error":"down"}' }), { down: true });
  assert.deepEqual(F.parseResult({ code: 0, stdout: '{"a":1}' }), { a: 1 });
});

test('errors', () => {
  assert.deepEqual(F.parseResult({ code: 1, stdout: '{"error":"bad"}' }), { error: 'bad' });
  assert.deepEqual(F.parseResult({ code: 1, stdout: '', stderr: 'boom\n' }), { error: 'boom' });
  assert.deepEqual(F.parseResult({ code: 1, stdout: '{"error":"closed","period":4}' }), { error: 'closed', period: 4 });
});

const at = (d, h = 0) => new Date(d + 'T00:00:00').getTime() + h * 3600_000;
const DAY_START = 4 * 3600_000;

test('week starts on Monday', () => {
  assert.deepEqual(F.weekRange('2026-11-18', DAY_START), { from: at('2026-11-16', 4), to: at('2026-11-23', 4) });
  assert.equal(F.weekRange('2026-11-22', DAY_START).from, at('2026-11-16', 4)); // Sunday
  assert.equal(F.weekRange('2026-11-16', DAY_START).from, at('2026-11-16', 4));
});

test('month range', () => {
  assert.deepEqual(F.monthRange('2026-11-18'), { from: at('2026-11-01', 4), to: at('2026-12-01', 4) });
});

test('ranges step back and on by an offset', () => {
  assert.deepEqual(F.weekRange('2026-11-18', DAY_START, -1), { from: at('2026-11-09', 4), to: at('2026-11-16', 4) });
  assert.deepEqual(F.weekRange('2026-11-22', DAY_START, 1), { from: at('2026-11-23', 4), to: at('2026-11-30', 4) });
  assert.deepEqual(F.monthRange('2026-11-18', DAY_START, -1), { from: at('2026-10-01', 4), to: at('2026-11-01', 4) });
  assert.deepEqual(F.monthRange('2026-01-10', DAY_START, -1), { from: at('2025-12-01', 4), to: at('2026-01-01', 4) }); // across a year
});

test('export file name follows the collector days', () => {
  // end cut at now: a week in progress is named by its last day so far
  assert.equal(F.exportName('easy', at('2026-11-16', 4), at('2026-11-23', 4), 'md', DAY_START, at('2026-11-18', 10)), 'easy_2026-11-16_2026-11-18.md');
  // just after 04:00 the new day has begun
  assert.equal(F.exportName('easy', at('2026-11-16', 4), at('2026-11-23', 4), 'md', DAY_START, at('2026-11-18', 3)), 'easy_2026-11-16_2026-11-17.md');
});

test('export file name', () => {
  assert.equal(F.exportName('easylauncher', at('2026-11-01'), at('2026-12-01'), 'md', 0, at('2026-12-05')), 'easylauncher_2026-11-01_2026-11-30.md');
  assert.equal(F.exportName('easylauncher', at('2026-11-16', 4), at('2026-11-23', 4), 'csv', DAY_START, at('2026-12-01')), 'easylauncher_2026-11-16_2026-11-22.csv');
});


test('session note', () => {
  assert.equal(F.sessionNote(false, 45), 'началась сама после простоя · закончится после 45 мин без активности');
  assert.equal(F.sessionNote(true, 30), 'начата вручную · закончится после 30 мин без активности');
});

test('history entries', () => {
  const t = (d, h, m) => new Date(2026, 10, d, h, m).getTime();
  const H6 = 6 * 3600_000;
  const names = { tricky: 'Tricky Network', u: 'Universe' };
  const opts = { name: (id) => R.targetName(id, Object.entries(names).map(([id, name]) => ({ id, name }))), activity: (id) => F.activityLabel(id), ruleText: R.ruleText, dayStartMs: H6 };
  const data = {
    corrections: [
      { id: 12, kind: 'project', project: 'tricky', start: t(16, 23, 39), end: t(17, 0, 16), made: t(17, 0, 40), was: [{ project: '', me: 7 * 60_000 }] },
      { id: 13, kind: 'add', project: 'u', start: t(16, 14, 0), end: t(16, 15, 30), made: t(16, 16, 5), note: 'созвон с заказчиком по ТЗ, офлайн' },
      { id: 2, kind: 'exclude', project: '', start: t(10, 12, 0), end: t(10, 12, 5) },
    ],
    changes: [
      { id: 3, kind: 'rule-add', at: t(17, 1, 2), from: t(1, 6, 0), ms: 4 * 60_000, source: 'inbox', rule: { kind: 'process', value: 'Telegram.exe', project: 'exclude' } },
      { id: 1, kind: 'rule-add', at: t(15, 21, 30), from: t(1, 6, 0), ms: 362 * 60_000, rule: { kind: 'process', value: 'idea64.exe', title: 'universe-', project: 'u' } },
    ],
  };
  const e = F.historyEntries(data, opts);
  assert.deepEqual(e.map((x) => [x.kind, x.title, x.sub]), [
    ['rule', 'Telegram.exe → Не работа', 'правило из разбора · применено к 1–16 ноя · 4м'],
    ['assign', '23:39–00:16 → Tricky Network', 'вручную · было: Другое 7м'],
    ['manual', '+ 14:00–15:30 · Universe', 'вручную · созвон с заказчиком по ТЗ, офлайн'],
    ['rule', 'Правило Universe: idea64.exe · «universe-»', 'новое · применено к 1–15 ноя · +6ч 2м'],
    ['excl', '12:00–12:05 → Не работа', 'вручную'],
  ]);
  assert.deepEqual(e[1].undo, ['correct', 'rm', '12']);
  assert.deepEqual(e[0].undo, ['history', 'undo', '3']);
  assert.equal(F.stamp(t(17, 0, 40)), '17 ноя, 00:40');
  assert.equal(F.appliedRange(new Date(2026, 9, 28, 6, 0).getTime(), t(3, 12, 0), H6), '28 окт – 3 ноя');
  assert.equal(F.appliedRange(t(16, 6, 0), t(17, 1, 0), H6), '16 ноя'); // 01:00 is still the 16th's day
  const del = F.historyEntries({ changes: [{ id: 4, kind: 'rule-delete', at: t(17, 2, 0), from: t(1, 6, 0), ms: 0, rule: null, prev: { kind: 'title', value: 'Jira', project: 'u' } }] }, opts);
  assert.deepEqual([del[0].key, del[0].title, del[0].sub], ['h:4', 'Правило Universe: «Jira»', 'удалено']);
});

test('manual range', () => {
  const H = 3600_000, M = 60_000;
  const day0 = new Date(2026, 10, 16).getTime();
  assert.equal(F.parseClock('14:00'), 840);
  assert.equal(F.parseClock('9:05'), 545);
  assert.equal(F.parseClock('24:00'), null);
  assert.equal(F.parseClock('14.00'), null);
  assert.deepEqual(F.manualRange('2026-11-16', 6 * H, 840, 930), { start: day0 + 14 * H, end: day0 + 15.5 * H });
  assert.deepEqual(F.manualRange('2026-11-16', 6 * H, 1410, 30), { start: day0 + 23.5 * H, end: day0 + 24.5 * H }); // past midnight
  assert.deepEqual(F.manualRange('2026-11-16', 6 * H, 60, 120), { start: day0 + 25 * H, end: day0 + 26 * H }); // before 06:00: the night after
  assert.equal(F.manualRange('2026-11-16', 6 * H, 930, 840), null);
  assert.equal(F.manualRange('2026-11-16', 6 * H, 300, 420), null); // 05:00 is the night after, 07:00 this morning
  assert.deepEqual(F.manualRange('2026-11-16', 6 * H, 360, 359), { start: day0 + 6 * H, end: day0 + 29 * H + 59 * M }); // 06:00 this day, 05:59 the next
  const me = [{ start: day0 + 14 * H, end: day0 + 14 * H + 25 * M }, { start: day0 + 16 * H, end: day0 + 17 * H }];
  assert.equal(F.overlapMs(me, day0 + 14 * H, day0 + 15.5 * H), 25 * M);
});

test('goal wording', () => {
  const u = { from: '2026-11-01', dir: 'min', minutes: 390, period: 'day', weekdays: [1, 2, 3, 4, 5], calendar: true, weekends: true };
  const t = { from: '2026-11-01', dir: 'min', minutes: 480, period: 'week' };
  assert.deepEqual([0, 0.5, 209, -209, 120].map(F.signed), ['0', '0', '+3ч 29м', '−3ч 29м', '+2ч']);
  assert.deepEqual(['6,5', '6.5', '8', '6,25', '0', '-1', 'abc', ''].map(F.parseHours), [390, 390, 480, 375, null, null, null, null]);
  assert.deepEqual([390, 480, 375].map(F.hoursText), ['6,5', '8', '6,25']);
  const r = F.roundRows([{ project: 'u', me: 181.5 * 60000 }, { project: 't', me: 36 * 60000 + 29999 }, { project: '', me: 29999 }]);
  assert.deepEqual([r.rows.map((x) => x.me / 60000), r.total / 60000], [[182, 36, 0], 218]);
  assert.equal(F.goalOn([u, { ...u, from: '2026-12-01', minutes: 420 }], '2026-11-30').minutes, 390);
  assert.equal(F.goalOn([u, { from: '2026-11-20', off: true }], '2026-11-21'), null);
  assert.equal(F.goalOn([{ ...u, from: '2026-12-01', minutes: 420 }, u], '2026-12-02').minutes, 420);
  assert.deepEqual([u, t, { ...t, period: 'month', minutes: 2400, dir: 'max' }].map(F.goalShort), ['6,5ч / д', '8ч / нед', '≤ 40ч / мес']);
  assert.deepEqual([u, { ...u, calendar: false }, t].map(F.goalLine), ['≥ 6ч 30м / раб. день', '≥ 6ч 30м / день', '≥ 8ч / нед.']);
  assert.equal(F.goalVersion(u), 'с 1 ноября 2026 — не меньше 6,5ч в день');
  assert.equal(F.goalVersion({ from: '2026-12-01', off: true }), 'с 1 декабря 2026 — цели нет');
  assert.equal(F.weeklyTotal([u, t, { ...t, minutes: 300 }]), 2730);
  assert.deepEqual([[t, 480, 480], [t, 360, 480], [{ ...t, dir: 'max' }, 300, 480], [{ ...t, dir: 'max' }, 540, 480]].map((a) => F.progressText(...a)), ['цель выполнена', 'не хватает 2ч', 'в пределах цели', 'сверх цели на 1ч']);
});

test('goal rows and the forecast', () => {
  const sum = (o) => ({ plan: 0, plan_to_date: 0, fact: 0, me: 0, balance: 0, forecast: 0, need: 0, days: 0, days_left: 0, ...o });
  const day = { period: 'day', dir: 'min', minutes: 390 };
  const u = { project: 'u', goal: day, day: sum({ me: 181, plan: 390, fact: 181, balance: -209 }), week: sum({}), month: sum({ plan: 7800, forecast: 4901 }), agents: 343 };
  const b = F.goalBlock(u);
  assert.deepEqual({ ...b, fill: Math.round(b.fill) }, { value: '3ч 1м', goal: '6ч 30м', fill: 46, sub: '−3ч 29м · агенты 5ч 43м' });
  const tr = { project: 't', goal: { period: 'week', dir: 'min', minutes: 480 }, day: sum({ me: 36 }), week: sum({ plan: 480, fact: 36 }), month: sum({}), agents: 0 };
  assert.deepEqual(F.goalBlock(tr), { value: '36м', goal: '8ч в нед.', fill: 7.5, sub: 'за неделю 36м' });
  const weekend = { ...u, day: sum({ me: 60, fact: 60, balance: 60 }), agents: 0 };
  assert.deepEqual(F.goalBlock(weekend), { value: '1ч', goal: null, fill: null, sub: null });
  assert.equal(F.forecastLine({ ...u, day: { ...u.day }, month: sum({ plan: 7800, forecast: 4901 }) }, 'Universe', '2026-11-16'), 'прогноз ноября по Universe ≈ 81ч 41м из 130ч');
});

test('week rows', () => {
  const days = F.weekDays('2026-11-12');
  assert.deepEqual([days[0], days[6]], ['2026-11-09', '2026-11-15']);
  assert.equal(F.weekTitle(days), '9 – 15 ноября');
  assert.equal(F.weekTitle(F.weekDays('2026-12-02')), '30 ноября – 6 декабря');
  assert.equal(F.weekRelative(days, '2026-11-16'), 'прошлая неделя');
  assert.equal(F.mainGoal({ t: { period: 'week' }, u: { period: 'day' } }, ['t', 'u']), 'u');
  const d = (day, me, plan, fact, agents = {}) => ({ day, me, agents, plan, fact, work: plan.u > 0 });
  const plan = {
    today: '2026-11-14',
    goals: { u: { period: 'day', minutes: 390 } },
    sums: { u: { balance: -38 } },
    days: [
      d('2026-11-09', { u: 400, '': 5 }, { u: 390 }, { u: 400 }, { u: 310 }),
      d('2026-11-10', { u: 300 }, { u: 390 }, { u: 300 }),
      d('2026-11-11', {}, { u: 390 }, {}),
      d('2026-11-12', { u: 390 }, { u: 390 }, { u: 390 }),
      d('2026-11-13', { u: 352 }, { u: 390 }, { u: 352 }),
      d('2026-11-14', { u: 30 }, { u: 0 }, { u: 30 }),
      d('2026-11-15', {}, { u: 0 }, {}),
    ],
  };
  const w = F.weekRows(plan, 'u');
  assert.deepEqual(w.rows.map((r) => [r.label, r.weekend, r.me, r.diff.text]), [
    ['пн 9', false, 405, '+10м'], ['вт 10', false, 300, '−1ч 30м'], ['ср 11', false, 0, '−6ч 30м'], ['чт 12', false, 390, '0'], ['пт 13', false, 352, '−38м'], ['сб 14', true, 30, '+30м'], ['вс 15', true, 0, ''],
  ]);
  assert.deepEqual(w.totals, { me: 1477, agents: 310, diff: '−38м' });
  assert.equal(w.totals.me, w.rows.reduce((s, r) => s + r.me, 0));
});

test('month cells', () => {
  const g = F.monthGrid('2026-11-16');
  assert.equal(g.length, 42);
  assert.deepEqual([g[5], g[6], g[35], g[36]], [null, '2026-11-01', '2026-11-30', null]);
  const day = { day: '2026-11-16', me: { universe: 181, tricky: 36, study: 3, easylauncher: 2, '': 13 }, plan: { universe: 390 } };
  const b = F.cellBars(day, ['universe', 'easylauncher', 'tricky', 'study'], 'universe', { fact: 2451, plan_to_date: 3900 }, '2026-11-16');
  assert.deepEqual(b.parts.map((p) => p.project), ['universe', 'easylauncher', 'tricky', 'study', '']);
  assert.equal(b.parts[0].pct, (181 / 480) * 100);
  assert.equal(b.tick, 81.25);
  assert.equal(b.forecast, null); // today is not after today
  const future = F.cellBars({ day: '2026-11-17', me: {}, plan: { universe: 390 } }, ['universe'], 'universe', { fact: 2451, plan_to_date: 3900 }, '2026-11-16');
  assert.equal(future.forecast, ((390 * 2451) / 3900 / 480) * 100);
});

test('days off', () => {
  assert.equal(F.offTitle('2026-11-23', '2026-11-27'), '23 – 27 ноября');
  assert.equal(F.offTitle('2026-11-30', '2026-12-02'), '30 ноября – 2 декабря');
  assert.deepEqual([1, 2, 5].map(F.workdaysText), ['1 раб. день', '2 раб. дня', '5 раб. дней']);
  const week = ['23', '24', '25', '26', '27'].map((d) => ({ day: `2026-11-${d}`, plan: { u: 390, t: 480 / 7, s: 300 / 7 } }));
  assert.deepEqual(['u', 't', 's'].map((p) => F.fmtMin(F.offReduce(week, p))), ['32ч 30м', '5ч 43м', '3ч 34м']);
  const e = F.historyEntries({ days_off: [{ id: 4, from: '2026-11-23', to: '2026-11-27', kind: 'otgul', projects: ['u', 't'], made: new Date(2026, 10, 17, 1, 0).getTime() }] }, { name: (id) => ({ u: 'Universe', t: 'Tricky Network' })[id], activity: () => '', ruleText: () => '', dayStartMs: 6 * 3600_000 });
  assert.deepEqual([e[0].kind, e[0].title, e[0].sub, e[0].undo], ['dayoff', '23–27 ноя · отгул', 'план снижен: Universe, Tricky Network', ['dayoff', 'rm', '4']]);
});
