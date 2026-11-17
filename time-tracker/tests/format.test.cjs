const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const code = fs.readFileSync(path.join(__dirname, '../mods/time-tracker/format.js'), 'utf8');
const F = new Function(`${code.replace(/^export /gm, '')}\nreturn { formatDuration, activityLabel, parseResult, weekRange, monthRange, exportName, sessionNote, catalog, plural, dayTitle, shareText, stamp, appliedRange, historyEntries, parseClock, manualRange, overlapMs };`)();
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
