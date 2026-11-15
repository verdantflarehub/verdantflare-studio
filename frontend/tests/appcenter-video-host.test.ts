import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import ts from 'typescript'
import { openMediaPreview, closeMediaPreview, validateMediaPreview, subscribeMediaPreview } from '../src/platform/media-preview.ts'
import { mediaAdapter, legacyMediaHeaders, validMediaRequest, isImmutableMediaRequest, isVideoChannelFailure } from '../src/app-center/legacy-media.ts'
import { videoPath } from '../src/app-center/legacy.ts'
import { videoPage, videoTaskLabel, videoTaskName } from '../src/app-center/legacy-video-navigation.ts'

const source = readFileSync(new URL('../src/app-center/LegacyMediaHost.vue', import.meta.url), 'utf8').split('<script setup lang="ts">')[1].split('</script>')[0].replace(/^import .*$/gm, '')
const code = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.None } }).outputText
const root = '/market?video=' + encodeURIComponent('/dashboard#tasks')
const detail = (id: string) => '/market?video=' + encodeURIComponent('/dashboard/tasks/' + id)

test('global preview authorizes bytes, rejects active content and cancels on route changes', async () => {
  const h = host(), states: any[] = []
  const off = subscribeMediaPreview(value => states.push(value))
  const media = { type: 'media-preview', id: 8, bytes: new Uint8Array([1]).buffer, mime: 'image/png', title: '参考图' }
  try {
    h.load()
    await h.receive(media)
    assert.equal(states.at(-1)?.title, '参考图')
    await h.receive({ type: 'media-close' })
    assert.equal(states.at(-1), null)
    await h.receive({ ...media, mime: 'image/svg+xml' })
    assert.equal(states.at(-1), null)
    assert.ok(h.posts.some(p => p.type === 'media-result' && p.ok === false))
    let allow!: () => void
    h.props.authorize = () => new Promise<void>(resolve => { allow = resolve })
    const pending = h.receive(media)
    h.props.route = detail('other'); h.load(); allow(); await pending
    assert.equal(states.at(-1), null)
  } finally { h.dispose(); off() }
})

function host(fetcher: typeof fetch = async () => Response.json({}), options: { adapterId?: 'video-iframe-v1' | 'image-iframe-v1'; route?: string; includeNsfw?: boolean } = {}) {
  const adapterId = options.adapterId ?? 'video-iframe-v1'
  const defaultRoute = adapterId === 'image-iframe-v1'
    ? '/market?image=' + encodeURIComponent('/dashboard#tasks')
    : root
  const props = { adapterId, route: options.route ?? defaultRoute, title: adapterId === 'image-iframe-v1' ? 'Image MCP' : 'Video MCP', authorize: async () => {} }
  const events: Array<[string, any]> = [], posts: any[] = [], scrolls: number[] = [], callbacks = new Map<number, FrameRequestCallback>()
  let next = 0, dispose = () => {}, mount = () => {}
  let currentPrefs = { includeNsfw: Boolean(options.includeNsfw) }
  const prefListeners = new Set<(value: { includeNsfw: boolean }) => void>()
  const hostContentPreferences = {
    get: () => ({ ...currentPrefs }),
    subscribe: (fn: (value: { includeNsfw: boolean }) => void) => {
      prefListeners.add(fn)
      return () => { prefListeners.delete(fn) }
    },
  }
  const setIncludeNsfw = (includeNsfw: boolean) => {
    currentPrefs = { includeNsfw }
    for (const fn of [...prefListeners]) fn({ ...currentPrefs })
  }
  const frameWindow = { postMessage: (data: any) => posts.push(data) }
  const win = { history: { scrollRestoration: 'auto' }, scrollY: 0, scrollTo: ({ top }: { top: number }) => { scrolls.push(top); win.scrollY = top }, scrollBy: () => {}, addEventListener: () => {}, removeEventListener: () => {} }
  const doc = { body: { dataset: { theme: 'light' } }, documentElement: { scrollHeight: 5000 } }
  const deps = { defineProps: () => props, defineEmits: () => (name: string, data: any) => events.push([name, data]),
    ref: (value: any) => ({ value }), computed: (get: () => any) => ({ get value() { return get() } }), watch: () => {}, onMounted: (fn: () => void) => { mount = fn }, onBeforeUnmount: (fn: () => void) => { dispose = fn },
    mediaAdapter, legacyMediaHeaders, validMediaRequest, isImmutableMediaRequest, isVideoChannelFailure, hostContentPreferences,
    videoPath, videoPage, videoTaskLabel, videoTaskName, openMediaPreview, closeMediaPreview, validateMediaPreview, fetchLegacyMedia: () => { throw Error('unexpected desktop fetch') },
    window: win, document: doc, location: { origin: 'https://studio.invalid' }, innerHeight: 1000, fetch: fetcher,
    MutationObserver: class { observe() {} disconnect() {} },
    requestAnimationFrame: (fn: FrameRequestCallback) => { callbacks.set(++next, fn); return next }, cancelAnimationFrame: (id: number) => callbacks.delete(id) }
  const api = new Function(...Object.keys(deps), code + '\nreturn { frame, src, load, receive };')(...Object.values(deps))
  api.frame.value = { contentWindow: frameWindow, style: {} }
  const channel = adapterId === 'image-iframe-v1' ? 'vf-image' : 'vf-video'
  const receive = (data: object) => api.receive({ source: frameWindow, origin: 'null', data: { channel, view: new URL(api.src.value, 'https://studio.invalid').searchParams.get('view'), ...data } })
  const load = () => api.load()
  const flush = () => { while (callbacks.size) { const batch = [...callbacks.values()]; callbacks.clear(); batch.forEach(fn => fn(0)) } }
  return { props, events, posts, scrolls, win, doc, frame: api.frame, src: api.src, setIncludeNsfw, receive, load, flush, mount: () => mount(), dispose: () => dispose() }
}

