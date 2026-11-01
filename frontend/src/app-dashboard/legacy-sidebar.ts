// Explicit legacy-mode data only; never used as a fallback for failed dynamic projections.
export const legacySidebarEntries = [
  { title: '项目管理', icon: 'folder', path: '/projects', query: {} },
  { title: '数字世界', icon: 'globe', path: '/projects', query: { mode: 'world' } },
  { title: '资源管理', icon: 'server', path: '/resources', query: {} },
  { title: '网络出口', icon: 'network', path: '/station/proxies', query: {} },
]
export function legacySidebarSelected(item: typeof legacySidebarEntries[number], route: { path: string; query: Record<string, unknown> }) {
  return route.path === item.path && (route.query.mode === 'world') === (item.query.mode === 'world')
}
