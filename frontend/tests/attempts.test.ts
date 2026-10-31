import assert from 'node:assert/strict'
import test from 'node:test'
import { withAttemptTimeout } from '../src/platform/attempts.ts'

const stall = (signal: AbortSignal) => new Promise<never>((_, reject) => signal.addEventListener('abort', () => reject(signal.reason)))

test('a stalled attempt is retried and a later success is returned', async () => {
  let calls = 0
  const value = await withAttemptTimeout(signal => ++calls < 3 ? stall(signal) : Promise.resolve('ok'), new AbortController().signal, 20, 3)
  assert.equal(value, 'ok'); assert.equal(calls, 3)
})

test('attempts are bounded and other failures or caller cancellation are not retried', async () => {
  let calls = 0
  await assert.rejects(withAttemptTimeout(signal => { calls++; return stall(signal) }, new AbortController().signal, 10, 2))
  assert.equal(calls, 2)
  calls = 0
  await assert.rejects(withAttemptTimeout(async () => { calls++; throw new Error('NAVIGATION_DENIED') }, new AbortController().signal, 10, 3), /NAVIGATION_DENIED/)
  assert.equal(calls, 1)
  const caller = new AbortController(); calls = 0
  const pending = withAttemptTimeout(signal => { calls++; return stall(signal) }, caller.signal, 1000, 3)
  caller.abort(); await assert.rejects(pending)
  assert.equal(calls, 1)
})
