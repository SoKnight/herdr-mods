const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const code = fs.readFileSync(path.join(__dirname, '../mods/time-tracker/lanes.js'), 'utf8');
const L = new Function(`${code.replace(/^export /gm, '')}\nreturn { bars, projectColor, rangeFromDrag, correctionIds, shiftDay, stepDay, tickStep, ticks, sessionWindow, composition, topWindow, ruleItem };`)();

const H = 3600_000;
const seg = (start, end, extra = {}) => ({ start, end, app: 'idea64.exe', title: 't', project: 'easylauncher', activity: 'development', why: '', corrected: false, ...extra });
const byProject = (me, projects) => L.bars(me.map((s) => ({ start: s.start, end: s.end, value: s.project, seg: s })), 0, 24 * H, 1440, (v) => L.projectColor(v, projects));

test('drag maps to whole minutes in order', () => {
  const from = 0, to = 24 * 3600_000;
  assert.deepEqual(L.rangeFromDrag(600, 300, from, to, 1440), { start: 300 * 60_000, end: 600 * 60_000 });
  assert.deepEqual(L.rangeFromDrag(-5, 2000, from, to, 1440), { start: 0, end: to });
  assert.deepEqual(L.rangeFromDrag(100.2, 100.3, from, to, 1440), { start: 100 * 60_000, end: 101 * 60_000 });
});

test('a drag ends by now, rounded down to the minute', () => {
  const from = 0, to = 24 * 3600_000, now = 500 * 60_000 + 30_000;
  assert.deepEqual(L.rangeFromDrag(300, 900, from, to, 1440, now), { start: 300 * 60_000, end: 500 * 60_000 });
  assert.deepEqual(L.rangeFromDrag(300, 400, from, to, 1440, now), { start: 300 * 60_000, end: 400 * 60_000 });
  assert.equal(L.rangeFromDrag(500, 900, from, to, 1440, now), null);
  assert.equal(L.rangeFromDrag(700, 900, from, to, 1440, now), null);
});

test('bars are positioned and coloured by project order', () => {
  const [bar] = L.bars([{ start: 6 * H, end: 12 * H, value: 'easylauncher', seg: {} }], 0, 24 * H, 1440, (v) => L.projectColor(v, [{ id: 'universe' }, { id: 'easylauncher' }]));
  assert.equal(bar.x, 360);
  assert.equal(bar.w, 360);
  assert.equal(bar.color, 'ansi2');
});

test('sub-pixel bars merge into a neighbour', () => {
  const t = 6 * H;
  const me = [seg(t, t + H), seg(t + H, t + H + 10_000, { project: 'universe' }), seg(t + H + 10_000, t + 2 * H)];
  const bars = byProject(me, [{ id: 'universe' }, { id: 'easylauncher' }]);
  assert.equal(bars.length, 1);
  assert.equal(bars[0].value, 'easylauncher');
  assert.equal(bars[0].x, 360);
  assert.equal(bars[0].w, 120);
  assert.equal(bars[0].segs.length, 3);
  // A visible gap keeps bars apart, and a different project is its own bar.
  const apart = byProject([seg(t, t + H), seg(t + 2 * H, t + 3 * H), seg(t + 3 * H, t + 4 * H, { project: 'universe' })], []);
  assert.equal(apart.length, 3);
});

test('a project has its own colour', () => {
  const projects = [{ id: 'u', color: 'ansi5' }, { id: 'e' }];
  assert.equal(L.projectColor('u', projects), 'ansi5');
  assert.equal(L.projectColor('e', projects), 'ansi2'); // no colour yet: by its place
  assert.equal(L.projectColor('', projects), 'dim');
  assert.equal(L.projectColor('x', projects), 'muted');
});

test('correction ids from why', () => {
  assert.deepEqual(L.correctionIds('правка #12: проект → easy; сигнал idea'), [12]);
  assert.deepEqual(L.correctionIds('правка #3: проект → a; агент x; правка #7: проект → b; правка #3: проект → a'), [3, 7]);
  assert.deepEqual(L.correctionIds('без проекта'), []);
});

test('days shift across months', () => {
  assert.equal(L.shiftDay('2026-11-30', 1), '2026-12-01');
  assert.equal(L.shiftDay('2026-03-01', -1), '2026-02-28');
});

