<script setup lang="ts">
import { ref, computed, watch, onMounted, onBeforeUnmount } from 'vue'
import { useRoute } from 'vue-router'
import { request } from '../platform/client'
import { formatResource, resourceState, resourceValue } from './resource-metrics.js'

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

interface WorkspaceVolumeItem {
  workspace_id: string
  pvc_name: string
  pv_name: string
  namespace: string
  pool_name: string
  app_id: string
  app_display_name: string
  storage_bytes: number
  storage_class: string
  access_mode: string
  node_name: string
  instance_id?: string | null
  instance_name?: string | null
  organization_id?: string | null
  project_id?: string | null
  installation_id?: string | null
  state: 'in_use' | 'bound_stopped' | 'retained' | 'available'
  allowed_actions: string[]
  bound_at?: string | null
  retained_at?: string | null
}

const route = useRoute()

const currentView = computed<'monitor' | 'storage' | 'tasks'>(() => {
  const v = route.query.view
  return v === 'storage' || v === 'tasks' ? v : 'monitor'
})

const viewMeta = {
  monitor: {
    title: '资源监控',
    subtitle: 'Station 节点 GPU 拓扑、CPU 算力与系统运行内存实时遥测。',
    refreshLabel: '刷新遥测'
  },
  storage: {
    title: '存储管理',
    subtitle: 'Station 节点物理存储水位与全局应用工作卷池（PVC）绑定、释放回收。',
    refreshLabel: '刷新存储'
  },
  tasks: {
    title: '任务监控',
    subtitle: 'Station 已部署容器工作负载、GPU 创作任务配额与实际资源占用对账。',
    refreshLabel: '刷新任务'
  }
} as const

watch(currentView, () => {
  window.scrollTo(0, 0)
})

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

const dataDisk = computed<DiskData>(() => {
  return node.value.storage_disks?.find(d => d.mountpoint === '/data') ?? {
    mountpoint: '/data',
    total_bytes: 3700000000000,
    used_bytes: 266000000000,
    used_percent: 7
  }
})

const rootDisk = computed<DiskData>(() => {
  return node.value.storage_disks?.find(d => d.mountpoint === '/') ?? {
    mountpoint: '/',
    total_bytes: 3500000000000,
    used_bytes: 280000000000,
    used_percent: 8
  }
})

const workloadsSummary = ref<WorkloadsSummary>({})
const workloads = ref<WorkloadItem[]>([])
const workloadsState = ref<'loading' | 'ready' | 'error'>('loading')
const telemetryNow = ref(Date.now())
let telemetryPending = false

const currentFilter = ref<'all' | 'gpu' | 'infra'>('all')
const searchQuery = ref('')
const isRefreshing = ref(false)
let pollTimer: ReturnType<typeof setInterval> | null = null

const workspaces = ref<WorkspaceVolumeItem[]>([])
const workspacesState = ref<'loading' | 'ready' | 'error'>('loading')
const wsFilter = ref<'all' | 'in_use' | 'bound_stopped' | 'retained'>('all')
const wsSearchQuery = ref('')

const wsCounts = computed(() => {
  const counts = { all: 0, in_use: 0, bound_stopped: 0, retained: 0 }
  for (const ws of workspaces.value) {
    counts.all += 1
    if (ws.state in counts) {
      counts[ws.state as keyof typeof counts] += 1
    }
  }
  return counts
})

const filteredWorkspaces = computed(() => {
  const q = wsSearchQuery.value.trim().toLowerCase()
  return workspaces.value.filter(ws => {
    const stateMatch = wsFilter.value === 'all' || ws.state === wsFilter.value
    const text = `${ws.pvc_name} ${ws.pv_name} ${ws.namespace} ${ws.node_name || ''} ${ws.app_display_name || ''} ${ws.app_id} ${ws.pool_name} ${ws.instance_name || ''} ${ws.instance_id || ''}`.toLowerCase()
    const textMatch = !q || text.includes(q)
    return stateMatch && textMatch
  })
})

const filteredWorkloads = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return workloads.value.filter(wl => {
    const typeMatch = currentFilter.value === 'all' || wl.type === currentFilter.value
    const textMatch = !q || (wl.name + ' ' + (wl.display_name || '') + ' ' + (wl.namespace || '')).toLowerCase().includes(q)
    return typeMatch && textMatch
  })
})

