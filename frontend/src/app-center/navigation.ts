import type { AppCenterState } from './controller'
import type { NavigationEnvelope, NavigationEntry } from '../navigation/controller'

export type AppCenterNavigationEntry = NavigationEntry & { cardKey?: string; bindingRevision?: string; openReason?: string }

/** The menu contributes title/order; the shared snapshot alone permits opening. */
export function appCenterNavigationEntries(navigation: NavigationEnvelope, state: AppCenterState, _now?: number): AppCenterNavigationEntry[] {
  const view = state.value
  const sameScope = view && (['userId', 'organizationId', 'stationId', 'authorizationEpoch'] as const)
    .every(key => view.scope[key] === navigation.projection.scope[key])
  const fresh = sameScope && !state.stale && state.status !== 'error'
  return navigation.projection.entries.map(entry => {
    const card = sameScope ? view.items.find(card => card.installation?.installationId === entry.installationId
      && card.appId === entry.appId && card.installedVersion === entry.appVersion) : undefined
    const open = card?.actions.find(action => action.kind === 'open' && action.entryId === card.entry?.entryId)
    if (fresh && card?.entry?.transport === 'dashboard' && open?.enabled) {
      return { ...entry, availability: 'ready', cardKey: card.cardKey, bindingRevision: card.bindingRevision }
    }
    return { ...entry, availability: fresh && card?.service.quality === 'known'
      && ['absent', 'stopped', 'starting', 'stopping', 'degraded'].includes(card.service.state) ? 'offline' : 'unknown',
      openReason: !fresh ? 'FACTS_UNAVAILABLE' : !card ? 'APPCENTER_BINDING_CHANGED' : open?.reasonCode ?? 'ENTRY_MISSING' }
  })
}
