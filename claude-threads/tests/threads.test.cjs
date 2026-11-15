// threads.js: Claude Code's prompt history grouped into sessions, searched, and matched to open terminals.
const { test } = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

const code = fs.readFileSync(path.join(__dirname, '../mods/claude-threads/threads.js'), 'utf8');
const T = new Function(`${code.replace(/^export /gm, '')}\nreturn { sessionsFromHistory, projectDir, searchSessions, openSessions, oneLine, ago };`)();

const line = (sessionId, display, timestamp, project = 'C:\\proj') => JSON.stringify({ display, pastedContents: {}, timestamp, project, sessionId });

test('prompts group into sessions, newest first', () => {
  const text = [line('a', 'fix the login', 1), line('b', 'add a panel', 2), line('a', 'and the logout', 3)].join('\n') + '\n';
  const sessions = T.sessionsFromHistory(text);
  assert.deepEqual(sessions.map((s) => s.id), ['a', 'b']);
  assert.deepEqual(sessions[0], { id: 'a', project: 'C:\\proj', first: 'fix the login', last: 'and the logout', count: 2, updated: 3, prompts: ['fix the login', 'and the logout'] });
});

test('slash commands are not titles', () => {
  const [s] = T.sessionsFromHistory([line('a', '/clear', 1), line('a', '/model', 2)].join('\n'));
  assert.equal(s.first, null);
  assert.equal(s.last, null);
  assert.equal(s.count, 2);
  const [t] = T.sessionsFromHistory([line('a', '/clear', 1), line('a', 'real work', 2)].join('\n'));
  assert.equal(t.first, 'real work');
});

test('a half-written last line and lines without a session are skipped', () => {
  const text = [JSON.stringify({ display: 'old', timestamp: 0, project: '/p' }), line('a', 'ok', 1), '{"display":"cut off'].join('\n');
  assert.deepEqual(T.sessionsFromHistory(text).map((s) => s.id), ['a']);
});

test('projectDir names the folder the way Claude Code does', () => {
  assert.equal(T.projectDir('C:\\Users\\SoKnight\\Development\\opensource\\herdr-mods'), 'C--Users-SoKnight-Development-opensource-herdr-mods');
  assert.equal(T.projectDir('/home/sam/my.app'), '-home-sam-my-app');
  assert.equal(T.projectDir('C:\\Учёба\\1'), 'C--------1');
});

test('search ignores case and shows the last matching prompt', () => {
  const sessions = T.sessionsFromHistory([line('a', 'Fix login', 1), line('a', 'more LOGIN work', 2), line('a', 'unrelated', 3), line('b', 'panel', 4)].join('\n'));
  assert.deepEqual(T.searchSessions(sessions, 'login').map((r) => [r.session.id, r.hit]), [['a', 'more LOGIN work']]);
  assert.deepEqual(T.searchSessions(sessions, '  ').map((r) => [r.session.id, r.hit]), [['b', null], ['a', null]]);
});

test('a session is open only while a terminal runs its process', () => {
  const registry = [{ pid: 10, sessionId: 'a' }, { pid: 11, sessionId: 'b' }];
  const open = T.openSessions(registry, [{ address: 't1', pid: 10 }, { address: 't2', pid: null }]);
  assert.deepEqual([...open], [['a', 't1']]);
});

test('text shows on one line, ages are short', () => {
  assert.equal(T.oneLine('a\n\tb\u0007 '), 'a b');
  assert.equal(T.oneLine(null), '');
  const now = 10 * 86400000;
  assert.equal(T.ago(now - 30000, now), 'just now');
  assert.equal(T.ago(now - 12 * 60000, now), '12m ago');
  assert.equal(T.ago(now - 3 * 3600000, now), '3h ago');
  assert.equal(T.ago(now - 6 * 86400000, now), '6d ago');
});
