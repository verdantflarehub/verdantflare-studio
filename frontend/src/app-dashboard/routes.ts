import { assertAppPath } from './sdk.ts'

export const installationPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
export const appIdPattern = /^(?![0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$)[a-z0-9][a-z0-9-]{0,63}$/
export function instanceWorkspaceLocation(installationId: string, workspaceSessionId: string) {
  if (!installationPattern.test(installationId) || !/^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(workspaceSessionId)) throw new Error('INVALID_WORKSPACE')
  return `/apps/${installationId}/instances/${workspaceSessionId}`
}
export function dashboardLocation(appId: string, path = '/') {
  if (!appIdPattern.test(appId)) throw new Error('INVALID_APP_ID')
  assertAppPath(path)
  return { path: `/apps/${appId}/`, query: path === '/' ? {} : { path } }
}
export function dashboardRoutePath(value: unknown): string {
  if (value === undefined) return '/'
  if (typeof value !== 'string') throw new Error('INVALID_PATH')
  assertAppPath(value)
  return value
}

/** Project owns its complete category → object trail on the browser route. */
export function dashboardOwnsBreadcrumb(appId: string | undefined, value: unknown): boolean {
  if (appId !== 'project') return false
  try { return new URL(dashboardRoutePath(value), 'https://app.invalid').pathname === '/projects' }
  catch { return false }
}
