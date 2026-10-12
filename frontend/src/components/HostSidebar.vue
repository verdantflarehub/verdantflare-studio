<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import vfLogo from '../assets/verdantflare-logo.png'
import { useHostStore } from '../platform/hostStore'
import { request } from '../platform/client'
import { useNavigationStore } from '../platform/navigationStore'
import AppNavigationPreferences from './AppNavigationPreferences.vue'
import { dashboardLocation } from '../app-dashboard/routes'
import type { NavigationEntry } from '../navigation/controller'
import { useAppCenterStore } from '../app-center/store'
import { appCenterNavigationEntries } from '../app-center/navigation'
import { reasonLabel } from '../app-center/labels'

const hostStore = useHostStore()
const navigation = useNavigationStore()
const appcenter = import.meta.env.VITE_APPCENTER_MODE === 'unified' ? useAppCenterStore() : null
const router = useRouter()
const route = useRoute()

let refreshTimer: ReturnType<typeof setInterval> | undefined
function refreshNavigation() { if (!document.hidden && hostStore.connected) void navigation.refresh() }
function clearNavigation() { navigation.clear() }
watch(() => JSON.stringify([hostStore.identity.user_id, hostStore.identity.organization_id, hostStore.identity.station_id, hostStore.identity.session_id, hostStore.identity.policy_version, hostStore.identity.revocation_version, hostStore.identity.roles, hostStore.identity.scopes]), () => {
  const identity = hostStore.identity
  navigation.clear(identity.user_id && identity.organization_id && identity.station_id
    ? { userId: identity.user_id, organizationId: identity.organization_id, stationId: identity.station_id } : null)
  refreshNavigation()
}, { immediate: true })
onMounted(() => {
  void hostStore.init()
  refreshTimer = setInterval(refreshNavigation, 30000)
  window.addEventListener('focus', refreshNavigation)
  window.addEventListener('studio:logout', clearNavigation)
  document.addEventListener('visibilitychange', refreshNavigation)
})
onBeforeUnmount(() => {
  clearInterval(refreshTimer)
  window.removeEventListener('focus', refreshNavigation)
  window.removeEventListener('studio:logout', clearNavigation)
  document.removeEventListener('visibilitychange', refreshNavigation)
  clearNavigation()
})

const icons: Record<string, string> = {
  folder: 'M3 7h7l2 2h9v10H3zM3 7V4h8l2 2h8v3',
  globe: 'M3 12h18M12 3c-3 3-3 15 0 18M12 3c3 3 3 15 0 18M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0',
  network: 'M13 2L3 14h9l-1 8 10-12h-9l1-8z',
  resources: 'M3 3h18v7H3zM3 14h18v7H3zM6 6h1M6 17h1',
  server: 'M3 3h18v7H3zM3 14h18v7H3zM6 6h1M6 17h1',
  app: 'M3 3h7v7H3zM14 3h7v7h-7zM3 14h7v7H3zM14 14h7v7h-7z',
}
function entryLocation(entry: NavigationEntry) { return dashboardLocation(entry.installationId, entry.route) }
function entryReason(entry: NavigationEntry) {
  if (!appcenter) return entry.availability === 'offline' ? '离线' : ''
  return reasonLabel('openReason' in entry ? entry.openReason as string : null)
}
async function openEntry(entry: NavigationEntry) {
  if (appcenter) {
    const value = navigation.state.value
    if (!value) return
    const current = appCenterNavigationEntries(value, appcenter.state).find(item => item.installationId === entry.installationId
      && item.entryId === entry.entryId && item.appVersion === entry.appVersion)
    if (!current?.cardKey || !current.bindingRevision) return
    appcenter.pinOpen(entry.installationId, current.bindingRevision)
    const failed = await router.push(entryLocation(current))
    if (failed) appcenter.takeOpenBinding(entry.installationId)
    return
  }
  await router.push(entryLocation(entry))
}
function entrySelected(entry: NavigationEntry) { return route.name === 'app-dashboard' && route.params.installationId === entry.installationId }
const resourcesEntry = computed(() => navigation.entries.find(entry => entry.appId === 'resources'))
function openResources() {
  if (navigation.legacy) void router.push('/resources')
  else if (resourcesEntry.value) void openEntry(resourcesEntry.value)
  else void router.push('/market')
}

const activeNav = computed<'proxies' | 'resources' | 'models' | 'market' | 'workbench' | 'projects' | 'world' | ''>(() => {
  if (route.path.startsWith('/station/proxies')) {
    return 'proxies'
  }
  if (route.path.startsWith('/resources')) {
    return 'resources'
  }
  if (route.path.startsWith('/models')) {
    return 'models'
  }
  if (route.path.startsWith('/workbench')) {
    return 'workbench'
  }
  if (route.path.startsWith('/projects') && route.query.mode === 'world') {
    return 'world'
  }
  if (route.path.startsWith('/projects')) {
    return 'projects'
  }
  if (route.path.startsWith('/market')) {
    return 'market'
  }
  return ''
})

