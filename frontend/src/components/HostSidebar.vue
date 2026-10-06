<script setup lang="ts">
import { computed, onMounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import vfLogo from '../assets/verdantflare-logo.png'
import { useHostStore } from '../platform/hostStore'
import { request } from '../platform/client'

const hostStore = useHostStore()
const router = useRouter()
const route = useRoute()

onMounted(() => {
  hostStore.init()
})

const activeNav = computed<'proxies' | 'resources' | 'models' | 'market' | 'workbench'>(() => {
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
  if (route.path.startsWith('/market')) {
    return 'market'
  }
  return 'market'
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
      <span>应用市场</span>
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

    <a class="nav-link"
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

    <a class="nav-link"
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

    <div class="side-bottom">
      <!-- 1. Station Cluster Online Card (Clickable to switch to Resource Center) -->
      <button type="button"
              class="side-cluster-card"
              data-nav="resources"
              id="sideClusterCard"
              @click="navTo('resources')"
              :title="`当前连接：${hostStore.clusterName} (${hostStore.clusterHost}) · 点击查看资源看板`">
        <div class="side-cluster-row">
          <div class="side-cluster-lead">
            <i class="dot" :class="{ on: hostStore.connected }"></i>
            <span class="side-cluster-name">{{ hostStore.clusterName }}</span>
          </div>
          <span class="side-cluster-pill">{{ hostStore.connected ? '在线' : '未连接' }}</span>
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
