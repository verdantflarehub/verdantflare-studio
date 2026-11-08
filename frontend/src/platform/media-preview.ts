export interface MediaGallery { id: string; index: number; count: number }
export interface MediaPreviewInput { bytes: ArrayBuffer; mime: string; title: string; gallery?: MediaGallery }
export interface MediaPreview { owner: object; url: string; title: string; closed?: () => void; gallery?: MediaGallery; navigate?: (index: number) => Promise<void>; loading?: boolean; error?: string }
const formats = new Set(['image/png', 'image/jpeg', 'image/webp', 'image/gif'])
export function validateMediaPreview(input: MediaPreviewInput): void {
  if (input?.gallery) {
    const g = input.gallery
    if (!/^[A-Za-z0-9_-]{1,64}$/.test(g.id) || !Number.isInteger(g.count) || g.count < 1 || g.count > 32 || !Number.isInteger(g.index) || g.index < 0 || g.index >= g.count) throw new Error('INVALID_MEDIA_PREVIEW')
  }
  // ArrayBuffers may originate in a trusted Dashboard's separate realm.
  if (!input || Object.prototype.toString.call(input.bytes) !== '[object ArrayBuffer]'
    || input.bytes.byteLength < 1 || input.bytes.byteLength > 128 * 1024 * 1024
    || !formats.has(input.mime) || typeof input.title !== 'string' || input.title.length > 256) throw new Error('INVALID_MEDIA_PREVIEW')
}
let current: MediaPreview | null = null
let pending: { owner: object; controller: AbortController } | null = null
const listeners = new Set<(value: MediaPreview | null) => void>()
function emit() { for (const listener of listeners) listener(current) }
export function subscribeMediaPreview(listener: (value: MediaPreview | null) => void) {
  listeners.add(listener); listener(current)
  return () => { listeners.delete(listener) }
}
export function closeMediaPreview(owner?: object) {
  if (pending && (!owner || pending.owner === owner)) {
    pending.controller.abort(); pending = null
  }
  if (!current || owner && current.owner !== owner) return
  const previous = current; current = null
  URL.revokeObjectURL(previous.url); emit(); previous.closed?.()
}
export function openMediaPreview(owner: object, input: MediaPreviewInput, closed?: () => void, navigate?: (index: number) => Promise<void>) {
  validateMediaPreview(input)
  const url = URL.createObjectURL(new Blob([input.bytes], { type: input.mime }))
  const previous = current
  if (previous?.owner === owner && previous.gallery?.id === input.gallery?.id && input.gallery) URL.revokeObjectURL(previous.url)
  else closeMediaPreview()
  current = { owner, url, title: input.title || '图片预览', closed, ...(navigate && input.gallery ? { gallery: { ...input.gallery }, navigate } : {}) }; emit()
}

// Native host views may reuse a Blob URL already authorized by ProjectReader.
// SDK/iframe callers still pass bytes; this is not an arbitrary URL fetch API.
export async function openMediaPreviewBlob(owner: object, source: string, title: string) {
  if (!source.startsWith('blob:')) throw new Error('图片尚未准备好。')
  pending?.controller.abort()
  const request = { owner, controller: new AbortController() }
  pending = request
  try {
    const response = await fetch(source, { signal: request.controller.signal })
    if (!response.ok) throw new Error('图片读取失败，请重试。')
    const blob = await response.blob()
    const bytes = await blob.arrayBuffer()
    if (pending !== request) return
    pending = null
    openMediaPreview(owner, { bytes, mime: blob.type, title: title.slice(0, 256) })
  } catch (error) {
    if (pending !== request) return
    pending = null
    throw error
  }
}

export async function stepMediaGallery(delta: -1 | 1) {
  const previous = current
  if (!previous?.gallery || !previous.navigate || previous.loading) return
  const index = (previous.gallery.index + delta + previous.gallery.count) % previous.gallery.count
  current = { ...previous, loading: true, error: undefined }; const pendingPreview = current; emit()
  try { await previous.navigate(index) }
  catch { if (current === pendingPreview) { current = { ...pendingPreview, loading: false, error: '无法读取该张图片，请重试。' }; emit() } }
}
