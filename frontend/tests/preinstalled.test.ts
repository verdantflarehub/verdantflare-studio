import assert from 'node:assert/strict'
import test from 'node:test'
import { createPreinstalledClient, createPreinstalledIntentStore, decodePreinstalled, preinstalledIntent } from '../src/navigation/preinstalled.ts'
import type { PreinstalledStatus } from '../src/navigation/preinstalled.ts'

const uid = '019a00b0-0000-7000-8000-000000000001'
const scope = JSON.stringify([uid, '019a00b0-0000-7000-8000-000000000002', '019a00b0-0000-7000-8000-000000000003'])
const other = JSON.stringify([uid, '019a00b0-0000-7000-8000-000000000002', '019a00b0-0000-7000-8000-000000000004'])
function status(): PreinstalledStatus {
  return { enabled: true, configDigest: `sha256:${'a'.repeat(64)}`, items: [{ appId: 'resources', state: 'adoption_required', code: '', operationId: '', installationId: '',
    adoptionEvidence: { expectedWorkloadUid: uid, templateDigest: `sha256:${'b'.repeat(64)}`, managementStatus: 'unmanaged', observedAt: '2026-10-11T06:00:00Z' } }] }
}
function storage() {
  const values = new Map<string, string>()
  return { getItem: (key: string) => values.get(key) ?? null, setItem: (key: string, value: string) => { values.set(key, value) }, removeItem: (key: string) => { values.delete(key) } }
}
const response = () => new Response(JSON.stringify(status()), { headers: { 'Content-Type': 'application/json' } })

test('only complete observed ownership and fixed release evidence enables adoption', () => {
  const value = status(), decoded = decodePreinstalled(value)
  const intent = preinstalledIntent('adopt', decoded.items[0])
  assert.deepEqual(intent.adopt, { resources: uid })
  assert.deepEqual(intent.adoptTemplateDigests, { resources: `sha256:${'b'.repeat(64)}` })
  assert.throws(() => preinstalledIntent('adopt', { ...decoded.items[0], adoptionEvidence: undefined }))
  for (const corrupt of [
    (v: any) => { v.items[0].adoptionEvidence.managementStatus = 'foreign' },
    (v: any) => { v.items[0].adoptionEvidence.templateDigest = 'latest' },
    (v: any) => { v.items[0].adoptionEvidence.expectedWorkloadUid = 'resources' },
    (v: any) => { v.items.push(v.items[0]) },
    (v: any) => { v.enabled = false },
    (v: any) => { v.items[0].state = 'succeeded' },
  ]) { const bad = status(); corrupt(bad); assert.throws(() => decodePreinstalled(bad)) }
})

test('lost replies survive reload and replay exactly the original explicit intent', async () => {
  const saved = storage(), journal = createPreinstalledIntentStore(saved), intent = preinstalledIntent('reinstall', status().items[0])
  journal.save(scope, intent)
  const bodies: string[] = []; let fail = true
  const client = createPreinstalledClient(async (_path, options) => {
    bodies.push(String(options?.body)); if (fail) throw new Error('lost reply'); return response()
  })
  await assert.rejects(client.execute(intent))
  assert.equal(Object.hasOwn(JSON.parse(bodies[0]), 'adoptTemplateDigests'), false)
  const reloaded = createPreinstalledIntentStore(saved)
  assert.equal(bodies.length, 1)
  assert.deepEqual(reloaded.read(scope), intent)
  assert.throws(() => reloaded.save(scope, preinstalledIntent('fill')), /UNRESOLVED/)
  fail = false; await client.execute(reloaded.read(scope)!)
  assert.equal(bodies[0], bodies[1])
  reloaded.clear(scope, intent.intentId); assert.equal(reloaded.read(scope), null)
})

test('unresolved intent is isolated by Station and stale acknowledgments cannot clear a newer intent', () => {
  const journal = createPreinstalledIntentStore(storage()), first = preinstalledIntent('fill'), next = preinstalledIntent('fill')
  journal.save(scope, first)
  assert.equal(journal.read(other), null)
  journal.clear(other, first.intentId); assert.deepEqual(journal.read(scope), first)
  journal.clear(scope, first.intentId); journal.save(scope, next)
  journal.clear(scope, first.intentId); assert.deepEqual(journal.read(scope), next)
  assert.throws(() => journal.read(JSON.stringify([uid, uid, undefined])))
})

test('status reads never submit commands; writes use only fixed local endpoints and validated evidence', async () => {
  const calls: { path: string; options: RequestInit }[] = []
  const client = createPreinstalledClient(async (path, options) => { calls.push({ path: String(path), options: options! }); return response() })
  await client.read(); assert.equal(calls[0].options.method, 'GET'); assert.equal(calls[0].options.body, undefined)
  const intent = preinstalledIntent('adopt', status().items[0]); await client.execute(intent)
  assert.deepEqual(JSON.parse(String(calls[1].options.body)).adoptTemplateDigests, intent.adoptTemplateDigests)
  assert.deepEqual(calls.map(call => call.path), ['/studio/preinstalled', '/studio/preinstalled/reconcile'])
  for (const call of calls) { assert.equal(call.options.credentials, 'same-origin'); assert.equal(call.options.redirect, 'error'); assert.equal(call.options.cache, 'no-store') }
  await assert.rejects(client.execute({ ...intent, adoptTemplateDigests: {} }))
  await assert.rejects(client.execute({ ...intent, retry: ['resources'] }))
  assert.equal(calls.length, 2)
})

test('oversized or non-JSON status responses never become installation facts', async () => {
  for (const bad of [new Response('{}', { headers: { 'Content-Type': 'text/html' } }), new Response(' '.repeat(128 * 1024 + 1), { headers: { 'Content-Type': 'application/json' } })]) {
    await assert.rejects(createPreinstalledClient(async () => bad).read(), /INVALID_RESPONSE/)
  }
})
