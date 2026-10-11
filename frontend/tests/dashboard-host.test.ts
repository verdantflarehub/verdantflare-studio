import test from 'node:test'
import assert from 'node:assert/strict'
import { DashboardCache } from '../src/app-dashboard/cache.ts'
import { validateLaunch, dashboardCacheKey } from '../src/app-dashboard/launch.ts'
import type { LaunchPolicy } from '../src/app-dashboard/launch.ts'
import type { CachedDashboard } from '../src/app-dashboard/cache.ts'

const context = { userId: 'user-1', organizationId: 'org-1', stationId: 'station-1', projectId: null,
  appId: 'sample', installationId: 'install-1', contextRevision: 'r1' }
const launch = { dashboardSessionId: 'session-1', installationId: 'install-1', appId: 'sample', appVersion: '1.0.0',
  artifactDigest: `sha256:${'a'.repeat(64)}`, adapter: 'wujie', entryUrl: '/apps/sample/dashboard/',
  sdkVersion: '1.0.0', grantedCapabilities: ['app.request'], contextRevision: 'r1', expiresAt: '2099-01-01T00:00:00Z' }
const policy: LaunchPolicy = { assetBaseUrl: 'https://studio.example/apps/sample/', context,
  installationId: 'install-1', appId: 'sample', appVersion: '1.0.0', artifactDigest: launch.artifactDigest,
  requiredCapabilities: ['app.request'], allowedCapabilities: ['app.request'], trusted: true }

test('launch rejects unknown fields, untrusted pages, mismatched scope and grants before execution', () => {
  assert.equal(validateLaunch(launch, policy).appId, 'sample')
  for (const changed of [{ ...launch, instanceId: 'wrong-identity' }, { ...launch, installationId: 'other' },
    { ...launch, sdkVersion: '2.0.0' }, { ...launch, adapter: 'isolated-iframe' },
    { ...launch, artifactDigest: `sha256:${'b'.repeat(64)}` }, { ...launch, grantedCapabilities: [] },
    { ...launch, grantedCapabilities: ['app.request', 'admin'] }, { ...launch, expiresAt: '2000-01-01T00:00:00Z' }]) {
    assert.throws(() => validateLaunch(changed, policy))
  }
  assert.throws(() => validateLaunch(launch, { ...policy, trusted: false }), { code: 'DASHBOARD_UNTRUSTED' })
})

test('launch confines assets to the platform-resolved root', () => {
  for (const entryUrl of ['https://evil.example/apps/sample/', '//evil.example/path', '/apps/sample-other/',
    '/apps/sample/../other/', '/apps/sample/%2e%2e/other/', '/apps/sample/%252fother',
    '/apps/sample/dashboard/?token=secret', '/apps/sample/dashboard/#route', 'javascript:alert(1)']) {
    assert.throws(() => validateLaunch({ ...launch, entryUrl }, policy), { code: 'INVALID_ENTRY' })
  }
})

test('cache identity separates subject, install, artifact and authorization; project changes preserve forms', () => {
  const checked = validateLaunch(launch, policy)
  const key = dashboardCacheKey(checked, context, 'auth-1')
  assert.equal(key, dashboardCacheKey(checked, { ...context, projectId: 'p2' }, 'auth-1'))
  assert.notEqual(key, dashboardCacheKey(checked, { ...context, userId: 'other' }, 'auth-1'))
  assert.notEqual(key, dashboardCacheKey(checked, context, 'auth-2'))
  assert.notEqual(key, dashboardCacheKey({ ...checked, artifactDigest: 'other' }, context, 'auth-1'))
})

function entry(events: string[], name: string, allowClose = true): CachedDashboard {
  return { hide: () => { events.push(`${name}:hidden`) }, show: () => { events.push(`${name}:active`) },
    prepareClose: async () => allowClose, dispose: async () => { events.push(`${name}:dispose`) } }
}

test('bounded LRU preserves cached entry and releases evicted resources', async () => {
  const events: string[] = [], cache = new DashboardCache(2)
  const a = await cache.activate('a', async () => entry(events, 'a'))
  await cache.activate('b', async () => entry(events, 'b'))
  assert.equal(await cache.activate('a', async () => { throw new Error('must reuse') }), a)
  await cache.activate('c', async () => entry(events, 'c'))
  assert.equal(cache.size, 2)
  assert.ok(events.includes('a:hidden'))
  assert.ok(events.includes('b:dispose'))
  assert.ok(!events.includes('a:dispose'))
  await cache.revoke()
  assert.equal(cache.size, 0)
})

test('dirty veto prevents eviction and creation; revoke bypasses veto', async () => {
  const events: string[] = [], cache = new DashboardCache(1)
  await cache.activate('a', async () => entry(events, 'a', false))
  await assert.rejects(cache.activate('b', async () => { throw new Error('must not create') }), /CACHE_CLOSE_CANCELLED/)
  assert.equal(await cache.close('a'), false)
  assert.equal(cache.size, 1)
  await cache.revoke()
  assert.deepEqual(events, ['a:active', 'a:dispose'])
})

