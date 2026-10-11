import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { createLegacyRecoverySource, legacyRecords } from '../src/app-center/legacy-commands.ts'
import { commandScope, parsePending } from '../src/app-center/command-storage.ts'
import { createCommandTransport } from '../src/app-center/command-transport.ts'
import type { CommandStorage } from '../src/app-center/command-storage'

const prepared = JSON.parse(readFileSync(new URL('./fixtures/appcenter-commands.generated.json', import.meta.url), 'utf8')).find((v: { name: string }) => v.name === 'absent-install').input
const command = prepared.command
const identity = { userId: '00000000-0000-7000-8000-000000000001', organizationId: command.organization_id, stationId: command.station_id }
const operation = { request_id: 'original-trace', operation_id: '00000000-0000-7000-8000-000000000010', organization_id: identity.organizationId,
  station_id: identity.stationId, app_id: command.app_id, app_version: command.app_version, action: command.action, status: 'succeeded', phase: 'completed',
  download: { downloaded_bytes: 0, total_bytes: null, percent: null }, error: null, updated_at: '2026-10-12T00:00:00Z' }
const envelope = () => ({ user: identity.userId, organization: identity.organizationId, station: identity.stationId, items: [[command.app_id, { command, op: operation, rejected: false, sending: true, error: '' }]] })

test('legacy import preserves original protocol and ignores cached terminal conclusion', () => {
  const records = legacyRecords(JSON.stringify(envelope()), identity)!
  assert.equal(records.length, 1); assert.equal(records[0].protocol, 'legacy')
  assert.equal(records[0].operationId, operation.operation_id)
  assert.equal(records[0].body, JSON.stringify(command))
  assert.deepEqual(parsePending(records[0], commandScope(identity)), { command })
  assert.equal('expected' in JSON.parse(records[0].body), false)
})
test('foreign legacy scope stays untouched; corrupt or mismatched original data is rejected', () => {
  const value = envelope()
  assert.equal(legacyRecords(JSON.stringify(value), { ...identity, userId: 'another-user' }), null)
  const bad = [ { ...value, extra: true }, { ...value, items: [['wrong-app', { command }]] },
    { ...value, items: [[command.app_id, { command: { ...command, endpoint: 'https://other.invalid' } }]] },
    { ...value, items: [[command.app_id, { command, op: { ...operation, app_id: 'another-app' } }]] },
    { ...value, items: [...value.items, ...value.items] } ]
  for (const input of bad) assert.throws(() => legacyRecords(JSON.stringify(input), identity))
})
test('session copy is removed only after committed import and only if unchanged', async () => {
  const original = JSON.stringify(envelope()), lease = { scope: commandScope(identity), epoch: 0 }, calls: string[] = []
  let raw: string | null = original, reject = false, replace = false
  const storage = { async importLegacy() { calls.push('import'); if (reject) throw Error('SAVE_FAILED'); if (replace) raw = original + ' ' } } as unknown as CommandStorage
  const importer = createLegacyRecoverySource(() => ({ getItem: () => raw, removeItem: () => { calls.push('remove'); raw = null } }))
  reject = true
  await assert.rejects(importer.import(storage, lease, identity, () => {}), /SAVE_FAILED/)
  assert.equal(raw, original)
  reject = false; replace = true
  await assert.rejects(importer.import(storage, lease, identity, () => {}), /LEGACY_CONFLICT/)
  assert.equal(raw, original + ' ')
  replace = false; raw = original
  await importer.import(storage, lease, identity, () => {})
  assert.equal(raw, null); assert.deepEqual(calls, ['import', 'import', 'import', 'remove'])
})
test('legacy transport replays original body only to old endpoint and preserves original trace', async () => {
  const calls: { path: string; body: string }[] = []
  const fetcher = (async (path: string, input: RequestInit) => {
    calls.push({ path, body: String(input.body) })
    return new Response(JSON.stringify(operation), { status: 202, headers: { 'Content-Type': 'application/json' } })
  }) as typeof fetch
  const body = JSON.stringify(command)
  const result = await createCommandTransport(fetcher).legacy(body, new AbortController().signal)
  assert.deepEqual(calls, [{ path: '/api/commands', body }]); assert.equal(result.request_id, 'original-trace')
})
