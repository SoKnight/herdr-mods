const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const code = fs.readFileSync(path.join(__dirname, '../mods/time-tracker/lanes.js'), 'utf8');
const L = new Function(`${code.replace(/^export /gm, '')}\nreturn { bars, projectColor, rangeFromDrag, correctionIds, shiftDay, ruleFor, stepDay, tickStep, ticks };`)();

const H = 3600_000;
const seg = (start, end, extra = {}) => ({ start, end, app: 'idea64.exe', title: 't', project: 'easylauncher', activity: 'development', why: '', corrected: false, ...extra });
const byProject = (me, order) => L.bars(me.map((s) => ({ start: s.start, end: s.end, value: s.project, seg: s })), 0, 24 * H, 1440, (v) => L.projectColor(v, order));

test('drag maps to whole minutes in order', () => {
  const from = 0, to = 24 * 3600_000;
  assert.deepEqual(L.rangeFromDrag(600, 300, from, to, 1440), { start: 300 * 60_000, end: 600 * 60_000 });
  assert.deepEqual(L.rangeFromDrag(-5, 2000, from, to, 1440), { start: 0, end: to });
  assert.deepEqual(L.rangeFromDrag(100.2, 100.3, from, to, 1440), { start: 100 * 60_000, end: 101 * 60_000 });
});

test('bars are positioned and coloured by project order', () => {
  const [bar] = L.bars([{ start: 6 * H, end: 12 * H, value: 'easylauncher', seg: {} }], 0, 24 * H, 1440, (v) => L.projectColor(v, ['universe', 'easylauncher']));
  assert.equal(bar.x, 360);
  assert.equal(bar.w, 360);
  assert.equal(bar.color, 'ansi2');
});

test('sub-pixel bars merge into a neighbour', () => {
  const t = 6 * H;
  const me = [seg(t, t + H), seg(t + H, t + H + 10_000, { project: 'universe' }), seg(t + H + 10_000, t + 2 * H)];
  const bars = byProject(me, ['universe', 'easylauncher']);
  assert.equal(bars.length, 1);
  assert.equal(bars[0].value, 'easylauncher');
  assert.equal(bars[0].x, 360);
  assert.equal(bars[0].w, 120);
  assert.equal(bars[0].segs.length, 3);
  // A visible gap keeps bars apart, and a different project is its own bar.
  const apart = byProject([seg(t, t + H), seg(t + 2 * H, t + 3 * H), seg(t + 3 * H, t + 4 * H, { project: 'universe' })], []);
  assert.equal(apart.length, 3);
});

test('project colours', () => {
  assert.equal(L.projectColor('', ['a']), 'dim');
  assert.equal(L.projectColor('a', ['a', 'b']), 'ansi1');
  assert.equal(L.projectColor('g', ['a', 'b', 'c', 'd', 'e', 'f', 'g']), 'ansi1');
  assert.equal(L.projectColor('zzz', ['a']), 'muted');
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

test('rules match a title as written', () => {
  assert.deepEqual(L.ruleFor({ kind: 'path', value: 'C:\\Dev\\new' }, 'easy'), { project: 'easy', kind: 'path', value: 'C:\\Dev\\new' });
  assert.deepEqual(L.ruleFor({ kind: 'title', value: 'a (1).kt' }, 'easy'), { project: 'easy', kind: 'title', value: '^a \\(1\\)\\.kt$' });
  const odd = '[x] $1 ^a|b \\ c+';
  const { value } = L.ruleFor({ kind: 'title', value: odd }, 'easy');
  assert.equal(value, '^\\[x\\] \\$1 \\^a\\|b \\\\ c\\+$');
  assert.ok(new RegExp(value).test(odd));
  assert.ok(!new RegExp(value).test(`${odd}!`));
  assert.deepEqual(L.ruleFor({ kind: 'figma', value: 'Launcher (v2)' }, 'easy'), { project: 'easy', kind: 'figma', value: '^Launcher \\(v2\\)$' });
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
