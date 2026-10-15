import { legacyAppLocation, legacyTarget, selectLegacyCard } from '../app-center/legacy.ts'
import type { AppCenterView } from '../app-center/schema.generated'
import type { AppTarget } from './app-target'

/** The only non-Dashboard targets accepted by openApp are existing media task pages. */
export function nativeTaskTarget(target: AppTarget) {
  const media = (target.appId === 'image-mcp' || target.appId === 'image-mcp-server')
    ? 'image'
    : (target.appId === 'video-mcp' || target.appId === 'video-mcp-server')
      ? 'video'
      : null
  if (!media) return null
  if (!/^\/dashboard\/tasks\/[a-zA-Z0-9_-]{1,256}$/.test(target.path)) throw new Error('INVALID_NAVIGATION')
  return legacyTarget('/market?' + new URLSearchParams({ [media]: target.path }))!
}

/** Caller supplies a freshly decoded, identity-bound snapshot and its server clock. */
export function nativeTaskLocation(target: AppTarget, view: AppCenterView, now: number, authorizationEpoch: string) {
  const legacy = nativeTaskTarget(target)
  if (!legacy) throw new Error('INVALID_NAVIGATION')
  if (!Number.isFinite(now) || Date.parse(view.validUntil) <= now) throw new Error('APPCENTER_VIEW_EXPIRED')
  if (view.scope.authorizationEpoch !== authorizationEpoch) throw new Error('CONTEXT_CHANGED')
  const card = selectLegacyCard(view, legacy)
  if (card.appId !== target.appId || card.entry?.quality !== 'known' || card.entry.availability !== 'available'
    || !card.actions.some(action => action.kind === 'open' && action.enabled && action.entryId === legacy.adapterId)) throw new Error('APPCENTER_OPEN_UNAVAILABLE')
  return legacyAppLocation(legacy, card.cardKey)
}
