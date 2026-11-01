import { sameNavigationPath, selectedNavigationChild } from './controller.ts'

export interface MenuNode { route: string; children?: { entryId: string; title: string; route: string; order: number }[] }

/** Only node capabilities determine interaction; application identity is irrelevant. */
export function menuState(node: MenuNode, currentPath: string, selected: boolean) {
  const hasChildren = !!node.children?.length
  const navigable = !!node.route && (!hasChildren || !node.children!.some(child => sameNavigationPath(child.route, node.route)))
  const selectedChild = selected ? selectedNavigationChild(node.children, currentPath)?.entryId : undefined
  const parent = { entryId: '', title: '', route: node.route, order: 0 }
  const parentCurrent = selected && navigable && !selectedChild && !!selectedNavigationChild([parent], currentPath)
  return { hasChildren, navigable, selectedChild, parentCurrent }
}
