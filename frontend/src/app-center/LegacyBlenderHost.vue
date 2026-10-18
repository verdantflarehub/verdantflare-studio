<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { mountBlender } from '../pages/blender-embed'
import { createBlenderFetcher } from './legacy-blender'
import '../theme/blender-management.css'

const props = defineProps<{ route: string; resourceBase?: string; mcpOrigin?: string; authorize: () => Promise<void> }>()
const emit = defineEmits<{ navigate: [route: string] }>()
const root = ref<HTMLElement>(), message = ref('')
const controller = new AbortController()
let adapter: ReturnType<typeof mountBlender> | undefined
onMounted(() => {
  const routeLocation = { get hash() { return '#' + props.route }, set hash(value: string) { emit('navigate', value.replace(/^#/, '')) }, origin: location.origin }
  const listen = (target: EventTarget, event: string, handler: EventListener) => {
    if (target === window && event === 'hashchange') return
    target.addEventListener(event, handler, { signal: controller.signal })
  }
  root.value!.addEventListener('click', event => {
    const anchor = (event.target as Element).closest<HTMLAnchorElement>('a[href^="#/market"]')
    if (!anchor) return
    event.preventDefault(); emit('navigate', anchor.getAttribute('href')!.slice(1))
  }, { capture: true, signal: controller.signal })
  adapter = mountBlender({ root: root.value!, location: routeLocation, listen, notice: (value: string) => { message.value = value }, authorize: props.authorize,
    fetcher: createBlenderFetcher(props.resourceBase), mcpOrigin: props.mcpOrigin ?? location.origin })
  void adapter.sync().catch(() => { message.value = '应用暂时无法读取，请返回后重试。' })
})
watch(() => props.route, () => { void adapter?.sync().catch(() => { message.value = '应用暂时无法读取，请返回后重试。' }) })
onBeforeUnmount(() => { controller.abort(); adapter?.dispose() })
</script>

<template>
  <div ref="root" class="legacy-blender-host">
    <p v-if="message" role="status">{{ message }}</p>
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
  </div>
</template>

