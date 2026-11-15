const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const root = path.resolve(__dirname, '..');
const runtime = path.join(root, 'mods/claude-threads');
const js = () => fs.readdirSync(runtime).filter((f) => f.endsWith('.js'));

test('the mod stands alone and opens as a popup', () => {
  const code = js().map((f) => fs.readFileSync(path.join(runtime, f), 'utf8')).join('\n');
  assert.doesNotMatch(code, /better-|branch-|panel-body|estro/);
  assert.deepEqual(JSON.parse(fs.readFileSync(path.join(runtime, 'mod.json'))), { $schema: './mod.schema.json', version: '1.0.0', place: 'none', title: 'Threads', overlay: { width: 400, height: 560 } });
  for (const file of ['README.md', 'RECIPE.md', 'preview.png', 'sync.sh', 'sync.ps1', 'mods/claude-threads/icons/threads.svg'])
    assert.ok(fs.existsSync(path.join(root, file)), file);
});

test('every name imported from another file of the mod is exported there', () => {
  for (const file of js()) {
    const code = fs.readFileSync(path.join(runtime, file), 'utf8');
    for (const [, names, from] of code.matchAll(/^import \{([^}]*)\} from "(\.[^"]+)";/gm)) {
      const target = fs.readFileSync(path.join(runtime, from), 'utf8');
      for (const name of names.split(',').map((n) => n.trim()).filter(Boolean))
        assert.match(target, new RegExp(`^export (async )?(function|const|class) ${name}\\b`, 'm'), `${file} imports ${name} from ${from}`);
    }
  }
});