test('Video owns scroll restoration only while its host is mounted', () => {
  const h = host()
  h.mount()
  assert.equal(h.win.history.scrollRestoration, 'manual')
  h.dispose()
  assert.equal(h.win.history.scrollRestoration, 'auto')
})

test('breadcrumb navigation and browser history restore list state after render and resize', async () => {
  const h = host()
  try {
    h.load(); await h.receive({ type: 'ready' })
    const state = { q: '纸船', project: 'project', engine: 'fal', status: 'succeeded', page: 2, table: true }
    await h.receive({ type: 'navigate', path: '/dashboard#tasks', state })
    assert.equal(h.events.filter(e => e[0] === 'navigate').length, 0)
    h.win.scrollY = 720; h.props.route = detail('A'); h.load()
    assert.equal(h.win.scrollY, 0)
    h.props.route = root; h.load(); await h.receive({ type: 'ready' })
    assert.deepEqual(h.posts.filter(p => p.type === 'restore').at(-1).state, state)
    await h.receive({ type: 'resize', height: 2600 }); h.flush()
    assert.equal(h.win.scrollY, 0)
    await h.receive({ type: 'navigate', path: '/dashboard#tasks', state }); h.flush()
    assert.equal(h.win.scrollY, 720)
    h.win.scrollY = 1700; h.props.route = detail('A'); h.load(); h.props.route = root; h.load()
    h.doc.documentElement.scrollHeight = 1200
    await h.receive({ type: 'navigate', path: '/dashboard#tasks', state }); h.flush()
    assert.equal(h.win.scrollY, 200)
  } finally { h.dispose() }
})

test('old response generations cannot supply a task name or touch the new page', async () => {
  let finish!: (value: Response) => void
  const h = host(() => new Promise<Response>(resolve => { finish = resolve }))
  try {
    h.load(); h.props.route = detail('A'); h.load()
    const pending = h.receive({ type: 'request', id: 1, path: '/api/tasks/A' })
    await Promise.resolve(); h.props.route = detail('B'); h.load()
    finish(Response.json({ video_task_id: 'A', project_id: '纸船', idempotency_key: '旧镜头' }))
    await pending
    assert.ok(!h.events.some(e => e[0] === 'taskLabel' && e[1].label.includes('旧镜头')))
    assert.ok(!h.posts.some(p => p.type === 'response' && p.id === 1))
  } finally { h.dispose() }
})

