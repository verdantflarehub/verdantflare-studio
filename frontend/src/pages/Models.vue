<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'

interface ModelItem {
  id: string
  name: string
  version: string
  category: 'video' | 'image' | 'music' | 'llm'
  categoryLabel: string
  arch: string
  params: string
  vramReqGb: number
  sizeGb: number
  status: 'ready' | 'cached' | 'cloud' | 'pending'
  statusLabel: string
  path: string
  quant: string
  boundApp: string
  description: string
}

const themeText = ref('深色主题')
const isRefreshing = ref(false)
const currentTab = ref<'all' | 'ready' | 'cloud' | 'pending'>('all')
const currentCategory = ref<'all' | 'video' | 'image' | 'music' | 'llm'>('all')
const searchQuery = ref('')
const selectedModel = ref<ModelItem | null>(null)

const models = ref<ModelItem[]>([
  {
    id: 'wan2.1-t2v-14b',
    name: 'Wan 2.1 T2V 14B',
    version: 'v2.1-fp16',
    category: 'video',
    categoryLabel: '视频生成',
    arch: 'DiT / Flow-Matching',
    params: '14B',
    vramReqGb: 24,
    sizeGb: 28.5,
    status: 'ready',
    statusLabel: '已预热就绪',
    path: '/data/models/wan2.1',
    quant: 'BF16 原生',
    boundApp: 'Video MCP Server',
    description: '阿里通义开源先进视频生成基模，支持文本到高质量高分辨率视频生成与首尾帧控制。'
  },
  {
    id: 'flux.1-dev',
    name: 'FLUX.1 [dev]',
    version: 'v1.0-bf16',
    category: 'image',
    categoryLabel: '图像生成',
    arch: 'Rectified Flow Transformer',
    params: '12B',
    vramReqGb: 16,
    sizeGb: 23.8,
    status: 'ready',
    statusLabel: '已预热就绪',
    path: '/data/models/flux',
    quant: 'BF16 原生',
    boundApp: 'Image MCP Server',
    description: 'Black Forest Labs 出品的新一代开源顶尖文生图模型，具备卓越的构图、文字渲染与光影细节。'
  },
  {
    id: 'minimax-hailuo-h3',
    name: 'MiniMax Hailuo H3',
    version: 'API-Cloud',
    category: 'video',
    categoryLabel: '视频生成',
    arch: 'Cloud Transformer Engine',
    params: 'Cloud Native',
    vramReqGb: 0,
    sizeGb: 0,
    status: 'cloud',
    statusLabel: '云端直连',
    path: 'https://api.minimax.chat',
    quant: 'FP16 云端推理',
    boundApp: 'Video MCP Server (API Channel)',
    description: 'MiniMax 海螺高保真物理世界视频生成模型，通过 Station 出口代理直连，零本地显存开销。'
  },
  {
    id: 'ltx-video-2b',
    name: 'LTX-Video 2B',
    version: 'v0.9.1-fp8',
    category: 'video',
    categoryLabel: '视频生成',
    arch: 'DiT Fast-Diffusion',
    params: '2B',
    vramReqGb: 10,
    sizeGb: 5.6,
    status: 'ready',
    statusLabel: '已预热就绪',
    path: '/data/models/ltx-video',
    quant: 'FP8 高速量化',
    boundApp: 'Video MCP Server',
    description: 'Lightricks 出品的极速轻量级视频生成算法，支持实时预览与快速镜头预演。'
  },
  {
    id: 'cosyvoice-300m',
    name: 'CosyVoice 300M',
    version: 'v2.0',
    category: 'music',
    categoryLabel: '音频与声音',
    arch: 'Multi-Style Speech LLM',
    params: '300M',
    vramReqGb: 4,
    sizeGb: 1.2,
    status: 'ready',
    statusLabel: '已挂载就绪',
    path: '/data/models/cosyvoice',
    quant: 'FP16',
    boundApp: 'Music / Audio Engine',
    description: '多语言、多情感音色克隆与对话语音基模，具备零样本音色复刻与情感表现力。'
  },
  {
    id: 'chattts',
    name: 'ChatTTS',
    version: 'v1.0-standard',
    category: 'music',
    categoryLabel: '音频与声音',
    arch: 'Conversational TTS',
    params: '400M',
    vramReqGb: 2,
    sizeGb: 0.9,
    status: 'ready',
    statusLabel: '已挂载就绪',
    path: '/data/models/chattts',
    quant: 'FP16',
    boundApp: 'Music / Audio Engine',
    description: '专门为对话场景优化的语音合成模型，支持笑声、叹息等真实微表情拟真语音。'
  },
  {
    id: 'hunyuan-video-13b',
    name: 'HunyuanVideo 13B',
    version: 'v1.0-bf16',
    category: 'video',
    categoryLabel: '视频生成',
    arch: 'Dual-Stream Transformer',
    params: '13B',
    vramReqGb: 24,
    sizeGb: 26.0,
    status: 'pending',
    statusLabel: '待同步权重',
    path: '/data/models/hunyuan',
    quant: 'BF16',
    boundApp: 'Video MCP Server',
    description: '腾讯混元开源顶尖视频生成模型，双流结构与完整中文物理世界语义理解。'
  }
])