test('stepping days: today and later ask for today', () => {
  assert.equal(L.stepDay(null, '2026-11-16', -1), '2026-11-15');
  assert.equal(L.stepDay('2026-11-15', '2026-11-16', -1), '2026-11-14');
  assert.equal(L.stepDay('2026-11-15', '2026-11-16', 1), null);
  assert.equal(L.stepDay(null, '2026-11-16', 1), null);
  // Today moved on while an older day was shown: stepping still lands on a real day.
  assert.equal(L.stepDay('2026-11-10', '2026-11-17', 1), '2026-11-11');
});

test('ticks split the range evenly', () => {
  assert.equal(L.tickStep(8 * H), H);
  assert.equal(L.tickStep(9 * H), 2 * H);
  assert.equal(L.tickStep(16 * H), 2 * H);
  assert.equal(L.tickStep(17 * H), 3 * H);
  const day = L.ticks(6 * H, 30 * H);
  assert.equal(day.length, 9);
  assert.equal(day[8], 30 * H);
  assert.deepEqual(L.ticks(22 * H, 28 * H), [22, 23, 24, 25, 26, 27, 28].map((h) => h * H));
});

test('session window: whole hours, three at least, even ticks', () => {
  const t = (d, h, m = 0) => new Date(2026, 10, d, h, m).getTime();
  assert.deepEqual(L.sessionWindow(t(16, 22, 11), t(17, 3, 55), t(17, 3, 56), true), { from: t(16, 22), to: t(17, 4) });
  assert.deepEqual(L.sessionWindow(t(16, 10, 20), t(16, 10, 40), t(16, 18), false), { from: t(16, 10), to: t(16, 13) });
  assert.deepEqual(L.sessionWindow(t(16, 9, 30), t(16, 19, 10), t(16, 19, 20), false), { from: t(16, 9), to: t(16, 21) });
  assert.deepEqual(L.sessionWindow(t(16, 10), t(16, 10, 30), t(16, 13, 30), true), { from: t(16, 10), to: t(16, 14) });
});

const M = 60_000;
test('composition of a range, most first', () => {
  const me = [seg(0, 25 * M, { project: 'tricky' }), seg(25 * M, 32 * M, { project: '' }), seg(32 * M, 40 * M, { project: 'universe' })];
  assert.deepEqual(L.composition(me, 0, 37 * M), [{ project: 'tricky', ms: 25 * M }, { project: '', ms: 7 * M }, { project: 'universe', ms: 5 * M }]);
  assert.deepEqual(L.composition(me, 50 * M, 60 * M), []);
});

test('the window a rule comes from', () => {
  const me = [
    seg(0, 5 * M, { app: 'chrome.exe', title: 'tricky-network/core', path: '' }),
    seg(5 * M, 8 * M, { app: 'idea64.exe', title: 'Main.kt', path: 'C:\\Dev\\easy' }),
    seg(8 * M, 10 * M, { app: 'chrome.exe', title: 'tricky-network/core', path: '' }),
  ];
  assert.deepEqual(L.topWindow(me, 0, 10 * M), { app: 'chrome.exe', title: 'tricky-network/core', path: '', server: '', ms: 7 * M });
  assert.equal(L.topWindow(me, 20 * M, 30 * M), null);
  assert.deepEqual(L.ruleItem({ app: 'idea64.exe', title: 'Main.kt', path: 'C:\\Dev\\easy' }), { kind: 'folder', value: 'C:\\Dev\\easy' });
  assert.deepEqual(L.ruleItem({ app: 'javaw.exe', title: 'Minecraft 26.3 - Multiplayer (3rd-party Server)', path: '', server: 'play.tricky.net:25565' }), { kind: 'minecraft', value: 'play.tricky.net:25565' });
  assert.deepEqual(L.ruleItem({ app: 'Figma.exe', title: 'Universe — App', path: '' }), { kind: 'figma', value: 'Universe — App' });
  assert.deepEqual(L.ruleItem({ app: 'chrome.exe', title: 'tricky-network/core', path: '' }), { kind: 'title', value: 'tricky-network/core' });
  assert.equal(L.ruleItem({ app: 'x.exe', title: '', path: '' }), null);
});
