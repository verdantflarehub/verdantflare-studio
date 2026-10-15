import test from 'node:test'
import assert from 'node:assert/strict'
import { DashboardSessions } from '../src/app-dashboard/sessions.ts'
import { browserDashboardPath, canonicalAppPath, dashboardLocation, dashboardRoutePath, dashboardSdkRoute } from '../src/app-dashboard/routes.ts'
import type { OpenedDashboard } from '../src/app-dashboard/web-client.ts'

const installation = '01980000-0000-7000-8000-000000000001'
function opened(serial = 1): OpenedDashboard {
  const context = { userId: 'u', organizationId: 'o', stationId: 's', projectId: null, appId: 'sample', installationId: installation, contextRevision: 'r' }
  const launch = { dashboardSessionId: `01980000-0000-4000-8000-${String(serial).padStart(12, '0')}`,
    installationId: installation, appId: 'sample', appVersion: '1.0.0', artifactDigest: 'sha256:' + 'a'.repeat(64), adapter: 'wujie' as const,
    entryUrl: '/entry', sdkVersion: '1.1.0', grantedCapabilities: [], contextRevision: 'r', expiresAt: '2099-01-01T00:00:00Z' }
  return { launch, context, policy: { assetBaseUrl: 'https://studio.invalid/', context, installationId: installation,
    appId: 'sample', appVersion: '1.0.0', artifactDigest: launch.artifactDigest, requiredCapabilities: [], allowedCapabilities: [], trusted: true } }
}

test('returning to a cached dashboard inspects the existing session rather than opening another', async () => {
  const events: string[] = [], original = opened()
  const sessions = new DashboardSessions({
    open: async id => { events.push(`open:${id}`); return original },
    inspect: async (sessionId, id) => { events.push(`inspect:${sessionId}:${id}`); return original },
    close: async id => { events.push(`close:${id}`) },
  })
  const signal = new AbortController().signal
  await sessions.select(installation, signal)
  await sessions.select(installation, signal)
  assert.equal(events.filter(event => event.startsWith('open:')).length, 1)
  assert.equal(events.filter(event => event.startsWith('inspect:')).length, 1)
  await sessions.release(installation, original.launch.dashboardSessionId)
  assert.equal(events.at(-1), `close:${original.launch.dashboardSessionId}`)
})

test('identity loss closes a late successful open and never publishes it', async () => {
  let finish!: (value: OpenedDashboard) => void
  const closed: string[] = []
  const sessions = new DashboardSessions({ open: async () => new Promise(resolve => { finish = resolve }),
    inspect: async () => { throw new Error('must not inspect') }, close: async id => { closed.push(id) } })
  const pending = sessions.select(installation, new AbortController().signal)
  await sessions.revoke()
  const value = opened(); finish(value)
  await assert.rejects(pending, { message: 'REQUEST_CANCELLED' })
  assert.deepEqual(closed, [value.launch.dashboardSessionId])
})

test('old disposal cannot remove a newer session and offline inspection does not allocate one', async () => {
  let count = 0, offline = false
  const inspections: string[] = []
  const sessions = new DashboardSessions({ open: async () => opened(++count),
    inspect: async id => { inspections.push(id); if (offline) throw new Error('DASHBOARD_CONNECTION_FAILED'); return opened(count) },
    close: async () => {},
  })
  const signal = new AbortController().signal
  const first = await sessions.select(installation, signal)
  await sessions.release(installation, first.launch.dashboardSessionId)
  const next = await sessions.select(installation, signal)
  await sessions.release(installation, first.launch.dashboardSessionId)
  offline = true
  await assert.rejects(sessions.select(installation, signal), { message: 'DASHBOARD_CONNECTION_FAILED' })
  offline = false
  await sessions.select(installation, signal)
  assert.equal(count, 2)
  assert.deepEqual(inspections, [next.launch.dashboardSessionId, next.launch.dashboardSessionId])
})

test('simultaneous view and cache disposal share one close request', async () => {
  let finish!: () => void, count = 0
  const value = opened()
  const sessions = new DashboardSessions({ open: async () => value, inspect: async () => value,
    close: async (_id, signal) => { count++; assert.ok(signal); await new Promise<void>(resolve => { finish = resolve }) } })
  await sessions.select(installation, new AbortController().signal)
  const first = sessions.revoke(), second = sessions.release(installation, value.launch.dashboardSessionId)
  assert.equal(count, 1)
  finish()
  await Promise.all([first, second])
})

test('generic dashboard route uses pure path segments and canonicalizes legacy query routes', () => {
  const target = dashboardLocation('music-mcp', '/instances?filter=running')
  assert.equal(target.path, '/apps/music-mcp/instances')
  assert.deepEqual(target.query, { filter: 'running' })
  assert.equal(browserDashboardPath({ params: { subpath: ['instances'] }, query: { filter: 'running' } }, 'music-mcp'), '/instances?filter=running')
  assert.equal(browserDashboardPath({ params: {}, query: { path: '/instances?filter=running' } }, 'music-mcp'), '/instances?filter=running')
  assert.deepEqual(dashboardLocation('project', '/projects?category=character'), { path: '/apps/project/character', query: {} })
  assert.deepEqual(dashboardLocation('resources', '/?view=tasks'), { path: '/apps/resources/tasks', query: {} })
  assert.equal(canonicalAppPath('project', '/projects?category=video&q=summer'), '/video?q=summer')
  assert.equal(dashboardSdkRoute('project', '/character?q=hero'), '/projects?category=character&q=hero')
  assert.equal(dashboardSdkRoute('resources', '/tasks'), '/?view=tasks')
  assert.equal(dashboardRoutePath(undefined), '/')
  for (const input of [['/one', '/two'], null, 'https://other.invalid', '//other.invalid', '/%2e%2e/private', '/%252fprivate']) {
    assert.throws(() => dashboardRoutePath(input))
  }
  assert.throws(() => dashboardLocation('Invalid_App'))
})

test('a late inspection cannot restore a closed session', async () => {
  let finish!: (value: OpenedDashboard) => void, count = 0
  const sessions = new DashboardSessions({ open: async () => opened(++count),
    inspect: async () => new Promise(resolve => { finish = resolve }), close: async () => {} })
  const signal = new AbortController().signal
  const original = await sessions.select(installation, signal)
  const checking = sessions.select(installation, signal)
  await sessions.release(installation, original.launch.dashboardSessionId)
  finish(original)
  await assert.rejects(checking, { message: 'REQUEST_CANCELLED' })
  assert.equal((await sessions.select(installation, signal)).launch.dashboardSessionId, opened(2).launch.dashboardSessionId)
})

test('overlapping opens retain only the latest selection and close the older response', async () => {
  const responses: Array<(value: OpenedDashboard) => void> = [], closed: string[] = []
  const sessions = new DashboardSessions({ open: async () => new Promise(resolve => { responses.push(resolve) }),
    inspect: async () => opened(2), close: async id => { closed.push(id) } })
  const signal = new AbortController().signal
  const first = sessions.select(installation, signal), second = sessions.select(installation, signal)
  responses[1](opened(2)); await second
  responses[0](opened(1))
  await assert.rejects(first, { message: 'REQUEST_CANCELLED' })
  assert.deepEqual(closed, [opened(1).launch.dashboardSessionId])
  assert.equal((await sessions.select(installation, signal)).launch.dashboardSessionId, opened(2).launch.dashboardSessionId)
})
