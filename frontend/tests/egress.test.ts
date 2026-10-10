import { test } from 'node:test';
import assert from 'node:assert/strict';
import { parseImport, exportData, type ProxyItem } from '../src/pages/egress.ts';

test('URL preview distinguishes valid, duplicate and invalid input without echoing secrets', () => {
 const rows = parseImport('socks5://alice:p%40ss@proxy.example:1080\nsocks5://alice:p%40ss@proxy.example:1080\nftp://proxy.example:1080\nsocks5://proxy.example:65536\nsocks5://proxy.example:1080/path', []);
 assert.deepEqual(rows.map(r => r.state), ['valid', 'duplicate', 'invalid', 'invalid', 'invalid']);
 assert.equal(rows[0].input?.password, 'p@ss');
 assert.ok(!rows[0].input?.name.includes('alice')); assert.ok(!rows[0].detail.includes('p@ss'));
});
test('export is explicit field allowlist and round trips metadata without credentials', () => {
 const row = { proxy_id: 'test', name: 'kept', protocol: 'socks5', host: 'proxy.example', port: 7891, status: 'active', tag: 'qa', note: 'note', expires_at: null, auth_configured: true, assigned_apps: [], revision: 3, probe: null, testing: false, updated_at: '', username: 'secret-user', password: 'secret-password' } as ProxyItem;
 const exported = exportData([row]); const text = JSON.stringify(exported);
 assert.ok(!text.includes('secret-user')); assert.ok(!text.includes('secret-password')); assert.ok(!text.includes('revision'));
 const back = parseImport(text, []); assert.equal(back[0].state, 'valid'); assert.equal(back[0].input?.note, 'note'); assert.equal(back[0].input?.status, 'pending'); assert.match(back[0].detail, /需重新填写/);
});
test('imports are bounded and unknown formats rejected', () => {
 assert.throws(() => parseImport(Array(51).fill('socks5://proxy.example:1080').join('\n'), []), /50/);
 assert.throws(() => parseImport('{"items":[]}', [])); assert.deepEqual(parseImport('', []), []);
});

test('HTTP default ports survive URL normalization; SOCKS requires an explicit port', () => {
 const rows = parseImport('http://proxy.example:80\nhttps://proxy.example:443\nsocks5://proxy.example', []);
 assert.deepEqual(rows.map(r => r.state), ['valid', 'valid', 'invalid']);
 assert.equal(rows[0].input?.port, 80); assert.equal(rows[1].input?.port, 443);
});

test('JSON preview rejects coerced fields and invalid management states', () => {
 const base = { name: 'proxy', protocol: 'socks5', host: 'proxy.example', port: 1080 };
 const rows = parseImport(JSON.stringify({ format: 'verdantflare-proxies-v1', items: [base, {...base, name: {}}, {...base, port: '1080'}, {...base, status: 'active'}] }), []);
 assert.deepEqual(rows.map(r => r.state), ['valid', 'invalid', 'invalid', 'invalid']);
});
