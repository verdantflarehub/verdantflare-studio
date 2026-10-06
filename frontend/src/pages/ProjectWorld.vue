<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { callTool, MCPRequestError, uuidv7 } from '../platform/mcp'

interface ProjectItem {
  project_id: string
  head_revision_id: string
  name: string
  category: string
  status: string
  created_at: string
}
interface AssetItem {
  asset_id: string
  head_asset_version_id: string
  name: string
  asset_type: string
  subjects: string[]
  created_at: string
}
interface ProjectOpen {
  project_id: string
  revision_id: string
  head_revision_id: string
  manifest: { files: Array<{ file_id: string; path: string; role: string }>; asset_refs: Array<{ asset_id: string; asset_version_id: string; purpose: string }>; domain_documents: Array<{ document_type: string; file_id: string }> }
}
interface AssetOpen {
  asset_id: string
  asset_version_id: string
  manifest: { name: string; asset_type: string; subjects: string[]; files: Array<{ file_id: string; path: string; role: string }>; source: { project_id: string; project_revision_id: string; relation: string }; depends_on: Array<{ asset_id: string; asset_version_id: string; purpose: string }> }
}

const router = useRouter()
const route = useRoute()
const mode = computed<'projects' | 'world'>(() => route.query.mode === 'world' ? 'world' : 'projects')
const projects = ref<ProjectItem[]>([])
const assets = ref<AssetItem[]>([])
const selectedProject = ref<ProjectOpen | null>(null)
const selectedAsset = ref<AssetOpen | null>(null)
const loading = ref(false)
const error = ref('')
const assetType = ref('')
const showCreate = ref(false)
const createName = ref('')
const createCategory = ref('music')
const createEntryPath = ref('review.md')
const createEntryText = ref('# 制作审核记录\n\n## 目标\n')

const assetTypes = [
  { value: '', label: '全部类型' },
  { value: 'voice-model', label: '声音模型' },
  { value: 'character-image', label: '人物形象' },
  { value: 'music-work', label: '音乐作品' },
  { value: 'video-project', label: '视频工程' }
]

function readableDate(value: string) {
  const date = new Date(value)
  return Number.isNaN(date.valueOf()) ? value : date.toLocaleDateString('zh-CN', { year: 'numeric', month: 'short', day: 'numeric' })
}
function shortID(value: string) { return value ? `${value.slice(0, 8)}…` : '—' }
function errorText(value: unknown) {
  if (value instanceof MCPRequestError && value.status === 401) return '当前会话已失效，请重新登录后再试。'
  if (value instanceof MCPRequestError && value.status === 403) return '当前身份没有访问这组 Project / World 数据的权限。'
  return value instanceof Error ? value.message : 'Project / World 服务暂时不可用。'
}

async function loadProjects() {
  loading.value = true
  error.value = ''
  selectedProject.value = null
  try {
    const result = await callTool<{ items?: ProjectItem[] }>('project.list', { limit: 50 })
    projects.value = result.items || []
    const requested = typeof route.query.project === 'string' ? route.query.project : ''
    if (requested) {
      const item = projects.value.find(project => project.project_id === requested)
      if (item) await openProject(item)
    }
  } catch (cause) {
    error.value = errorText(cause)
  } finally {
    loading.value = false
  }
}

async function loadWorld() {
  loading.value = true
  error.value = ''
  selectedAsset.value = null
  try {
    const args: Record<string, unknown> = { limit: 50 }
    if (assetType.value) args.asset_type = assetType.value
    const result = await callTool<{ items?: AssetItem[] }>('world.list', args)
    assets.value = result.items || []
  } catch (cause) {
    error.value = errorText(cause)
  } finally {
    loading.value = false
  }
}

async function openProject(item: ProjectItem) {
  error.value = ''
  try {
    selectedProject.value = await callTool<ProjectOpen>('project.open', { project_id: item.project_id, revision_id: item.head_revision_id }, item.project_id)
    router.replace({ query: { ...route.query, project: item.project_id } })
  } catch (cause) {
    error.value = errorText(cause)
  }
}

async function openAsset(item: AssetItem) {
  error.value = ''
  try {
    selectedAsset.value = await callTool<AssetOpen>('world.get', { asset_id: item.asset_id, asset_version_id: item.head_asset_version_id })
  } catch (cause) {
    error.value = errorText(cause)
  }
}

