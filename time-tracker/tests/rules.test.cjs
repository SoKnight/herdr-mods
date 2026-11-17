const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const code = fs.readFileSync(path.join(__dirname, '../mods/time-tracker/rules.js'), 'utf8');
const R = new Function(`${code.replace(/^export /gm, '')}\nreturn { RULE_KINDS, COLORS, ruleText, freeColor, targetName, guessName, monthStartTitle, deleteAsks, timeRange, inboxLeft };`)();

test('rule text', () => {
  assert.equal(R.ruleText({ kind: 'folder', value: 'D:\\Work\\Universe' }), 'D:\\Work\\Universe\\**');
  assert.equal(R.ruleText({ kind: 'process', value: 'idea64.exe', title: 'universe-' }), 'idea64.exe · «universe-»');
  assert.equal(R.ruleText({ kind: 'process', value: 'Telegram.exe' }), 'Telegram.exe');
  assert.equal(R.ruleText({ kind: 'title', value: 'Universe' }), '«Universe»');
  assert.equal(R.ruleText({ kind: 'figma', value: 'Universe — App' }), '«Universe — App»');
  assert.equal(R.ruleText({ kind: 'minecraft', value: 'localhost:25565' }), 'Minecraft · localhost:25565');
  assert.equal(R.ruleText({ kind: 'folder', value: 'D:\\Work\\Tricky' }, true), 'папка D:\\Work\\Tricky\\**');
  assert.equal(R.ruleText({ kind: 'process', value: 'Telegram.exe' }, true), 'процесс Telegram.exe');
  assert.equal(R.ruleText({ kind: 'process', value: 'idea64.exe', title: 'universe-' }, true), 'процесс idea64.exe · заголовок содержит «universe-»');
  assert.equal(R.ruleText({ kind: 'title', value: 'tricky-network' }, true), 'заголовок содержит «tricky-network»');
  assert.equal(R.ruleText({ kind: 'figma', value: 'X' }, true), 'Figma «X»');
  assert.equal(R.ruleText({ kind: 'minecraft', value: 'a:1' }, true), 'Minecraft · a:1');
});

test('free colour', () => {
  const p = (color, archived = false) => ({ id: color, color, archived });
  assert.equal(R.freeColor([p('ansi1'), p('ansi2'), p('ansi4')]), 'ansi3');
  assert.equal(R.freeColor([p('ansi1', true)]), 'ansi1');
  assert.equal(R.freeColor(R.COLORS.map((c) => p(c))), 'ansi1');
});

test('targets, names and dates', () => {
  const projects = [{ id: 'u', name: 'Universe' }];
  assert.equal(R.targetName('exclude', projects), 'Не работа');
  assert.equal(R.targetName('leisure', projects), 'Досуг');
  assert.equal(R.targetName('u', projects), 'Universe');
  assert.equal(R.guessName({ kind: 'folder', value: 'D:\\Work\\EasyPayments\\' }), 'EasyPayments');
  assert.equal(R.guessName({ kind: 'minecraft', value: 'play.tricky.net:25565' }), 'play.tricky.net');
  assert.equal(R.guessName({ kind: 'figma', value: 'EasyLauncher — UI' }), 'EasyLauncher — UI');
  assert.equal(R.monthStartTitle('2026-11-16'), '1 ноября');
  assert.deepEqual(R.RULE_KINDS.map((k) => k.label), ['Папка', 'Процесс', 'Заголовок', 'Figma', 'Minecraft']);
});

test('delete asks when it moves this month', () => {
  const nov1 = new Date(2026, 10, 1, 6).getTime();
  assert.equal(R.deleteAsks({ ms: 0, from: nov1 }, '2026-11-17'), false);
  assert.equal(R.deleteAsks({ ms: 60000, from: nov1 }, '2026-11-17'), true);
  assert.equal(R.deleteAsks({ ms: 0 }, '2026-11-17'), true);
  assert.equal(R.deleteAsks({ ms: 0, from: new Date(2026, 9, 20, 6).getTime() }, '2026-11-17'), true);
});

test('inbox times and what is left', () => {
  const t = (h, m) => new Date(2026, 10, 17, h, m).getTime();
  assert.equal(R.timeRange(t(0, 3), t(0, 6)), '00:03–00:06');
  assert.equal(R.timeRange(t(3, 5), t(3, 6)), '03:05');
  const items = [{ ms: 180_000, suggest: { project: 'tricky' } }, { ms: 240_000, suggest: { project: 'exclude' } }, { ms: 240_000, suggest: null }, { ms: 60_000, suggest: { project: 'study' } }];
  assert.deepEqual(R.inboxLeft(items, {}), { ms: 720_000, windows: 4, hints: 3 });
  assert.deepEqual(R.inboxLeft(items, { 0: { project: 'tricky' } }), { ms: 540_000, windows: 3, hints: 2 });
});
