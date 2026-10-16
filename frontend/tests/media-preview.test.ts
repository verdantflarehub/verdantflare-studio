import test from 'node:test'
import assert from 'node:assert/strict'
import { openMediaPreview, openMediaPreviewBlob, closeMediaPreview, subscribeMediaPreview, validateMediaPreview, type MediaPreview } from '../src/platform/media-preview.ts'
const input = () => ({ bytes: new Uint8Array([1, 2, 3]).buffer, mime: 'image/png', title: '图片' })
test('preview accepts only bounded raster bytes, never URLs, SVG or HTML', () => {
  validateMediaPreview(input())
  for (const patch of [{ mime: 'image/svg+xml' }, { mime: 'text/html' }, { bytes: 'https://other.invalid/' }, { bytes: new ArrayBuffer(0) }, { title: 'x'.repeat(257) }]) {
    assert.throws(() => validateMediaPreview({ ...input(), ...patch } as any), /INVALID_MEDIA_PREVIEW/)
  }
})

test('native authorized blob uses the shared preview and rejects remote URLs', async () => {
  const owner = {}, source = URL.createObjectURL(new Blob(['image'], { type: 'image/png' }))
  let state: MediaPreview | null = null
  const off = subscribeMediaPreview(value => { state = value })
  try {
    await assert.rejects(openMediaPreviewBlob(owner, 'https://example.invalid/a.png', '图'))
    await openMediaPreviewBlob(owner, source, '角色图')
    assert.equal((state as MediaPreview | null)?.title, '角色图')
    assert.equal((state as MediaPreview | null)?.owner, owner)
  } finally { closeMediaPreview(); off(); URL.revokeObjectURL(source) }
})

test('late native reads cannot reopen after cancellation or replace a newer preview', async () => {
  const originalFetch = globalThis.fetch, owner = {}, other = {}
  let resolve!: (response: Response) => void, state: MediaPreview | null = null
  const off = subscribeMediaPreview(value => { state = value })
  globalThis.fetch = (() => new Promise<Response>(done => { resolve = done })) as typeof fetch
  try {
    for (const replacement of [false, true]) {
      const reading = openMediaPreviewBlob(owner, 'blob:authorized', '旧图')
      if (replacement) openMediaPreview(other, input())
      else closeMediaPreview(owner)
      resolve(new Response(new Blob(['image'], { type: 'image/png' })))
      await reading
      assert.equal((state as MediaPreview | null)?.owner ?? null, replacement ? other : null)
      closeMediaPreview()
    }
  } finally { globalThis.fetch = originalFetch; closeMediaPreview(); off() }
})
test('replacement revokes the old URL, stale owners cannot close the new image', async () => {
  const first = {}, second = {}, states: Array<MediaPreview | null> = []
  let closed = 0
  const off = subscribeMediaPreview(value => states.push(value))
  try {
    openMediaPreview(first, input(), () => closed++)
    const old = states.at(-1)!.url
    openMediaPreview(second, input())
    assert.equal(closed, 1)
    await assert.rejects(fetch(old))
    closeMediaPreview(first)
    const next = states.at(-1)!.url
    assert.equal((await fetch(next)).status, 200)
    closeMediaPreview(second)
    assert.equal(states.at(-1), null)
    await assert.rejects(fetch(next))
  } finally { closeMediaPreview(); off() }
})
