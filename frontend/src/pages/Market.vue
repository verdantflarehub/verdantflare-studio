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
        <div class="side-cluster-meta">Core 就绪 · 2 卡 RTX 5090</div>
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
        <!-- 1. The 4 Hardware Resource Pillars (Top KPI Row) -->
        <section class="res-metric-grid">
          <!-- Card 1: GPU Cluster -->
          <article class="res-metric-card" id="kpiGpu">
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
            <div class="res-metric-meter"><div class="res-metric-meter-fill" id="resGpuMeter" style="width: 9%;"></div></div>
            <div class="res-metric-sub" id="resGpuSub">显存 5.8 / 64 GB · PCIe 5.0 P2P</div>
          </article>

          <!-- Card 2: CPU Compute -->
          <article class="res-metric-card" id="kpiCpu">
            <div class="res-metric-top">
              <span class="res-metric-label">CPU 处理器算力</span>
              <div class="res-metric-icon">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                  <rect x="4" y="4" width="16" height="16" rx="2"></rect>
                  <path d="M9 9h6v6H9zM9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 15h3M1 9h3M1 15h3"></path>
                </svg>
              </div>
            </div>
            <div class="res-metric-val" id="resCpuVal">1.4% <small>/ 24 vCPUs</small></div>
            <div class="res-metric-meter"><div class="res-metric-meter-fill" id="resCpuMeter" style="width: 2%;"></div></div>
            <div class="res-metric-sub" id="resCpuSub">Load 0.19 · 24 核心高频运算</div>
          </article>

          <!-- Card 3: System RAM -->
          <article class="res-metric-card" id="kpiRam">
            <div class="res-metric-top">
              <span class="res-metric-label">系统运行内存</span>
              <div class="res-metric-icon">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                  <path d="M2 7h20v10H2zM6 7v10M10 7v10M14 7v10M18 7v10"></path>
                </svg>
              </div>
            </div>
            <div class="res-metric-val" id="resRamVal">16.0 <small>/ 256 GB</small></div>
            <div class="res-metric-meter"><div class="res-metric-meter-fill" id="resRamMeter" style="width: 6.4%;"></div></div>
            <div class="res-metric-sub" id="resRamSub">可用 233 GB · 水位 6.4% 零换页</div>
          </article>

          <!-- Card 4: NVMe Fast Storage -->
          <article class="res-metric-card" id="kpiStorage">
            <div class="res-metric-top">
              <span class="res-metric-label">NVMe 本地存储</span>
              <div class="res-metric-icon">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                  <ellipse cx="12" cy="5" rx="9" ry="3"></ellipse>
                  <path d="M21 12c0 1.66-4 3-9 3s-9-1.34-9-3"></path>
                  <path d="M3 5v14c0 1.66 4 3 9 3s9-1.34 9-3V5"></path>
                </svg>
              </div>
            </div>
            <div class="res-metric-val" id="resStorageVal">546 GB <small>/ 7.3 TB</small></div>
            <div class="res-metric-meter"><div class="res-metric-meter-fill" id="resStorageMeter" style="width: 8%;"></div></div>
            <div class="res-metric-sub" id="resStorageSub">/data 3.7TB + / 3.5TB 就绪</div>
          </article>
        </section>

        <!-- 2. GPU 5090 Topology & Live Telemetry -->
        <section class="res-section">
          <div class="res-section-title">
            <div class="res-section-title-left">
              <span id="resTopologyTitle">RTX 5090 双卡拓扑架构 (2-GPU Node)</span>
              <span class="res-section-badge">PCIe 5.0 ×16 · P2P DMA 就绪</span>
              <span class="res-section-badge highlight">2GB 安全防爆余量</span>
            </div>
            <span class="eyebrow">NVIDIA System Management Interface (DCGM)</span>
          </div>
          <div class="gpu-grid">
            <!-- GPU cards dynamically populated by market.js -->
            <div class="gpu-card" data-gpu="0">
              <div class="gpu-card-head">
                <span class="gpu-id">GPU 0</span>
                <span class="gpu-name">NVIDIA GeForce RTX 5090 · 32GB</span>
                <span class="gpu-state active"><i class="gpu-dot"></i>运行中</span>
              </div>
              <div class="gpu-bar-wrap"><div class="gpu-bar-fill" style="width: 18%;"></div></div>
              <div class="gpu-stat-row">
                <span class="gpu-stat-vram">显存 <strong>5.8</strong> / 32 GB</span>
                <span class="gpu-stat-telemetry"><span class="gpu-temp">48°C</span> · <span class="gpu-power">23W</span></span>
              </div>
              <div class="gpu-workload">
                <svg class="workload-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"></circle><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path></svg>
                <span>承载：Image MCP (活跃模型推理就绪)</span>
              </div>
            </div>
            <div class="gpu-card" data-gpu="1">
              <div class="gpu-card-head">
                <span class="gpu-id">GPU 1</span>
                <span class="gpu-name">NVIDIA GeForce RTX 5090 · 32GB</span>
                <span class="gpu-state standby"><i class="gpu-dot"></i>就绪待命</span>
              </div>
              <div class="gpu-bar-wrap"><div class="gpu-bar-fill" style="width: 2%;"></div></div>
              <div class="gpu-stat-row">
                <span class="gpu-stat-vram">显存 <strong>0.0</strong> / 32 GB</span>
                <span class="gpu-stat-telemetry"><span class="gpu-temp">41°C</span> · <span class="gpu-power">6W</span></span>
              </div>
              <div class="gpu-workload">
                <svg class="workload-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"></circle><polyline points="12 6 12 12 14 14"></polyline></svg>
                <span>承载：Video MCP (待机就绪)</span>
              </div>
            </div>
          </div>
        </section>

        <!-- 3. Host Compute & Memory System (CPU + RAM 2-Column Split) -->
        <section class="res-section">
          <div class="res-section-title">
            <div class="res-section-title-left">
              <span>主机运算与系统内存</span>
              <span class="res-section-badge">24 vCPUs · 256 GB ECC DDR5</span>
            </div>
            <span class="eyebrow">Host Architecture & Kernel Telemetry</span>
          </div>
          <div class="res-host-grid">
            <!-- Left: CPU Details -->
            <div class="host-detail-card" id="cpuDetailCard">
              <div class="host-detail-head">
                <div class="host-detail-title-wrap">
                  <div class="host-icon-box">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><rect x="4" y="4" width="16" height="16" rx="2"></rect><path d="M9 9h6v6H9zM9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 15h3M1 9h3M1 15h3"></path></svg>
                  </div>
                  <div>
                    <div class="host-detail-name">CPU 处理器运算单元</div>
                    <div class="host-detail-desc">24 核心 / 线程 · x86_64 Linux 6.12</div>
                  </div>
                </div>
                <div class="host-detail-stat" id="cpuLivePct">1.4% <small>利用率</small></div>
              </div>
              <div class="res-metric-meter host-meter"><div class="res-metric-meter-fill" id="cpuLiveBar" style="width: 2%;"></div></div>
              <div class="host-meta-grid">
                <div class="host-meta-item">
                  <span class="host-meta-lbl">计算规格</span>
                  <strong class="host-meta-val" id="cpuCoresVal">24 vCPUs</strong>
                </div>
                <div class="host-meta-item">
                  <span class="host-meta-lbl">系统负载 (1m/5m/15m)</span>
                  <strong class="host-meta-val" id="cpuLoadVal">0.19, 0.17, 0.21</strong>
                </div>
                <div class="host-meta-item">
                  <span class="host-meta-lbl">操作系统内核</span>
                  <strong class="host-meta-val">Linux 6.12 (Debian)</strong>
                </div>
                <div class="host-meta-item">
                  <span class="host-meta-lbl">调度排队状态</span>
                  <strong class="host-meta-val highlight-green">零排队延迟 · 就绪</strong>
                </div>
              </div>
            </div>

            <!-- Right: Memory Details -->
            <div class="host-detail-card" id="ramDetailCard">
              <div class="host-detail-head">
                <div class="host-detail-title-wrap">
                  <div class="host-icon-box">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M2 7h20v10H2zM6 7v10M10 7v10M14 7v10M18 7v10"></path></svg>
                  </div>
                  <div>
                    <div class="host-detail-name">系统运行内存池 (RAM)</div>
                    <div class="host-detail-desc">256 GB 高速通道 · 零换页开销</div>
                  </div>
                </div>
                <div class="host-detail-stat" id="ramLiveVal">16.0 <small>/ 256 GB (6.4%)</small></div>
              </div>
              <div class="res-metric-meter host-meter"><div class="res-metric-meter-fill" id="ramLiveBar" style="width: 6.4%;"></div></div>
              <div class="host-meta-grid">
                <div class="host-meta-item">
                  <span class="host-meta-lbl">活跃占用 (Used)</span>
                  <strong class="host-meta-val" id="ramUsedVal">16.0 GB</strong>
                </div>
                <div class="host-meta-item">
                  <span class="host-meta-lbl">文件系统缓存 (Buff/Cache)</span>
                  <strong class="host-meta-val" id="ramCacheVal">27.4 GB</strong>
                </div>
                <div class="host-meta-item">
                  <span class="host-meta-lbl">剩余可用 (Available)</span>
                  <strong class="host-meta-val highlight-green" id="ramAvailVal">233.1 GB</strong>
                </div>
                <div class="host-meta-item">
                  <span class="host-meta-lbl">虚拟内存交换 (Swap)</span>
                  <strong class="host-meta-val">0 GB (零换页抖动)</strong>
                </div>
              </div>
            </div>
          </div>
        </section>

        <!-- 4. Storage Systems & NVMe Volumes -->
        <section class="res-section">
          <div class="res-section-title">
            <div class="res-section-title-left">
              <span>存储系统与持久化卷</span>
              <span class="res-section-badge">NVMe PCIe 4.0/5.0 直通</span>
              <span class="res-section-badge">7.2 GB/s 读带宽</span>
            </div>
            <span class="eyebrow">Local Fast-Tier & S3 Sync</span>
          </div>
          <div class="res-storage-cards">
            <!-- /data Partition -->
            <div class="storage-card" id="storageDataCard">
              <div class="storage-head">
                <div class="storage-mount-wrap">
                  <svg class="storage-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                    <rect x="2" y="2" width="20" height="8" rx="2" ry="2"></rect>
                    <rect x="2" y="14" width="20" height="8" rx="2" ry="2"></rect>
                    <line x1="6" y1="6" x2="6.01" y2="6"></line>
                    <line x1="6" y1="18" x2="6.01" y2="18"></line>
                  </svg>
                  <span class="storage-mount">/data</span>
                  <span class="storage-badge">AI 模型与制品高速卷 · Ext4</span>
                </div>
                <span class="storage-val" id="storageDataVal">266 GB <small>/ 3.7 TB (7%)</small></span>
              </div>
              <div class="storage-bar"><div class="storage-fill" id="storageDataBar" style="width: 7%;"></div></div>
              <div class="storage-meta">
                <span>承载：/data/models (模型缓存) + /data/artifacts (制品库)</span>
                <span class="storage-status-ok"><i class="gpu-dot"></i>可用余量：3.48 TB</span>
              </div>
            </div>

            <!-- / Root Partition -->
            <div class="storage-card" id="storageRootCard">
              <div class="storage-head">
                <div class="storage-mount-wrap">
                  <svg class="storage-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                    <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"></path>
                    <polyline points="7 10 12 15 17 10"></polyline>
                    <line x1="12" y1="15" x2="12" y2="3"></line>
                  </svg>
                  <span class="storage-mount">/</span>
                  <span class="storage-badge">宿主系统盘 · Ext4</span>
                </div>
                <span class="storage-val" id="storageRootVal">280 GB <small>/ 3.5 TB (8%)</small></span>
              </div>
              <div class="storage-bar"><div class="storage-fill" id="storageRootBar" style="width: 8%;"></div></div>
              <div class="storage-meta">
                <span>承载：Containerd 容器镜像层 + Pod 临时存储</span>
                <span class="storage-status-ok"><i class="gpu-dot"></i>可用余量：3.28 TB</span>
              </div>
            </div>
          </div>
        </section>

        <!-- 5. Workload Quota & Real Telemetry Section (Section 5) -->
        <section class="res-section" id="workloadSection">
          <div class="res-section-title">
            <div class="res-section-title-left">
              <span>已部署工作负载与算力配额对账</span>
              <span class="res-section-badge highlight" id="wlCountBadge">5 个活跃容器组</span>
              <span class="res-section-badge">DCGM + cAdvisor 实时同频</span>
              <span class="res-section-badge amber" style="background:rgba(181,128,50,0.12); color:#b58032; border-color:rgba(181,128,50,0.3);">动态防爆审计就绪</span>
            </div>
            <span class="eyebrow">CONTAINER LEVEL QUOTA & ACTUAL IN-USE</span>
          </div>

          <div class="workloads-container">
            <!-- 总体配额与预留对账总览条 -->
            <div class="quota-summary-strip" id="quotaSummaryStrip">
              <div class="quota-item">
                <span class="quota-item-lbl">
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="4" width="16" height="16" rx="2"></rect></svg>
                  GPU 卡分配率 (Quotas)
                </span>
                <div class="quota-item-val" id="quotaGpuVal">2 / 2 卡 <small>(100% 绑定)</small></div>
                <div class="quota-mini-meter"><div class="quota-mini-meter-fill" id="quotaGpuMeter" style="width: 100%;"></div></div>
              </div>

              <div class="quota-item">
                <span class="quota-item-lbl">
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 12h-4l-3 9L9 3l-3 9H2"></path></svg>
                  显存实际占用 (In-Use VRAM)
                </span>
                <div class="quota-item-val" id="quotaVramVal">5.8 / 64 GB <small>(9.1% 水位)</small></div>
                <div class="quota-mini-meter"><div class="quota-mini-meter-fill" id="quotaVramMeter" style="width: 9.1%;"></div></div>
              </div>

              <div class="quota-item">
                <span class="quota-item-lbl">
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="4" width="16" height="16" rx="2"></rect></svg>
                  CPU 申请配额 (Requests)
                </span>
                <div class="quota-item-val" id="quotaCpuVal">18.7 / 24 核 <small>(77.9% 预留)</small></div>
                <div class="quota-mini-meter"><div class="quota-mini-meter-fill" id="quotaCpuMeter" style="width: 77.9%;"></div></div>
              </div>

              <div class="quota-item">
                <span class="quota-item-lbl">
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M2 7h20v10H2z"></path></svg>
                  内存申请配额 (Requests)
                </span>
                <div class="quota-item-val" id="quotaMemVal">98.2 / 256 GB <small>(38.4% 预留)</small></div>
                <div class="quota-mini-meter"><div class="quota-mini-meter-fill" id="quotaMemMeter" style="width: 38.4%;"></div></div>
              </div>
            </div>

            <!-- 过滤器与搜索栏 -->
            <div class="workload-filters">
              <div class="workload-tabs">
                <button type="button" class="workload-tab-btn active" data-filter="all">全部工作负载 (5)</button>
                <button type="button" class="workload-tab-btn" data-filter="gpu">GPU 创作应用 (2)</button>
                <button type="button" class="workload-tab-btn" data-filter="infra">平台与基础设施 (3)</button>
              </div>
              <div class="workload-search">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"></circle><line x1="21" y1="21" x2="16.65" y2="16.65"></line></svg>
                <input type="text" id="workloadSearchInput" placeholder="过滤 Pod、应用名称或命名空间...">
              </div>
            </div>

            <!-- 正在进行的流水线操作通知 (如果存在) -->
            <div id="railActivity" style="display:none;"></div>

            <!-- 工作负载真实遥测卡片列表 -->
            <div class="workload-list" id="workloadList">
              <!-- Initial placeholders rendered here, will be refreshed dynamically via API -->
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
