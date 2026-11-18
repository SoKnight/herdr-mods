const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const code = fs.readFileSync(path.join(__dirname, '../mods/time-tracker/chip.js'), 'utf8');
const C = new Function(`${code.replace(/^export /gm, '')}\nreturn { SILENT_MS, silence, chipState, reminders };`)();
const M = 60_000;
const now = 1_000_000_000;

test('silence', () => {
  assert.equal(C.silence(false, now - 30_000, now), null);
  assert.equal(C.silence(false, 0, now), null); // no poll yet
  assert.deepEqual(C.silence(false, now - 2 * M, now), { since: now - 2 * M });
  assert.deepEqual(C.silence(true, 0, now), { since: 0 });
  assert.deepEqual(C.silence(true, now - 12 * M, now), { since: now - 12 * M });
});

test('chip states', () => {
  const st = { today_me_ms: 235 * M, state: 'working', project: 'tricky', project_name: 'Tricky Network', last_active: now - M, last_sample: now - 5000 };
  assert.deepEqual(C.chipState(null, 0, now), { kind: 'wait' });
  assert.deepEqual(C.chipState({ down: true }, 0, now), { kind: 'silent', since: 0 });
  assert.deepEqual(C.chipState({ down: true }, now - 12 * M, now), { kind: 'silent', since: now - 12 * M });
  assert.deepEqual(C.chipState({ error: 'config: bad' }, 0, now), { kind: 'error', text: 'config: bad' });
  assert.deepEqual(C.chipState(st, 0, now), { kind: 'working', me: 235 * M, project: 'tricky', name: 'Tricky Network', agents: 0 });
  assert.deepEqual(C.chipState({ ...st, state: 'away', last_active: now - 12 * M }, 0, now), { kind: 'away', me: 235 * M, away: 12 * M });
  // A session with no activity yet is not "away for 56 years".
  assert.deepEqual(C.chipState({ ...st, state: 'away', last_active: 0, project: '', project_name: '' }, 0, now), { kind: 'working', me: 235 * M, project: '', name: '', agents: 0 });
  assert.deepEqual(C.chipState({ ...st, state: 'none' }, 0, now), { kind: 'none', me: 235 * M });
  assert.deepEqual(C.chipState({ ...st, last_sample: now - 2 * M }, 0, now), { kind: 'silent', since: now - 2 * M });
});

test('chip states with agents', () => {
  const st = { today_me_ms: 235 * M, state: 'working', project: 'tricky', project_name: 'Tricky Network', last_active: now - M, last_sample: now - 5000 };
  const waiting = { project: 'easy', name: 'EasyLauncher', since: now - 4 * M };
  const w = { kind: 'waiting', me: 235 * M, wait: 4 * M, project: 'easy', name: 'EasyLauncher' };
  assert.deepEqual(C.chipState({ ...st, agents_working: 1, waiting }, 0, now), w);
  assert.deepEqual(C.chipState({ ...st, state: 'away', last_active: now - 12 * M, waiting }, 0, now), w);
  assert.deepEqual(C.chipState({ ...st, state: 'none', waiting }, 0, now), w);
  assert.deepEqual(C.chipState({ ...st, waiting: { ...waiting, since: now + 5000 } }, 0, now), { ...w, wait: 0 }); // clocks a little apart
  assert.deepEqual(C.chipState({ ...st, agents_working: 2, waiting: null }, 0, now), { kind: 'working', me: 235 * M, project: 'tricky', name: 'Tricky Network', agents: 2 });
  assert.deepEqual(C.chipState({ ...st, state: 'away', last_active: now - 12 * M, agents_working: 2 }, 0, now), { kind: 'away', me: 235 * M, away: 12 * M });
  assert.deepEqual(C.chipState({ ...st, waiting, last_sample: now - 2 * M }, 0, now), { kind: 'silent', since: now - 2 * M });
  assert.deepEqual(C.chipState({ down: true, waiting }, now - 2 * M, now), { kind: 'silent', since: now - 2 * M });
});

const F = new Function(`${fs.readFileSync(path.join(__dirname, '../mods/time-tracker/format.js'), 'utf8').replace(/^export /gm, '')}\nreturn { formatDuration };`)();
const day = (ms) => { const d = new Date(ms), p = (n) => String(n).padStart(2, '0'); return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`; };

test('reminders', () => {
  const R = (c, remind, sent = new Set()) => C.reminders(c, remind, now, sent, F.formatDuration);
  const quiet = (ago) => ({ kind: 'silent', since: now - ago });
  const on = { silent: true, close: '' };
  assert.deepEqual(R(quiet(11 * M), on), [{ key: `silent:${now - 11 * M}`, title: 'Time Tracker: сборщик молчит', body: 'Нет данных 11м — время сейчас не считается.' }]);
  assert.deepEqual(R(quiet(9 * M), on), []);
  assert.deepEqual(R(quiet(11 * M), on, new Set([`silent:${now - 11 * M}`])), []);
  assert.deepEqual(R(quiet(11 * M), { silent: false, close: '' }), []);
  assert.equal(R(quiet(11 * M), null).length, 1); // no answer yet: defaults
  assert.deepEqual(R({ kind: 'none', me: 0 }, { silent: true, close: '2026-10' }), [{ key: `close:2026-10:${day(now)}`, title: 'Time Tracker: октябрь не закрыт', body: 'Проверьте и закройте месяц в Отчёте.' }]);
  assert.deepEqual(R({ kind: 'none', me: 0 }, { silent: true, close: '2026-10' }, new Set([`close:2026-10:${day(now)}`])), []);
});
