export interface HostIdentity {
  user_id?: string
  username?: string
  organization_id?: string
  organization_name?: string
  station_id?: string
  station_name?: string
  session_id?: string
  policy_version?: number
  revocation_version?: number
  roles?: string[]
  scopes?: string[]
}
export type SessionRead = { status: 'authenticated'; identity: HostIdentity } | { status: 'anonymous' | 'error' | 'superseded' }

/** Retry only the idempotent identity read; denied sessions need a real login. */
export async function readHostSession(read: () => Promise<Response>, current: () => boolean): Promise<SessionRead> {
  for (let attempt = 0; attempt < 3; attempt++) {
    try {
      const response = await read()
      if (!current()) return { status: 'superseded' }
      if (response.status === 401 || response.status === 403) return { status: 'anonymous' }
      if (response.status === 200) {
        const value: unknown = await response.json()
        if (!current()) return { status: 'superseded' }
        if (value && typeof value === 'object' && 'user_id' in value && typeof value.user_id === 'string' && value.user_id) {
          return { status: 'authenticated', identity: { ...value, user_id: value.user_id } }
        }
      } else if (response.status < 500) return { status: 'error' }
    } catch { if (!current()) return { status: 'superseded' } }
  }
  return { status: 'error' }
}
