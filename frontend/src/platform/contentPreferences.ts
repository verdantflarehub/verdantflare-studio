export const NSFW_PREF_STORAGE_KEY = 'vf.studio.nsfw_display.v1'
export const NSFW_MAX_TTL_MS = 24 * 60 * 60 * 1000
export const NSFW_SOON_THRESHOLD_MS = 30 * 60 * 1000

export interface NsfwPreferenceRecord {
  enabled_at: string
  expires_at: string
  user_id: string
  organization_id: string
}

export interface SessionIdentityScope {
  userId: string
  organizationId: string
}

export interface ContentPreferencesState {
  readonly includeNsfw: boolean
  readonly enabledAt: string | null
  readonly expiresAt: string | null
  readonly remainingMs: number
  readonly expiringSoon: boolean
  readonly storageAvailable: boolean
}

export type DisableReason = 'manual' | 'expired' | 'logout' | 'identity-change' | 'storage'

export type ContentPreferencesListener = (
  state: Readonly<ContentPreferencesState>,
  reason?: DisableReason | 'enabled',
) => void

export interface StorageLike {
  getItem(key: string): string | null
  setItem(key: string, value: string): void
  removeItem(key: string): void
}

const pad2 = (n: number): string => String(n).padStart(2, '0')

export function formatRemainingTime(ms: number): string {
  if (!Number.isFinite(ms) || ms <= 0) return '已关闭'
  if (ms < 60_000) return '即将关闭'
  const minutes = Math.floor(ms / 60_000)
  if (minutes < 60) return `剩余 ${minutes} 分钟`
  return `剩余 ${Math.floor(minutes / 60)} 小时 ${minutes % 60} 分`
}

export function formatExpiresAt(isoOrMs: string | number): string {
  const d = new Date(isoOrMs)
  if (Number.isNaN(d.getTime())) return ''
  return `${pad2(d.getMonth() + 1)}-${pad2(d.getDate())} ${pad2(d.getHours())}:${pad2(d.getMinutes())}`
}

export function formatClockTime(isoOrMs: string | number): string {
  const d = new Date(isoOrMs)
  if (Number.isNaN(d.getTime())) return ''
  return `${pad2(d.getHours())}:${pad2(d.getMinutes())}`
}

export function probeStorageAvailable(storage: StorageLike | null | undefined): storage is StorageLike {
  if (!storage) return false
  const probeKey = '__vf_studio_storage_probe__'
  try {
    storage.setItem(probeKey, '1')
    const ok = storage.getItem(probeKey) === '1'
    storage.removeItem(probeKey)
    return ok
  } catch {
    return false
  }
}

export interface ContentPreferencesControllerOptions {
  storage?: StorageLike | null
  now?: () => number
  onPostMessageSync?: (payload: { includeNsfw: boolean; expiresAt: string | null }) => void
}