function navTo(target: string) {
  if (target === 'resources') {
    router.push('/resources')
  } else if (target === 'models') {
    router.push('/models')
  } else if (target === 'workbench') {
    router.push('/workbench')
  } else if (target === 'market') {
    router.push('/market')
  } else if (target === 'proxies') {
    router.push('/station/proxies')
  } else if (target === 'projects') {
    router.push('/projects')
  } else if (target === 'world') {
    router.push({ path: '/projects', query: { mode: 'world' } })
  }
}

async function handleUserClick() {
  if (!hostStore.connected) {
    const dialog = document.getElementById('loginDialog') as HTMLDialogElement | null
    if (dialog && !dialog.open) dialog.showModal()
    return
  }
  if (confirm(`当前操作员：${hostStore.displayName}\n是否退出当前登录状态？`)) {
    try {
      await request({ path: 'logout', method: 'POST' })
    } catch {}
    hostStore.setIdentity({})
    sessionStorage.removeItem('studio.operations')
    window.dispatchEvent(new CustomEvent('studio:logout'))
    const dialog = document.getElementById('loginDialog') as HTMLDialogElement | null
    if (dialog && !dialog.open) dialog.showModal()
  }
}

function handleHelpClick() {
  const toast = document.getElementById('toast')
  if (toast) {
    toast.textContent = '帮助与设计说明正在完善中'
    toast.classList.add('show')
    setTimeout(() => toast.classList.remove('show'), 3000)
  }
}
</script>

