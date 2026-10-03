<script setup lang="ts">
import { onMounted, onBeforeUnmount } from "vue"
import { mountMarket } from "./market"
import vfLogo from "../assets/verdantflare-logo.png"

let dispose: (() => void) | undefined
onMounted(() => { dispose = mountMarket() })
onBeforeUnmount(() => dispose?.())
</script>

<template>
  <!-- Left Sidebar: Complete Integrated Brand & Primary Navigation -->
  <aside class="side" aria-label="Studio 主菜单">
    <div class="side-brand">
      <img id="vfSideLogo"
           :src="vfLogo"
           alt="VerdantFlare"
           width="26"
           height="26">
      <div class="brand-text">
        <span>青焰 · VerdantFlare</span>
        <small>STUDIO OS</small>
      </div>
    </div>

    <button class="side-create-btn"
            id="newTask"
            aria-label="新建创作任务">
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
       href="#/market"
       data-nav="workbench">
      <span class="nav-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
          <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"></path>
          <polyline points="9 22 9 12 15 12 15 22"></polyline>
        </svg>
      </span>
      <span>工作台</span>
    </a>

    <a class="nav-link selected"
       href="#/market"
       data-nav="market">
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

    <div class="nav-sub-group" id="groupNav">
      <!-- Injected by JavaScript -->
    </div>

    <a class="nav-link"
       href="#/market"
       data-nav="models">
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
       href="#/market"
       data-nav="resources">
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
       href="#/station/proxies"
       data-nav="proxies">
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
              title="当前连接：5090集群 (k8s.dev.verdantflarehub.com) · 点击查看资源看板">
        <div class="side-cluster-row">
          <div class="side-cluster-lead">
            <i class="dot on"></i>
            <span class="side-cluster-name">5090集群</span>
          </div>
          <span class="side-cluster-pill">在线</span>
        </div>
        <div class="side-cluster-meta">Core 就绪 · 8 卡 RTX 5090</div>
      </button>

      <!-- 2. User Login Card -->
      <div class="side-user-card"
           id="sideUserCard"
           title="当前操作员：admin · 青岚创意工作室">
        <div class="avatar">VF</div>
        <div class="side-user-info">
          <span class="side-user-name">admin</span>
          <span class="side-user-team">青岚创意工作室</span>
        </div>
      </div>

      <a class="nav-link side-help-link"
         href="#/market"
         data-placeholder="true"
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
              <span class="result-count" id="resultCount">等待连接</span>
            </div>
            <div id="catalog"></div>
          </section>
        </div>
      </div>

      <!-- Resource Management Dashboard (Decoupled Rail into Dedicated View) -->
      <div id="resourceView" class="resource-dashboard" hidden>
        <!-- KPI Row -->
        <section class="res-metric-grid">
          <article class="res-metric-card">
            <span class="res-metric-label">GPU 集群算力</span>
            <div class="res-metric-val">8× <small>RTX 5090</small></div>
            <span class="res-metric-sub">单节点 · dev.verdantflarehub.com</span>
          </article>
          <article class="res-metric-card">
            <span class="res-metric-label">显存使用率</span>
            <div class="res-metric-val" id="resVramTotal">92 <small>/ 256 GB</small></div>
            <span class="res-metric-sub">动态显存池 · 36% 水位</span>
          </article>
          <article class="res-metric-card">
            <span class="res-metric-label">本地模型缓存</span>
            <div class="res-metric-val">184 <small>GB</small></div>
            <span class="res-metric-sub">Fast-NVMe · 8 项就绪</span>
          </article>
          <article class="res-metric-card">
            <span class="res-metric-label">活跃生成管道</span>
            <div class="res-metric-val" id="resPipelineCount">2 <small>Active</small></div>
            <span class="res-metric-sub">MiniMax H3 · Wan Video</span>
          </article>
        </section>

        <!-- GPU 5090 8-Card Topology Cards -->
        <section class="res-section">
          <div class="res-section-title">
            <span>RTX 5090 集群拓扑卡片 (8-GPU Node)</span>
            <span class="eyebrow" style="margin-left:auto;">NVIDIA System Management Interface</span>
          </div>
          <div class="gpu-grid">
            <div class="gpu-card" data-gpu="0">
              <div class="gpu-card-head"><span class="gpu-id">GPU 0</span><span class="gpu-name">RTX 5090 · 32GB</span><span class="gpu-state active">运行中</span></div>
              <div class="gpu-bar-wrap"><div class="gpu-bar-fill" style="width: 72%;"></div></div>
              <div class="gpu-stat-row"><span>显存 23.0 / 32 GB</span><span>58°C · 340W</span></div>
              <div class="gpu-workload">承载：MiniMax H3 VDN (实例 0)</div>
            </div>
            <div class="gpu-card" data-gpu="1">
              <div class="gpu-card-head"><span class="gpu-id">GPU 1</span><span class="gpu-name">RTX 5090 · 32GB</span><span class="gpu-state active">运行中</span></div>
              <div class="gpu-bar-wrap"><div class="gpu-bar-fill" style="width: 72%;"></div></div>
              <div class="gpu-stat-row"><span>显存 23.0 / 32 GB</span><span>56°C · 335W</span></div>
              <div class="gpu-workload">承载：MiniMax H3 VDN (实例 1)</div>
            </div>
            <div class="gpu-card" data-gpu="2">
              <div class="gpu-card-head"><span class="gpu-id">GPU 2</span><span class="gpu-name">RTX 5090 · 32GB</span><span class="gpu-state active">运行中</span></div>
              <div class="gpu-bar-wrap"><div class="gpu-bar-fill" style="width: 58%;"></div></div>
              <div class="gpu-stat-row"><span>显存 18.5 / 32 GB</span><span>52°C · 280W</span></div>
              <div class="gpu-workload">承载：Wan 2.1 Video 推理</div>
            </div>
            <div class="gpu-card" data-gpu="3">
              <div class="gpu-card-head"><span class="gpu-id">GPU 3</span><span class="gpu-name">RTX 5090 · 32GB</span><span class="gpu-state active">运行中</span></div>
              <div class="gpu-bar-wrap"><div class="gpu-bar-fill" style="width: 45%;"></div></div>
              <div class="gpu-stat-row"><span>显存 14.4 / 32 GB</span><span>49°C · 210W</span></div>
              <div class="gpu-workload">承载：ComfyUI 渲染管线</div>
            </div>
            <div class="gpu-card" data-gpu="4">
              <div class="gpu-card-head"><span class="gpu-id">GPU 4</span><span class="gpu-name">RTX 5090 · 32GB</span><span class="gpu-state standby">就绪待命</span></div>
              <div class="gpu-bar-wrap"><div class="gpu-bar-fill" style="width: 8%;"></div></div>
              <div class="gpu-stat-row"><span>显存 2.6 / 32 GB</span><span>38°C · 45W</span></div>
              <div class="gpu-workload">承载：系统待命 (可调度)</div>
            </div>
            <div class="gpu-card" data-gpu="5">
              <div class="gpu-card-head"><span class="gpu-id">GPU 5</span><span class="gpu-name">RTX 5090 · 32GB</span><span class="gpu-state standby">就绪待命</span></div>
              <div class="gpu-bar-wrap"><div class="gpu-bar-fill" style="width: 8%;"></div></div>
              <div class="gpu-stat-row"><span>显存 2.6 / 32 GB</span><span>37°C · 42W</span></div>
              <div class="gpu-workload">承载：系统待命 (可调度)</div>
            </div>
            <div class="gpu-card" data-gpu="6">
              <div class="gpu-card-head"><span class="gpu-id">GPU 6</span><span class="gpu-name">RTX 5090 · 32GB</span><span class="gpu-state standby">就绪待命</span></div>
              <div class="gpu-bar-wrap"><div class="gpu-bar-fill" style="width: 8%;"></div></div>
              <div class="gpu-stat-row"><span>显存 2.6 / 32 GB</span><span>39°C · 45W</span></div>
              <div class="gpu-workload">承载：系统待命 (可调度)</div>
            </div>
            <div class="gpu-card" data-gpu="7">
              <div class="gpu-card-head"><span class="gpu-id">GPU 7</span><span class="gpu-name">RTX 5090 · 32GB</span><span class="gpu-state standby">就绪待命</span></div>
              <div class="gpu-bar-wrap"><div class="gpu-bar-fill" style="width: 8%;"></div></div>
              <div class="gpu-stat-row"><span>显存 2.6 / 32 GB</span><span>36°C · 40W</span></div>
              <div class="gpu-workload">承载：系统待命 (可调度)</div>
            </div>
          </div>
        </section>

        <!-- Storage & Volume Watermarks -->
        <section class="res-section">
          <div class="res-section-title">
            <span>存储系统与持久化卷</span>
            <span class="eyebrow" style="margin-left:auto;">Ceph / NVMe Local Storage</span>
          </div>
          <div class="res-storage-cards">
            <div class="storage-card">
              <div class="storage-title"><span>/data/models (Fast-NVMe 模型缓存)</span><span class="storage-val">184 GB / 1.8 TB</span></div>
              <div class="storage-bar"><div class="storage-fill" style="width: 10%;"></div></div>
              <div class="storage-meta"><span>已缓存 8 个模型文件</span><span>状态：健康 (读写就绪)</span></div>
            </div>
            <div class="storage-card">
              <div class="storage-title"><span>/data/artifacts (创作输出与产物)</span><span class="storage-val">420 GB / 3.6 TB</span></div>
              <div class="storage-bar"><div class="storage-fill" style="width: 12%;"></div></div>
              <div class="storage-meta"><span>归档策略：成都 S3 自动同步</span><span>可用空间：3.18 TB</span></div>
            </div>
          </div>
        </section>

        <!-- Deployment Activity Queue -->
        <section class="res-section">
          <div class="res-section-title">
            <span>部署流水线与任务状态</span>
            <span class="eyebrow" style="margin-left:auto;">Informer 同步中</span>
          </div>
          <div id="railActivity">
            <div class="activity-empty">
              暂无进行中的部署任务
              <p class="muted" style="margin-top:4px">所有已纳管工作负载健康运行中</p>
            </div>
          </div>
        </section>
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

  <!-- Toast Notification -->
  <div id="toast" class="toast" role="status" aria-live="polite"></div>

  <!-- Login Dialog -->
  <dialog id="loginDialog" aria-labelledby="loginTitle">
    <form id="loginForm">
      <div class="login-brand-header">
        <div class="login-logo-badge">
          <svg viewBox="0 0 24 24" width="22" height="22" fill="none" stroke="#087e60" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 2L2 7l10 5 10-5-10-5z"></path>
            <path d="M2 17l10 5 10-5"></path>
            <path d="M2 12l10 5 10-5"></path>
          </svg>
        </div>
        <div>
          <span class="eyebrow">VERDANTFLARE STATION</span>
          <h2 id="loginTitle">登录 Station</h2>
        </div>
      </div>
      <p class="login-subtitle">连接你的青焰创作工作区，调度 5090 集群算力与服务。</p>
      
      <div class="form-group">
        <label for="loginUsername">用户名</label>
        <div class="input-wrap">
          <input id="loginUsername" name="username" autocomplete="username" required value="admin" placeholder="输入用户名">
        </div>
      </div>

      <div class="form-group">
        <label for="loginPassword">访问密码</label>
        <div class="input-wrap">
          <input id="loginPassword" name="password" type="password" autocomplete="current-password" required placeholder="输入访问凭据密码">
        </div>
      </div>

      <p id="loginError" role="alert"></p>

      <button class="btn primary submit-btn" type="submit">
        <span>连接并进入工作区</span>
        <svg viewBox="0 0 24 24" width="16" height="16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
          <polyline points="9 18 15 12 9 6"></polyline>
        </svg>
      </button>
    </form>
  </dialog>
</template>
