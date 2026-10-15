import type { NavigationPreference, NavigationTransport } from './controller'

/** Web and Wails local asset requests use the same host path. Go owns desktop cookies. */
export function createNavigationTransport(fetcher: typeof fetch = fetch): NavigationTransport {
  async function call(path: string, body: NavigationPreference | undefined, signal: AbortSignal) {
    const response = await fetcher(path, { method: body ? 'POST' : 'GET', credentials: 'same-origin', cache: 'no-store', redirect: 'error', signal,
      headers: body ? { 'Content-Type': 'application/json' } : {}, body: body ? JSON.stringify(body) : undefined })
    if (response.status === 401 || response.status === 403) { await response.body?.cancel(); throw new Error('NAVIGATION_DENIED') }
    if (!response.body || response.headers.get('Content-Type')?.split(';')[0].trim() !== 'application/json') throw new Error('NAVIGATION_UNAVAILABLE')
    const reader = response.body.getReader(), decoder = new TextDecoder('utf-8', { fatal: true })
    let length = 0, text = ''
    try {
      for (;;) { const part = await reader.read(); if (part.done) break; length += part.value.byteLength; if (length > 2 << 20) throw new Error('NAVIGATION_UNAVAILABLE'); text += decoder.decode(part.value, { stream: true }) }
      text += decoder.decode()
      const value: unknown = JSON.parse(text)
      if (!response.ok) {
        if (response.status === 409) throw new Error('NAVIGATION_PREFERENCE_CONFLICT')
        throw new Error('NAVIGATION_UNAVAILABLE')
      }
      return value
    } catch (error) { await reader.cancel().catch(() => undefined); throw error }
    finally { reader.releaseLock() }
  }
  return { read: signal => call('/studio/navigation', undefined, signal), write: (body, signal) => call('/studio/navigation/preferences', body, signal) }
}
