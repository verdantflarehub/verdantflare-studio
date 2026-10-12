import { AppCenterError } from './controller.ts'
import type { AppCenterTransport } from './controller'
import { responseClock } from '../pages/response-clock.js'

/** Wails' in-process asset handler forwards this same fixed path to Studio. */
export function createAppCenterTransport(fetcher: typeof fetch = fetch, monotonic = () => performance.now()): AppCenterTransport {
  const clocks = new WeakMap<object, () => number>()
  return { clock: value => value && typeof value === 'object' ? clocks.get(value) : undefined, async read(signal) {
    const started = monotonic()
    const response = await fetcher('/studio/appcenter/v1/view', { method: 'GET', credentials: 'same-origin', cache: 'no-store', redirect: 'error', signal })
    if (response.status === 401 || response.status === 403) throw new AppCenterError(response.status, response.status === 401 ? 'UNAUTHENTICATED' : 'PERMISSION_DENIED')
    if (!response.body || response.headers.get('Content-Type')?.split(';')[0].trim() !== 'application/json') throw new AppCenterError(502, 'APPCENTER_INVALID_VIEW')
    const reader = response.body.getReader(), decoder = new TextDecoder('utf-8', { fatal: true })
    let size = 0, text = ''
    try {
      for (;;) {
        const part = await reader.read(); if (part.done) break
        size += part.value.byteLength; if (size > 4 << 20) throw new AppCenterError(502, 'APPCENTER_LIMIT_EXCEEDED')
        text += decoder.decode(part.value, { stream: true })
      }
      text += decoder.decode()
      const value: unknown = JSON.parse(text)
      if (!response.ok) {
        const code = value && typeof value === 'object' && 'code' in value && typeof value.code === 'string' && /^[A-Z][A-Z0-9_]{0,127}$/.test(value.code) ? value.code : 'APPCENTER_DEPENDENCY_UNAVAILABLE'
        throw new AppCenterError(response.status, code)
      }
      if (!value || typeof value !== 'object' || !('requestId' in value) || value.requestId !== response.headers.get('X-Request-ID')) throw new AppCenterError(502, 'APPCENTER_INVALID_VIEW')
      try { clocks.set(value, responseClock(response.headers.get('Date'), started, monotonic)) }
      catch { throw new AppCenterError(502, 'APPCENTER_INVALID_VIEW') }
      return value
    } catch (error) { await reader.cancel().catch(() => undefined); throw error }
    finally { reader.releaseLock() }
  } }
}
