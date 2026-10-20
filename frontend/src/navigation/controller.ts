export interface NavigationScope { userId: string; organizationId: string; stationId: string; authorizationEpoch: string }
export interface NavigationChild { entryId: string; title: string; route: string; order: number }
export interface NavigationEntry {
  children?: NavigationChild[]
  installationId: string; appId: string; appVersion: string; entryId: string; title: string; iconRef: string
  route: string; group: string; order: number; pinned: boolean; pinSource: 'none' | 'default' | 'user'
  availability: 'ready' | 'offline' | 'unknown'
}
export interface NavigationEnvelope {
  mode: 'legacy' | 'preinstalled'
  projection: { scope: NavigationScope; revision: string; entries: NavigationEntry[] }
  preferenceRevision: string
}
export interface NavigationPreference { appId: string; entryId: string; pinned: boolean; order: number; preferenceRevision: string }
export interface NavigationTransport { read(signal: AbortSignal): Promise<unknown>; write(input: NavigationPreference, signal: AbortSignal): Promise<unknown> }
export interface NavigationState { status: 'idle' | 'loading' | 'ready' | 'error'; value: NavigationEnvelope | null; saving: boolean; error: string }
export type NavigationIdentity = Pick<NavigationScope, 'userId' | 'organizationId' | 'stationId'>

