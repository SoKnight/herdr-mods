const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const code = fs.readFileSync(path.join(__dirname, '../mods/time-tracker/format.js'), 'utf8');
const F = new Function(`${code.replace(/^export /gm, '')}\nreturn { formatDuration, activityLabel, parseResult, weekRange, monthRange, sessionNote, catalog, plural, dayTitle, shareText, stamp, appliedRange, historyEntries, parseClock, manualRange, overlapMs, fmtMin, signed, roundRows, parseHours, hoursText, goalOn, goalShort, goalLine, goalVersion, weeklyTotal, progressText, goalBlock, forecastLine, weekDays, weekTitle, weekRelative, mainGoal, dayDiff, weekRows, monthGrid, cellBars, offTitle, workdaysText, offReduce, money, parseMoney, monthOptions, goalStrip, reportFootnote, exportFile, payOn, payShort, dialogWrites, monthTitle, monthGenitive, closedText, offDaysText, agentsText, parseDay, goalFromNote, goalNow, goalReplaceText, goalRemoveText };`)();
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

test('report wording', () => {
  assert.deepEqual([0, 769, 15000, 100000, 1234567].map(F.money), ['0 ₽', '769 ₽', '15 000 ₽', '100 000 ₽', '1 234 567 ₽']);
  assert.deepEqual(['100 000', '15000', ' 7 ', '0', '1,5', 'abc', ''].map(F.parseMoney), [100000, 15000, 7, null, null, null, null]);
  assert.deepEqual(['100\u00a0000', '1\u202f000', '100 000 000', '100 000 001', '9'.repeat(22)].map(F.parseMoney), [100000, 1000, 100000000, null, null]);
  assert.deepEqual(F.monthOptions('2026-11-16', 3), [{ id: '2026-11', label: 'Ноябрь 2026' }, { id: '2026-10', label: 'Октябрь 2026' }, { id: '2026-09', label: 'Сентябрь 2026' }]);
  assert.equal(F.monthOptions('2026-01-05', 2)[1].id, '2025-12');
  const u = { dir: 'min', minutes: 390, period: 'day', weekdays: [1, 2, 3, 4, 5], calendar: true, weekends: true };
  assert.deepEqual(F.goalStrip(u), { text: '6ч 30м в рабочий день', sub: 'активное время · выходные в зачёт' });
  assert.deepEqual(F.goalStrip({ dir: 'max', minutes: 2400, period: 'month' }), { text: 'не больше 40ч в месяц', sub: 'активное время' });
  const rep = { goal: u, holidays: ['2026-11-04'] };
  assert.equal(F.reportFootnote(rep, true), 'Рабочие дни — по производственному календарю (4 ноября — праздник). Время в выходные идёт в зачёт. Агенты в разницу не входят.');
  assert.equal(F.reportFootnote({ goal: { ...u, weekends: false }, holidays: ['2026-05-01', '2026-05-09', '2026-05-11'] }, true), 'Рабочие дни — по производственному календарю (1, 9 и 11 мая — праздники). Время в выходные в цель не идёт. Агенты в разницу не входят.');
  assert.equal(F.exportFile('universe', '2026-11'), 'universe_2026-11.csv');
});

