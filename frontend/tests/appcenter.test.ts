import assert from 'node:assert/strict'
import test from 'node:test'
import { AppCenterError, createAppCenterController, decodeView, selectCards } from '../src/app-center/controller.ts'
import { createAppCenterTransport } from '../src/app-center/transport.ts'
import { validAppCenterView } from '../src/app-center/schema.generated.ts'
import type { AppCenterCard, AppCenterView } from '../src/app-center/schema.generated.ts'

const now = Date.parse('2026-10-12T00:00:00Z'), digest = `sha256:${'a'.repeat(64)}`
const id = (n: number) => `00000000-0000-7000-8000-${String(n).padStart(12, '0')}`
const identity = { userId: id(1), organizationId: id(2), stationId: id(3) }
function view(): AppCenterView {
  const card: AppCenterCard = {
    cardKey: `installation:${id(4)}`, appId: 'sample', origin: 'installation',
    display: { name: 'Sample', description: '', groupId: 'system', publisherId: null, iconRef: null, resourceTags: [], quality: 'known' },
    installedVersion: '1.0.0', candidateVersion: null,
    installation: { installationId: id(4), revision: 1, appVersion: '1.0.0', state: 'installed', lastOperationId: null }, installationQuality: 'known',
    service: { quality: 'known', state: 'ready', observedAt: new Date(now).toISOString(), reasonCode: null, observationKey: `obs:${'b'.repeat(64)}` },
    operation: { quality: 'known', blocking: false, operationId: null, protocol: null, status: null, phase: null, canRead: false, updatedAt: null },
    entry: { entryId: 'dashboard', transport: 'dashboard', quality: 'known', availability: 'available', reasonCode: null, requiresServiceReady: true, minimumHostVersion: null },
    bindingRevision: digest, statusCode: 'READY', primaryActionId: 'open', preinstalled: false,
    actions: [{ id: 'detail', kind: 'detail', enabled: true, reasonCode: null, entryId: null, confirmation: 'none' },
      { id: 'open', kind: 'open', enabled: true, reasonCode: null, entryId: 'dashboard', confirmation: 'none' }],
  }
  return { schemaVersion: 1, requestId: 'test-request', scope: { ...identity, authorizationEpoch: 'c'.repeat(64) }, revision: digest,
    generatedAt: new Date(now).toISOString(), validUntil: new Date(now + 15000).toISOString(), partial: false,
    sources: ['catalog', 'installations', 'observations', 'operations', 'presentation', 'entries'].map(source => ({ source, quality: 'known', revision: digest, observedAt: new Date(now).toISOString(), reasonCode: null })) as AppCenterView['sources'],
    groups: [{ id: 'system', label: 'System', order: 1 }], items: [card] }
}
function deferred<T>() { let resolve!: (value: T) => void; const promise = new Promise<T>(yes => { resolve = yes }); return { promise, resolve } }

test('generated schema rejects malformed values and executable extensions', () => {
  assert.equal(validAppCenterView(view()), true)
  for (const mutate of [
    (v: any) => { v.schemaVersion = 2 },
    (v: any) => { v.items[0].token = 'unexpected' },
    (v: any) => { v.items[0].service.state = 'invented' },
    (v: any) => { v.items[0].actions[1].enabled = false },
    (v: any) => { v.items[0].operation.operationId = id(9) },
    (v: any) => { v.items[0].installation.revision = 0 },
    (v: any) => { v.items[0].display.resourceTags = null },
    (v: any) => { v.generatedAt = 'not-a-date' },
    (v: any) => { v.generatedAt = '2026-02-30T00:00:00Z' },
    (v: any) => { delete v.items[0].entry },
  ]) { const data = view(); mutate(data); assert.equal(validAppCenterView(data), false) }
})

test('decoder binds identity, card keys, unique actions, source completeness and expiry', () => {
  assert.equal(decodeView(view(), identity, now).items.length, 1)
  for (const mutate of [
    (v: AppCenterView) => { v.scope.userId = id(8) },
    (v: AppCenterView) => { v.items.push(v.items[0]) },
    (v: AppCenterView) => { v.items[0].cardKey = `installation:${id(8)}` },
    (v: AppCenterView) => { v.items[0].primaryActionId = 'absent' },
    (v: AppCenterView) => { v.items[0].actions.push(v.items[0].actions[0]) },
    (v: AppCenterView) => { v.sources.pop() },
    (v: AppCenterView) => { v.validUntil = new Date(now).toISOString() },
    (v: AppCenterView) => { v.validUntil = new Date(now + 16000).toISOString() },
  ]) { const data = view(); mutate(data); assert.throws(() => decodeView(data, identity, now)) }
})

