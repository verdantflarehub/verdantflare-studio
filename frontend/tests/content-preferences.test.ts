import test from 'node:test'
import assert from 'node:assert/strict'
import {
  NSFW_MAX_TTL_MS,
  NSFW_PREF_STORAGE_KEY,
  createContentPreferencesController,
  formatRemainingTime,
  submitContentRatingAction,
} from '../src/platform/contentPreferences.ts'
import type { StorageLike } from '../src/platform/contentPreferences.ts'
import { createAppSDK } from '../src/app-dashboard/sdk.ts'
import { createDashboardWebClient } from '../src/app-dashboard/web-client.ts'
import type { AppContext, DashboardLaunch, SDKAdapters } from '../src/app-dashboard/types.ts'

function memoryStorage(blocked = false): StorageLike {
  const map = new Map<string, string>()
  return {
    getItem(key: string) {
      if (blocked) throw new Error('storage blocked')
      return map.get(key) ?? null
    },
    setItem(key: string, value: string) {
      if (blocked) throw new Error('storage blocked')
      map.set(key, value)
    },
    removeItem(key: string) {
      if (blocked) throw new Error('storage blocked')
      map.delete(key)
    },
  }
}

test('contentPreferences defaults to off, binds to user/org, expires at 24h, and clears on logout or account switch', () => {
  let clock = Date.parse('2026-10-24T00:00:00Z')
  const storage = memoryStorage()
  const synced: Array<{ includeNsfw: boolean; expiresAt: string | null }> = []
  const ctrl = createContentPreferencesController({
    storage,
    now: () => clock,
    onPostMessageSync: payload => synced.push(payload),
  })

  // Default off
  assert.equal(ctrl.get().includeNsfw, false)
  assert.equal(ctrl.get().storageAvailable, true)

  // Cannot enable without authenticated session
  assert.throws(() => ctrl.enable(), /UNAUTHENTICATED/)

  ctrl.setSessionIdentity({ userId: 'usr-1', organizationId: 'org-1' })
  const enabled = ctrl.enable()
  assert.equal(enabled.includeNsfw, true)
  assert.equal(enabled.remainingMs, NSFW_MAX_TTL_MS)
  assert.equal(enabled.expiringSoon, false)
  assert.equal(synced.at(-1)?.includeNsfw, true)

  // Advance to 12 minutes remaining -> expiringSoon becomes true
  clock += NSFW_MAX_TTL_MS - 12 * 60_000
  const soon = ctrl.get()
  assert.equal(soon.includeNsfw, true)
  assert.equal(soon.expiringSoon, true)
  assert.equal(formatRemainingTime(soon.remainingMs), '剩余 12 分钟')

  // Advance past 24h -> automatically invalid and removed from storage
  clock += 13 * 60_000
  const expired = ctrl.checkAndSync('expired')
  assert.equal(expired.includeNsfw, false)
  assert.equal(storage.getItem(NSFW_PREF_STORAGE_KEY), null)

  // Re-enable and verify account switch clears immediately
  ctrl.enable()
  assert.equal(ctrl.get().includeNsfw, true)
  const switched = ctrl.setSessionIdentity({ userId: 'usr-2', organizationId: 'org-1' })
  assert.equal(switched.includeNsfw, false)
  assert.equal(storage.getItem(NSFW_PREF_STORAGE_KEY), null)

  // Re-enable and verify logout clears immediately
  ctrl.enable()
  assert.equal(ctrl.get().includeNsfw, true)
  const loggedOut = ctrl.setSessionIdentity(null)
  assert.equal(loggedOut.includeNsfw, false)
  assert.equal(storage.getItem(NSFW_PREF_STORAGE_KEY), null)

  ctrl.dispose()
})

test('contentPreferences forbids enabling when localStorage is unavailable (no session-only fallback)', () => {
  const ctrl = createContentPreferencesController({
    storage: memoryStorage(true),
    now: () => Date.now(),
  })
  ctrl.setSessionIdentity({ userId: 'usr-1', organizationId: 'org-1' })
  assert.equal(ctrl.get().storageAvailable, false)
  assert.throws(() => ctrl.enable(), /STORAGE_UNAVAILABLE/)
  assert.equal(ctrl.get().includeNsfw, false)
  ctrl.dispose()
})

