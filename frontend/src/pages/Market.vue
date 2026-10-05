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
       href="#/market?mode=models"
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
       href="#/market?mode=resources"
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
            <div class="res-metric-top">
              <span class="res-metric-label">GPU 集群算力</span>
              <div class="res-metric-icon">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                  <rect x="4" y="4" width="16" height="16" rx="2"></rect>
                  <rect x="9" y="9" width="6" height="6"></rect>
                  <path d="M9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 15h3M1 9h3M1 15h3"></path>
                </svg>
              </div>
            </div>
            <div class="res-metric-val" id="resGpuCount">2× <small>RTX 5090</small></div>
            <div class="res-metric-sub">
              <span class="dot on"></span>
              <span>单节点 · dev.verdantflarehub.com</span>
            </div>
          </article>
          <article class="res-metric-card">
            <div class="res-metric-top">
              <span class="res-metric-label">显存使用率</span>
              <div class="res-metric-icon">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M12 2v20M17 5H9.5a3.5 3.5 0 0 0 0 7h5a3.5 3.5 0 0 1 0 7H6"></path>
                </svg>
              </div>
            </div>
            <div class="res-metric-val" id="resVramTotal">7.3 <small>/ 64 GB</small></div>
            <div class="res-metric-meter"><div class="res-metric-meter-fill" style="width: 11%;"></div></div>
            <div class="res-metric-sub">动态显存池 · 11% 水位 · 2GB 防爆余量</div>
          </article>
          <article class="res-metric-card">
            <div class="res-metric-top">
              <span class="res-metric-label">本地模型缓存</span>
              <div class="res-metric-icon">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                  <ellipse cx="12" cy="5" rx="9" ry="3"></ellipse>
                  <path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3"></path>
                  <path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5"></path>
                </svg>
              </div>
            </div>
            <div class="res-metric-val">184 <small>GB / 1.8 TB</small></div>
            <div class="res-metric-meter"><div class="res-metric-meter-fill" style="width: 10%;"></div></div>
            <div class="res-metric-sub">Fast-NVMe · 8 项模型就绪</div>
          </article>
          <article class="res-metric-card">
            <div class="res-metric-top">
              <span class="res-metric-label">活跃生成管道</span>
              <div class="res-metric-icon">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                  <polyline points="22 12 18 12 15 21 9 3 6 12 2 12"></polyline>
                </svg>
              </div>
            </div>
            <div class="res-metric-val" id="resPipelineCount">2 <small>Active</small></div>
            <div class="res-metric-sub">
              <span class="dot on"></span>
              <span>MiniMax H3 · Wan Video 并发</span>
            </div>
          </article>
        </section>

        <!-- GPU 5090 Topology Cards -->
        <section class="res-section">
          <div class="res-section-title">
            <div class="res-section-title-left">
              <span id="resTopologyTitle">RTX 5090 集群拓扑卡片 (2-GPU Node)</span>
              <span class="res-section-badge">PCIe 5.0 ×16 · P2P DMA 就绪</span>
            </div>
            <span class="eyebrow">NVIDIA System Management Interface</span>
          </div>
          <div class="gpu-grid">
            <div class="gpu-card" data-gpu="0">
              <div class="gpu-card-head">
                <span class="gpu-id">GPU 0</span>
                <span class="gpu-name">RTX 5090 · 32GB</span>
                <span class="gpu-state active"><i class="gpu-dot"></i>运行中</span>
              </div>
              <div class="gpu-bar-wrap"><div class="gpu-bar-fill" style="width: 21%;"></div></div>
              <div class="gpu-stat-row">
                <span class="gpu-stat-vram">显存 <strong>6.6</strong> / 32 GB</span>
                <span class="gpu-stat-telemetry"><span class="gpu-temp">48°C</span> · <span class="gpu-power">120W</span></span>
              </div>
              <div class="gpu-workload">
                <svg class="workload-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"></circle><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path></svg>
                <span>承载：活动模型推理任务</span>
              </div>
            </div>
            <div class="gpu-card" data-gpu="1">
              <div class="gpu-card-head">
                <span class="gpu-id">GPU 1</span>
                <span class="gpu-name">RTX 5090 · 32GB</span>
                <span class="gpu-state standby"><i class="gpu-dot"></i>就绪待命</span>
              </div>
              <div class="gpu-bar-wrap"><div class="gpu-bar-fill" style="width: 2%;"></div></div>
              <div class="gpu-stat-row">
                <span class="gpu-stat-vram">显存 <strong>0.6</strong> / 32 GB</span>
                <span class="gpu-stat-telemetry"><span class="gpu-temp">42°C</span> · <span class="gpu-power">45W</span></span>
              </div>
              <div class="gpu-workload">
                <svg class="workload-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"></circle><polyline points="12 6 12 12 14 14"></polyline></svg>
                <span>承载：待机就绪 (2GB 防爆余量就绪)</span>
              </div>
            </div>
          </div>
        </section>

        <!-- Storage & Volume Watermarks -->
        <section class="res-section">
          <div class="res-section-title">
            <div class="res-section-title-left">
              <span>存储系统与持久化卷</span>
              <span class="res-section-badge">Ceph + NVMe Local Storage</span>
            </div>
            <span class="eyebrow">Local Fast-Tier & S3 Sync</span>
          </div>
          <div class="res-storage-cards">
            <div class="storage-card">
              <div class="storage-head">
                <div class="storage-mount-wrap">
                  <svg class="storage-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                    <rect x="2" y="2" width="20" height="8" rx="2" ry="2"></rect>
                    <rect x="2" y="14" width="20" height="8" rx="2" ry="2"></rect>
                    <line x1="6" y1="6" x2="6.01" y2="6"></line>
                    <line x1="6" y1="18" x2="6.01" y2="18"></line>
                  </svg>
                  <span class="storage-mount">/data/models</span>
                  <span class="storage-badge">Fast-NVMe PCIe 5.0</span>
                </div>
                <span class="storage-val">184 GB <small>/ 1.8 TB (10%)</small></span>
              </div>
              <div class="storage-bar"><div class="storage-fill" style="width: 10%;"></div></div>
              <div class="storage-meta">
                <span>已缓存 8 个模型权重文件</span>
                <span class="storage-status-ok"><i class="gpu-dot"></i>健康 (7.2 GB/s 顺序读)</span>
              </div>
            </div>
            <div class="storage-card">
              <div class="storage-head">
                <div class="storage-mount-wrap">
                  <svg class="storage-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                    <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
                    <polyline points="7 10 12 15 17 10"></polyline>
                    <line x1="12" y1="15" x2="12" y2="3"></line>
                  </svg>
                  <span class="storage-mount">/data/artifacts</span>
                  <span class="storage-badge">Local SSD + Ceph RBD</span>
                </div>
                <span class="storage-val">420 GB <small>/ 3.6 TB (12%)</small></span>
              </div>
              <div class="storage-bar"><div class="storage-fill" style="width: 12%;"></div></div>
              <div class="storage-meta">
                <span>归档策略：成都 S3 历史异步归档</span>
                <span class="storage-status-ok"><i class="gpu-dot"></i>可用余量：3.18 TB</span>
              </div>
            </div>
          </div>
        </section>

        <!-- Deployment Activity Queue -->
        <section class="res-section">
          <div class="res-section-title">
            <div class="res-section-title-left">
              <span>部署流水线与任务状态</span>
              <span class="res-section-badge">
                <span class="dot on" style="display:inline-block; width:6px; height:6px; margin-right:4px;"></span>
                Informer 实时同频
              </span>
            </div>
            <span class="eyebrow">Cluster Pods & Workloads</span>
          </div>
          <div id="railActivity">
            <div class="activity-empty">
              <div class="activity-empty-shield">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                  <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"></path>
                  <path d="M9 12l2 2 4-4"></path>
                </svg>
              </div>
              <div class="activity-empty-title">所有已纳管创作工作负载健康运行中</div>
              <p class="activity-empty-sub">暂无阻塞或排队中的部署任务 · Station Core 与集群 Informer 实时同频</p>
              <div class="activity-workloads">
                <span class="activity-pill"><i class="dot on"></i>video-mcp-server: 就绪</span>
                <span class="activity-pill"><i class="dot on"></i>image-mcp-server: 就绪</span>
                <span class="activity-pill"><i class="dot on"></i>station-core: 运行中</span>
              </div>
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