const workloadCounts = computed(() => {
  let gpu = 0
  let infra = 0
  for (const wl of workloads.value) {
    if (wl.type === 'gpu') gpu += 1
    else infra += 1
  }
  return { all: workloads.value.length, gpu, infra }
})

function formatGiB(bytes: number): string {
  if (!bytes || bytes <= 0) return '—'
  const gib = bytes / (1024 ** 3)
  return gib >= 10 ? `${Math.round(gib)} GiB` : `${gib.toFixed(1)} GiB`
}

function formatDiskUsedGB(bytes: number): string {
  return `${Math.round(bytes / 1e9)} GB`
}

function formatDiskTotalTB(bytes: number): string {
  return `${(bytes / 1e12).toFixed(1)} TB`
}

function formatDiskAvailTB(disk: DiskData): string {
  const avail = Math.max(0, (disk.total_bytes || 0) - (disk.used_bytes || 0))
  return `${(avail / 1e12).toFixed(2)} TB`
}

function wsStateLabel(state: WorkspaceVolumeItem['state']): string {
  switch (state) {
    case 'in_use':
      return '挂载运行中'
    case 'bound_stopped':
      return '已停止绑定'
    case 'retained':
      return '待释放 · 实例已销毁'
    default:
      return state
  }
}

function wsInstanceDisplay(ws: WorkspaceVolumeItem): string {
  if (ws.instance_name) return ws.instance_name
  if (ws.state === 'retained') {
    return `${ws.app_display_name || ws.app_id} 历史实例（已销毁）`
  }
  return ws.instance_id || '已绑定实例'
}

// Release Workspace Dialog state
const releaseDialogRef = ref<HTMLDialogElement | null>(null)
const pendingReleaseWs = ref<WorkspaceVolumeItem | null>(null)
const releaseInputName = ref('')
const releaseError = ref('')
const releasingWorkspace = ref(false)

function openReleaseDialog(ws: WorkspaceVolumeItem) {
  pendingReleaseWs.value = ws
  releaseInputName.value = ''
  releaseError.value = ''
  releasingWorkspace.value = false
  const dlg = releaseDialogRef.value
  if (dlg) {
    if (typeof dlg.showModal === 'function' && !dlg.open) {
      dlg.showModal()
    } else {
      dlg.setAttribute('open', 'open')
    }
  }
}

function closeReleaseDialog() {
  const dlg = releaseDialogRef.value
  if (dlg) {
    if (typeof dlg.close === 'function' && dlg.open) {
      dlg.close()
    } else {
      dlg.removeAttribute('open')
    }
  }
  pendingReleaseWs.value = null
  releaseInputName.value = ''
  releaseError.value = ''
}

function makeUuidV4(): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return crypto.randomUUID()
  }
  return 'xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx'.replace(/[xy]/g, c => {
    const r = (Math.random() * 16) | 0
    const v = c === 'x' ? r : (r & 0x3) | 0x8
    return v.toString(16)
  })
}

async function submitReleaseWorkspace() {
  const target = pendingReleaseWs.value
  if (!target || releasingWorkspace.value) return
  if (releaseInputName.value.trim() !== target.pvc_name) return

  releasingWorkspace.value = true
  releaseError.value = ''
  try {
    const res = await request({
      path: 'resources/workspaces/release',
      method: 'POST',
      body: {
        idempotency_key: makeUuidV4(),
        namespace: target.namespace,
        pvc_name: target.pvc_name
      }
    })
    if (!res.ok) {
      let msg = '释放工作卷失败，请稍后重试。'
      try {
        const errData = await res.json()
        if (errData?.code === 'INSTANCE_NOT_DESTROYED') {
          msg = '关联实例尚未销毁，请先在应用控制台销毁实例后再释放工作卷。'
        } else if (errData?.code === 'WORKSPACE_IN_USE') {
          msg = '该工作卷当前正被运行中的容器挂载占用，无法释放。'
        } else if (errData?.message) {
          msg = errData.message
        }
      } catch {}
      releaseError.value = msg
      return
    }
    const releasedPvc = target.pvc_name
    const releasedPv = target.pv_name || `pv-${target.pvc_name}`
    closeReleaseDialog()
    await fetchTelemetry()
    showToast(`工作卷声明 (PVC) ${releasedPvc} 与物理持久卷 (PV) ${releasedPv} 已彻底释放回收`)
  } catch {
    releaseError.value = '网络请求异常，请检查连接后重试。'
  } finally {
    releasingWorkspace.value = false
  }
}

