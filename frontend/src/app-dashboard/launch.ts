import type { AppContext, DashboardLaunch } from './types'

export class DashboardLaunchError extends Error {
  constructor(readonly code: string) { super(code); this.name = 'DashboardLaunchError' }
}

export interface LaunchPolicy {
  // Host configuration, never supplied by Dashboard or query parameters.
  assetBaseUrl: string
  context: AppContext
  installationId: string
  appId: string
  appVersion: string
  artifactDigest: string
  requiredCapabilities: readonly string[]
  allowedCapabilities: readonly string[]
  trusted: boolean
}

const fields = ['dashboardSessionId', 'installationId', 'appId', 'appVersion', 'artifactDigest', 'adapter',
  'entryUrl', 'sdkVersion', 'grantedCapabilities', 'contextRevision', 'expiresAt']
const identifier = /^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$/
const version = /^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/
const fail = (code: string): never => { throw new DashboardLaunchError(code) }

/** Validate the server projection before allowing any page code to execute. */
export function validateLaunch(input: unknown, policy: LaunchPolicy, now = Date.now()): DashboardLaunch {
  if (!input || typeof input !== 'object' || Array.isArray(input)) fail('INVALID_LAUNCH')
  const value = input as Record<string, unknown>
  if (Object.keys(value).length !== fields.length || fields.some(key => !Object.hasOwn(value, key))) fail('INVALID_LAUNCH')
  for (const key of fields.filter(key => key !== 'grantedCapabilities')) {
    if (typeof value[key] !== 'string' || !value[key]) fail('INVALID_LAUNCH')
  }
  for (const key of ['dashboardSessionId', 'installationId', 'appId', 'contextRevision']) {
    if (!identifier.test(value[key] as string)) fail('INVALID_LAUNCH')
  }
  if (!version.test(value.appVersion as string) || !version.test(value.sdkVersion as string)) fail('INVALID_VERSION')
  if (!(value.sdkVersion as string).startsWith('1.')) fail('SDK_VERSION_UNSUPPORTED')
  if (!/^sha256:[a-f0-9]{64}$/.test(value.artifactDigest as string)) fail('INVALID_DIGEST')
  if (value.adapter !== 'wujie') fail('ADAPTER_UNSUPPORTED')
  if (!policy.trusted) fail('DASHBOARD_UNTRUSTED')
  if (value.appId !== policy.appId || value.appId !== policy.context.appId
    || value.installationId !== policy.installationId || value.installationId !== policy.context.installationId
    || value.contextRevision !== policy.context.contextRevision) fail('CONTEXT_MISMATCH')
  if (value.artifactDigest !== policy.artifactDigest || value.appVersion !== policy.appVersion) fail('ARTIFACT_MISMATCH')
  const expiry = Date.parse(value.expiresAt as string)
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:\d{2})$/.test(value.expiresAt as string)
    || !Number.isFinite(expiry) || expiry <= now) fail('SESSION_EXPIRED')
  const grants = value.grantedCapabilities
  if (!Array.isArray(grants) || grants.length > 128 || grants.some(item => typeof item !== 'string' || !identifier.test(item))
    || new Set(grants).size !== grants.length) fail('INVALID_CAPABILITIES')
  const capabilities = grants as string[]
  if (capabilities.some(item => !policy.allowedCapabilities.includes(item))) fail('CAPABILITY_DENIED')
  if (policy.requiredCapabilities.some(item => !capabilities.includes(item))) fail('CAPABILITY_MISSING')
  const raw = value.entryUrl as string
  // Reject URL normalization tricks before resolving against the trusted base.
  if (raw.length > 2048 || /[\\\s\u0000-\u001f\u007f]/.test(raw) || /%(?:2e|2f|5c|25)/i.test(raw)
    || raw.split(/[/?#]/).some(part => part === '.' || part === '..')) fail('INVALID_ENTRY')
  try {
    const base = new URL(policy.assetBaseUrl)
    const entry = new URL(raw, base)
    if (!['https:', 'http:'].includes(base.protocol) || base.username || base.password
      || !base.pathname.endsWith('/') || entry.origin !== base.origin || entry.username || entry.password
      || !entry.pathname.startsWith(base.pathname) || entry.search || entry.hash) fail('INVALID_ENTRY')
  } catch { fail('INVALID_ENTRY') }
  return Object.freeze({ ...value, grantedCapabilities: Object.freeze([...capabilities]) }) as unknown as DashboardLaunch
}

export function dashboardCacheKey(launch: DashboardLaunch, context: AppContext, authorizationEpoch: string): string {
  // JSON tuples avoid delimiter collisions. Project is coordinated via context updates.
  return JSON.stringify([context.userId, context.organizationId, context.stationId, launch.installationId,
    launch.appId, launch.artifactDigest, launch.sdkVersion.split('.')[0], authorizationEpoch])
}