test('report wording: the rest of the strip, footnote and file name', () => {
  assert.deepEqual(F.goalStrip({ dir: 'min', minutes: 480, period: 'week' }), { text: '8ч в неделю', sub: 'активное время' });
  assert.deepEqual(F.goalStrip({ dir: 'min', minutes: 390, period: 'day', calendar: false, weekends: false }), { text: '6ч 30м в день', sub: 'активное время' });
  const u = { dir: 'min', minutes: 390, period: 'day', calendar: true, weekends: true };
  assert.equal(F.reportFootnote({ goal: u, holidays: [] }, false), 'Рабочие дни — по производственному календарю. Время в выходные идёт в зачёт.');
  assert.equal(F.reportFootnote({ goal: u, holidays: ['2026-11-04', '2026-11-11'] }, false), 'Рабочие дни — по производственному календарю (4 и 11 ноября — праздники). Время в выходные идёт в зачёт.');
  assert.equal(F.reportFootnote({ goal: { ...u, calendar: false }, holidays: ['2026-11-04'] }, true), 'Время в выходные идёт в зачёт. Агенты в разницу не входят.');
  // week and month goals: no calendar and no weekend sentence (checkGoal keeps weekends false for them)
  assert.equal(F.reportFootnote({ goal: { dir: 'min', minutes: 480, period: 'week', weekends: false }, holidays: ['2026-11-04'] }, true), 'Агенты в разницу не входят.');
  assert.equal(F.reportFootnote({ goal: null, holidays: ['2026-11-04'] }, true), 'Агенты в разницу не входят.');
  assert.equal(F.reportFootnote({ goal: null, holidays: [] }, false), '');
  assert.equal(F.exportFile('a<b>:c"d/e\\f|g?h*', '2026-11'), 'abcdefgh_2026-11.csv');
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

test('goal changes in history', () => {
  const g = { from: '2026-11-01', dir: 'min', minutes: 390, period: 'day', weekdays: [1, 2, 3, 4, 5], calendar: true };
  const at = (d) => new Date(2026, 10, d, 12, 0).getTime();
  const changes = [
    { id: 7, kind: 'goal-set', at: at(20), project: 'easy', goal: { from: '2026-11-20', off: true }, goals: [g, { from: '2026-11-20', off: true }], prev_goals: [g] },
    { id: 6, kind: 'goal-set', at: at(19), project: 'easy', goal: g, goals: [g], prev_goals: [] },
    { id: 5, kind: 'later-kind', at: at(18) }, // a kind this version doesn't know: left out
  ];
  const rows = F.historyEntries({ changes }, { name: () => 'Easy', activity: (x) => x, ruleText: () => '', dayStartMs: 0 });
  assert.deepEqual(rows.map((r) => [r.key, r.kind, r.title, r.sub]), [
    ['h:7', 'goal', 'Цель Easy убрана', 'с 20 ноября · было: ≥ 6ч 30м / раб. день'],
    ['h:6', 'goal', 'Цель Easy: ≥ 6ч 30м / раб. день', 'с 1 ноября · новая'],
  ]);
  assert.deepEqual(rows[0].undo, ['history', 'undo', '7']);
  const dec = F.historyEntries({ changes: [{ id: 8, kind: 'goal-set', at: at(20), project: 'easy', goal: { ...g, from: '2025-12-01' }, prev_goals: [] }] }, { name: () => 'Easy' });
  assert.equal(dec[0].sub, 'с 1 декабря 2025 · новая');
});

test('pay wording', () => {
  const pays = [{ from: '2026-10', kind: 'fixed', amount: 90000 }, { from: '2026-12', kind: 'hourly', amount: 2000 }, { from: '2027-02', kind: 'none' }];
  assert.equal(F.payOn(pays, '2026-09'), null);
  assert.deepEqual([F.payOn(pays, '2026-11'), F.payOn(pays, '2027-01'), F.payOn(pays, '2027-03')].map(F.payShort), ['90 000 ₽ / мес + премии', '2 000 ₽ / ч', null]);
  const at = new Date(2026, 10, 18, 12, 0).getTime();
  const changes = [
    { id: 9, kind: 'pay-set', at, project: 'uni', pay: { from: '2026-12', kind: 'none' }, prev_pays: pays.slice(0, 2) },
    { id: 8, kind: 'pay-set', at: at - 1, project: 'uni', pay: { from: '2026-11', kind: 'fixed', amount: 100000 }, prev_pays: pays.slice(0, 1) },
    { id: 7, kind: 'pay-set', at: at - 2, project: 'uni', pay: { from: '2026-09', kind: 'hourly', amount: 1500 }, prev_pays: [] },
  ];
  const rows = F.historyEntries({ changes }, { name: () => 'Universe' });
  assert.deepEqual(rows.map((r) => [r.key, r.kind, r.title, r.sub]), [
    ['h:9', 'pay', 'Оплата Universe снята', 'с декабря 2026 · было: 2 000 ₽ / ч'],
    ['h:8', 'pay', 'Оплата Universe: 100 000 ₽ / мес + премии', 'с ноября 2026 · было: 90 000 ₽ / мес + премии'],
    ['h:7', 'pay', 'Оплата Universe: 1 500 ₽ / ч', 'с сентября 2026 · новая'],
  ]);
  assert.deepEqual(rows[0].undo, ['history', 'undo', '9']);
  const off = F.historyEntries({ changes: [{ id: 10, kind: 'pay-set', at, project: 'uni', pay: { from: '2026-11', kind: 'none' }, prev_pays: [] }] }, { name: () => 'Universe' });
  assert.deepEqual([off[0].title, off[0].sub], ['Оплата Universe снята', 'с ноября 2026']);
});

test('the goal dialog writes what was touched', () => {
  const week = { goal: ['min', '8', 'week', [1, 2, 3, 4, 5], true, true, '2026-11-01'], pay: ['none', ''] };
  assert.deepEqual(F.dialogWrites(week, { ...week, pay: ['fixed', '100 000'] }), { goal: false, pay: true });
  assert.deepEqual(F.dialogWrites(week, { ...week, goal: ['min', '8', 'week', [1, 2, 3, 4, 5], true, true, '2026-12-01'] }), { goal: true, pay: false });
  assert.deepEqual(F.dialogWrites(week, week), { goal: false, pay: false });
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
  assert.equal(F.weeklyTotal([{ from: '2026-11-01', dir: 'min', minutes: 390, period: 'day' }, t]), 480); // hand-edited, no weekdays
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
  // ждали: this project's waits of the day, once they make a minute.
  const M = 60_000;
  const waits = [{ start: 0, end: 30 * M, project: 'u' }, { start: 40 * M, end: 48 * M, project: 'u' }, { start: 0, end: 90 * M, project: 't' }];
  assert.equal(F.goalBlock(u, waits).sub, '−3ч 29м · агенты 5ч 43м, ждали 38м');
  assert.equal(F.goalBlock(u, [{ start: 0, end: 59_000, project: 'u' }]).sub, '−3ч 29м · агенты 5ч 43м');
  assert.equal(F.goalBlock(tr, waits).sub, 'за неделю 36м');
  assert.equal(F.agentsText(343 * M, 38 * M), 'агенты 5ч 43м, ждали 38м');
  assert.equal(F.agentsText(343 * M, 59_999), 'агенты 5ч 43м');
  assert.equal(F.agentsText(0, 0), 'агенты —');
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

test('month edges', () => {
  const g = F.monthGrid('2026-06-10'); // June 2026 starts on a Monday
  assert.deepEqual([g[0], g[29], g[30]], ['2026-06-01', '2026-06-30', null]);
  const b = F.cellBars({ day: '2026-11-10', me: { universe: 10, old: 5, '': 3 }, plan: {} }, ['universe'], null, null, '2026-11-16');
  assert.deepEqual(b.parts.map((p) => p.project), ['universe', 'old', '']);
  assert.deepEqual([b.tick, b.forecast], [null, null]);
  assert.equal(F.offTitle('2026-11-23', '2026-11-23'), '23 ноября');
  const opts = { name: (id) => id, activity: () => '', ruleText: () => '', dayStartMs: 0 };
  const e = F.historyEntries({ days_off: [
    { id: 1, from: '2026-11-23', to: '2026-11-23', kind: 'otgul', projects: ['u'], made: 3 },
    { id: 2, from: '2026-11-30', to: '2026-12-02', kind: 'vacation', projects: [], made: 2 },
  ] }, opts);
  assert.deepEqual(e.map((x) => [x.title, x.sub]), [['23 ноя · отгул', 'план снижен: u'], ['30 ноя – 2 дек · отпуск', 'план не менялся']]);
});

test('month close wording', () => {
  assert.deepEqual([F.monthTitle('2026-10'), F.monthGenitive('2026-11')], ['Октябрь', 'ноября']);
  assert.equal(F.closedText({ error: 'closed', period: 3, month: '2026-11' }), 'Ноябрь закрыт. Открыть заново?');
  assert.equal(F.closedText({ error: 'closed', period: 3, month: '' }), null);
  assert.equal(F.offDaysText({ holidays: ['2026-11-04'], days_off: [{ from: '2026-11-23', to: '2026-11-27' }] }), '4 ноя, 23–27 ноя');
  assert.equal(F.offDaysText({ holidays: [], days_off: [] }), 'нет');
  const lookup = { name: (id) => id, activity: () => '', ruleText: () => '', dayStartMs: 6 * 3600_000 };
  const e = F.historyEntries({ changes: [
    { id: 9, at: new Date(2026, 11, 2, 12, 0).getTime(), kind: 'month-reopen', month: '2026-10', period: 2 },
    { id: 8, at: new Date(2026, 10, 2, 12, 0).getTime(), kind: 'month-close', month: '2026-10', period: 2, sent: true },
  ] }, lookup);
  assert.deepEqual(e.map((x) => [x.kind, x.title, x.sub ?? null, Boolean(x.border), x.undo]), [
    ['month-reopen', 'Октябрь открыт заново', 'закрытые цифры сохранены', false, ['history', 'undo', '9']],
    ['month-close', 'Октябрь закрыт · отчёт отправлен', null, true, ['history', 'undo', '8']],
  ]);
});

test('parseDay reads ДД.ММ and ДД.ММ.ГГГГ', () => {
  assert.equal(F.parseDay('16.11', '2026-11-18'), '2026-11-16');
  assert.equal(F.parseDay(' 1.2.2027 ', '2026-11-18'), '2027-02-01');
  assert.equal(F.parseDay('31.11', '2026-11-18'), null);
  assert.equal(F.parseDay('16/11', '2026-11-18'), null);
  assert.equal(F.parseDay('', '2026-11-18'), null);
});

test('goalFromNote warns about the versions a save keeps or replaces', () => {
  const g = (from, off) => (off ? { from, off: true } : { from, dir: 'min', minutes: 390, period: 'day' });
  assert.equal(F.goalFromNote([g('2026-11-01')], '2026-11-16'), 'С 1 ноября уже действует цель — она останется до 16 ноября.');
  assert.equal(F.goalFromNote([g('2026-11-16')], '2026-11-01'), 'Версии с 16 ноября будут заменены.');
  assert.equal(F.goalFromNote([g('2026-11-01', true)], '2026-11-16'), null);
  assert.equal(F.goalFromNote([g('2026-11-16')], '2026-11-16'), null);
  assert.equal(F.goalFromNote(undefined, '2026-11-16'), null);
});

test('goal dialog says what is in force and asks before replacing or removing', () => {
  const g = (from) => ({ from, dir: 'min', minutes: 390, period: 'day' });
  assert.equal(F.goalNow(g('2026-11-16')), 'Сейчас: не меньше 6,5ч в день, с 16 ноября 2026');
  assert.equal(F.goalNow(null), 'Сейчас цели нет.');
  assert.equal(F.goalReplaceText([g('2026-11-16')], '2026-11-01'), 'Цель с 1 ноября заменит версии с 16 ноября. Сохранить?');
  assert.equal(F.goalReplaceText([g('2026-11-16')], '2026-11-16'), null);
  assert.equal(F.goalRemoveText([g('2026-11-16')], '2026-11-16'), 'Цели не будет с 16 ноября. Убрать?');
  assert.equal(F.goalRemoveText([g('2026-10-01'), g('2026-11-16')], '2026-11-16'), 'Цели не будет с 16 ноября, до этого остаётся цель с 1 октября. Убрать?');
});