test('authorization failures clear a previously readable name without forwarding the request', async () => {
  let fetched = 0
  const h = host(async () => { fetched++; return Response.json({ video_task_id: 'A', project_id: '纸船', idempotency_key: 'shot-key', title: '河上漂流的纸船' }) })
  try {
    h.load(); h.props.route = detail('A'); h.load()
    await h.receive({ type: 'request', id: 1, path: '/api/tasks/A' })
    assert.equal(h.events.at(-1)?.[1].label, '河上漂流的纸船')
    h.props.authorize = async () => { throw Error('denied') }
    await h.receive({ type: 'request', id: 2, path: '/api/tasks/A' })
    assert.equal(h.events.at(-1)?.[1].label, '任务不可用')
    assert.equal(fetched, 1)
  } finally { h.dispose() }
})

test('rename conflicts and project write denial remain business errors inside the task', async () => {
  for (const status of [409, 403]) {
    const h = host(async () => Response.json({ error: status === 409 ? 'title_conflict' : 'permission_denied' }, { status }))
    try {
      h.load(); h.props.route = detail('A'); h.load()
      await h.receive({ type: 'request', id: 1, method: 'POST', path: '/api/tasks/A/title', body: JSON.stringify({ title: '纸船', expected_title_revision: 0 }) })
      assert.equal(h.posts.at(-1).status, status)
      assert.ok(!h.events.some(e => e[0] === 'error'))
    } finally { h.dispose() }
  }
})

test('rename proxy accepts only its exact method, path and body', () => {
  const adapter = mediaAdapter('video-iframe-v1')!
  const body = JSON.stringify({title: '纸船', expected_title_revision: 0})
  assert.ok(validMediaRequest(adapter, 'POST', '/api/tasks/A/title', body))
  for (const [method, path, input] of [
    ['GET', '/api/tasks/A/title', null], ['POST', '/api/tasks/A/title?route=x', body],
    ['POST', '/api/tasks/A/title/other', body], ['POST', '/api/tasks/A/title', '{}'],
    ['POST', '/api/tasks/A/title', JSON.stringify({title:'纸船', expected_title_revision:true})],
    ['POST', '/api/tasks/A/title', JSON.stringify({title:'纸船', expected_title_revision:0, route:'other'})],
  ]) assert.equal(validMediaRequest(adapter, method as string, path as string, input), false)
})

test('channel 401 keeps its exact diagnostic inside the active Video workspace', async () => {
  for (const source of ['higgsfield', 'fal', 'grok', 'google']) {
    const error = { code: 'authorization_required', message: 'Membership authorization expired.', source, retryable: false, accepted: false, http_status: 401 }
    const h = host(async () => Response.json({ error }, { status: 401 }))
    try {
      h.load(); await h.receive({ type: 'ready' })
      await h.receive({ type: 'request', id: 1, method: 'POST', path: '/api/higgsfield/preflight', body: '{}' })
      assert.equal(h.posts.at(-1).status, 401)
      assert.deepEqual(JSON.parse(new TextDecoder().decode(h.posts.at(-1).body)), { error })
      assert.ok(!h.events.some(e => e[0] === 'error'))
      await h.receive({ type: 'request', id: 2, path: '/api/dashboard' })
      assert.equal(h.posts.at(-1).id, 2)
    } finally { h.dispose() }
  }
})

test('host 401 and revoked 410 still invalidate the workspace', async () => {
  for (const [status, body] of [[401, {}], [401, { error: { source: 'video', code: 'unauthorized', message: 'Host authorization failed.', retryable: false } }],
      [410, { error: { source: 'grok', code: 'authorization_required', message: 'Expired.', retryable: false } }]] as const) {
    const h = host(async () => Response.json(body, { status }))
    try {
      h.load(); await h.receive({ type: 'ready' })
      await h.receive({ type: 'request', id: 1, path: '/api/dashboard' })
      assert.ok(h.events.some(e => e[0] === 'error'))
    } finally { h.dispose() }
  }
})

