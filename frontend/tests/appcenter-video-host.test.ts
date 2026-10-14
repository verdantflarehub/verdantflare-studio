import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import ts from 'typescript'
import { mediaAdapter, validMediaRequest } from '../src/app-center/legacy-media.ts'
import { videoPage, videoTaskLabel } from '../src/app-center/legacy-video-navigation.ts'

const source = readFileSync(new URL('../src/app-center/LegacyMediaHost.vue', import.meta.url), 'utf8').split('<script setup lang="ts">')[1].split('</script>')[0].replace(/^import .*$/gm, '')
const code = ts.transpileModule(source, { compilerOptions: { target: ts.ScriptTarget.ES2022, module: ts.ModuleKind.None } }).outputText
const root = '/market?video=' + encodeURIComponent('/dashboard#tasks')
const detail = (id: string) => '/market?video=' + encodeURIComponent('/dashboard/tasks/' + id)

function host(fetcher: typeof fetch = async () => Response.json({})) {
  const props = { adapterId: 'video-iframe-v1', route: root, title: 'Video MCP', authorize: async () => {} }
  const events: Array<[string, any]> = [], posts: any[] = [], scrolls: number[] = [], callbacks = new Map<number, FrameRequestCallback>()
  let next = 0, dispose = () => {}, mount = () => {}
  const frameWindow = { postMessage: (data: any) => posts.push(data) }
  const win = { history: { scrollRestoration: 'auto' }, scrollY: 0, scrollTo: ({ top }: { top: number }) => { scrolls.push(top); win.scrollY = top }, scrollBy: () => {}, addEventListener: () => {}, removeEventListener: () => {} }
  const doc = { body: { dataset: { theme: 'light' } }, documentElement: { scrollHeight: 5000 } }
  const deps = { defineProps: () => props, defineEmits: () => (name: string, data: any) => events.push([name, data]),
    ref: (value: any) => ({ value }), computed: (get: () => any) => ({ get value() { return get() } }), watch: () => {}, onMounted: (fn: () => void) => { mount = fn }, onBeforeUnmount: (fn: () => void) => { dispose = fn },
    mediaAdapter, validMediaRequest, videoPage, videoTaskLabel, fetchLegacyMedia: () => { throw Error('unexpected desktop fetch') },
    window: win, document: doc, location: { origin: 'https://studio.invalid' }, innerHeight: 1000, fetch: fetcher,
    MutationObserver: class { observe() {} disconnect() {} },
    requestAnimationFrame: (fn: FrameRequestCallback) => { callbacks.set(++next, fn); return next }, cancelAnimationFrame: (id: number) => callbacks.delete(id) }
  const api = new Function(...Object.keys(deps), code + '\nreturn { frame, src, load, receive };')(...Object.values(deps))
  api.frame.value = { contentWindow: frameWindow, style: {} }
  const receive = (data: object) => api.receive({ source: frameWindow, origin: 'null', data: { channel: 'vf-video', view: new URL(api.src.value, 'https://studio.invalid').searchParams.get('view'), ...data } })
  const load = () => api.load()
  const flush = () => { while (callbacks.size) { const batch = [...callbacks.values()]; callbacks.clear(); batch.forEach(fn => fn(0)) } }
  return { props, events, posts, scrolls, win, doc, receive, load, flush, mount: () => mount(), dispose: () => dispose() }
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
