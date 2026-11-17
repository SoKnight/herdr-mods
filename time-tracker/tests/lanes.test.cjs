const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const code = fs.readFileSync(path.join(__dirname, '../mods/time-tracker/lanes.js'), 'utf8');
const L = new Function(`${code.replace(/^export /gm, '')}\nreturn { lanes, projectColor, targetFor, rangeFromDrag, correctionIds, shiftDay, ruleFor, stepDay, laneSpan, viewRange, axisStep };`)();

const H = 3600_000;
const day = { from: 0, to: 24 * H };
const seg = (start, end, extra = {}) => ({ start, end, app: 'idea64.exe', title: 't', project: 'easylauncher', activity: 'development', why: '', corrected: false, ...extra });
const timeline = (me, agents = []) => ({ power: [], nodata: [], agents_unknown: [], me, agents });
const lane = (all, id) => all.find((l) => l.id === id);

test('selection target', () => {
  assert.equal(L.targetFor(['me', 'project']), 'me');
  assert.equal(L.targetFor(['agents']), 'agents');
  assert.equal(L.targetFor(['project', 'agents']), 'both');
  assert.equal(L.targetFor(['power', 'agents']), 'agents');
});

test('drag maps to whole minutes in order', () => {
  const from = 0, to = 24 * 3600_000;
  assert.deepEqual(L.rangeFromDrag(600, 300, from, to, 1440), { start: 300 * 60_000, end: 600 * 60_000 });
  assert.deepEqual(L.rangeFromDrag(-5, 2000, from, to, 1440), { start: 0, end: to });
  assert.deepEqual(L.rangeFromDrag(100.2, 100.3, from, to, 1440), { start: 100 * 60_000, end: 101 * 60_000 });
});

test('bars are positioned and coloured by project order', () => {
  const all = L.lanes(timeline([seg(6 * H, 12 * H)]), day.from, day.to, 1440, ['universe', 'easylauncher']);
  assert.deepEqual(all.map((l) => l.id), ['power', 'me', 'app', 'project', 'activity', 'agents']);
  const [bar] = lane(all, 'project').bars;
  assert.equal(bar.x, 360);
  assert.equal(bar.w, 360);
  assert.equal(bar.value, 'easylauncher');
  assert.equal(bar.color, 'ansi2');
  assert.equal(lane(all, 'activity').bars[0].value, 'development');
  assert.equal(lane(all, 'activity').bars[0].color, 'muted'); // a kind the list doesn't name
  const kinds = L.lanes(timeline([seg(6 * H, 12 * H)]), day.from, day.to, 1440, [], ['design', 'development']);
  assert.equal(lane(kinds, 'activity').bars[0].color, 'ansi2');
  assert.equal(lane(all, 'app').bars[0].value, 'idea64.exe');
});

test('sub-pixel bars merge into a neighbour', () => {
  const t = 6 * H;
  const me = [seg(t, t + H), seg(t + H, t + H + 10_000, { project: 'universe' }), seg(t + H + 10_000, t + 2 * H)];
  const bars = lane(L.lanes(timeline(me), day.from, day.to, 1440, ['universe', 'easylauncher']), 'project').bars;
  assert.equal(bars.length, 1);
  assert.equal(bars[0].value, 'easylauncher');
  assert.equal(bars[0].x, 360);
  assert.equal(bars[0].w, 120);
  assert.equal(bars[0].segs.length, 3);
  // A visible gap keeps bars apart, and a different project is its own bar.
  const apart = lane(L.lanes(timeline([seg(t, t + H), seg(t + 2 * H, t + 3 * H), seg(t + 3 * H, t + 4 * H, { project: 'universe' })]), day.from, day.to, 1440, []), 'project').bars;
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

test('a drag covers whole lanes, clamped to the lanes', () => {
  assert.deepEqual(L.laneSpan(30, 5, 5, 22), { top: 0, bottom: 1 });
  assert.deepEqual(L.laneSpan(50, 200, 5, 22), { top: 2, bottom: 4 });
  assert.deepEqual(L.laneSpan(-10, 3, 5, 22), { top: 0, bottom: 0 });
});

test('the drawn range hugs the counted time, in whole hours, three at least', () => {
  const day = { from: 4 * H, to: 28 * H };
  assert.deepEqual(L.viewRange({ me: [], agents: [] }, day.from, day.to, 22 * H), day);
  const short = { me: [{ start: 21.5 * H, end: 21.7 * H }], agents: [{ start: 21.6 * H, end: 21.9 * H }] };
  assert.deepEqual(L.viewRange(short, day.from, day.to, 22.2 * H), { from: 20 * H, to: 23 * H });
  const long = { me: [{ start: 9.25 * H, end: 18.5 * H }], agents: [] };
  assert.deepEqual(L.viewRange(long, day.from, day.to, 30 * H), { from: 9 * H, to: 28 * H });
  const early = { me: [{ start: 4.2 * H, end: 4.4 * H }], agents: [] };
  assert.deepEqual(L.viewRange(early, day.from, day.to, 4.5 * H), { from: 4 * H, to: 7 * H });
  assert.equal(L.axisStep(3 * H), H);
  assert.equal(L.axisStep(10 * H), 2 * H);
  assert.equal(L.axisStep(24 * H), 3 * H);
});
