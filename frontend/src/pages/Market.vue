<script setup lang="ts">
import { onMounted, onBeforeUnmount } from "vue"
import { mountMarket } from "./market"
import "../theme/blender-management.css"
import '../theme/comfyui-management.css'

let dispose: (() => void) | undefined
onMounted(() => { dispose = mountMarket() })
onBeforeUnmount(() => dispose?.())
</script>

<template>
  <!-- Main Content Area: Pure Workspace -->
  <main class="main">
    <div class="workspace">
      <!-- Clean, Focused Heading Zone for Sub-business Workspace -->
      <header class="heading">
        <div class="heading-left">
          <h1 id="pageTitle">应用市场</h1>
          <p id="pageSubtitle">为 Station 安装创作应用，管理模型与运行状态。</p>
        </div>
        <div class="heading-actions">
          <button class="btn ghost" id="theme" aria-label="切换浅色或深色主题">深色主题</button>
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
      <section id="blenderHost" hidden>
        <div class="blender-header">
          <nav id="blenderBreadcrumb" class="blender-breadcrumb" aria-label="面包屑"></nav>
          <button id="blenderCreate" class="btn primary" disabled title="实例创建暂未开放" hidden>创建实例</button>
          <a id="blenderManageLink" class="btn" hidden>实例详情</a>
          <div id="blenderToolbar" class="blender-toolbar" hidden>
            <button class="btn" id="blenderRelease">释放控制权</button>
            <button class="btn primary" id="blenderSave">保存工程</button>
            <button class="btn" id="blenderCopy">复制 MCP 地址</button>
          </div>
        </div>
        <section id="blenderList" class="blender-management" aria-label="Blender 实例" hidden>
          <div class="intro"><div><h1>你的创作环境</h1><p>独立的 Blender 工作区，项目与计算资源在这里管理。</p></div><button class="btn" id="blenderRefresh">刷新</button></div>
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

      <section id="comfyuiHost" class="comfyui-management" aria-label="ComfyUI 实例" hidden>
        <div class="topline"><nav class="crumbs" aria-label="面包屑"><a href="#/market">应用市场</a><span>/</span><a href="#/market?comfyui=instances">ComfyUI</a><span id="comfyuiInstanceCrumb" hidden>/ <span id="comfyuiInstanceName" aria-current="page"></span></span></nav><div class="actions"><span id="comfyuiEditorMessage" class="comfyui-editor-status" role="status" hidden></span><button id="comfyuiRefresh" class="btn">刷新</button><button id="comfyuiTheme" class="btn" aria-label="切换主题">◐</button></div></div>
        <div id="comfyuiDirectory"><div class="intro"><h1>你的 ComfyUI 实例</h1></div>
        <div class="filterbar"><span id="comfyuiCount">我的实例 · —</span><input id="comfyuiSearch" type="search" aria-label="搜索 ComfyUI 实例" placeholder="搜索实例"></div>
        <p id="comfyuiMessage" role="status">正在获取实例…</p>
        <div id="comfyuiRows"></div></div>
        <section id="comfyuiWorkspace" hidden><div id="comfyuiEditorFallback" class="comfyui-editor-fallback"><p id="comfyuiEditorHint">正在打开 ComfyUI…</p></div><div id="comfyuiFrameHost"></div></section>
      </section>

      <div id="marketView" class="market-columns">
        <div class="market-primary">
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
              <button class="chip" data-group="image">Image</button>
              <button class="chip" data-group="music">Music</button>
              <button class="chip" data-group="video">Video</button>
              <button class="chip" data-group="desktop">创作工具</button>
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
</template>
