import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { createCommands } from '../src/app-center/commands.ts'
import { commandScope, createCommandStorage, parsePending } from '../src/app-center/command-storage.ts'
import type { CommandStorage, PendingCommand } from '../src/app-center/command-storage'
import type { CommandTransport } from '../src/app-center/command-transport'
import { createCommandTransport } from '../src/app-center/command-transport.ts'
import { validCommandOperation } from '../src/app-center/schema.generated.ts'
import type { AppCenterCard, CommandCore, CommandOperation } from '../src/app-center/schema.generated'

const vectors = JSON.parse(readFileSync(new URL('./fixtures/appcenter-commands.generated.json', import.meta.url), 'utf8'))
const prepared = vectors.find((item: { name: string }) => item.name === 'absent-install').input as CommandCore
const identity = { userId: '00000000-0000-7000-8000-000000000001', organizationId: prepared.command.organization_id, stationId: prepared.command.station_id }
const card = { cardKey: 'catalog:' + prepared.command.app_id, appId: prepared.command.app_id, bindingRevision: 'sha256:' + 'a'.repeat(64),
  candidateVersion: prepared.command.app_version, actions: [{ id: 'install', kind: 'install', enabled: true }] } as AppCenterCard
const operation: CommandOperation = { request_id: 'original-trace', operation_id: '00000000-0000-7000-8000-000000000010', organization_id: identity.organizationId,
  station_id: identity.stationId, app_id: prepared.command.app_id, app_version: prepared.command.app_version, action: 'install', status: 'accepted', phase: 'checking',
  download: { downloaded_bytes: 0, total_bytes: null, percent: null }, error: null, updated_at: '2026-10-12T00:00:00Z' }
function harness() {
  const records = new Map<string, PendingCommand>(), events: string[] = []
  let epoch = 0, failSave = false, failSubmit = false, terminal = false, holdSave: (() => Promise<void>) | undefined
  const storage: CommandStorage = {
    async load(scope) { return { lease: { scope, epoch }, records: structuredClone([...records.values()]) } },
    async put(lease, record) {
      if (holdSave) await holdSave()
      if (failSave || lease.epoch !== epoch) throw new Error('APPCENTER_RECOVERY_STORAGE_UNAVAILABLE')
      records.set(record.key, structuredClone(record)); events.push('saved')
    },
    async remove(lease, key) { assert.equal(lease.epoch, epoch); events.push('removed'); records.delete(key) },
    async clear() { epoch++; records.clear() },
  }
  const sent: string[] = []
  const transport: CommandTransport = {
    async prepare(input) { events.push('prepared'); return { ...prepared, command: { ...prepared.command, request_id: input.requestId, idempotency_key: input.idempotencyKey } } },
    async submit(body) { events.push('sent'); sent.push(body); if (failSubmit) throw new Error('APPCENTER_SUBMISSION_UNKNOWN'); return structuredClone(terminal ? { ...operation, status: 'succeeded', phase: 'completed' } : operation) },
    async read() { events.push('read'); return structuredClone(terminal ? { ...operation, status: 'succeeded', phase: 'completed' } : operation) },
  }
  let next = 0
  const create = () => createCommands(storage, transport, () => `fixed-key-${++next}`)
  return { storage, records, events, sent, transport, create,
    set failSave(value: boolean) { failSave = value }, set failSubmit(value: boolean) { failSubmit = value }, set terminal(value: boolean) { terminal = value },
    set holdSave(value: (() => Promise<void>) | undefined) { holdSave = value } }
}

