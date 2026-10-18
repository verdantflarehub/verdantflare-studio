<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref } from "vue"
import { useRoute, useRouter } from 'vue-router'
import AppDashboardWorkspace from '../app-dashboard/AppDashboardWorkspace.vue'
import { createDashboardWebClient } from '../app-dashboard/web-client'
import { dashboardLocation } from '../app-dashboard/routes'
import AppNavigationPreferences from '../components/AppNavigationPreferences.vue'
import PreinstalledApps from '../components/PreinstalledApps.vue'
import { useNavigationStore } from '../platform/navigationStore'
import { mountMarket } from "./market"
import { CATALOG_GROUPS } from "./catalog"
import "../theme/blender-management.css"
import '../theme/comfyui-management.css'

let dispose: (() => void) | undefined
const route = useRoute(), router = useRouter()
const dashboardActive = computed(() => route.name === 'app-dashboard' || route.name === 'app-dashboard-legacy' || route.name === 'app-instance')
const installationId = computed(() => typeof route.params.appId === 'string' ? route.params.appId : typeof route.params.installationId === 'string' ? route.params.installationId : '')
const titles = ref<Record<string, string>>({})
const detailAppId = ref('')
const navigation = useNavigationStore()
const client = ['http:', 'https:'].includes(location.protocol)
  ? createDashboardWebClient(location.origin) : undefined
onMounted(() => { dispose = mountMarket({
  loadDashboards: () => client?.installations(AbortSignal.timeout(10000)) ?? Promise.resolve([]),
  openDashboard: (id: string) => router.push(dashboardLocation(id)),
  onCatalog: (items: { app_id: string; display_name: string }[]) => { titles.value = Object.fromEntries(items.map(item => [item.app_id, item.display_name])) },
  onDetail: (appId: string | null) => { detailAppId.value = appId ?? '' },
  onInstallationsChanged: () => navigation.refresh(),
}) })
onBeforeUnmount(() => dispose?.())
</script>

