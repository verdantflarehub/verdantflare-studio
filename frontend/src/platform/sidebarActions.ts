import type { NavigationEntry } from '../navigation/controller.ts'

// Host utilities are not business-menu rendering rules.
export function stationNavigationTarget(entries: NavigationEntry[]) {
  return entries.find(entry => entry.appId === 'resources')
}
export const legacyStationLocation = '/resources'
