<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { LegacyAdapterID } from './legacy'
import { videoPath } from './legacy'
import { mediaAdapter, validMediaRequest } from './legacy-media'
import { fetchLegacyMedia } from './legacy-media-transfer'
import { videoPage, videoTaskLabel, videoTaskName } from './legacy-video-navigation'
import { openMediaPreview, closeMediaPreview, validateMediaPreview } from '../platform/media-preview'

const props = defineProps<{ adapterId: LegacyAdapterID; route: string; title: string; resourceBase?: string; authorize: () => Promise<void> }>()
const emit = defineEmits<{ navigate: [route: string]; error: [message: string]; reconnect: []; taskLabel: [value: { route: string; label: string }] }>()
const frame = ref<HTMLIFrameElement>(), src = ref(''), state = ref<'loading' | 'ready' | 'error'>('loading'), message = ref('')
const adapter = computed(() => mediaAdapter(props.adapterId))
let generation = 0, disposed = false, controller = new AbortController(), deadline: ReturnType<typeof setTimeout> | undefined
let currentPath = '', viewState: unknown = null
const mediaOwner = {}
let mediaRevision = 0
function closePreview() { mediaRevision++; closeMediaPreview(mediaOwner) }
let currentBase = '', currentRoute = ''
const taskNames = new Map<string, string>()
let previousScrollRestoration: ScrollRestoration | undefined
let listScroll: number | null = null, restoreScroll: number | null = null, listRendered = false, scrollFrame = 0
const isTaskList = (route: string) => {
  const page = videoPage(route)
  return props.adapterId === 'video-iframe-v1' && page.path.startsWith('/dashboard') && !page.taskId && ['tasks', 'higgsfield'].includes(page.section)
}
function restoreListScroll() {
  if (restoreScroll == null || !listRendered || !isTaskList(props.route)) return
  cancelAnimationFrame(scrollFrame)
  const epoch = generation
  scrollFrame = requestAnimationFrame(() => {
    scrollFrame = requestAnimationFrame(() => {
      if (disposed || epoch !== generation || restoreScroll == null || !isTaskList(props.route)) return
      window.scrollTo({ top: Math.min(restoreScroll, Math.max(0, document.documentElement.scrollHeight - innerHeight)), behavior: 'instant' })
      restoreScroll = null
    })
  })
}
const pending = new Set<number>()
const theme = () => document.body.dataset.theme === 'light' ? 'light' : 'dark'
function post(value: object) { frame.value?.contentWindow?.postMessage({ channel: 'vf-studio', ...value }, '*') }
function failed(text: string) { closePreview(); if (props.adapterId === 'video-iframe-v1') emit('taskLabel', { route: props.route, label: '任务不可用' }); state.value = 'error'; message.value = text; deadline !== undefined && clearTimeout(deadline) }
function load(force = false) {
  closePreview()
  const a = adapter.value
  if (!a) return
  const path = new URL(props.route, location.origin).searchParams.get(a.query)!
  const [pathname, hash] = path.split('#')
  const cachedName = taskNames.get(videoPage(props.route).taskId)
  if (cachedName) emit('taskLabel', { route: props.route, label: cachedName })
  const base = props.resourceBase ?? a.prefix
  if (props.adapterId === 'video-iframe-v1' && currentRoute !== props.route) {
    if (isTaskList(currentRoute)) listScroll = window.scrollY
    restoreScroll = isTaskList(props.route) ? listScroll : null
    listRendered = false
    if (!isTaskList(props.route)) window.scrollTo({ top: 0, behavior: 'instant' })
    currentRoute = props.route
  }
  if (!force && currentBase === base && (currentPath === pathname || props.adapterId === 'image-iframe-v1') && src.value && state.value === 'ready') { post({ type: 'route', path, hash: hash || 'tasks' }); return }
  currentBase = base
  currentPath = pathname; generation++; controller.abort(); controller = new AbortController(); pending.clear(); deadline !== undefined && clearTimeout(deadline)
  if (props.adapterId === 'video-iframe-v1') emit('taskLabel', { route: props.route, label: '加载中' })
  state.value = 'loading'; message.value = '正在加载应用…'
  src.value = base + (props.adapterId === 'image-iframe-v1' ? '/dashboard' : pathname) + '?embed=1&theme=' + theme() + '&view=' + generation + (hash ? '#' + hash : '')
  deadline = setTimeout(() => failed('应用尚未响应，请重试。'), 20000)
}
async function receive(event: MessageEvent) {
  const a = adapter.value, data = event.data
  if (disposed || !a || event.source !== frame.value?.contentWindow || event.origin !== 'null' || !data || data.channel !== a.channel || data.view !== String(generation)) return
  if (data.type === 'media-close') { closePreview(); return }
  if (data.type === 'media-preview') {
    if (!Number.isSafeInteger(data.id) || data.id < 0) return
    const epoch = generation, revision = ++mediaRevision
    try {
      validateMediaPreview(data)
      await props.authorize()
      if (disposed || epoch !== generation || revision !== mediaRevision) return
      openMediaPreview(mediaOwner, data, () => { if (!disposed && epoch === generation) post({ type: 'media-closed', id: data.id }) })
      post({ type: 'media-result', id: data.id, ok: true })
    } catch { if (!disposed && epoch === generation) post({ type: 'media-result', id: data.id, ok: false }) }
    return
  }
  if (data.type === 'ready') {
    deadline !== undefined && clearTimeout(deadline); state.value = 'ready'; message.value = ''; post({ type: 'theme', theme: theme() }); post({ type: 'restore', state: viewState })
    if (a.channel === 'vf-image') {
      const path = new URL(props.route, location.origin).searchParams.get(a.query)!
      post({ type: 'route', path, hash: path.split('#')[1] || 'tasks' })
    }
    return
  }
  if (data.type === 'navigate' && typeof data.path === 'string' && (a.channel === 'vf-video' ? videoPath : /^\/dashboard(?:\/tasks\/[a-zA-Z0-9_-]+)?(?:#(?:tasks|models|mcp))?$/).test(data.path)) {
    if (data.state && typeof data.state === 'object') viewState = data.state
    const current = new URL(props.route, location.origin).searchParams.get(a.query)
    if (props.adapterId === 'video-iframe-v1' && (data.path === current || data.path === '/dashboard#tasks' && current === '/dashboard')) {
      if (isTaskList(props.route) && data.state) { listRendered = true; restoreListScroll() }
      return
    }
    emit('navigate', '/market?' + a.query + '=' + encodeURIComponent(data.path)); return
  }
  if (data.type === 'resize' && Number.isFinite(data.height)) { if (frame.value) frame.value.style.height = Math.max(600, Math.min(100000, data.height)) + 'px'; restoreListScroll(); return }
  if (data.type === 'wheel' && Number.isFinite(data.deltaY)) {
    if (Number.isSafeInteger(data.captureId) && data.captureId > 0) {
      post({ type: 'restore', viewport: { captureId: data.captureId, scrollY: window.scrollY } }); return
    }
    window.scrollBy({ top: Math.max(-1000, Math.min(1000, data.deltaY)), behavior: 'auto' }); return
  }
  if (data.type !== 'request' || !Number.isSafeInteger(data.id) || data.id < 0 || typeof data.path !== 'string' || pending.has(data.id) || pending.size >= 32) return
  const epoch = generation, method = data.method ?? 'GET', signal = controller.signal
  const reply = (value: object) => { if (!disposed && epoch === generation && !signal.aborted) post({ type: 'response', id: data.id, ...value }) }
  if (!validMediaRequest(a, method, data.path, data.body)) { reply({ status: 403, body: new ArrayBuffer(0) }); return }
  pending.add(data.id)
  try {
    try { await props.authorize() }
    catch {
      if (!disposed && epoch === generation && !signal.aborted && props.adapterId === 'video-iframe-v1' && data.path === `/api/tasks/${videoPage(props.route).taskId}`) emit('taskLabel', { route: props.route, label: '任务不可用' })
      reply({ status: 503, body: new ArrayBuffer(0) }); return
    }
    if (disposed || epoch !== generation || signal.aborted) return
    const options: RequestInit = { method, credentials: props.resourceBase ? 'omit' : 'same-origin', cache: 'no-store', redirect: 'error',
      headers: method === 'POST' ? { 'Content-Type': 'application/json' } : {}, body: method === 'POST' ? data.body : undefined,
      signal: AbortSignal.any([signal, AbortSignal.timeout(90000)]) }
    const response = props.resourceBase ? await fetchLegacyMedia(fetch, props.resourceBase, data.path, options) : await fetch(a.prefix + data.path, options)
    const body = await response.arrayBuffer()
    if (disposed || epoch !== generation || signal.aborted) return
    if (props.adapterId === 'video-iframe-v1') {
      let task: unknown = null
      const requestedTask = /^\/api\/(?:higgsfield\/)?tasks\/([a-zA-Z0-9_-]+)$/.exec(data.path)?.[1]
      if (response.status === 200 && requestedTask) {
        try { task = JSON.parse(new TextDecoder().decode(body)) } catch { /* Invalid payload never supplies a name. */ }
        if (task && typeof task === 'object' && (task as Record<string, unknown>).video_task_id === requestedTask) {
          if (taskNames.size >= 100) taskNames.clear()
          taskNames.set(requestedTask, videoTaskName(task))
        }
      } else if (requestedTask) {
        taskNames.delete(requestedTask)
      }
      const label = videoTaskLabel(props.route, data.path, response.status, task)
      if (label !== null) emit('taskLabel', { route: props.route, label })
    }
    const titleEdit = props.adapterId === 'video-iframe-v1' && method === 'POST' && /^\/api\/tasks\/[a-zA-Z0-9_-]+\/title$/.test(data.path)
    // Video project denials and quote conflicts are business errors. Keep the
    // workspace open so members can select another project or refresh a quote.
    if ([401, 410].includes(response.status) || a.channel !== 'vf-video' && !titleEdit && [403, 409].includes(response.status)) { failed('当前应用访问权限或入口已失效，请返回后重新打开。'); emit('error', message.value) }
    reply({ status: response.status, contentType: response.headers.get('Content-Type') || 'application/octet-stream', body })
  } catch {
    if (!disposed && epoch === generation && !signal.aborted && props.adapterId === 'video-iframe-v1' && data.path === `/api/tasks/${videoPage(props.route).taskId}`) emit('taskLabel', { route: props.route, label: '任务不可用' })
    reply({ status: 503, body: new ArrayBuffer(0) })
  }
  finally { if (epoch === generation) pending.delete(data.id) }
}
const observer = new MutationObserver(() => { if (!disposed) post({ type: 'theme', theme: theme() }) })
watch(() => [props.adapterId, props.route, props.resourceBase], () => load())
onMounted(() => {
  if (props.adapterId === 'video-iframe-v1') {
    previousScrollRestoration = window.history.scrollRestoration
    window.history.scrollRestoration = 'manual'
  }
  window.addEventListener('message', receive); observer.observe(document.body, { attributes: true, attributeFilter: ['data-theme'] }); load()
})
onBeforeUnmount(() => {
  closePreview()
  disposed = true; generation++; cancelAnimationFrame(scrollFrame); controller.abort(); deadline !== undefined && clearTimeout(deadline); pending.clear(); observer.disconnect(); window.removeEventListener('message', receive)
  if (previousScrollRestoration !== undefined && window.history.scrollRestoration === 'manual') window.history.scrollRestoration = previousScrollRestoration
})
</script>

<template>
  <section class="legacy-media-host" :aria-label="title">
    <div class="legacy-media-status"><p v-if="message" role="status">{{ message }}</p><button v-if="state === 'error'" class="btn" @click="resourceBase ? emit('reconnect') : load(true)">重新连接</button></div>
    <iframe :key="adapterId === 'video-iframe-v1' ? src : adapterId" ref="frame" :src="src" :title="title" data-embedded="true" sandbox="allow-scripts allow-downloads" referrerpolicy="no-referrer" />
  </section>
</template>

<style scoped>
.legacy-media-host { display: flex; flex-direction: column; min-width: 0; }
.legacy-media-host iframe { width: 100%; min-height: 600px; border: 0; background: transparent; }
.legacy-media-status { display: flex; align-items: center; gap: 12px; color: var(--muted); }
</style>