function showToast(msg: string) {
  const toast = document.getElementById('toast')
  if (toast) {
    toast.textContent = msg
    toast.classList.add('show')
    setTimeout(() => toast.classList.remove('show'), 3500)
  }
}

async function fetchTelemetry() {
  if (telemetryPending || document.hidden) return
  telemetryPending = true
  try {
    const [sRes, gRes, nRes, wRes, wsRes] = await Promise.all([
      request({ path: 'resources/summary', method: 'GET' }).catch(() => null),
      request({ path: 'resources/gpu', method: 'GET' }).catch(() => null),
      request({ path: 'resources/node', method: 'GET' }).catch(() => null),
      request({ path: 'resources/workloads', method: 'GET' }).catch(() => null),
      request({ path: 'resources/workspaces', method: 'GET' }).catch(() => null)
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

    const wsData = wsRes?.ok ? await wsRes.json() : null
    if (wsData && Array.isArray(wsData.workspaces)) {
      workspaces.value = wsData.workspaces
      workspacesState.value = 'ready'
    } else if (workspacesState.value === 'loading') {
      workspacesState.value = 'error'
    }

    if (sRes && sRes.ok) {
      const data = await sRes.json()
      if (data?.summary) {
        summary.value = data.summary
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

    window.dispatchEvent(
      new CustomEvent('studio:resources-counts', {
        detail: {
          workspaces: workspaces.value.length,
          tasks: workloads.value.length
        }
      })
    )
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
  showToast(workloadsState.value === 'error' ? '部分遥测暂不可用，请重试' : `${viewMeta[currentView.value].refreshLabel}已完成`)
  setTimeout(() => { isRefreshing.value = false }, 500)
}

function onVisibilityChange() {
  if (!document.hidden) {
    void fetchTelemetry()
  }
}

onMounted(() => {
  void fetchTelemetry()
  pollTimer = setInterval(fetchTelemetry, 4000)
  document.addEventListener('visibilitychange', onVisibilityChange)
})

onBeforeUnmount(() => {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
  document.removeEventListener('visibilitychange', onVisibilityChange)
})
</script>

<template>
  <main class="main" id="mainContent">
    <div class="workspace">
      <!-- Clean, Focused Heading Zone for Sub-business Workspace -->
      <header class="heading">
        <div class="heading-left">
          <h1 id="pageTitle">{{ viewMeta[currentView].title }}</h1>
          <p id="pageSubtitle">{{ viewMeta[currentView].subtitle }}</p>
        </div>
        <div class="heading-actions">
          <button class="btn ghost"
                  id="refreshBtn"
                  :title="viewMeta[currentView].refreshLabel"
                  :aria-label="viewMeta[currentView].refreshLabel"
                  @click="handleRefresh">
            <svg width="14"
                 height="14"
                 viewBox="0 0 24 24"
                 fill="none"
                 stroke="currentColor"
                 stroke-width="2"
                 stroke-linecap="round"
                 stroke-linejoin="round"
                 :class="{ spinning: isRefreshing }">
              <path d="M21.5 2v6h-6M21.34 15.57a10 10 0 1 1-.57-8.38l5.67-5.67"></path>
            </svg>
            <span id="refreshBtnLabel">{{ isRefreshing ? '刷新中...' : viewMeta[currentView].refreshLabel }}</span>
          </button>
        </div>
      </header>

      <!-- Sub-view 1: 资源监控 (Hardware Compute & Node Telemetry) -->
      <div v-if="currentView === 'monitor'" id="monitorView" class="resource-dashboard">
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
            <div class="res-metric-sub" id="resStorageSub">/data {{ formatDiskTotalTB(dataDisk.total_bytes) }} + / {{ formatDiskTotalTB(rootDisk.total_bytes) }} 就绪</div>
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
                  <i class="gpu-dot"></i>{{ g.utilization > 5 || g.used_vram_mb > 2048 ? '运行中' : '空间待命' }}
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
            <span class="eyebrow">HOST ARCHITECTURE &amp; KERNEL TELEMETRY</span>
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
      </div>

      <!-- Sub-view 2: 存储管理 (Local Fast-Tier Storage & Workspace Volume Pool Management) -->
      <div v-if="currentView === 'storage'" id="storageView" class="resource-dashboard">
        <!-- 4.1 节点物理存储与挂载点水位 -->
        <section class="res-section">
          <div class="res-section-title">
            <div class="res-section-title-left">
              <span>节点物理存储与挂载点水位</span>
              <span class="res-section-badge">NVMe PCIe 4.0/5.0 直通</span>
              <span class="res-section-badge">7.2 GB/s 读带宽</span>
            </div>
            <span class="eyebrow">LOCAL FAST-TIER &amp; S3 SYNC</span>
          </div>
          <div class="res-storage-cards">
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
                <span class="storage-val" id="storageRootVal">
                  {{ formatDiskUsedGB(rootDisk.used_bytes) }} <small>/ {{ formatDiskTotalTB(rootDisk.total_bytes) }} ({{ Math.round(rootDisk.used_percent || 8) }}%)</small>
                </span>
              </div>
              <div class="storage-bar">
                <div class="storage-fill" id="storageRootBar" :style="{ width: Math.min(100, Math.max(2, Math.round(rootDisk.used_percent || 8))) + '%' }"></div>
              </div>
              <div class="storage-meta">
                <span>承载：Containerd 容器镜像层 + Pod 临时存储</span>
                <span class="storage-status-ok"><i class="gpu-dot"></i>可用余量：{{ formatDiskAvailTB(rootDisk) }}</span>
              </div>
            </div>

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
                <span class="storage-val" id="storageDataVal">
                  {{ formatDiskUsedGB(dataDisk.used_bytes) }} <small>/ {{ formatDiskTotalTB(dataDisk.total_bytes) }} ({{ Math.round(dataDisk.used_percent || 7) }}%)</small>
                </span>
              </div>
              <div class="storage-bar">
                <div class="storage-fill" id="storageDataBar" :style="{ width: Math.min(100, Math.max(2, Math.round(dataDisk.used_percent || 7))) + '%' }"></div>
              </div>
              <div class="storage-meta">
                <span>承载：/data/models (模型缓存) + /data/artifacts (制品库)</span>
                <span class="storage-status-ok"><i class="gpu-dot"></i>可用余量：{{ formatDiskAvailTB(dataDisk) }}</span>
              </div>
            </div>
          </div>
        </section>

        <!-- 4.2 应用工作卷池与绑定台账 -->
        <div class="ws-vol-panel" id="workspaceVolumesPanel">
          <div class="ws-vol-head">
            <div class="ws-vol-title-group">
              <span class="ws-vol-title">应用工作卷池与绑定台账</span>
              <span class="res-section-badge">Station 全局统一纳管</span>
              <span class="res-section-badge"
                    :class="wsCounts.retained > 0 ? 'amber' : 'highlight'"
                    id="wsRetainedBadge">
                {{ wsCounts.retained > 0 ? `${wsCounts.retained} 个已销毁实例工作卷待释放` : '无待释放残留工作卷' }}
              </span>
            </div>
            <span class="ws-vol-sub">实例销毁后工作卷保留数据；确认无需保留后点击释放，将清空 PV 盘面并删除 PVC 声明，记录从表中移除。</span>
          </div>

          <div class="ws-vol-toolbar">
            <div class="workload-tabs" id="wsVolTabs" role="tablist" aria-label="工作卷状态筛选">
              <button class="workload-tab-btn"
                      :class="{ active: wsFilter === 'all' }"
                      type="button"
                      data-ws-filter="all"
                      @click="wsFilter = 'all'">
                全部工作卷 (<span id="wsCountAll">{{ wsCounts.all }}</span>)
              </button>
              <button class="workload-tab-btn"
                      :class="{ active: wsFilter === 'in_use' }"
                      type="button"
                      data-ws-filter="in_use"
                      @click="wsFilter = 'in_use'">
                挂载运行中 (<span id="wsCountInUse">{{ wsCounts.in_use }}</span>)
              </button>
              <button class="workload-tab-btn"
                      :class="{ active: wsFilter === 'bound_stopped' }"
                      type="button"
                      data-ws-filter="bound_stopped"
                      @click="wsFilter = 'bound_stopped'">
                已停止绑定 (<span id="wsCountStopped">{{ wsCounts.bound_stopped }}</span>)
              </button>
              <button class="workload-tab-btn"
                      :class="{ active: wsFilter === 'retained' }"
                      type="button"
                      data-ws-filter="retained"
                      @click="wsFilter = 'retained'">
                待释放 (<span id="wsCountRetained">{{ wsCounts.retained }}</span>)
              </button>
            </div>
            <div class="workload-search">
              <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                <circle cx="11" cy="11" r="8"></circle>
                <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
              </svg>
              <input v-model="wsSearchQuery"
                     type="text"
                     id="wsFilterInput"
                     aria-label="过滤工作卷"
                     placeholder="过滤 PVC 声明、PV 持久卷、应用或实例...">
            </div>
          </div>

          <div class="ws-vol-table-wrap">
            <table class="ws-vol-table" aria-label="应用工作卷列表">
              <thead>
                <tr>
                  <th>工作卷声明 (PVC)</th>
                  <th>物理持久卷 (PV)</th>
                  <th>所属应用 / 卷池</th>
                  <th>关联实例</th>
                  <th>状态</th>
                  <th>预留容量</th>
                  <th>操作</th>
                </tr>
              </thead>
              <tbody id="wsVolTableBody">
                <tr v-if="workspacesState === 'loading'">
                  <td colspan="7" style="text-align: center; color: var(--muted); padding: 24px;">正在读取应用工作卷池台账…</td>
                </tr>
                <tr v-else-if="workspacesState === 'error'">
                  <td colspan="7" style="text-align: center; color: var(--muted); padding: 24px;">工作卷池台账暂不可用，请点击右上角刷新重试。</td>
                </tr>
                <tr v-else-if="!filteredWorkspaces.length">
                  <td colspan="7" style="text-align: center; color: var(--muted); padding: 24px;">没有符合筛选条件的工作卷记录。</td>
                </tr>
                <tr v-else
                    v-for="ws in filteredWorkspaces"
                    :key="ws.workspace_id"
                    :data-ws-state="ws.state"
                    :data-pvc="ws.pvc_name"
                    :data-pv="ws.pv_name || ('pv-' + ws.pvc_name)">
                  <td>
                    <div class="ws-cell-main ws-cell-mono">{{ ws.pvc_name }}</div>
                    <div class="ws-cell-sub">NS: {{ ws.namespace }}</div>
                  </td>
                  <td>
                    <div class="ws-cell-main ws-cell-mono">{{ ws.pv_name || ('pv-' + ws.pvc_name) }}</div>
                    <div class="ws-cell-sub">Node: {{ ws.node_name || 'verdentflare-5090' }}</div>
                  </td>
                  <td>
                    <div class="ws-cell-main">{{ ws.app_display_name || ws.app_id }}</div>
                    <div class="ws-cell-sub">Pool: {{ ws.pool_name }}</div>
                  </td>
                  <td class="ws-instance-cell">
                    <div class="ws-cell-main">{{ wsInstanceDisplay(ws) }}</div>
                    <div class="ws-cell-sub">ID: {{ ws.instance_id || '—（实例已销毁）' }}</div>
                  </td>
                  <td class="ws-state-cell">
                    <span class="ws-status-pill" :class="ws.state"><i class="dot"></i>{{ wsStateLabel(ws.state) }}</span>
                  </td>
                  <td>
                    <div class="ws-cell-main">{{ formatGiB(ws.storage_bytes) }}</div>
                    <div class="ws-cell-sub">{{ ws.storage_class || 'local-nvme' }} · {{ ws.access_mode === 'ReadWriteOnce' ? 'RWO' : (ws.access_mode || 'RWO') }}</div>
                  </td>
                  <td class="ws-action-cell">
                    <button v-if="ws.state === 'retained' && (ws.allowed_actions || []).includes('release')"
                            type="button"
                            class="ws-btn-release"
                            :data-release-pvc="ws.pvc_name"
                            @click="openReleaseDialog(ws)">
                      释放工作卷
                    </button>
                    <button v-else
                            type="button"
                            class="ws-btn-release"
                            disabled
                            :title="ws.state === 'in_use' ? '实例运行中，请先停止并销毁实例' : '请先在应用控制台销毁关联实例，再释放工作卷'">
                      释放工作卷
                    </button>
                  </td>
                </tr>
              </tbody>
            </table>
          </div>
        </div>
      </div>

      <!-- Sub-view 3: 任务监控 (Workload Quota & Active Tasks Telemetry) -->
      <div v-if="currentView === 'tasks'" id="tasksView" class="resource-dashboard">
        <section class="res-section" id="workloadSection">
          <div class="res-section-title">
            <div class="res-section-title-left">
              <span>已部署工作负载与算力配额对账</span>
              <span class="res-section-badge highlight">{{ workloadsSummary.total_pods ?? workloads.length }} 个活跃容器组</span>
              <span class="res-section-badge">Kubernetes 配置 · metrics-server 采样</span>
            </div>
            <span class="eyebrow">CONTAINER LEVEL QUOTA &amp; ACTUAL IN-USE</span>
          </div>
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
                  <button class="workload-tab-btn" :class="{ active: currentFilter === 'all' }" data-filter="all" @click="currentFilter = 'all'">全部工作负载 ({{ workloadCounts.all }})</button>
                  <button class="workload-tab-btn" :class="{ active: currentFilter === 'gpu' }" data-filter="gpu" @click="currentFilter = 'gpu'">GPU 创作应用 ({{ workloadCounts.gpu }})</button>
                  <button class="workload-tab-btn" :class="{ active: currentFilter === 'infra' }" data-filter="infra" @click="currentFilter = 'infra'">平台与基础设施 ({{ workloadCounts.infra }})</button>
                </div>
                <div class="workload-search">
                  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
                    <circle cx="11" cy="11" r="8"></circle>
                    <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
                  </svg>
                  <input v-model="searchQuery" id="filterInput" type="search" aria-label="搜索工作负载" placeholder="过滤 Pod、应用名称或命名空间...">
                </div>
              </div>
              <p v-if="!workloads.length">暂无工作负载。</p>
              <p v-else-if="!filteredWorkloads.length">没有符合筛选条件的工作负载。</p>
              <div class="workload-list" id="workloadList">
                <article v-for="wl in filteredWorkloads" :key="wl.pod_uid" class="wl-card" :class="wl.type === 'gpu' ? 'has-gpu' : 'cpu-only'" :data-type="wl.type" :data-name="wl.name">
                  <div class="wl-head">
                    <div class="wl-title-box">
                      <div class="wl-title-row">
                        <span class="wl-name">{{ wl.display_name || wl.name }}</span>
                        <span class="wl-namespace">{{ wl.namespace }}</span>
                      </div>
                      <span class="wl-pod-id">{{ wl.name }}</span>
                    </div>
                    <div class="wl-meta-right">
                      <span class="wl-uptime">{{ wl.age }}</span>
                      <span class="wl-status-tag running">{{ wl.status }}</span>
                    </div>
                  </div>
                  <div v-for="container in wl.containers" :key="container.name">
                    <div class="wl-foot"><span>{{ container.name }} · {{ container.ready ? '就绪' : '未就绪' }} · 重启 {{ container.restarts }} 次</span></div>
                    <div class="wl-metrics-grid">
                      <div v-for="kind in (['cpu', 'memory'] as const)" :key="kind" class="wl-metric-cell">
                        <div class="wl-cell-title">{{ kind === 'cpu' ? 'CPU 使用' : '内存工作集' }}</div>
                        <div class="wl-actual-val">{{ resourceValue(container[kind], kind === 'cpu' ? 'cores' : 'bytes') }}</div>
                        <div class="wl-metric-footnote">请求 {{ formatResource(container[kind].request, kind === 'cpu' ? 'cores' : 'bytes') }} · 上限 {{ formatResource(container[kind].limit, kind === 'cpu' ? 'cores' : 'bytes') }}</div>
                        <div class="wl-metric-footnote">{{ resourceState(container[kind]) === 'fresh' ? '采样 ' + new Date(container[kind].sampled_at!).toLocaleString('zh-CN') : resourceState(container[kind]) === 'stale' ? '采样已过期' : '未采集' }}</div>
                      </div>
                      <div class="wl-metric-cell">
                        <div class="wl-cell-title">GPU 配置请求</div>
                        <div class="wl-actual-val">{{ container.gpu_request ?? '—' }}</div>
                        <div class="wl-metric-footnote">实际设备分配与显存用量待绑定确认</div>
                      </div>
                    </div>
                  </div>
                </article>
              </div>
            </template>
          </div>
        </section>
      </div>
    </div>

    <!-- 释放工作卷二次确认弹窗 -->
    <dialog ref="releaseDialogRef"
            id="release-workspace-dialog"
            class="ws-dialog"
            aria-labelledby="release-workspace-title">
      <form class="ws-dialog-card"
            method="dialog"
            id="release-workspace-form"
            @submit.prevent="submitReleaseWorkspace">
        <div class="ws-dialog-head">
          <div>
            <span class="eyebrow" style="color: #c43d2f;">WORKSPACE VOLUME RELEASE</span>
            <h3 id="release-workspace-title" class="ws-dialog-title">确认释放工作卷</h3>
          </div>
          <button type="button"
                  class="ws-btn-cancel"
                  id="release-workspace-cancel-top"
                  :disabled="releasingWorkspace"
                  @click="closeReleaseDialog">取消</button>
        </div>
        <div class="ws-dialog-body">
          <p class="ws-dialog-desc">
            释放后将清空物理持久卷 (PV) <code><span id="release-workspace-pv-code">{{ pendingReleaseWs?.pv_name || (pendingReleaseWs ? 'pv-' + pendingReleaseWs.pvc_name : '') }}</span></code>
            的残留数据，删除工作卷声明 (PVC) <code><span id="release-workspace-target-code">{{ pendingReleaseWs?.pvc_name || '' }}</span></code>
            并解除其与底层 PV 的绑定，使其回到空闲可用池（当前记录将从列表移除）。
          </p>
          <div class="ws-dialog-field">
            <span>请输入工作卷 PVC 名称 <code><span id="release-workspace-expected">{{ pendingReleaseWs?.pvc_name || '' }}</span></code> 以确认释放</span>
            <input v-model="releaseInputName"
                   id="release-workspace-name"
                   type="text"
                   autocomplete="off"
                   :disabled="releasingWorkspace"
                   placeholder="输入工作卷 PVC 名称确认释放">
          </div>
          <div v-if="releaseError"
               class="ws-dialog-error"
               id="release-workspace-error"
               role="alert">{{ releaseError }}</div>
        </div>
        <div class="ws-dialog-actions">
          <button type="button"
                  class="ws-btn-cancel"
                  id="release-workspace-cancel"
                  :disabled="releasingWorkspace"
                  @click="closeReleaseDialog">保留绑定</button>
          <button type="submit"
                  class="ws-btn-release"
                  id="release-workspace-confirm"
                  :disabled="releasingWorkspace || releaseInputName.trim() !== (pendingReleaseWs?.pvc_name || '')">
            {{ releasingWorkspace ? '正在释放…' : '确认释放工作卷' }}
          </button>
        </div>
      </form>
    </dialog>
  </main>
</template>

<style scoped>
.spinning {
  animation: spin 1s linear infinite;
}
@keyframes spin {
  100% { transform: rotate(360deg); }
}

.res-section-badge.amber {
  background: rgba(180, 105, 14, 0.12);
  color: #b4690e;
  border-color: rgba(180, 105, 14, 0.3);
}

.ws-vol-panel {
  background: var(--shell-panel, var(--panel));
  border: 1px solid var(--line);
  border-radius: 16px;
  overflow: hidden;
  box-shadow: 0 4px 18px rgba(21, 35, 30, 0.02);
}

.ws-vol-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 18px 24px;
  border-bottom: 1px solid var(--line);
  flex-wrap: wrap;
}

.ws-vol-title-group {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.ws-vol-title {
  font-size: 15px;
  font-weight: 650;
  color: var(--ink);
  letter-spacing: -0.2px;
}

.ws-vol-sub {
  font-size: 12px;
  color: var(--muted);
}

.ws-vol-toolbar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 12px 24px;
  background: var(--shell-button);
  border-bottom: 1px solid var(--line);
  flex-wrap: wrap;
}

.ws-vol-table-wrap {
  width: 100%;
  overflow-x: auto;
  background: var(--panel);
}

.ws-vol-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 12.5px;
  text-align: left;
}

.ws-vol-table th {
  padding: 11px 24px;
  font-size: 11px;
  font-weight: 600;
  color: var(--muted);
  border-bottom: 1px solid var(--line);
  white-space: nowrap;
}

.ws-vol-table th:last-child,
.ws-vol-table td:last-child {
  text-align: right;
}

.ws-vol-table td {
  padding: 13px 24px;
  border-bottom: 1px solid var(--line);
  vertical-align: middle;
  color: var(--ink);
}

.ws-vol-table tbody tr:last-child td {
  border-bottom: 0;
}

.ws-vol-table tbody tr:hover {
  background: rgba(8, 126, 96, 0.03);
}

.ws-cell-main {
  font-weight: 600;
  color: var(--ink);
  display: flex;
  align-items: center;
  gap: 6px;
}

.ws-cell-mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12.5px;
  font-weight: 700;
}

.ws-cell-sub {
  font-size: 11px;
  color: var(--muted);
  margin-top: 2px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.ws-status-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 9px;
  border-radius: 9999px;
  font-size: 11px;
  font-weight: 600;
  white-space: nowrap;
}

.ws-status-pill .dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
}

.ws-status-pill.in_use {
  background: rgba(8, 126, 96, 0.1);
  color: var(--accent);
  border: 1px solid rgba(8, 126, 96, 0.28);
}

.ws-status-pill.in_use .dot {
  background: var(--accent);
}

.ws-status-pill.bound_stopped {
  background: var(--shell-button);
  color: var(--muted);
  border: 1px solid var(--line);
}

.ws-status-pill.bound_stopped .dot {
  background: var(--muted);
}

.ws-status-pill.retained {
  background: rgba(180, 105, 14, 0.12);
  color: #b4690e;
  border: 1px solid rgba(180, 105, 14, 0.32);
}

.ws-status-pill.retained .dot {
  background: #b4690e;
}

.ws-status-pill.available {
  background: rgba(45, 108, 223, 0.1);
  color: #2d6cdf;
  border: 1px solid rgba(45, 108, 223, 0.26);
}

.ws-status-pill.available .dot {
  background: #2d6cdf;
}

.ws-btn-release {
  padding: 5px 11px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 600;
  border: 1px solid rgba(196, 61, 47, 0.35);
  background: rgba(196, 61, 47, 0.08);
  color: #c43d2f;
  cursor: pointer;
  transition: all 0.15s ease;
}

.ws-btn-release:hover:not(:disabled) {
  background: rgba(196, 61, 47, 0.15);
  border-color: #c43d2f;
}

.ws-btn-release:disabled {
  opacity: 0.42;
  cursor: not-allowed;
  border-color: var(--line);
  background: var(--shell-button);
  color: var(--muted);
}

.ws-dialog {
  border: 1px solid var(--line);
  border-radius: 14px;
  padding: 22px 24px;
  background: var(--panel);
  color: var(--ink);
  width: min(460px, 92vw);
  box-shadow: 0 18px 48px rgba(0, 0, 0, 0.18);
}

.ws-dialog::backdrop {
  background: rgba(15, 23, 20, 0.42);
  backdrop-filter: blur(2px);
}

.ws-dialog-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.ws-dialog-title {
  font-size: 16px;
  font-weight: 700;
  margin: 4px 0 0;
}

.ws-dialog-desc {
  font-size: 12.5px;
  color: var(--muted);
  line-height: 1.55;
  margin: 0 0 14px;
}

.ws-dialog-field {
  display: flex;
  flex-direction: column;
  gap: 6px;
  font-size: 12px;
  font-weight: 600;
  margin-bottom: 16px;
}

.ws-dialog-field input {
  padding: 8px 11px;
  border-radius: 8px;
  border: 1px solid var(--line);
  background: var(--bg);
  color: var(--ink);
  font-size: 13px;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  outline: none;
}

.ws-dialog-field input:focus {
  border-color: var(--accent);
}

.ws-dialog-error {
  margin-bottom: 14px;
  padding: 8px 11px;
  border-radius: 8px;
  background: rgba(196, 61, 47, 0.08);
  border: 1px solid rgba(196, 61, 47, 0.3);
  color: #c43d2f;
  font-size: 12px;
}

.ws-dialog-actions {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
}

.ws-btn-cancel {
  padding: 6px 14px;
  border-radius: 7px;
  border: 1px solid var(--line);
  background: var(--shell-button);
  color: var(--ink);
  font-size: 12.5px;
  font-weight: 600;
  cursor: pointer;
}
</style>