<template>
  <!-- Main Content Area: Pure Workspace -->
  <main v-show="!dashboardActive" class="main">
    <div class="workspace">
      <!-- Clean, Focused Heading Zone for Sub-business Workspace -->
      <header class="heading">
        <div class="heading-left">
          <h1 id="pageTitle">应用中心</h1>
          <p id="pageSubtitle">为 Station 安装创作应用，管理模型与运行状态。</p>
        </div>
        <div class="heading-actions">
          <button class="icon-btn"
                  id="reset"
                  title="刷新应用与集群状态"
                  aria-label="刷新状态">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <path d="M21.5 2v6h-6M21.34 15.57a10 10 0 1 1-.57-8.38l5.67-5.67"></path>
            </svg>
          </button>
        </div>
      </header>

      <!-- Studio OS 视口级优雅加载态 (VF 青焰美学 Viewport Loader) -->
      <div id="viewportLoader" class="viewport-loader" hidden>
        <div class="loader-content">
          <div class="loader-logo-wrap">
            <svg class="loader-flame" viewBox="0 0 64 64" fill="none" xmlns="http://www.w3.org/2000/svg">
              <path d="M32 4L54 26L32 60L10 26L32 4Z" fill="url(#vfFlameGrad)" />
              <path d="M32 14L44 28L32 48L20 28L32 14Z" fill="var(--bg)" opacity="0.88" />
              <defs>
                <linearGradient id="vfFlameGrad" x1="10" y1="4" x2="54" y2="60" gradientUnits="userSpaceOnUse">
                  <stop stop-color="#087e60" />
                  <stop offset="0.5" stop-color="#75c7ab" />
                  <stop offset="1" stop-color="#5379ac" />
                </linearGradient>
              </defs>
            </svg>
          </div>
          <div class="loader-text" id="loaderText">正在连接应用工作台…</div>
          <div class="loader-bar-track">
            <div class="loader-bar-thumb"></div>
          </div>
        </div>
      </div>

      <!-- Video MCP Embedded Viewport -->
      <section id="videoHost" hidden>
        <div class="video-host-bar">
          <nav class="video-breadcrumb" aria-label="面包屑">
            <ol id="videoBreadcrumb"></ol>
          </nav>
          <span id="videoMessage" role="status"></span>
          <button class="btn" id="videoRetry">重新连接</button>
        </div>
        <iframe id="videoFrame" title="Video MCP 工作区" sandbox="allow-scripts allow-downloads" referrerpolicy="no-referrer"></iframe>
      </section>

      <!-- Image MCP Embedded Viewport -->
      <section id="imageHost" hidden>
        <div class="video-host-bar">
          <nav class="video-breadcrumb" aria-label="面包屑">
            <ol id="imageBreadcrumb"></ol>
          </nav>
          <span id="imageMessage" role="status"></span>
          <button class="btn" id="imageRetry">重新连接</button>
        </div>
        <iframe id="imageFrame" title="Image MCP 工作区" sandbox="allow-scripts allow-downloads" referrerpolicy="no-referrer"></iframe>
      </section>

      <!-- Primary Market Columns (Full Width Workspace) -->
      <section id="blenderHost" class="blender-v02-host" hidden>
        <div class="breadcrumb-bar blender-header">
          <nav id="blenderBreadcrumb" class="breadcrumb blender-breadcrumb" aria-label="面包屑"></nav>
          <div class="actions">
            <a id="blenderManageLink" class="btn sm" hidden>实例详情</a>
            <div id="blenderToolbar" class="blender-toolbar actions" hidden>
              <button class="btn sm" id="blenderRelease" type="button">释放控制权</button>
              <button class="btn sm primary" id="blenderSave" type="button">保存工程</button>
              <button class="btn sm" id="blenderCopy" type="button">复制 MCP 地址</button>
            </div>
          </div>
        </div>
        <header id="blenderAppHeader" class="topline studio-app-heading">
          <div class="brand">
            <div class="mark"><img src="/assets/blender-logo.svg" alt=""></div>
            <div>
              <div class="title-line">
                <h1>Blender</h1>
                <span class="badge running"><span class="dot"></span>5090 独占工作站</span>
              </div>
              <p class="subtitle">云端 3D 创作与渲染工作站 · 独立工作区与 5090 算力管理</p>
            </div>
          </div>
          <div class="actions">
            <button class="btn" id="blenderRefresh" type="button">
              <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M20 7v5h-5M4 17v-5h5M6 6a8 8 0 0 1 14 6M4 12a8 8 0 0 0 14 6"/></svg>刷新
            </button>
            <button id="blenderCreate" class="btn primary" type="button" disabled title="实例创建暂未开放">
              <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 5v14M5 12h14"/></svg>创建实例
            </button>
          </div>
        </header>
        <section id="blenderList" class="blender-management" aria-label="Blender 实例" hidden>
          <dl id="blenderSummary" class="summary panel" hidden></dl>
          <div class="toolbar filterbar">
            <div id="blenderFilters" class="filters filter-pills" role="group" aria-label="运行状态筛选">
              <button type="button" class="filter filter-pill" data-status="all" aria-pressed="true"><span>全部</span><span class="filter-count num" data-filter-count="all">0</span></button>
              <button type="button" class="filter filter-pill" data-status="running" aria-pressed="false"><span>运行中</span><span class="filter-count num" data-filter-count="running">0</span></button>
              <button type="button" class="filter filter-pill" data-status="stopped" aria-pressed="false"><span>已停止</span><span class="filter-count num" data-filter-count="stopped">0</span></button>
              <button type="button" class="filter filter-pill" data-status="failed" aria-pressed="false"><span>异常</span><span class="filter-count num" data-filter-count="failed">0</span></button>
            </div>
            <div class="filterbar-right">
              <label class="search">
                <span class="sr-only">搜索实例或项目</span>
                <svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="10" cy="10" r="6"/><path d="m15 15 5 5"/></svg>
                <input id="blenderSearch" type="search" placeholder="搜索实例名称、ID 或关联项目" aria-label="搜索实例或项目">
              </label>
            </div>
          </div>
          <div id="blenderSkeleton" class="blender-skeleton" aria-busy="true" hidden>
            <div class="panel table-wrap sk-table">
              <div class="skeleton sk-row" style="--i:1"></div>
              <div class="skeleton sk-row" style="--i:2"></div>
              <div class="skeleton sk-row" style="--i:3"></div>
              <div class="skeleton sk-row" style="--i:4"></div>
            </div>
          </div>
          <div id="blenderTableWrap" class="panel table-wrap" hidden>
            <table class="instance-table" aria-label="Blender 实例列表">
              <thead>
                <tr>
                  <th>实例名称</th>
                  <th>状态</th>
                  <th>CPU</th>
                  <th>内存</th>
                  <th>GPU</th>
                  <th class="col-actions">操作</th>
                </tr>
              </thead>
              <tbody id="blenderRows"></tbody>
            </table>
            <div id="blenderPagination" class="table-pagination num" aria-label="列表分页">
              <span id="blenderPageSummary">共 0 个实例 · 第 1 / 1 页（每页 10 条）</span>
              <div class="page-actions">
                <button type="button" class="btn sm" id="blenderPrevPage" disabled>上一页</button>
                <button type="button" class="btn sm" id="blenderNextPage" disabled>下一页</button>
              </div>
            </div>
          </div>
          <section id="blenderEmpty" class="panel empty" hidden>
            <div class="app-icon"><img src="/assets/blender-logo.svg" alt=""></div>
            <h2 id="blenderEmptyTitle">暂无 Blender 实例</h2>
            <p id="blenderListMessage" class="muted" role="status">正在读取实例…</p>
            <div class="actions">
              <button id="blenderClearFilter" class="btn" type="button" hidden>清除筛选</button>
              <button id="blenderListRetry" class="btn" type="button" hidden>重试</button>
            </div>
          </section>
        </section>
        <section id="blenderDetail" class="blender-management" aria-label="Blender 实例详情" hidden></section>
        <dialog id="blenderCreateDialog" class="blender-management blender-create-dialog" aria-labelledby="blenderCreateTitle">
          <section id="blenderCreateView" class="blender-management" aria-label="创建 Blender 实例"></section>
        </dialog>
        <section id="blenderWorkspace" hidden>
          <div id="blenderViewport" class="blender-viewport workspace-viewport">
            <div id="blenderLoading" class="blender-loading workspace-loading">
              <div class="workspace-identity">
                <img class="blender-mark workspace-mark" src="/assets/blender-logo.svg" alt="">
                <h2 id="blenderWorkspaceTitle">Blender</h2>
              </div>
              <div id="blenderProgress" class="blender-progress workspace-bar" aria-hidden="true"></div>
              <p id="blenderMessage" class="workspace-message" role="status" aria-live="polite">正在打开</p>
              <div class="workspace-actions">
                <button id="blenderAction" class="btn" type="button">取消连接</button>
              </div>
            </div>
            <iframe id="blenderFrame" title="Blender 三维工程编辑器" sandbox="allow-scripts allow-same-origin allow-pointer-lock" allow="autoplay; fullscreen" referrerpolicy="no-referrer" hidden></iframe>
          </div>
        </section>
      </section>

      <section id="comfyuiHost" class="comfyui-management" aria-label="ComfyUI 实例" hidden>
        <div class="topline"><nav class="crumbs" aria-label="面包屑"><a href="#/market?comfyui=instances">ComfyUI</a><span id="comfyuiInstanceCrumb" hidden>/ <span id="comfyuiInstanceName" aria-current="page"></span></span></nav><div class="actions"><span id="comfyuiEditorMessage" class="comfyui-editor-status" role="status" hidden></span><button id="comfyuiRefresh" class="btn">刷新</button></div></div>
        <div id="comfyuiDirectory"><div class="intro"><h1>你的 ComfyUI 实例</h1></div>
        <div class="filterbar"><span id="comfyuiCount">我的实例 · —</span><input id="comfyuiSearch" type="search" aria-label="搜索 ComfyUI 实例" placeholder="搜索实例"></div>
        <p id="comfyuiMessage" role="status">正在获取实例…</p>
        <div id="comfyuiRows"></div></div>
        <section id="comfyuiWorkspace" hidden><div id="comfyuiEditorFallback" class="comfyui-editor-fallback"><p id="comfyuiEditorHint">正在打开 ComfyUI…</p></div><div id="comfyuiFrameHost"></div></section>
      </section>

      <div id="marketView" class="market-columns">
        <div class="market-primary">
          <PreinstalledApps :active="!dashboardActive" :titles="titles" />
          <section class="catalog-section">
            <div class="tabs-row">
              <div class="tabs">
                <button class="tab active" data-tab="all">全部应用 <span id="allCount">—</span></button>
                <button class="tab" data-tab="installed">已安装 <span id="installedCount">—</span></button>
                <button class="tab" data-tab="active">进行中 <span id="activeCount">—</span></button>
              </div>
              <div class="search-box">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
                  <circle cx="11" cy="11" r="8"></circle>
                  <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
                </svg>
                <input id="search" placeholder="搜索应用、模型或能力..." aria-label="搜索应用、模型或能力" type="search">
              </div>
            </div>
            <div class="filters-bar">
              <button class="chip active" data-group="all">全部</button>
              <button v-for="group in CATALOG_GROUPS" :key="group.id" class="chip" :data-group="group.id">{{ group.label }}</button>
              <span class="result-count" id="resultCount">等待连接</span>
            </div>
            <div id="catalog"></div>
          </section>
        </div>
      </div>


    </div>
  </main>

  <!-- Launch App Dialog -->
  <dialog id="launchDialog">
    <div style="display:flex; justify-content:space-between; align-items:center; margin-bottom:12px;">
      <div>
        <span class="eyebrow">CREATE WITH APPS</span>
        <h2 style="margin-top:4px;">选择创作应用</h2>
      </div>
      <button class="btn ghost" id="closeLaunch" aria-label="关闭选择">✕</button>
    </div>
    <p>选择应用进入专业工作台；未安装的应用可从市场一键安装。</p>
    <div id="launchApps" style="display:flex; flex-direction:column; gap:10px; margin-top:16px;"></div>
  </dialog>

  <!-- Detail Drawer -->
  <div class="overlay" id="overlay">
    <section class="drawer" role="dialog" aria-modal="true" aria-labelledby="detailTitle" tabindex="-1">
      <div class="drawer-head">
        <span class="eyebrow">APPLICATION DETAIL</span>
        <button class="btn ghost" id="closeDrawer" aria-label="关闭应用详情">✕</button>
      </div>
      <div id="detailHead"></div>
      <div id="actions" class="action-row"></div>
      <AppNavigationPreferences v-if="detailAppId" :app-id="detailAppId" />
      <div id="progress"></div>
      <div class="detail-tabs" role="tablist">
        <button role="tab" data-detail-tab="overview" class="active">概览</button>
        <button role="tab" data-detail-tab="models">模型依赖</button>
        <button role="tab" data-detail-tab="capabilities">能力与渠道</button>
        <button role="tab" data-detail-tab="logs">操作记录</button>
      </div>
      <div id="detailContent" class="detail-content"></div>
    </section>
  </div>

  <!-- Confirm Dialog -->
  <dialog id="confirm">
    <h2 id="confirmTitle"></h2>
    <p id="confirmText"></p>
    <div class="buttons">
      <button class="btn" id="confirmCancel">取消</button>
      <button class="btn primary" id="confirmOK">确定</button>
    </div>
  </dialog>
  <AppDashboardWorkspace :active="dashboardActive" :installation-id="installationId" :titles="titles" />
</template>
