import { validAppCenterView } from './schema.generated.ts'
import type { AppCenterCard, AppCenterScope, AppCenterView } from './schema.generated'

export type Identity = Pick<AppCenterScope, 'userId' | 'organizationId' | 'stationId'>
export interface AppCenterTransport { read(signal: AbortSignal): Promise<unknown> }
export interface AppCenterState {
  status: 'idle' | 'loading' | 'empty' | 'ready' | 'error'
  value: AppCenterView | null
  refreshing: boolean
  stale: boolean
  error: string
}
export class AppCenterError extends Error {
  constructor(readonly status: number, readonly code: string) { super(code); this.name = 'AppCenterError' }
}
const invalid = (): never => { throw new AppCenterError(502, 'APPCENTER_INVALID_VIEW') }
const scopeMatches = (a: Identity, b: Identity) => a.userId === b.userId && a.organizationId === b.organizationId && a.stationId === b.stationId

/** Shape is generated from central schemas; the remaining checks bind one snapshot. */
export function decodeView(value: unknown, identity: Identity, now: number): AppCenterView {
  if (!validAppCenterView(value) || !scopeMatches(value.scope, identity)) return invalid()
  const generated = Date.parse(value.generatedAt), until = Date.parse(value.validUntil)
  if (generated > now + 2000 || until <= now || until > generated + 15000) return invalid()
  const keys = new Set<string>(), sourceNames = new Set<string>(), groupNames = new Set<string>()
  for (const source of value.sources) { if (sourceNames.has(source.source)) return invalid(); sourceNames.add(source.source) }
  for (const source of ['catalog', 'installations', 'observations', 'operations', 'presentation', 'entries']) if (!sourceNames.has(source)) return invalid()
  for (const group of value.groups) { if (groupNames.has(group.id)) return invalid(); groupNames.add(group.id) }
  for (const card of value.items) {
    if (keys.has(card.cardKey) || (card.origin === 'installation') !== (card.installation !== null)) return invalid()
    keys.add(card.cardKey)
    if (card.installation && (card.cardKey !== `installation:${card.installation.installationId}` || card.installedVersion !== card.installation.appVersion)) return invalid()
	if (card.origin === 'catalog' && card.cardKey !== `catalog:${card.appId}` || card.origin === 'observation' && card.cardKey !== card.service.observationKey) return invalid()
    const actionIds = new Set<string>()
    for (const action of card.actions) {
      if (actionIds.has(action.id) || action.kind === 'open' && (!card.entry || action.entryId !== card.entry.entryId)) return invalid()
      actionIds.add(action.id)
    }
    if (!actionIds.has(card.primaryActionId)) return invalid()
  }
  return structuredClone(value)
}

/** Counts, filters and launchers consume the same cards; no App names or deployment guesses. */
export function selectCards(view: AppCenterView | null, query = '', group = 'all', filter = 'all', fresh = true) {
  const all = view?.items ?? []
  const installed = (card: AppCenterCard) => card.installationQuality === 'known' && card.installation?.state === 'installed'
  const active = (card: AppCenterCard) => card.operation.quality === 'known' && card.operation.blocking
  const needle = query.trim().toLocaleLowerCase()
  const items = all.filter(card => (group === 'all' || card.display.groupId === group)
    && (filter === 'all' || filter === 'installed' && installed(card) || filter === 'active' && active(card))
    && (!needle || [card.display.name, card.display.description, card.appId].some(text => text.toLocaleLowerCase().includes(needle))))
  return { items, preinstalled: items.filter(card => card.preinstalled), applications: items.filter(card => !card.preinstalled), counts: {
    all: view && fresh ? all.length : null,
    installed: view && fresh && all.every(card => card.installationQuality === 'known') ? all.filter(installed).length : null,
    active: view && fresh && all.every(card => card.operation.quality === 'known') ? all.filter(active).length : null,
  } }
}

export function createAppCenterController(transport: AppCenterTransport, now = Date.now) {
  let state: AppCenterState = { status: 'idle', value: null, refreshing: false, stale: true, error: '' }
  let identity: Identity | null = null, generation = 0, pending: AbortController | null = null
  const listeners = new Set<(state: AppCenterState) => void>()
  function publish(next: AppCenterState) { state = next; for (const listener of listeners) listener(state) }
  function cancel() { generation++; pending?.abort(); pending = null }
  function clear(next: Identity | null = null) {
    cancel(); identity = next ? { ...next } : null
    publish({ status: 'idle', value: null, refreshing: false, stale: true, error: '' })
  }
  function expire() {
    if (state.value && !state.stale && Date.parse(state.value.validUntil) <= now()) publish({ ...state, stale: true })
  }
  function canExecute(cardKey: string, actionId: string, bindingRevision: string): boolean {
    expire()
    if (state.stale || !state.value || state.status === 'error') return false
    return state.value.items.find(card => card.cardKey === cardKey && card.bindingRevision === bindingRevision)?.actions.some(action => action.id === actionId && action.enabled) ?? false
  }
  async function refresh(): Promise<void> {
    if (!identity) return
    expire(); cancel()
    const epoch = generation, expected = { ...identity }, controller = new AbortController()
    pending = controller
    const timer = setTimeout(() => controller.abort(), 10000)
    publish({ ...state, status: state.value ? state.status : 'loading', refreshing: true, error: '' })
    try {
      const raw = await transport.read(controller.signal)
      if (epoch !== generation) return
      if (controller.signal.aborted) throw new AppCenterError(408, 'APPCENTER_TIMEOUT')
      const value = decodeView(raw, expected, now())
      publish({ status: value.items.length ? 'ready' : 'empty', value, refreshing: false, stale: false, error: '' })
    } catch (error) {
      if (epoch !== generation) return
      const denied = error instanceof AppCenterError && (error.status === 401 || error.status === 403)
      publish({ status: 'error', value: denied ? null : state.value, refreshing: false, stale: true,
        error: error instanceof AppCenterError ? error.code : 'APPCENTER_DEPENDENCY_UNAVAILABLE' })
    } finally { clearTimeout(timer); if (pending === controller) pending = null }
  }
  function dispose() { clear(); listeners.clear() }
  function suspend() { cancel(); expire(); publish({ ...state, status: state.value ? state.status : 'idle', refreshing: false }) }
  return { get state() { return state }, clear, refresh, expire, canExecute, dispose, suspend,
    subscribe(listener: (state: AppCenterState) => void) { listeners.add(listener); listener(state); return () => { listeners.delete(listener) } },
  }
}