test('save failure forbids first submission; successful transaction precedes send', async () => {
  const h = harness(), c = h.create(); await c.setScope(identity)
  h.failSave = true
  await assert.rejects(c.execute(card, 'install', () => true), /STORAGE_UNAVAILABLE/)
  assert.equal(h.sent.length, 0)
  h.failSave = false
  await c.execute(card, 'install', () => true)
  assert.deepEqual(h.events.slice(-4), ['prepared', 'saved', 'sent', 'saved'])
  assert.equal(h.records.size, 1)
})
test('response loss and controller reconstruction replay exact persisted body', async () => {
  const h = harness(), c = h.create(); await c.setScope(identity); h.failSubmit = true
  await c.execute(card, 'install', () => true)
  assert.equal(c.state.items[0].error, 'APPCENTER_SUBMISSION_UNKNOWN')
  c.dispose(); const restored = h.create(); await restored.setScope(identity)
  assert.equal(h.sent.length, 1, 'restore must not submit automatically')
  h.failSubmit = false; await restored.resume(restored.state.items[0].record.key)
  assert.equal(h.sent.length, 2); assert.equal(h.sent[0], h.sent[1])
  assert.equal(h.events.filter(event => event === 'prepared').length, 1)
})
test('terminal submission is read back before deleting durable record', async () => {
  const h = harness(), c = h.create(); await c.setScope(identity); h.terminal = true
  await c.execute(card, 'install', () => true)
  assert.deepEqual(h.events, ['prepared', 'saved', 'sent', 'saved', 'read', 'removed'])
  assert.equal(h.records.size, 0); assert.equal(c.state.items[0].operation?.status, 'succeeded')
})
test('known operation resumes with read only; duplicate app submission is blocked', async () => {
  const h = harness(), c = h.create(); await c.setScope(identity); await c.execute(card, 'install', () => true)
  await assert.rejects(c.execute(card, 'install', () => true), /RECOVERY_PENDING/)
  await c.resume(c.state.items[0].record.key)
  assert.equal(h.sent.length, 1); assert.equal(h.events.at(-2), 'read')
})
test('scope switch while save is pending suppresses submission and late state', async () => {
  const h = harness(), c = h.create(); await c.setScope(identity)
  let release!: () => void
  h.holdSave = () => new Promise<void>(resolve => { release = resolve })
  const execution = c.execute(card, 'install', () => true)
  await Promise.resolve(); await Promise.resolve()
  await c.setScope(null); release()
  await assert.rejects(execution, /SCOPE_CHANGED/)
  assert.equal(h.sent.length, 0); assert.equal(c.state.items.length, 0)
})
test('logout epoch rejects another tab pending save and prevents resubmission', async () => {
  const h = harness(), c = h.create(); await c.setScope(identity); h.failSubmit = true
  await c.execute(card, 'install', () => true)
  const other = h.create(); await other.setScope(identity)
  await c.clearScope(commandScope(identity)); await other.resume(other.state.items[0].record.key)
  assert.equal(h.sent.length, 1); assert.equal(other.state.items[0].error, 'APPCENTER_RECOVERY_SCOPE_CHANGED')
})
test('binding changes after prepare cannot be silently reconfirmed', async () => {
  const h = harness(), c = h.create(); await c.setScope(identity)
  let checks = 0
  await assert.rejects(c.execute(card, 'install', () => ++checks === 1), /BINDING_CHANGED/)
  assert.equal(h.records.size, 0); assert.equal(h.sent.length, 0)
})
test('durable records reject changed scope, injected fields and oversized input', async () => {
  const record = { key: prepared.command.idempotency_key, body: JSON.stringify({ schemaVersion: 1, prepared }), operationId: null }
  assert.deepEqual(parsePending(record, commandScope(identity)), prepared)
  assert.throws(() => parsePending(record, commandScope({ ...identity, stationId: 'other' })))
  assert.throws(() => parsePending({ ...record, credentials: 'forbidden' } as PendingCommand, commandScope(identity)))
  assert.throws(() => parsePending({ ...record, body: ' '.repeat(17000) }, commandScope(identity)))
  await assert.rejects(createCommandStorage(undefined).load(commandScope(identity)), /STORAGE_UNAVAILABLE/)
})
test('operation schema preserves old semantics and rejects false terminal state', () => {
  assert.equal(validCommandOperation(operation), true)
  assert.equal(validCommandOperation({ ...operation, status: 'succeeded' }), false)
  assert.equal(validCommandOperation({ ...operation, status: 'failed' }), false)
  assert.equal(validCommandOperation({ ...operation, download: { downloaded_bytes: 1, total_bytes: null, percent: 5 } }), false)
  assert.equal(validCommandOperation({ ...operation, status: 'failed', error: { code: 'CONFLICT', message: 'failed', request_id: 'trace', details: { cause: 'test' } } }), true)
})
test('transport rejects foreign operation and preserves exact fixed HTTP body', async () => {
  const body = JSON.stringify({ schemaVersion: 1, prepared }), sent: string[] = []
  const fetcher = (async (_path: unknown, input: RequestInit) => {
    sent.push(String(input.body)); return new Response(JSON.stringify({ ...operation, app_id: 'other' }), { status: 202, headers: { 'Content-Type': 'application/json' } })
  }) as typeof fetch
  await assert.rejects(createCommandTransport(fetcher).submit(body, new AbortController().signal), /SUBMISSION_UNKNOWN/)
  assert.equal(sent[0], body)
})
