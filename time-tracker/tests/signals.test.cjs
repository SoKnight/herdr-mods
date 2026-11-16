const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const code = fs.readFileSync(path.join(__dirname, '../mods/time-tracker/signals.js'), 'utf8');
const S = new Function(`${code.replace(/^export /gm, '')}\nreturn { beatFor, shouldSend };`)();

test('beat uses the focused terminal folder, else the shown space folder', () => {
  assert.equal(S.beatFor({ cwd: 'C:\\Dev\\easyx\\EasyLauncher' }, [], 1).project_path, 'C:\\Dev\\easyx\\EasyLauncher');
  assert.equal(S.beatFor({ cwd: null }, [{ active: true, folder: 'C:\\Dev\\tricky-network' }], 1).project_path, 'C:\\Dev\\tricky-network');
  assert.equal(S.beatFor(null, [], 1), null);
  assert.deepEqual(S.beatFor({ cwd: 'P' }, [], 7), { source: 'herdr', time: 7, app: 'herdr', project_path: 'P', focused: true });
});

test('send on folder change or every 30 s', () => {
  const a = { project_path: 'A' };
  assert.equal(S.shouldSend({ beat: a, at: 0 }, a, 29999), false);
  assert.equal(S.shouldSend({ beat: a, at: 0 }, a, 30000), true);
  assert.equal(S.shouldSend({ beat: a, at: 0 }, { project_path: 'B' }, 1), true);
  assert.equal(S.shouldSend(null, a, 1), true);
});
