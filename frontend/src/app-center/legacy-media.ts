import type { LegacyAdapterID } from './legacy'

export const mediaAdapters = {
  'image-iframe-v1': {
    prefix: '/apps/image', channel: 'vf-image', query: 'image',
    read: /^\/(api\/(tasks(?:\/[a-zA-Z0-9_-]+)?|tasks\/stats|mcp\/status)|artifacts\/[a-zA-Z0-9_-]+\/content)$/,
    write: /^\/api\/(tasks|artifacts\/upload)$/,
  },
  'video-iframe-v1': {
    prefix: '/apps/video', channel: 'vf-video', query: 'video',
    read: /^\/(api\/(dashboard|capabilities|mcp\/status|models|models\/[a-z0-9-]+\/instances(?:\/[a-zA-Z0-9._-]+(?:\/gpus\/[a-zA-Z0-9._:-]+)?)?|tasks\/[a-zA-Z0-9_-]+(?:\/(?:thumbnail|content))?)|artifacts\/[a-zA-Z0-9_-]+\/content)$/,
    write: /^\/api\/(tasks|tasks\/[a-zA-Z0-9_-]+\/(result|title)|artifacts\/import)$/,
  },
} as const
export function mediaAdapter(id: LegacyAdapterID) { return id === 'blender-host-v1' ? null : mediaAdapters[id] }
export function validMediaRequest(adapter: NonNullable<ReturnType<typeof mediaAdapter>>, method: string, path: string, body: unknown): boolean {
  if (path.length > 2048 || /[#\\\u0000-\u001f\u007f]/.test(path) || path.split('?')[0].includes('%') || !path.startsWith('/')
    || body != null && (typeof body !== 'string' || body.length > 1048576)) return false
  const pathname = path.split('?')[0]
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
