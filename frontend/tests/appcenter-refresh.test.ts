import assert from 'node:assert/strict'
import test from 'node:test'
import { createSharedRefresh } from '../src/app-center/refresh.ts'

function deferred() { let resolve!: () => void; const promise = new Promise<void>(done => { resolve = done }); return { promise, resolve } }

test('expired-view consumers await the same fresh read instead of canceling one another', async () => {
  const read = deferred(); let calls = 0, fresh = false, ticks = 0
  const shared = createSharedRefresh(async () => { calls++; await read.promise; fresh = true }, () => ticks++)
  const waiting = Array.from({ length: 32 }, () => shared.refresh())
  assert.ok(waiting.every(p => p === waiting[0]))
  await Promise.resolve(); assert.equal(calls, 1); assert.equal(fresh, false)
  read.resolve(); await Promise.all(waiting)
  assert.equal(fresh, true); assert.equal(ticks, 1)
})

test('identity or visibility invalidation cannot let the old completion clear a new read', async () => {
  const first = deferred(), next = deferred(); let calls = 0, ticks = 0
  const shared = createSharedRefresh(() => ++calls === 1 ? first.promise : next.promise, () => ticks++)
  const old = shared.refresh(); await Promise.resolve()
  shared.invalidate(); const current = shared.refresh(); await Promise.resolve()
  first.resolve(); await old
  assert.equal(ticks, 0); assert.equal(shared.refresh(), current); assert.equal(calls, 2)
  next.resolve(); await current; assert.equal(ticks, 1)
})

test('invalidated queued reads never start; a failed read permits a later retry', async () => {
  let calls = 0, ticks = 0
  const shared = createSharedRefresh(async () => { calls++; if (calls === 1) throw Error('offline') }, () => ticks++)
  const queued = shared.refresh(); shared.invalidate(); await queued; assert.equal(calls, 0)
  await assert.rejects(shared.refresh(), /offline/)
  await shared.refresh(); assert.equal(calls, 2); assert.equal(ticks, 2)
})

test('hints during a read coalesce into one following read without canceling current work', async () => {
  const first = deferred(), next = deferred(); let calls = 0, ticks = 0
  const shared = createSharedRefresh(() => ++calls === 1 ? first.promise : next.promise, () => ticks++)
  const initial = shared.refresh(); await Promise.resolve()
  for (let i = 0; i < 50; i++) assert.equal(shared.refreshLatest(), initial)
  first.resolve(); await Promise.resolve(); await Promise.resolve()
  assert.equal(calls, 2); assert.equal(ticks, 0)
  next.resolve(); await initial
  assert.equal(ticks, 1)
})

test('scope invalidation discards a queued notification read', async () => {
  const first = deferred(); let calls = 0
  const shared = createSharedRefresh(async () => { calls++; await first.promise }, () => {})
  const initial = shared.refresh(); await Promise.resolve()
  shared.refreshLatest(); shared.invalidate(); first.resolve(); await initial
  assert.equal(calls, 1)
})
