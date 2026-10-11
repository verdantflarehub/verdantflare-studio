import assert from 'node:assert/strict'
import test from 'node:test'
import { createOperationProgress, createOperationReads } from '../src/app-center/operation-progress.ts'
import { AppCenterError } from '../src/app-center/controller.ts'
import type { AppCenterCard, CommandOperation } from '../src/app-center/schema.generated'

const identity = { userId: '00000000-0000-7000-8000-000000000001', organizationId: '00000000-0000-7000-8000-000000000002', stationId: '00000000-0000-7000-8000-000000000003' }
const operation: CommandOperation = { request_id: 'original-trace', operation_id: '00000000-0000-7000-8000-000000000010', organization_id: identity.organizationId,
  station_id: identity.stationId, app_id: 'sample', app_version: '1.0.0', action: 'install', status: 'running', phase: 'downloading',
  download: { downloaded_bytes: 20, total_bytes: 100, percent: 20 }, error: null, updated_at: '2026-10-12T00:00:00Z' }
const card = { appId: 'sample', installedVersion: '2.0.0', operation: { quality: 'known', canRead: true, protocol: 'lifecycle', operationId: operation.operation_id, updatedAt: operation.updated_at } } as AppCenterCard
const signal = () => new AbortController().signal

test('operation detail uses historical operation version and clears on target or scope change', async () => {
  const progress = createOperationProgress(async () => structuredClone(operation))
  progress.clear(identity); progress.select(card); await progress.refresh(false, signal())
  assert.equal(progress.state.value?.app_version, '1.0.0')
  progress.select({ ...card, operation: { ...card.operation, protocol: 'transition' } })
  assert.equal(progress.state.value, null)
  await progress.refresh(false, signal()); assert.equal(progress.state.value, null)
  progress.select(card); await progress.refresh(false, signal()); progress.clear(null)
  assert.equal(progress.state.value, null)
})
test('failed read preserves stale data; authorization failure clears it', async () => {
  let error: Error | null = null
  const progress = createOperationProgress(async () => { if (error) throw error; return structuredClone(operation) })
  progress.clear(identity); progress.select(card); await progress.refresh(false, signal())
  error = new AppCenterError(503, 'SERVICE_UNAVAILABLE'); await progress.refresh(false, signal())
  assert.equal(progress.state.value?.download.percent, 20); assert.equal(progress.state.stale, true)
  error = new AppCenterError(403, 'PERMISSION_DENIED'); await progress.refresh(false, signal())
  assert.equal(progress.state.value, null); assert.equal(progress.state.error, 'PERMISSION_DENIED')
})
test('foreign operation identity and late responses cannot replace current scope', async () => {
  let release!: (value: CommandOperation) => void
  const progress = createOperationProgress(() => new Promise(resolve => { release = resolve }))
  progress.clear(identity); progress.select(card)
  const pending = progress.refresh(false, signal()); release({ ...operation, app_id: 'foreign' }); await pending
  assert.equal(progress.state.value, null); assert.equal(progress.state.error, 'APPCENTER_INVALID_OPERATION')
  const late = progress.refresh(false, signal()); progress.clear(null); release(operation); await late
  assert.equal(progress.state.value, null); assert.equal(progress.state.busy, false)
})
test('hidden reads abort; terminal result stops periodic reads but manual refresh works', async () => {
  let count = 0, runningSignal: AbortSignal | undefined
  const progress = createOperationProgress(async (_id, signal) => { count++; runningSignal = signal; return { ...operation, status: 'succeeded', phase: 'completed' } })
  progress.clear(identity); progress.select(card)
  await progress.refresh(false, signal()); await progress.refresh(false, signal()); assert.equal(count, 1)
  await progress.refresh(true, signal()); assert.equal(count, 2)
  const pending = progress.refresh(true, signal()); progress.suspend(); await pending
  assert.equal(runningSignal?.aborted, true); assert.equal(progress.state.stale, true)
})
test('shared refresh cycle reads each operation once even when second consumer starts later', async () => {
  let count = 0
  const reads = createOperationReads(async () => { count++; return structuredClone(operation) })
  await reads.cycle(async () => {
    await reads.read(operation.operation_id, signal())
    await reads.read(operation.operation_id, signal())
  })
  assert.equal(count, 1)
  await reads.read(operation.operation_id, signal()); assert.equal(count, 2)
})
