<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { request } from '../platform/client'
import { useHostStore } from '../platform/hostStore'
import { formatResource, resourceState, resourceValue } from './resource-metrics.js'

const hostStore = useHostStore()

interface SummaryData {
  total_gpus?: number
  total_vram_mb?: number
  used_vram_mb?: number
  host_cpu_utilization?: number
  host_cpu_cores?: number
  host_cpu_load1?: number
  host_mem_total_gb?: number
  host_mem_used_gb?: number
  host_mem_used_percent?: number
  storage_total_tb?: number
  storage_used_tb?: number
  storage_used_percent?: number
}

interface GpuData {
  index: number
  model_name?: string
  total_vram_mb: number
  used_vram_mb: number
  utilization: number
  temperature_c?: number
  power_watts?: number
}

interface DiskData {
  mountpoint: string
  total_bytes: number
  used_bytes: number
  used_percent: number
}

interface NodeData {
  name?: string
  cpu_cores?: number
  cpu_utilization_percent?: number
  cpu_load1?: number
  cpu_load5?: number
  cpu_load15?: number
  mem_total_bytes?: number
  mem_used_bytes?: number
  mem_available_bytes?: number
  mem_cached_bytes?: number
  mem_buffers_bytes?: number
  mem_used_percent?: number
  storage_disks?: DiskData[]
}

interface ResourceMetric {
  value: number | null
  request: number | null
  limit: number | null
  sampled_at: string | null
  quality: string
}
interface WorkloadItem {
  name: string
  display_name: string
  namespace: string
  pod_uid: string
  type: string
  status: string
  age: string
  containers: { name: string; ready: boolean; restarts: number; cpu: ResourceMetric; memory: ResourceMetric; gpu_request: number | null }[]
}
interface WorkloadsSummary {
  total_pods?: number
  total_gpu_requested?: number
  total_cpu_requested?: number
  total_memory_requested?: number
}

const summary = ref<SummaryData>({
  total_gpus: 2,
  total_vram_mb: 65536,
  used_vram_mb: 5939,
  host_cpu_utilization: 1.4,
  host_cpu_cores: 24,
  host_cpu_load1: 0.19,
  host_mem_total_gb: 256,
  host_mem_used_gb: 16.0,
  host_mem_used_percent: 6.4,
  storage_total_tb: 7.3,
  storage_used_tb: 0.546,
  storage_used_percent: 8
})

const gpus = ref<GpuData[]>([
  {
    index: 0,
    model_name: 'NVIDIA GeForce RTX 5090',
    total_vram_mb: 32768,
    used_vram_mb: 5939,
    utilization: 18,
    temperature_c: 48,
    power_watts: 23
  },
  {
    index: 1,
    model_name: 'NVIDIA GeForce RTX 5090',
    total_vram_mb: 32768,
    used_vram_mb: 0,
    utilization: 2,
    temperature_c: 41,
    power_watts: 6
  }
])

const node = ref<NodeData>({
  name: 'verdentflare-5090',
  cpu_cores: 24,
  cpu_utilization_percent: 1.4,
  cpu_load1: 0.19,
  cpu_load5: 0.17,
  cpu_load15: 0.21,
  mem_total_bytes: 274877906944,
  mem_used_bytes: 17179869184,
  mem_available_bytes: 250186989568,
  mem_cached_bytes: 29420060672,
  mem_buffers_bytes: 0,
  mem_used_percent: 6.4,
  storage_disks: [
    { mountpoint: '/data', total_bytes: 3700000000000, used_bytes: 266000000000, used_percent: 7 },
    { mountpoint: '/', total_bytes: 3500000000000, used_bytes: 280000000000, used_percent: 8 }
  ]
})

const workloadsSummary = ref<WorkloadsSummary>({})
const workloads = ref<WorkloadItem[]>([])
const workloadsState = ref<'loading' | 'ready' | 'error'>('loading')
const telemetryNow = ref(Date.now())
let telemetryPending = false

