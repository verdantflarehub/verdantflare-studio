import { assertAppPath } from './sdk.ts'

export const installationPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
export const appIdPattern = /^(?![0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$)[a-z0-9][a-z0-9-]{0,63}$/
const projectCategories = new Set(['image', 'character', 'music', 'video', 'ai_short_drama'])

export function instanceWorkspaceLocation(installationId: string, workspaceSessionId: string) {
  if (!installationPattern.test(installationId) || !/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(workspaceSessionId)) throw new Error('INVALID_WORKSPACE')
  return `/apps/${installationId}/instances/${workspaceSessionId}`
}

export function canonicalAppPath(appId: string | undefined, path = '/'): string {
  assertAppPath(path)
  const u = new URL(path, 'https://app.invalid')
  if (appId === 'project') {
    if (u.pathname === '/' || u.pathname === '/projects') {
      const cat = u.searchParams.get('category')
      if (cat && projectCategories.has(cat)) {
        u.pathname = '/' + cat
        u.searchParams.delete('category')
      } else if (u.pathname === '/projects' && !cat && !u.search) {
        u.pathname = '/character'
      }
    } else if (projectCategories.has(u.pathname.slice(1)) && u.searchParams.get('category') === u.pathname.slice(1)) {
      u.searchParams.delete('category')
    }
  } else if (appId === 'resources') {
    if (u.pathname === '/') {
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
  }
  return u.pathname + u.search
}

export function dashboardSdkRoute(appId: string | undefined, path = '/'): string {
  assertAppPath(path)
  const u = new URL(path, 'https://app.invalid')
  if (appId === 'project') {
    if (u.pathname === '/') {
      u.pathname = '/projects'
      if (!u.searchParams.has('category')) u.searchParams.set('category', 'character')
    } else if (projectCategories.has(u.pathname.slice(1))) {
      const cat = u.pathname.slice(1)
      const next = new URLSearchParams({ category: cat })
      u.searchParams.forEach((v, k) => { if (k !== 'category') next.set(k, v) })
      return '/projects?' + next.toString()
    }
  } else if (appId === 'resources') {
    if (u.pathname === '/storage' || u.pathname === '/tasks') {
      const view = u.pathname.slice(1)
      u.pathname = '/'
      u.searchParams.set('view', view)
    } else if (u.pathname === '/monitor') {
      u.pathname = '/'
      u.searchParams.delete('view')
    }
  }
  return u.pathname + u.search
}

export function dashboardLocation(appId: string, path = '/') {
  if (!appIdPattern.test(appId)) throw new Error('INVALID_APP_ID')
  const canonical = canonicalAppPath(appId, path)
  const u = new URL(canonical, 'https://app.invalid')
  const query: Record<string, string> = {}
  u.searchParams.forEach((v, k) => { query[k] = v })
  return { path: u.pathname === '/' ? `/apps/${appId}/` : `/apps/${appId}${u.pathname}`, query }
}

export function dashboardRoutePath(value: unknown): string {
  if (value === undefined) return '/'
  if (typeof value !== 'string') throw new Error('INVALID_PATH')
  assertAppPath(value)
  return value
}

export function browserDashboardPath(route: { params?: Record<string, unknown>; query?: Record<string, unknown> }, appId?: string): string {
  const subpath = route.params?.subpath
  const segments = Array.isArray(subpath)
    ? subpath.filter((s): s is string => typeof s === 'string' && s.length > 0)
    : typeof subpath === 'string' && subpath ? [subpath] : []
  if (Array.isArray(subpath) && segments.length !== subpath.length) throw new Error('INVALID_PATH')
  if (segments.length === 0 && route.query?.path !== undefined) {
    return canonicalAppPath(appId, dashboardRoutePath(route.query.path))
  }
  const basePath = segments.length > 0 ? '/' + segments.join('/') : '/'
  const params = new URLSearchParams()
  for (const [k, v] of Object.entries(route.query ?? {})) {
    if (k === 'path') continue
    if (typeof v === 'string') params.set(k, v)
    else if (v !== undefined) throw new Error('INVALID_PATH')
  }
  const qs = params.toString()
  return canonicalAppPath(appId, dashboardRoutePath(basePath + (qs ? '?' + qs : '')))
}

/** Project owns its complete category → object trail on the browser route. */
export function dashboardOwnsBreadcrumb(appId: string | undefined, value: unknown): boolean {
  if (appId !== 'project') return false
  try {
    const pathname = new URL(dashboardRoutePath(value), 'https://app.invalid').pathname
    return pathname === '/projects' || projectCategories.has(pathname.slice(1))
  } catch { return false }
}
