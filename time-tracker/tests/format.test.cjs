const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const code = fs.readFileSync(path.join(__dirname, '../mods/time-tracker/format.js'), 'utf8');
const F = new Function(`${code.replace(/^export /gm, '')}\nreturn { formatDuration, activityLabel, parseResult, weekRange, monthRange, exportName, sessionNote, catalog, plural, dayTitle, shareText };`)();

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
