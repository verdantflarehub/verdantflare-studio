<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRouter } from 'vue-router'
import type { RouteLocationRaw } from 'vue-router'
import { useHostStore } from '../platform/hostStore'
import { useAppCenterStore } from './store'
import { mountEditor } from '../pages/comfyui-editor.js'
import { comfyuiManagementCard, comfyuiManagementLocation, readComfyUIBookmark } from './comfyui-bookmark'
import type { ComfyUIBookmark } from './comfyui-bookmark'
import { reasonLabel } from './labels'

const props = defineProps<{ target: ComfyUIBookmark | null; returnLocation: RouteLocationRaw }>()
const router = useRouter(), host = useHostStore(), store = useAppCenterStore()
const container = ref<HTMLElement>(), instance = ref<{ alias: string; name: string } | null>(null)
const error = ref(''), status = ref(''), loading = ref(false), retry = ref(0)
let pending: AbortController | undefined, generation = 0, editor: ReturnType<typeof mountEditor> | undefined
const identity = computed(() => JSON.stringify([host.connected, host.identity.user_id, host.identity.organization_id,
  host.identity.station_id, host.identity.session_id, host.identity.policy_version, host.identity.revocation_version,
  host.identity.roles, host.identity.scopes]))
function clear() { editor?.dispose(); editor = undefined; instance.value = null; status.value = '' }
async function prepareLeave() {
  try { await editor?.close(); return true }
  catch { status.value = '草稿暂未保存，请保持此页并重试；也可在 ComfyUI 中导出工作流。'; return false }
}
onBeforeRouteLeave(prepareLeave)
onBeforeRouteUpdate(async (to, from) => to.fullPath === from.fullPath || await prepareLeave())
watch(() => [props.target?.alias, identity.value, retry.value], async () => {
  const epoch = ++generation, target = props.target
  pending?.abort(); clear(); error.value = ''; loading.value = !!target
  if (!target || !host.connected) { loading.value = false; return }
  const controller = new AbortController(); pending = controller
  const signal = AbortSignal.any([controller.signal, AbortSignal.timeout(30000)])
  const current = () => epoch === generation && !signal.aborted
  try {
    if (target.alias === 'instances') {
      await store.refresh()
      if (!current()) return
      if (store.state.error) throw new Error(store.state.error)
      const view = store.state.value
      if (!view) throw new Error('APPCENTER_DEPENDENCY_UNAVAILABLE')
      const card = comfyuiManagementCard(view), action = card.actions.find(item => item.kind === 'open')!
      if (!store.canExecute(card.cardKey, action.id, card.bindingRevision)) throw new Error('APPCENTER_OPEN_UNAVAILABLE')
      if (await router.replace(comfyuiManagementLocation(card))) throw new Error('APPCENTER_OPEN_UNAVAILABLE')
    } else {
      const resolved = await readComfyUIBookmark(target.alias, signal)
      if (!current()) return
      instance.value = resolved
      await nextTick()
      if (!current() || !container.value) return
      editor = mountEditor({ alias: resolved.alias, container: container.value,
        onStatus: (_state: string, message: string) => { if (epoch === generation) status.value = message } })
    }
  } catch (cause) {
    if (epoch === generation && !controller.signal.aborted) error.value = reasonLabel(cause instanceof Error ? cause.message : 'APPCENTER_OPEN_UNAVAILABLE')
  } finally { if (epoch === generation) loading.value = false }
}, { immediate: true, flush: 'sync' })
onBeforeUnmount(() => { generation++; pending?.abort(); clear() })
</script>

<template>
  <main v-if="target" class="main embed-active comfyui-bookmark"><div class="workspace">
    <nav class="dashboard-breadcrumb" aria-label="面包屑"><span>ComfyUI</span><span v-if="instance"> / {{ instance.name }}</span></nav>
    <header class="heading"><div class="heading-left"><h1>{{ instance?.name || 'ComfyUI' }}</h1></div><div class="heading-actions"><button v-if="instance" class="btn" @click="async () => { if (await prepareLeave()) retry++ }">重新连接</button><button class="btn" @click="router.push(returnLocation)">返回应用中心</button></div></header>
    <p v-if="host.sessionStatus === 'loading' || loading" role="status">正在打开 ComfyUI…</p>
    <div v-else-if="!host.connected" class="panel bookmark-message"><p>请登录以打开应用。</p><button class="btn" @click="host.loginOpen = true">登录 Station</button></div>
    <div v-if="error" class="panel bookmark-message" role="alert"><p>{{ error }}</p><button class="btn" @click="retry++">重试</button></div>
    <p v-if="status" role="status">{{ status }}</p>
    <div v-if="instance" ref="container" class="comfyui-editor" />
  </div></main>
</template>

<style scoped>
.dashboard-breadcrumb { display: flex; gap: 6px; margin-bottom: 6px; color: var(--muted); font-size: 13px; }
.dashboard-breadcrumb a { color: var(--muted); text-decoration: none; }
.bookmark-message { margin: 20px 0; padding: 20px; color: var(--muted); }
.comfyui-editor { width: 100%; min-height: 520px; }
.comfyui-editor :deep(iframe) { display: block; width: 100%; height: max(520px, calc(100dvh - 160px)); border: 0; }
</style>
