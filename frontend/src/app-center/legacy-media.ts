import type { LegacyAdapterID } from './legacy'

export const mediaAdapters = {
  'image-iframe-v1': {
    prefix: '/apps/image', channel: 'vf-image', query: 'image',
    read: /^\/(api\/(tasks(?:\/[a-zA-Z0-9_-]+)?|tasks\/stats|mcp\/status)|artifacts\/[a-zA-Z0-9_-]+\/(content|thumbnail))$/,
    write: /^\/api\/(tasks|artifacts\/upload)$/,
  },
  'video-iframe-v1': {
    prefix: '/apps/video', channel: 'vf-video', query: 'video',
    read: /^\/(api\/(dashboard|capabilities|model-catalog|mcp\/status|models|models\/[a-z0-9-]+\/instances(?:\/[a-zA-Z0-9._-]+(?:\/gpus\/[a-zA-Z0-9._:-]+)?)?|tasks\/[a-zA-Z0-9_-]+(?:\/(?:thumbnail|content))?)|artifacts\/[a-zA-Z0-9_-]+\/content)$/,
    write: /^\/api\/(tasks|tasks\/[a-zA-Z0-9_-]+\/(result|title)|artifacts\/import)$/,
  },
} as const

const immutableMediaPath = /^\/(artifacts\/[a-zA-Z0-9_-]+\/(content|thumbnail)|api\/tasks\/[a-zA-Z0-9_-]+\/(thumbnail|content)|api\/higgsfield\/(assets\/art_[a-zA-Z0-9_-]+\/content|tasks\/[a-zA-Z0-9_-]+\/(thumbnail|content)))$/
export function isImmutableMediaRequest(method: string, path: string): boolean {
  return method === 'GET' && immutableMediaPath.test(path.split('?')[0])
}
export function mediaAdapter(id: LegacyAdapterID) { return id === 'blender-host-v1' ? null : mediaAdapters[id] }
export function validMediaRequest(adapter: NonNullable<ReturnType<typeof mediaAdapter>>, method: string, path: string, body: unknown): boolean {
  if (path.length > 2048 || /[#\\\u0000-\u001f\u007f]/.test(path) || path.split('?')[0].includes('%') || !path.startsWith('/')
    || body != null && (typeof body !== 'string' || body.length > 1048576)) return false
  const pathname = path.split('?')[0]
  if (adapter.channel === 'vf-video' && pathname.startsWith('/api/higgsfield/')) {
    return method === 'GET' && body == null && /^\/api\/higgsfield\/(projects|capabilities|assets|assets\/art_[a-zA-Z0-9_-]+\/content|tasks|tasks\/[a-zA-Z0-9_-]+(?:\/(?:thumbnail|content))?)$/.test(pathname)
      || method === 'POST' && /^\/api\/higgsfield\/(preflight|tasks|attempt|imports\/(prepare|chunk|status|commit)|tasks\/[a-zA-Z0-9_-]+\/result)$/.test(pathname)
  }
  if (adapter.channel === 'vf-video' && pathname.endsWith('/title')) {
    if (method !== 'POST' || pathname !== path || typeof body !== 'string') return false
    try {
      const value = JSON.parse(body)
      if (!value || typeof value !== 'object' || Array.isArray(value) || Object.keys(value).sort().join(',') !== 'expected_title_revision,title') return false
      if (typeof value.title !== 'string' || !Number.isSafeInteger(value.expected_title_revision) || value.expected_title_revision < 0) return false
      const length = [...value.title.trim()].length
      if (length < 1 || length > 64 || /[<>\p{C}\u2028\u2029]/u.test(value.title)) return false
    } catch { return false }
  }
  return method === 'GET' ? adapter.read.test(pathname) && body == null : method === 'POST' && adapter.write.test(pathname)
}

/** A channel credential failure does not revoke the Studio workspace session.
 * Classification is bounded and does not grant access or expose response text. */
export function isVideoChannelFailure(contentType: string | null, body: ArrayBuffer): boolean {
  if (contentType?.split(';')[0].trim().toLowerCase() !== 'application/json' || body.byteLength > 65536) return false
  try {
    const value = JSON.parse(new TextDecoder().decode(body)), error = value?.error
    return !!error && typeof error === 'object' && !Array.isArray(error)
      && ['higgsfield', 'fal', 'grok', 'google'].includes(error.source)
      && typeof error.code === 'string' && /^[a-z][a-z0-9_]{0,95}$/.test(error.code)
      && typeof error.message === 'string' && error.message.length > 0 && error.message.length <= 512
      && typeof error.retryable === 'boolean'
      && (error.accepted === undefined || typeof error.accepted === 'boolean')
      && (error.http_status === undefined || Number.isInteger(error.http_status) && error.http_status >= 400 && error.http_status <= 599)
  } catch { return false }
}
