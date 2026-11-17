const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '..');
const runtime = path.join(root, 'mods/time-tracker');
const js = (dir = '') => fs.readdirSync(path.join(runtime, dir)).filter((f) => f.endsWith('.js')).map((f) => path.join(dir, f));

test('the mod stands alone and opens as an overlay', () => {
  const code = [...js(), ...js('views')].map((f) => fs.readFileSync(path.join(runtime, f), 'utf8')).join('\n');
  assert.doesNotMatch(code, /better-|branch-|panel-body|estro|claude-threads/);
  assert.deepEqual(JSON.parse(fs.readFileSync(path.join(runtime, 'mod.json'))), { $schema: './mod.schema.json', version: '1.0.0', place: 'none', title: 'Time', overlay: { width: 920, height: 580 } });
  for (const file of ['sync.sh', 'sync.ps1', 'mods/time-tracker/icons/clock.svg']) assert.ok(fs.existsSync(path.join(root, file)), file);
});

test('the mod takes no settings', () => {
  const schema = JSON.parse(fs.readFileSync(path.join(runtime, 'mod.schema.json')));
  assert.ok(!('settings' in schema.properties));
  assert.match(schema.description, /no settings/);
});

test('sync keeps the installed bin folder', () => {
  assert.match(fs.readFileSync(path.join(root, 'sync.sh'), 'utf8'), /--exclude bin\b/);
  assert.match(fs.readFileSync(path.join(root, 'sync.ps1'), 'utf8'), /\/XD bin\b/);
});

test('every name imported from another file of the mod is exported there', () => {
  for (const file of [...js(), ...js('views')]) {
    const code = fs.readFileSync(path.join(runtime, file), 'utf8');
    for (const [, names, from] of code.matchAll(/^import \{([^}]*)\} from "(\.[^"]+)";/gm)) {
      const target = fs.readFileSync(path.join(runtime, path.dirname(file), from), 'utf8');
      for (const name of names.split(',').map((n) => n.trim()).filter(Boolean))
        assert.match(target, new RegExp(`^export (async )?(function|const|class) ${name}\\b`, 'm'), `${file} imports ${name} from ${from}`);
    }
  }
});