async function createProject() {
  if (!createName.value.trim() || !createEntryPath.value.trim()) return
  loading.value = true
  error.value = ''
  try {
    await callTool('project.create', {
      commit_id: uuidv7(),
      name: createName.value.trim(),
      category: createCategory.value,
      entry_path: createEntryPath.value.trim(),
      entry_text: createEntryText.value
    })
    showCreate.value = false
    createName.value = ''
    await loadProjects()
  } catch (cause) {
    error.value = errorText(cause)
  } finally {
    loading.value = false
  }
}

async function useSelectedAsset() {
  if (!selectedProject.value || !selectedAsset.value) return
  const projectID = selectedProject.value.project_id
  loading.value = true
  error.value = ''
  try {
    await callTool('project.use_asset', {
      project_id: projectID,
      expected_revision_id: selectedProject.value.revision_id,
      commit_id: uuidv7(),
      asset_id: selectedAsset.value.asset_id,
      asset_version_id: selectedAsset.value.asset_version_id,
      purpose: `${selectedAsset.value.manifest.asset_type}-reference`
    }, projectID)
    await loadProjects()
    const item = projects.value.find(project => project.project_id === projectID)
    if (item) await openProject(item)
  } catch (cause) {
    error.value = errorText(cause)
  } finally {
    loading.value = false
  }
}

function selectMode(next: 'projects' | 'world') {
  const query: Record<string, string> = {}
  if (typeof route.query.mock === 'string') query.mock = route.query.mock
  if (next === 'world') query.mode = 'world'
  router.push({ path: '/projects', query })
}

watch(mode, next => { if (next === 'projects') void loadProjects(); else void loadWorld() })
watch(assetType, () => { if (mode.value === 'world') void loadWorld() })
onMounted(() => { if (mode.value === 'projects') void loadProjects(); else void loadWorld() })
</script>

