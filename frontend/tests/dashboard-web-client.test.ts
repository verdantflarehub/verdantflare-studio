import test from 'node:test'
import assert from 'node:assert/strict'
import { createDashboardWebClient } from '../src/app-dashboard/web-client.ts'

const installationId = '01980000-0000-7000-8000-000000000001'
const sessionId = '01980000-0000-4000-8000-000000000002'
function opened() {
  return { context: { userId: 'user-one', organizationId: 'org-one', stationId: 'station-one', projectId: null,
    appId: 'sample', installationId, contextRevision: 'epoch-one' }, launch: {
    dashboardSessionId: sessionId, installationId, appId: 'sample', appVersion: '1.0.0', artifactDigest: 'sha256:' + 'a'.repeat(64),
    adapter: 'wujie', entryUrl: `/studio/dashboards/${sessionId}/assets/dashboard/index.html`, sdkVersion: '1.0.0',
    grantedCapabilities: ['app.request'], contextRevision: 'epoch-one', expiresAt: '2099-01-01T00:00:00Z' } }
}
const json = (value: unknown, status = 200) => new Response(JSON.stringify(value), { status,
  headers: { 'Content-Type': 'application/json', 'X-Request-ID': 'request-one', 'Date': new Date().toUTCString() } })

test('Dashboard and workspace expiry follow server time, charge latency and reject missing time',async t=>{
  const server=Date.parse('2026-10-12T04:00:00Z')
  let elapsed=0,delay=0,date:string|null=new Date(server).toUTCString()
  const fetcher=async(url: any)=>{
    elapsed+=delay
    const value=String(url).includes('instance-workspaces')?workspace():opened()
    if('launch' in value)value.launch.expiresAt=new Date(server+30000).toISOString()
    else value.expiresAt=new Date(server+30000).toISOString()
    const response=json(value);if(date===null)response.headers.delete('Date');else response.headers.set('Date',date)
    return response
  }
  const client=createDashboardWebClient('https://studio.invalid',fetcher,()=>elapsed)
  for(const skew of [-3600000,3600000]){
    t.mock.method(Date,'now',()=>server+skew)
    const result=await client.open(installationId)
    assert.equal(result.policy.now!(),server+1000)
    assert.equal((await client.inspectWorkspace(sessionId,installationId)).now!(),server+1000)
  }
  // An older response cannot rewind the clock to revive a deadline.
  date=new Date(server-10000).toUTCString()
  await client.open(installationId)
  assert.equal(client.now(),server+1000)
  date=null
  await assert.rejects(client.open(installationId),{code:'INVALID_DASHBOARD_RESPONSE'})
  date='invalid'
  await assert.rejects(client.open(installationId),{code:'INVALID_DASHBOARD_RESPONSE'})
  date=new Date(server).toUTCString();delay=31000
  await assert.rejects(client.open(installationId),{code:'SESSION_EXPIRED'})
})

test('AppCenter opening uses its fixed binding endpoint and the original launch validator without fallback', async () => {
  const input = { schemaVersion: 1 as const, cardKey: `installation:${installationId}`, entryId: 'dashboard',
    expectedViewRevision: 'sha256:' + 'a'.repeat(64), expectedBindingRevision: 'sha256:' + 'b'.repeat(64) }
  for (const scenario of ['valid', 'changed', 'wrong-transport', 'private-field', 'foreign-launch']) {
    let calls = 0
    const client = createDashboardWebClient('https://studio.invalid', async (url, options) => {
      calls++
      assert.equal(url, 'https://studio.invalid/studio/appcenter/v1/open')
      assert.equal(options?.method, 'POST')
      assert.deepEqual(JSON.parse(String(options?.body)), input)
      if (scenario === 'changed') return json({code: 'APPCENTER_BINDING_CHANGED'}, 409)
      const payload: any = {schemaVersion: 1, transport: 'dashboard', opened: opened()}
      if (scenario === 'wrong-transport') payload.transport = 'legacy'
      if (scenario === 'private-field') payload.backend = 'private'
      if (scenario === 'foreign-launch') payload.opened.launch.entryUrl = 'https://foreign.invalid/entry'
      return json(payload)
    })
    if (scenario === 'valid') assert.equal((await client.openAppCenter(input, installationId)).launch.installationId, installationId)
    else await assert.rejects(client.openAppCenter(input, installationId))
    assert.equal(calls, 1)
  }
})

function workspace() { return {workspaceSessionId:sessionId,installationId,appId:'sample',instanceId:'instance-one',entryId:'editor',
  entryUrl:`/studio/instance-workspaces/${sessionId}/assets/vf/workspace.html`,
  assetBaseUrl:`/studio/instance-assets/${'a'.repeat(64)}/`,expiresAt:'2099-01-01T00:00:00Z'} }
