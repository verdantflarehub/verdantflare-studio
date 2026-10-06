<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { callTool, MCPRequestError, uuidv7 } from '../platform/mcp'
import { closeWorkspace, chooseWorkspaceDirectory, chooseWorkspaceFiles, fetchWorkspaceFile, getWorkspaceStatus, importWorkspaceFiles, isDesktopHost, openWorkspace, resumeWorkspace, saveWorkspaceFiles, saveWorkspaceTexts, switchWorkspaceToHead, previewWorkspaceConflict, resolveWorkspaceConflict, type DesktopConflictPreview, type DesktopConflictFile, type DesktopWorkspaceFileInput, type DesktopWorkspaceFileStatus, type DesktopWorkspaceState } from '../platform/desktop'

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
  manifest: { entry_document_id: string; files: Array<{ file_id: string; path: string; role: string; content_ref: { store_id: string; artifact_id: string; version_id: string } }>; asset_refs: Array<{ asset_id: string; asset_version_id: string; purpose: string }>; domain_documents: Array<{ document_type: string; file_id: string }> }
}
interface AssetOpen {
  asset_id: string
  asset_version_id: string
  manifest: { name: string; asset_type: string; subjects: string[]; files: Array<{ file_id: string; path: string; role: string }>; source: { project_id: string; project_revision_id: string; relation: string }; depends_on: Array<{ asset_id: string; asset_version_id: string; purpose: string }> }
}
interface WorkspaceFile {
  file_id: string
  path: string
  role: string
  content_ref: { store_id: string; artifact_id: string; version_id: string }
}
interface WorkspaceManifest {
  project_id: string
  files: WorkspaceFile[]
}
interface WorkspaceSession {
  workspace_id: string
  state: DesktopWorkspaceState
  manifest: WorkspaceManifest
  statuses: DesktopWorkspaceFileStatus[]
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
const entryText = ref('')
const entryLoading = ref(false)
const entrySaving = ref(false)
const desktop = isDesktopHost()
const workspace = ref<WorkspaceSession | null>(null)
const workspaceLoading = ref(false)
const workspaceSaving = ref(false)
const workspaceDirectory = ref('')
const workspaceError = ref('')
const selectedWorkspaceFiles = ref<string[]>([])
const conflictPreview = ref<DesktopConflictPreview | null>(null)
const conflictChoices = ref<Record<number, string>>({})
const conflictTexts = ref<Record<number, string>>({})
const conflictComparison = ref<Record<number, { base: string; head: string }>>({})
const conflictReady = computed(() => !!conflictPreview.value && conflictPreview.value.files.every(file => ['head', 'pending', 'text'].includes(conflictChoices.value[file.index] || '')))

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

function isTextWorkspaceFile(file: WorkspaceFile | undefined): boolean {
  if (!file) return false
  return /\.(md|markdown|txt|json)$/i.test(file.path)
}

function workspaceFile(fileID: string): WorkspaceFile | undefined {
  return workspace.value?.manifest.files.find(file => file.file_id === fileID)
}

function workspaceStatusLabel(state: string): string {
  switch (state) {
    case 'clean': return '本地与远端一致'
    case 'not_materialized': return '尚未下载'
    case 'missing': return '本地文件已移除'
    case 'modified': return '本地已修改'
    case 'conflict': return '路径冲突'
    default: return state
  }
}

function workspaceMIME(path: string): string {
  const extension = path.toLowerCase().split('.').pop() || ''
  const known: Record<string, string> = { png: 'image/png', jpg: 'image/jpeg', jpeg: 'image/jpeg', webp: 'image/webp', gif: 'image/gif', wav: 'audio/wav', mp3: 'audio/mpeg', flac: 'audio/flac', mp4: 'video/mp4', webm: 'video/webm', json: 'application/json', md: 'text/markdown', txt: 'text/plain' }
  return known[extension] || 'application/octet-stream'
}

function setWorkspaceStatuses(statuses: DesktopWorkspaceFileStatus[]) {
  if (!workspace.value) return
  workspace.value.statuses = statuses
  const available = new Set(statuses.filter(status => isTextWorkspaceFile(workspaceFile(status.file_id))).map(status => status.file_id))
  const current = selectedWorkspaceFiles.value.filter(fileID => available.has(fileID))
  selectedWorkspaceFiles.value = current.length
    ? current
    : statuses.filter(status => status.state === 'modified' && available.has(status.file_id)).map(status => status.file_id)
}

async function refreshWorkspaceStatus() {
  if (!workspace.value) return
  workspaceLoading.value = true
  workspaceError.value = ''
  try {
    const result = await getWorkspaceStatus(workspace.value.workspace_id)
    workspace.value.state = result.state
    workspace.value.manifest = result.manifest as WorkspaceManifest
    setWorkspaceStatuses(result.files || [])
  } catch (cause) {
    workspaceError.value = errorText(cause)
  } finally {
    workspaceLoading.value = false
  }
}

async function openLocalWorkspace() {
  if (!desktop || !selectedProject.value) return
  workspaceError.value = ''
  try {
    const directory = await chooseWorkspaceDirectory()
    if (!directory) return
    workspaceLoading.value = true
    if (workspace.value) {
      await closeWorkspace(workspace.value.workspace_id)
      workspace.value = null
      workspaceDirectory.value = ''
      selectedWorkspaceFiles.value = []
    }
    const result = await openWorkspace(selectedProject.value.project_id, directory)
    const manifest = result.manifest as WorkspaceManifest
    if (!manifest || !Array.isArray(manifest.files) || manifest.project_id !== selectedProject.value.project_id) {
      throw new Error('桌面工作副本返回了无效的工程清单。')
    }
    workspaceDirectory.value = directory
    workspace.value = { workspace_id: result.workspace_id, state: result.state, manifest, statuses: [] }
    await refreshWorkspaceStatus()
  } catch (cause) {
    workspaceError.value = errorText(cause)
  } finally {
    workspaceLoading.value = false
  }
}

async function fetchLocalWorkspaceFile(fileID: string) {
  if (!workspace.value) return
  workspaceLoading.value = true
  workspaceError.value = ''
  try {
    await fetchWorkspaceFile(workspace.value.workspace_id, fileID)
    await refreshWorkspaceStatus()
  } catch (cause) {
    workspaceError.value = errorText(cause)
  } finally {
    workspaceLoading.value = false
  }
}

async function saveLocalWorkspaceTexts() {
  if (!workspace.value || selectedWorkspaceFiles.value.length === 0) return
  workspaceSaving.value = true
  workspaceError.value = ''
  try {
    await saveWorkspaceTexts(workspace.value.workspace_id, selectedWorkspaceFiles.value)
    await refreshWorkspaceStatus()
    await loadProjects()
  } catch (cause) {
    workspaceError.value = errorText(cause)
  } finally {
    workspaceSaving.value = false
  }
}

async function saveLocalWorkspaceFiles() {
  if (!workspace.value) return
  const files: DesktopWorkspaceFileInput[] = workspace.value.statuses
    .filter(status => status.state === 'modified' && !isTextWorkspaceFile(workspaceFile(status.file_id)))
    .map(status => {
      const file = workspaceFile(status.file_id)
      return { file_id: status.file_id, path: file?.path || status.path, role: file?.role || 'reference', mime: workspaceMIME(file?.path || status.path) }
    })
  if (files.length === 0) return
  workspaceSaving.value = true
  workspaceError.value = ''
  try {
    const response = await saveWorkspaceFiles(workspace.value.workspace_id, files)
    applyWorkspaceCommit(response.result)
    await refreshWorkspaceStatus()
    await loadProjects()
  } catch (cause) {
    workspaceError.value = errorText(cause)
  } finally {
    workspaceSaving.value = false
  }
}

function applyWorkspaceCommit(value: unknown) {
  if (!workspace.value || !value || typeof value !== 'object') return
  const commit = value as { revision_id?: unknown; manifest?: unknown }
  if (typeof commit.revision_id === 'string') workspace.value.state.base_revision_id = commit.revision_id
  if (commit.manifest && typeof commit.manifest === 'object' && Array.isArray((commit.manifest as { files?: unknown }).files)) {
    workspace.value.manifest = commit.manifest as WorkspaceManifest
  }
}

async function importLocalWorkspaceFiles() {
  if (!workspace.value) return
  workspaceSaving.value = true
  workspaceError.value = ''
  try {
    const selected = await chooseWorkspaceFiles()
    if (selected.length === 0) return
    const imports = selected.map(sourcePath => {
      const name = sourcePath.split(/[\\/]/).pop() || ''
      return { source_path: sourcePath, path: name, role: 'reference', mime: workspaceMIME(name) }
    })
    const response = await importWorkspaceFiles(workspace.value.workspace_id, imports)
    applyWorkspaceCommit(response.result)
    await refreshWorkspaceStatus()
    await loadProjects()
  } catch (cause) {
    workspaceError.value = errorText(cause)
  } finally {
    workspaceSaving.value = false
  }
}

async function resumeLocalWorkspace() {
  if (!workspace.value) return
  workspaceSaving.value = true
  workspaceError.value = ''
  try {
    await resumeWorkspace(workspace.value.workspace_id)
    await refreshWorkspaceStatus()
    await loadProjects()
  } catch (cause) {
    workspaceError.value = errorText(cause)
  } finally {
    workspaceSaving.value = false
  }
}

async function previewLocalConflict() {
  if (!workspace.value) return
  workspaceSaving.value = true
  workspaceError.value = ''
  conflictPreview.value = null
  try {
    conflictPreview.value = await previewWorkspaceConflict(workspace.value.workspace_id)
    conflictChoices.value = {}
    conflictTexts.value = Object.fromEntries(conflictPreview.value.files.map(file => [file.index, file.pending.text || '']))
    conflictComparison.value = {}
  } catch (cause) {
    workspaceError.value = errorText(cause)
  } finally {
    workspaceSaving.value = false
  }
}

async function compareConflictText(file: DesktopConflictFile) {
  const preview = conflictPreview.value
  if (!preview) return
  workspaceSaving.value = true
  workspaceError.value = ''
  try {
    const read = async (source: DesktopConflictFile['base'], revision: string) => {
      if (!source) return '该修订中没有此文件。'
      const result = await callTool<{ text: string }>('artifact.read', {
        mode: 'text', content_ref: source.content_ref,
        access: { project_id: preview.project_id, project_revision_id: revision }
      }, preview.project_id)
      return result.text
    }
    const [base, head] = await Promise.all([read(file.base, preview.base_revision_id), read(file.head, preview.head_revision_id)])
    conflictComparison.value[file.index] = { base, head }
  } catch (cause) {
    workspaceError.value = errorText(cause)
  } finally {
    workspaceSaving.value = false
  }
}

async function resolveLocalConflict() {
  const preview = conflictPreview.value
  if (!workspace.value || !preview || !conflictReady.value) return
  workspaceSaving.value = true
  workspaceError.value = ''
  try {
    await resolveWorkspaceConflict(workspace.value.workspace_id, {
      commit_id: preview.commit_id, request_sha256: preview.request_sha256, head_revision_id: preview.head_revision_id,
      choices: preview.files.map(file => ({ index: file.index, choice: conflictChoices.value[file.index]!,
        ...(conflictChoices.value[file.index] === 'text' ? { text: conflictTexts.value[file.index] || '' } : {}) }))
    })
    conflictPreview.value = null
    await refreshWorkspaceStatus()
    await loadProjects()
  } catch (cause) {
    conflictPreview.value = null
    workspaceError.value = `${errorText(cause)}。若提交响应中断，请使用“恢复待提交”；若当前修订已变化，请重新预览。`
  } finally {
    workspaceSaving.value = false
  }
}

async function switchLocalWorkspaceToHead() {
  if (!workspace.value) return
  workspaceSaving.value = true
  workspaceError.value = ''
  try {
    const response = await switchWorkspaceToHead(workspace.value.workspace_id)
    const manifest = response.manifest as WorkspaceManifest
    if (!response.state || !manifest || !Array.isArray(manifest.files) || manifest.project_id !== workspace.value.state.project_id) {
      throw new Error('切换基础修订返回了无效的工程清单。')
    }
    workspace.value.state = response.state
    workspace.value.manifest = manifest
    await refreshWorkspaceStatus()
    await loadProjects()
  } catch (cause) {
    workspaceError.value = errorText(cause)
  } finally {
    workspaceSaving.value = false
  }
}

async function closeLocalWorkspace() {
  if (!workspace.value) return
  const current = workspace.value.workspace_id
  workspaceLoading.value = true
  try {
    await closeWorkspace(current)
  } catch (cause) {
    workspaceError.value = errorText(cause)
    return
  } finally {
    workspaceLoading.value = false
  }
  workspace.value = null
  conflictPreview.value = null
  workspaceDirectory.value = ''
  selectedWorkspaceFiles.value = []
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
    if (workspace.value && workspace.value.state.project_id !== item.project_id) {
      await closeLocalWorkspace()
    }
    selectedProject.value = await callTool<ProjectOpen>('project.open', { project_id: item.project_id, revision_id: item.head_revision_id }, item.project_id)
    const entry = selectedProject.value.manifest.files.find(file => file.file_id === selectedProject.value?.manifest.entry_document_id)
    if (entry) {
      entryLoading.value = true
      const read = await callTool<{ text?: string }>('artifact.read', {
        mode: 'text',
        content_ref: entry.content_ref,
        access: { project_id: selectedProject.value.project_id, project_revision_id: selectedProject.value.revision_id }
      }, selectedProject.value.project_id)
      entryText.value = read.text || ''
    }
    router.replace({ query: { ...route.query, project: item.project_id } })
  } catch (cause) {
    error.value = errorText(cause)
  } finally {
    entryLoading.value = false
  }
}