<template>
  <main class="main">
    <div class="workspace project-world-page">
      <header class="heading">
        <div class="heading-left">
          <div class="eyebrow">PROJECT / WORLD</div>
          <h1>{{ mode === 'projects' ? '项目' : '数字世界' }}</h1>
          <p>{{ mode === 'projects' ? '保存 MD 与工程 JSON，跨电脑继续同一份创作。' : '按资产类型找到可复用的固定版本，来源项目始终保留。' }}</p>
        </div>
        <div class="heading-actions">
          <button class="btn" :class="{ primary: mode === 'projects' }" @click="selectMode('projects')">项目</button>
          <button class="btn" :class="{ primary: mode === 'world' }" @click="selectMode('world')">数字世界</button>
          <button v-if="mode === 'projects'" class="btn primary" @click="showCreate = !showCreate">新建项目</button>
          <button class="icon-btn" title="刷新" aria-label="刷新" @click="mode === 'projects' ? loadProjects() : loadWorld()">↻</button>
        </div>
      </header>

      <div v-if="error" class="state error-state" role="alert">
        <strong>暂时无法读取</strong><span>{{ error }}</span>
        <button class="btn" @click="mode === 'projects' ? loadProjects() : loadWorld()">重试</button>
      </div>

      <form v-if="mode === 'projects' && showCreate" class="create-panel" @submit.prevent="createProject">
        <div class="section-title"><span>新建项目</span><small>首个保存修订由服务端生成</small></div>
        <div class="form-grid">
          <label>项目名称<input v-model="createName" required maxlength="256" placeholder="例如：小月舞蹈镜头" /></label>
          <label>分类<select v-model="createCategory"><option value="music">music</option><option value="video">video</option><option value="image">image</option><option value="music-mv">music-mv</option></select></label>
          <label>入口 MD 路径<input v-model="createEntryPath" required placeholder="review.md" /></label>
        </div>
        <label>入口内容<textarea v-model="createEntryText" rows="5" maxlength="1048576" /></label>
        <div class="form-actions"><button type="button" class="btn" @click="showCreate = false">取消</button><button type="submit" class="btn primary" :disabled="loading">保存到项目</button></div>
      </form>

      <div v-if="loading" class="state loading-state" aria-live="polite">正在读取服务端修订与固定引用…</div>

      <template v-if="mode === 'projects' && !loading">
        <section class="content-grid">
          <div class="list-panel">
            <div class="section-title"><span>我的项目</span><small>{{ projects.length }} 个可访问项目</small></div>
            <div v-if="projects.length === 0" class="state empty-state">还没有已提交项目。新建项目后，入口 MD 与工程清单会一起保存。</div>
            <button v-for="project in projects" :key="project.project_id" class="list-card" :class="{ selected: selectedProject?.project_id === project.project_id }" @click="openProject(project)">
              <span class="card-glyph">▣</span><span class="card-main"><strong>{{ project.name }}</strong><small>{{ project.category }} · {{ project.status }} · {{ readableDate(project.created_at) }}</small></span><span class="card-id">{{ shortID(project.project_id) }}</span>
            </button>
          </div>
          <aside class="detail-panel">
            <div v-if="!selectedProject" class="state empty-state"><strong>选择一个项目</strong><span>打开时读取指定修订，客户端不会自动跟随远端 head。</span></div>
            <template v-else>
              <div class="section-title"><span>工程修订</span><span class="version-pill">{{ shortID(selectedProject.revision_id) }}</span></div>
              <dl class="facts"><dt>Project ID</dt><dd>{{ selectedProject.project_id }}</dd><dt>当前 head</dt><dd>{{ shortID(selectedProject.head_revision_id) }}</dd><dt>入口与文件</dt><dd>{{ selectedProject.manifest.files.length }} 个文件</dd><dt>固定资产</dt><dd>{{ selectedProject.manifest.asset_refs.length }} 个版本</dd></dl>
              <div class="subheading">文件清单</div><ul class="plain-list"><li v-for="file in selectedProject.manifest.files" :key="file.file_id"><span>{{ file.path }}</span><small>{{ file.role }} · {{ shortID(file.file_id) }}</small></li></ul>
              <div v-if="selectedProject.manifest.asset_refs.length" class="subheading">已固定引用</div><ul v-if="selectedProject.manifest.asset_refs.length" class="plain-list"><li v-for="asset in selectedProject.manifest.asset_refs" :key="asset.asset_id + asset.purpose"><span>{{ asset.purpose }}</span><small>{{ shortID(asset.asset_id) }} · {{ shortID(asset.asset_version_id) }}</small></li></ul>
              <div v-if="selectedAsset" class="reuse-panel"><strong>待带入：{{ selectedAsset.manifest.name }}</strong><span>提交后只增加当前项目的固定版本引用，来源资产和原项目不移动。</span><button class="btn primary" :disabled="loading" @click="useSelectedAsset">固定此版本到当前项目</button></div>
            </template>
          </aside>
        </section>
      </template>

      <template v-if="mode === 'world' && !loading">
        <section class="toolbar"><label>资产类型<select v-model="assetType"><option v-for="item in assetTypes" :key="item.value" :value="item.value">{{ item.label }}</option></select></label><span class="toolbar-note">World 只登记明确选中的文件和固定版本</span></section>
        <section class="content-grid">
          <div class="list-panel"><div class="section-title"><span>可复用资产</span><small>{{ assets.length }} 个可见资产</small></div><div v-if="assets.length === 0" class="state empty-state">当前类型没有可见资产。</div><button v-for="asset in assets" :key="asset.asset_id" class="list-card" :class="{ selected: selectedAsset?.asset_id === asset.asset_id }" @click="openAsset(asset)"><span class="card-glyph">◉</span><span class="card-main"><strong>{{ asset.name }}</strong><small>{{ asset.asset_type }} · {{ asset.subjects.join('、') || '未标主体' }}</small></span><span class="card-id">{{ shortID(asset.head_asset_version_id) }}</span></button></div>
          <aside class="detail-panel"><div v-if="!selectedAsset" class="state empty-state"><strong>选择一个资产</strong><span>读取固定版本详情后才能进入项目复用。</span></div><template v-else><div class="section-title"><span>{{ selectedAsset.manifest.name }}</span><span class="version-pill">{{ shortID(selectedAsset.asset_version_id) }}</span></div><dl class="facts"><dt>资产类型</dt><dd>{{ selectedAsset.manifest.asset_type }}</dd><dt>主体</dt><dd>{{ selectedAsset.manifest.subjects.join('、') }}</dd><dt>来源项目修订</dt><dd>{{ shortID(selectedAsset.manifest.source.project_id) }} · {{ shortID(selectedAsset.manifest.source.project_revision_id) }}</dd><dt>关系</dt><dd>{{ selectedAsset.manifest.source.relation }}</dd></dl><div class="subheading">可用文件</div><ul class="plain-list"><li v-for="file in selectedAsset.manifest.files" :key="file.file_id"><span>{{ file.path }}</span><small>{{ file.role }} · {{ shortID(file.file_id) }}</small></li></ul><button class="btn primary detail-action" @click="selectMode('projects')">带入项目后固定引用</button></template></aside>
        </section>
      </template>
    </div>
  </main>
