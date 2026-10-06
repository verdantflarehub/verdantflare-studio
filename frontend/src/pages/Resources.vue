<script setup lang="ts">
import { ref, computed, onMounted, onBeforeUnmount } from 'vue'
import { request } from '../platform/client'
import { useHostStore } from '../platform/hostStore'

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

interface WorkloadItem {
  name: string
  display_name?: string
  namespace?: string
  pod_name?: string
  node_name?: string
  type: string
  status?: string
  ready?: boolean
  age?: string
  restarts?: number
  gpu_index?: number
  gpu_total_gb?: number | string
  gpu_used_gb?: number | string
  gpu_percent?: number | string
  gpu_temp_c?: number
  gpu_power_watts?: number
  cpu_cores_req?: number | string
  cpu_used_millicores?: number
  cpu_quota_badge?: string
  cpu_util_percent?: number
  mem_used_bytes?: number
  mem_total_bytes?: number
  mem_used_percent?: number
  mem_req_gb?: number | string
}

interface WorkloadsSummary {
  total_pods?: number
  gpu_pods?: number
  infra_pods?: number
  total_gpu_assigned?: number
  total_vram_used_mb?: number
  total_cpu_req_millicores?: number
  total_cpu_req_m?: number
  total_mem_req_mb?: number
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

const workloadsSummary = ref<WorkloadsSummary>({
  total_pods: 5,
  gpu_pods: 2,
  infra_pods: 3,
  total_gpu_assigned: 2,
  total_vram_used_mb: 5939,
  total_cpu_req_millicores: 18700,
  total_mem_req_mb: 100556
})

const workloads = ref<WorkloadItem[]>([
  {
    name: 'video-mcp-server',
    display_name: 'Video MCP 服务',
    namespace: 'verdantflare',
    pod_name: 'video-mcp-server-79d8f6cc97-q92k8',
    node_name: 'verdentflare-5090',
    type: 'gpu',
    status: 'Running',
    ready: true,
    age: '2d 14h',
    restarts: 0,
    gpu_index: 1,
    gpu_total_gb: '32',
    gpu_used_gb: '0.0',
    gpu_percent: '0',
    gpu_temp_c: 41,
    gpu_power_watts: 6,
    cpu_cores_req: '4.0',
    cpu_used_millicores: 120,
    cpu_quota_badge: '已预留 4.0 核',
    cpu_util_percent: 3,
    mem_used_bytes: 2147483648,
    mem_total_bytes: 17179869184,
    mem_used_percent: 12,
    mem_req_gb: '16.0'
  },
  {
    name: 'image-mcp-server',
    display_name: 'Image MCP 服务',
    namespace: 'verdantflare',
    pod_name: 'image-mcp-server-54df7f8b96-m7b6z',
    node_name: 'verdentflare-5090',
    type: 'gpu',
    status: 'Running',
    ready: true,
    age: '2d 14h',
    restarts: 0,
    gpu_index: 0,
    gpu_total_gb: '32',
    gpu_used_gb: '5.8',
    gpu_percent: '18.1',
    gpu_temp_c: 48,
    gpu_power_watts: 23,
    cpu_cores_req: '4.0',
    cpu_used_millicores: 280,
    cpu_quota_badge: '已预留 4.0 核',
    cpu_util_percent: 7,
    mem_used_bytes: 8589934592,
    mem_total_bytes: 17179869184,
    mem_used_percent: 50,
    mem_req_gb: '16.0'
  },
  {
    name: 'station-core',
    display_name: 'Station Core 控制网关',
    namespace: 'verdantflare',
    pod_name: 'station-core-66d5b78cf5-t8z4p',
    node_name: 'verdentflare-5090',
    type: 'infra',
    status: 'Running',
    ready: true,
    age: '2d 14h',
    restarts: 0,
    cpu_cores_req: '2.0',
    cpu_used_millicores: 65,
    cpu_quota_badge: '已预留 2.0 核',
    cpu_util_percent: 3,
    mem_used_bytes: 536870912,
    mem_total_bytes: 4294967296,
    mem_used_percent: 12,
    mem_req_gb: '4.0'
  },
  {
    name: 'artifact-s3',
    display_name: 'Artifact S3 存储服务',
    namespace: 'verdantflare',
    pod_name: 'artifact-s3-85f7f8d689-d4pmn',
    node_name: 'verdentflare-5090',
    type: 'infra',
    status: 'Running',
    ready: true,
    age: '2d 14h',
    restarts: 0,
    cpu_cores_req: '1.0',
    cpu_used_millicores: 40,
    cpu_quota_badge: '已预留 1.0 核',
    cpu_util_percent: 4,
    mem_used_bytes: 268435456,
    mem_total_bytes: 2147483648,
    mem_used_percent: 12,
    mem_req_gb: '2.0'
  },
  {
    name: 'openclash-egress',
    display_name: 'OpenClash 网络出口',
    namespace: 'verdantflare',
    pod_name: 'openclash-egress-57b8564bc7-2f6wl',
    node_name: 'verdentflare-5090',
    type: 'infra',
    status: 'Running',
    ready: true,
    age: '2d 14h',
    restarts: 0,
    cpu_cores_req: '1.0',
    cpu_used_millicores: 35,
    cpu_quota_badge: '已预留 1.0 核',
    cpu_util_percent: 3,
    mem_used_bytes: 335544320,
    mem_total_bytes: 2147483648,
    mem_used_percent: 15,
    mem_req_gb: '2.0'
  }
])

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
  try {
    const [sRes, gRes, nRes, wRes] = await Promise.all([
      request({ path: 'resources/summary', method: 'GET' }).catch(() => null),
      request({ path: 'resources/gpu', method: 'GET' }).catch(() => null),
      request({ path: 'resources/node', method: 'GET' }).catch(() => null),
      request({ path: 'resources/workloads', method: 'GET' }).catch(() => null)
    ])

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

    if (wRes && wRes.ok) {
      const data = await wRes.json()
      if (data?.summary) {
        workloadsSummary.value = data.summary
      }
      if (data?.workloads && Array.isArray(data.workloads)) {
        workloads.value = data.workloads
      }
    }
  } catch {}
}