test('instance open and workspace inspection preserve instance binding and use an independent close route', async () => {
  const calls: string[] = []
  const client = createDashboardWebClient('https://studio.invalid', async (url, options) => {
    calls.push(`${options?.method}:${new URL(String(url)).pathname}`)
    if (options?.method === 'POST') assert.deepEqual(JSON.parse(String(options.body)), {instanceId:'instance-one',entryId:'editor'})
    return options?.method === 'DELETE' ? new Response(null,{status:204}) : json(workspace())
  })
  await client.openInstance(sessionId,installationId,{instanceId:'instance-one',entryId:'editor'})
  await client.inspectWorkspace(sessionId,installationId)
  await client.closeWorkspace(sessionId)
  assert.deepEqual(calls,[`POST:/studio/dashboards/${sessionId}/instances/open`,
    `GET:/studio/instance-workspaces/${sessionId}`,`DELETE:/studio/instance-workspaces/${sessionId}`])
})
test('instance entry rejects substituted identities, external URLs, expired sessions and private metadata', async () => {
  for (const changes of [{instanceId:'other'},{installationId:'other'},{entryId:'other'},
    {entryUrl:'https://outside.invalid/editor'},{entryUrl:`/studio/instance-workspaces/${sessionId}/assets/../outside.html`},
    {assetBaseUrl:'https://outside.invalid/'},{token:'private'},{expiresAt:'2000-01-01T00:00:00Z'}]) {
    const client = createDashboardWebClient('https://studio.invalid',async()=>json({...workspace(),...changes}))
    await assert.rejects(client.openInstance(sessionId,installationId,{instanceId:'instance-one',entryId:'editor'}))
  }
})

test('Artifact capability rejects foreign session paths and arbitrary capability names', async () => {
  const handle = '01980000-0000-4000-8000-000000000009'
  let path = `/studio/dashboards/${sessionId}/content/${handle}`
  let calls = 0
  const client = createDashboardWebClient('https://studio.invalid', async (url) => {
    calls++
    assert.equal(url, `https://studio.invalid/studio/dashboards/${sessionId}/capabilities/artifact.read`)
    return json({ mode: 'download', content_path: path })
  })
  const input = { mode: 'download' }
  assert.deepEqual(await client.capability(sessionId, 'artifact.read', input), { mode: 'download', content_path: path })
  for (const invalid of [`https://elsewhere.invalid/${handle}`, `/studio/dashboards/${handle}/content/${handle}`, path+'?token=x']) {
    path = invalid
    await assert.rejects(client.capability(sessionId, 'artifact.read', input), { code: 'INVALID_CONTENT_RESPONSE' })
  }
  await assert.rejects(client.capability(sessionId, 'shell.exec', input), { code: 'CAPABILITY_UNAVAILABLE' })
  assert.equal(calls, 4)
})

test('Web client obtains platform context and confines assets to the returned session', async () => {
  const client = createDashboardWebClient('https://studio.invalid', async (url, options) => {
    assert.equal(url, 'https://studio.invalid/studio/dashboards')
    assert.equal(options?.credentials, 'same-origin')
    assert.equal(options?.redirect, 'error')
    assert.equal(options?.cache, 'no-store')
    assert.deepEqual(JSON.parse(String(options?.body)), { installationId })
    assert.deepEqual(options?.headers, { 'Content-Type': 'application/json' })
    return json(opened())
  })
  const result = await client.open(installationId)
  assert.equal(result.launch.appId, 'sample')
  assert.equal(result.policy.assetBaseUrl, `https://studio.invalid/studio/dashboards/${sessionId}/assets/`)
  assert.equal(result.context.projectId, null)
})

test('Web client rejects foreign entries, private fields and context substitution', async () => {
  for (const mutate of [
    (value: any) => { value.launch.entryUrl = 'https://other.invalid/index.html' },
    (value: any) => { value.launch.entryUrl = '/studio/dashboards/other/assets/index.html' },
    (value: any) => { value.context.installationId = 'other' },
    (value: any) => { value.context.token = 'should-never-appear' },
    (value: any) => { value.launch.contextRevision = 'other' },
    (value: any) => { value.launch.adapter = 'isolated-iframe' },
    (value: any) => { value.launch.backend = 'private' },
  ]) {
    const value = opened(); mutate(value)
    const client = createDashboardWebClient('https://studio.invalid', async () => json(value))
    await assert.rejects(client.open(installationId))
  }
})