const filteredModels = computed(() => {
  const q = searchQuery.value.trim().toLowerCase()
  return models.value.filter(m => {
    const tabMatch = currentTab.value === 'all' ||
      (currentTab.value === 'ready' && m.status === 'ready') ||
      (currentTab.value === 'cloud' && m.status === 'cloud') ||
      (currentTab.value === 'pending' && m.status === 'pending')
    const catMatch = currentCategory.value === 'all' || m.category === currentCategory.value
    const textMatch = !q || (m.name + ' ' + m.arch + ' ' + m.boundApp + ' ' + m.description).toLowerCase().includes(q)
    return tabMatch && catMatch && textMatch
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

function handleRefresh() {
  isRefreshing.value = true
  showToast('模型依赖与缓存状态已刷新')
  setTimeout(() => { isRefreshing.value = false }, 500)
}

function openModelDetail(m: ModelItem) {
  selectedModel.value = m
}

function closeDetail() {
  selectedModel.value = null
}

function warmUpModel(m: ModelItem) {
  showToast(`已向 Station 调度器发送模型 [${m.name}] 显存预热请求`)
}

onMounted(() => {
  initTheme()
})
</script>

<template>
  <main class="main">
    <div class="workspace">
      <!-- Heading -->
      <header class="heading">
        <div class="heading-left">
          <h1 id="pageTitle">模型市场</h1>
          <p id="pageSubtitle">查看与管理 Station 声明的 AI 基础模型、权重版本、显存规格与本地缓存状态。</p>
        </div>
        <div class="heading-actions">
          <button class="btn ghost" id="theme" @click="toggleTheme">{{ themeText }}</button>
          <button class="icon-btn"
                  id="reset"
                  title="刷新模型与权重状态"
                  aria-label="刷新模型状态"
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

      <!-- Top KPI Row -->
      <section class="res-metric-grid" style="margin-bottom: 24px;">
        <article class="res-metric-card">
          <div class="res-metric-top">
            <span class="res-metric-label">纳管基础模型</span>
            <div class="res-metric-icon">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                <circle cx="12" cy="12" r="8"></circle>
                <line x1="12" y1="2" x2="12" y2="6"></line>
                <line x1="12" y1="18" x2="12" y2="22"></line>
              </svg>
            </div>
          </div>
          <div class="res-metric-val">7 <small>款核心基模</small></div>
          <div class="res-metric-meter"><div class="res-metric-meter-fill" style="width: 100%;"></div></div>
          <div class="res-metric-sub">覆盖文生图、视频生成、情感音频与大语言模型</div>
        </article>

        <article class="res-metric-card">
          <div class="res-metric-top">
            <span class="res-metric-label">显存适配峰值</span>
            <div class="res-metric-icon">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                <rect x="4" y="4" width="16" height="16" rx="2"></rect>
              </svg>
            </div>
          </div>
          <div class="res-metric-val">24 <small>GB / 32 GB (单卡适配)</small></div>
          <div class="res-metric-meter"><div class="res-metric-meter-fill" style="width: 75%;"></div></div>
          <div class="res-metric-sub">RTX 5090 32GB 专属预留 · 2GB 防爆余量就绪</div>
        </article>

        <article class="res-metric-card">
          <div class="res-metric-top">
            <span class="res-metric-label">高速缓存卷占用</span>
            <div class="res-metric-icon">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                <rect x="2" y="2" width="20" height="8" rx="2"></rect>
                <rect x="2" y="14" width="20" height="8" rx="2"></rect>
              </svg>
            </div>
          </div>
          <div class="res-metric-val">320 <small>GB / 3.7 TB (8.6%)</small></div>
          <div class="res-metric-meter"><div class="res-metric-meter-fill" style="width: 8.6%;"></div></div>
          <div class="res-metric-sub">挂载于 /data/models · NVMe PCIe 5.0 高速直通</div>
        </article>

        <article class="res-metric-card">
          <div class="res-metric-top">
            <span class="res-metric-label">推理后端就绪度</span>
            <div class="res-metric-icon">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                <path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"></path>
                <polyline points="22 4 12 14.01 9 11.01"></polyline>
              </svg>
            </div>
          </div>
          <div class="res-metric-val highlight-green">100% <small>服务在线</small></div>
          <div class="res-metric-meter"><div class="res-metric-meter-fill" style="width: 100%;"></div></div>
          <div class="res-metric-sub">Video MCP + Image MCP 实例健康同频</div>
        </article>
      </section>

      <!-- Catalog / Filter section -->
      <section class="catalog-section">
        <div class="tabs-row">
          <div class="tabs">
            <button class="tab" :class="{ active: currentTab === 'all' }" @click="currentTab = 'all'">
              全部模型 <span>{{ models.length }}</span>
            </button>
            <button class="tab" :class="{ active: currentTab === 'ready' }" @click="currentTab = 'ready'">
              已就绪 <span>{{ models.filter(m => m.status === 'ready').length }}</span>
            </button>
            <button class="tab" :class="{ active: currentTab === 'cloud' }" @click="currentTab = 'cloud'">
              云端直连 <span>{{ models.filter(m => m.status === 'cloud').length }}</span>
            </button>
            <button class="tab" :class="{ active: currentTab === 'pending' }" @click="currentTab = 'pending'">
              待下载 <span>{{ models.filter(m => m.status === 'pending').length }}</span>
            </button>
          </div>
          <div class="search-box">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="11" cy="11" r="8"></circle>
              <line x1="21" y1="21" x2="16.65" y2="16.65"></line>
            </svg>
            <input v-model="searchQuery" placeholder="搜索模型名称、架构或权重规格..." aria-label="搜索模型" type="search">
          </div>
        </div>

        <div class="filters-bar">
          <button class="chip" :class="{ active: currentCategory === 'all' }" @click="currentCategory = 'all'">全部</button>
          <button class="chip" :class="{ active: currentCategory === 'video' }" @click="currentCategory = 'video'">视频生成 (Video)</button>
          <button class="chip" :class="{ active: currentCategory === 'image' }" @click="currentCategory = 'image'">图像生成 (Image)</button>
          <button class="chip" :class="{ active: currentCategory === 'music' }" @click="currentCategory = 'music'">音频与声音 (Music)</button>
          <span class="result-count">{{ filteredModels.length }} 个模型</span>
        </div>

        <!-- Models Cards Grid -->
        <div class="cards-grid" style="display: grid; grid-template-columns: repeat(auto-fill, minmax(360px, 1fr)); gap: 16px; margin-top: 16px;">
          <article v-for="m in filteredModels" :key="m.id" class="card">
            <div class="card-head">
              <div class="app-icon-badge" style="width: 44px; height: 44px; border-radius: 10px; background: var(--shell-button); display: grid; place-items: center; border: 1px solid var(--line); flex-shrink: 0;">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" style="width: 22px; height: 22px; color: var(--accent);">
                  <circle cx="12" cy="12" r="8"></circle>
                  <polygon points="12 8 8 12 12 16 16 12 12 8"></polygon>
                </svg>
              </div>
              <div class="card-title-group" style="flex: 1; min-width: 0;">
                <div style="display: flex; align-items: baseline; justify-content: space-between; gap: 8px;">
                  <h3 style="font-size: 16px; font-weight: 600; margin: 0; color: var(--ink);">{{ m.name }}</h3>
                  <span class="pill" style="font-size: 11px; font-family: monospace; background: var(--shell-button); padding: 2px 6px; border-radius: 4px; border: 1px solid var(--line);">{{ m.version }}</span>
                </div>
                <div style="font-size: 12px; color: var(--muted); margin-top: 2px;">
                  {{ m.arch }} · {{ m.params }}
                </div>
              </div>
            </div>

            <p class="card-desc" style="font-size: 13px; color: var(--muted); line-height: 1.5; margin: 12px 0 16px 0; min-height: 40px;">
              {{ m.description }}
            </p>

            <div class="tags" style="display: flex; flex-wrap: wrap; gap: 6px; margin-bottom: 16px;">
              <span class="tag" style="background: var(--shell-button); border: 1px solid var(--line); font-size: 11px; padding: 2px 8px; border-radius: 4px;">
                显存: {{ m.vramReqGb ? `${m.vramReqGb} GB` : '云端直连' }}
              </span>
              <span class="tag" style="background: var(--shell-button); border: 1px solid var(--line); font-size: 11px; padding: 2px 8px; border-radius: 4px;">
                权重: {{ m.sizeGb ? `${m.sizeGb} GB` : 'API 零本地' }}
              </span>
              <span class="tag" style="background: var(--shell-button); border: 1px solid var(--line); font-size: 11px; padding: 2px 8px; border-radius: 4px;">
                精度: {{ m.quant }}
              </span>
            </div>

            <div class="card-foot" style="display: flex; justify-content: space-between; align-items: center; border-top: 1px solid var(--line); padding-top: 12px;">
              <span class="status" :class="m.status === 'ready' || m.status === 'cloud' ? 'running' : ''" style="display: flex; align-items: center; gap: 6px; font-size: 12px;">
                <i class="dot" :class="{ on: m.status === 'ready' || m.status === 'cloud' }"></i>
                {{ m.statusLabel }}
              </span>
              <div class="card-buttons" style="display: flex; gap: 8px;">
                <button class="btn ghost" style="font-size: 12px; padding: 4px 10px;" @click="openModelDetail(m)">规格详情</button>
                <button v-if="m.status === 'ready'" class="btn primary" style="font-size: 12px; padding: 4px 12px;" @click="warmUpModel(m)">预热显存</button>
                <button v-else-if="m.status === 'pending'" class="btn primary" style="font-size: 12px; padding: 4px 12px;" @click="showToast('开始从模型制品库同步权重')">拉取权重</button>
              </div>
            </div>
          </article>
        </div>
      </section>
    </div>

    <!-- Model Detail Drawer -->
    <div class="overlay" :class="{ open: !!selectedModel }" :style="{ display: selectedModel ? 'flex' : 'none' }" @click.self="closeDetail">
      <section v-if="selectedModel" class="drawer" role="dialog" aria-modal="true" tabindex="-1">
        <div class="drawer-head">
          <span class="eyebrow">MODEL SPECIFICATION</span>
          <button class="btn ghost" @click="closeDetail" aria-label="关闭模型详情">✕</button>
        </div>
        <div style="margin-top: 12px;">
          <h2 style="font-size: 20px; font-weight: 700; margin: 0 0 6px 0;">{{ selectedModel.name }}</h2>
          <p style="color: var(--muted); font-size: 13px;">{{ selectedModel.description }}</p>
        </div>
        <div style="display: flex; gap: 10px; margin: 16px 0;">
          <button v-if="selectedModel.status === 'ready'" class="btn primary" @click="warmUpModel(selectedModel)">预热到显存</button>
          <button class="btn" @click="showToast('已复制模型权重路径与环境变量配置')">复制挂载配置</button>
        </div>
        <div class="detail-content" style="margin-top: 20px;">
          <h3 style="font-size: 14px; font-weight: 600; margin-bottom: 12px;">模型运行拓扑与挂载配置</h3>
          <div style="display: grid; grid-template-columns: 1fr 1fr; gap: 12px; background: var(--shell-button); padding: 16px; border-radius: 8px; border: 1px solid var(--line);">
            <div><small style="color: var(--muted);">模型架构</small><div style="font-weight: 600;">{{ selectedModel.arch }}</div></div>
            <div><small style="color: var(--muted);">参数规模</small><div style="font-weight: 600;">{{ selectedModel.params }}</div></div>
            <div><small style="color: var(--muted);">建议显存</small><div style="font-weight: 600;">{{ selectedModel.vramReqGb }} GB VRAM</div></div>
            <div><small style="color: var(--muted);">量化规格</small><div style="font-weight: 600;">{{ selectedModel.quant }}</div></div>
            <div><small style="color: var(--muted);">本地存储路径</small><div style="font-weight: 600; font-family: monospace;">{{ selectedModel.path }}</div></div>
            <div><small style="color: var(--muted);">纳管微服务应用</small><div style="font-weight: 600;">{{ selectedModel.boundApp }}</div></div>
          </div>
        </div>
      </section>
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