test('revoke during async creation disposes late entry without activation', async () => {
  const events: string[] = [], cache = new DashboardCache(2)
  let resolve!: (value: CachedDashboard) => void
  const opened = cache.activate('a', () => new Promise(done => { resolve = done }))
  await Promise.resolve()
  await cache.revoke()
  resolve(entry(events, 'a'))
  await assert.rejects(opened, /CACHE_REVOKED/)
  assert.deepEqual(events, ['a:dispose'])
  assert.equal(cache.size, 0)
})

test('revocation cancels an in-progress mount and prevents queued mounts', async () => {
  const cache = new DashboardCache(2)
  let signal!: AbortSignal
  const first = cache.activate('a', input => {
    signal = input
    return new Promise((_, reject) => input.addEventListener('abort', () => reject(new Error('cancelled')), { once: true }))
  })
  const queued = cache.activate('b', async () => { throw new Error('must not start') })
  await Promise.resolve()
  await cache.revoke()
  assert.equal(signal.aborted, true)
  await assert.rejects(first, /cancelled/)
  await assert.rejects(queued, /CACHE_REVOKED/)
})

test('revocation during context coordination disposes a newly created entry', async () => {
  const events: string[] = [], cache = new DashboardCache(2)
  let resume!: () => void
  let started!: () => void
  const preparing = new Promise<void>(resolve => { started = resolve })
  const opened = cache.activate('a', async () => entry(events, 'a'), async () => {
    started()
    await new Promise<void>(resolve => { resume = resolve })
  })
  await preparing
  await cache.revoke()
  resume()
  await assert.rejects(opened, /CACHE_REVOKED/)
  assert.deepEqual(events, ['a:dispose'])
})

test('failed mount preserves the current page and a later retry can succeed', async () => {
  const events: string[] = [], cache = new DashboardCache(2)
  await cache.activate('a', async () => entry(events, 'a'))
  await assert.rejects(cache.activate('b', async () => { throw new Error('load failed') }), /load failed/)
  assert.deepEqual(events, ['a:active'])
  assert.equal(cache.size, 1)
  await cache.activate('b', async () => entry(events, 'b'))
  assert.deepEqual(events, ['a:active', 'a:hidden', 'b:active'])
})

test('context veto preserves cached forms and disposes a failed new page', async () => {
  const events: string[] = [], cache = new DashboardCache(3)
  const a = await cache.activate('a', async () => entry(events, 'a'))
  await cache.activate('b', async () => entry(events, 'b'))
  const veto = async () => { throw new Error('CONTEXT_CHANGE_CANCELLED') }
  await assert.rejects(cache.activate('a', async () => { throw new Error('must reuse') }, veto), /CONTEXT_CHANGE_CANCELLED/)
  assert.equal(cache.size, 2)
  assert.ok(!events.includes('a:dispose'))
  await assert.rejects(cache.activate('c', async () => entry(events, 'c'), veto), /CONTEXT_CHANGE_CANCELLED/)
  assert.equal(cache.size, 2)
  assert.equal(events.at(-1), 'c:dispose')
  assert.equal(await cache.activate('a', async () => { throw new Error('must reuse') }), a)
})

test('failed activation releases the broken page and restores the previous page', async () => {
  const events: string[] = [], cache = new DashboardCache(2)
  await cache.activate('a', async () => entry(events, 'a'))
  const broken = { ...entry(events, 'b'), show() { throw new Error('DASHBOARD_DISPOSED') } }
  await assert.rejects(cache.activate('b', async () => broken), /DASHBOARD_DISPOSED/)
  assert.equal(cache.size, 1)
  assert.deepEqual(events, ['a:active', 'a:hidden', 'b:dispose', 'a:active'])
})

test('returning to the directory hides a dashboard while retaining its state', async () => {
  const events: string[] = [], cache = new DashboardCache(2)
  const original = await cache.activate('a', async () => entry(events, 'a'))
  await cache.hideActive()
  assert.equal(cache.size, 1)
  assert.equal(await cache.activate('a', async () => { throw new Error('must reuse') }), original)
  assert.deepEqual(events, ['a:active', 'a:hidden', 'a:active'])
})

test('leaving AppCenter checks hidden dirty pages without destroying retained state', async () => {
  const events: string[] = [], cache = new DashboardCache(2)
  await cache.activate('a', async () => entry(events, 'a', false))
  await cache.activate('b', async () => entry(events, 'b'))
  assert.equal(await cache.prepareLeave(), false)
  assert.equal(cache.size, 2)
  assert.deepEqual(events, ['a:active', 'a:hidden', 'b:active'])
  await cache.revoke()
  assert.equal(await cache.prepareLeave(), true)
})

test('invalidation during initial preparation cannot activate or dispose the page twice', async () => {
  const events: string[] = [], cache = new DashboardCache(2)
  let resume!: () => void, started!: () => void
  const preparing = new Promise<void>(resolve => { started = resolve })
  const opening = cache.activate('a', async () => entry(events, 'a'), async () => {
    started()
    await new Promise<void>(resolve => { resume = resolve })
  })
  await preparing
  await cache.invalidate('a')
  resume()
  await assert.rejects(opening, /CACHE_REVOKED/)
  assert.equal(cache.size, 0)
  assert.deepEqual(events, ['a:dispose'])
})
