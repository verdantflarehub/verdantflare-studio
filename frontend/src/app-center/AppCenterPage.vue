<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useHostStore } from '../platform/hostStore'
import { useAppCenterStore } from './store'
import { selectCards } from './controller'
import { directoryLocation, directoryView } from './directory-route'
import { actionLabels, reasonLabel } from './labels'
import type { AppCenterCard } from './schema.generated'
import AppCard from './AppCard.vue'
import AppCommandConfirm from './AppCommandConfirm.vue'
import { versionLabel } from './card-actions'
import AppDashboardWorkspace from '../app-dashboard/AppDashboardWorkspace.vue'
import { createDashboardWebClient } from '../app-dashboard/web-client'
import { dashboardLocation } from '../app-dashboard/routes'
import AppInstallDetail from './AppInstallDetail.vue'
import AppCommandRecovery from './AppCommandRecovery.vue'
import LegacyWorkspace from './LegacyWorkspace.vue'
import ComfyUIBookmarkWorkspace from './ComfyUIBookmarkWorkspace.vue'
import { comfyuiBookmark } from './comfyui-bookmark'
import { createLegacyClient, legacyLocation, legacyTarget } from './legacy'
import type { LegacyOpened } from './legacy'

const host = useHostStore(), store = useAppCenterStore(), route = useRoute(), router = useRouter()
store.setVisible('directory', true)
const query = ref(''), group = ref('all'), filter = ref(directoryView(route.query.view)), message = ref(''), menuKey = ref('')
watch(() => [route.path, route.query.view] as const, ([path, view]) => {
  if (path === '/market') filter.value = directoryView(view)
}, { immediate: true })
const returnLocation = computed(() => directoryLocation(filter.value))
const confirmation = ref<{ card: AppCenterCard; actionId: string; kind: string; scope: string; impact: string } | null>(null)
const submitting = ref(false)
async function confirmCommand() {
  const pending = confirmation.value
  if (!pending || submitting.value) return
  if (pending.scope !== JSON.stringify(store.state.value?.scope) || !store.canExecute(pending.card.cardKey, pending.actionId, pending.card.bindingRevision)) {
    confirmation.value = null; message.value = reasonLabel('APPCENTER_BINDING_CHANGED'); return
  }
  submitting.value = true
  try { await store.execute(pending.card, pending.actionId); message.value = '' }
  catch (error) { message.value = reasonLabel(error instanceof Error ? error.message : 'APPCENTER_SUBMISSION_UNKNOWN') }
  finally { submitting.value = false; confirmation.value = null }
}
async function refreshPage() { if (!host.connected) await host.init(); else await store.refresh(true) }
const selectedKey = computed(() => typeof route.query.card === 'string' ? route.query.card : '')
const selected = computed(() => store.state.value?.items.find(card => card.cardKey === selectedKey.value))
const businessRoute = computed(() => route.name === 'app-dashboard' || route.name === 'app-instance')
const legacy = computed(() => {
  try { return { target: legacyTarget(route.fullPath), comfyui: comfyuiBookmark(route.fullPath), error: '' } }
  catch (error) { return { target: null, comfyui: null, error: error instanceof Error ? error.message : 'INVALID_LEGACY_ROUTE' } }
})
const installationId = computed(() => typeof route.params.installationId === 'string' ? route.params.installationId : '')
const titles = computed(() => Object.fromEntries((store.state.value?.items ?? []).map(card => [card.appId, card.display.name])))
const dashboardClient = createDashboardWebClient(location.origin)
const legacyClient = createLegacyClient()
async function authorizeDashboard(id: string, signal: AbortSignal) {
  await store.refresh()
  if (signal.aborted) throw new Error('REQUEST_CANCELLED')
  const card = store.state.value?.items.find(item => item.installation?.installationId === id)
  const action = card?.actions.find(item => item.kind === 'open')
  if (!card || !action || !store.canExecute(card.cardKey, action.id, card.bindingRevision)) throw new Error('APPCENTER_OPEN_UNAVAILABLE')
}
async function openDashboard(id: string, signal?: AbortSignal) {
  const view = store.state.value, card = view?.items.find(item => item.installation?.installationId === id)
  const action = card?.actions.find(item => item.kind === 'open')
  if (!view || !card || !action?.entryId || !store.canExecute(card.cardKey, action.id, card.bindingRevision)) throw new Error('APPCENTER_OPEN_UNAVAILABLE')
  const opened = await dashboardClient.openAppCenter({ schemaVersion: 1, cardKey: card.cardKey, entryId: action.entryId,
    expectedViewRevision: view.revision, expectedBindingRevision: card.bindingRevision }, id, signal)
  const current = store.state.value
  if (signal?.aborted || !current || JSON.stringify(current.scope) !== JSON.stringify(view.scope)
    || opened.context.contextRevision !== view.scope.authorizationEpoch || opened.context.userId !== view.scope.userId
    || opened.context.organizationId !== view.scope.organizationId || opened.context.stationId !== view.scope.stationId
    || opened.launch.appVersion !== card.installedVersion || opened.launch.appId !== card.appId) {
    await dashboardClient.close(opened.launch.dashboardSessionId, AbortSignal.timeout(5000)).catch(() => undefined)
    throw new Error('APPCENTER_BINDING_CHANGED')
  }
  return opened
}
const result = computed(() => selectCards(store.state.value, query.value, group.value, filter.value))
const groups = computed(() => {
  const names = new Map((store.state.value?.groups ?? []).map(item => [item.id, item.label]))
  for (const card of store.state.value?.items ?? []) if (!names.has(card.display.groupId)) names.set(card.display.groupId, '未分类')
  return Array.from(names, ([id, label]) => ({ id, label }))
})
const groupMeta: Record<string, { title: string; subtitle: string }> = {
  image: { title: '图像应用', subtitle: 'Image' },
  video: { title: '视频制作', subtitle: 'Video' },
  music: { title: '音频制作', subtitle: 'Music' },
  desktop: { title: '创作工具', subtitle: 'Desktop Tools' },
  system: { title: '系统应用', subtitle: 'System' },
}
const visibleSections = computed(() => {
  const items = result.value.items
  const groupOrder = new Map((store.state.value?.groups ?? []).map((g, idx) => [g.id, g.order ?? idx]))
  const map = new Map<string, AppCenterCard[]>()
  for (const card of items) {
    const gid = card.display.groupId || 'uncategorized'
    let list = map.get(gid)
    if (!list) {
      list = []
      map.set(gid, list)
    }
    list.push(card)
  }
  const sections: { id: string; title: string; subtitle: string; order: number; items: AppCenterCard[] }[] = []
  for (const [gid, cardList] of map.entries()) {
    if (!cardList.length) continue
    const meta = groupMeta[gid]
    const label = groups.value.find(g => g.id === gid)?.label ?? gid
    const order = groupOrder.get(gid) ?? 999
    sections.push({
      id: gid,
      title: meta?.title ?? label,
      subtitle: meta?.subtitle ?? gid.toUpperCase(),
      order,
      items: cardList
    })
  }
  sections.sort((a, b) => a.order - b.order)
  return sections
})
let listScroll = 0
function back() { void router.replace(returnLocation.value) }
async function perform(card: AppCenterCard, actionId: string) {
  const action = card.actions.find(item => item.id === actionId)
  if (actionId === 'detail' || action?.kind === 'progress' && action.enabled) {
    await router.push({ query: { ...route.query, card: card.cardKey } }); return
  }
  if (action?.kind === 'refresh') { await store.refresh(); return }
  if (!action) return
  if (!store.canExecute(card.cardKey, actionId, card.bindingRevision)) {
    if (store.state.stale) {
      await store.refresh()
      const freshCard = store.state.value?.items.find(item => item.cardKey === card.cardKey)
      if (freshCard) {
        card = freshCard
        const freshAction = card.actions.find(item => item.id === actionId)
        if (!freshAction || !store.canExecute(card.cardKey, actionId, card.bindingRevision)) {
          message.value = reasonLabel(freshAction?.reasonCode ?? (action.kind === 'open' ? 'APPCENTER_OPEN_UNAVAILABLE' : '')) || '应用状态已更新，请重新操作。'
          return
        }
      } else {
        message.value = reasonLabel('APPCENTER_OPEN_UNAVAILABLE')
        return
      }
    } else {
      const code = action.reasonCode ?? (action.kind === 'open' ? 'APPCENTER_OPEN_UNAVAILABLE' : '')
      if (code) message.value = reasonLabel(code)
      return
    }
  }
  if (['install', 'start', 'stop', 'restart', 'delete'].includes(action.kind)) {
    const impacts: Record<string, string> = {
      install: '将安装此版本的应用管理服务。', start: '将启动应用管理服务。',
      stop: '将停止应用管理服务，相关业务可能暂时无法访问。此操作与关闭页面独立，不会批量停止工作实例；活动任务和依赖保护由服务端检查。', restart: '将重启应用管理服务，相关业务可能暂时中断。',
      delete: '将移除此应用安装。项目资料、历史任务和产物保留，模型清理独立进行。请先保存当前工作；活动任务和依赖保护由服务端检查。',
    }
    if (confirmation.value || submitting.value) return
    confirmation.value = { card: structuredClone(card), actionId, kind: action.kind, scope: JSON.stringify(store.state.value?.scope), impact: impacts[action.kind] }
    return
  }
  if (action.kind === 'open' && card.entry?.transport === 'dashboard' && card.installation) {
    store.pinOpen(card.installation.installationId, card.bindingRevision)
    const failed = await router.push(dashboardLocation(card.installation.installationId))
    if (failed) store.takeOpenBinding(card.installation.installationId)
    return
  }
  if (action.kind === 'open' && card.entry?.transport === 'legacy') {
    const view = store.state.value
    if (!view) return
    let opened: LegacyOpened | undefined = undefined, adopted = false
    try {
      const result = await legacyClient.open(view, card, AbortSignal.timeout(30000))
      opened = result
      if (!store.state.value || JSON.stringify(store.state.value.scope) !== JSON.stringify(view.scope) || !store.canExecute(card.cardKey, action.id, card.bindingRevision)) throw new Error('APPCENTER_BINDING_CHANGED')
      store.pinOpen(card.cardKey, card.bindingRevision, opened)
      const failed = await router.push(legacyLocation(opened, card.cardKey))
      if (failed) store.takeOpenBinding(card.cardKey)
      else adopted = true
    } catch (error) {
      message.value = reasonLabel(error instanceof Error ? error.message : 'APPCENTER_OPEN_UNAVAILABLE')
    } finally {
      if (opened && !adopted) void legacyClient.close(opened)
    }
    return
  }
  // The current directory delivery does not promise an unconnected mutation or
  // entry adapter. This dispatcher will gain only the finite contracted kinds.
  message.value = '此操作暂不可用，请稍后重试。'
}
const directoryVisible = computed(() => !selectedKey.value && !businessRoute.value && !legacy.value.target && !legacy.value.comfyui && !legacy.value.error)
watch(directoryVisible, async (visible, previous) => {
  if (!visible && previous) listScroll = window.scrollY
  await nextTick()
  window.scrollTo({ top: visible ? listScroll : 0 })
})
watch(() => JSON.stringify([host.identity.user_id, host.identity.organization_id, host.identity.station_id]), () => {
  query.value = ''; group.value = 'all'; message.value = ''; menuKey.value = ''; confirmation.value = null; listScroll = 0
})
watch(() => JSON.stringify(store.state.value?.scope), () => { confirmation.value = null; menuKey.value = '' })
watch([filter, group, query, selectedKey, businessRoute], () => { menuKey.value = '' })
watch(selectedKey, key => store.setDetail(key), { immediate: true })
watch(() => !businessRoute.value && !legacy.value.target && !legacy.value.comfyui && !legacy.value.error, active => store.setProgressVisible(active), { immediate: true })
onBeforeUnmount(() => { store.setDetail(''); store.setVisible('directory', false) })
</script>

