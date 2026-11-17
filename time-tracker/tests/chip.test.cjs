const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const code = fs.readFileSync(path.join(__dirname, '../mods/time-tracker/chip.js'), 'utf8');
const C = new Function(`${code.replace(/^export /gm, '')}\nreturn { SILENT_MS, silence, chipState };`)();
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
  assert.deepEqual(C.chipState(st, 0, now), { kind: 'working', me: 235 * M, project: 'tricky', name: 'Tricky Network' });
  assert.deepEqual(C.chipState({ ...st, state: 'away', last_active: now - 12 * M }, 0, now), { kind: 'away', me: 235 * M, away: 12 * M });
  // A session with no activity yet is not "away for 56 years".
  assert.deepEqual(C.chipState({ ...st, state: 'away', last_active: 0, project: '', project_name: '' }, 0, now), { kind: 'working', me: 235 * M, project: '', name: '' });
  assert.deepEqual(C.chipState({ ...st, state: 'none' }, 0, now), { kind: 'none', me: 235 * M });
  assert.deepEqual(C.chipState({ ...st, last_sample: now - 2 * M }, 0, now), { kind: 'silent', since: now - 2 * M });
});
