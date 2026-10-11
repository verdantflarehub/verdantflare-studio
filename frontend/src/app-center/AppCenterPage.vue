<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useHostStore } from '../platform/hostStore'
import { useAppCenterStore } from './store'
import { selectCards } from './controller'
import { actionLabels, statusLabels, reasonLabel, factLabel } from './labels'
import type { AppCenterCard } from './schema.generated'
import AppCard from './AppCard.vue'

const host = useHostStore(), store = useAppCenterStore(), route = useRoute(), router = useRouter()
const query = ref(''), group = ref('all'), filter = ref('all'), message = ref('')
const selectedKey = computed(() => typeof route.query.card === 'string' ? route.query.card : '')
const selected = computed(() => store.state.value?.items.find(card => card.cardKey === selectedKey.value))
const businessRoute = computed(() => route.name === 'app-dashboard' || route.name === 'app-instance')
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
    listScroll = window.scrollY
    await router.push({ query: { card: card.cardKey } }); await nextTick(); window.scrollTo({ top: 0 }); return
  }
  if (action?.kind === 'refresh') { await store.refresh(); return }
  if (!action || !store.canExecute(card.cardKey, actionId, card.bindingRevision)) return
  // The current directory delivery does not promise an unconnected mutation or
  // entry adapter. This dispatcher will gain only the finite contracted kinds.
  message.value = '此操作暂不可用，请稍后重试。'
}
watch(selectedKey, async (next, previous) => { if (!next && previous) { await nextTick(); window.scrollTo({ top: listScroll }) } })
watch(() => JSON.stringify([host.identity.user_id, host.identity.organization_id, host.identity.station_id]), () => {
  query.value = ''; group.value = 'all'; filter.value = 'all'; message.value = ''; listScroll = 0
})
onMounted(() => store.setVisible('directory', true))
onBeforeUnmount(() => store.setVisible('directory', false))
</script>

<template>
  <main class="main appcenter-page"><div class="workspace">
    <header class="heading"><div class="heading-left"><h1>应用中心</h1><p>管理 Station 应用、安装与运行状态。</p></div><div class="heading-actions"><button class="btn" :disabled="store.state.refreshing" @click="store.refresh()">{{ store.state.refreshing ? '刷新中…' : '刷新' }}</button></div></header>
    <p v-if="!host.connected" class="panel appcenter-message">请登录以查看应用。</p>
    <template v-else>
      <p v-if="store.state.error" class="panel appcenter-message" role="alert">{{ reasonLabel(store.state.error) }}<span v-if="store.state.value"> 当前保留上次读取的信息，操作已暂停。</span></p>
      <p v-if="message" class="appcenter-message" role="status">{{ message }}</p>
      <section v-if="businessRoute" class="panel appcenter-message"><h2>应用入口暂不可用</h2><p>请返回应用中心查看当前状态。</p><button class="btn" @click="back">返回应用中心</button></section>
      <section v-else-if="selectedKey" class="appcenter-detail">
        <button class="btn ghost" @click="back">返回应用中心</button>
        <template v-if="selected">
          <h2>{{ selected.display.name }}</h2><p>{{ selected.display.description || '暂无应用说明' }}</p>
          <dl class="panel appcenter-facts"><dt>状态</dt><dd>{{ statusLabels[selected.statusCode] ?? '状态待确认' }}</dd><dt>安装版本</dt><dd>{{ selected.installedVersion ?? '无已确认安装' }}</dd><dt>候选版本</dt><dd>{{ selected.candidateVersion ?? '暂无' }}</dd><dt>安装状态</dt><dd>{{ selected.installation ? factLabel(selected.installation.state) : '无安装记录' }}</dd><dt>服务状态</dt><dd>{{ factLabel(selected.service.state) }}</dd><dt>入口</dt><dd>{{ selected.entry ? factLabel(selected.entry.availability) : '尚无可用入口' }}</dd></dl>
          <div class="action-row"><template v-for="action in selected.actions.filter(item => item.kind !== 'detail')" :key="action.id"><div><button class="btn" :disabled="!action.enabled || store.state.stale && action.kind !== 'refresh'" @click="perform(selected, action.id)">{{ actionLabels[action.kind] }}</button><p v-if="!action.enabled" class="muted">{{ reasonLabel(action.reasonCode) }}</p></div></template></div>
          <section v-if="selected.operation.canRead" class="panel appcenter-message"><h3>最近操作</h3><p>{{ factLabel(selected.operation.status) }} · {{ factLabel(selected.operation.phase) }}</p></section>
          <p v-else-if="selected.operation.blocking">应用正在变更，请稍后刷新。</p>
        </template><p v-else class="panel appcenter-message">此应用已不在当前列表中，请返回后刷新。</p>
      </section>
      <section v-else class="catalog-section">
        <div class="tabs-row"><div class="tabs"><button v-for="tab in tabs" :key="tab.id" class="tab" :class="{ active: filter === tab.id }" :aria-pressed="filter === tab.id" @click="filter = tab.id">{{ tab.label }} <span>{{ result.counts[tab.id] ?? '—' }}</span></button></div><label class="search-box"><input v-model="query" type="search" aria-label="搜索应用" placeholder="搜索应用" /></label></div>
        <div class="filters-bar"><button class="chip" :class="{ active: group === 'all' }" :aria-pressed="group === 'all'" @click="group = 'all'">全部</button><button v-for="item in groups" :key="item.id" class="chip" :class="{ active: group === item.id }" :aria-pressed="group === item.id" @click="group = item.id">{{ item.label }}</button><span class="result-count">{{ store.state.value ? `${result.items.length} 个应用` : '等待读取' }}</span></div>
        <p v-if="store.state.status === 'loading' || store.state.status === 'idle'" class="panel appcenter-message" role="status">正在读取应用信息…</p>
        <p v-else-if="store.state.status === 'empty'" class="panel appcenter-message">当前没有可见应用。</p>
        <p v-else-if="store.state.value && !result.items.length" class="panel appcenter-message">没有符合筛选条件的应用。</p>
        <div class="appcenter-grid"><AppCard v-for="card in result.items" :key="card.cardKey" :card="card" :stale="store.state.stale" @action="perform" /></div>
      </section>
    </template>
  </div></main>
</template>

<style scoped>
.appcenter-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(280px, 1fr)); gap: 18px; }
.appcenter-message { padding: 20px; margin: 16px 0; color: var(--muted); }
.appcenter-detail { display: grid; gap: 20px; }
.appcenter-facts { display: grid; grid-template-columns: minmax(90px, 160px) 1fr; gap: 12px; padding: 24px; }
.appcenter-facts dt { color: var(--muted); }
.appcenter-detail .action-row { flex-wrap: wrap; align-items: start; }
.appcenter-detail .action-row p { font-size: 12px; margin-top: 6px; }
.search-box input { min-width: 160px; }
@media (max-width: 640px) { .appcenter-grid { grid-template-columns: 1fr; } }
</style>