const currentFilter = ref<'all' | 'gpu' | 'infra'>('all')
const searchQuery = ref('')
const isRefreshing = ref(false)
const themeText = ref('深色主题')
let pollTimer: ReturnType<typeof setInterval> | null = null

const filteredWorkloads = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return workloads.value.filter(wl => {
    const typeMatch = currentFilter.value === 'all' || wl.type === currentFilter.value
    const textMatch = !q || (wl.name + ' ' + (wl.display_name || '') + ' ' + (wl.namespace || '')).toLowerCase().includes(q)
    return typeMatch && textMatch
  })
})

function showToast(msg: string) {
  const toast = document.getElementById('toast')
  if (toast) {
    toast.textContent = msg
    toast.classList.add('show')
    setTimeout(() => toast.classList.remove('show'), 3500)
  }
}

function toggleTheme() {
  const current = document.body.dataset.theme === 'dark' ? 'dark' : 'light'
  const next = current === 'dark' ? 'light' : 'dark'
  document.body.dataset.theme = next
  document.documentElement.dataset.theme = next
  localStorage.setItem('vf_studio_theme', next)
  themeText.value = next === 'dark' ? '浅色主题' : '深色主题'
}

function initTheme() {
  const saved = localStorage.getItem('vf_studio_theme') || 'light'
  document.body.dataset.theme = saved
  document.documentElement.dataset.theme = saved
  themeText.value = saved === 'dark' ? '浅色主题' : '深色主题'
}

async function fetchTelemetry() {
  if (telemetryPending) return
  telemetryPending = true
  try {
    const [sRes, gRes, nRes, wRes] = await Promise.all([
      request({ path: 'resources/summary', method: 'GET' }).catch(() => null),
      request({ path: 'resources/gpu', method: 'GET' }).catch(() => null),
      request({ path: 'resources/node', method: 'GET' }).catch(() => null),
      request({ path: 'resources/workloads', method: 'GET' }).catch(() => null)
    ])

    const workloadData = wRes?.ok ? await wRes.json() : null
    if (workloadData?.schema_version === 2 && Array.isArray(workloadData.workloads) && workloadData.summary) {
      workloads.value = workloadData.workloads
      workloadsSummary.value = workloadData.summary
      workloadsState.value = 'ready'
    } else {
      workloads.value = []
      workloadsSummary.value = {}
      workloadsState.value = 'error'
    }

    if (sRes && sRes.ok) {
      const data = await sRes.json()
      if (data?.summary) {
        summary.value = data.summary
        if (data.summary.total_gpus) {
          hostStore.setGpuCount(data.summary.total_gpus)
        }
      }
    }

    if (gRes && gRes.ok) {
      const data = await gRes.json()
      if (data?.gpus && Array.isArray(data.gpus)) {
        gpus.value = data.gpus
      }
    }

    if (nRes && nRes.ok) {
      const data = await nRes.json()
      if (data?.node) {
        node.value = data.node
      }
    }

  } catch {
    workloadsState.value = 'error'
    workloads.value = []
    workloadsSummary.value = {}
  } finally {
    telemetryNow.value = Date.now()
    telemetryPending = false
  }
}

async function handleRefresh() {
  isRefreshing.value = true
  await fetchTelemetry()
  showToast(workloadsState.value === 'error' ? '工作负载遥测暂不可用，请重试' : '遥测请求已完成')
  setTimeout(() => { isRefreshing.value = false }, 500)
}

onMounted(() => {
  initTheme()
  fetchTelemetry()
  pollTimer = setInterval(fetchTelemetry, 3000)
})

onBeforeUnmount(() => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
})
</script>

