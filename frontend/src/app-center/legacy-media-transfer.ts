import { AppCenterError } from './controller.ts'

const chunkBytes = 4 << 20, mediaBytes = 1 << 30
const binaryPath = /^\/(?:artifacts\/[a-zA-Z0-9_-]+\/content|api\/tasks\/[a-zA-Z0-9_-]+\/thumbnail)$/
const mediaTypes = new Set(['image/png', 'image/jpeg', 'image/webp', 'image/gif', 'image/avif', 'video/mp4', 'video/webm', 'audio/mpeg', 'audio/wav', 'audio/ogg', 'application/octet-stream'])
const invalid = (): never => { throw new AppCenterError(502, 'INVALID_LEGACY_RESPONSE') }
const cancelled = (signal?: AbortSignal) => { if (signal?.aborted) throw new AppCenterError(408, 'REQUEST_CANCELLED') }

async function readChunk(response: Response, expected: number, signal?: AbortSignal): Promise<ArrayBuffer> {
  if (!response.body || expected < 0 || expected > chunkBytes) return invalid()
  const reader = response.body.getReader(), output = new Uint8Array(expected)
  let size = 0
  try {
    for (;;) {
      cancelled(signal)
      const part = await reader.read()
      cancelled(signal)
      if (part.done) break
      if (size + part.value.byteLength > expected) return invalid()
      output.set(part.value, size); size += part.value.byteLength
    }
    if (size !== expected) return invalid()
    return output.buffer
  } catch (error) { await reader.cancel().catch(() => undefined); throw error }
  finally { reader.releaseLock() }
}

/** Windows Wails buffers each response. Assemble bounded ranges in the parent
 * WebView, retaining the same local workspace and immutable content target. */
export async function fetchLegacyMedia(fetcher: typeof fetch, resourceBase: string, path: string, options: RequestInit): Promise<Response> {
  if (options.method !== 'GET' || !binaryPath.test(path)) return fetcher(resourceBase + path, options)
  let offset = 0, total = 0, contentType = '', etag: string | null = null
  const chunks: ArrayBuffer[] = []
  const signal = options.signal ?? undefined
  for (;;) {
    cancelled(signal)
    const headers = new Headers(options.headers)
    headers.set('Range', `bytes=${offset}-${offset + chunkBytes - 1}`)
    const response = await fetcher(resourceBase + path, { ...options, headers })
    if (!response.ok) return response
    try {
      cancelled(signal)
      const type = response.headers.get('Content-Type')?.split(';')[0].trim() ?? ''
      const lengthText = response.headers.get('Content-Length') ?? ''
      if (!mediaTypes.has(type) || !/^\d+$/.test(lengthText)) return invalid()
      const length = Number(lengthText)
      if (!Number.isSafeInteger(length) || length > chunkBytes) return invalid()
      if (response.status === 200 && offset === 0) {
        const data = await readChunk(response, length, signal)
        return new Response(data, { headers: { 'Content-Type': type, 'Content-Length': String(length) } })
      }
      const range = /^bytes (\d+)-(\d+)\/(\d+)$/.exec(response.headers.get('Content-Range') ?? '')
      if (response.status !== 206 || !range) return invalid()
      const [start, end, size] = range.slice(1).map(Number)
      if (![start, end, size].every(Number.isSafeInteger) || size < 1 || size > mediaBytes || start !== offset
        || end !== Math.min(offset + chunkBytes - 1, size - 1) || length !== end - start + 1) return invalid()
      if (offset === 0) { total = size; contentType = type; etag = response.headers.get('ETag') }
      if (total !== size || contentType !== type || etag !== response.headers.get('ETag')) return invalid()
      chunks.push(await readChunk(response, length, signal))
      offset += length
      if (offset === total) return new Response(new Blob(chunks, { type: contentType }), { headers: { 'Content-Type': contentType, 'Content-Length': String(total) } })
    } catch (error) { await response.body?.cancel().catch(() => undefined); throw error }
  }
}