<template>
  <aside class="side" aria-label="Studio 主菜单">
    <a class="side-brand" href="#/market" @click.prevent="navTo('market')">
      <img id="vfSideLogo"
           :src="vfLogo"
           alt="VerdantFlare"
           width="26"
           height="26">
      <div class="brand-text">
        <span>青焰 · VerdantFlare</span>
        <small>STUDIO OS</small>
      </div>
    </a>

    <button class="side-create-btn"
            id="newTask"
            aria-label="新建创作任务"
            disabled
            title="创作任务开发中">
      <svg width="15"
           height="15"
           viewBox="0 0 24 24"
           fill="none"
           stroke="currentColor"
           stroke-width="2.2"
           stroke-linecap="round"
           stroke-linejoin="round">
        <line x1="12" y1="5" x2="12" y2="19"></line>
        <line x1="5" y1="12" x2="19" y2="12"></line>
      </svg>
      <span>新建创作任务</span>
    </button>

    <div class="nav-label">创作空间</div>
    <a class="nav-link"
       :class="{ selected: activeNav === 'workbench' }"
       href="#/workbench"
       data-nav="workbench"
       @click.prevent="navTo('workbench')">
      <span class="nav-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
          <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"></path>
          <polyline points="9 22 9 12 15 12 15 22"></polyline>
        </svg>
      </span>
      <span>工作台</span>
    </a>

    <a v-if="navigation.legacy" class="nav-link"
       :class="{ selected: activeNav === 'projects' }"
       href="#/projects"
       data-nav="projects"
       @click.prevent="navTo('projects')">
      <span class="nav-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
          <path d="M3 7h7l2 2h9v10H3zM3 7V4h8l2 2h8v3" />
        </svg>
      </span>
      <span>项目</span>
    </a>

    <a v-if="navigation.legacy" class="nav-link"
       :class="{ selected: activeNav === 'world' }"
       href="#/projects?mode=world"
       data-nav="world"
       @click.prevent="navTo('world')">
      <span class="nav-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="12" r="9" />
          <path d="M3 12h18M12 3c-3 3-3 15 0 18M12 3c3 3 3 15 0 18" />
        </svg>
      </span>
      <span>数字世界</span>
    </a>

    <a class="nav-link"
       :class="{ selected: activeNav === 'market' }"
       href="#/market"
       data-nav="market"
       @click.prevent="navTo('market')">
      <span class="nav-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
          <rect x="3" y="3" width="7" height="7"></rect>
          <rect x="14" y="3" width="7" height="7"></rect>
          <rect x="14" y="14" width="7" height="7"></rect>
          <rect x="3" y="14" width="7" height="7"></rect>
        </svg>
      </span>
      <span>应用中心</span>
    </a>

    <div class="nav-sub-group" id="groupNav" v-show="activeNav === 'market'">
      <!-- Injected by JavaScript -->
    </div>

    <a class="nav-link"
       :class="{ selected: activeNav === 'models' }"
       href="#/models"
       data-nav="models"
       @click.prevent="navTo('models')">
      <span class="nav-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
          <circle cx="12" cy="12" r="8"></circle>
          <line x1="12" y1="2" x2="12" y2="6"></line>
          <line x1="12" y1="18" x2="12" y2="22"></line>
          <line x1="4.93" y1="4.93" x2="7.76" y2="7.76"></line>
          <line x1="16.24" y1="16.24" x2="19.07" y2="19.07"></line>
          <line x1="2" y1="12" x2="6" y2="12"></line>
          <line x1="18" y1="12" x2="22" y2="12"></line>
        </svg>
      </span>
      <span>模型市场</span>
    </a>

    <a v-if="navigation.legacy" class="nav-link"
       :class="{ selected: activeNav === 'resources' }"
       href="#/resources"
       data-nav="resources"
       @click.prevent="navTo('resources')">
      <span class="nav-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
          <rect x="2" y="2" width="20" height="8" rx="2" ry="2"></rect>
          <rect x="2" y="14" width="20" height="8" rx="2" ry="2"></rect>
          <line x1="6" y1="6" x2="6.01" y2="6"></line>
          <line x1="6" y1="18" x2="6.01" y2="18"></line>
        </svg>
      </span>
      <span>资源管理</span>
    </a>

    <a v-if="navigation.legacy" class="nav-link"
       :class="{ selected: activeNav === 'proxies' }"
       href="#/station/proxies"
       data-nav="proxies"
       @click.prevent="navTo('proxies')">
      <span class="nav-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
          <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z"></path>
        </svg>
      </span>
      <span>网络出口</span>
    </a>

    <template v-if="navigation.state.value?.mode === 'preinstalled'">
      <div v-for="entry in navigation.pinned" :key="`${entry.installationId}/${entry.entryId}`" class="app-navigation-row">
        <a class="nav-link" :class="{ selected: entrySelected(entry) }" :href="router.resolve(entryLocation(entry)).href"
           :aria-current="entrySelected(entry) ? 'page' : undefined" :aria-disabled="appcenter && entry.availability !== 'ready' ? 'true' : undefined"
           :title="entry.availability !== 'ready' ? entryReason(entry) : undefined" @click.prevent="openEntry(entry)">
          <span class="nav-icon"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><path :d="icons[entry.iconRef] || icons.app" /></svg></span>
          <span>{{ entry.title }}</span><small v-if="entry.availability !== 'ready'">{{ entry.availability === 'offline' ? '离线' : '待确认' }}</small>
        </a>
        <button type="button" class="unpin-entry" :disabled="navigation.state.saving" :aria-label="`取消固定${entry.title}`" :title="`取消固定${entry.title}`" @click="navigation.setPreference(entry, false)">×</button>
      </div>
      <details class="menu-preferences"><summary>管理菜单</summary><AppNavigationPreferences /></details>
    </template>
    <p v-if="navigation.state.status === 'loading'" role="status" class="navigation-status">正在加载应用菜单…</p>
    <div v-if="navigation.state.error" role="status" class="navigation-status">
      <span>{{ navigation.state.error }}</span><button type="button" @click="navigation.refresh()">重试</button>
    </div>

    <div class="side-bottom">
      <!-- Host health does not require the Resources application to be mounted. -->
      <button type="button"
              class="side-cluster-card"
              id="sideClusterCard"
              @click="openResources"
              :title="`${hostStore.clusterName} · ${navigation.legacy || resourcesEntry ? '查看资源管理' : '打开应用中心'}`">
        <div class="side-cluster-row">
          <div class="side-cluster-lead">
            <i class="dot" :class="{ on: hostStore.connected }"></i>
            <span class="side-cluster-name">{{ hostStore.clusterName }}</span>
          </div>
          <span class="side-cluster-pill">{{ hostStore.connected ? '已登录' : '未连接' }}</span>
        </div>
        <div class="side-cluster-meta">{{ hostStore.clusterMeta }}</div>
      </button>

      <!-- 2. User Login Card -->
      <div class="side-user-card"
           id="sideUserCard"
           @click="handleUserClick"
           style="cursor: pointer;"
           :title="`当前操作员：${hostStore.displayName} · ${hostStore.teamName} (点击退出)`">
        <div class="avatar">{{ hostStore.avatar }}</div>
        <div class="side-user-info">
          <span class="side-user-name">{{ hostStore.displayName }}</span>
          <span class="side-user-team">{{ hostStore.teamName }}</span>
        </div>
      </div>

      <a class="nav-link side-help-link"
         href="#/market"
         data-placeholder="true"
         @click.prevent="handleHelpClick"
         title="帮助与设计说明">
        <span class="nav-icon">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="9"></circle>
            <path d="M9.8 9a2.4 2.4 0 1 1 3.2 2.3c-.7.3-1 1-1 1.7v.3M12 17h.01"></path>
          </svg>
        </span>
        <span>帮助与设计说明</span>
      </a>
    </div>
  </aside>
</template>

<style scoped>
.app-navigation-row { display: flex; align-items: center; }
.app-navigation-row .nav-link { flex: 1; min-width: 0; }
.app-navigation-row .nav-link > span:not(.nav-icon) { overflow-wrap: anywhere; }
.app-navigation-row small { color: var(--muted); margin-left: auto; }
.unpin-entry { color: var(--muted); padding: 8px; border-radius: 6px; }
.unpin-entry:hover { color: var(--accent); background: var(--shell-button); }
.menu-preferences { margin: 4px 12px 12px; font-size: 12px; color: var(--muted); }
.menu-preferences summary { cursor: pointer; padding: 6px 0; }
.navigation-status { padding: 8px 12px; font-size: 12px; color: var(--muted); }
.navigation-status button { display: block; padding: 4px 0; color: var(--accent); }
</style>
