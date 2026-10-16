import test from 'node:test'
import assert from 'node:assert/strict'
import { createAppSDK, assertAppPath } from '../src/app-dashboard/sdk.ts'
import type { AppContext, DashboardLaunch, SDKAdapters } from '../src/app-dashboard/types.ts'

const initial: AppContext = {
  userId: 'user-1', organizationId: 'org-1', stationId: 'station-1', projectId: null,
  appId: 'sample', installationId: 'install-1', contextRevision: 'r1',
}

test('host route generations reject stale updates; theme and route listeners are revoked', t => {
  const session = fixture()
  t.after(session.dispose)
  const paths: string[] = [], themes: string[] = []
  session.sdk.navigation.subscribe(route => paths.push(route.path))
  session.sdk.theme.subscribe(theme => themes.push(theme))
  session.setRoute({ path: '/detail?version=2', revision: 2 })
  session.setRoute({ path: '/old', revision: 1 })
  assert.equal(session.sdk.navigation.get().path, '/detail?version=2')
  assert.throws(() => session.setRoute({ path: '/different', revision: 2 }), { code: 'ROUTE_REVISION_REQUIRED' })
  assert.throws(() => session.setRoute({ path: '//outside', revision: 3 }), { code: 'INVALID_PATH' })
  session.setVisibility(false)
  session.setTheme('dark')
  session.setRoute({ path: '/', revision: 3 })
  assert.deepEqual(paths, ['/detail?version=2', '/'])
  assert.deepEqual(themes, ['dark'])
  assert.deepEqual(session.calls, [])
  session.dispose()
  assert.throws(() => session.sdk.theme.get(), { code: 'SESSION_REVOKED' })
  assert.throws(() => session.setRoute({ path: '/', revision: 4 }), { code: 'SESSION_REVOKED' })
})
function fixture(overrides: Partial<SDKAdapters> = {}, grants = ['app.request', 'instances.open', 'files.select']) {
  const calls: unknown[] = []
  const launch: DashboardLaunch = {
    dashboardSessionId: 'session-1', installationId: 'install-1', appId: 'sample', appVersion: '1.0.0',
    artifactDigest: `sha256:${'a'.repeat(64)}`, adapter: 'wujie', entryUrl: '/dashboard/sample/',
    sdkVersion: '1.0.0', grantedCapabilities: grants, contextRevision: 'r1',
    expiresAt: new Date(Date.now() + 60_000).toISOString(),
  }
  const adapters: SDKAdapters = {
    navigate: async input => { calls.push(input) },
    selectFiles: async () => [{ handle: 'file-1', name: 'a.json', size: 2, mime: 'application/json', privatePath: 'private' }],
    callCapability: async () => null,
    requestApp: async input => { calls.push(input); return { status: 200, data: [], requestId: 'req-1' } },
    openInstance: async input => { calls.push(input) }, hasUserActivation: () => true,
    reportListenerError: error => { calls.push(error) }, ...overrides,
  }
  return { ...createAppSDK(launch, initial, adapters), calls, launch }
}

test('media display uses existing bytes and closes on navigation, hide and disposal', async t => {
  let opened = 0, closed = 0
  const session = fixture({ previewMedia: async () => { opened++ }, closeMedia: () => { closed++ } }, [])
  t.after(session.dispose)
  const input = { bytes: new Uint8Array([1]).buffer, mime: 'image/png', title: '参考图' }
  await session.sdk.media.preview(input)
  assert.equal(opened, 1)
  await assert.rejects(session.sdk.media.preview({ ...input, mime: 'image/svg+xml' }), /INVALID_MEDIA_PREVIEW/)
  session.setRoute({ path: '/next', revision: 1 }); assert.equal(closed, 1)
  session.setVisibility(false); assert.equal(closed, 2)
  await assert.rejects(session.sdk.media.preview(input), /DASHBOARD_HIDDEN/)
  session.dispose(); assert.equal(closed, 3)
  await assert.rejects(session.sdk.media.preview(input), /SESSION_REVOKED/)
})

