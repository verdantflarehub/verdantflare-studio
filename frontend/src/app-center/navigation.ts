import type { AppCenterState } from './controller'
import type { NavigationEnvelope, NavigationEntry } from '../navigation/controller'

export type AppCenterNavigationEntry = NavigationEntry & { cardKey?: string; bindingRevision?: string; openReason?: string }

/** The menu contributes title/order; the shared snapshot alone permits opening. */
export function appCenterNavigationEntries(navigation: NavigationEnvelope, state: AppCenterState, _now?: number): AppCenterNavigationEntry[] {
  const view = state.value
  const sameScope = view && (['userId', 'organizationId', 'stationId', 'authorizationEpoch'] as const)
    .every(key => view.scope[key] === navigation.projection.scope[key])
  const fresh = sameScope && state.status === 'ready'
  return navigation.projection.entries.map(entry => {
    const card = sameScope ? view.items.find(card => card.installation?.installationId === entry.installationId
      && card.appId === entry.appId && card.installedVersion === entry.appVersion) : undefined
    const open = card?.actions.find(action => action.kind === 'open' && action.entryId === card.entry?.entryId)
    // The badge shows the observed service; clicking needs only the static
    // open binding, so a stopped App still opens and explains itself inside.
    const offline = fresh && card?.service.quality === 'known' && ['absent', 'stopped', 'starting', 'stopping', 'degraded'].includes(card.service.state)
    if (fresh && card?.entry?.transport === 'dashboard' && open?.enabled) {
      const ready = card.service.quality === 'known' && card.service.state === 'ready'
      return { ...entry, availability: ready ? 'ready' : offline ? 'offline' : 'unknown', cardKey: card.cardKey, bindingRevision: card.bindingRevision }
    }
    return { ...entry, availability: offline ? 'offline' : 'unknown',
      openReason: !fresh ? 'FACTS_UNAVAILABLE' : !card ? 'APPCENTER_BINDING_CHANGED' : open?.reasonCode ?? 'ENTRY_MISSING' }
  })
}
