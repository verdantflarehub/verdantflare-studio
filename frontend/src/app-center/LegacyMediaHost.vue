<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import type { LegacyAdapterID } from './legacy'
import { mediaAdapter, validMediaRequest } from './legacy-media'
import { fetchLegacyMedia } from './legacy-media-transfer'

const props = defineProps<{ adapterId: LegacyAdapterID; route: string; title: string; resourceBase?: string; authorize: () => Promise<void> }>()
const emit = defineEmits<{ navigate: [route: string]; error: [message: string]; reconnect: [] }>()
const frame = ref<HTMLIFrameElement>(), src = ref(''), state = ref<'loading' | 'ready' | 'error'>('loading'), message = ref('')
const adapter = computed(() => mediaAdapter(props.adapterId))
let generation = 0, disposed = false, controller = new AbortController(), deadline: ReturnType<typeof setTimeout> | undefined
let currentPath = '', viewState: unknown = null
let currentBase = ''
const pending = new Set<number>()
const theme = () => document.body.dataset.theme === 'light' ? 'light' : 'dark'
function post(value: object) { frame.value?.contentWindow?.postMessage({ channel: 'vf-studio', ...value }, '*') }
function failed(text: string) { state.value = 'error'; message.value = text; clearTimeout(deadline) }
function load(force = false) {
  const a = adapter.value
  if (!a) return
  const path = new URL(props.route, location.origin).searchParams.get(a.query)!
  const [pathname, hash] = path.split('#')
  const base = props.resourceBase ?? a.prefix
  if (!force && currentBase === base && currentPath === pathname && src.value && state.value === 'ready') { post({ type: 'route', hash: hash || 'tasks' }); return }
  currentBase = base
  currentPath = pathname; generation++; controller.abort(); controller = new AbortController(); pending.clear(); clearTimeout(deadline)
  state.value = 'loading'; message.value = '正在加载应用…'
  src.value = base + pathname + '?embed=1&theme=' + theme() + '&view=' + generation + (hash ? '#' + hash : '')
  deadline = setTimeout(() => failed('应用尚未响应，请重试。'), 20000)
}
async function receive(event: MessageEvent) {
  const a = adapter.value, data = event.data
  if (disposed || !a || event.source !== frame.value?.contentWindow || event.origin !== 'null' || !data || data.channel !== a.channel || data.view !== String(generation)) return
  if (data.type === 'ready') { clearTimeout(deadline); state.value = 'ready'; message.value = ''; post({ type: 'theme', theme: theme() }); post({ type: 'restore', state: viewState }); return }
  if (data.type === 'navigate' && typeof data.path === 'string' && /^\/dashboard(?:\/tasks\/[a-zA-Z0-9_-]+)?(?:#(?:tasks|models|mcp))?$/.test(data.path)) {
    if (data.state && typeof data.state === 'object') viewState = data.state
    emit('navigate', '/market?' + a.query + '=' + encodeURIComponent(data.path)); return
  }
  if (data.type === 'resize' && Number.isFinite(data.height)) { if (frame.value) frame.value.style.height = Math.max(600, Math.min(100000, data.height)) + 'px'; return }
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
    catch { reply({ status: 503, body: new ArrayBuffer(0) }); return }
    if (disposed || epoch !== generation || signal.aborted) return
    const options: RequestInit = { method, credentials: props.resourceBase ? 'omit' : 'same-origin', cache: 'no-store', redirect: 'error',
      headers: method === 'POST' ? { 'Content-Type': 'application/json' } : {}, body: method === 'POST' ? data.body : undefined,
      signal: AbortSignal.any([signal, AbortSignal.timeout(90000)]) }
    const response = props.resourceBase ? await fetchLegacyMedia(fetch, props.resourceBase, data.path, options) : await fetch(a.prefix + data.path, options)
    if ([401, 403, 409, 410].includes(response.status)) { failed('当前应用访问权限或入口已失效，请返回后重新打开。'); emit('error', message.value) }
    reply({ status: response.status, contentType: response.headers.get('Content-Type') || 'application/octet-stream', body: await response.arrayBuffer() })
  } catch { reply({ status: 503, body: new ArrayBuffer(0) }) }
  finally { if (epoch === generation) pending.delete(data.id) }
}
const observer = new MutationObserver(() => { if (!disposed) post({ type: 'theme', theme: theme() }) })
watch(() => [props.adapterId, props.route, props.resourceBase], () => load())
onMounted(() => { window.addEventListener('message', receive); observer.observe(document.body, { attributes: true, attributeFilter: ['data-theme'] }); load() })
onBeforeUnmount(() => { disposed = true; generation++; controller.abort(); clearTimeout(deadline); pending.clear(); observer.disconnect(); window.removeEventListener('message', receive) })
</script>

<template>
  <section class="legacy-media-host" :aria-label="title">
    <div class="legacy-media-status"><p v-if="message" role="status">{{ message }}</p><button v-if="state === 'error'" class="btn" @click="resourceBase ? emit('reconnect') : load(true)">重新连接</button></div>
    <iframe ref="frame" :src="src" :title="title" data-embedded="true" sandbox="allow-scripts allow-downloads" referrerpolicy="no-referrer" />
  </section>
</template>

<style scoped>
.legacy-media-host { display: flex; flex-direction: column; min-width: 0; }
.legacy-media-host iframe { width: 100%; min-height: 600px; border: 0; background: transparent; }
.legacy-media-status { display: flex; align-items: center; gap: 12px; color: var(--muted); }
</style>
