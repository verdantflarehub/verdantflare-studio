export const STUDIO_FRONTEND_VERSION = '0.5.230'
const reloadGuardKey = 'vf.studio.reloadedBuild.v1'

let remoteBuildVersion: string | null = null
let probing: Promise<boolean> | null = null

function isDesktop(): boolean {
  return typeof location !== 'undefined' && new URLSearchParams(location.search).get('host') === 'desktop'
}

export function hasServerBuildSkew(localVersion = STUDIO_FRONTEND_VERSION, serverVersion = remoteBuildVersion): boolean {
  return Boolean(localVersion && serverVersion && localVersion !== 'dev' && serverVersion !== 'dev' && localVersion !== serverVersion)
}

export function consumeBuildReloadTicket(
  serverVersion: string | null = remoteBuildVersion,
  storage: Pick<Storage, 'getItem' | 'setItem'> | undefined = globalThis.sessionStorage,
): boolean {
  if (!hasServerBuildSkew(STUDIO_FRONTEND_VERSION, serverVersion) || !serverVersion) return false
  try {
    if (storage?.getItem(reloadGuardKey) === serverVersion) return false
    storage?.setItem(reloadGuardKey, serverVersion)
  } catch {
    // Storage may be disabled in strict privacy contexts; still allow a single reload per runtime.
    remoteBuildVersion = STUDIO_FRONTEND_VERSION
  }
  return true
}

export function probeServerBuild(fetcher: typeof fetch = fetch): Promise<boolean> {
  if (isDesktop() || typeof location === 'undefined' || !['http:', 'https:'].includes(location.protocol)) {
    return Promise.resolve(false)
  }
  if (probing) return probing
  probing = (async () => {
    try {
      const res = await fetcher('/health', { method: 'GET', credentials: 'same-origin', cache: 'no-store' })
      if (!res.ok) return false
      const data: unknown = await res.json()
      if (data && typeof data === 'object' && 'service' in data && data.service === 'verdantflare-studio'
        && 'version' in data && typeof data.version === 'string' && /^[0-9A-Za-z._-]{1,64}$/.test(data.version)) {
        remoteBuildVersion = data.version
      }
      return hasServerBuildSkew()
    } catch {
      return false
    } finally {
      probing = null
    }
  })()
  return probing
}

export function reloadIfBuildChanged(reloader: () => void = () => location.reload()): boolean {
  if (isDesktop() || !consumeBuildReloadTicket()) return false
  reloader()
  return true
}
