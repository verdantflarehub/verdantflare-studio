export type DirectoryView = 'installed' | 'all'

/** The legacy /market bookmark defaults to My Apps. */
export function directoryView(value: unknown): DirectoryView {
  return value === 'all' ? 'all' : 'installed'
}

export function directoryLocation(view: DirectoryView) {
  return { path: '/market', query: { view } }
}

/** /market also accepts old application bookmarks; they are not directory pages. */
export function isDirectoryRoute(route: { path: string; query: Record<string, unknown> }) {
  return route.path === '/market' && !['video', 'image', 'music', 'speech', 'blender', 'comfyui'].some(key => route.query[key] !== undefined)
}