test('malformed, oversized and non-JSON 401 bodies cannot masquerade as channel diagnostics', () => {
  const error = { source: 'grok', code: 'unauthorized', message: 'Credential rejected.', retryable: false }
  const bytes = (value: unknown) => new TextEncoder().encode(JSON.stringify(value)).buffer
  for (const value of [{}, { error: 'unauthorized' }, { error: { ...error, source: 'studio' } },
      { error: { ...error, code: '/private/credential' } }, { error: { ...error, retryable: 'false' } },
      { error: { ...error, accepted: 'false' } }, { error: { ...error, http_status: 200 } },
      { error: { ...error, message: 'x'.repeat(513) } }]) assert.equal(isVideoChannelFailure('application/json', bytes(value)), false)
  assert.equal(isVideoChannelFailure('text/html', bytes({ error })), false)
  assert.equal(isVideoChannelFailure('application/json', new ArrayBuffer(65537)), false)
  assert.equal(isVideoChannelFailure('application/json', new TextEncoder().encode('{').buffer), false)
})

test('image-iframe-v1 resize clamps height across [600, 2000000] and ignores non-finite values', async () => {
  const h = host(undefined, { adapterId: 'image-iframe-v1' })
  try {
    h.load()
    await h.receive({ type: 'ready' })
    await h.receive({ type: 'resize', height: 320 })
    assert.equal(h.frame.value.style.height, '600px')
    await h.receive({ type: 'resize', height: 2400 })
    assert.equal(h.frame.value.style.height, '2400px')
    await h.receive({ type: 'resize', height: 295000 })
    assert.equal(h.frame.value.style.height, '295000px')
    await h.receive({ type: 'resize', height: 2500000 })
    assert.equal(h.frame.value.style.height, '2000000px')
    for (const bad of [Number.NaN, Number.POSITIVE_INFINITY, Number.NEGATIVE_INFINITY]) {
      await h.receive({ type: 'resize', height: bad })
      assert.equal(h.frame.value.style.height, '2000000px')
    }
  } finally { h.dispose() }
})

test('image-iframe-v1 posts contentPreferences on ready, proxies keyset before_cursor with NSFW header, and reloads on preference change', async () => {
  const seenRequests: Array<{ url: string; headers: Record<string, string> }> = []
  const h = host(async (input, init) => {
    seenRequests.push({ url: String(input), headers: (init?.headers as Record<string, string>) ?? {} })
    return Response.json({ tasks: [], total: 0, limit: 50, offset: 0, has_more: false, next_cursor: null })
  }, { adapterId: 'image-iframe-v1', includeNsfw: false })
  try {
    h.mount()
    const firstSrc = h.src.value
    await h.receive({ type: 'ready' })
    assert.deepEqual(h.posts.find(p => p.type === 'contentPreferences'), {
      channel: 'vf-studio',
      type: 'contentPreferences',
      includeNsfw: false,
    })
    await h.receive({
      type: 'request',
      id: 1,
      path: '/api/tasks?limit=50&offset=0&before_cursor=' + encodeURIComponent('4413:2026-11-15T04:00:00.000Z'),
    })
    assert.equal(seenRequests.length, 1)
    assert.equal(seenRequests[0].url, '/apps/image/api/tasks?limit=50&offset=0&before_cursor=4413%3A2026-11-15T04%3A00%3A00.000Z')
    assert.equal(seenRequests[0].headers['X-VF-Include-NSFW'], undefined)

    h.setIncludeNsfw(true)
    assert.notEqual(h.src.value, firstSrc, 'toggling includeNsfw must force reload to a new iframe generation')
    await h.receive({ type: 'ready' })
    assert.deepEqual(h.posts.filter(p => p.type === 'contentPreferences').at(-1), {
      channel: 'vf-studio',
      type: 'contentPreferences',
      includeNsfw: true,
    })
    await h.receive({
      type: 'request',
      id: 2,
      path: '/api/tasks?limit=50&offset=0&before_cursor=' + encodeURIComponent('4363:2026-11-15T03:50:00.000Z'),
    })
    assert.equal(seenRequests.length, 2)
    assert.equal(seenRequests[1].headers['X-VF-Include-NSFW'], '1')
  } finally { h.dispose() }
})
