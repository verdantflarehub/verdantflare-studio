import { AppCenterError } from './controller.ts'
import type { AppCenterCard, AppCenterView } from './schema.generated'

export type LegacyAdapterID = 'image-iframe-v1' | 'video-iframe-v1' | 'blender-host-v1'
export interface LegacyTarget { adapterId: LegacyAdapterID; route: string; cardKey?: string }
export interface LegacyOpened { schemaVersion: 1; transport: 'legacy'; adapterId: LegacyAdapterID; bindingId: string; bindingRevision: string; route: string; resourceBase?: string }
const id = /^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$/
const mediaPath = /^\/dashboard(?:\/tasks\/[a-zA-Z0-9_-]+)?(?:#(?:tasks|models|mcp))?$/
const invalid = (): never => { throw new AppCenterError(400, 'INVALID_LEGACY_ROUTE') }

/** Historical query names live only in this finite compatibility adapter. */
export function legacyTarget(route: string): LegacyTarget | null {
  if (route.split(/[?#]/)[0] !== '/market') return null
  if (route.length > 2048 || /[\\\u0000-\u001f\u007f]/.test(route)) return invalid()
  const u = new URL(route, 'https://studio.invalid')
  if (u.origin !== 'https://studio.invalid' || u.pathname !== '/market') return invalid()
  const q = u.searchParams
  if (q.has('comfyui')) throw new AppCenterError(409, 'LEGACY_ENTRY_UNMAPPED')
  if (!['image', 'video', 'blender'].some(key => q.has(key))) return null
  if (u.hash || route.includes('#')) return invalid()
  q.forEach((_value, key) => { if (q.getAll(key).length !== 1) invalid() })
  const cardKey = q.get('legacyCard') ?? undefined
  if (cardKey !== undefined && !id.test(cardKey)) return invalid()
  q.delete('legacyCard')
  const finish = (adapterId: LegacyAdapterID): LegacyTarget => { q.sort(); return { adapterId, route: '/market?' + q.toString(), cardKey } }
  for (const [key, adapter] of [['image', 'image-iframe-v1'], ['video', 'video-iframe-v1']] as const) {
    if (q.has(key)) { if (q.size !== 1 || !mediaPath.test(q.get(key)!)) return invalid(); return finish(adapter) }
  }
  const alias = q.get('blender') ?? ''
  if (!['instances', 'create'].includes(alias) && !/^blender[A-Za-z0-9_-]{1,57}$/.test(alias)) return invalid()
  const pairs: [string, string][] = []; q.forEach((value, key) => pairs.push([key, value]))
  for (const [key, value] of pairs) {
    if (key === 'blender') continue
    if (key === 'panel' && !['instances', 'create'].includes(alias) && ['overview', 'resources', 'activity'].includes(value)) continue
    if (key === 'operation' && q.has('panel') && /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(value)) continue
    return invalid()
  }
  return finish('blender-host-v1')
}

export function legacyLocation(opened: Pick<LegacyOpened, 'adapterId' | 'route'>, cardKey: string): string {
  const target = legacyTarget(opened.route)
  if (!target || target.adapterId !== opened.adapterId || !id.test(cardKey)) return invalid()
  return target.route + '&legacyCard=' + encodeURIComponent(cardKey)
}

export function selectLegacyCard(view: AppCenterView, target: LegacyTarget): AppCenterCard {
  const matches = view.items.filter(card => card.entry?.transport === 'legacy' && card.entry.entryId === target.adapterId
    && (!target.cardKey || target.cardKey === card.cardKey))
  if (!matches.length) throw new AppCenterError(409, 'LEGACY_ENTRY_UNMAPPED')
  if (matches.length !== 1) throw new AppCenterError(409, 'LEGACY_ENTRY_AMBIGUOUS')
  return matches[0]
}

const desktopWorkspace = /^\/studio\/legacy-media\/[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
export function createLegacyClient(fetcher: typeof fetch = fetch, desktop = typeof location !== 'undefined' && new URLSearchParams(location.search).get('host') === 'desktop') {
  async function close(opened: Pick<LegacyOpened, 'resourceBase'>): Promise<void> {
    if (!desktop || !opened.resourceBase || !desktopWorkspace.test(opened.resourceBase)) return
    await fetcher(opened.resourceBase, { method: 'DELETE', credentials: 'omit', cache: 'no-store', redirect: 'error', signal: AbortSignal.timeout(5000) }).catch(() => undefined)
  }
  return { close, async open(view: AppCenterView, card: AppCenterCard, signal?: AbortSignal): Promise<LegacyOpened> {
    const entry = card.entry
    if (!entry || entry.transport !== 'legacy') throw new AppCenterError(409, 'APPCENTER_OPEN_UNAVAILABLE')
    if (desktop && !['image-iframe-v1', 'video-iframe-v1'].includes(entry.entryId)) throw new AppCenterError(409, 'HOST_UNSUPPORTED')
    const response = await fetcher(desktop ? '/studio/legacy-media' : '/studio/appcenter/v1/open', { method: 'POST', credentials: desktop ? 'omit' : 'same-origin', cache: 'no-store', redirect: 'error', signal,
      headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ schemaVersion: 1, cardKey: card.cardKey, entryId: entry.entryId,
        expectedViewRevision: view.revision, expectedBindingRevision: card.bindingRevision }) })
    const resourceBase = desktop ? '/studio/legacy-media/' + response.headers.get('X-Studio-Legacy-Workspace') : undefined
    if (!response.body || response.headers.get('Content-Type')?.split(';')[0].trim() !== 'application/json') {
      await close({ resourceBase }); throw new AppCenterError(502, 'APPCENTER_INVALID_OPEN')
    }
    const reader = response.body.getReader(), decoder = new TextDecoder('utf-8', { fatal: true })
    let length = 0, text = ''
    try {
      for (;;) { const chunk = await reader.read(); if (chunk.done) break; length += chunk.value.byteLength
        if (length > 16384) throw new AppCenterError(502, 'APPCENTER_INVALID_OPEN')
        text += decoder.decode(chunk.value, { stream: true }) }
      text += decoder.decode()
      const value = JSON.parse(text)
      if (signal?.aborted) throw new AppCenterError(408, 'REQUEST_CANCELLED')
      if (!response.ok) throw new AppCenterError(response.status, typeof value?.code === 'string' && /^[A-Z][A-Z0-9_]{0,127}$/.test(value.code) ? value.code : 'APPCENTER_OPEN_UNAVAILABLE')
      if (desktop && (!resourceBase || !desktopWorkspace.test(resourceBase))) throw new AppCenterError(409, 'HOST_UNSUPPORTED')
      if (!value || Object.keys(value).sort().join(',') !== 'adapterId,bindingId,bindingRevision,route,schemaVersion,transport'
        || value.schemaVersion !== 1 || value.transport !== 'legacy' || value.adapterId !== entry.entryId
        || typeof value.bindingId !== 'string' || !id.test(value.bindingId) || value.bindingRevision !== card.bindingRevision
        || typeof value.route !== 'string') throw new AppCenterError(502, 'APPCENTER_INVALID_OPEN')
      const target = legacyTarget(value.route)
      if (!target || target.adapterId !== value.adapterId || target.cardKey || target.route !== value.route) throw new AppCenterError(502, 'APPCENTER_INVALID_OPEN')
      return desktop ? { ...value, resourceBase } : value as LegacyOpened
    } catch (error) { await reader.cancel().catch(() => undefined); await close({ resourceBase }); throw error }
    finally { reader.releaseLock() }
  } }
}
