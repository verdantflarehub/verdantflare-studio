import assert from 'node:assert/strict'
import test from 'node:test'
import { AppCenterError, createAppCenterController, decodeView, selectCards } from '../src/app-center/controller.ts'
import { createAppCenterTransport } from '../src/app-center/transport.ts'
import { validAppCenterView } from '../src/app-center/schema.generated.ts'
import type { AppCenterCard, AppCenterView } from '../src/app-center/schema.generated.ts'
import { appCenterNavigationEntries } from '../src/app-center/navigation.ts'
import { managementActions, versionLabel } from '../src/app-center/card-actions.ts'
import { publisherLabel } from '../src/app-center/presentation.ts'
import type { NavigationEnvelope } from '../src/navigation/controller.ts'

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

test('my apps retains installed services through stop, failure and removal progress until authoritative removal', () => {
  const data = view(), card = data.items[0]
  for (const state of ['stopped', 'degraded', 'unknown'] as const) {
    card.service.state = state
    card.operation.blocking = true
    card.operation.phase = 'deleting'
    assert.deepEqual(selectCards(data, '', 'all', 'installed').items, [card])
    assert.equal(selectCards(data).counts.installed, 1)
  }
  card.installation!.state = 'removed'
  assert.equal(selectCards(data, '', 'all', 'installed').items.length, 0)
  assert.deepEqual(selectCards(data).items, [card])
  assert.equal(managementActions(card, false).length, 0)
  card.installation!.state = 'failed'
  assert.equal(selectCards(data, '', 'all', 'installed').items.length, 0)
  assert.deepEqual(selectCards(data, '', 'all', 'active').items, [card], 'operation remains recoverable in all apps')
})

test('management menu preserves server action ids, denies stale or absent capabilities and ignores installation source', () => {
  const card = view().items[0]
  card.actions.push({ id: 'service.stop', kind: 'stop', enabled: true, reasonCode: null, entryId: null, confirmation: 'service-stop' },
    { id: 'installation.remove', kind: 'delete', enabled: false, reasonCode: 'PERMISSION_DENIED', entryId: null, confirmation: 'installation-delete' })
  const before = structuredClone(card)
  const actions = managementActions(card, false)
  assert.equal(actions[0].id, 'service.stop'); assert.equal(actions[0].enabled, true)
  assert.equal(actions[1].id, 'installation.remove'); assert.equal(actions[1].enabled, false)
  assert.equal(actions[1].reason, '当前账号没有权限')
  assert.ok(managementActions(card, true).every(item => !item.enabled && item.reason.includes('已过期')))
  card.preinstalled = true
  assert.deepEqual(managementActions(card, false), actions)
  card.preinstalled = before.preinstalled
  assert.deepEqual(card, before, 'presentation does not mutate bindings or permission facts')
  card.actions = card.actions.filter(item => item.kind !== 'stop')
  assert.equal(managementActions(card, false)[0].id, null)
  assert.equal(managementActions(card, false)[0].enabled, false)
  card.installation = null
  assert.deepEqual(managementActions(card, false), [])
})

test('missing metadata stays explicit and candidate updates do not replace installation versions', () => {
  const card = view().items[0]
  assert.equal(publisherLabel(null), '发布方资料待补充')
  card.candidateVersion = '2.0.0'
  assert.equal(versionLabel(card), '1.0.0')
  card.installedVersion = null
  assert.equal(versionLabel(card), '版本待确认')
  card.installation = null
  assert.equal(versionLabel(card), '2.0.0')
  card.candidateVersion = null
  assert.equal(versionLabel(card), '版本待确认')
})

test('declared models, capabilities and publishers share directory filtering without granting actions', () => {
  const v = view(), card = v.items[0]
  card.display.models = ['Example Model']; card.display.capabilities = ['scene.read']; card.display.publisherId = 'publisher'
  card.preinstalled = true
  const before = structuredClone(card.actions), binding = card.bindingRevision
  for (const query of ['example MODEL', 'SCENE.READ', 'publisher']) {
    const found = selectCards(v, query, 'system', 'installed')
    assert.deepEqual(found.items, [card]); assert.deepEqual(found.preinstalled, [card]); assert.equal(found.applications.length, 0)
    assert.equal(selectCards(v, query, 'other').items.length, 0)
    assert.equal(selectCards(v, query, 'system', 'active').items.length, 0)
  }
  card.display.models = []; card.display.capabilities = []
  assert.equal(selectCards(v, 'Example Model').items.length, 0)
  assert.equal(selectCards(v, 'scene.read').items.length, 0)
  assert.deepEqual(card.actions, before); assert.equal(card.bindingRevision, binding)
})

