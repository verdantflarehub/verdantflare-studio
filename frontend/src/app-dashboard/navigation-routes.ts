import type { NavigationEntry } from '../navigation/controller.ts'
import { canonicalAppPath } from './routes.ts'

/** Legacy declarations are normalized at the existing app-route boundary, before menu matching. */
export function navigationRouteNode(entry: NavigationEntry) {
  return { route: entry.route ? canonicalAppPath(entry.appId, entry.route) : '',
    children: entry.children?.map(child => ({ ...child, route: canonicalAppPath(entry.appId, child.route) })) }
}
