import { validateLaunch } from './launch.ts'
import type { LaunchPolicy } from './launch'
import type { AppContext, AppRequest, AppResponse, DashboardLaunch, Json } from './types'
import { responseClock } from '../pages/response-clock.js'
import { hostContentPreferences } from '../platform/contentPreferences.ts'

export class DashboardTransportError extends Error {
  constructor(readonly code: string) { super(code); this.name = 'DashboardTransportError' }
}
export interface OpenedDashboard { launch: DashboardLaunch; context: AppContext; policy: LaunchPolicy }
export interface AppCenterOpenInput {
  schemaVersion: 1; cardKey: string; entryId: string; expectedViewRevision: string; expectedBindingRevision: string
}
export interface DashboardInstallation { installationId: string; appId: string; appVersion: string }
export interface InstanceWorkspaceLaunch {
  workspaceSessionId: string; installationId: string; appId: string; instanceId: string; instanceName?: string; entryId: string
  entryUrl: string; assetBaseUrl: string; expiresAt: string
  now?: () => number
}

const identifier = /^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$/
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[47][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const failure = (code: string): never => { throw new DashboardTransportError(code) }
const codeFrom = (data: Json): string => data && typeof data === 'object' && !Array.isArray(data)
  && typeof data.code === 'string' && /^[A-Z][A-Z0-9_]{0,63}$/.test(data.code) ? data.code : 'DASHBOARD_REQUEST_FAILED'

async function readJSON(response: Response, maximum: number): Promise<Json> {
  if (response.status === 204) return null
  if (response.headers.get('Content-Type')?.split(';')[0].trim() !== 'application/json' || !response.body) {
    return failure('INVALID_DASHBOARD_RESPONSE')
  }
  const reader = response.body.getReader(), decoder = new TextDecoder('utf-8', { fatal: true })
  let size = 0, text = ''
  try {
    for (;;) {
      const chunk = await reader.read()
      if (chunk.done) break
      size += chunk.value.byteLength
      if (size > maximum) failure('INVALID_DASHBOARD_RESPONSE')
      text += decoder.decode(chunk.value, { stream: true })
    }
    text += decoder.decode()
    return JSON.parse(text) as Json
  } catch {
    await reader.cancel().catch(() => undefined)
    return failure('INVALID_DASHBOARD_RESPONSE')
  } finally { reader.releaseLock() }
}

/** Host-owned Web transport. Never inject this client or its fetcher into App code. */
export function createDashboardWebClient(
  origin: string,
  fetcher: typeof fetch = fetch,
  monotonic = () => performance.now(),
  includeNsfw?: () => boolean,
) {
  const resolveIncludeNsfw = includeNsfw ?? (fetcher === fetch ? () => hostContentPreferences.get().includeNsfw : () => false)
  const base = new URL(origin)
  if (!['https:', 'http:'].includes(base.protocol) || base.origin !== origin) failure('INVALID_DASHBOARD_ORIGIN')
  let clock: (() => number) | undefined
  const now = () => clock?.() ?? Date.now()

  function sessionPath(id: string) {
    if (!uuid.test(id) || id[14] !== '4') failure('INVALID_DASHBOARD_SESSION')
    return `/studio/dashboards/${id}`
  }
  function workspacePath(id: string) {
    if (!uuid.test(id) || id[14] !== '4') failure('INVALID_WORKSPACE_SESSION')
    return `/studio/instance-workspaces/${id}`
  }
  function workspace(response: AppResponse, installationId: string): InstanceWorkspaceLaunch {
    if (response.status !== 200) return failure(codeFrom(response.data))
    const value = response.data
    const keys = value && typeof value === 'object' && !Array.isArray(value) ? Object.keys(value).sort().join(',') : ''
    if (!value || typeof value !== 'object' || Array.isArray(value)
      || (keys !== 'appId,assetBaseUrl,entryId,entryUrl,expiresAt,installationId,instanceId,workspaceSessionId'
        && keys !== 'appId,assetBaseUrl,entryId,entryUrl,expiresAt,installationId,instanceId,instanceName,workspaceSessionId')
      || value.installationId !== installationId || !uuid.test(installationId) || installationId[14] !== '7'
      || ['appId', 'instanceId', 'entryId'].some(key => typeof value[key] !== 'string' || !identifier.test(value[key] as string))
      || (value.instanceName !== undefined && (typeof value.instanceName !== 'string' || !value.instanceName.trim() || value.instanceName.length > 256))
      || typeof value.workspaceSessionId !== 'string' || typeof value.entryUrl !== 'string'
      || typeof value.assetBaseUrl !== 'string' || !/^\/studio\/instance-assets\/[a-f0-9]{64}\/$/.test(value.assetBaseUrl)
      || typeof value.expiresAt !== 'string' || !Number.isFinite(Date.parse(value.expiresAt)) || !Number.isFinite(now()) || Date.parse(value.expiresAt) <= now()) return failure('INVALID_WORKSPACE_RESPONSE')
    const root = workspacePath(value.workspaceSessionId) + '/assets/'
    if (!value.entryUrl.startsWith(root) || !/^[a-zA-Z0-9._/-]+$/.test(value.entryUrl)
      || value.entryUrl.split('/').slice(1).some(part => !part || part === '.' || part === '..')
      || !value.entryUrl.endsWith('.html')) return failure('INVALID_WORKSPACE_RESPONSE')
    return Object.freeze({ ...value, now }) as unknown as InstanceWorkspaceLaunch
  }
  async function call(path: string, method: string, body: unknown, signal?: AbortSignal, extraHeaders?: Record<string, string>): Promise<AppResponse> {
    if (signal?.aborted) return failure('REQUEST_CANCELLED')
    try {
      const started = monotonic()
      const headers: Record<string, string> = body === undefined ? {} : { 'Content-Type': 'application/json' }
      if (extraHeaders) Object.assign(headers, extraHeaders)
      const response = await fetcher(new URL(path, base).href, { method, credentials: 'same-origin', cache: 'no-store',
        redirect: 'error', signal, headers,
        body: body === undefined ? undefined : JSON.stringify(body),
      })
      const data = await readJSON(response, 4 << 20)
      if (signal?.aborted) return failure('REQUEST_CANCELLED')
      if (response.ok && response.status !== 204) {
        try {
          const observed = responseClock(response.headers.get('Date'), started, monotonic)
          if (!clock || observed() > clock()) clock = observed
        } catch {
          if (!clock) clock = () => Date.now()
        }
      }
      return { status: response.status, data, requestId: response.headers.get('X-Request-ID') ?? '' }
    } catch (error) {
      if (signal?.aborted) return failure('REQUEST_CANCELLED')
      if (error instanceof DashboardTransportError) throw error
      return failure('DASHBOARD_CONNECTION_FAILED')
    }
  }

  function opened(response: AppResponse, installationId: string): OpenedDashboard {
      if (!uuid.test(installationId) || installationId[14] !== '7') return failure('INVALID_INSTALLATION')
      if (response.status !== 200) return failure(codeFrom(response.data))
      const envelope = response.data
      if (!envelope || typeof envelope !== 'object' || Array.isArray(envelope)
        || Object.keys(envelope).sort().join(',') !== 'context,launch') return failure('INVALID_DASHBOARD_RESPONSE')
      const rawContext = envelope.context
      const contextFields = ['userId', 'organizationId', 'stationId', 'projectId', 'appId', 'installationId', 'contextRevision']
      if (!rawContext || typeof rawContext !== 'object' || Array.isArray(rawContext)
        || Object.keys(rawContext).length !== contextFields.length || contextFields.some(key => !Object.hasOwn(rawContext, key))
        || contextFields.filter(key => key !== 'projectId').some(key => typeof rawContext[key] !== 'string' || !identifier.test(rawContext[key] as string))
        || rawContext.projectId !== null || rawContext.installationId !== installationId) return failure('INVALID_DASHBOARD_RESPONSE')
      const rawLaunch = envelope.launch
      if (!rawLaunch || typeof rawLaunch !== 'object' || Array.isArray(rawLaunch)) return failure('INVALID_DASHBOARD_RESPONSE')
      const context = Object.freeze({ ...rawContext }) as unknown as AppContext
      const assetBaseUrl = new URL(sessionPath(String(rawLaunch.dashboardSessionId)) + '/assets/', base).href
      // Trust derives from this authenticated platform response. App declarations
      // and URL parameters cannot manufacture a policy or supply credentials.
      const policy: LaunchPolicy = { assetBaseUrl, context, installationId, appId: context.appId,
        appVersion: rawLaunch.appVersion as string, artifactDigest: rawLaunch.artifactDigest as string,
        requiredCapabilities: [], allowedCapabilities: Array.isArray(rawLaunch.grantedCapabilities)
          ? rawLaunch.grantedCapabilities as string[] : [], trusted: true, now }
      const launch = validateLaunch(rawLaunch, policy)
      return { launch, context, policy: Object.freeze(policy) }
  }

  return Object.freeze({
    now,
    async openAppCenter(input: AppCenterOpenInput, installationId: string, signal?: AbortSignal): Promise<OpenedDashboard> {
      const response = await call('/studio/appcenter/v1/open', 'POST', input, signal)
      if (response.status !== 200) return failure(codeFrom(response.data))
      const body = response.data
      if (!body || typeof body !== 'object' || Array.isArray(body)
        || Object.keys(body).sort().join(',') !== 'opened,schemaVersion,transport'
        || body.schemaVersion !== 1 || body.transport !== 'dashboard') return failure('INVALID_DASHBOARD_RESPONSE')
      return opened({ ...response, data: body.opened }, installationId)
    },
    async openInstance(sessionId: string, installationId: string, input: {instanceId: string; entryId: string}, signal?: AbortSignal): Promise<InstanceWorkspaceLaunch> {
      if (!identifier.test(input.instanceId) || !identifier.test(input.entryId)) return failure('INVALID_INSTANCE_ENTRY')
      const result = workspace(await call(sessionPath(sessionId) + '/instances/open', 'POST', input, signal), installationId)
      if (result.instanceId !== input.instanceId || result.entryId !== input.entryId) return failure('INVALID_WORKSPACE_RESPONSE')
      return result
    },
    async inspectWorkspace(id: string, installationId: string, signal?: AbortSignal): Promise<InstanceWorkspaceLaunch> {
      const result = workspace(await call(workspacePath(id), 'GET', undefined, signal), installationId)
      if (result.workspaceSessionId !== id) return failure('INVALID_WORKSPACE_RESPONSE')
      return result
    },
    async closeWorkspace(id: string, signal?: AbortSignal): Promise<void> {
      const response = await call(workspacePath(id), 'DELETE', undefined, signal)
      if (response.status !== 204) failure(codeFrom(response.data))
    },
    async installations(signal?: AbortSignal): Promise<DashboardInstallation[]> {
      const response = await call('/studio/dashboard-installations', 'GET', undefined, signal)
      if (response.status !== 200) return failure(codeFrom(response.data))
      const body = response.data
      if (!body || typeof body !== 'object' || Array.isArray(body) || Object.keys(body).join(',') !== 'items'
        || !Array.isArray(body.items) || body.items.length > 1000) return failure('INVALID_DASHBOARD_RESPONSE')
      const seen = new Set<string>()
      return body.items.map(item => {
        if (!item || typeof item !== 'object' || Array.isArray(item)
          || Object.keys(item).sort().join(',') !== 'appId,appVersion,installationId'
          || typeof item.installationId !== 'string' || !uuid.test(item.installationId) || item.installationId[14] !== '7'
          || seen.has(item.installationId) || typeof item.appId !== 'string' || !identifier.test(item.appId)
          || typeof item.appVersion !== 'string' || !/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(item.appVersion)) return failure('INVALID_DASHBOARD_RESPONSE')
        seen.add(item.installationId)
        return { installationId: item.installationId, appId: item.appId, appVersion: item.appVersion }
      })
    },
    async open(installationId: string, signal?: AbortSignal): Promise<OpenedDashboard> {
      if (!uuid.test(installationId) || installationId[14] !== '7') return failure('INVALID_INSTALLATION')
      return opened(await call('/studio/dashboards', 'POST', { installationId }, signal), installationId)
    },
    async inspect(sessionId: string, installationId: string, signal?: AbortSignal): Promise<OpenedDashboard> {
      const result = opened(await call(sessionPath(sessionId), 'GET', undefined, signal), installationId)
      if (result.launch.dashboardSessionId !== sessionId) return failure('INVALID_DASHBOARD_RESPONSE')
      return result
    },
    request(sessionId: string, input: AppRequest, signal?: AbortSignal): Promise<AppResponse> {
      const extraHeaders = resolveIncludeNsfw() ? { 'X-VF-Include-NSFW': '1' } : undefined
      return call(sessionPath(sessionId) + '/requests', 'POST', input, signal, extraHeaders)
    },
    async capability(sessionId: string, name: string, input: Json, signal?: AbortSignal): Promise<Json> {
      if (name !== 'artifact.read' && name !== 'artifact.write' && name !== 'workspace.local') return failure('CAPABILITY_UNAVAILABLE')
      const response = await call(sessionPath(sessionId) + '/capabilities/' + name, 'POST', input, signal)
      if (response.status !== 200) return failure(codeFrom(response.data))
      if (name === 'workspace.local') return response.data
      const value = response.data
      if (!value || typeof value !== 'object' || Array.isArray(value)) return failure('INVALID_CONTENT_RESPONSE')
      if (value.mode === 'download') {
        const prefix = sessionPath(sessionId) + '/content/'
        const requested = input && typeof input === 'object' && !Array.isArray(input) ? input.representation : undefined
        const stable = typeof value.content_path === 'string' && typeof requested === 'string'
          && /^\/studio\/media\/[a-f0-9]{64}\/[a-f0-9]{64}\/(original-v1|thumbnail-(320|640)-v1|poster-(320|640)-v1)$/.test(value.content_path)
          && value.content_path.endsWith('/' + requested)
        if (!stable && (typeof value.content_path !== 'string' || !value.content_path.startsWith(prefix)
          || !uuid.test(value.content_path.slice(prefix.length)) || value.content_path.slice(prefix.length)[14] !== '4')) return failure('INVALID_CONTENT_RESPONSE')
      }
      return value
    },
    async close(sessionId: string, signal?: AbortSignal): Promise<void> {
      const response = await call(sessionPath(sessionId), 'DELETE', undefined, signal)
      if (response.status !== 204) failure(codeFrom(response.data))
    },
  })
}
