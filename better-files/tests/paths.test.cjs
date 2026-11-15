// paths.js: one spelling for Windows paths, POSIX ones untouched, and every mod's copy the same.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const file = path.join(__dirname, '../mods/better-files/paths.js');
const canonical = fs.readFileSync(file, 'utf8');
const P = new Function(`${canonical.replace(/^export /gm, '')}\nreturn { isWindowsPath, normPath, samePath, insidePath, relativePath, joinPath, baseName, nativePath, psQuote };`)();

test('a Windows path is told by its shape', () => {
  for (const p of ['C:\\x', 'c:/x', 'C:', '\\\\host\\share', '//host/share']) assert.ok(P.isWindowsPath(p), p);
  for (const p of ['/home/x', 'src/a.ts', '~/x', '', null, 'C', '//']) assert.ok(!P.isWindowsPath(p), String(p));
});

test('normPath gives Windows paths one spelling and leaves POSIX ones alone', () => {
  assert.equal(P.normPath('c:\\Users\\x\\proj\\'), 'C:/Users/x/proj');
  assert.equal(P.normPath('C:\\Users\\x/proj/src'), 'C:/Users/x/proj/src');
  assert.equal(P.normPath('C:\\'), 'C:/');
  assert.equal(P.normPath('c:'), 'C:/');
  assert.equal(P.normPath('\\\\host\\share\\a'), '//host/share/a');
  assert.equal(P.normPath('/home/x/proj/'), '/home/x/proj/');
  assert.equal(P.normPath(undefined), '');
});

test('Windows paths compare without case, POSIX ones with it', () => {
  assert.ok(P.samePath('C:\\Users\\X\\Proj', 'c:/users/x/proj/'));
  assert.ok(!P.samePath('/home/X', '/home/x'));
  assert.ok(P.insidePath('C:\\proj\\src\\a.ts', 'c:/Proj'));
  assert.ok(P.insidePath('C:/proj', 'C:\\proj'));
  assert.ok(!P.insidePath('C:/project', 'C:/proj'));
  assert.ok(P.insidePath('C:/a', 'C:\\'));
  assert.ok(P.insidePath('/repo/src', '/repo'));
  assert.ok(!P.insidePath('/repos', '/repo'));
});

test('relativePath and joinPath', () => {
  assert.equal(P.relativePath('C:\\proj\\src\\a.ts', 'C:/proj'), 'src/a.ts');
  assert.equal(P.relativePath('C:/proj', 'c:\\proj'), '');
  assert.equal(P.relativePath('C:/a.md', 'C:\\'), 'a.md');
  assert.equal(P.relativePath('/elsewhere', '/repo'), null);
  assert.equal(P.joinPath('C:\\proj', 'src/a.ts'), 'C:/proj/src/a.ts');
  assert.equal(P.joinPath('C:\\', 'a.md'), 'C:/a.md');
  assert.equal(P.joinPath('/repo', 'a.md'), '/repo/a.md');
  assert.equal(P.joinPath('/', 'a.md'), '/a.md');
  assert.equal(P.joinPath('/repo', ''), '/repo');
});

test('baseName, nativePath and psQuote', () => {
  assert.equal(P.baseName('C:\\Users\\x\\proj'), 'proj');
  assert.equal(P.baseName('/repo/src/'), 'src');
  assert.equal(P.nativePath('c:/Users/x/a.md'), 'C:\\Users\\x\\a.md');
  assert.equal(P.nativePath('/repo/a.md'), '/repo/a.md');
  assert.equal(P.psQuote("it's ‘x’"), "'it''s ‘‘x’’'");
});

test('every mod keeps an identical copy of paths.js', () => {
  const repo = path.join(__dirname, '../..');
  const copies = [];
  const walk = (dir) => {
    for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
      if (entry.name === 'node_modules' || entry.name.startsWith('.')) continue;
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) walk(full);
      else if (entry.name === 'paths.js') copies.push(full);
    }
  };
  walk(repo);
  for (const copy of copies) assert.equal(fs.readFileSync(copy, 'utf8'), canonical, path.relative(repo, copy));
});