export function createContentPreferencesController(options: ContentPreferencesControllerOptions = {}) {
  const now = options.now ?? Date.now
  const storage = options.storage !== undefined
    ? options.storage
    : (typeof globalThis !== 'undefined' && 'localStorage' in globalThis ? globalThis.localStorage : null)

  let session: SessionIdentityScope | null = null
  const listeners = new Set<ContentPreferencesListener>()
  let expiryTimer: ReturnType<typeof setTimeout> | null = null
  let lastIncludeNsfw = false

  function isStorageAvailable(): boolean {
    return probeStorageAvailable(storage)
  }

  function removeStoredRecord(): void {
    if (!storage) return
    try {
      storage.removeItem(NSFW_PREF_STORAGE_KEY)
    } catch {
      // Ignore storage removal errors
    }
  }

  function clearExpiryTimer(): void {
    if (expiryTimer !== null) {
      clearTimeout(expiryTimer)
      expiryTimer = null
    }
  }

  function readValidRecord(): NsfwPreferenceRecord | null {
    if (!isStorageAvailable() || !storage || !session || !session.userId || !session.organizationId) {
      return null
    }
    let raw: string | null = null
    try {
      raw = storage.getItem(NSFW_PREF_STORAGE_KEY)
    } catch {
      return null
    }
    if (!raw) return null
    let parsed: unknown
    try {
      parsed = JSON.parse(raw)
    } catch {
      removeStoredRecord()
      return null
    }
    if (!parsed || typeof parsed !== 'object' || Array.isArray(parsed)) {
      removeStoredRecord()
      return null
    }
    const rec = parsed as Record<string, unknown>
    if (
      typeof rec.enabled_at !== 'string' ||
      typeof rec.expires_at !== 'string' ||
      typeof rec.user_id !== 'string' ||
      typeof rec.organization_id !== 'string'
    ) {
      removeStoredRecord()
      return null
    }
    const enabledMs = Date.parse(rec.enabled_at)
    const expiresMs = Date.parse(rec.expires_at)
    const currentMs = now()
    if (
      !Number.isFinite(enabledMs) ||
      !Number.isFinite(expiresMs) ||
      expiresMs <= enabledMs ||
      expiresMs - enabledMs > NSFW_MAX_TTL_MS ||
      rec.user_id !== session.userId ||
      rec.organization_id !== session.organizationId ||
      currentMs >= expiresMs
    ) {
      removeStoredRecord()
      return null
    }
    return {
      enabled_at: rec.enabled_at,
      expires_at: rec.expires_at,
      user_id: rec.user_id,
      organization_id: rec.organization_id,
    }
  }

  function scheduleExpiry(expiresAtIso: string): void {
    clearExpiryTimer()
    const expiresMs = Date.parse(expiresAtIso)
    if (!Number.isFinite(expiresMs)) return
    const delay = Math.min(Math.max(expiresMs - now(), 1), 2_147_483_647)
    expiryTimer = setTimeout(() => {
      if (now() >= expiresMs) {
        disable('expired')
      } else {
        scheduleExpiry(expiresAtIso)
      }
    }, delay)
  }

  function buildSnapshot(): Readonly<ContentPreferencesState> {
    const storageAvailable = isStorageAvailable()
    const rec = readValidRecord()
    if (!rec) {
      clearExpiryTimer()
      return Object.freeze({
        includeNsfw: false,
        enabledAt: null,
        expiresAt: null,
        remainingMs: 0,
        expiringSoon: false,
        storageAvailable,
      })
    }
    const remainingMs = Math.max(0, Date.parse(rec.expires_at) - now())
    scheduleExpiry(rec.expires_at)
    return Object.freeze({
      includeNsfw: true,
      enabledAt: rec.enabled_at,
      expiresAt: rec.expires_at,
      remainingMs,
      expiringSoon: remainingMs > 0 && remainingMs < NSFW_SOON_THRESHOLD_MS,
      storageAvailable,
    })
  }

  function notify(reason?: DisableReason | 'enabled'): Readonly<ContentPreferencesState> {
    const snap = buildSnapshot()
    lastIncludeNsfw = snap.includeNsfw
    options.onPostMessageSync?.({ includeNsfw: snap.includeNsfw, expiresAt: snap.expiresAt })
    for (const listener of [...listeners]) {
      try {
        listener(snap, reason)
      } catch {
        // Listener errors must not break notification chain
      }
    }
    return snap
  }

  function setSessionIdentity(next: SessionIdentityScope | null): Readonly<ContentPreferencesState> {
    if (!next || !next.userId || !next.organizationId) {
      const hadSession = session !== null
      session = null
      removeStoredRecord()
      clearExpiryTimer()
      return notify(hadSession ? 'logout' : undefined)
    }
    const changed =
      session !== null &&
      (session.userId !== next.userId || session.organizationId !== next.organizationId)
    session = { userId: next.userId, organizationId: next.organizationId }
    if (changed) {
      removeStoredRecord()
      clearExpiryTimer()
      return notify('identity-change')
    }
    // Also validate existing stored record against the newly set session identity
    const snap = buildSnapshot()
    if (snap.includeNsfw !== lastIncludeNsfw) {
      return notify(snap.includeNsfw ? 'enabled' : 'identity-change')
    }
    return snap
  }

  function enable(): Readonly<ContentPreferencesState> {
    if (!isStorageAvailable() || !storage) {
      throw new Error('STORAGE_UNAVAILABLE')
    }
    if (!session || !session.userId || !session.organizationId) {
      throw new Error('UNAUTHENTICATED')
    }
    // Cannot extend an already active 24h window without closing first
    const existing = readValidRecord()
    if (existing) {
      return buildSnapshot()
    }
    const currentMs = now()
    const expiresMs = currentMs + NSFW_MAX_TTL_MS
    const record: NsfwPreferenceRecord = {
      enabled_at: new Date(currentMs).toISOString(),
      expires_at: new Date(expiresMs).toISOString(),
      user_id: session.userId,
      organization_id: session.organizationId,
    }
    storage.setItem(NSFW_PREF_STORAGE_KEY, JSON.stringify(record))
    return notify('enabled')
  }

  function disable(reason: DisableReason = 'manual'): Readonly<ContentPreferencesState> {
    removeStoredRecord()
    clearExpiryTimer()
    return notify(reason)
  }

  function checkAndSync(reason: DisableReason = 'expired'): Readonly<ContentPreferencesState> {
    const snap = buildSnapshot()
    if (snap.includeNsfw !== lastIncludeNsfw) {
      return notify(snap.includeNsfw ? 'enabled' : reason)
    }
    return snap
  }

  function subscribe(listener: ContentPreferencesListener): () => void {
    if (typeof listener !== 'function') {
      throw new TypeError('INVALID_LISTENER')
    }
    listeners.add(listener)
    return () => {
      listeners.delete(listener)
    }
  }

  function dispose(): void {
    clearExpiryTimer()
    listeners.clear()
  }

  return Object.freeze({
    get: buildSnapshot,
    enable,
    disable,
    setSessionIdentity,
    checkAndSync,
    subscribe,
    dispose,
  })
}

