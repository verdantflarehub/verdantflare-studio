import assert from 'node:assert/strict'
import test from 'node:test'
import { createLegacyClient, legacyLocation, legacyTarget, selectLegacyCard } from '../src/app-center/legacy.ts'
import { mediaAdapter, validMediaRequest } from '../src/app-center/legacy-media.ts'
import { createBlenderFetcher, validStudioOrigin, blenderMCPAddress } from '../src/app-center/legacy-blender.ts'
import type { AppCenterCard, AppCenterView } from '../src/app-center/schema.generated.ts'

const digest = `sha256:${'a'.repeat(64)}`, key = `obs:${'b'.repeat(64)}`
const card = { cardKey: key, bindingRevision: digest, entry: { transport: 'legacy', entryId: 'image-iframe-v1' } } as AppCenterCard
const view = { revision: digest, items: [card] } as AppCenterView
const opened = { schemaVersion: 1, transport: 'legacy', adapterId: 'image-iframe-v1', bindingId: 'old-deployment', bindingRevision: digest, route: '/market?image=%2Fdashboard%23tasks' }

test('desktop media opens a bound local workspace and releases it without changing the remote JSON contract', async () => {
  const handle = '01980000-0000-4000-8000-000000000001', calls: string[] = []
  const client = createLegacyClient(async (url, options) => {
    calls.push(`${options!.method} ${url}`)
    assert.equal(options!.credentials, 'omit')
    if (options!.method === 'DELETE') return new Response(null, { status: 204 })
    assert.equal(url, '/studio/legacy-media')
    return Response.json(opened, { headers: { 'X-Studio-Legacy-Workspace': handle } })
  }, true)
  const result = await client.open(view, card)
  assert.deepEqual(result, { ...opened, resourceBase: '/studio/legacy-media/' + handle })
  await client.close(result)
  assert.deepEqual(calls, ['POST /studio/legacy-media', 'DELETE /studio/legacy-media/' + handle])
})

test('desktop Blender binds management and MCP origin to the original workspace', async () => {
  const handle = '01980000-0000-4000-8000-000000000001', resourceBase = '/studio/legacy-blender/' + handle
  const blender = { ...card, entry: { ...card.entry!, entryId: 'blender-host-v1' } }
  const reply = { ...opened, adapterId: 'blender-host-v1', route: '/market?blender=instances' }
  for (const origin of ['https://studio.invalid', 'http://127.0.0.1:9000', null, 'https://studio.invalid/', 'https://user@studio.invalid', 'http://untrusted.invalid', 'https://studio.invalid/path']) {
    let closed = 0
    const client = createLegacyClient(async (url, init) => {
      if (init!.method === 'DELETE') { assert.equal(url, resourceBase); closed++; return new Response(null, { status: 204 }) }
      assert.equal(url, '/studio/legacy-blender')
      return Response.json(reply, { headers: { 'X-Studio-Legacy-Workspace': handle, ...(origin ? { 'X-Studio-Legacy-Origin': origin } : {}) } })
    }, true)
    if (validStudioOrigin(origin)) {
      assert.deepEqual(await client.open(view, blender), { ...reply, resourceBase, mcpOrigin: origin })
      assert.equal(blenderMCPAddress(origin, 'blenderA', '/mcp/blenderA'), origin + '/mcp/blenderA')
    } else { await assert.rejects(client.open(view, blender), /APPCENTER_INVALID_OPEN/); assert.equal(closed, 1) }
  }
  const controller = new AbortController(), raw = '{ "id": "original", "method": "tools/call" }'
  const calls: string[] = []
  const fetcher = createBlenderFetcher(resourceBase, async (url, init) => {
    assert.equal(url, resourceBase + '/requests'); assert.equal(init!.signal, controller.signal); assert.equal(init!.credentials, 'omit')
    calls.push(init!.body as string); return Response.json({ ok: true })
  })
  await fetcher('/mcp/blenderA', { method: 'POST', body: raw, signal: controller.signal })
  await fetcher('/studio/apps/blender/instances', { signal: controller.signal })
  assert.deepEqual(calls.map(value => JSON.parse(value)), [{ method: 'POST', path: '/mcp/blenderA', body: raw }, { method: 'GET', path: '/studio/apps/blender/instances', body: '' }])
  for (const path of ['https://other.invalid/mcp/blenderA', '/api/commands', '/mcp/blenderA?secret=x', '/mcp/%62lenderA']) await assert.rejects(fetcher(path))
  assert.throws(() => blenderMCPAddress('https://studio.invalid', 'blenderA', '/mcp/blenderB'))
  assert.equal(calls.length, 2)
})

