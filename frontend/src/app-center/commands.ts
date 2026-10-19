import { AppCenterError } from './controller.ts'
import type { Identity } from './controller'
import { commandScope, parsePending } from './command-storage.ts'
import type { CommandLease, CommandStorage, PendingCommand } from './command-storage'
import { operationMatches } from './command-transport.ts'
import type { CommandTransport } from './command-transport'
import type { AppCenterCard, CommandOperation, CommandPrepare } from './schema.generated'
import type { LegacyRecoverySource } from './legacy-commands'

export interface CommandItem { record: PendingCommand; operation: CommandOperation | null; busy: boolean; error: string }
export interface CommandState { items: CommandItem[]; loading: boolean; error: string; preparing: boolean }
const errorCode = (error: unknown) => error instanceof Error && /^[A-Z][A-Z0-9_]{0,127}$/.test(error.message) ? error.message : 'APPCENTER_SUBMISSION_UNKNOWN'
const changed = () => new Error('APPCENTER_RECOVERY_SCOPE_CHANGED')
export function createCommands(storage: CommandStorage, transport: CommandTransport, uuid = () => crypto.randomUUID(), legacy?: LegacyRecoverySource) {
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
      let saved = await storage.load(scope); current(generation)
      if (legacy && identity) {
        await legacy.import(storage, saved.lease, identity, () => current(generation)); current(generation)
        saved = await storage.load(scope); current(generation)
      }
      lease = saved.lease
      publish({ items: saved.records.map(record => ({ record, operation: null, busy: false, error: '' })), loading: false })
    } catch (error) { if (epoch === generation) publish({ loading: false, error: errorCode(error) }) }
  }
  function itemUpdate(key: string, patch: Partial<CommandItem>) {
    publish({ items: state.items.map(item => item.record.key === key ? { ...item, ...patch } : item) })
  }
  async function resume(key: string, readOnly = false, signal?: AbortSignal) {
    const item = state.items.find(item => item.record.key === key)
    if (!item || item.busy || !lease || signal?.aborted || readOnly && (!item.record.operationId || item.operation && ['succeeded', 'failed'].includes(item.operation.status) && !item.error)) return
    const generation = epoch, active = lease, abort = new AbortController()
    const cancel = () => abort.abort()
    signal?.addEventListener('abort', cancel, { once: true })
    const timer = setTimeout(() => abort.abort(), 35000)
    pending.add(abort); itemUpdate(key, { busy: true, error: '' })
    try {
      // Recheck the persistent epoch before sending; logout in another tab
      // invalidates this lease. Never recreate a missing record here.
      const saved = await storage.load(active.scope); current(generation)
      const original = saved.records.find(record => record.key === key)
      if (saved.lease.epoch !== active.epoch || !original || original.body !== item.record.body) throw changed()
      abort.signal.throwIfAborted()
      if (readOnly && !original.operationId) return
      const prepared = parsePending(original, active.scope)
      const operation = original.operationId
        ? await transport.read(original.operationId, abort.signal)
        : original.protocol === 'legacy' ? await transport.legacy(original.body, abort.signal) : await transport.submit(original.body, abort.signal)
      current(generation)
      abort.signal.throwIfAborted()
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
        itemUpdate(key, { operation: confirmed })
        if (confirmed.status === 'succeeded' || confirmed.status === 'failed') {
          await storage.remove(active, key); current(generation)
          itemUpdate(key, { operation: confirmed })
        }
      }
    } catch (error) {
      if (epoch === generation && !signal?.aborted) {
        if (!item.record.operationId && error instanceof AppCenterError && [400, 409].includes(error.status)) {
          await storage.remove(active, key).catch(() => undefined)
          if (epoch === generation) publish({ items: state.items.filter(entry => entry.record.key !== key) })
        } else {
          itemUpdate(key, { error: errorCode(error),
            ...(error instanceof AppCenterError && [401, 403].includes(error.status) ? { operation: null } : {}) })
        }
      }
    }
    finally { clearTimeout(timer); signal?.removeEventListener('abort', cancel); pending.delete(abort); if (epoch === generation) itemUpdate(key, { busy: false }) }
  }
  async function reconcile(cards: AppCenterCard[]) {
    if (!lease || state.loading || !cards.length) return
    const generation = epoch, active = lease
    const byApp = new Map(cards.map(c => [c.appId, c]))
    const staleKeys: string[] = []
    for (const item of state.items) {
      if (item.busy || item.record.operationId) continue
      try {
        const pending = parsePending(item.record, active.scope)
        const card = byApp.get(pending.command.app_id)
        if (!card || card.operation.blocking) continue
        if ('expected' in pending) {
          const curId = card.installation?.installationId ?? null
          const curRev = card.installation?.revision ?? null
          if (pending.expected.installationId !== curId || pending.expected.installationRevision !== curRev
            || (['install', 'upgrade'].includes(pending.command.action ?? '') && card.installedVersion === pending.command.app_version)) {
            staleKeys.push(item.record.key)
          }
        } else if (pending.command.app_version !== card.installedVersion && pending.command.app_version !== card.candidateVersion) {
          staleKeys.push(item.record.key)
        }
      } catch { staleKeys.push(item.record.key) }
    }
    if (!staleKeys.length) return
    for (const key of staleKeys) {
      if (epoch !== generation) return
      await storage.remove(active, key).catch(() => undefined)
    }
    if (epoch === generation) publish({ items: state.items.filter(item => !staleKeys.includes(item.record.key)) })
  }
  async function dismiss(key: string) {
    if (!lease) return
    const generation = epoch, active = lease
    await storage.remove(active, key)
    if (epoch === generation) publish({ items: state.items.filter(item => item.record.key !== key) })
  }
  async function execute(card: AppCenterCard, actionId: string, canExecute: () => boolean) {
    if (!lease || state.loading || state.error || state.preparing || !canExecute()) throw new Error('APPCENTER_RECOVERY_STORAGE_UNAVAILABLE')
    const generation = epoch, active = lease
    for (const existing of state.items) {
      if (existing.busy || existing.record.operationId) continue
      try {
        const pending = parsePending(existing.record, scope)
        if (pending.command.app_id === card.appId && (existing.error || ('expected' in pending && (pending.expected.installationId !== (card.installation?.installationId ?? null) || pending.expected.installationRevision !== (card.installation?.revision ?? null))))) {
          await storage.remove(active, existing.record.key); current(generation)
          publish({ items: state.items.filter(item => item.record.key !== existing.record.key) })
        }
      } catch { /* ignore */ }
    }
    if (state.items.some(item => parsePending(item.record, scope).command.app_id === card.appId && (!item.operation || !['succeeded', 'failed'].includes(item.operation.status) || item.error))) throw new Error('APPCENTER_RECOVERY_PENDING')
    const action = card.actions.find(item => item.id === actionId)
    if (!action || !['install', 'upgrade', 'start', 'stop', 'restart', 'delete'].includes(action.kind)) throw new Error('ACTION_UNSUPPORTED')
    const abort = new AbortController()
    const timer = setTimeout(() => abort.abort(), 15000)
    pending.add(abort); publish({ preparing: true })
    try {
      const input: CommandPrepare = { schemaVersion: 1, requestId: uuid(), idempotencyKey: uuid(), cardKey: card.cardKey,
        actionId: action.kind as CommandPrepare['actionId'], expectedBindingRevision: card.bindingRevision }
      const prepared = await transport.prepare(input, abort.signal); current(generation)
      if (!canExecute() || prepared.command.app_id !== card.appId || prepared.command.app_version !== (['install', 'upgrade'].includes(action.kind) ? card.candidateVersion : card.installedVersion)) throw new Error('APPCENTER_BINDING_CHANGED')
      const record: PendingCommand = { key: input.idempotencyKey, body: JSON.stringify({ schemaVersion: 1, prepared }), operationId: null }
      parsePending(record, active.scope)
      await storage.put(active, record); current(generation)
      publish({ items: [...state.items.filter(item => !item.operation || !['succeeded', 'failed'].includes(item.operation.status) || item.error), { record, operation: null, busy: false, error: '' }] })
      await resume(record.key)
    } finally { clearTimeout(timer); pending.delete(abort); if (epoch === generation) publish({ preparing: false }) }
  }
  async function clearScope(target: string) {
    if (target === scope) { stop(); publish({ items: [], loading: false, error: '', preparing: false }) }
    try { await storage.clear(target) }
    catch (error) { if (target === scope) publish({ error: errorCode(error) }); throw error }
  }
  return {
    get state() { return state }, setScope, execute, resume, reconcile, dismiss, clearScope,
    async refreshKnown(signal: AbortSignal) {
      const generation = epoch
      const keys = state.items.filter(item => item.record.operationId && (!item.operation || !['succeeded', 'failed'].includes(item.operation.status) || item.error)).map(item => item.record.key)
      let index = 0
      await Promise.all(Array.from({ length: Math.min(4, keys.length) }, async () => {
        while (index < keys.length && !signal.aborted && epoch === generation) await resume(keys[index++], true, signal)
      }))
    },
    subscribe(listener: (value: CommandState) => void) { listeners.add(listener); return () => listeners.delete(listener) },
    dispose() { stop(); listeners.clear() },
  }
}
