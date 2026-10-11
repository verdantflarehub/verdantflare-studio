export interface AdoptionEvidence { expectedWorkloadUid: string; templateDigest: string; managementStatus: 'managed' | 'unmanaged'; observedAt: string }
export interface PreinstalledItem { appId: string; state: string; code: string; operationId: string; installationId: string; adoptionEvidence?: AdoptionEvidence }
export interface PreinstalledStatus { enabled: boolean; configDigest: string; items: PreinstalledItem[] }
export interface PreinstalledIntent { intentId: string; retry: string[]; reinstall: string[]; adopt: Record<string, string>; adoptTemplateDigests: Record<string, string> }
const states = new Set(['pending', 'waiting', 'blocked', 'ready', 'upgrade_required', 'suppressed', 'unknown', 'failed', 'accepted', 'running', 'confirming', 'adoption_required'])
const digest = /^sha256:[a-f0-9]{64}$/
const id = /^[a-z0-9][a-z0-9._-]{0,62}$/
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const object = (v: unknown): v is Record<string, unknown> => v !== null && typeof v === 'object' && !Array.isArray(v)
const invalid = (): never => { throw new Error('PREINSTALLED_INVALID_RESPONSE') }
export function decodePreinstalled(value: unknown): PreinstalledStatus {
  if (!object(value) || typeof value.enabled !== 'boolean' || typeof value.configDigest !== 'string' || !Array.isArray(value.items) || value.items.length > 128
    || value.enabled && !digest.test(value.configDigest) || !value.enabled && (value.configDigest !== '' || value.items.length !== 0)) return invalid()
  const seen = new Set<string>()
  for (const item of value.items) {
    if (!object(item) || typeof item.appId !== 'string' || !id.test(item.appId) || seen.has(item.appId) || !states.has(String(item.state))
      || typeof item.code !== 'string' || typeof item.operationId !== 'string' || item.operationId !== '' && !uuid.test(item.operationId)
      || typeof item.installationId !== 'string' || item.installationId !== '' && !uuid.test(item.installationId)) return invalid()
    seen.add(item.appId)
    if (item.adoptionEvidence != null) {
      const e = item.adoptionEvidence
      if (!object(e) || typeof e.expectedWorkloadUid !== 'string' || !uuid.test(e.expectedWorkloadUid) || typeof e.templateDigest !== 'string'
        || !digest.test(e.templateDigest) || typeof e.managementStatus !== 'string' || !['managed', 'unmanaged'].includes(e.managementStatus)
        || typeof e.observedAt !== 'string' || !Number.isFinite(Date.parse(e.observedAt))) return invalid()
    }
  }
  return structuredClone(value) as unknown as PreinstalledStatus
}

export function decodePreinstalledIntent(value: unknown): PreinstalledIntent {
  if (!object(value) || Object.keys(value).sort().join(',') !== 'adopt,adoptTemplateDigests,intentId,reinstall,retry'
    || typeof value.intentId !== 'string' || !uuid.test(value.intentId) || !object(value.adopt) || !object(value.adoptTemplateDigests)) return invalid()
  const seen = new Set<string>()
  for (const list of [value.retry, value.reinstall, Object.keys(value.adopt)]) {
    if (!Array.isArray(list) || list.length > 128) return invalid()
    for (const app of list) { if (typeof app !== 'string' || !id.test(app) || seen.has(app)) return invalid(); seen.add(app) }
  }
  if (Object.keys(value.adopt).sort().join(',') !== Object.keys(value.adoptTemplateDigests).sort().join(',')) return invalid()
  for (const [app, uid] of Object.entries(value.adopt)) {
    if (typeof uid !== 'string' || !uuid.test(uid) || typeof value.adoptTemplateDigests[app] !== 'string' || !digest.test(value.adoptTemplateDigests[app])) return invalid()
  }
  if (JSON.stringify(value).length > 4096) return invalid()
  return structuredClone(value) as unknown as PreinstalledIntent
}

export function preinstalledIntent(kind: 'fill' | 'retry' | 'reinstall' | 'adopt', item?: PreinstalledItem): PreinstalledIntent {
  if (kind !== 'fill' && (!item || kind === 'adopt' && !item.adoptionEvidence)) return invalid()
  return decodePreinstalledIntent({ intentId: crypto.randomUUID(), retry: kind === 'retry' ? [item!.appId] : [],
    reinstall: kind === 'reinstall' ? [item!.appId] : [],
    adopt: kind === 'adopt' ? { [item!.appId]: item!.adoptionEvidence!.expectedWorkloadUid } : {},
    adoptTemplateDigests: kind === 'adopt' ? { [item!.appId]: item!.adoptionEvidence!.templateDigest } : {} })
}

/** Store only explicit, unresolved intents. Reading never submits an operation. */
export function createPreinstalledIntentStore(storage: Pick<Storage, 'getItem' | 'setItem' | 'removeItem'>) {
  const key = (scope: string) => {
    const ids: unknown = JSON.parse(scope)
    if (!Array.isArray(ids) || ids.length !== 3 || ids.some(value => typeof value !== 'string' || !uuid.test(value))) return invalid()
    return `studio.preinstalled.intent:${scope}`
  }
  function read(scope: string): PreinstalledIntent | null {
    const value = storage.getItem(key(scope))
    if (value === null) return null
    if (value.length > 4096) return invalid()
    return decodePreinstalledIntent(JSON.parse(value))
  }
  return { read,
    save(scope: string, intent: PreinstalledIntent) {
      const value = decodePreinstalledIntent(intent), existing = read(scope)
      if (existing && JSON.stringify(existing) !== JSON.stringify(value)) throw new Error('PREINSTALLED_INTENT_UNRESOLVED')
      storage.setItem(key(scope), JSON.stringify(value))
    },
    clear(scope: string, intentId: string) { if (read(scope)?.intentId === intentId) storage.removeItem(key(scope)) },
  }
}

export function createPreinstalledClient(fetcher: typeof fetch = fetch) {
  async function call(intent?: PreinstalledIntent, signal?: AbortSignal) {
    if (intent) intent = decodePreinstalledIntent(intent)
    const payload = intent && { ...intent, adoptTemplateDigests: Object.keys(intent.adopt).length ? intent.adoptTemplateDigests : undefined }
    const response = await fetcher(intent ? '/studio/preinstalled/reconcile' : '/studio/preinstalled', {
      method: intent ? 'POST' : 'GET', credentials: 'same-origin', cache: 'no-store', redirect: 'error', signal,
      headers: intent ? { 'Content-Type': 'application/json' } : {}, body: payload ? JSON.stringify(payload) : undefined,
    })
    if (!response.ok) throw new Error(response.status === 403 ? 'PREINSTALLED_DENIED' : 'PREINSTALLED_UNAVAILABLE')
    if (response.headers.get('Content-Type')?.split(';')[0].trim() !== 'application/json' || !response.body) return invalid()
    const reader = response.body.getReader(), chunks: Uint8Array[] = []
    let size = 0
    try {
      for (;;) {
        const { value, done } = await reader.read(); if (done) break
        size += value.length; if (size > 128 << 10) { await reader.cancel(); return invalid() }
        chunks.push(value)
      }
    } finally { reader.releaseLock() }
    const bytes = new Uint8Array(size); let offset = 0
    for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.length }
    return decodePreinstalled(JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes)))
  }
  return { read: (signal?: AbortSignal) => call(undefined, signal), execute: (intent: PreinstalledIntent, signal?: AbortSignal) => call(intent, signal) }
}
