<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { DashboardCache } from './cache'
import { dashboardCacheKey, validateLaunch } from './launch'
import type { LaunchPolicy } from './launch'
import type { AppContext, SDKAdapters } from './types'
import { createAppSDK } from './sdk'
import { mountWujieDashboard } from './wujie-adapter'
import type { WujieDashboard } from './wujie-adapter'

interface Selection {
  launch: unknown
  context: AppContext
  policy: LaunchPolicy
  authorizationEpoch: string
  adapters: SDKAdapters
  route?: string
  releaseSession?(): Promise<void>
}
const props = defineProps<{
  selection: Selection | null
  confirmDiscard: () => Promise<boolean>
  visible?: boolean
}>()
const emit = defineEmits<{ error: [code: string]; ready: []; retry: []; cancelled: [] }>()
const surface = ref<HTMLElement>()
const state = ref<'empty' | 'loading' | 'ready' | 'error'>('empty')
const cache = new DashboardCache<WujieDashboard>(3)
let epoch = '', revision = 0
let activeKey: string | undefined

watch([() => props.selection, surface, () => props.visible], async ([selection, container, visible]) => {
  const current = ++revision
  if (!container) return
  const nextEpoch = selection ? JSON.stringify([selection.context.userId, selection.context.organizationId,
    selection.context.stationId, selection.authorizationEpoch]) : ''
  try {
    if (nextEpoch !== epoch || !selection) {
      epoch = nextEpoch
      await cache.revoke()
    }
    if (current !== revision) return
    if (!selection) { state.value = 'empty'; return }
    if (visible === false) { await cache.hideActive(); return }
    state.value = 'loading'
    const launch = validateLaunch(selection.launch, selection.policy)
    const key = dashboardCacheKey(launch, selection.context, selection.authorizationEpoch)
    await cache.activate(key, async signal => {
      const session = createAppSDK(launch, selection.context, selection.adapters)
      session.setRoute({ path: selection.route ?? '/', revision: current })
      let released = false
      const releaseSession = async () => {
        if (released) return
        released = true
        await selection.releaseSession?.()
      }
      try {
        return await mountWujieDashboard({ container, launch, session, signal, confirmDiscard: props.confirmDiscard,
          releaseSession,
          onInvalidated() {
            void cache.invalidate(key).catch(() => emit('error', 'DASHBOARD_DISPOSE_FAILED'))
            if (activeKey === key) { state.value = 'error'; emit('error', 'SESSION_REVOKED') }
          },
        })
      } catch (error) { session.dispose(); await releaseSession(); throw error }
    }, async entry => {
      await entry.updateContext(selection.context)
      entry.updateRoute({ path: selection.route ?? '/', revision: current })
    })
    if (current === revision) { activeKey = key; state.value = 'ready'; emit('ready') }
  } catch (error) {
    if (current !== revision) return
    // Emit only a bounded code; raw backend errors may contain internal URLs.
    const message = error instanceof Error ? error.message : ''
    if (message === 'CONTEXT_CHANGE_CANCELLED' || message === 'CACHE_CLOSE_CANCELLED') {
      state.value = 'ready'
      emit('cancelled')
      return
    }
    state.value = 'error'
    emit('error', /^[A-Z][A-Z_]{0,63}$/.test(message) ? message : 'DASHBOARD_LOAD_FAILED')
  }
}, { flush: 'post' })

defineExpose({
  prepareLeave: () => cache.prepareLeave(),
  closeActive: async () => !activeKey || await cache.close(activeKey),
  revoke: () => cache.revoke(),
})

onBeforeUnmount(() => {
  revision++
  void cache.revoke().catch(() => emit('error', 'DASHBOARD_DISPOSE_FAILED'))
})
</script>

<template>
  <section class="app-dashboard-host" :aria-busy="state === 'loading'">
    <p v-if="state === 'empty'" role="status">选择应用以打开控制台。</p>
    <p v-else-if="state === 'loading'" role="status">正在加载应用控制台…</p>
    <div v-else-if="state === 'error'" role="alert">
      <p>应用控制台加载失败，请重试。</p>
      <button class="btn" @click="emit('retry')">重试</button>
    </div>
    <div ref="surface" v-show="state === 'ready'" />
  </section>
</template>

<style scoped>
.app-dashboard-host { min-width: 0; width: 100%; color: var(--ink); }
.app-dashboard-host p { color: var(--muted); }
</style>