test('AppSDK contentPreferences notifies subscribers and closes active media preview when disabled', () => {
  let mediaClosed = 0
  const context: AppContext = {
    userId: 'user-1',
    organizationId: 'org-1',
    stationId: 'station-1',
    projectId: null,
    appId: 'image',
    installationId: 'install-1',
    contextRevision: 'r1',
  }
  const launch: DashboardLaunch = {
    dashboardSessionId: 'session-1',
    installationId: 'install-1',
    appId: 'image',
    appVersion: '1.0.0',
    artifactDigest: `sha256:${'a'.repeat(64)}`,
    adapter: 'wujie',
    entryUrl: '/dashboard/image/',
    sdkVersion: '1.2.0',
    grantedCapabilities: ['app.request'],
    contextRevision: 'r1',
    expiresAt: new Date(Date.now() + 60_000).toISOString(),
  }
  const adapters: SDKAdapters = {
    navigate: async () => {},
    selectFiles: async () => [],
    callCapability: async () => null,
    requestApp: async () => ({ status: 200, data: {}, requestId: 'req-1' }),
    openInstance: async () => {},
    closeMedia: () => { mediaClosed++ },
    hasUserActivation: () => true,
    reportListenerError: () => {},
  }
  const session = createAppSDK(launch, context, adapters)
  const states: boolean[] = []
  session.sdk.contentPreferences.subscribe(s => states.push(s.includeNsfw))

  assert.equal(session.sdk.contentPreferences.get().includeNsfw, false)
  session.setContentPreferences({ includeNsfw: true, expiresAt: '2026-10-25T00:00:00Z' })
  assert.equal(session.sdk.contentPreferences.get().includeNsfw, true)
  session.setContentPreferences({ includeNsfw: false, expiresAt: null })
  assert.equal(session.sdk.contentPreferences.get().includeNsfw, false)
  assert.equal(mediaClosed, 1)
  assert.deepEqual(states, [true, false])
  session.dispose()
})

test('DashboardWebClient injects X-VF-Include-NSFW: 1 on app requests only when local switch is active', async () => {
  const sessionId = '01980000-0000-4000-8000-000000000002'
  let active = false
  const seenHeaders: Array<Record<string, string>> = []
  const client = createDashboardWebClient(
    'https://studio.invalid',
    async (_url, options) => {
      seenHeaders.push((options?.headers ?? {}) as Record<string, string>)
      return new Response(JSON.stringify({ ok: true }), {
        status: 200,
        headers: { 'Content-Type': 'application/json', 'X-Request-ID': 'req-1' },
      })
    },
    () => 1000,
    () => active,
  )

  await client.request(sessionId, { method: 'GET', path: '/tasks' })
  assert.equal(seenHeaders[0]['X-VF-Include-NSFW'], undefined)

  active = true
  await client.request(sessionId, { method: 'GET', path: '/tasks' })
  assert.equal(seenHeaders[1]['X-VF-Include-NSFW'], '1')
})

test('submitContentRatingAction sends POST to /api/v1/content-ratings/:domain/:version_id/:action', async () => {
  const calls: Array<{ url: string; body: string }> = []
  const res = await submitContentRatingAction('image', 'ver_01960000-0000-7000-8000-000000000001', 'clear', {
    reason: 'false positive',
    projectId: 'prj_01',
    fetcher: (async (url: string, init?: RequestInit) => {
      calls.push({ url, body: String(init?.body ?? '') })
      return new Response(JSON.stringify({ rating: 'general', human_cleared: true }), { status: 200 })
    }) as typeof fetch,
  })
  assert.equal(res.status, 200)
  assert.equal(calls.length, 1)
  assert.equal(calls[0].url, '/api/v1/content-ratings/image/ver_01960000-0000-7000-8000-000000000001/clear')
  assert.deepEqual(JSON.parse(calls[0].body), { reason: 'false positive', project_id: 'prj_01' })
})
