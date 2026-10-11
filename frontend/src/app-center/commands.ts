import { AppCenterError } from './controller.ts'
import type { Identity } from './controller'
import { commandScope, parsePending } from './command-storage.ts'
import type { CommandLease, CommandStorage, PendingCommand } from './command-storage'
import { operationMatches } from './command-transport.ts'
import type { CommandTransport } from './command-transport'
import type { AppCenterCard, CommandOperation, CommandPrepare } from './schema.generated'

export interface CommandItem { record: PendingCommand; operation: CommandOperation | null; busy: boolean; error: string }
export interface CommandState { items: CommandItem[]; loading: boolean; error: string; preparing: boolean }
const errorCode = (error: unknown) => error instanceof Error ? error.message : 'APPCENTER_SUBMISSION_UNKNOWN'
const changed = () => new Error('APPCENTER_RECOVERY_SCOPE_CHANGED')
export function createCommands(storage: CommandStorage, transport: CommandTransport, uuid = () => crypto.randomUUID()) {
  let state: CommandState = { items: [], loading: false, error: '', preparing: false }
  let scope = '', epoch = 0, lease: CommandLease | null = null
  const listeners = new Set<(state: CommandState) => void>(), pending = new Set<AbortController>()
  const publish = (patch: Partial<CommandState>) => { state = { ...state, ...patch }; for (const listener of listeners) listener(state) }
  const current = (generation: number) => { if (epoch !== generation) throw changed() }
  function stop() { epoch++; for (const request of pending) request.abort(); pending.clear(); lease = null }
  async function setScope(identity: Identity | null) {
    stop(); scope = identity ? commandScope(identity) : ''; const generation = epoch
    publish({ items: [], loading: Boolean(scope), error: '', preparing: false })
    if (!scope) return
    try {
      const saved = await storage.load(scope); current(generation)
      lease = saved.lease
      publish({ items: saved.records.map(record => ({ record, operation: null, busy: false, error: '' })), loading: false })
    } catch (error) { if (epoch === generation) publish({ loading: false, error: errorCode(error) }) }
  }
  function itemUpdate(key: string, patch: Partial<CommandItem>) {
    publish({ items: state.items.map(item => item.record.key === key ? { ...item, ...patch } : item) })
  }
  async function resume(key: string) {
    const item = state.items.find(item => item.record.key === key)
    if (!item || item.busy || !lease) return
    const generation = epoch, active = lease, abort = new AbortController()
    const timer = setTimeout(() => abort.abort(), 35000)
    pending.add(abort); itemUpdate(key, { busy: true, error: '' })
    try {
      // Recheck the persistent epoch before sending; logout in another tab
      // invalidates this lease. Never recreate a missing record here.
      const saved = await storage.load(active.scope); current(generation)
      const original = saved.records.find(record => record.key === key)
      if (saved.lease.epoch !== active.epoch || !original || original.body !== item.record.body) throw changed()
      const prepared = parsePending(original, active.scope)
      const operation = original.operationId
        ? await transport.read(original.operationId, abort.signal)
        : await transport.submit(original.body, abort.signal)
      current(generation)
      if (!operationMatches(operation, prepared)) throw new AppCenterError(502, 'APPCENTER_SUBMISSION_UNKNOWN')
      const record = { ...original, operationId: operation.operation_id }
      await storage.put(active, record); current(generation)
      itemUpdate(key, { record, operation })
      // A terminal submit response alone is not the cleanup proof. Confirm it
      // by the existing authenticated operation reader before removing.
      if (operation.status === 'succeeded' || operation.status === 'failed') {
        const confirmed = original.operationId ? operation : await transport.read(operation.operation_id, abort.signal)
        current(generation)
        if (!operationMatches(confirmed, prepared) || confirmed.operation_id !== operation.operation_id) throw new AppCenterError(502, 'APPCENTER_SUBMISSION_UNKNOWN')
        if (confirmed.status === 'succeeded' || confirmed.status === 'failed') {
          await storage.remove(active, key); current(generation)
          itemUpdate(key, { operation: confirmed })
        }
      }
    } catch (error) { if (epoch === generation) itemUpdate(key, { error: errorCode(error) }) }
    finally { clearTimeout(timer); pending.delete(abort); if (epoch === generation) itemUpdate(key, { busy: false }) }
  }
  async function execute(card: AppCenterCard, actionId: string, canExecute: () => boolean) {
    if (!lease || state.loading || state.error || state.preparing || !canExecute()) throw new Error('APPCENTER_RECOVERY_STORAGE_UNAVAILABLE')
    if (state.items.some(item => parsePending(item.record, scope).command.app_id === card.appId && (!item.operation || !['succeeded', 'failed'].includes(item.operation.status) || item.error))) throw new Error('APPCENTER_RECOVERY_PENDING')
    const action = card.actions.find(item => item.id === actionId)
    if (!action || !['install', 'start', 'stop', 'restart', 'delete'].includes(action.kind)) throw new Error('ACTION_UNSUPPORTED')
    const generation = epoch, active = lease, abort = new AbortController()
    const timer = setTimeout(() => abort.abort(), 15000)
    pending.add(abort); publish({ preparing: true })
    try {
      const input: CommandPrepare = { schemaVersion: 1, requestId: uuid(), idempotencyKey: uuid(), cardKey: card.cardKey,
        actionId: action.kind as CommandPrepare['actionId'], expectedBindingRevision: card.bindingRevision }
      const prepared = await transport.prepare(input, abort.signal); current(generation)
      if (!canExecute() || prepared.command.app_id !== card.appId || prepared.command.app_version !== (action.kind === 'install' ? card.candidateVersion : card.installedVersion)) throw new Error('APPCENTER_BINDING_CHANGED')
      const record: PendingCommand = { key: input.idempotencyKey, body: JSON.stringify({ schemaVersion: 1, prepared }), operationId: null }
      parsePending(record, active.scope)
      await storage.put(active, record); current(generation)
      publish({ items: [...state.items, { record, operation: null, busy: false, error: '' }] })
      await resume(record.key)
    } finally { clearTimeout(timer); pending.delete(abort); if (epoch === generation) publish({ preparing: false }) }
  }
  async function clearScope(target: string) {
    if (target === scope) { stop(); publish({ items: [], loading: false, error: '', preparing: false }) }
    try { await storage.clear(target) }
    catch (error) { if (target === scope) publish({ error: errorCode(error) }); throw error }
  }
  return {
    get state() { return state }, setScope, execute, resume, clearScope,
    subscribe(listener: (value: CommandState) => void) { listeners.add(listener); return () => listeners.delete(listener) },
    dispose() { stop(); listeners.clear() },
  }
}
