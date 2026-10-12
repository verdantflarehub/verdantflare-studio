import assert from 'node:assert/strict'
import test from 'node:test'
import { fetchLegacyMedia } from '../src/app-center/legacy-media-transfer.ts'

const base = '/studio/legacy-media/01980000-0000-4000-8000-000000000001', size = 4 << 20
const options: RequestInit = { method: 'GET', credentials: 'omit', redirect: 'error', cache: 'no-store' }

test('desktop media assembles exact ranges and keeps credentials out of requests', async () => {
  const ranges: string[] = [], total = size + 3
  const result = await fetchLegacyMedia(async (url, input) => {
    assert.equal(url, base + '/artifacts/sample/content')
    assert.equal(input?.credentials, 'omit')
    ranges.push(new Headers(input?.headers).get('Range')!)
    const offset = ranges.length === 1 ? 0 : size, length = offset === 0 ? size : 3
    return new Response(new Uint8Array(length).fill(offset ? 2 : 1), { status: 206, headers: {
      'Content-Type': 'video/mp4', 'Content-Length': String(length), 'Content-Range': `bytes ${offset}-${offset + length - 1}/${total}`, ETag: '"fixed"',
    } })
  }, base, '/artifacts/sample/content', options)
  assert.deepEqual(ranges, [`bytes=0-${size - 1}`, `bytes=${size}-${size * 2 - 1}`])
  assert.equal(result.status, 200)
  assert.equal(result.headers.get('Content-Length'), String(total))
  const content = new Uint8Array(await result.arrayBuffer())
  assert.equal(content.length, total); assert.equal(content[0], 1); assert.equal(content[size], 2)
})

test('small media and ordinary API requests keep their original semantics', async () => {
  const media = await fetchLegacyMedia(async () => new Response('small', { headers: { 'Content-Type': 'image/png', 'Content-Length': '5' } }), base, '/artifacts/sample/content', options)
  assert.equal(await media.text(), 'small')
  const post = { ...options, method: 'POST', body: '{"idempotency_key":"original"}' }
  let count = 0
  await fetchLegacyMedia(async (_url, input) => { count++; assert.equal(input, post); return Response.json({ accepted: true }) }, base, '/api/tasks', post)
  assert.equal(count, 1)
})

test('range metadata, payload lengths, media types and total size are verified', async () => {
  for (const headers of [
    { 'Content-Range': 'bytes 1-3/4', 'Content-Length': '3', 'Content-Type': 'image/png' },
    { 'Content-Range': 'bytes 0-3/4', 'Content-Length': '4', 'Content-Type': 'text/html' },
    { 'Content-Range': 'bytes 0-3/4', 'Content-Length': '3', 'Content-Type': 'image/png' },
    { 'Content-Range': 'bytes 0-3/9999999999999999999999', 'Content-Length': '4', 'Content-Type': 'image/png' },
    { 'Content-Range': `bytes 0-${size - 1}/${(1 << 30) + 1}`, 'Content-Length': String(size), 'Content-Type': 'image/png' },
  ]) await assert.rejects(fetchLegacyMedia(async () => new Response('1234', { status: 206, headers }), base, '/artifacts/sample/content', options), /INVALID_LEGACY_RESPONSE/)
  for (const length of ['3', '5', String(size + 1)]) {
    await assert.rejects(fetchLegacyMedia(async () => new Response('1234', { headers: { 'Content-Type': 'image/png', 'Content-Length': length } }), base, '/artifacts/sample/content', options), /INVALID_LEGACY_RESPONSE/)
  }
})

test('changing entities, failures and cancellation discard partial results without retry', async () => {
  for (const mode of ['etag', 'size', 'cancel', 'failure']) {
    let calls = 0
    const controller = new AbortController()
    const promise = fetchLegacyMedia(async () => {
      calls++
      if (calls === 2 && mode === 'failure') return Response.json({ code: 'UNAUTHENTICATED' }, { status: 401 })
      if (calls === 2 && mode === 'cancel') controller.abort()
      const offset = calls === 1 ? 0 : size, length = calls === 1 ? size : 1
      return new Response(new Uint8Array(length), { status: 206, headers: { 'Content-Type': 'image/png', 'Content-Length': String(length),
        'Content-Range': `bytes ${offset}-${offset + length - 1}/${size + (calls === 2 && mode === 'size' ? 2 : 1)}`, ETag: calls === 2 && mode === 'etag' ? '"new"' : '"old"' } })
    }, base, '/artifacts/sample/content', { ...options, signal: controller.signal })
    if (mode === 'failure') assert.equal((await promise).status, 401)
    else await assert.rejects(promise, mode === 'cancel' ? /REQUEST_CANCELLED/ : /INVALID_LEGACY_RESPONSE/)
    assert.equal(calls, 2)
  }
})
