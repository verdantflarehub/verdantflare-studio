import assert from 'node:assert/strict'
import test from 'node:test'
import { comfyuiBookmark, comfyuiBookmarkInstance, comfyuiManagementCard, comfyuiManagementLocation, readComfyUIBookmark } from '../src/app-center/comfyui-bookmark.ts'
import { legacyTarget } from '../src/app-center/legacy.ts'
import { reasonLabels, statusLabels } from '../src/app-center/labels.ts'
import type { AppCenterCard, AppCenterView } from '../src/app-center/schema.generated.ts'

const installationId = '01980000-0000-7000-8000-000000000001'
const card = { appId: 'comfyui', installationQuality: 'known', installation: { installationId, state: 'installed' },
  entry: { transport: 'dashboard', entryId: 'dashboard' }, actions: [{ id: 'open', kind: 'open', entryId: 'dashboard', enabled: true, reasonCode: null }] } as AppCenterCard
const view = { items: [card] } as AppCenterView
const instance = { alias: 'comfyuiA', name: '原实例', status: 'running', observed_at: '2026-10-14T00:00:00Z', editor_state: 'ready', allowed_actions: ['open'] }
const directory = { schema_version: 1, instances: [instance] }

test('historical management and exact instance bookmarks remain separate from legacy application adapters', () => {
  for (const alias of ['instances', 'comfyuiA', 'comfyui01980000000070008000000000000001']) {
    const route = '/market?comfyui=' + alias
    assert.deepEqual(comfyuiBookmark(route), { alias })
    assert.equal(legacyTarget(route), null)
  }
  assert.equal(comfyuiBookmark('/apps/' + installationId + '/dashboard'), null)
  assert.equal(comfyuiBookmark('/market?view=installed'), null)
  for (const route of ['/market?comfyui=', '/market?comfyui=instances&comfyui=comfyuiA', '/market?comfyui=comfyuiA&image=%2Fdashboard',
    '/market?comfyui=instances&view=installed', '/market?comfyui=instances#', '/market?comfyui=comfyuiA#evil',
    '/market?comfyui=comfyui%00A', '/market?comfyui=../other', '/market?comfyui=instances\t']) {
    assert.throws(() => comfyuiBookmark(route), /INVALID_LEGACY_ROUTE/, route)
    assert.throws(() => legacyTarget(route), /INVALID_LEGACY_ROUTE/, route)
  }
})

test('management bookmark uses current authorized installation and its existing Dashboard', () => {
  assert.equal(comfyuiManagementCard(view), card)
  assert.deepEqual(comfyuiManagementLocation(card), { path: `/apps/${installationId}/dashboard`, query: {} })
  const newId = '01980000-0000-7000-8000-000000000002'
  const current = { ...card, installation: { ...card.installation!, installationId: newId } }
  assert.equal(comfyuiManagementLocation(comfyuiManagementCard({ ...view, items: [current] })).path, `/apps/${newId}/dashboard`)
  assert.throws(() => comfyuiManagementCard({ ...view, items: [card, current] }), /LEGACY_ENTRY_AMBIGUOUS/)
  assert.throws(() => comfyuiManagementCard({ ...view, items: [] }), /APPCENTER_OPEN_UNAVAILABLE/)
  for (const change of [{ installation: null }, { installation: { ...card.installation!, state: 'removed' } },
    { installationQuality: 'conflict' }, { entry: null }, { entry: { ...card.entry!, transport: 'legacy' } }]) {
    assert.throws(() => comfyuiManagementCard({ ...view, items: [{ ...card, ...change } as AppCenterCard] }), /APPCENTER_OPEN_UNAVAILABLE/)
  }
  assert.throws(() => comfyuiManagementCard({ ...view, items: [{ ...card, actions: [{ ...card.actions[0], enabled: false, reasonCode: 'PERMISSION_DENIED' }] }] }), /PERMISSION_DENIED/)
})

test('old instance bookmark must match the old authenticated directory and never invent a manager instance', () => {
  assert.deepEqual(comfyuiBookmarkInstance(directory, 'comfyuiA'), { alias: 'comfyuiA', name: '原实例' })
  assert.throws(() => comfyuiBookmarkInstance(directory, 'comfyuiB'), /INSTANCE_NOT_FOUND/)
  assert.throws(() => comfyuiBookmarkInstance({ ...directory, instances: [instance, instance] }, 'comfyuiA'), /LEGACY_ENTRY_AMBIGUOUS/)
  for (const change of [{ status: 'stopped' }, { observed_at: null }, { editor_state: 'unavailable' }, { allowed_actions: [] }]) {
    assert.throws(() => comfyuiBookmarkInstance({ ...directory, instances: [{ ...instance, ...change }] }, 'comfyuiA'), /INSTANCE_UNAVAILABLE/)
  }
  for (const body of [null, {}, { ...directory, schema_version: 2 }, { ...directory, instances: [{ ...instance, name: null }] }]) {
    assert.throws(() => comfyuiBookmarkInstance(body, 'comfyuiA'), /APPCENTER_INVALID_VIEW/)
  }
})

test('instance recovery uses only the existing authenticated read route and respects denial, cancellation and response bounds', async () => {
  const controller = new AbortController()
  const read = (fetcher: typeof fetch) => readComfyUIBookmark('comfyuiA', controller.signal, fetcher)
  assert.deepEqual(await read(async (url, options) => {
    assert.equal(url, '/studio/apps/comfyui/instances')
    assert.equal(options!.method, undefined)
    assert.equal(options!.credentials, 'same-origin')
    assert.equal(options!.redirect, 'error')
    assert.equal(options!.signal, controller.signal)
    return Response.json(directory)
  }), { alias: 'comfyuiA', name: '原实例' })
  for (const [status, code] of [[401, 'UNAUTHENTICATED'], [403, 'PERMISSION_DENIED'], [503, 'APPCENTER_DEPENDENCY_UNAVAILABLE']] as const) {
    await assert.rejects(read(async () => new Response(null, { status })), new RegExp(code))
  }
  await assert.rejects(read(async () => new Response('{}')), /APPCENTER_INVALID_VIEW/)
  await assert.rejects(read(async () => new Response('x'.repeat((1 << 20) + 1), { headers: { 'Content-Type': 'application/json' } })), /APPCENTER_INVALID_VIEW/)
  await assert.rejects(readComfyUIBookmark('../other', controller.signal, async () => { throw Error('must not fetch') }), /INVALID_LEGACY_ROUTE/)
  await assert.rejects(read(async () => { controller.abort(); return Response.json(directory) }), /REQUEST_CANCELLED/)
})

test('user-facing status and errors contain no internal reconciliation requirement', () => {
  for (const label of [...Object.values(statusLabels), ...Object.values(reasonLabels)]) assert.doesNotMatch(label, /对账/)
})
