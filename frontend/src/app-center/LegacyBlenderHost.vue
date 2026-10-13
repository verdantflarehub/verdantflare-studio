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
      <section id="blenderHost" hidden>
        <div class="blender-header">
          <nav id="blenderBreadcrumb" class="blender-breadcrumb" aria-label="面包屑" hidden></nav>
          <button id="blenderCreate" class="btn primary" disabled title="实例创建暂未开放" hidden>创建实例</button>
          <a id="blenderManageLink" class="btn" hidden>实例详情</a>
          <div id="blenderToolbar" class="blender-toolbar" hidden>
            <button class="btn" id="blenderRelease">释放控制权</button>
            <button class="btn primary" id="blenderSave">保存工程</button>
            <button class="btn" id="blenderCopy">复制 MCP 地址</button>
          </div>
        </div>
        <section id="blenderList" class="blender-management" aria-label="Blender 实例" hidden>
          <div class="intro"><button class="btn" id="blenderRefresh">刷新</button></div>
          <dl id="blenderSummary" class="summary panel" hidden></dl>
          <div class="filterbar"><div id="blenderFilters" class="filters" aria-label="运行状态筛选"><button class="filter" data-status="all" aria-pressed="true">全部</button><button class="filter" data-status="running" aria-pressed="false">运行中</button><button class="filter" data-status="stopped" aria-pressed="false">已停止</button><button class="filter" data-status="failed" aria-pressed="false">异常</button></div><label class="search"><span class="sr-only">搜索实例或项目</span><input id="blenderSearch" type="search" placeholder="搜索实例或项目" aria-label="搜索实例或项目"></label></div>
          <div id="blenderRows" class="instance-stack"></div>
          <p id="blenderListMessage" role="status" hidden></p>
          <button id="blenderListRetry" class="btn" hidden>重试</button>
        </section>
        <section id="blenderDetail" class="blender-management" aria-label="Blender 实例详情" hidden></section>
          <section id="blenderCreateView" class="blender-management" aria-label="创建 Blender 实例" hidden></section>
        <section id="blenderWorkspace" hidden>
          <div id="blenderViewport" class="blender-viewport">
            <div id="blenderLoading" class="blender-loading">
              <img class="blender-mark" src="/assets/blender-logo.svg" alt="">
              <h2>Blender</h2>
              <div id="blenderProgress" class="blender-progress" aria-hidden="true"></div>
              <p id="blenderMessage" role="status" aria-live="polite">正在打开</p>
              <button id="blenderAction" class="btn ghost">取消连接</button>
            </div>
            <iframe id="blenderFrame" title="Blender 三维工程编辑器" sandbox="allow-scripts allow-same-origin allow-pointer-lock" allow="autoplay; fullscreen" referrerpolicy="no-referrer" hidden></iframe>
          </div>
        </section>
      </section>
  </div>
</template>

<style scoped>
.blender-breadcrumb {
  display: none !important;
}
.blender-header {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 12px;
}
.intro {
  display: flex;
  justify-content: flex-end;
  margin-bottom: 12px;
}
</style>
