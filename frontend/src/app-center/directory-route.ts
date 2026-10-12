export type DirectoryView = 'installed' | 'all'

/** The legacy /market bookmark defaults to My Apps. */
export function directoryView(value: unknown): DirectoryView {
  return value === 'all' ? 'all' : 'installed'
}

export function directoryLocation(view: DirectoryView) {
  return { path: '/market', query: { view } }
}
