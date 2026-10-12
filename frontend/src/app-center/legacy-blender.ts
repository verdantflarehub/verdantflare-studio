import { AppCenterError } from './controller.ts'

const workspace = /^\/studio\/legacy-blender\/[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/

export function validStudioOrigin(value: unknown): value is string {
  if (typeof value !== 'string' || value.length > 2048) return false
  try {
    const u = new URL(value)
    return value === u.origin && !u.username && !u.password && (u.protocol === 'https:' || u.protocol === 'http:'
      && (u.hostname === 'localhost' || u.hostname === '[::1]' || /^127(?:\.[0-9]{1,3}){3}$/.test(u.hostname)))
  } catch { return false }
}

/** Wrap management and save JSON in the original Go-owned workspace. No retries. */
export function createBlenderFetcher(resourceBase?: string, fetcher: typeof fetch = fetch): typeof fetch {
  if (resourceBase === undefined) return fetcher
  if (!workspace.test(resourceBase)) throw new AppCenterError(409, 'LEGACY_WORKSPACE_GONE')
  return async (input, init) => {
    const method = init?.method ?? 'GET', body = init?.body ?? ''
    if (typeof input !== 'string' || input.length > 2048 || !/^\/(?:studio\/apps\/blender|mcp)\/[A-Za-z0-9_/-]+$/.test(input)
      || !['GET', 'POST'].includes(method) || typeof body !== 'string' || method === 'GET' && body !== ''
      || new TextEncoder().encode(body).length > 16384) throw new AppCenterError(400, 'INVALID_LEGACY_REQUEST')
    return fetcher(resourceBase + '/requests', { method: 'POST', credentials: 'omit', cache: 'no-store', redirect: 'error',
      signal: init?.signal, keepalive: init?.keepalive, headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ method, path: input, body }) })
  }
}

export function blenderMCPAddress(origin: string, alias: string, path: string): string {
  if (!validStudioOrigin(origin) || !/^blender[A-Za-z0-9_-]{1,57}$/.test(alias) || path !== '/mcp/' + alias) throw new AppCenterError(502, 'INVALID_LEGACY_RESPONSE')
  return origin + path
}
