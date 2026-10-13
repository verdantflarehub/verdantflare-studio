import test from 'node:test'
import assert from 'node:assert/strict'
import { installDashboardRuntime } from '../src/app-dashboard/runtime.ts'

test('Dashboard extracted timers retain their Window receiver and dispose pending work', () => {
  let next = 0
  const work = new Map<number, { handler: Function; args: unknown[] }>()
  const realm: any = { Date,
    setTimeout(this: unknown, handler: Function, _delay: number, ...args: unknown[]) {
      assert.equal(this, realm); work.set(++next, {handler,args}); return next
    },
    clearTimeout(this: unknown, id: number) { assert.equal(this, realm); work.delete(id) },
  }
  realm.setInterval = realm.setTimeout; realm.clearInterval = realm.clearTimeout
  const original = Date.now
  const dispose = installDashboardRuntime(realm, () => 123456)
  const timers = { set: realm.setTimeout, clear: realm.clearTimeout, interval: realm.setInterval }
  let called = 0
  const handle = timers.set(function(this: unknown, n: number) { assert.equal(this,realm);called+=n },0,2)
  const scheduled = work.get(handle)!
  scheduled.handler(...scheduled.args)
  assert.equal(called,2)
  timers.clear(handle)
  timers.interval(()=>called++,10)
  timers.set(()=>called++,10)
  dispose();dispose()
  assert.equal(work.size,0)
  assert.equal(timers.set(()=>called++,0),0)
  assert.equal(Date.now,original)
})

test('Dashboard runtime preserves native Date without global proxying', () => {
  const realm: any = { Date, setTimeout, clearTimeout, setInterval, clearInterval }
  let server = 123456
  const dispose = installDashboardRuntime(realm, () => server)
  assert.equal(realm.Date, Date)
  assert.equal(typeof realm.Date.now, 'function')
  dispose()
})