</template>

<style scoped>
.project-world-page { max-width: 1380px; }
.eyebrow { color: var(--accent); font-size: 11px; letter-spacing: .14em; font-weight: 700; }
.heading-actions { display: flex; gap: 8px; align-items: center; flex-wrap: wrap; justify-content: flex-end; }
.state { display: flex; align-items: center; gap: 10px; padding: 22px; border: 1px dashed var(--line); border-radius: 12px; color: var(--muted); background: var(--shell-panel); }
.state strong { color: var(--ink); }
.error-state { border-color: color-mix(in srgb, var(--danger) 45%, var(--line)); color: var(--danger); }
.loading-state { justify-content: center; margin-bottom: 16px; }
.empty-state { min-height: 120px; justify-content: center; flex-direction: column; text-align: center; }
.create-panel, .list-panel, .detail-panel { padding: 20px; border: 1px solid var(--line); border-radius: 14px; background: var(--shell-panel); }
.create-panel { margin-bottom: 16px; }
.section-title { display: flex; align-items: center; justify-content: space-between; gap: 12px; margin-bottom: 14px; color: var(--ink); font-weight: 700; }
.section-title small, .toolbar-note { color: var(--muted); font-size: 12px; font-weight: 400; }
.form-grid { display: grid; grid-template-columns: 2fr 1fr 1.5fr; gap: 12px; }
label { display: grid; gap: 6px; color: var(--muted); font-size: 12px; }
input, select, textarea { width: 100%; border: 1px solid var(--line); border-radius: 8px; background: var(--shell-button); color: var(--ink); padding: 9px 10px; font: inherit; font-size: 13px; }
textarea { resize: vertical; margin-top: 12px; }
.form-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 12px; }
.content-grid { display: grid; grid-template-columns: minmax(0, 1.1fr) minmax(320px, .9fr); gap: 16px; }
.list-card { width: 100%; display: flex; align-items: center; gap: 12px; padding: 13px 12px; margin-top: 8px; border: 1px solid var(--line); border-radius: 10px; background: var(--shell-button); color: var(--ink); text-align: left; cursor: pointer; }
.list-card:hover, .list-card.selected { border-color: var(--accent); background: color-mix(in srgb, var(--accent) 8%, var(--shell-button)); }
.card-glyph { width: 32px; height: 32px; display: grid; place-items: center; border-radius: 8px; background: color-mix(in srgb, var(--accent) 12%, transparent); color: var(--accent); font-size: 17px; }
.card-main { min-width: 0; flex: 1; display: grid; gap: 4px; }
.card-main strong { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.card-main small, .card-id, .plain-list small { color: var(--muted); font-size: 11px; }
.card-id, .version-pill { font-family: ui-monospace, SFMono-Regular, Consolas, monospace; font-size: 11px; color: var(--muted); }
.toolbar { display: flex; align-items: end; gap: 18px; margin-bottom: 16px; }
.toolbar label { width: 220px; }
.facts { display: grid; grid-template-columns: 120px minmax(0, 1fr); gap: 9px 12px; margin: 0 0 20px; font-size: 12px; }
.facts dt { color: var(--muted); }
.facts dd { color: var(--ink); margin: 0; overflow-wrap: anywhere; }
.subheading { margin: 16px 0 8px; color: var(--muted); font-size: 12px; font-weight: 700; }
.plain-list { display: grid; gap: 7px; margin: 0; padding: 0; list-style: none; }
.plain-list li { display: flex; justify-content: space-between; gap: 10px; padding: 9px 10px; border: 1px solid var(--line); border-radius: 8px; font-size: 12px; }
.detail-action { margin-top: 18px; width: 100%; }
.reuse-panel { display: grid; gap: 7px; margin-top: 20px; padding: 12px; border: 1px solid color-mix(in srgb, var(--accent) 35%, var(--line)); border-radius: 10px; background: color-mix(in srgb, var(--accent) 7%, var(--shell-panel)); color: var(--ink); font-size: 12px; }
.reuse-panel span { color: var(--muted); line-height: 1.5; }
@media (max-width: 800px) { .content-grid, .form-grid { grid-template-columns: 1fr; } .heading-actions { justify-content: flex-start; } }
</style>