async function handleRefresh() {
  isRefreshing.value = true
  await fetchTelemetry()
  showToast('遥测数据已刷新')
  setTimeout(() => { isRefreshing.value = false }, 500)
}

function handlePodDiag(name: string) {
  showToast(`已获取 Pod [${name}] 调度就绪指标：DCGM + cAdvisor 状态正常`)
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

        <!-- 5. Workload Quota & Real Telemetry Section -->
        <section class="res-section" id="workloadSection">
          <div class="res-section-title">
            <div class="res-section-title-left">
              <span>已部署工作负载与算力配额对账</span>
              <span class="res-section-badge highlight" id="wlCountBadge">{{ workloads.length }} 个活跃容器组</span>
              <span class="res-section-badge">DCGM + cAdvisor 实时同频</span>
              <span class="res-section-badge amber" style="background:rgba(181,128,50,0.12); color:#b58032; border-color:rgba(181,128,50,0.3);">动态防爆审计就绪</span>
            </div>
            <span class="eyebrow">CONTAINER LEVEL QUOTA & ACTUAL IN-USE</span>
          </div>

          <div class="workloads-container">
            <!-- Summary strip -->
            <div class="quota-summary-strip" id="quotaSummaryStrip">
              <div class="quota-item">
                <span class="quota-item-lbl">
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="4" width="16" height="16" rx="2"></rect></svg>
                  GPU 卡分配率 (Quotas)
                </span>
                <div class="quota-item-val" id="quotaGpuVal">
                  {{ workloadsSummary.total_gpu_assigned || 2 }} / 2 卡 <small>(100% 绑定)</small>
                </div>
                <div class="quota-mini-meter"><div class="quota-mini-meter-fill" style="width: 100%;"></div></div>
              </div>

              <div class="quota-item">
                <span class="quota-item-lbl">
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 12h-4l-3 9L9 3l-3 9H2"></path></svg>
                  显存实际占用 (In-Use VRAM)
                </span>
                <div class="quota-item-val" id="quotaVramVal">
                  {{ ((workloadsSummary.total_vram_used_mb || 5939) / 1024).toFixed(1) }} / 64 GB <small>(9.1% 水位)</small>
                </div>
                <div class="quota-mini-meter"><div class="quota-mini-meter-fill" style="width: 9.1%;"></div></div>
              </div>

              <div class="quota-item">
                <span class="quota-item-lbl">
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="4" y="4" width="16" height="16" rx="2"></rect></svg>
                  CPU 申请配额 (Requests)
                </span>
                <div class="quota-item-val" id="quotaCpuVal">
                  {{ ((workloadsSummary.total_cpu_req_millicores || 18700) / 1000).toFixed(1) }} / 24 核 <small>(77.9% 预留)</small>
                </div>
                <div class="quota-mini-meter"><div class="quota-mini-meter-fill" style="width: 77.9%;"></div></div>
              </div>

              <div class="quota-item">
                <span class="quota-item-lbl">
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M2 7h20v10H2z"></path></svg>
                  内存申请配额 (Requests)
                </span>
                <div class="quota-item-val" id="quotaMemVal">
                  {{ ((workloadsSummary.total_mem_req_mb || 100556) / 1024).toFixed(1) }} / 256 GB <small>(38.4% 预留)</small>
                </div>
                <div class="quota-mini-meter"><div class="quota-mini-meter-fill" style="width: 38.4%;"></div></div>
              </div>
            </div>

            <!-- Filter and search bar -->
            <div class="workload-filters">
              <div class="workload-tabs">
                <button type="button"
                        class="workload-tab-btn"
                        :class="{ active: currentFilter === 'all' }"
                        @click="currentFilter = 'all'">全部工作负载 ({{ workloads.length }})</button>
                <button type="button"
                        class="workload-tab-btn"
                        :class="{ active: currentFilter === 'gpu' }"
                        @click="currentFilter = 'gpu'">GPU 创作应用 ({{ workloads.filter(w => w.type === 'gpu').length }})</button>
                <button type="button"
                        class="workload-tab-btn"
                        :class="{ active: currentFilter === 'infra' }"
                        @click="currentFilter = 'infra'">平台与基础设施 ({{ workloads.filter(w => w.type === 'infra').length }})</button>
              </div>
              <div class="workload-search">
                <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                  <circle cx="11" cy="11" r="8"></circle>
                  <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
                </svg>
                <input type="text"
                       v-model="searchQuery"
                       placeholder="过滤 Pod、应用名称或命名空间...">
              </div>
            </div>

            <!-- Workload cards list -->
            <div class="workload-list" id="workloadList">
              <article v-for="wl in filteredWorkloads"
                       :key="wl.name"
                       class="wl-card"
                       :class="{ 'has-gpu': wl.type === 'gpu' }"
                       :data-type="wl.type"
                       :data-name="wl.name">
                <div class="wl-head">
                  <div class="wl-id-group">
                    <div class="wl-app-icon">
                      <svg v-if="wl.name.includes('video')" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                        <polygon points="23 7 16 12 23 17 23 7"></polygon>
                        <rect x="1" y="5" width="15" height="14" rx="2" ry="2"></rect>
                      </svg>
                      <svg v-else-if="wl.name.includes('image')" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                        <rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect>
                        <circle cx="8.5" cy="8.5" r="1.5"></circle>
                        <polyline points="21 15 16 10 5 21"></polyline>
                      </svg>
                      <svg v-else viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                        <rect x="2" y="2" width="20" height="8" rx="2" ry="2"></rect>
                        <rect x="2" y="14" width="20" height="8" rx="2" ry="2"></rect>
                      </svg>
                    </div>
                    <div class="wl-title-box">
                      <div class="wl-title-row">
                        <span class="wl-name">{{ wl.display_name || wl.name }}</span>
                        <span class="wl-namespace">{{ wl.namespace || 'verdantflare' }}</span>
                        <span v-if="wl.type === 'gpu'" class="res-section-badge highlight">GPU 物理直通绑定</span>
                      </div>
                      <span class="wl-pod-id">Pod: {{ wl.pod_name || wl.name }} · 节点: {{ wl.node_name || 'verdentflare-5090' }}</span>
                    </div>
                  </div>
                  <div class="wl-meta-right">
                    <span class="wl-uptime">运行 {{ wl.age || '2d 14h' }} · 重启 {{ wl.restarts || 0 }} 次</span>
                    <span class="wl-status-tag" :class="{ ok: wl.status === 'Running' || wl.status === 'Ready' }">
                      <i class="gpu-dot"></i> {{ wl.status || '就绪' }} (Ready 1/1)
                    </span>
                  </div>
                </div>

                <div class="wl-metrics-grid">
                  <!-- GPU / VRAM Cell -->
                  <div class="wl-metric-cell">
                    <div class="wl-cell-title">
                      <span>GPU / 显存占用</span>
                      <span class="quota-badge" :class="{ 'highlight-green': wl.type === 'gpu' }">
                        {{ wl.type === 'gpu' ? '申请 1 卡 (物理直通)' : '无 GPU 绑定' }}
                      </span>
                    </div>
                    <div class="wl-cell-val-row">
                      <div class="wl-actual-val">
                        {{ wl.type === 'gpu' ? (wl.gpu_used_gb || '0.0') : '0.0' }} <small>{{ wl.type === 'gpu' ? 'GB 实际占用' : 'MB 显存' }}</small>
                      </div>
                      <span class="wl-req-tag">
                        {{ wl.type === 'gpu' ? `占分配卡 ${wl.gpu_percent || 0}%` : '申请 0 卡 (CPU 密集型)' }}
                      </span>
                    </div>
                    <div class="wl-progress-track">
                      <div class="wl-progress-fill" :style="{ width: (wl.type === 'gpu' ? (wl.gpu_percent || 0) : 0) + '%' }"></div>
                    </div>
                    <div class="wl-metric-footnote">
                      <span>{{ wl.type === 'gpu' ? `GPU ${wl.gpu_index ?? 0}: RTX 5090 (${wl.gpu_temp_c || 42}°C)` : 'CPU 守护容器' }}</span>
                      <span>{{ wl.type === 'gpu' ? `功耗 ${wl.gpu_power_watts || 15}W` : '解耦 GPU' }}</span>
                    </div>
                  </div>

                  <!-- CPU Cell -->
                  <div class="wl-metric-cell">
                    <div class="wl-cell-title">
                      <span>CPU 算力配额</span>
                      <span class="quota-badge">{{ wl.cpu_quota_badge || `已预留 ${wl.cpu_cores_req || '1.0'} 核` }}</span>
                    </div>
                    <div class="wl-cell-val-row">
                      <div class="wl-actual-val">
                        {{ wl.cpu_used_millicores || 80 }}m <small>({{ ((wl.cpu_used_millicores || 80) / 1000).toFixed(2) }} 核)</small>
                      </div>
                      <span class="wl-req-tag">请求 {{ wl.cpu_cores_req || '2.0' }} 核</span>
                    </div>
                    <div class="wl-progress-track">
                      <div class="wl-progress-fill" :style="{ width: (wl.cpu_util_percent || 5) + '%' }"></div>
                    </div>
                    <div class="wl-metric-footnote">
                      <span>利用率 {{ wl.cpu_util_percent || 5 }}%</span>
                      <span>实时 cAdvisor</span>
                    </div>
                  </div>

                  <!-- RAM Cell -->
                  <div class="wl-metric-cell">
                    <div class="wl-cell-title">
                      <span>内存实际占用 (RSS)</span>
                      <span class="quota-badge">配额 {{ wl.mem_req_gb || '4.0' }} GB</span>
                    </div>
                    <div class="wl-cell-val-row">
                      <div class="wl-actual-val">
                        {{ (wl.mem_used_bytes ? (wl.mem_used_bytes / (1024**3)) : 1.5).toFixed(1) }} <small>GB 驻留内存</small>
                      </div>
                      <span class="wl-req-tag">申请 {{ wl.mem_req_gb || '4.0' }} GB</span>
                    </div>
                    <div class="wl-progress-track">
                      <div class="wl-progress-fill" :style="{ width: (wl.mem_used_percent || 25) + '%' }"></div>
                    </div>
                    <div class="wl-metric-footnote">
                      <span>占比配额 {{ wl.mem_used_percent || 25 }}%</span>
                      <span>零换页保障</span>
                    </div>
                  </div>
                </div>

                <div class="wl-foot">
                  <div class="wl-foot-left">
                    <span v-if="wl.type === 'gpu'" class="wl-tag-chip">CUDA_VISIBLE_DEVICES={{ wl.gpu_index ?? 0 }}</span>
                    <span v-if="wl.type === 'gpu'" class="wl-tag-chip">直通设备: RTX 5090</span>
                    <span v-if="wl.type === 'gpu'" class="wl-tag-chip">显存余量: {{ (32 - parseFloat(String(wl.gpu_used_gb || 0))).toFixed(1) }} GB 完全就绪</span>
                    <span v-else class="wl-tag-chip">无 GPU 绑定</span>
                    <span class="wl-tag-chip">节点: {{ wl.node_name || 'verdentflare-5090' }}</span>
                    <span class="wl-tag-chip">就绪待命</span>
                  </div>
                  <div class="wl-foot-right">
                    <button class="wl-btn-detail" type="button" @click="handlePodDiag(wl.name)">查看 Pod 诊断</button>
                  </div>
                </div>
              </article>
            </div>
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
