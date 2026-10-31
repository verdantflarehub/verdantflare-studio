import assert from 'node:assert/strict'
import test from 'node:test'
import { withHedge } from '../src/platform/attempts.ts'

const stall = (signal: AbortSignal) => new Promise<never>((_, reject) => signal.addEventListener('abort', () => reject(signal.reason)))
const later = <T>(ms: number, value: T, signal: AbortSignal) => new Promise<T>((resolve, reject) => { const t = setTimeout(() => resolve(value), ms); signal.addEventListener('abort', () => { clearTimeout(t); reject(signal.reason) }) })

test('a fast read never hedges', async () => {
  let calls = 0
  assert.equal(await withHedge(signal => { calls++; return later(5, 'ok', signal) }, new AbortController().signal, 50), 'ok')
  assert.equal(calls, 1)
})

test('a stalled read is hedged, the first success wins and the rest are aborted', async () => {
  let calls = 0, stalledAborted = false
  const value = await withHedge(signal => { calls++; if (calls === 1) { signal.addEventListener('abort', () => { stalledAborted = true }); return stall(signal) } return later(5, 'second', signal) }, new AbortController().signal, 20, 3, 1000)
  assert.equal(value, 'second'); assert.equal(calls, 2); assert.equal(stalledAborted, true)
})

test('attempts are bounded, real failures settle at once and caller cancellation stops all', async () => {
  let calls = 0
  await assert.rejects(withHedge(signal => { calls++; return stall(signal) }, new AbortController().signal, 10, 2, 30))
  assert.equal(calls, 2)
  calls = 0
  await assert.rejects(withHedge(async () => { calls++; throw new Error('NAVIGATION_DENIED') }, new AbortController().signal, 10, 3), /NAVIGATION_DENIED/)
  assert.equal(calls, 1)
  const caller = new AbortController(); calls = 0
  const pending = withHedge(signal => { calls++; return stall(signal) }, caller.signal, 1000, 3, 5000)
  caller.abort(); await assert.rejects(pending)
  assert.equal(calls, 1)
})