test('Web client retains business conflict status and closes only the page session', async () => {
  const calls: string[] = []
  const client = createDashboardWebClient('https://studio.invalid', async (url, options) => {
    calls.push(String(url))
    if (options?.method === 'DELETE') {
      assert.equal(options.body, undefined)
      assert.equal(url, `https://studio.invalid/studio/dashboards/${sessionId}`)
      return new Response(null, { status: 204 })
    }
    assert.equal(url, `https://studio.invalid/studio/dashboards/${sessionId}/requests`)
    assert.deepEqual(JSON.parse(String(options?.body)), { method: 'POST', path: '/instances/two/start', body: { profile: null } })
    return json({ code: 'INSTANCE_BUSY' }, 409)
  })
  const response = await client.request(sessionId, { method: 'POST', path: '/instances/two/start', body: { profile: null } })
  assert.equal(response.status, 409)
  assert.equal(response.requestId, 'request-one')
  assert.deepEqual(response.data, { code: 'INSTANCE_BUSY' })
  await client.close(sessionId)
  assert.equal(calls.length, 2)
})

test('workspace capability preserves stable recovery errors and never retries an uncertain save', async () => {
  for (const code of ['PENDING_COMMIT', 'REVISION_CONFLICT', 'LOCAL_FILE_CONFLICT', 'WORKSPACE_BUSY', 'WORKSPACE_CANCELLED']) {
    let calls = 0
    const client = createDashboardWebClient('https://studio.invalid', async (url, options) => {
      calls++
      assert.equal(url, `https://studio.invalid/studio/dashboards/${sessionId}/capabilities/workspace.local`)
      assert.equal(options?.method, 'POST')
      return json({ code }, 409)
    })
    await assert.rejects(client.capability(sessionId, 'workspace.local', { operation: 'resume', workspace_handle: 'opaque' }), { code, message: code })
    assert.equal(calls, 1)
  }
})

test('Web client propagates cancellation without retrying or leaking transport details', async () => {
  const controller = new AbortController()
  let calls = 0
  const client = createDashboardWebClient('https://studio.invalid', async (_url, options) => {
    calls++
    assert.equal(options?.signal, controller.signal)
    controller.abort()
    throw new Error('private transport detail')
  })
  await assert.rejects(client.request(sessionId, { method: 'GET', path: '/instances' }, controller.signal), { code: 'REQUEST_CANCELLED' })
  await assert.rejects(client.open(installationId, controller.signal), { code: 'REQUEST_CANCELLED' })
  assert.equal(calls, 1)
})

test('Web client rejects oversized and non-JSON responses', async () => {
  for (const make of [
    () => new Response('<html>login</html>', { headers: { 'Content-Type': 'text/html' } }),
    () => new Response(' '.repeat((4 << 20) + 1), { headers: { 'Content-Type': 'application/json' } }),
    () => new Response(new Uint8Array([255]), { headers: { 'Content-Type': 'application/json' } }),
  ]) {
    const client = createDashboardWebClient('https://studio.invalid', async () => make())
    await assert.rejects(client.open(installationId), { code: 'INVALID_DASHBOARD_RESPONSE' })
  }
})

test('Web client reads declared entries without opening a session and inspects the original session', async () => {
  const calls: string[] = []
  const client = createDashboardWebClient('https://studio.invalid', async (url, options) => {
    calls.push(String(url)); assert.equal(options?.method, 'GET'); assert.equal(options?.body, undefined)
    return String(url).endsWith('dashboard-installations')
      ? json({ items: [{ installationId, appId: 'sample', appVersion: '1.0.0' }] }) : json(opened())
  })
  assert.deepEqual(await client.installations(), [{ installationId, appId: 'sample', appVersion: '1.0.0' }])
  assert.equal((await client.inspect(sessionId, installationId)).launch.dashboardSessionId, sessionId)
  assert.equal(calls.length, 2)
})

test('Web client rejects duplicate or private entry metadata and substituted inspection sessions', async () => {
  const item = { installationId, appId: 'sample', appVersion: '1.0.0' }
  for (const value of [{ items: [item, item] }, { items: [{ ...item, origin: 'private' }] }, { items: [{ ...item, installationId: 'pod-id' }] }]) {
    const client = createDashboardWebClient('https://studio.invalid', async () => json(value))
    await assert.rejects(client.installations(), { code: 'INVALID_DASHBOARD_RESPONSE' })
  }
  const other = opened(); other.launch.dashboardSessionId = '01980000-0000-4000-8000-000000000003'
  other.launch.entryUrl = `/studio/dashboards/${other.launch.dashboardSessionId}/assets/dashboard/index.html`
  const client = createDashboardWebClient('https://studio.invalid', async () => json(other))
  await assert.rejects(client.inspect(sessionId, installationId), { code: 'INVALID_DASHBOARD_RESPONSE' })
})