async function saveEntry() {
  if (!selectedProject.value) return
  const project = selectedProject.value
  const entry = project.manifest.files.find(file => file.file_id === project.manifest.entry_document_id)
  if (!entry) return
  entrySaving.value = true
  error.value = ''
  try {
    await callTool('project.commit', {
      project_id: project.project_id,
      expected_revision_id: project.revision_id,
      commit_id: uuidv7(),
      changes: {
        upsert_files: [{ file_id: entry.file_id, path: entry.path, role: entry.role, text: entryText.value, mime: 'text/markdown' }]
      }
    }, project.project_id)
    await loadProjects()
    const item = projects.value.find(value => value.project_id === project.project_id)
    if (item) await openProject(item)
  } catch (cause) {
    error.value = errorText(cause)
  } finally {
    entrySaving.value = false
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
onBeforeUnmount(() => { if (workspace.value) void closeLocalWorkspace() })
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
              <div class="subheading">入口 MD</div>
              <div v-if="entryLoading" class="inline-state">正在按当前修订读取入口文档…</div>
              <textarea v-else v-model="entryText" class="entry-editor" rows="9" aria-label="入口 MD" />
              <div class="entry-actions"><span>保存会创建新修订，并使用当前修订做并发检查。</span><button class="btn primary" :disabled="entryLoading || entrySaving" @click="saveEntry">{{ entrySaving ? '保存中…' : '保存入口 MD' }}</button></div>
              <section v-if="desktop" class="workspace-panel">
                <div class="section-title"><span>本地工作副本</span><small>显式下载与保存，不后台同步</small></div>
                <div v-if="workspaceError" class="workspace-error" role="alert">{{ workspaceError }}</div>
                <template v-if="!workspace">
                  <p class="workspace-note">选择一台电脑上的项目目录，Studio 会创建或恢复其中的 <code>.vf</code> 状态。服务端修订仍是跨电脑继续项目的依据。</p>
                  <button class="btn primary" :disabled="workspaceLoading" @click="openLocalWorkspace">{{ workspaceLoading ? '打开中…' : '选择目录并打开工作副本' }}</button>
                </template>
                <template v-else>
                  <dl class="workspace-facts"><dt>本地目录</dt><dd>{{ workspaceDirectory }}</dd><dt>基础修订</dt><dd>{{ shortID(workspace.state.base_revision_id) }}</dd><dt>文件状态</dt><dd>{{ workspace.statuses.filter(item => item.state === 'modified').length }} 个已修改 · {{ workspace.statuses.filter(item => item.state === 'not_materialized' || item.state === 'missing').length }} 个待下载</dd></dl>
                  <div class="workspace-actions"><button class="btn" :disabled="workspaceLoading" @click="refreshWorkspaceStatus">{{ workspaceLoading ? '读取中…' : '刷新状态' }}</button><button v-if="selectedProject.head_revision_id !== workspace.state.base_revision_id" class="btn" :disabled="workspaceSaving || workspaceLoading" @click="switchLocalWorkspaceToHead">切换到当前修订</button><button class="btn" :disabled="workspaceSaving || selectedWorkspaceFiles.length === 0" @click="saveLocalWorkspaceTexts">{{ workspaceSaving ? '保存中…' : `保存选中文本（${selectedWorkspaceFiles.length}）` }}</button><button class="btn" :disabled="workspaceSaving || !workspace.statuses.some(item => item.state === 'modified' && !isTextWorkspaceFile(workspaceFile(item.file_id)))" @click="saveLocalWorkspaceFiles">保存已修改媒体</button><button class="btn" :disabled="workspaceSaving" @click="importLocalWorkspaceFiles">导入本地文件</button><button class="btn" :disabled="workspaceSaving" @click="resumeLocalWorkspace">恢复待提交</button><button class="btn" :disabled="workspaceSaving || workspaceLoading" @click="previewLocalConflict">处理提交冲突</button><button class="btn" :disabled="workspaceLoading" @click="closeLocalWorkspace">关闭</button></div>
                  <ul class="workspace-file-list"><li v-for="status in workspace.statuses" :key="status.file_id"><label v-if="isTextWorkspaceFile(workspaceFile(status.file_id))" class="workspace-check"><input v-model="selectedWorkspaceFiles" type="checkbox" :value="status.file_id" :disabled="status.state !== 'modified'" /><span>{{ workspaceFile(status.file_id)?.path }}</span></label><span v-else class="workspace-file-name">{{ workspaceFile(status.file_id)?.path }}</span><span class="workspace-file-state" :data-state="status.state">{{ workspaceStatusLabel(status.state) }}</span><button v-if="status.state !== 'modified' && status.state !== 'conflict'" class="btn compact" :disabled="workspaceLoading" @click="fetchLocalWorkspaceFile(status.file_id)">下载/校验</button></li></ul>
                  <section v-if="conflictPreview" class="conflict-panel" aria-label="处理提交冲突">
                    <div class="section-title"><span>确认每个文件的处理方式</span><span class="version-pill">当前 {{ shortID(conflictPreview.head_revision_id) }}</span></div>
                    <p class="workspace-note">上次提交已确认冲突。逐文件选择后提交；本地文件和原请求留档保留。这里的“上次内容”指当时提交的快照，后续本地编辑不会自动带入。</p>
                    <article v-for="file in conflictPreview.files" :key="file.index" class="conflict-file">
                      <strong>{{ file.pending.path }}</strong>
                      <small>{{ file.remote_changed ? '当前版本与基础版本不同' : '远端未修改此文件' }}{{ file.can_keep_pending ? '' : ' · 路径已被占用或文件结构已变化，可保留当前结构后另行导入本地草稿' }}</small>
                      <label>处理方式<select v-model="conflictChoices[file.index]" class="field" :disabled="workspaceSaving">
                        <option value="" disabled>请选择</option><option value="head">保留当前版本</option>
                        <option v-if="file.can_keep_pending" value="pending">采用上次提交内容</option>
                        <option v-if="file.can_keep_pending && file.pending.text !== undefined" value="text">手动合并文本</option>
                      </select></label>
                      <template v-if="file.pending.text !== undefined">
                        <button class="btn compact" :disabled="workspaceSaving" @click="compareConflictText(file)">比较基础和当前文本</button>
                        <div v-if="conflictComparison[file.index]" class="conflict-comparison">
                          <label>基础版本<textarea class="field" readonly :value="conflictComparison[file.index]?.base" /></label>
                          <label>当前版本<textarea class="field" readonly :value="conflictComparison[file.index]?.head" /></label>
                        </div>
                        <label>{{ conflictChoices[file.index] === 'text' ? '合并后提交的文本' : '上次提交的文本' }}<textarea class="field" :readonly="conflictChoices[file.index] !== 'text' || workspaceSaving" :value="conflictChoices[file.index] === 'text' ? conflictTexts[file.index] : file.pending.text" @input="conflictTexts[file.index] = ($event.target as HTMLTextAreaElement).value" /></label>
                      </template>
                    </article>
                    <p class="workspace-note">提交后本地正文仍原样保留，与新修订不同的文件继续显示“已修改”。</p>
                    <div class="workspace-actions"><button class="btn primary" :disabled="workspaceSaving || !conflictReady" @click="resolveLocalConflict">确认并应用选择</button><button class="btn" :disabled="workspaceSaving" @click="conflictPreview = null">取消</button></div>
                  </section>
                  <p class="workspace-note">入口 MD 和媒体都按文件单独下载；发现本地修改后，勾选文本并显式保存。若提交响应中断，使用“恢复待提交”继续同一个提交。</p>
                </template>
              </section>
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
.entry-editor { margin-top: 0; min-height: 160px; line-height: 1.55; font-family: ui-monospace, SFMono-Regular, Consolas, monospace; }
.entry-actions { display: flex; align-items: center; justify-content: space-between; gap: 10px; margin-top: 8px; color: var(--muted); font-size: 11px; }
.inline-state { padding: 18px 10px; border: 1px dashed var(--line); border-radius: 8px; color: var(--muted); font-size: 12px; }
.detail-action { margin-top: 18px; width: 100%; }
.reuse-panel { display: grid; gap: 7px; margin-top: 20px; padding: 12px; border: 1px solid color-mix(in srgb, var(--accent) 35%, var(--line)); border-radius: 10px; background: color-mix(in srgb, var(--accent) 7%, var(--shell-panel)); color: var(--ink); font-size: 12px; }
.reuse-panel span { color: var(--muted); line-height: 1.5; }
.workspace-panel { display: grid; gap: 12px; margin-top: 20px; padding: 14px; border: 1px solid color-mix(in srgb, var(--accent) 30%, var(--line)); border-radius: 10px; background: color-mix(in srgb, var(--accent) 4%, var(--shell-panel)); }
.workspace-panel .section-title { margin-bottom: 0; }
.workspace-note { margin: 0; color: var(--muted); font-size: 11px; line-height: 1.55; }
.workspace-note code { color: var(--ink); }
.workspace-error { padding: 9px 10px; border-radius: 7px; color: var(--danger); background: color-mix(in srgb, var(--danger) 8%, transparent); font-size: 11px; overflow-wrap: anywhere; }
.workspace-facts { display: grid; grid-template-columns: 80px minmax(0, 1fr); gap: 6px 10px; margin: 0; font-size: 11px; }
.workspace-facts dt { color: var(--muted); }
.workspace-facts dd { margin: 0; color: var(--ink); overflow-wrap: anywhere; }
.workspace-actions { display: flex; flex-wrap: wrap; gap: 7px; }
.workspace-file-list { display: grid; gap: 6px; margin: 0; padding: 0; list-style: none; max-height: 280px; overflow: auto; }
.workspace-file-list li { display: flex; align-items: center; gap: 8px; min-height: 34px; padding: 6px 8px; border: 1px solid var(--line); border-radius: 7px; font-size: 11px; }
.workspace-check, .workspace-file-name { display: flex; align-items: center; gap: 7px; min-width: 0; flex: 1; color: var(--ink); }
.workspace-check span, .workspace-file-name { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.workspace-file-state { color: var(--muted); white-space: nowrap; }
.workspace-file-state[data-state="modified"], .workspace-file-state[data-state="conflict"], .workspace-file-state[data-state="missing"] { color: var(--danger); }
.workspace-file-state[data-state="clean"] { color: var(--accent); }
.btn.compact { padding: 5px 7px; font-size: 10px; white-space: nowrap; }
@media (max-width: 800px) { .content-grid, .form-grid { grid-template-columns: 1fr; } .heading-actions { justify-content: flex-start; } }
.conflict-panel{display:grid;gap:12px;margin-top:14px;padding:14px;border:1px solid var(--line);border-radius:8px;background:var(--subtle)}
.conflict-file{display:grid;gap:9px;min-width:0;padding:12px 0;border-top:1px solid var(--line)}
.conflict-file strong{overflow-wrap:anywhere}.conflict-file small{color:var(--muted);line-height:1.6}.conflict-file label{display:grid;gap:6px;font-size:12px}.conflict-file textarea{width:100%;min-height:130px;resize:vertical;box-sizing:border-box}.conflict-comparison{display:grid;gap:10px;grid-template-columns:repeat(auto-fit,minmax(min(240px,100%),1fr))}
</style>
