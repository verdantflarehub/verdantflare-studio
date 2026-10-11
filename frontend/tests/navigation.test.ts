import assert from 'node:assert/strict'
import test from 'node:test'
import { createNavigationController, decodeNavigation } from '../src/navigation/controller.ts'
import { createNavigationTransport } from '../src/navigation/transport.ts'
import type { NavigationEnvelope, NavigationPreference, NavigationIdentity } from '../src/navigation/controller.ts'

const identity: NavigationIdentity = { userId: '019a00b0-0000-7000-8000-000000000001', organizationId: '019a00b0-0000-7000-8000-000000000002', stationId: '019a00b0-0000-7000-8000-000000000003' }
function envelope(who = identity): NavigationEnvelope {
  return { mode: 'preinstalled', preferenceRevision: '12', projection: { scope: { ...who, authorizationEpoch: 'epoch1' }, revision: `sha256:${'a'.repeat(64)}`, entries: [{
    installationId: '019a00b0-0000-7000-8000-000000000004', appId: 'project', appVersion: '0.1.0', entryId: 'main', title: '项目', iconRef: 'folder', route: '/', group: 'applications', order: 10, pinned: true, pinSource: 'default', availability: 'unknown',
  }] } }
}
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(yes => { resolve = yes }); return { promise, resolve } }

test('validates the current scope, routes and explicit migration mode', () => {
  assert.equal(decodeNavigation(envelope(), identity).projection.entries[0].availability, 'unknown')
  const legacy = envelope(); legacy.mode = 'legacy'; legacy.projection.entries = []
  assert.equal(decodeNavigation(legacy, identity).mode, 'legacy')
  for (const mutate of [
    (v: NavigationEnvelope) => { v.projection.scope.stationId = identity.userId },
    (v: NavigationEnvelope) => { v.mode = 'legacy' },
    (v: NavigationEnvelope) => { v.projection.entries[0].route = '//other.example' },
    (v: NavigationEnvelope) => { v.projection.entries[0].route = '/%2e%2e' },
    (v: NavigationEnvelope) => { v.projection.entries[0].iconRef = 'https://tracker.example' },
    (v: NavigationEnvelope) => { v.projection.entries.push(v.projection.entries[0]) },
  ]) { const value = envelope(); mutate(value); assert.throws(() => decodeNavigation(value, identity)) }
})

test('a late response from an old Station cannot restore its entries', async () => {
  const first = deferred<unknown>(); let count = 0
  const other = { ...identity, stationId: '019a00b0-0000-7000-8000-000000000099' }
  const controller = createNavigationController({ read: () => ++count === 1 ? first.promise : Promise.resolve(envelope(other)), write: async () => { throw new Error('unexpected') } })
  controller.clear(identity); const pending = controller.refresh()
  controller.clear(other); await controller.refresh()
  first.resolve(envelope()); await pending
  assert.equal(controller.state.value?.projection.scope.stationId, other.stationId)
  controller.clear(); assert.equal(controller.state.value, null)
})

test('permission or transport failure clears a prior projection without legacy fallback', async () => {
  let fail = false
  const controller = createNavigationController({ read: async () => { if (fail) throw new Error('denied'); return envelope() }, write: async () => { throw new Error('unexpected') } })
  controller.clear(identity); await controller.refresh(); assert.equal(controller.state.status, 'ready')
  fail = true; await controller.refresh()
  assert.equal(controller.state.status, 'error'); assert.equal(controller.state.value, null)
})

test('unpin uses server preference revision and a conflict refreshes without replaying the write', async () => {
  let writes = 0; const updated = envelope(); updated.preferenceRevision = '13'; updated.projection.entries[0].pinned = false; updated.projection.entries[0].pinSource = 'user'
  const inputs: NavigationPreference[] = []
  const controller = createNavigationController({ read: async () => writes ? updated : envelope(), write: async input => { writes++; inputs.push(input); throw new Error('NAVIGATION_PREFERENCE_CONFLICT') } })
  controller.clear(identity); await controller.refresh()
  await controller.setPreference(controller.state.value!.projection.entries[0], false)
  assert.equal(writes, 1); assert.deepEqual(inputs[0], { appId: 'project', entryId: 'main', pinned: false, order: 10, preferenceRevision: '12' })
  assert.equal(controller.state.value?.projection.entries[0].pinned, false)
  assert.match(controller.state.error, /其他窗口/)
})

test('logout while a preference write is pending invalidates its successful response', async () => {
  const write = deferred<unknown>()
  const controller = createNavigationController({ read: async () => envelope(), write: () => write.promise })
  controller.clear(identity); await controller.refresh()
  const pending = controller.setPreference(controller.state.value!.projection.entries[0], false)
  controller.clear(); write.resolve(envelope()); await pending
  assert.equal(controller.state.status, 'idle'); assert.equal(controller.state.value, null)
})

test('the host transport uses only the fixed local paths and same-origin credentials', async () => {
  const calls: { path: string; options: RequestInit }[] = []
  const fetcher: typeof fetch = async (input, options) => { calls.push({ path: String(input), options: options! }); return new Response(JSON.stringify(envelope()), { headers: { 'Content-Type': 'application/json' } }) }
  const transport = createNavigationTransport(fetcher)
  const signal = new AbortController().signal
  await transport.read(signal)
  await transport.write({ appId: 'project', entryId: 'main', pinned: false, order: 10, preferenceRevision: '12' }, signal)
  assert.deepEqual(calls.map(call => call.path), ['/studio/navigation', '/studio/navigation/preferences'])
  for (const call of calls) { assert.equal(call.options.credentials, 'same-origin'); assert.equal(call.options.redirect, 'error'); assert.equal(call.options.cache, 'no-store') }
})