test('desktop rejects missing handles and closes a lease whose reply is stale or invalid', async () => {
  await assert.rejects(createLegacyClient(async () => Response.json(opened), true).open(view, card), /HOST_UNSUPPORTED/)
  const controller = new AbortController(), handle = '01980000-0000-4000-8000-000000000001'
  let closed = 0
  const client = createLegacyClient(async (_url, options) => {
    if (options!.method === 'DELETE') { closed++; return new Response(null, { status: 204 }) }
    controller.abort()
    return Response.json(opened, { headers: { 'X-Studio-Legacy-Workspace': handle } })
  }, true)
  await assert.rejects(client.open(view, card, controller.signal), /REQUEST_CANCELLED/)
  assert.equal(closed, 1)
})

test('finite legacy bookmarks preserve explicit card context and reject mixed routes', () => {
  for (const route of [opened.route, '/market?video=%2Fdashboard%2Ftasks%2Fsample', '/market?blender=instances', '/market?blender=blender123&panel=overview']) {
    const target = legacyTarget(route)!
    assert.equal(target.route, route)
    const withCard = legacyLocation(target, key)
    assert.equal(legacyTarget(withCard)?.cardKey, key)
    assert.equal(legacyTarget(withCard)?.route, route)
  }
  assert.equal(legacyTarget('/apps/installation/dashboard?path=/'), null)
  assert.equal(legacyTarget('/market?card=sample'), null)
  for (const route of ['/market?image=%2Fdashboard&video=%2Fdashboard', '/market?image=%2Fdashboard&image=%2Fdashboard',
    '/market?image=%2Fdashboard&admin=true', '/market?image=%2Fdashboard%2F..%2Fapi', '/market?blender=instances&panel=overview',
    '/market?blender=blender123&panel=invalid', '/market?blender=blender123&operation=invalid', '/market?image=%2Fdashboard%23evil',
    '/market?image=%2Fdashboard&legacyCard=bad/path', '/market?comfyui=instances', '/market?comfyui=instances#invalid',
    '/market?image=%2Fdashboard#invalid', '/market?image=%2Fdashboard#', '/market?image=%2Fdash\tboard']) assert.throws(() => legacyTarget(route), undefined, route)
})

test('bookmark resolution never guesses among same-adapter objects or requires an installation', () => {
  const target = legacyTarget(opened.route)!
  assert.equal(selectLegacyCard(view, target).cardKey, key)
  const other = { ...card, cardKey: `obs:${'c'.repeat(64)}` }
  const many = { ...view, items: [card, other] }
  assert.throws(() => selectLegacyCard(many, target), /LEGACY_ENTRY_AMBIGUOUS/)
  assert.equal(selectLegacyCard(many, { ...target, cardKey: other.cardKey }).cardKey, other.cardKey)
  assert.throws(() => selectLegacyCard({ ...view, items: [] }, target), /LEGACY_ENTRY_UNMAPPED/)
  assert.throws(() => selectLegacyCard(view, { ...target, adapterId: 'video-iframe-v1' }), /LEGACY_ENTRY_UNMAPPED/)
})

