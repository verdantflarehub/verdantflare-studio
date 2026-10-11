import { AppCenterError } from './controller.ts'
import type { Identity } from './controller'
import type { AppCenterCard, CommandOperation } from './schema.generated'

type Read = (id: string, signal: AbortSignal) => Promise<CommandOperation>
/** One snapshot of each operation per visible refresh cycle. */
export function createOperationReads(read: Read) {
  let batch: Map<string, Promise<CommandOperation>> | null = null
  return {
    async cycle(action: () => Promise<void>) {
      const current = new Map<string, Promise<CommandOperation>>()
      batch = current
      try { await action() } finally { if (batch === current) batch = null }
    },
    read(id: string, signal: AbortSignal) {
      signal.throwIfAborted()
      if (!batch) return read(id, signal)
      let pending = batch.get(id)
      if (!pending) { pending = read(id, signal); batch.set(id, pending) }
      return pending
    },
    invalidate() { batch = null },
  }
}
export interface OperationProgress { value: CommandOperation | null; busy: boolean; stale: boolean; error: string }
export function createOperationProgress(read: Read) {
  let state: OperationProgress = { value: null, busy: false, stale: false, error: '' }
  let identity: Identity | null = null, target: { id: string; appId: string; updated: string | null } | null = null, epoch = 0
  let pending: AbortController | null = null
  const listeners = new Set<(state: OperationProgress) => void>()
  const publish = (patch: Partial<OperationProgress>) => { state = { ...state, ...patch }; for (const listener of listeners) listener(state) }
  function suspend() { epoch++; pending?.abort(); pending = null; publish({ busy: false, stale: Boolean(state.value) }) }
  function clear(next: Identity | null) { suspend(); identity = next; target = null; publish({ value: null, stale: false, error: '' }) }
  function select(card: AppCenterCard | undefined) {
    const next = card?.operation.canRead && card.operation.quality === 'known' && card.operation.protocol === 'lifecycle' && card.operation.operationId
      ? { id: card.operation.operationId, appId: card.appId, updated: card.operation.updatedAt } : null
    if (next?.id !== target?.id || next?.appId !== target?.appId) { suspend(); publish({ value: null, stale: false, error: '' }) }
    else if (next?.updated !== target?.updated) publish({ stale: Boolean(state.value) })
    target = next
  }
  async function refresh(force: boolean, signal: AbortSignal) {
    if (!identity || !target || state.busy || signal.aborted || !force && state.value && ['succeeded', 'failed'].includes(state.value.status) && !state.stale) return
    const generation = epoch, who = { ...identity }, expected = { ...target }, abort = new AbortController()
    const cancel = () => abort.abort(), timer = setTimeout(cancel, 15000)
    signal.addEventListener('abort', cancel, { once: true }); pending = abort
    publish({ busy: true, error: '' })
    try {
      const operation = await read(expected.id, abort.signal)
      if (generation !== epoch) return
      abort.signal.throwIfAborted()
      if (operation.operation_id !== expected.id || operation.app_id !== expected.appId || operation.organization_id !== who.organizationId || operation.station_id !== who.stationId) throw new AppCenterError(502, 'APPCENTER_INVALID_OPERATION')
      publish({ value: operation, stale: false })
    } catch (error) {
      if (generation !== epoch || signal.aborted) return
      const denied = error instanceof AppCenterError && [401, 403].includes(error.status)
      publish({ value: denied ? null : state.value, stale: !denied && Boolean(state.value), error: error instanceof AppCenterError ? error.code : 'APPCENTER_OPERATION_UNAVAILABLE' })
    } finally {
      clearTimeout(timer); signal.removeEventListener('abort', cancel)
      if (generation === epoch) { pending = null; publish({ busy: false }) }
    }
  }
  return { get state() { return state }, clear, select, refresh, suspend,
    subscribe(listener: (value: OperationProgress) => void) { listeners.add(listener); return () => listeners.delete(listener) },
    dispose() { clear(null); listeners.clear() },
  }
}
