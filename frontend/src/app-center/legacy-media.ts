import type { LegacyAdapterID } from './legacy'

export const mediaAdapters = {
  'image-iframe-v1': {
    prefix: '/apps/image', channel: 'vf-image', query: 'image',
    read: /^\/(api\/(tasks(?:\/[a-zA-Z0-9_-]+)?|tasks\/stats|mcp\/status)|artifacts\/[a-zA-Z0-9_-]+\/content)$/,
    write: /^\/api\/(tasks|artifacts\/upload)$/,
  },
  'video-iframe-v1': {
    prefix: '/apps/video', channel: 'vf-video', query: 'video',
    read: /^\/(api\/(dashboard|capabilities|mcp\/status|models|models\/[a-z0-9-]+\/instances(?:\/[a-zA-Z0-9._-]+(?:\/gpus\/[a-zA-Z0-9._:-]+)?)?|tasks\/[a-zA-Z0-9_-]+(?:\/thumbnail)?)|artifacts\/[a-zA-Z0-9_-]+\/content)$/,
    write: /^\/api\/(tasks|tasks\/[a-zA-Z0-9_-]+\/result|artifacts\/import)$/,
  },
} as const
export function mediaAdapter(id: LegacyAdapterID) { return id === 'blender-host-v1' ? null : mediaAdapters[id] }
export function validMediaRequest(adapter: NonNullable<ReturnType<typeof mediaAdapter>>, method: string, path: string, body: unknown): boolean {
  if (path.length > 2048 || /[#\\\u0000-\u001f\u007f]/.test(path) || path.split('?')[0].includes('%') || !path.startsWith('/')
    || body != null && (typeof body !== 'string' || body.length > 1048576)) return false
  const pathname = path.split('?')[0]
  return method === 'GET' ? adapter.read.test(pathname) && body == null : method === 'POST' && adapter.write.test(pathname)
}