test('one SDK opens multiple instances; hiding and disposing never invoke instance operations', async t => {
  const session = fixture()
  t.after(session.dispose)
  await session.sdk.instances.open({ instanceId: 'instance-a', entryId: 'editor' })
  await session.sdk.instances.open({ instanceId: 'instance-b', entryId: 'editor' })
  const events: string[] = []
  session.sdk.lifecycle.subscribe(event => events.push(event))
  session.setVisibility(false)
  session.setVisibility(true)
  session.dispose()
  session.dispose()
  assert.deepEqual(events, ['hidden', 'active', 'dispose'])
  assert.equal(session.calls.length, 2)
  await assert.rejects(session.sdk.instances.open({ instanceId: 'instance-a', entryId: 'editor' }), { code: 'SESSION_REVOKED' })
})

test('hiding cancels file selection and read; context replacement and disposal release handles', async t => {
  let releases = 0
  const session = fixture({selectFiles: () => new Promise(() => {}), callCapability: () => new Promise(() => {}),
    releaseFiles: () => { releases++ }}, ['files.select', 'files.read'])
  t.after(session.dispose)
  const select = assert.rejects(session.sdk.files.select({purpose:'json.import'}), {code:'REQUEST_CANCELLED'})
  session.setVisibility(false); await select
  session.setVisibility(true)
  const read = assert.rejects(session.sdk.capabilities.call('files.read',{handle:'opaque',purpose:'json.import'}), {code:'REQUEST_CANCELLED'})
  session.setVisibility(false); await read
  session.updateContext({...initial, projectId:'project-2', contextRevision:'r2'})
  assert.equal(releases,1)
  session.dispose();session.dispose()
  assert.equal(releases,2)
})

test('grants and initial context are copied and exposed without host internals', async t => {
  const grants = ['app.request']
  const session = fixture({}, grants)
  t.after(session.dispose)
  grants.push('instances.open')
  await assert.rejects(session.sdk.instances.open({ instanceId: 'a', entryId: 'editor' }), { code: 'CAPABILITY_DENIED' })
  assert.throws(() => { (session.sdk.context.get() as AppContext).userId = 'other' }, TypeError)
  assert.equal(session.sdk.context.get().userId, 'user-1')
  assert.equal('dispose' in session.sdk, false)
})

test('reject URL, encoded traversal and disallowed methods before transport', async t => {
  const session = fixture()
  t.after(session.dispose)
  for (const path of ['https://invalid.test', '//host/path', '/../private', '/%2e%2e/private', '/%252e/private', '/a%2fb', '/a\\b', '/%00']) {
    assert.throws(() => assertAppPath(path), { code: 'INVALID_PATH' })
  }
  assertAppPath('/instances?filter=running')
  for (const path of ['/instances?filter=running', '/instances/', '/instances//one', '/%69nstances']) {
    await assert.rejects(session.sdk.app.request({ path, method: 'GET' }), { code: 'INVALID_PATH' })
  }
  await assert.rejects(session.sdk.app.request({ path: '/', method: 'TRACE' as 'GET' }), { code: 'INVALID_REQUEST' })
  assert.deepEqual(session.calls, [])
})

test('hidden cancels reads, retains accepted writes, and blocks new requests', async t => {
  const pending: Array<(value: { status: number; data: null; requestId: string }) => void> = []
  const session = fixture({ requestApp: async () => new Promise(resolve => pending.push(resolve)) })
  t.after(session.dispose)
  const read = session.sdk.app.request({ path: '/instances', method: 'GET' })
  const write = session.sdk.app.request({ path: '/instances', method: 'POST', body: {} })
  session.setVisibility(false)
  await assert.rejects(read, { code: 'REQUEST_CANCELLED' })
  await assert.rejects(session.sdk.app.request({ path: '/instances', method: 'GET' }), { code: 'DASHBOARD_HIDDEN' })
  pending[1]({ status: 202, data: null, requestId: 'op-1' })
  assert.equal((await write).status, 202)
})

test('revocation settles callers even if transport ignores cancellation; stale response cannot escape', async () => {
  let resolve!: (value: { status: number; data: null; requestId: string }) => void
  const session = fixture({ requestApp: () => new Promise(done => { resolve = done }) })
  const read = session.sdk.app.request({ path: '/instances', method: 'GET' })
  session.dispose()
  await assert.rejects(read, { code: 'REQUEST_CANCELLED' })
  resolve({ status: 200, data: null, requestId: 'stale' })
  assert.throws(() => session.sdk.context.get(), { code: 'SESSION_REVOKED' })
})

