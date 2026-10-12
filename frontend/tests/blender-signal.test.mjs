import assert from 'node:assert/strict';
import test from 'node:test';
import vm from 'node:vm';
import { readFileSync } from 'node:fs';

const script = readFileSync(new URL('../../internal/transport/desktop/blender_signal.js', import.meta.url), 'utf8');
const workspace = '/studio/legacy-blender/01980000-0000-4000-8000-000000000001';
const signalId = '01980000-0000-4000-8000-000000000002';
const path = workspace + '/signals/' + signalId;
const session = 's'.repeat(43), origin = 'http://wails.localhost';
const pathname = '/apps/blender/blenderA/' + session + '/';
const url = 'ws://wails.localhost' + pathname + 'webrtc/signalling/';
const tick = () => new Promise(resolve => setImmediate(resolve));
function deferred() { let resolve, reject; const promise = new Promise((yes, no) => { resolve = yes; reject = no; }); return { promise, resolve, reject }; }
function fixture(handler, dataset = {}) {
  const calls = [], events = new EventTarget();
  const window = { addEventListener: events.addEventListener.bind(events), fetch: async (input, init) => {
    calls.push({ input, init });
    assert.equal(init.credentials, 'omit'); assert.equal(init.redirect, 'error');
    if (init.method === 'DELETE') return new Response(null, { status: 204 });
    if (handler) { const result = handler(input, init); if (result !== undefined) return result; }
    if (input === workspace + '/signals') return Response.json({ signalId });
    if (init.method === 'POST') return new Response(null, { status: 204 });
    return new Promise((_, reject) => init.signal.addEventListener('abort', () => reject(init.signal.reason), { once: true }));
  } };
  class CloseEvent extends Event { constructor(type, values) { super(type); Object.assign(this, values); } }
  const context = vm.createContext({ window, document: { currentScript: { dataset: { workspace, alias: 'blenderA', session, ...dataset } } },
    location: { origin, pathname }, URL, EventTarget, Event, MessageEvent, CloseEvent, DOMException, TextEncoder, TextDecoder, AbortController, AbortSignal, queueMicrotask });
  vm.runInContext(script, context);
  return { calls, events, Socket: window.WebSocket };
}

test('fixed editor URL, configuration and subprotocols fail closed without a native fallback', () => {
  const { Socket, calls } = fixture();
  for (const target of [url + '?x', url + '#x', url.replace('blenderA', 'blenderB'), url.replace('signalling', '%73ignalling'), 'wss://other.invalid/socket', '/webrtc/signalling/']) assert.throws(() => new Socket(target));
  assert.throws(() => new Socket(url, 'protocol'));
  assert.throws(() => new Socket(url, ['protocol']));
  assert.equal(calls.length, 0);
  assert.throws(() => new (fixture(undefined, { workspace: workspace + '/extra' }).Socket)(url));
});

test('opens once, dispatches text in order and serializes sends without replay', async () => {
  const firstSend = deferred(); let sends = 0, polls = 0;
  const { Socket, calls } = fixture((_url, init) => {
    if (init.method === 'GET' && polls++ === 0) return Response.json({ message: 'peer-sdp' });
    if (_url === path && init.method === 'POST' && ++sends === 1) return firstSend.promise;
  });
  const socket = new Socket(new URL(url)), received = [], opened = deferred();
  socket.onopen = () => opened.resolve();
  socket.addEventListener('message', event => received.push(event.data));
  assert.equal(socket.readyState, Socket.CONNECTING);
  assert.throws(() => socket.send('early'), { name: 'InvalidStateError' });
  await opened.promise;
  socket.send('first'); socket.send('第二');
  assert.equal(sends, 1); assert.equal(socket.bufferedAmount, 11);
  firstSend.resolve(new Response(null, { status: 204 }));
  await tick(); await tick();
  assert.equal(sends, 2); assert.equal(socket.bufferedAmount, 0);
  assert.deepEqual(received, ['peer-sdp']);
  assert.deepEqual(calls.filter(call => call.input === path && call.init.method === 'POST').map(call => JSON.parse(call.init.body).message), ['first', '第二']);
  assert.throws(() => new Socket(url), { name: 'InvalidStateError' });
  socket.close(); await tick();
  assert.equal(socket.readyState, Socket.CLOSED);
  assert.equal(calls.filter(call => call.init.method === 'DELETE').length, 1);
});

test('late opening response after close releases its original signal and never opens', async () => {
  const opening = deferred(); const { Socket, calls } = fixture(() => opening.promise);
  const socket = new Socket(url); let opened = 0, closed = 0;
  socket.onopen = () => opened++;
  socket.onclose = () => closed++;
  socket.close(); opening.resolve(Response.json({ signalId }));
  await tick(); await tick();
  assert.equal(opened, 0); assert.equal(closed, 1);
  assert.equal(calls.filter(call => call.init.method === 'GET').length, 0);
  assert.equal(calls.at(-1).input, path); assert.equal(calls.at(-1).init.method, 'DELETE');
});

test('ambiguous send failure closes, cancels reads and never retries queued messages', async () => {
  let sends = 0;
  const { Socket, calls } = fixture((_url, init) => { if (_url === path && init.method === 'POST') { sends++; return Promise.reject(Error('lost response')); } });
  const socket = new Socket(url), opened = deferred(), events = [];
  socket.onopen = () => opened.resolve(); socket.onerror = () => events.push('error'); socket.onclose = event => events.push(event.code);
  await opened.promise; socket.send('original'); socket.send('queued');
  await tick();
  assert.equal(sends, 1); assert.equal(socket.bufferedAmount, 0); assert.deepEqual(events, ['error', 1006]);
  assert.ok(calls.find(call => call.init.method === 'GET').init.signal.aborted);
});

test('rejects binary, malformed text, oversized messages and outgoing queue overflow', async () => {
  for (const value of [new Uint8Array([1]), '\ud800', 'x'.repeat((256 << 10) + 1), '😀'.repeat(65537), 'queue']) {
    const pending = deferred();
    const { Socket } = fixture((_url, init) => { if (_url === path && init.method === 'POST') return pending.promise; });
    const socket = new Socket(url), opened = deferred(); socket.onopen = () => opened.resolve();
    await opened.promise;
    if (value === 'queue') for (let index = 0; index < 33; index++) socket.send('queued'); else socket.send(value);
    await tick(); assert.equal(socket.readyState, Socket.CLOSED);
    pending.resolve(new Response(null, { status: 204 }));
  }
});

test('invalid incoming envelopes close and pagehide cancels a live poll', async () => {
  for (const response of [() => Response.json({ message: 'x', extra: true }), () => Response.json({ message: '\ud800' }),
    () => Response.json({ message: 'x'.repeat((256 << 10) + 1) }), () => new Response('bad', { headers: { 'Content-Type': 'text/plain' } })]) {
    const { Socket } = fixture((_url, init) => init.method === 'GET' ? response() : undefined);
    const socket = new Socket(url); await tick(); await tick(); assert.equal(socket.readyState, Socket.CLOSED);
  }
  const { Socket, calls, events } = fixture(); const socket = new Socket(url);
  await tick(); events.dispatchEvent(new Event('pagehide')); await tick();
  assert.equal(socket.readyState, Socket.CLOSED);
  assert.ok(calls.find(call => call.init.method === 'GET').init.signal.aborted);
  assert.equal(calls.at(-1).init.method, 'DELETE');
});