test('display arrays preserve missing versus declared empty and reject malformed or duplicate terms', () => {
  const v = view()
  assert.equal(validAppCenterView(v), true)
  for (const name of ['models', 'capabilities'] as const) {
    v.items[0].display[name] = []
    assert.equal(validAppCenterView(v), true)
    v.items[0].display[name] = ['模型 Alpha', 'sample.read']
    assert.equal(validAppCenterView(v), true)
    for (const invalid of [['same', 'same'], [''], ['  '], [' padded'], ['bad\nvalue'], ['x'.repeat(129)], Array.from({length:65}, (_, i) => String(i)), null, 'text']) {
      const bad = structuredClone(v); Object.assign(bad.items[0].display, { [name]: invalid })
      assert.equal(validAppCenterView(bad), false, `${name}: ${JSON.stringify(invalid)}`)
    }
    delete v.items[0].display[name]
    assert.equal(validAppCenterView(v), true)
  }
})

test('menus use the shared snapshot while preserving title, pin and order', () => {
  const v = view()
  const navigation: NavigationEnvelope = { mode: 'preinstalled', preferenceRevision: '7', projection: {
    scope: v.scope, revision: digest, entries: [{ installationId: id(4), appId: 'sample', appVersion: '1.0.0', entryId: 'main',
      title: 'My title', iconRef: 'app', route: '/', group: 'applications', order: 17, pinned: false, pinSource: 'user', availability: 'unknown' }],
  } }
  const state = { status: 'ready' as const, value: v, stale: false, error: '', refreshing: false }
  const entry = appCenterNavigationEntries(navigation, state, now)[0]
  assert.equal(entry.availability, 'ready')
  assert.equal(entry.title, 'My title')
  assert.equal(entry.order, 17)
  assert.equal(entry.pinned, false)
  assert.equal(entry.bindingRevision, v.items[0].bindingRevision)
  assert.equal(entry.entryId, 'main', 'navigation entry is not the Dashboard entry')
  for (const scenario of ['expired', 'failed', 'stale', 'scope', 'version', 'reinstalled', 'blocked', 'wrong-entry', 'wrong-transport', 'removed']) {
    const next = structuredClone(state), nav = structuredClone(navigation)
    let at = now
    switch (scenario) {
      case 'expired': at += 15001; break
      case 'failed': next.stale = true; next.error = 'SOURCE_UNAVAILABLE'; break
      case 'stale': next.stale = true; break
      case 'scope': next.value.scope.authorizationEpoch = 'changed'; break
      case 'version': next.value.items[0].installedVersion = '1.1.0'; break
      case 'reinstalled': next.value.items[0].installation!.installationId = id(9); break
      case 'blocked': next.value.items[0].actions[1].enabled = false; next.value.items[0].actions[1].reasonCode = 'OPERATION_IN_PROGRESS'; break
      case 'wrong-entry': next.value.items[0].actions[1].entryId = 'other'; break
      case 'wrong-transport': next.value.items[0].entry!.transport = 'legacy'; break
      case 'removed': next.value.items = []; break
    }
    const closed = appCenterNavigationEntries(nav, next, at)[0]
    assert.notEqual(closed.availability, 'ready', scenario)
    assert.equal(closed.bindingRevision, undefined, scenario)
    assert.equal(closed.title, 'My title')
    assert.equal(closed.pinned, false)
  }
  state.value.items[0].service.state = 'stopped'
  state.value.items[0].actions[1].enabled = false
  state.value.items[0].actions[1].reasonCode = 'SERVICE_NOT_READY'
  assert.equal(appCenterNavigationEntries(navigation, state, now)[0].availability, 'offline')
  assert.equal(appCenterNavigationEntries(navigation, state, now)[0].openReason, 'SERVICE_NOT_READY')
  assert.equal(navigation.projection.entries[0].availability, 'unknown', 'source preferences not mutated')
})

test('preinstalled and normal sections partition the same search and count result', () => {
  const v = view(), second = structuredClone(v.items[0])
  v.items[0].preinstalled = true
  second.cardKey = `installation:${id(9)}`; second.installation!.installationId = id(9)
  second.appId = 'another'; second.display.name = 'Another'; second.preinstalled = false
  v.items.push(second)
  const all = selectCards(v)
  assert.equal(all.counts.all, 2)
  assert.equal(all.counts.installed, 2)
  assert.deepEqual(all.preinstalled.map(card => card.cardKey), [v.items[0].cardKey])
  assert.deepEqual(all.applications.map(card => card.cardKey), [second.cardKey])
  assert.equal(new Set([...all.preinstalled, ...all.applications].map(card => card.cardKey)).size, all.items.length)
  const search = selectCards(v, 'Another')
  assert.equal(search.preinstalled.length, 0)
  assert.equal(search.applications.length, 1)
  assert.equal(search.counts.all, 2)
  assert.equal(selectCards(v, '', 'system', 'installed').preinstalled.length, 1)
})

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
  const fetcher = (async (url, init) => { seen.push([url, init?.method, init?.credentials, init?.redirect]); return snapshotResponse() }) as typeof fetch
  assert.deepEqual(await createAppCenterTransport(fetcher).read(new AbortController().signal), view())
  assert.deepEqual(seen, [['/studio/appcenter/v1/view', 'GET', 'same-origin', 'error']])
  const denied = createAppCenterTransport((async () => new Response('forbidden', { status: 403 })) as typeof fetch)
  await assert.rejects(denied.read(new AbortController().signal), (e: unknown) => e instanceof AppCenterError && e.status === 403)
})

