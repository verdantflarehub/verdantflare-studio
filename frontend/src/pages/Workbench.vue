<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { useRouter } from 'vue-router'

const router = useRouter()
const themeText = ref('深色主题')
const isRefreshing = ref(false)

interface ProjectItem {
  id: string
  name: string
  type: string
  director: string
  status: string
  statusType: 'running' | 'done' | 'review'
  shotsCount: number
  updatedAt: string
  desc: string
}

interface PipelineRun {
  id: string
  project: string
  capability: string
  model: string
  costVram: string
  duration: string
  status: 'succeeded' | 'running' | 'queued'
}

const projects = ref<ProjectItem[]>([
  {
    id: 'proj-xiaoyue',
    name: '《小月》MV 视觉生成与分镜工程',
    type: 'Music-MV',
    director: '周屿',
    status: '进行中',
    statusType: 'running',
    shotsCount: 12,
    updatedAt: '10 分钟前',
    desc: '基于小月单曲的国风水墨科幻 MV 镜头拆解，包含 12 组 Wan2.1 首尾帧生成单元与分镜资产。'
  },
  {
    id: 'proj-cowboy',
    name: '《牛仔》音频分轨与母带制作',
    type: 'Music',
    director: 'Mengsk',
    status: '已交付',
    statusType: 'done',
    shotsCount: 6,
    updatedAt: '2 小时前',
    desc: '吉他、贝斯与人声分轨渲染，CosyVoice 拟真人声与母带动态平衡制作。'
  },
  {
    id: 'proj-sanguo',
    name: '三国数字世界观资产工程',
    type: 'World Assets',
    director: '青焰设计组',
    status: '已入库',
    statusType: 'review',
    shotsCount: 128,
    updatedAt: '昨天',
    desc: '三国人物身份定义、兵器道具、场景布局与专属 LoRA 微调权重数字资产库。'
  }
])

const recentRuns = ref<PipelineRun[]>([
  {
    id: 'run-9821',
    project: '《小月》MV',
    capability: 'Video 生成 (Wan2.1-14B)',
    model: 'Wan 2.1 T2V 14B',
    costVram: '24 GB',
    duration: '42s',
    status: 'succeeded'
  },
  {
    id: 'run-9820',
    project: '《小月》MV',
    capability: 'Image 首帧生成 (FLUX.1)',
    model: 'FLUX.1 [dev]',
    costVram: '16 GB',
    duration: '12s',
    status: 'succeeded'
  },
  {
    id: 'run-9819',
    project: '《牛仔》单曲',
    capability: '音频母带渲染 (Audio Engine)',
    model: 'CosyVoice 300M',
    costVram: '4 GB',
    duration: '8s',
    status: 'succeeded'
  },
  {
    id: 'run-9818',
    project: '《小月》MV',
    capability: '运镜轨迹合成 (LTX-Video)',
    model: 'LTX-Video 2B',
    costVram: '10 GB',
    duration: '6s',
    status: 'running'
  }
])

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
  showToast('工作台项目与流水线状态已刷新')
  setTimeout(() => { isRefreshing.value = false }, 500)
}

function launchApp(type: 'video' | 'image' | 'music') {
  if (type === 'video') {
    router.push({ path: '/market', query: { video: '/dashboard#tasks' } })
  } else if (type === 'image') {
    router.push({ path: '/market', query: { image: '/dashboard#tasks' } })
  } else {
    showToast('正在连接 Music App 音频工作台…')
  }
}