const id = /^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$/
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const installation = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const version = /^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/
const digest = /^sha256:[a-f0-9]{64}$/
const record = (v: unknown): v is Record<string, unknown> => !!v && typeof v === 'object' && !Array.isArray(v)
const matches = (v: unknown, pattern: RegExp): v is string => typeof v === 'string' && pattern.test(v)
const invalid = (): never => { throw new Error('INVALID_NAVIGATION_RESPONSE') }
const validOrder = (v: unknown): v is number => typeof v === 'number' && Number.isSafeInteger(v) && v >= 0 && v <= 1000000
const validRoute = (v: unknown): v is string => typeof v === 'string' && v.length <= 2048 && (v === '/' || /^\/(?:[a-zA-Z0-9_-][a-zA-Z0-9._-]*)(?:\/[a-zA-Z0-9_-][a-zA-Z0-9._-]*)*$/.test(v))
const validTitle = (v: unknown): v is string => typeof v === 'string' && v.trim() === v && Array.from(v).length > 0 && Array.from(v).length <= 48 && !/[<>\u0000-\u001f\u007f-\u009f]/.test(v)
export function validChildRoute(value: unknown): value is string {
  if (typeof value !== 'string' || value.length > 2048) return false
  const [path, query, extra] = value.split('?')
  if (!validRoute(path) || extra !== undefined) return false
  if (query === undefined) return true
  const seen = new Set<string>()
  return query.split('&').every(part => {
    if (!/^[a-zA-Z0-9][a-zA-Z0-9._-]*=[a-zA-Z0-9._-]+$/.test(part)) return false
    const key = part.split('=')[0]
    if (seen.has(key)) return false
    seen.add(key); return true
  })
}
function normalizeChildUrl(u: URL): URL {
  if (u.pathname === '/projects') {
    const cat = u.searchParams.get('category')
    if (cat && ['image', 'character', 'music', 'video', 'ai_short_drama'].includes(cat)) {
      u.pathname = '/' + cat
      u.searchParams.delete('category')
    }
  } else if (u.pathname === '/') {
    const view = u.searchParams.get('view')
    if (view === 'storage' || view === 'tasks') {
      u.pathname = '/' + view
      u.searchParams.delete('view')
    } else if (view === 'monitor') {
      u.searchParams.delete('view')
    }
  } else if (u.pathname === '/monitor') {
    u.pathname = '/'
  }
  return u
}
export function sameNavigationPath(left: string, right: string): boolean {
  const normalize = (value: string) => {
    try {
      const u = normalizeChildUrl(new URL(value, 'https://app.invalid'))
      u.searchParams.sort()
      return u.pathname + u.search
    } catch {
      const [path, query] = value.split('?')
      return path + (query ? '?' + query.split('&').sort().join('&') : '')
    }
  }
  return normalize(left) === normalize(right)
}
/** Selection only: extra business parameters do not erase the declared category. */
export function selectedNavigationChild(children: NavigationChild[] | undefined, path: string): NavigationChild | undefined {
  if (!children?.length || !path.startsWith('/') || path.startsWith('//') || /[#\\\r\n]/.test(path)) return
  const rawCurrent = new URL(path, 'https://app.invalid')
  if (rawCurrent.origin !== 'https://app.invalid') return
  let duplicate = false
  rawCurrent.searchParams.forEach((_value, key) => { if (rawCurrent.searchParams.getAll(key).length !== 1) duplicate = true })
  if (duplicate) return
  const current = normalizeChildUrl(rawCurrent)
  const candidates = children.flatMap(child => {
    const declared = normalizeChildUrl(new URL(child.route, 'https://app.invalid'))
    if (declared.pathname !== current.pathname) return []
    const keys: string[] = []
    declared.searchParams.forEach((_value, key) => keys.push(key))
    if (!keys.every(key => declared.searchParams.get(key) === current.searchParams.get(key))) return []
    return [{ child, score: keys.length }]
  }).sort((a, b) => b.score - a.score)
  return candidates[0] && (!candidates[1] || candidates[0].score > candidates[1].score) ? candidates[0].child : undefined
}
const denied = (error: unknown) => error instanceof Error && ['NAVIGATION_DENIED', 'INVALID_NAVIGATION_RESPONSE'].includes(error.message)

export function decodeNavigation(value: unknown, expected: NavigationIdentity): NavigationEnvelope {
  if (!record(value) || !['legacy', 'preinstalled'].includes(String(value.mode)) || !record(value.projection) || !matches(value.preferenceRevision, /^(0|[1-9][0-9]*)$/)) return invalid()
  const p = value.projection, scope = p.scope
  if (!record(scope) || !matches(scope.userId, uuid) || !matches(scope.organizationId, uuid) || !matches(scope.stationId, uuid) || !matches(scope.authorizationEpoch, id)
    || scope.userId !== expected.userId || scope.organizationId !== expected.organizationId || scope.stationId !== expected.stationId
    || !matches(p.revision, digest) || !Array.isArray(p.entries) || p.entries.length > 128 || value.mode === 'legacy' && p.entries.length !== 0) return invalid()
  const seen = new Set<string>(), installs = new Set<string>()
  for (const e of p.entries) {
    if (!record(e) || !matches(e.installationId, installation) || !matches(e.appId, id) || !matches(e.appVersion, version) || !matches(e.entryId, id)
      || typeof e.title !== 'string' || e.title.trim() !== e.title || Array.from(e.title).length < 1 || Array.from(e.title).length > 48 || /[<>\u0000-\u001f\u007f]/.test(e.title)
      || !matches(e.iconRef, id) || !(validRoute(e.route) || e.route === '' && Array.isArray(e.children) && e.children.length > 0) || !matches(e.group, id) || !validOrder(e.order) || typeof e.pinned !== 'boolean'
      || !['none', 'default', 'user'].includes(String(e.pinSource)) || !['ready', 'offline', 'unknown'].includes(String(e.availability))) return invalid()
    if (seen.has(e.appId) || installs.has(e.installationId)) return invalid()
    seen.add(e.appId); installs.add(e.installationId)
    if (e.children !== undefined) {
      if (!Array.isArray(e.children) || e.children.length > 32) return invalid()
      const children = new Set([e.entryId])
      for (const child of e.children) {
        if (!record(child) || !matches(child.entryId, id) || children.has(child.entryId) || !validTitle(child.title) || !validChildRoute(child.route) || !validOrder(child.order)
          || Object.keys(child).some(key => !['entryId', 'title', 'route', 'order'].includes(key))) return invalid()
        children.add(child.entryId)
      }
    }
  }
  return structuredClone(value) as unknown as NavigationEnvelope
}

/** Scope and request generations prevent a prior user or slower poll restoring menus. */
export function createNavigationController(transport: NavigationTransport) {
  let state: NavigationState = { status: 'idle', value: null, saving: false, error: '' }
  let identity: NavigationIdentity | null = null, generation = 0, pending: AbortController | null = null
  const listeners = new Set<(state: NavigationState) => void>()
  function publish(next: NavigationState) { state = next; for (const listener of listeners) listener(state) }
  function cancel() { generation++; pending?.abort(); pending = null }
  function clear(next: NavigationIdentity | null = null) {
    cancel(); identity = next ? { ...next } : null
    publish({ status: 'idle', value: null, saving: false, error: '' })
  }
  async function refresh(): Promise<void> {
    if (!identity || state.saving) return
    cancel()
    const revision = generation, expected = { ...identity }, controller = new AbortController()
    pending = controller
    const timer = setTimeout(() => controller.abort(), 10000)
    const nextStatus = state.value ? 'ready' : 'loading'
    if (state.status !== nextStatus || state.error !== '') {
      publish({ ...state, status: nextStatus, error: '' })
    }
    try {
      const value = decodeNavigation(await transport.read(controller.signal), expected)
      if (revision !== generation) return
      if (controller.signal.aborted) throw new Error('NAVIGATION_TIMEOUT')
      if (state.status === 'ready' && state.value && !state.saving && !state.error
        && state.value.mode === value.mode
        && state.value.preferenceRevision === value.preferenceRevision
        && state.value.projection.revision === value.projection.revision) {
        return
      }
      publish({ status: 'ready', value, saving: false, error: '' })
    } catch (error) {
      if (revision === generation) publish({ status: 'error', value: denied(error) ? null : state.value, saving: false, error: '应用菜单暂时无法加载，请重试。' })
    } finally { clearTimeout(timer); if (pending === controller) pending = null }
  }
  async function setPreference(entry: NavigationEntry, pinned: boolean, order = entry.order): Promise<void> {
    if (!identity || state.saving || state.status !== 'ready' || state.value?.mode !== 'preinstalled' || !validOrder(order)) return
    const current = state.value.projection.entries.find(e => e.appId === entry.appId && e.entryId === entry.entryId && e.installationId === entry.installationId)
    if (!current) return
    cancel()
    const revision = generation, expected = { ...identity }, controller = new AbortController()
    pending = controller
    const timer = setTimeout(() => controller.abort(), 10000)
    const input = { appId: entry.appId, entryId: entry.entryId, pinned, order, preferenceRevision: state.value.preferenceRevision }
    publish({ ...state, saving: true, error: '' })
    try {
      const value = decodeNavigation(await transport.write(input, controller.signal), expected)
      if (revision !== generation) return
      if (controller.signal.aborted) throw new Error('NAVIGATION_TIMEOUT')
      publish({ status: 'ready', value, saving: false, error: '' })
    } catch (error) {
      if (revision !== generation) return
      const conflict = error instanceof Error && error.message === 'NAVIGATION_PREFERENCE_CONFLICT'
      publish({ status: 'error', value: denied(error) ? null : state.value, saving: false, error: '' })
      await refresh()
      if (revision + 1 !== generation) return
      publish({ ...state, error: conflict ? '菜单偏好已在其他窗口更改，已刷新，请重新操作。' : '菜单偏好未能确认保存，请检查当前状态后重试。' })
    } finally { clearTimeout(timer); if (pending === controller) pending = null }
  }
  return { get state() { return state }, clear, refresh, setPreference,
    subscribe(listener: (state: NavigationState) => void) { listeners.add(listener); listener(state); return () => { listeners.delete(listener) } },
  }
}
