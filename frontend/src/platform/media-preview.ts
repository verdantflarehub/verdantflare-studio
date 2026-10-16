export interface MediaPreviewInput { bytes: ArrayBuffer; mime: string; title: string }
export interface MediaPreview { owner: object; url: string; title: string; closed?: () => void }
const formats = new Set(['image/png', 'image/jpeg', 'image/webp', 'image/gif'])
export function validateMediaPreview(input: MediaPreviewInput): void {
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
export function openMediaPreview(owner: object, input: MediaPreviewInput, closed?: () => void) {
  validateMediaPreview(input)
  const url = URL.createObjectURL(new Blob([input.bytes], { type: input.mime }))
  closeMediaPreview()
  current = { owner, url, title: input.title || '图片预览', closed }; emit()
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
