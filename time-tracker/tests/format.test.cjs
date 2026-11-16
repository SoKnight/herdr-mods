const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const code = fs.readFileSync(path.join(__dirname, '../mods/time-tracker/format.js'), 'utf8');
const F = new Function(`${code.replace(/^export /gm, '')}\nreturn { formatDuration, activityLabel, parseResult, weekRange, monthRange, exportName, previewMarkdown };`)();

test('durations', () => {
  assert.equal(F.formatDuration(18_720_000), '5ч 12м');
  assert.equal(F.formatDuration(2_700_000), '45м');
  assert.equal(F.formatDuration(0), '0м');
});

test('activity labels fall back to the id', () => {
  const labels = { development: 'Разработка', architecture: 'Проектирование', design: 'Дизайн', documentation: 'Документация', study: 'Учёба' };
  for (const [id, label] of Object.entries(labels)) assert.equal(F.activityLabel(id), label);
  assert.equal(F.activityLabel('custom'), 'custom');
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

test('export file name', () => {
  assert.equal(F.exportName('easylauncher', at('2026-11-01'), at('2026-12-01'), 'md'), 'easylauncher_2026-11-01_2026-11-30.md');
  assert.equal(F.exportName('easylauncher', at('2026-11-16', 4), at('2026-11-23', 4), 'csv'), 'easylauncher_2026-11-16_2026-11-22.csv');
});

test('preview rounds each cell and keeps agents out of the total', () => {
  const p = { days: { '2026-11-16': { agents: 20 * 60_000, activities: { development: 37 * 60_000, '': 4 * 60_000 } } } };
  const md = F.previewMarkdown('Easy', at('2026-11-16', 4), at('2026-11-17', 4), p, 15, true);
  assert.match(md, /\| Разработка \| 30 \|/);
  assert.match(md, /\| Без вида \| 0 \|/);
  assert.match(md, /\*\*Итого: 30 мин\*\*/);
  assert.match(md, /агентов \(для справки\): 15 мин/);
});