test('all filters and counts use installation facts and operation facts', () => {
  const data = view(), stopped = structuredClone(data.items[0]), pending = structuredClone(data.items[0]), observed = structuredClone(data.items[0])
  stopped.cardKey = `installation:${id(5)}`; stopped.installation!.installationId = id(5); stopped.service.state = 'stopped'
  pending.cardKey = `installation:${id(6)}`; pending.installation!.installationId = id(6); pending.installation!.state = 'pending'; pending.operation.blocking = true
  observed.cardKey = `obs:${'d'.repeat(64)}`; observed.origin = 'observation'; observed.installation = null; observed.installedVersion = null
  data.items.push(stopped, pending, observed)
  assert.deepEqual(selectCards(data).counts, { all: 4, installed: 2, active: 1 })
  assert.equal(selectCards(data, '', 'system', 'installed').items.length, 2)
  assert.equal(selectCards(data, 'sample', 'all', 'active').items[0].cardKey, pending.cardKey)
  assert.equal(selectCards(data, 'absent').items.length, 0)
  assert.deepEqual(selectCards(data, '', 'all', 'all', false).counts, { all: null, installed: null, active: null })
})

test('late requests cannot restore a cleared identity or overwrite newer data', async () => {
  const first = deferred<unknown>(), second = deferred<unknown>(); let calls = 0
  const controller = createAppCenterController({ read: () => ++calls === 1 ? first.promise : second.promise }, () => now)
  controller.clear(identity)
  const a = controller.refresh(), b = controller.refresh()
  const newer = view(); newer.items = []; second.resolve(newer); await b
  assert.equal(controller.state.status, 'empty')
  first.resolve(view()); await a
  assert.equal(controller.state.status, 'empty')
  const late = deferred<unknown>(), other = createAppCenterController({ read: () => late.promise }, () => now)
  other.clear(identity); const pending = other.refresh(); other.clear(); late.resolve(view()); await pending
  assert.equal(other.state.value, null)
  controller.dispose(); other.dispose()
})

test('failed refresh retains read-only same-scope data; authentication loss clears it', async () => {
  let failure: Error | null = null
  const controller = createAppCenterController({ read: async () => { if (failure) throw failure; return view() } }, () => now)
  controller.clear(identity); await controller.refresh()
  assert.equal(controller.canExecute(view().items[0].cardKey, 'open', digest), true)
  failure = new AppCenterError(503, 'APPCENTER_DEPENDENCY_UNAVAILABLE'); await controller.refresh()
  assert.equal(controller.state.status, 'error'); assert.equal(controller.state.value?.items.length, 1)
  assert.equal(controller.canExecute(view().items[0].cardKey, 'open', digest), false)
  failure = new AppCenterError(401, 'UNAUTHENTICATED'); await controller.refresh()
  assert.equal(controller.state.value, null); controller.dispose()
})

test('expiration and old bindings cannot execute even before a polling tick', async () => {
  let time = now
  const controller = createAppCenterController({ read: async () => view() }, () => time)
  controller.clear(identity); await controller.refresh()
  const card = view().items[0]
  assert.equal(controller.canExecute(card.cardKey, 'open', `sha256:${'d'.repeat(64)}`), false)
  time += 15000
  assert.equal(controller.canExecute(card.cardKey, 'open', digest), false)
  assert.equal(controller.state.stale, true); controller.dispose()
})

test('Web and desktop use one fixed endpoint and clear unauthorized non-JSON responses', async () => {
  const seen: unknown[] = []
  const fetcher = (async (url, init) => { seen.push([url, init?.method, init?.credentials, init?.redirect]); return new Response(JSON.stringify(view()), { headers: { 'Content-Type': 'application/json', 'X-Request-ID': 'test-request' } }) }) as typeof fetch
  assert.deepEqual(await createAppCenterTransport(fetcher).read(new AbortController().signal), view())
  assert.deepEqual(seen, [['/studio/appcenter/v1/view', 'GET', 'same-origin', 'error']])
  const denied = createAppCenterTransport((async () => new Response('forbidden', { status: 403 })) as typeof fetch)
  await assert.rejects(denied.read(new AbortController().signal), (e: unknown) => e instanceof AppCenterError && e.status === 403)
})