function openProject(p: ProjectItem) {
  showToast(`已载入项目上下文：${p.name}`)
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
          <h1 id="pageTitle">创作工作台</h1>
          <p id="pageSubtitle">影视 AI 创作项目、多模态资产生产线与当前活跃任务调度中枢。</p>
        </div>
        <div class="heading-actions">
          <button class="btn ghost" id="theme" @click="toggleTheme">{{ themeText }}</button>
          <button class="icon-btn"
                  id="reset"
                  title="刷新工作台状态"
                  aria-label="刷新状态"
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

      <!-- 1. Station Online & Cluster Status Banner -->
      <div style="background: var(--shell-panel); border: 1px solid var(--line); border-radius: 14px; padding: 20px 24px; margin-bottom: 24px; display: flex; justify-content: space-between; align-items: center; box-shadow: 0 4px 18px rgba(21, 35, 30, 0.03);">
        <div style="display: flex; align-items: center; gap: 16px;">
          <div style="width: 48px; height: 48px; border-radius: 12px; background: rgba(8, 126, 96, 0.1); border: 1px solid rgba(8, 126, 96, 0.2); display: grid; place-items: center; color: var(--accent);">
            <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 2L2 7l10 5 10-5-10-5zM2 17l10 5 10-5M2 12l10 5 10-5"></path>
            </svg>
          </div>
          <div>
            <div style="display: flex; align-items: center; gap: 8px;">
              <h2 style="font-size: 16px; font-weight: 700; margin: 0; color: var(--ink);">Station 生产环境已就绪</h2>
              <span class="pill" style="font-size: 11px; background: rgba(8, 126, 96, 0.12); color: var(--accent); border: 1px solid rgba(8, 126, 96, 0.25); padding: 2px 8px; border-radius: 4px;">双卡 5090 在线</span>
            </div>
            <p style="font-size: 13px; color: var(--muted); margin: 4px 0 0 0;">
              连接节点：5090集群 (k8s.dev) · DCGM 遥测同频 · 高速卷 /data 3.7TB 就绪 · 零排队延迟
            </p>
          </div>
        </div>
        <div style="display: flex; gap: 10px;">
          <button class="btn" @click="router.push('/resources')">查看硬件资源大盘</button>
          <button class="btn primary" @click="router.push('/market')">浏览应用市场</button>
        </div>
      </div>

      <!-- 2. Creative Studio Engines (Quick Launch Row) -->
      <section style="margin-bottom: 28px;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 14px;">
          <div style="font-size: 14px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.5px; color: var(--muted);">
            创作工具工作区 · 快捷进入
          </div>
          <span style="font-size: 12px; color: var(--muted-2);">支持专业视口平铺展开</span>
        </div>

        <div style="display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 16px;">
          <!-- Card 1: Video MCP -->
          <article class="res-metric-card" style="cursor: pointer;" @click="launchApp('video')">
            <div class="res-metric-top">
              <span class="res-metric-label">VIDEO PRODUCTION</span>
              <div class="res-metric-icon">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                  <polygon points="23 7 16 12 23 17 23 7"></polygon>
                  <rect x="1" y="5" width="15" height="14" rx="2" ry="2"></rect>
                </svg>
              </div>
            </div>
            <div style="font-size: 18px; font-weight: 700; color: var(--ink); margin: 4px 0;">Video MCP 工作台</div>
            <p style="font-size: 12px; color: var(--muted); line-height: 1.5; margin: 4px 0 12px 0;">
              AI 视频生成、首尾帧运镜镜头生产与视频任务调度
            </p>
            <div style="display: flex; justify-content: space-between; align-items: center; border-top: 1px solid var(--line); padding-top: 10px;">
              <span class="status running" style="font-size: 12px;"><i class="dot on"></i>服务就绪</span>
              <span style="font-size: 12px; font-weight: 600; color: var(--accent);">进入工作区 →</span>
            </div>
          </article>

          <!-- Card 2: Image MCP -->
          <article class="res-metric-card" style="cursor: pointer;" @click="launchApp('image')">
            <div class="res-metric-top">
              <span class="res-metric-label">VISUAL ASSETS</span>
              <div class="res-metric-icon">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                  <rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect>
                  <circle cx="8.5" cy="8.5" r="1.5"></circle>
                  <polyline points="21 15 16 10 5 21"></polyline>
                </svg>
              </div>
            </div>
            <div style="font-size: 18px; font-weight: 700; color: var(--ink); margin: 4px 0;">Image MCP 工作台</div>
            <p style="font-size: 12px; color: var(--muted); line-height: 1.5; margin: 4px 0 12px 0;">
              FLUX.1 高质量原画构图、分镜资产与角色概念生成
            </p>
            <div style="display: flex; justify-content: space-between; align-items: center; border-top: 1px solid var(--line); padding-top: 10px;">
              <span class="status running" style="font-size: 12px;"><i class="dot on"></i>服务就绪</span>
              <span style="font-size: 12px; font-weight: 600; color: var(--accent);">进入工作区 →</span>
            </div>
          </article>

          <!-- Card 3: Music Studio -->
          <article class="res-metric-card" style="cursor: pointer;" @click="launchApp('music')">
            <div class="res-metric-top">
              <span class="res-metric-label">AUDIO & STEMS</span>
              <div class="res-metric-icon">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
                  <path d="M9 18V5l12-2v13"></path>
                  <circle cx="6" cy="18" r="3"></circle>
                  <circle cx="18" cy="16" r="3"></circle>
                </svg>
              </div>
            </div>
            <div style="font-size: 18px; font-weight: 700; color: var(--ink); margin: 4px 0;">Music 音频工作台</div>
            <p style="font-size: 12px; color: var(--muted); line-height: 1.5; margin: 4px 0 12px 0;">
              单曲企划、分轨工程渲染与 CosyVoice 拟真人声合成
            </p>
            <div style="display: flex; justify-content: space-between; align-items: center; border-top: 1px solid var(--line); padding-top: 10px;">
              <span class="status running" style="font-size: 12px;"><i class="dot on"></i>服务就绪</span>
              <span style="font-size: 12px; font-weight: 600; color: var(--accent);">进入工作区 →</span>
            </div>
          </article>
        </div>
      </section>

      <!-- 3. Recent Projects & World Assets -->
      <section style="margin-bottom: 28px;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 14px;">
          <div style="font-size: 14px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.5px; color: var(--muted);">
            进行中创作工程与世界观资产
          </div>
          <span style="font-size: 12px; color: var(--muted-2);">共 3 个活跃工程</span>
        </div>

        <div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(360px, 1fr)); gap: 16px;">
          <article v-for="p in projects" :key="p.id" class="card">
            <div class="card-head">
              <div style="width: 40px; height: 40px; border-radius: 10px; background: var(--shell-button); border: 1px solid var(--line); display: grid; place-items: center; color: var(--accent); flex-shrink: 0;">
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" style="width: 20px; height: 20px;">
                  <path d="M22 19a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h5l2 3h9a2 2 0 0 1 2 2z"></path>
                </svg>
              </div>
              <div style="flex: 1; min-width: 0;">
                <div style="display: flex; align-items: baseline; justify-content: space-between; gap: 8px;">
                  <h3 style="font-size: 15px; font-weight: 600; margin: 0; color: var(--ink);">{{ p.name }}</h3>
                  <span class="pill" style="font-size: 11px; padding: 2px 6px; border-radius: 4px; background: var(--shell-button); border: 1px solid var(--line);">
                    {{ p.type }}
                  </span>
                </div>
                <div style="font-size: 12px; color: var(--muted); margin-top: 2px;">
                  负责人：{{ p.director }} · {{ p.shotsCount }} 个分镜/资产单元 · {{ p.updatedAt }}
                </div>
              </div>
            </div>

            <p class="card-desc" style="font-size: 13px; color: var(--muted); margin: 12px 0 16px 0; min-height: 42px;">
              {{ p.desc }}
            </p>

            <div class="card-foot" style="display: flex; justify-content: space-between; align-items: center; border-top: 1px solid var(--line); padding-top: 12px;">
              <span class="status" :class="p.statusType === 'running' ? 'running' : p.statusType === 'done' ? 'running' : ''" style="font-size: 12px;">
                <i class="dot" :class="{ on: p.statusType === 'running' || p.statusType === 'done' }"></i>
                {{ p.status }}
              </span>
              <button class="btn primary" style="font-size: 12px; padding: 4px 12px;" @click="openProject(p)">载入工程</button>
            </div>
          </article>
        </div>
      </section>

      <!-- 4. Pipeline Task Stream / Activity Log -->
      <section style="background: var(--shell-panel); border: 1px solid var(--line); border-radius: 14px; padding: 20px 24px;">
        <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 14px;">
          <div style="font-size: 14px; font-weight: 600; text-transform: uppercase; letter-spacing: 0.5px; color: var(--muted);">
            近期生产管线与任务流水线
          </div>
          <span style="font-size: 12px; color: var(--muted-2);">实时调度记录</span>
        </div>

        <div style="display: flex; flex-direction: column; gap: 8px;">
          <div v-for="run in recentRuns"
               :key="run.id"
               style="display: flex; align-items: center; justify-content: space-between; padding: 10px 14px; background: var(--shell-button); border: 1px solid var(--line); border-radius: 8px; font-size: 13px;">
            <div style="display: flex; align-items: center; gap: 12px;">
              <span style="font-family: monospace; font-weight: 600; color: var(--accent);">{{ run.id }}</span>
              <strong style="color: var(--ink);">{{ run.project }}</strong>
              <span style="color: var(--muted);">{{ run.capability }}</span>
            </div>
            <div style="display: flex; align-items: center; gap: 16px;">
              <span class="tag" style="font-size: 11px; background: var(--shell-panel); border: 1px solid var(--line); padding: 2px 6px; border-radius: 4px;">{{ run.model }}</span>
              <span style="color: var(--muted); font-size: 12px;">耗时 {{ run.duration }} · 显存 {{ run.costVram }}</span>
              <span class="status" :class="{ running: run.status === 'succeeded' || run.status === 'running' }" style="font-size: 12px;">
                <i class="dot" :class="{ on: run.status === 'succeeded' || run.status === 'running' }"></i>
                {{ run.status === 'succeeded' ? '完成' : '运行中' }}
              </span>
            </div>
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
