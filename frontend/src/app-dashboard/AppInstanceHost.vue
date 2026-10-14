<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { createDashboardWebClient } from './web-client'

const props = defineProps<{ workspaceSessionId: string; installationId: string }>()
const emit = defineEmits<{ (e: 'loaded', appId: string): void }>()
const client = createDashboardWebClient(location.origin)
const frame = ref<HTMLIFrameElement>(), entry = ref(''), message = ref('正在打开实例工作区…')
const lifetime = new AbortController()
let port: MessagePort | undefined, expiry: ReturnType<typeof setTimeout> | undefined, loading: ReturnType<typeof setTimeout> | undefined
let closed = false, pendingClose: Promise<boolean> | undefined
let finishClose: ((value: boolean) => void) | undefined, closeId = ''
const theme = () => document.body.dataset.theme === 'dark' ? 'dark' : 'light'
const observer = new MutationObserver(() => port?.postMessage({kind:'theme',theme:theme()}))
function initialize(event: MessageEvent) {
  if (port || event.source !== frame.value?.contentWindow || event.origin !== location.origin || event.data?.kind !== 'instance-workspace:hello') return
  const channel = new MessageChannel(); port = channel.port1
  port.onmessage = ({data}) => {
    if (data?.kind === 'status') {
      if (data.state === 'ready' || data.state === 'error') clearTimeout(loading)
      message.value = data.state === 'error' ? '实例工作区连接中断，请保存或导出后重新打开。' : ''
    }
    if (data?.requestId === closeId && (data.kind === 'closed' || data.kind === 'close-error')) finishClose?.(data.kind === 'closed')
  }
  frame.value!.contentWindow!.postMessage({kind:'instance-workspace:init',theme:theme()},location.origin,[channel.port2])
}
function prepareClose(): Promise<boolean> {
  if (closed || !port) return Promise.resolve(true)
  if (pendingClose) return pendingClose
  pendingClose = new Promise<boolean>(resolve => {
    const timer = setTimeout(() => finishClose?.(false), 30000)
    finishClose = value => { clearTimeout(timer); finishClose = undefined; if (value) closed = true; resolve(value) }
    closeId = crypto.randomUUID()
    port!.postMessage({kind:'close',requestId:closeId})
  }).then(saved => {
    if (lifetime.signal.aborted) return false
    closed = saved || window.confirm('实例工作区未能完成保存。确定关闭并放弃尚未保存的修改吗？')
    return closed
  })
    .finally(() => { pendingClose = undefined })
  return pendingClose
}
defineExpose({prepareClose})
onMounted(async () => {
  window.addEventListener('message', initialize)
  observer.observe(document.body,{attributes:true,attributeFilter:['data-theme']})
  try {
    const launch = await client.inspectWorkspace(props.workspaceSessionId, props.installationId, lifetime.signal)
    if (lifetime.signal.aborted) return
    emit('loaded', launch.appId)
    entry.value = launch.entryUrl
    message.value = ''
    loading = setTimeout(() => { message.value = '实例工作区加载超时，请关闭后重试。' }, 30000)
    expiry = setTimeout(() => { lifetime.abort(); entry.value = ''; port?.close(); port = undefined; finishClose?.(false); message.value = '实例工作区会话已过期，请重新打开。' }, Math.max(0, Date.parse(launch.expiresAt)-client.now()))
  } catch { if (!lifetime.signal.aborted) message.value = '实例工作区暂不可用，请返回控制台重新打开。' }
})
onBeforeUnmount(() => {
  lifetime.abort(); clearTimeout(expiry); clearTimeout(loading); observer.disconnect(); port?.close(); finishClose?.(false)
  window.removeEventListener('message', initialize)
  void client.closeWorkspace(props.workspaceSessionId, AbortSignal.timeout(5000)).catch(()=>undefined)
})
</script>

<template>
  <section class="instance-workspace">
    <p v-if="message" role="status">{{ message }}</p>
    <iframe v-if="entry" ref="frame" :src="entry" title="应用实例工作区" sandbox="allow-scripts allow-same-origin allow-downloads" referrerpolicy="no-referrer" />
  </section>
</template>

<style scoped>
.instance-workspace { min-width: 0; width: 100%; }
.instance-workspace p { margin: 0 0 8px 0; color: var(--muted); }
.instance-workspace iframe { display: block; width: 100%; height: max(520px, calc(100dvh - 64px)); border: 1px solid var(--line); border-radius: 10px; background: var(--panel); }
</style>