function snapshotResponse(value = view(), date: string | null = new Date(now).toUTCString()) {
  const headers = new Headers({ 'Content-Type': 'application/json', 'X-Request-ID': value.requestId })
  if (date !== null) headers.set('Date', date)
  return new Response(JSON.stringify(value), { headers })
}

test('server snapshot clock tolerates skew and wall-clock changes without extending expiry', async () => {
  for (const skew of [-3600000, -10000, 0, 3600000]) {
    let elapsed = 0, wall = now + skew
    const transport = createAppCenterTransport((async () => { elapsed += 250; return snapshotResponse() }) as typeof fetch, () => elapsed)
    const controller = createAppCenterController(transport, () => wall)
    controller.clear(identity); await controller.refresh()
    assert.equal(controller.state.status, 'ready')
    assert.equal(controller.currentTime(), now + 1250)
    const card = view().items[0]
    wall -= 86400000; elapsed = 13999
    assert.equal(controller.canExecute(card.cardKey, 'open', digest), true)
    wall += 172800000; elapsed = 14000
    assert.equal(controller.canExecute(card.cardKey, 'open', digest), false)
    assert.equal(controller.state.stale, true)
    controller.dispose()
  }
})

test('missing, malformed or stale response time fails closed, including body transit', async () => {
  for (const date of [null, 'garbage', '2026-10-12', new Date(now + 20000).toUTCString(), new Date(now - 10000).toUTCString()]) {
    const transport = createAppCenterTransport((async () => snapshotResponse(view(), date)) as typeof fetch, () => 0)
    const controller = createAppCenterController(transport, () => now)
    controller.clear(identity); await controller.refresh()
    assert.equal(controller.state.status, 'error', String(date)); assert.equal(controller.state.value, null)
    controller.dispose()
  }
  let elapsed = 0
  const transport = createAppCenterTransport((async () => {
    const response = snapshotResponse()
    return new Response(new ReadableStream({ pull(stream) {
      elapsed = 14500
      stream.enqueue(new TextEncoder().encode(JSON.stringify(view()))); stream.close()
    } }), { headers: response.headers })
  }) as typeof fetch, () => elapsed)
  const controller = createAppCenterController(transport, () => now)
  controller.clear(identity); await controller.refresh()
  assert.equal(controller.state.status, 'error'); controller.dispose()
})

test('early source expiry, hidden page and identity reset block actions', async () => {
  let elapsed = 0
  const early = view(); early.validUntil = new Date(now + 2500).toISOString()
  const transport = createAppCenterTransport((async () => snapshotResponse(early)) as typeof fetch, () => elapsed)
  const controller = createAppCenterController(transport, () => now - 3600000), card = early.items[0]
  controller.clear(identity); await controller.refresh()
  elapsed = 1500
  assert.equal(controller.canExecute(card.cardKey, 'open', digest), false)
  elapsed = 0; await controller.refresh(); controller.suspend()
  assert.equal(controller.canExecute(card.cardKey, 'open', digest), false)
  controller.clear({ ...identity, userId: id(8) }); await controller.refresh()
  assert.equal(controller.state.value, null)
  assert.equal(controller.currentTime(), now - 3600000, 'invalid scope cannot install a clock')
  controller.dispose()
})

test('late and rejected response clocks cannot recalibrate the active snapshot', async () => {
  let elapsed = 0, calls = 0
  const first = deferred<Response>()
  const transport = createAppCenterTransport((async () => ++calls === 1 ? first.promise : snapshotResponse()) as typeof fetch, () => elapsed)
  const controller = createAppCenterController(transport, () => now - 3600000)
  controller.clear(identity); const pending = controller.refresh()
  elapsed = 500; await controller.refresh()
  assert.equal(controller.currentTime(), now + 1000)
  first.resolve(snapshotResponse(view(), new Date(now + 3600000).toUTCString())); await pending
  assert.equal(controller.currentTime(), now + 1000)
  elapsed = 14500
  assert.equal(controller.canExecute(view().items[0].cardKey, 'open', digest), false)
  controller.dispose()
})