test('cross-principal context changes revoke; project changes require new revision', t => {
  const session = fixture()
  t.after(session.dispose)
  assert.throws(() => session.updateContext({ ...initial, projectId: 'project-2' }), { code: 'CONTEXT_REVISION_REQUIRED' })
  session.updateContext({ ...initial, projectId: 'project-2', contextRevision: 'r2' })
  assert.equal(session.sdk.context.get().projectId, 'project-2')
  assert.throws(() => session.updateContext({ ...initial, userId: 'user-2' }), { code: 'CONTEXT_MISMATCH' })
  assert.throws(() => session.sdk.context.get(), { code: 'SESSION_REVOKED' })
})

test('file selection requires activation and strips private adapter fields', async t => {
  const denied = fixture({ hasUserActivation: () => false })
  const allowed = fixture()
  t.after(denied.dispose); t.after(allowed.dispose)
  await assert.rejects(denied.sdk.files.select({ purpose: 'workflow' }), { code: 'USER_ACTIVATION_REQUIRED' })
  assert.deepEqual(await allowed.sdk.files.select({ purpose: 'workflow' }), [
    { handle: 'file-1', name: 'a.json', size: 2, mime: 'application/json' },
  ])
})

test('native workspace selection requires a current gesture; resume keeps its original recovery error', async t => {
  const calls: unknown[] = []
  const session = fixture({ hasUserActivation: () => false, callCapability: async (_name, input) => {
    calls.push(input); throw Object.assign(new Error('PENDING_COMMIT'), { code: 'PENDING_COMMIT' })
  } }, ['workspace.local'])
  t.after(session.dispose)
  for (const operation of ['openDirectory', 'importFiles']) {
    await assert.rejects(session.sdk.capabilities.call('workspace.local', { operation }), { code: 'USER_ACTIVATION_REQUIRED' })
  }
  assert.equal(calls.length, 0)
  await assert.rejects(session.sdk.capabilities.call('workspace.local', { operation: 'resume', workspace_handle: 'opaque' }), { code: 'PENDING_COMMIT' })
  assert.equal(calls.length, 1)
})

test('throwing listeners do not prevent remaining listeners or disposal', () => {
  const session = fixture()
  const events: string[] = []
  session.sdk.lifecycle.subscribe(() => { throw new Error('listener failed') })
  session.sdk.lifecycle.subscribe(event => events.push(event))
  session.dispose()
  assert.deepEqual(events, ['dispose'])
  assert.equal(session.calls.length, 1)
})

test('expiry revokes an idle session and releases subscribers without another call', t => {
  t.mock.timers.enable({ apis: ['Date', 'setTimeout'], now: 1000 })
  const session = fixture()
  const events: string[] = []
  session.sdk.lifecycle.subscribe(event => events.push(event))
  t.mock.timers.tick(60_001)
  assert.deepEqual(events, ['dispose'])
  assert.throws(() => session.sdk.context.get(), { code: 'SESSION_REVOKED' })
})

test('bounded concurrency and deadline release callers even when transport hangs', async t => {
  t.mock.timers.enable({ apis: ['Date', 'setTimeout'], now: 1000 })
  const session = fixture({ requestApp: () => new Promise(() => {}) })
  t.after(session.dispose)
  const reads = Array.from({ length: 16 }, () => session.sdk.app.request({ path: '/instances', method: 'GET' }))
  await assert.rejects(session.sdk.app.request({ path: '/instances', method: 'GET' }), { code: 'TOO_MANY_REQUESTS' })
  const rejected = reads.map(read => assert.rejects(read, { code: 'REQUEST_CANCELLED' }))
  t.mock.timers.tick(30_001)
  await Promise.all(rejected)
})

test('non-JSON and oversized requests never reach an adapter', async t => {
  const session = fixture()
  t.after(session.dispose)
  for (const body of [NaN, { field: undefined }, 'x'.repeat(1024 * 1024 + 1)]) {
    await assert.rejects(session.sdk.app.request({ path: '/instances', method: 'POST', body: body as never }), { code: 'INVALID_BODY' })
  }
  assert.deepEqual(session.calls, [])
})