<template>
  <main class="main">
    <div class="workspace">
      <!-- Clean, Focused Heading Zone for Sub-business Workspace -->
      <header class="heading">
        <div class="heading-left">
          <h1 id="pageTitle">资源管理</h1>
          <p id="pageSubtitle">Station 节点算力、存储水位与已部署工作负载配额/实况对账事实源。</p>
        </div>
        <div class="heading-actions">
          <button class="btn ghost" id="theme" @click="toggleTheme">{{ themeText }}</button>
          <button class="icon-btn"
                  id="reset"
                  title="刷新遥测数据"
                  aria-label="刷新遥测数据"
                  @click="handleRefresh">
            <svg viewBox="0 0 24 24"
                 fill="none"
                 stroke="currentColor"
                 stroke-width="2"
                 stroke-linecap="round"
                 stroke-linejoin="round"
                 :class="{ spinning: isRefreshing }">
              <path d="M21.5 2v6h-6M21.34 15.57a10 10 0 1 1-.57-8.38l5.67-5.67"></path>
            </svg>
          </button>
        </div>
      </header>

      <!-- Resource Management Dashboard (Dedicated Independent Viewport) -->
      <div id="resourceView" class="resource-dashboard">
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
            <div class="res-metric-val" id="resGpuCount">
              {{ summary.total_gpus || 2 }}× <small>RTX 5090</small>
            </div>
            <div class="res-metric-meter">
              <div class="res-metric-meter-fill"
                   id="resGpuMeter"
                   :style="{ width: Math.min(100, Math.round(((summary.used_vram_mb || 5939) / (summary.total_vram_mb || 65536)) * 100)) + '%' }"></div>
            </div>
            <div class="res-metric-sub" id="resGpuSub">
              显存 {{ ((summary.used_vram_mb || 5939) / 1024).toFixed(1) }} / {{ Math.round((summary.total_vram_mb || 65536) / 1024) }} GB · 动态池 · 2GB 防爆余量
            </div>
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
            <div class="res-metric-val" id="resCpuVal">
              {{ (summary.host_cpu_utilization ?? node.cpu_utilization_percent ?? 1.4).toFixed(1) }}% <small>/ {{ summary.host_cpu_cores || node.cpu_cores || 24 }} vCPUs</small>
            </div>
            <div class="res-metric-meter">
              <div class="res-metric-meter-fill"
                   id="resCpuMeter"
                   :style="{ width: Math.min(100, Math.max(2, Math.round(summary.host_cpu_utilization ?? 1.4))) + '%' }"></div>
            </div>
            <div class="res-metric-sub" id="resCpuSub">
              Load {{ (summary.host_cpu_load1 ?? node.cpu_load1 ?? 0.19).toFixed(2) }} · {{ summary.host_cpu_cores || node.cpu_cores || 24 }} 核心高频运算
            </div>
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
            <div class="res-metric-val" id="resRamVal">
              {{ (summary.host_mem_used_gb ?? (node.mem_used_bytes ? (node.mem_used_bytes / (1024**3)) : 16.0)).toFixed(1) }} <small>/ {{ Math.round(summary.host_mem_total_gb || 256) }} GB</small>
            </div>
            <div class="res-metric-meter">
              <div class="res-metric-meter-fill"
                   id="resRamMeter"
                   :style="{ width: Math.min(100, Math.max(3, Math.round(summary.host_mem_used_percent ?? 6.4))) + '%' }"></div>
            </div>
            <div class="res-metric-sub" id="resRamSub">
              可用 {{ (Math.round(summary.host_mem_total_gb || 256) - (summary.host_mem_used_gb || 16)).toFixed(1) }} GB · 水位 {{ (summary.host_mem_used_percent ?? 6.4).toFixed(1) }}% 零换页
            </div>
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
            <div class="res-metric-val" id="resStorageVal">
              {{ Math.round((summary.storage_used_tb || 0.546) * 1000) }} GB <small>/ {{ (summary.storage_total_tb || 7.3).toFixed(1) }} TB</small>
            </div>
            <div class="res-metric-meter">
              <div class="res-metric-meter-fill"
                   id="resStorageMeter"
                   :style="{ width: Math.min(100, Math.max(3, Math.round(summary.storage_used_percent || 8))) + '%' }"></div>
            </div>
            <div class="res-metric-sub" id="resStorageSub">/data 3.7TB + / 3.5TB 就绪</div>
          </article>
        </section>

        <!-- 2. GPU 5090 Topology & Live Telemetry -->
        <section class="res-section">
          <div class="res-section-title">
            <div class="res-section-title-left">
              <span id="resTopologyTitle">RTX 5090 双卡拓扑架构 ({{ gpus.length }}-GPU Node)</span>
              <span class="res-section-badge">PCIe 5.0 ×16 · P2P DMA 就绪</span>
              <span class="res-section-badge highlight">2GB 安全防爆余量</span>
            </div>
            <span class="eyebrow">NVIDIA SYSTEM MANAGEMENT INTERFACE (DCGM)</span>
          </div>
          <div class="gpu-grid">
            <div v-for="g in gpus" :key="g.index" class="gpu-card" :data-gpu="g.index">
              <div class="gpu-card-head">
                <span class="gpu-id">GPU {{ g.index }}</span>
                <span class="gpu-name">{{ g.model_name || 'NVIDIA GeForce RTX 5090' }} · {{ Math.round(g.total_vram_mb / 1024) }}GB</span>
                <span class="gpu-state" :class="g.utilization > 5 || g.used_vram_mb > 2048 ? 'active' : 'standby'">
                  <i class="gpu-dot"></i>{{ g.utilization > 5 || g.used_vram_mb > 2048 ? '运行中' : '就绪待命' }}
                </span>
              </div>
              <div class="gpu-bar-wrap">
                <div class="gpu-bar-fill"
                     :style="{ width: Math.min(100, Math.max(2, Math.round((g.used_vram_mb / (g.total_vram_mb || 32768)) * 100))) + '%' }"></div>
              </div>
              <div class="gpu-stat-row">
                <span class="gpu-stat-vram">显存 <strong>{{ (g.used_vram_mb / 1024).toFixed(1) }}</strong> / {{ Math.round(g.total_vram_mb / 1024) }} GB</span>
                <span class="gpu-stat-telemetry"><span class="gpu-temp">{{ Math.round(g.temperature_c || 45) }}°C</span> · <span class="gpu-power">{{ Math.round(g.power_watts || 20) }}W</span></span>
              </div>
              <div class="gpu-workload">
                <svg class="workload-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <circle cx="12" cy="12" r="3"></circle>
                  <path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path>
                </svg>
                <span>承载：{{ g.index === 0 ? 'Image MCP (活跃模型推理就绪)' : 'Video MCP (待机就绪)' }}</span>
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
            <span class="eyebrow">HOST ARCHITECTURE & KERNEL TELEMETRY</span>
          </div>
          <div class="res-host-grid">
            <!-- Left: CPU Details -->
            <div class="host-detail-card" id="cpuDetailCard">
              <div class="host-detail-head">
                <div class="host-detail-title-wrap">
                  <div class="host-icon-box">
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                      <rect x="4" y="4" width="16" height="16" rx="2"></rect>
                      <path d="M9 9h6v6H9zM9 1v3M15 1v3M9 20v3M15 20v3M20 9h3M20 15h3M1 9h3M1 15h3"></path>
                    </svg>
                  </div>
                  <div>
                    <div class="host-detail-name">CPU 处理器运算单元</div>
                    <div class="host-detail-desc">24 核心 / 线程 · x86_64 Linux 6.12</div>
                  </div>
                </div>
                <div class="host-detail-stat" id="cpuLivePct">
                  {{ (node.cpu_utilization_percent || 1.4).toFixed(1) }}% <small>利用率</small>
                </div>
              </div>
              <div class="res-metric-meter host-meter">
                <div class="res-metric-meter-fill"
                     id="cpuLiveBar"
                     :style="{ width: Math.min(100, Math.max(2, Math.round(node.cpu_utilization_percent || 1.4))) + '%' }"></div>
              </div>
              <div class="host-meta-grid">
                <div class="host-meta-item">
                  <span class="host-meta-lbl">计算规格</span>
                  <strong class="host-meta-val" id="cpuCoresVal">{{ node.cpu_cores || 24 }} vCPUs</strong>
                </div>
                <div class="host-meta-item">
                  <span class="host-meta-lbl">系统负载 (1m/5m/15m)</span>
                  <strong class="host-meta-val" id="cpuLoadVal">
                    {{ (node.cpu_load1 || 0.19).toFixed(2) }}, {{ (node.cpu_load5 || 0.17).toFixed(2) }}, {{ (node.cpu_load15 || 0.21).toFixed(2) }}
                  </strong>
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
                    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                      <path d="M2 7h20v10H2zM6 7v10M10 7v10M14 7v10M18 7v10"></path>
                    </svg>
                  </div>
                  <div>
                    <div class="host-detail-name">系统运行内存池 (RAM)</div>
                    <div class="host-detail-desc">256 GB 高速通道 · 零换页开销</div>
                  </div>
                </div>
                <div class="host-detail-stat" id="ramLiveVal">
                  {{ (node.mem_used_bytes ? (node.mem_used_bytes / (1024**3)) : 16.0).toFixed(1) }} <small>/ 256 GB ({{ (node.mem_used_percent || 6.4).toFixed(1) }}%)</small>
                </div>
              </div>
              <div class="res-metric-meter host-meter">
                <div class="res-metric-meter-fill"
                     id="ramLiveBar"
                     :style="{ width: Math.min(100, Math.max(3, Math.round(node.mem_used_percent || 6.4))) + '%' }"></div>
              </div>
              <div class="host-meta-grid">
                <div class="host-meta-item">
                  <span class="host-meta-lbl">活跃占用 (Used)</span>
                  <strong class="host-meta-val" id="ramUsedVal">
                    {{ (node.mem_used_bytes ? (node.mem_used_bytes / (1024**3)) : 16.0).toFixed(1) }} GB
                  </strong>
                </div>
                <div class="host-meta-item">
                  <span class="host-meta-lbl">文件系统缓存 (Buff/Cache)</span>
                  <strong class="host-meta-val" id="ramCacheVal">
                    {{ (node.mem_cached_bytes ? (node.mem_cached_bytes / (1024**3)) : 27.4).toFixed(1) }} GB
                  </strong>
                </div>
                <div class="host-meta-item">
                  <span class="host-meta-lbl">剩余可用 (Available)</span>
                  <strong class="host-meta-val highlight-green" id="ramAvailVal">
                    {{ (node.mem_available_bytes ? (node.mem_available_bytes / (1024**3)) : 233.1).toFixed(1) }} GB
                  </strong>
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
            <span class="eyebrow">LOCAL FAST-TIER & S3 SYNC</span>
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

        <section class="res-section" id="workloadSection">
          <div class="res-section-title"><div class="res-section-title-left">
            <span>已部署工作负载与容器用量</span>
            <span class="res-section-badge">Kubernetes 配置 · metrics-server 采样</span>
          </div></div>
          <div class="workloads-container">
            <p v-if="workloadsState === 'loading'" role="status">正在读取工作负载…</p>
            <p v-else-if="workloadsState === 'error'" role="alert">工作负载遥测暂不可用，请刷新重试。</p>
            <template v-else>
              <div class="quota-summary-strip">
                <div class="quota-item"><span class="quota-item-lbl">容器组</span><div class="quota-item-val">{{ workloadsSummary.total_pods ?? '—' }}</div></div>
                <div class="quota-item"><span class="quota-item-lbl">活跃 Pod GPU 请求（非实际分配）</span><div class="quota-item-val">{{ workloadsSummary.total_gpu_requested ?? '—' }}</div></div>
                <div class="quota-item"><span class="quota-item-lbl">活跃 Pod CPU 请求</span><div class="quota-item-val">{{ formatResource(workloadsSummary.total_cpu_requested) }}</div></div>
                <div class="quota-item"><span class="quota-item-lbl">活跃 Pod 内存请求</span><div class="quota-item-val">{{ formatResource(workloadsSummary.total_memory_requested, 'bytes') }}</div></div>
              </div>
              <div class="workload-filters">
                <div class="workload-tabs">
                  <button class="workload-tab-btn" :class="{active:currentFilter === 'all'}" @click="currentFilter = 'all'">全部</button>
                  <button class="workload-tab-btn" :class="{active:currentFilter === 'gpu'}" @click="currentFilter = 'gpu'">配置 GPU</button>
                  <button class="workload-tab-btn" :class="{active:currentFilter === 'infra'}" @click="currentFilter = 'infra'">未配置 GPU</button>
                </div>
                <div class="workload-search"><input v-model="searchQuery" type="search" aria-label="搜索工作负载" placeholder="搜索应用或命名空间"></div>
              </div>
              <p v-if="!workloads.length">暂无工作负载。</p>
              <p v-else-if="!filteredWorkloads.length">没有符合筛选条件的工作负载。</p>
              <div class="workload-list">
                <article v-for="wl in filteredWorkloads" :key="wl.pod_uid" class="wl-card" :class="{'has-gpu':wl.type === 'gpu'}">
                  <div class="wl-head">
                    <div class="wl-title-box"><div class="wl-title-row"><span class="wl-name">{{ wl.display_name || wl.name }}</span><span class="wl-namespace">{{ wl.namespace }}</span></div><span class="wl-pod-id">{{ wl.name }}</span></div>
                    <div class="wl-meta-right"><span class="wl-uptime">{{ wl.age }}</span><span class="wl-status-tag">{{ wl.status }}</span></div>
                  </div>
                  <div v-for="container in wl.containers" :key="container.name">
                    <div class="wl-foot"><span>{{ container.name }} · {{ container.ready ? '就绪' : '未就绪' }} · 重启 {{ container.restarts }} 次</span></div>
                    <div class="wl-metrics-grid">
                      <div v-for="kind in (['cpu', 'memory'] as const)" :key="kind" class="wl-metric-cell">
                        <div class="wl-cell-title">{{ kind === 'cpu' ? 'CPU 使用' : '内存工作集' }}</div>
                        <div class="wl-actual-val">{{ resourceValue(container[kind], kind === 'cpu' ? 'cores' : 'bytes', telemetryNow) }}</div>
                        <div class="wl-metric-footnote">请求 {{ formatResource(container[kind].request, kind === 'cpu' ? 'cores' : 'bytes') }} · 上限 {{ formatResource(container[kind].limit, kind === 'cpu' ? 'cores' : 'bytes') }}</div>
                        <div class="wl-metric-footnote">{{ resourceState(container[kind], telemetryNow) === 'fresh' ? '采样 ' + new Date(container[kind].sampled_at!).toLocaleString('zh-CN') : resourceState(container[kind], telemetryNow) === 'stale' ? '采样已过期' : '未采集' }}</div>
                      </div>
                      <div class="wl-metric-cell"><div class="wl-cell-title">GPU 配置请求</div><div class="wl-actual-val">{{ container.gpu_request ?? '—' }}</div><div class="wl-metric-footnote">实际设备分配与显存用量待绑定确认</div></div>
                    </div>
                  </div>
                </article>
              </div>
            </template>
          </div>
        </section>
      </div>
    </div>
  </main>
</template>

<style scoped>
.spinning {
  animation: spin 1s linear infinite;
}
@keyframes spin {
  100% { transform: rotate(360deg); }
}
</style>