test('legacy open pins the exact input and rejects alternate adapters, routes, extra fields and stale replies', async () => {
  let calls = 0
  const client = createLegacyClient(async (url, options) => {
    calls++; assert.equal(url, '/studio/appcenter/v1/open'); assert.equal(options!.method, 'POST'); assert.equal(options!.redirect, 'error')
    assert.deepEqual(JSON.parse(options!.body as string), { schemaVersion: 1, cardKey: key, entryId: 'image-iframe-v1', expectedViewRevision: digest, expectedBindingRevision: digest })
    return Response.json(opened)
  })
  assert.deepEqual(await client.open(view, card), opened)
  assert.equal(calls, 1)
  for (const change of [{ adapterId: 'video-iframe-v1' }, { bindingRevision: `sha256:${'c'.repeat(64)}` },
    { route: 'https://other.invalid' }, { route: '/market?video=%2Fdashboard' }, { endpoint: 'http://other.invalid' }, { bindingId: '../other' },
    { route: opened.route + '&legacyCard=' + key }]) {
    const invalid = createLegacyClient(async () => Response.json({ ...opened, ...change }))
    await assert.rejects(invalid.open(view, card))
  }
  await assert.rejects(createLegacyClient(async () => Response.json({ code: 'APPCENTER_BINDING_CHANGED' }, { status: 409 })).open(view, card), /APPCENTER_BINDING_CHANGED/)
  await assert.rejects(createLegacyClient(async () => new Response('x'.repeat(17000), { headers: { 'Content-Type': 'application/json' } })).open(view, card), /APPCENTER_INVALID_OPEN/)
  const controller = new AbortController()
  await assert.rejects(createLegacyClient(async () => { controller.abort(); return Response.json(opened) }).open(view, card, controller.signal), /REQUEST_CANCELLED/)
})

test('iframe requests stay within the old finite API and do not select arbitrary upstreams', () => {
  const image = mediaAdapter('image-iframe-v1')!, video = mediaAdapter('video-iframe-v1')!
  assert.equal(mediaAdapter('blender-host-v1'), null)
  assert.equal(validMediaRequest(image, 'GET', '/api/tasks?limit=20', undefined), true)
  assert.equal(validMediaRequest(image, 'POST', '/api/tasks', '{}'), true)
  assert.equal(validMediaRequest(video, 'GET', '/api/models/test/instances/worker/gpus/0', undefined), true)
  assert.equal(validMediaRequest(video, 'POST', '/api/tasks/task123/result', '{}'), true)
  assert.equal(validMediaRequest(video, 'GET', '/api/capabilities', undefined), true)
  assert.equal(validMediaRequest(video, 'POST', '/api/capabilities', '{}'), false)
  assert.equal(validMediaRequest(video, 'GET', '/api/capabilities/secrets', undefined), false)
  assert.equal(validMediaRequest(image, 'GET', '/api/capabilities', undefined), false)
  for (const [method, path, body] of [['GET', 'https://other.invalid/api/tasks', undefined], ['DELETE', '/api/tasks', undefined],
    ['GET', '/api/%74asks', undefined], ['GET', '/api/tasks#x', undefined], ['POST', '/api/models', '{}'],
    ['GET', '/api/tasks?q=a\nb', undefined], ['GET', '/api/tasks?q=a\\b', undefined], ['GET', '/api/tasks?q=\u007f', undefined],
    ['GET', '/api/tasks', '{}'], ['POST', '/api/tasks', {}], ['POST', '/api/tasks', 'x'.repeat(1048577)]]) {
    assert.equal(validMediaRequest(image, method as string, path as string, body), false)
  }
})

test('open binding supports pinning and taking revision with opened payload', () => {
  const map = new Map<string, { revision: string; payload?: unknown }>()
  const pinOpen = (id: string, binding: string, payload?: unknown) => { map.set(id, { revision: binding, payload }) }
  const takeOpenBinding = (id: string) => { const item = map.get(id); map.delete(id); return item?.revision }
  const takeOpenEntry = <T = unknown>(id: string) => { const item = map.get(id); map.delete(id); return item as { revision: string; payload?: T } | undefined }

  pinOpen('card-1', 'rev-1', opened)
  const entry = takeOpenEntry<typeof opened>('card-1')
  assert.equal(entry?.revision, 'rev-1')
  assert.deepEqual(entry?.payload, opened)
  assert.equal(takeOpenBinding('card-1'), undefined)

  pinOpen('card-2', 'rev-2')
  assert.equal(takeOpenBinding('card-2'), 'rev-2')
  assert.equal(takeOpenBinding('card-2'), undefined)
})

