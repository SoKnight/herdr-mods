// client.js with a fake exec/appHome: the exe path, the --json flag and the missing-exe case.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const dir = path.join(__dirname, '../mods/time-tracker');
const strip = (f) => fs.readFileSync(path.join(dir, f), 'utf8').replace(/^import .*$/gm, '').replace(/^export /gm, '');
const load = (exec, exists = async () => true) =>
  new Function('exec', 'appHome', 'exists', `${strip('format.js')}\n${strip('client.js')}\nreturn call;`)(exec, () => 'H', exists);

test('call runs the installed exe with --json and parses the output', async () => {
  let seen;
  const call = load(async (cmd, args, opts) => ((seen = [cmd, args, opts]), { code: 0, stdout: '{"x":1}' }));
  assert.deepEqual(await call(['status']), { x: 1 });
  assert.deepEqual(seen, ['H/mods/time-tracker/bin/herdr-time.exe', ['status', '--json'], { timeout: 10000 }]);
});

test('a missing exe is down, other failures are errors', async () => {
  assert.deepEqual(await load(async () => { throw new Error('program not found'); })(['status']), { down: true });
  assert.deepEqual(await load(async () => { throw new Error('boom'); })(['status']), { error: 'boom' });
  assert.deepEqual(await load(async () => ({ code: 3, stdout: '{"error":"down"}' }))(['status']), { down: true });
});

test('an exe that is not there is down without running anything', async () => {
  let ran = false;
  const call = load(async () => ((ran = true), { code: 0, stdout: '{}' }), async () => false);
  assert.deepEqual(await call(['status']), { down: true });
  assert.equal(ran, false);
  assert.deepEqual(await load(async () => ({ code: 0, stdout: '{}' }), async () => { throw new Error('x'); })(['status']), { down: true });
});
