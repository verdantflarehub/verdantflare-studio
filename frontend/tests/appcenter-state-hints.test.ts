import assert from 'node:assert/strict'
import test from 'node:test'
import { createStateCalibration, subscribeStateHints } from '../src/app-center/state-hints.ts'

test('with a hint stream there is no periodic read: one read per activation and per hint', t => {
  t.mock.timers.enable({ apis: ['setInterval'] })
  let calls = 0, notify = () => {}, closed = 0
  const calibration = createStateCalibration(async () => { calls++ }, callback => { notify = callback; return () => { closed++ } })
  calibration.activate('scope-a'); assert.equal(calls, 1)
  t.mock.timers.tick(10 * 60000); assert.equal(calls, 1)
  for (let i = 0; i < 3; i++) notify()
  assert.equal(calls, 4)
  calibration.activate(null); assert.equal(closed, 1)
  notify(); t.mock.timers.tick(60000); assert.equal(calls, 4)
  calibration.activate('scope-a'); assert.equal(calls, 5)
  calibration.dispose()
})

test('desktop or unsupported subscription retains calibration and obsolete scope callbacks are ignored', t => {
  t.mock.timers.enable({ apis: ['setInterval'] })
  let calls = 0, old = () => {}
  const fallback = createStateCalibration(async () => { calls++ })
  fallback.activate('desktop'); t.mock.timers.tick(30000); assert.equal(calls, 2); fallback.dispose()
  const scoped = createStateCalibration(async () => { calls++ }, notify => { old = notify; return () => {} })
  scoped.activate('a'); const obsolete = old; scoped.activate('b'); const before = calls
  obsolete(); assert.equal(calls, before)
  old(); assert.equal(calls, before + 1); scoped.dispose()
})

test('SSE reconnect, malformed data and late messages only request scoped refreshes', t => {
  t.mock.timers.enable({ apis: ['setTimeout'] })
  const streams: { listener?: (event: MessageEvent) => void; onerror?: () => void; closed: boolean }[] = []
  let refreshes = 0
  const close = subscribeStateHints(() => { refreshes++ }, () => {
    const stream = { closed: false, listener: undefined as ((event: MessageEvent) => void) | undefined, onerror: undefined as (() => void) | undefined,
      close() { this.closed = true }, addEventListener(_: string, listener: (event: MessageEvent) => void) { this.listener = listener } }
    streams.push(stream); return stream as unknown as EventSource
  })
  const emit = (index: number, data: string) => streams[index].listener?.({ data } as MessageEvent)
  emit(0, '{"schemaVersion":1,"revision":"boot-z:10"}'); emit(0, '{"schemaVersion":1,"revision":"boot-a:1"}')
  assert.equal(refreshes, 2)
  streams[0].onerror?.(); assert.equal(streams[0].closed, true); assert.equal(refreshes, 3)
  emit(0, '{"schemaVersion":1,"revision":"late"}'); assert.equal(refreshes, 3)
  t.mock.timers.tick(10000); assert.equal(streams.length, 2)
  emit(1, '{"schemaVersion":1,"revision":"private","state":"ready"}')
  assert.equal(streams[1].closed, true); assert.equal(refreshes, 4)
  close(); t.mock.timers.tick(10000); assert.equal(streams.length, 2)
})
