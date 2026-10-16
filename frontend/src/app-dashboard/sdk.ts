import type {
  AppContext, AppRequest, AppRoute, AppSDK, DashboardLaunch, Json, Lifecycle, SDKAdapters, Theme,
} from './types'
import { validateMediaPreview } from '../platform/media-preview.ts'

export class AppSDKError extends Error {
  constructor(readonly code: string) { super(code); this.name = 'AppSDKError' }
}

const fail = (code: string): never => { throw new AppSDKError(code) }
const methods = new Set(['GET', 'POST', 'PUT', 'PATCH', 'DELETE'])

// Restrict caller input before URL parsing can normalize traversal or separators.
// App policies and backend authorization must still validate the resolved target.
export function assertAppPath(path: string): void {
  if (typeof path !== 'string' || !path.startsWith('/') || path.startsWith('//') || path.length > 2048
    || /[\\\s#\u0000-\u001f\u007f]/.test(path)) fail('INVALID_PATH')
  const pathname = path.split('?')[0]
  if (/%(?:2f|5c|25)/i.test(pathname)) fail('INVALID_PATH')
  let decoded: string
  try { decoded = decodeURIComponent(pathname) } catch { return fail('INVALID_PATH') }
  if (/[\\\s\u0000-\u001f\u007f]/.test(decoded)
    || decoded.split('/').some(part => part === '.' || part === '..')) fail('INVALID_PATH')
}

function id(value: string): string {
  if (typeof value !== 'string' || !/^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$/.test(value)) fail('INVALID_IDENTIFIER')
  return value
}

function jsonCopy(value: Json, maxBytes = 1024 * 1024): Json {
  // Serialize once to detach caller-owned objects before crossing an async boundary.
  let encoded: string | undefined
  try {
    encoded = JSON.stringify(value, (_, item: unknown) => {
      if (typeof item === 'number' && !Number.isFinite(item)) fail('INVALID_BODY')
      if (['undefined', 'function', 'symbol', 'bigint'].includes(typeof item)) fail('INVALID_BODY')
      return item
    })
  } catch { return fail('INVALID_BODY') }
  if (encoded === undefined || new TextEncoder().encode(encoded).length > maxBytes) fail('INVALID_BODY')
  return JSON.parse(encoded!) as Json
}

function contextCopy(context: AppContext): Readonly<AppContext> {
  return Object.freeze({
    userId: id(context.userId), organizationId: id(context.organizationId), stationId: id(context.stationId),
    projectId: context.projectId === null ? null : id(context.projectId), appId: id(context.appId),
    installationId: id(context.installationId), contextRevision: id(context.contextRevision),
  })
}

/** Create one revocable SDK per authorized Dashboard load, never per instance. */
export function createAppSDK(launch: DashboardLaunch, initial: AppContext, adapters: SDKAdapters,
  now: () => number = adapters.now ?? Date.now) {
  let context = contextCopy(initial)
  const sessionId = id(launch.dashboardSessionId)
  const version = launch.sdkVersion
  const expiry = Date.parse(launch.expiresAt)
  if (!/^1\.\d+\.\d+$/.test(version)) fail('SDK_VERSION_UNSUPPORTED')
  if (!Number.isFinite(expiry) || !Number.isFinite(now()) || expiry <= now()) fail('SESSION_EXPIRED')
  if (context.appId !== launch.appId || context.installationId !== launch.installationId
    || context.contextRevision !== launch.contextRevision) fail('CONTEXT_MISMATCH')
  const grants = new Set(launch.grantedCapabilities)
  let state: Lifecycle = 'active'
  const contextListeners = new Set<(context: Readonly<AppContext>) => void>()
  const lifecycleListeners = new Set<(event: Lifecycle) => void>()
  const themeListeners = new Set<(theme: Theme) => void>()
  const routeListeners = new Set<(route: Readonly<AppRoute>) => void>()
  let theme: Theme = 'light'
  let route: Readonly<AppRoute> = Object.freeze({ path: '/', revision: 0 })
  const inflight = new Map<AbortController, boolean>()
  let expiryTimer: ReturnType<typeof setTimeout>
  function scheduleExpiry() {
    expiryTimer = setTimeout(() => {
      if (now() >= expiry) dispose()
      else scheduleExpiry()
    }, Math.min(Math.max(expiry - now(), 1), 2_147_483_647))
  }
  scheduleExpiry()

  function emit<T>(listeners: Set<(event: T) => void>, value: T) {
    for (const listener of [...listeners]) {
      try { listener(value) } catch (error) {
        try { adapters.reportListenerError(error) } catch { /* cleanup must continue */ }
      }
    }
  }
  function dispose() {
    if (state === 'dispose') return
    state = 'dispose'
    clearTimeout(expiryTimer)
    for (const controller of inflight.keys()) controller.abort()
    inflight.clear()
    adapters.closeMedia?.()
    adapters.releaseFiles?.()
    emit(lifecycleListeners, 'dispose')
    contextListeners.clear()
    lifecycleListeners.clear()
    themeListeners.clear()
    routeListeners.clear()
  }
  function live() {
    if (state === 'dispose') fail('SESSION_REVOKED')
    if (!Number.isFinite(now()) || now() >= expiry) { dispose(); fail('SESSION_EXPIRED') }
  }
  function grant(name: string) {
    live()
    if (!grants.has(name)) fail('CAPABILITY_DENIED')
  }
  function subscribe<T>(listeners: Set<(event: T) => void>, listener: (event: T) => void) {
    live()
    if (typeof listener !== 'function') fail('INVALID_LISTENER')
    listeners.add(listener)
    return () => { listeners.delete(listener) }
  }
  async function invoke<T>(operation: (signal: AbortSignal) => Promise<T>, read: boolean, caller?: AbortSignal, timeoutMs = 30_000): Promise<T> {
    live()
    if (state === 'hidden') fail('DASHBOARD_HIDDEN')
    if (caller?.aborted) fail('REQUEST_CANCELLED')
    if (inflight.size >= 16) fail('TOO_MANY_REQUESTS')
    const controller = new AbortController()
    const revision = context.contextRevision
    const cancel = () => controller.abort()
    const deadline = setTimeout(cancel, timeoutMs)
    caller?.addEventListener('abort', cancel, { once: true })
    inflight.set(controller, read)
    let cancelPromise: (() => void) | undefined
    try {
      // Adapters may ignore AbortSignal; callers must nevertheless settle on revoke.
      const aborted = new Promise<never>((_, reject) => {
        cancelPromise = () => reject(new AppSDKError('REQUEST_CANCELLED'))
        controller.signal.addEventListener('abort', cancelPromise, { once: true })
      })
      const result = await Promise.race([operation(controller.signal), aborted])
      live()
      if (revision !== context.contextRevision) fail('CONTEXT_CHANGED')
      if (controller.signal.aborted) fail('REQUEST_CANCELLED')
      return result
    } finally {
      clearTimeout(deadline)
      caller?.removeEventListener('abort', cancel)
      if (cancelPromise) controller.signal.removeEventListener('abort', cancelPromise)
      inflight.delete(controller)
    }
  }

  const sdk: AppSDK = Object.freeze({
    version,
    media: Object.freeze({
      preview: async (input: import('./types').MediaPreviewInput) => {
        live(); validateMediaPreview(input)
        if (!adapters.previewMedia) fail('MEDIA_PREVIEW_UNSUPPORTED')
        const copy = { bytes: input.bytes.slice(0), mime: input.mime, title: input.title }
        return invoke(signal => adapters.previewMedia!(copy, signal), true)
      },
      close: () => { adapters.closeMedia?.() },
    }),
    context: Object.freeze({
      get: () => { live(); return context },
      subscribe: (listener: (value: Readonly<AppContext>) => void) => subscribe(contextListeners, listener),
    }),
    theme: Object.freeze({
      get: () => { live(); return theme },
      subscribe: (listener: (value: Theme) => void) => subscribe(themeListeners, listener),
    }),
    navigation: Object.freeze({
      get: () => { live(); return route },
      subscribe: (listener: (value: Readonly<AppRoute>) => void) => subscribe(routeListeners, listener),
      request: async (input: { target: 'app' | 'appcenter'; path: string }) => {
      grant('navigation.request')
      if (input.target !== 'app' && input.target !== 'appcenter') fail('INVALID_NAVIGATION')
      assertAppPath(input.path)
      return invoke(signal => adapters.navigate({ target: input.target, path: input.path }, signal), false)
    } }),
    files: Object.freeze({ select: async (input: { purpose: string; multiple?: boolean }) => {
      grant('files.select')
      if (!adapters.hasUserActivation()) fail('USER_ACTIVATION_REQUIRED')
      const purpose = id(input.purpose)
      if (input.multiple !== undefined && typeof input.multiple !== 'boolean') fail('INVALID_SELECTION')
      return invoke(async signal => {
        const files = await adapters.selectFiles({ purpose, multiple: input.multiple }, signal)
        return Object.freeze(files.map(file => Object.freeze({
          handle: file.handle, name: file.name, size: file.size, mime: file.mime,
        })))
      }, true, undefined, 120_000)
    } }),
    capabilities: Object.freeze({ call: async (name: string, input: Json, options?: { signal?: AbortSignal }) => {
      grant(id(name))
      const body = jsonCopy(input)
      if (name === 'workspace.local' && body && typeof body === 'object' && !Array.isArray(body)
        && ['openDirectory', 'importFiles'].includes(String(body.operation)) && !adapters.hasUserActivation()) fail('USER_ACTIVATION_REQUIRED')
      return invoke(signal => adapters.callCapability(name, body, signal), name === 'files.read', options?.signal,
        name === 'workspace.local' && version !== '1.0.0' ? 30 * 60_000 : 30_000)
    } }),
    app: Object.freeze({ request: async (input: AppRequest, options?: { signal?: AbortSignal }) => {
      grant('app.request')
      assertAppPath(input.path)
      if (!methods.has(input.method) || (input.method === 'GET' && input.body !== undefined)) fail('INVALID_REQUEST')
      // v1 API policies have exact path segments and no query negotiation.
      // Navigation may still carry its own App route query parameters.
      if (!/^\/(?:[a-zA-Z0-9._:-]+\/)*[a-zA-Z0-9._:-]+$/.test(input.path)) fail('INVALID_PATH')
      const request: AppRequest = { path: input.path, method: input.method }
      if (input.body !== undefined) request.body = jsonCopy(input.body)
      return invoke(signal => adapters.requestApp(request, signal), input.method === 'GET', options?.signal,
        version === '1.0.0' ? 30_000 : 65_000)
    } }),
    instances: Object.freeze({ open: async (input: { instanceId: string; entryId: string }) => {
      grant('instances.open')
      const target = { instanceId: id(input.instanceId), entryId: id(input.entryId) }
      return invoke(signal => adapters.openInstance(target, signal), false)
    } }),
    lifecycle: Object.freeze({ subscribe: (listener: (event: Lifecycle) => void) => subscribe(lifecycleListeners, listener) }),
  })

  return Object.freeze({
    sdk, sessionId,
    setTheme(next: Theme) {
      live()
      if (next !== 'light' && next !== 'dark') fail('INVALID_THEME')
      if (next === theme) return
      theme = next
      emit(themeListeners, theme)
    },
    setRoute(next: AppRoute) {
      live()
      assertAppPath(next.path)
      if (!Number.isSafeInteger(next.revision) || next.revision < 0) fail('INVALID_ROUTE_REVISION')
      if (next.revision < route.revision) return
      if (next.revision === route.revision) {
        if (next.path !== route.path) fail('ROUTE_REVISION_REQUIRED')
        return
      }
      route = Object.freeze({ path: next.path, revision: next.revision })
      adapters.closeMedia?.()
      emit(routeListeners, route)
    },
    setVisibility(visible: boolean) {
      live()
      const next = visible ? 'active' : 'hidden'
      if (next === state) return
      state = next
      if (!visible) adapters.closeMedia?.()
      if (!visible) for (const [controller, read] of inflight) if (read) controller.abort()
      emit(lifecycleListeners, next)
    },
    // Call only AFTER the host coordinates project-dependent dirty forms.
    updateContext(next: AppContext) {
      live()
      const replacement = contextCopy(next)
      for (const key of ['userId', 'organizationId', 'stationId', 'appId', 'installationId'] as const) {
        if (replacement[key] !== context[key]) { dispose(); fail('CONTEXT_MISMATCH') }
      }
      if (replacement.projectId !== context.projectId && replacement.contextRevision === context.contextRevision) {
        fail('CONTEXT_REVISION_REQUIRED')
      }
      if (replacement.contextRevision !== context.contextRevision) {
        adapters.closeMedia?.()
        for (const controller of inflight.keys()) controller.abort()
        adapters.releaseFiles?.()
      }
      context = replacement
      emit(contextListeners, context)
    },
    dispose,
  })
}

export type AppSDKSession = ReturnType<typeof createAppSDK>
