import { AppCenterError } from './controller.ts'
import { dashboardLocation } from '../app-dashboard/routes.ts'
import type { AppCenterCard, AppCenterView } from './schema.generated'

export interface ComfyUIBookmark { alias: string }
const fail = (code: string): never => { throw new AppCenterError(409, code) }

/** Only this compatibility adapter understands historical ComfyUI query names. */
export function comfyuiBookmark(route: string): ComfyUIBookmark | null {
  if (route.split(/[?#]/)[0] !== '/market') return null
  const url = new URL(route, 'https://studio.invalid')
  if (!url.searchParams.has('comfyui')) return null
  const query = url.searchParams, alias = query.get('comfyui') ?? ''
  if (route.length > 2048 || /[\\\u0000-\u001f\u007f]/.test(route) || route.includes('#')
    || url.origin !== 'https://studio.invalid' || url.pathname !== '/market'
    || query.size !== 1 || alias !== 'instances' && !/^comfyui[A-Za-z0-9]{1,57}$/.test(alias)) {
    throw new AppCenterError(400, 'INVALID_LEGACY_ROUTE')
  }
  return { alias }
}

export function comfyuiManagementCard(view: AppCenterView): AppCenterCard {
  const matches = view.items.filter(card => card.appId === 'comfyui' && card.installation?.state === 'installed')
  if (matches.length > 1) return fail('LEGACY_ENTRY_AMBIGUOUS')
  const card = matches[0]
  if (!card) return fail('APPCENTER_OPEN_UNAVAILABLE')
  const open = card.actions.find(action => action.kind === 'open')
  if (!open?.enabled) return fail(open?.reasonCode ?? 'APPCENTER_OPEN_UNAVAILABLE')
  if (card.installationQuality !== 'known' || card.entry?.transport !== 'dashboard' || open.entryId !== card.entry.entryId) {
    return fail('APPCENTER_OPEN_UNAVAILABLE')
  }
  return card
}

export function comfyuiManagementLocation(card: AppCenterCard) {
  if (!card.installation) return fail('APPCENTER_OPEN_UNAVAILABLE')
  return dashboardLocation(card.appId)
}

/** The old directory owns old aliases; a new manager installation is irrelevant. */
export function comfyuiBookmarkInstance(value: unknown, alias: string): { alias: string; name: string } {
  const body = value as { schema_version?: unknown; instances?: unknown }
  if (!body || body.schema_version !== 1 || !Array.isArray(body.instances) || body.instances.length > 1000) {
    return fail('APPCENTER_INVALID_VIEW')
  }
  const matches = body.instances.filter(item => item && item.alias === alias)
  if (matches.length > 1) return fail('LEGACY_ENTRY_AMBIGUOUS')
  const item = matches[0]
  if (!item) return fail('INSTANCE_NOT_FOUND')
  if (typeof item.name !== 'string' || !Array.isArray(item.allowed_actions)) return fail('APPCENTER_INVALID_VIEW')
  if (!Number.isFinite(Date.parse(item.observed_at)) || item.status !== 'running'
    || item.editor_state !== 'ready' || !item.allowed_actions.includes('open')) {
    return fail('INSTANCE_UNAVAILABLE')
  }
  return { alias, name: item.name }
}

export async function readComfyUIBookmark(alias: string, signal: AbortSignal, fetcher: typeof fetch = fetch) {
  if (!/^comfyui[A-Za-z0-9]{1,57}$/.test(alias)) throw new AppCenterError(400, 'INVALID_LEGACY_ROUTE')
  const response = await fetcher('/studio/apps/comfyui/instances', {
    credentials: 'same-origin', cache: 'no-store', redirect: 'error', signal,
  })
  if (!response.ok) return fail(response.status === 401 ? 'UNAUTHENTICATED' : response.status === 403
    ? 'PERMISSION_DENIED' : 'APPCENTER_DEPENDENCY_UNAVAILABLE')
  if (response.headers.get('Content-Type')?.split(';')[0].trim() !== 'application/json' || !response.body) {
    return fail('APPCENTER_INVALID_VIEW')
  }
  const reader = response.body.getReader(), decoder = new TextDecoder('utf-8', { fatal: true })
  let text = '', size = 0
  try {
    for (;;) {
      const chunk = await reader.read()
      if (chunk.done) break
      size += chunk.value.byteLength
      if (size > 1 << 20) return fail('APPCENTER_INVALID_VIEW')
      text += decoder.decode(chunk.value, { stream: true })
    }
    text += decoder.decode()
    if (signal.aborted) return fail('REQUEST_CANCELLED')
    return comfyuiBookmarkInstance(JSON.parse(text), alias)
  } finally { await reader.cancel().catch(() => undefined); reader.releaseLock() }
}