<template>
  <main v-show="!businessRoute && !legacy.target && !legacy.comfyui && !legacy.error" class="main appcenter-page"><div class="workspace">
    <header class="heading"><div class="heading-left"><h1>{{ filter === 'installed' ? '我的应用' : '所有应用' }}</h1><p>管理 Station 应用、安装与运行状态。</p></div><div class="heading-actions"><button class="btn" :disabled="store.state.refreshing || host.sessionStatus === 'loading'" @click="refreshPage">{{ store.state.refreshing || host.sessionStatus === 'loading' ? '刷新中…' : '刷新' }}</button></div></header>
    <div v-if="!host.connected" class="panel appcenter-message" role="status">
      <p v-if="host.sessionStatus === 'loading'">正在恢复登录状态…</p>
      <template v-else-if="host.sessionStatus === 'error'"><p>暂时无法连接 Station，请重试。</p><button class="btn" @click="host.init()">重试连接</button></template>
      <template v-else><p>请登录以查看应用。</p><button class="btn primary" @click="host.loginOpen = true">登录 Station</button></template>
    </div>
    <template v-else>
      <p v-if="store.state.error" class="panel appcenter-message" role="alert">{{ reasonLabel(store.state.error) }}<span v-if="store.state.value"> 当前保留上次读取的信息，操作已暂停。</span></p>
      <p v-if="message" class="appcenter-message" role="status">{{ message }}</p>
      <p v-if="store.commandState.error" class="panel appcenter-message" role="alert">{{ reasonLabel(store.commandState.error) }}</p>
      <AppCommandRecovery v-if="!selectedKey" :state="store.commandState" :titles="titles" @resume="store.resumeCommand" />
      <template v-if="selectedKey">
        <AppInstallDetail v-if="selected" :card="selected" :stale="store.state.stale" :operation-progress="store.operationState" @refresh-operation="store.refresh(true)" :group-label="groups.find(item => item.id === selected?.display.groupId)?.label ?? '未分类'" @back="back" @action="perform">
          <AppCommandRecovery :state="store.commandState" :titles="titles" :app-id="selected.appId" :except-operation-id="store.operationState.value?.operation_id" @resume="store.resumeCommand" />
        </AppInstallDetail>
        <div v-else class="panel appcenter-message"><p>此应用已不在当前列表中，请返回后刷新。</p><button class="btn" @click="back">返回应用中心</button></div>
      </template>
      <section v-else class="catalog-section">
        <div class="appcenter-toolbar"><label class="search-box"><input v-model="query" type="search" aria-label="搜索应用、模型或能力" placeholder="搜索应用、模型或能力" /></label></div>
        <div class="filters-bar"><button class="chip" :class="{ active: group === 'all' }" :aria-pressed="group === 'all'" @click="group = 'all'">全部</button><button v-for="item in groups" :key="item.id" class="chip" :class="{ active: group === item.id }" :aria-pressed="group === item.id" @click="group = item.id">{{ item.label }}</button><span class="result-count">{{ store.state.value ? `${result.items.length} 个应用` : '等待读取' }}</span></div>
        <p v-if="store.state.status === 'loading' || store.state.status === 'idle'" class="panel appcenter-message" role="status">正在读取应用信息…</p>
        <p v-else-if="store.state.status === 'empty'" class="panel appcenter-message">当前没有可见应用。</p>
        <p v-else-if="store.state.value && !result.items.length" class="panel appcenter-message">没有符合筛选条件的应用。</p>
        <div v-else class="catalog-groups">
          <section v-for="sec in visibleSections" :key="sec.id" class="group-section">
            <div class="section-title">
              <h2>{{ sec.title }}</h2>
              <small>{{ sec.subtitle }} · {{ sec.items.length }}</small>
              <span class="line" aria-hidden="true"></span>
            </div>
            <div class="appcenter-grid">
              <AppCard v-for="card in sec.items" :key="card.cardKey" :card="card" :stale="store.state.stale" :menu-open="menuKey === card.cardKey" @menu="menuKey = $event" @action="perform" />
            </div>
          </section>
        </div>
      </section>
    </template>
  </div></main>
  <AppDashboardWorkspace :active="businessRoute" :installation-id="installationId" :titles="titles"
    :open-dashboard="openDashboard" :authorize-dashboard="authorizeDashboard" :return-location="returnLocation" />
  <LegacyWorkspace :target="legacy.target" :route-error="legacy.error" :return-location="returnLocation" />
  <ComfyUIBookmarkWorkspace :target="legacy.comfyui" :return-location="returnLocation" />
  <AppCommandConfirm v-if="confirmation" :title="actionLabels[confirmation.kind]" :name="confirmation.card.display.name" :version="confirmation.kind === 'install' ? confirmation.card.candidateVersion ?? '版本待确认' : versionLabel(confirmation.card)" :installation-id="confirmation.card.installation?.installationId ?? null" :impact="confirmation.impact" :busy="submitting" @confirm="confirmCommand" @cancel="confirmation = null" />
</template>

<style scoped>
.group-section { margin-bottom: 28px; }
.section-title { display: flex; align-items: center; gap: 10px; margin: 22px 0 14px; }
.section-title h2 { font-size: 15px; font-weight: 650; letter-spacing: -0.2px; color: var(--ink); margin: 0; }
.section-title small { color: var(--muted-2); font-size: 11px; }
.section-title .line { height: 1px; background: var(--line); flex: 1; margin-left: 6px; }
.appcenter-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 18px; }
.appcenter-message { padding: 20px; margin: 16px 0; color: var(--muted); }
.appcenter-toolbar { display: flex; justify-content: flex-end; margin-bottom: 16px; }
.search-box input { min-width: 160px; }
@media (max-width: 640px) { .appcenter-grid { grid-template-columns: 1fr; } }
</style>
