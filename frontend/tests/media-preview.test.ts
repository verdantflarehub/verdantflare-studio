import test from 'node:test'
import assert from 'node:assert/strict'
import { openMediaPreview, closeMediaPreview, subscribeMediaPreview, validateMediaPreview, type MediaPreview } from '../src/platform/media-preview.ts'
const input = () => ({ bytes: new Uint8Array([1, 2, 3]).buffer, mime: 'image/png', title: '图片' })
test('preview accepts only bounded raster bytes, never URLs, SVG or HTML', () => {
  validateMediaPreview(input())
  for (const patch of [{ mime: 'image/svg+xml' }, { mime: 'text/html' }, { bytes: 'https://other.invalid/' }, { bytes: new ArrayBuffer(0) }, { title: 'x'.repeat(257) }]) {
    assert.throws(() => validateMediaPreview({ ...input(), ...patch } as any), /INVALID_MEDIA_PREVIEW/)
  }
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
