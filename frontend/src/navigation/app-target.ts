import type { RouteLocationRaw } from 'vue-router'
import type { AppCenterView } from '../app-center/schema.generated'
import { assertAppPath } from '../app-dashboard/sdk.ts'
import { dashboardLocation } from '../app-dashboard/routes.ts'
import type { Json } from '../app-dashboard/types'
import type { NavigationEnvelope } from './controller'

const deniedReasonCodes = new Set(['PERMISSION_DENIED', 'FORBIDDEN', 'CAPABILITY_DENIED'])
const appIdPattern = /^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$/

export interface AppTarget { appId: string; path: string }
export function decodeAppTarget(value: Json): AppTarget {
  if (!value || typeof value !== 'object' || Array.isArray(value) || Object.keys(value).sort().join(',') !== 'appId,path'
    || typeof value.appId !== 'string' || !appIdPattern.test(value.appId)
    || typeof value.path !== 'string') throw new Error('INVALID_NAVIGATION')
  assertAppPath(value.path)
  return { appId: value.appId, path: value.path }
}
export function appCenterReturnLocation(path: string, fallback: RouteLocationRaw = '/market'): RouteLocationRaw {
  assertAppPath(path)
  const parsed = new URL(path, 'https://studio.invalid')
  if (parsed.pathname !== '/') throw new Error('INVALID_PATH')
  if (!parsed.search) return fallback
  const app = parsed.searchParams.get('app')
  if (!app || parsed.searchParams.size !== 1 || parsed.searchParams.getAll('app').length !== 1 || !appIdPattern.test(app)) {
    throw new Error('INVALID_PATH')
  }
  return { path: '/market', query: { view: 'all', q: app } }
}
export function appTargetLocation(target: AppTarget, navigation: NavigationEnvelope) {
  if (navigation.mode !== 'preinstalled') throw new Error('APP_NAVIGATION_UNAVAILABLE')
  const entries = navigation.projection.entries.filter(entry => entry.appId === target.appId)
  if (entries.length !== 1) throw new Error('APP_NAVIGATION_UNAVAILABLE')
  return dashboardLocation(entries[0].appId, target.path)
}
/** Resolves an installed AppCenter Dashboard app (including non-sidebar apps such as viewer and tryon). */
export function appCenterTargetLocation(target: AppTarget, view: AppCenterView, now: number, authorizationEpoch: string) {
  if (!Number.isFinite(now) || Date.parse(view.validUntil) <= now) throw new Error('APPCENTER_VIEW_EXPIRED')
  if (view.scope.authorizationEpoch !== authorizationEpoch) throw new Error('CONTEXT_CHANGED')
  const matches = view.items.filter(card => card.appId === target.appId && card.installation?.state === 'installed')
  if (matches.length !== 1) throw new Error('APP_NAVIGATION_UNAVAILABLE')
  const card = matches[0]
  if (card.installationQuality !== 'known' || !card.entry || card.entry.transport !== 'dashboard') {
    throw new Error('APP_NAVIGATION_UNAVAILABLE')
  }
  const open = card.actions.find(action => action.kind === 'open' && action.entryId === card.entry?.entryId)
    ?? card.actions.find(action => action.kind === 'open')
  if (card.entry.availability === 'denied'
    || (card.entry.reasonCode !== null && deniedReasonCodes.has(card.entry.reasonCode))
    || (open?.reasonCode !== null && open?.reasonCode !== undefined && deniedReasonCodes.has(open.reasonCode))) {
    throw new Error('PERMISSION_DENIED')
  }
  if (card.entry.quality !== 'known' || card.entry.availability !== 'available' || !open?.enabled || open.entryId !== card.entry.entryId) {
    throw new Error('APP_NAVIGATION_UNAVAILABLE')
  }
  return dashboardLocation(card.appId, target.path)
}

/** Only compatibility bookmarks are product-specific; Dashboard loading stays generic. */
export function legacyAppTarget(path: string): AppTarget | null {
  assertAppPath(path)
  const parsed = new URL(path, 'https://studio.invalid')
  parsed.searchParams.forEach((_value, key) => { if (parsed.searchParams.getAll(key).length !== 1) throw new Error('INVALID_NAVIGATION') })
  if (parsed.pathname === '/resources') {
    const view = parsed.searchParams.get('view')
    if (view && !['monitor', 'storage', 'tasks'].includes(view)) throw new Error('INVALID_NAVIGATION')
    return { appId: 'resources', path: '/' + parsed.search }
  }
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
