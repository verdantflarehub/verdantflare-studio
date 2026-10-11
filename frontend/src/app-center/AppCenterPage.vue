<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useHostStore } from '../platform/hostStore'
import { useAppCenterStore } from './store'
import { selectCards } from './controller'
import { actionLabels, factLabel, reasonLabel } from './labels'
import type { AppCenterCard } from './schema.generated'
import AppCard from './AppCard.vue'
import AppDashboardWorkspace from '../app-dashboard/AppDashboardWorkspace.vue'
import { createDashboardWebClient } from '../app-dashboard/web-client'
import { dashboardLocation } from '../app-dashboard/routes'
import AppInstallDetail from './AppInstallDetail.vue'
import LegacyWorkspace from './LegacyWorkspace.vue'
import { createLegacyClient, legacyLocation, legacyTarget } from './legacy'

const host = useHostStore(), store = useAppCenterStore(), route = useRoute(), router = useRouter()
store.setVisible('directory', true)
const query = ref(''), group = ref('all'), filter = ref('all'), message = ref('')
const theme = ref(document.body.dataset.theme === 'dark' ? 'dark' : 'light')
try { theme.value = localStorage.getItem('vf_studio_theme') === 'dark' ? 'dark' : 'light' } catch { /* use current theme */ }
watch(theme, value => {
  document.body.dataset.theme = value; document.documentElement.dataset.theme = value
  try { localStorage.setItem('vf_studio_theme', value) } catch { /* theme still works without storage */ }
}, { immediate: true })
const selectedKey = computed(() => typeof route.query.card === 'string' ? route.query.card : '')
const selected = computed(() => store.state.value?.items.find(card => card.cardKey === selectedKey.value))
const businessRoute = computed(() => route.name === 'app-dashboard' || route.name === 'app-instance')
const legacy = computed(() => {
  try { return { target: legacyTarget(route.fullPath), error: '' } }
  catch (error) { return { target: null, error: error instanceof Error ? error.message : 'INVALID_LEGACY_ROUTE' } }
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
  const expected = store.takeOpenBinding(id)
  if (expected && expected !== card.bindingRevision) throw new Error('APPCENTER_BINDING_CHANGED')
}
async function openDashboard(id: string, signal?: AbortSignal) {
  const view = store.state.value, card = view?.items.find(item => item.installation?.installationId === id)
  const action = card?.actions.find(item => item.kind === 'open')
  if (!view || !card || !action?.entryId || !store.canExecute(card.cardKey, action.id, card.bindingRevision)) throw new Error('APPCENTER_OPEN_UNAVAILABLE')
  const opened = await dashboardClient.openAppCenter({ schemaVersion: 1, cardKey: card.cardKey, entryId: action.entryId,
    expectedViewRevision: view.revision, expectedBindingRevision: card.bindingRevision }, id, signal)
  const current = store.state.value
  if (signal?.aborted || !current || JSON.stringify(current.scope) !== JSON.stringify(view.scope)
    || current.items.find(item => item.cardKey === card.cardKey)?.bindingRevision !== card.bindingRevision
    || opened.context.contextRevision !== view.scope.authorizationEpoch || opened.context.userId !== view.scope.userId
    || opened.context.organizationId !== view.scope.organizationId || opened.context.stationId !== view.scope.stationId
    || opened.launch.appVersion !== card.installedVersion || opened.launch.appId !== card.appId) {
    await dashboardClient.close(opened.launch.dashboardSessionId, AbortSignal.timeout(5000)).catch(() => undefined)
    throw new Error('APPCENTER_BINDING_CHANGED')
  }
  return opened
}
const result = computed(() => selectCards(store.state.value, query.value, group.value, filter.value, !store.state.stale))
const groups = computed(() => {
  const names = new Map((store.state.value?.groups ?? []).map(item => [item.id, item.label]))
  for (const card of store.state.value?.items ?? []) if (!names.has(card.display.groupId)) names.set(card.display.groupId, '未分类')
  return Array.from(names, ([id, label]) => ({ id, label }))
})
const tabs = [{ id: 'all', label: '全部应用' }, { id: 'installed', label: '已安装' }, { id: 'active', label: '进行中' }] as const
let listScroll = 0
function back() { void router.replace({ path: '/market', query: {} }) }
async function perform(card: AppCenterCard, actionId: string) {
  const action = card.actions.find(item => item.id === actionId)
  if (actionId === 'detail' || action?.kind === 'progress' && action.enabled) {
    await router.push({ query: { card: card.cardKey } }); return
  }
  if (action?.kind === 'refresh') { await store.refresh(); return }
  if (!action || !store.canExecute(card.cardKey, actionId, card.bindingRevision)) return
  if (['install', 'start', 'stop', 'restart', 'delete'].includes(action.kind)) {
    const impacts: Record<string, string> = {
      install: '将安装此版本的应用管理服务。', start: '将启动应用管理服务。',
      stop: '将关闭应用管理服务，相关业务可能暂时无法访问。', restart: '将重启应用管理服务，相关业务可能暂时中断。',
      delete: '将删除此应用安装。请先确认业务数据和工作已妥善保存。',
    }
    if (!window.confirm(`${actionLabels[action.kind]}「${card.display.name}」？\n${impacts[action.kind]}`)) return
    try { await store.execute(card, actionId); message.value = '' }
    catch (error) { message.value = reasonLabel(error instanceof Error ? error.message : 'APPCENTER_SUBMISSION_UNKNOWN') }
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
    try {
      if (new URLSearchParams(location.search).get('host') === 'desktop') throw new Error('HOST_UNSUPPORTED')
      const opened = await legacyClient.open(view, card, AbortSignal.timeout(30000))
      if (!store.state.value || JSON.stringify(store.state.value.scope) !== JSON.stringify(view.scope) || !store.canExecute(card.cardKey, action.id, card.bindingRevision)) throw new Error('APPCENTER_BINDING_CHANGED')
      store.pinOpen(card.cardKey, card.bindingRevision)
      const failed = await router.push(legacyLocation(opened, card.cardKey))
      if (failed) store.takeOpenBinding(card.cardKey)
    } catch (error) { message.value = reasonLabel(error instanceof Error ? error.message : 'APPCENTER_OPEN_UNAVAILABLE') }
    return
  }
  // The current directory delivery does not promise an unconnected mutation or
  // entry adapter. This dispatcher will gain only the finite contracted kinds.
  message.value = '此操作暂不可用，请稍后重试。'
}
const directoryVisible = computed(() => !selectedKey.value && !businessRoute.value && !legacy.value.target && !legacy.value.error)
watch(directoryVisible, async (visible, previous) => {
  if (!visible && previous) listScroll = window.scrollY
  await nextTick()
  window.scrollTo({ top: visible ? listScroll : 0 })
})
watch(() => JSON.stringify([host.identity.user_id, host.identity.organization_id, host.identity.station_id]), () => {
  query.value = ''; group.value = 'all'; filter.value = 'all'; message.value = ''; listScroll = 0
})
onBeforeUnmount(() => store.setVisible('directory', false))
</script>

<template>
  <main v-show="!businessRoute && !legacy.target && !legacy.error" class="main appcenter-page"><div class="workspace">
    <header class="heading"><div class="heading-left"><h1>应用中心</h1><p>管理 Station 应用、安装与运行状态。</p></div><div class="heading-actions"><button class="btn ghost" aria-label="切换浅色或深色主题" @click="theme = theme === 'dark' ? 'light' : 'dark'">{{ theme === 'dark' ? '浅色主题' : '深色主题' }}</button><button class="btn" :disabled="store.state.refreshing" @click="store.refresh()">{{ store.state.refreshing ? '刷新中…' : '刷新' }}</button></div></header>
    <p v-if="!host.connected" class="panel appcenter-message">请登录以查看应用。</p>
    <template v-else>
      <p v-if="store.state.error" class="panel appcenter-message" role="alert">{{ reasonLabel(store.state.error) }}<span v-if="store.state.value"> 当前保留上次读取的信息，操作已暂停。</span></p>
      <p v-if="message" class="appcenter-message" role="status">{{ message }}</p>
      <p v-if="store.commandState.error" class="panel appcenter-message" role="alert">{{ reasonLabel(store.commandState.error) }}</p>
      <section v-if="store.commandState.items.length" class="panel appcenter-recovery" aria-label="操作与恢复">
        <h2>操作与恢复</h2>
        <div v-for="item in store.commandState.items" :key="item.record.key" class="recovery-item">
          <p>{{ item.operation?.app_id ?? JSON.parse(item.record.body).prepared.command.app_id }} · {{ item.operation ? factLabel(item.operation.status) : '提交结果待确认' }}<span v-if="item.operation"> · {{ factLabel(item.operation.phase) }}</span></p>
          <p v-if="item.operation?.download.percent != null">下载进度 {{ item.operation.download.percent }}%</p>
          <p v-if="item.operation?.status === 'failed'">操作未完成，请检查应用状态后重新确认。</p>
          <p v-if="item.error" role="alert">{{ reasonLabel(item.error) }}</p>
          <button v-if="!item.operation || !['succeeded', 'failed'].includes(item.operation.status) || item.error" class="btn" :disabled="item.busy" @click="store.resumeCommand(item.record.key)">{{ item.busy ? '正在确认…' : item.record.operationId ? '刷新进度' : '恢复原请求' }}</button>
        </div>
      </section>
      <template v-if="selectedKey">
        <AppInstallDetail v-if="selected" :card="selected" :stale="store.state.stale" :group-label="groups.find(item => item.id === selected?.display.groupId)?.label ?? '未分类'" @back="back" @action="perform" />
        <div v-else class="panel appcenter-message"><p>此应用已不在当前列表中，请返回后刷新。</p><button class="btn" @click="back">返回应用中心</button></div>
      </template>
      <section v-else class="catalog-section">
        <div class="tabs-row"><div class="tabs"><button v-for="tab in tabs" :key="tab.id" class="tab" :class="{ active: filter === tab.id }" :aria-pressed="filter === tab.id" @click="filter = tab.id">{{ tab.label }} <span>{{ result.counts[tab.id] ?? '—' }}</span></button></div><label class="search-box"><input v-model="query" type="search" aria-label="搜索应用、模型或能力" placeholder="搜索应用、模型或能力" /></label></div>
        <div class="filters-bar"><button class="chip" :class="{ active: group === 'all' }" :aria-pressed="group === 'all'" @click="group = 'all'">全部</button><button v-for="item in groups" :key="item.id" class="chip" :class="{ active: group === item.id }" :aria-pressed="group === item.id" @click="group = item.id">{{ item.label }}</button><span class="result-count">{{ store.state.value ? `${result.items.length} 个应用` : '等待读取' }}</span></div>
        <p v-if="store.state.status === 'loading' || store.state.status === 'idle'" class="panel appcenter-message" role="status">正在读取应用信息…</p>
        <p v-else-if="store.state.status === 'empty'" class="panel appcenter-message">当前没有可见应用。</p>
        <p v-else-if="store.state.value && !result.items.length" class="panel appcenter-message">没有符合筛选条件的应用。</p>
        <section v-if="result.preinstalled.length" class="appcenter-preinstalled" aria-label="系统预装应用">
          <h2>系统预装应用</h2>
          <div class="appcenter-grid"><AppCard v-for="card in result.preinstalled" :key="card.cardKey" :card="card" :stale="store.state.stale" @action="perform" /></div>
        </section>
        <div class="appcenter-grid"><AppCard v-for="card in result.applications" :key="card.cardKey" :card="card" :stale="store.state.stale" @action="perform" /></div>
      </section>
    </template>
  </div></main>
  <AppDashboardWorkspace :active="businessRoute" :installation-id="installationId" :titles="titles"
    :open-dashboard="openDashboard" :authorize-dashboard="authorizeDashboard" />
  <LegacyWorkspace :target="legacy.target" :route-error="legacy.error" />
</template>

<style scoped>
.appcenter-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 18px; }
.appcenter-message { padding: 20px; margin: 16px 0; color: var(--muted); }
.appcenter-preinstalled { margin-bottom: 24px; }
.appcenter-preinstalled h2 { font-size: 18px; margin: 0 0 16px; }
.appcenter-recovery { padding: 20px; margin: 16px 0; background: var(--panel); border: 1px solid var(--line); border-radius: 12px; }
.appcenter-recovery h2 { font-size: 18px; margin: 0 0 12px; }
.recovery-item + .recovery-item { border-top: 1px solid var(--line); margin-top: 12px; padding-top: 12px; }
.search-box input { min-width: 160px; }
@media (max-width: 640px) { .appcenter-grid { grid-template-columns: 1fr; } }
</style>
