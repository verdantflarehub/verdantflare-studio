import { assertAppPath } from './sdk.ts'

export const installationPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
export function instanceWorkspaceLocation(installationId: string, workspaceSessionId: string) {
  if (!installationPattern.test(installationId) || !/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(workspaceSessionId)) throw new Error('INVALID_WORKSPACE')
  return `/apps/${installationId}/instances/${workspaceSessionId}`
}
export function dashboardLocation(installationId: string, path = '/') {
  if (!installationPattern.test(installationId)) throw new Error('INVALID_INSTALLATION')
  assertAppPath(path)
  return { path: `/apps/${installationId}/dashboard`, query: path === '/' ? {} : { path } }
}
export function dashboardRoutePath(value: unknown): string {
  if (value === undefined) return '/'
  if (typeof value !== 'string') throw new Error('INVALID_PATH')
  assertAppPath(value)
  return value
}
