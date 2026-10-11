import { assertAppPath } from '../app-dashboard/sdk.ts'
import { dashboardLocation } from '../app-dashboard/routes.ts'
import type { Json } from '../app-dashboard/types'
import type { NavigationEnvelope } from './controller'

export interface AppTarget { appId: string; path: string }
export function decodeAppTarget(value: Json): AppTarget {
  if (!value || typeof value !== 'object' || Array.isArray(value) || Object.keys(value).sort().join(',') !== 'appId,path'
    || typeof value.appId !== 'string' || !/^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$/.test(value.appId)
    || typeof value.path !== 'string') throw new Error('INVALID_NAVIGATION')
  assertAppPath(value.path)
  return { appId: value.appId, path: value.path }
}
export function appTargetLocation(target: AppTarget, navigation: NavigationEnvelope) {
  if (navigation.mode !== 'preinstalled') throw new Error('APP_NAVIGATION_UNAVAILABLE')
  const entries = navigation.projection.entries.filter(entry => entry.appId === target.appId)
  if (entries.length !== 1) throw new Error('APP_NAVIGATION_UNAVAILABLE')
  return dashboardLocation(entries[0].installationId, target.path)
}

/** Only compatibility bookmarks are product-specific; Dashboard loading stays generic. */
export function legacyAppTarget(path: string): AppTarget | null {
  assertAppPath(path)
  const parsed = new URL(path, 'https://studio.invalid')
  parsed.searchParams.forEach((_value, key) => { if (parsed.searchParams.getAll(key).length !== 1) throw new Error('INVALID_NAVIGATION') })
  if (parsed.pathname === '/resources') return { appId: 'resources', path: '/' }
  if (parsed.pathname === '/station/proxies') return { appId: 'network', path: '/' }
  if (parsed.pathname !== '/projects') return null
  const mode = parsed.searchParams.get('mode')
  if (mode && !['world', 'projects'].includes(mode)) throw new Error('INVALID_NAVIGATION')
  parsed.searchParams.delete('mode')
  if (mode !== 'world' && parsed.searchParams.get('view') === 'files') {
    parsed.searchParams.delete('view')
    const project = parsed.searchParams.get('project')
    if (project && !/^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(project)) throw new Error('INVALID_NAVIGATION')
    return { appId: 'project', path: (project ? '/workspace' : '/projects') + parsed.search }
  }
  return { appId: mode === 'world' ? 'world' : 'project', path: (mode === 'world' ? '/' : '/projects') + parsed.search }
}