export type ContentRatingDomain = 'image' | 'speech' | 'music' | 'video'

export async function submitContentRatingAction(
  domain: ContentRatingDomain,
  versionId: string,
  action: 'clear' | 'mark',
  options: { reason?: string; projectId?: string; fetcher?: typeof fetch } = {},
): Promise<{ status: number; data: unknown }> {
  if (!['image', 'speech', 'music', 'video'].includes(domain)) {
    throw new Error('INVALID_DOMAIN')
  }
  if (action !== 'clear' && action !== 'mark') {
    throw new Error('INVALID_ACTION')
  }
  if (typeof versionId !== 'string' || !/^[a-zA-Z0-9][a-zA-Z0-9._:-]{0,127}$/.test(versionId)) {
    throw new Error('INVALID_VERSION_ID')
  }
  const reason = options.reason?.trim() ?? ''
  if (reason.length > 200) {
    throw new Error('REASON_TOO_LONG')
  }
  const payload: Record<string, string> = {}
  if (reason) payload.reason = reason
  if (options.projectId) payload.project_id = options.projectId
  const fetcher = options.fetcher ?? fetch
  const response = await fetcher(`/api/v1/content-ratings/${domain}/${versionId}/${action}`, {
    method: 'POST',
    credentials: 'same-origin',
    cache: 'no-store',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  let data: unknown = null
  try {
    data = await response.json()
  } catch {
    data = null
  }
  return { status: response.status, data }
}

function broadcastNsfwPreferenceToFrames(payload: { includeNsfw: boolean; expiresAt: string | null }): void {
  if (typeof document === 'undefined') return
  const message = {
    channel: 'vf-studio',
    type: 'contentPreferences',
    includeNsfw: payload.includeNsfw,
    expiresAt: payload.expiresAt,
  }
  for (const frameId of ['imageFrame', 'videoFrame']) {
    const frame = document.getElementById(frameId) as HTMLIFrameElement | null
    try {
      frame?.contentWindow?.postMessage(message, '*')
    } catch {
      // Ignore cross-frame postMessage errors
    }
  }
}

export const hostContentPreferences = createContentPreferencesController({
  onPostMessageSync: broadcastNsfwPreferenceToFrames,
})

if (typeof window !== 'undefined') {
  window.addEventListener('storage', (event: StorageEvent) => {
    if (event.key === NSFW_PREF_STORAGE_KEY || event.key === null) {
      hostContentPreferences.checkAndSync('storage')
    }
  })
  window.addEventListener('focus', () => {
    hostContentPreferences.checkAndSync('expired')
  })
  window.addEventListener('studio:logout', () => {
    hostContentPreferences.setSessionIdentity(null)
  })
  if (typeof document !== 'undefined') {
    document.addEventListener('visibilitychange', () => {
      if (!document.hidden) {
        hostContentPreferences.checkAndSync('expired')
      }
    })
  }
}
