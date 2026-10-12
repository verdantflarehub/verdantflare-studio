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

test('Dashboard Date uses service time while explicit construction and native identity remain intact', () => {
  const realm: any = { Date, setTimeout, clearTimeout, setInterval, clearInterval }
  let server = 123456
  const dispose = installDashboardRuntime(realm,()=>server)
  assert.equal(realm.Date.now(),server)
  assert.equal(new realm.Date().getTime(),server)
  assert.equal(realm.Date(),new Date(server).toString())
  assert.equal(new realm.Date(0).getTime(),0)
  assert.equal(new realm.Date('2020-01-01').toISOString(),'2020-01-01T00:00:00.000Z')
  assert.equal(realm.Date.UTC(2020,0,1),Date.UTC(2020,0,1))
  assert.equal(realm.Date.parse('2020-01-01'),Date.parse('2020-01-01'))
  assert.ok(new realm.Date() instanceof realm.Date)
  assert.ok(new realm.Date() instanceof Date)
  class Child extends realm.Date {}
  assert.ok(new Child() instanceof Child)
  server+=1000
  assert.equal(new realm.Date().getTime(),server)
  dispose()
})
