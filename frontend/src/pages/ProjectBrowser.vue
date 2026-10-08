<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, shallowRef, triggerRef, watch } from 'vue'
import { onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import { callTool } from '../platform/mcp'
import { isProjectID, ProjectReader, readerError, type Cast, type Detail, type ProjectItem } from '../platform/project-reader'
import ProjectMedia from '../components/ProjectMedia.vue'
import ProjectPrompt from '../components/ProjectPrompt.vue'
import ProjectCover from '../components/ProjectCover.vue'

const route = useRoute(), router = useRouter()
const query = (key: string) => typeof route.query[key] === 'string' ? route.query[key] as string : ''
const projectID = computed(() => query('project')), revisionID = computed(() => query('revision'))
const filter = computed(() => query('category')), search = computed(() => query('q'))
const categories = [{ key: '', label: '全部项目' }, { key: 'character', label: '角色' }, { key: 'music', label: '音乐' }, { key: 'video', label: '短视频' }]
const categoryName = (key: string) => categories.find(c => c.key === key)?.label || '项目'
const reader = shallowRef<ProjectReader | null>(null), loading = ref(false), error = ref('')
const items = ref<ProjectItem[]>([]), cursor = ref(''), listLoading = ref(false), listError = ref(''), listReady = ref(false)
let request = 0, listRequest = 0, alive = true, signedOut = false
const project = computed(() => reader.value?.project), content = computed(() => reader.value?.content)
const character = computed(() => content.value?.character), music = computed(() => content.value?.music), video = computed(() => content.value?.video)
const tabs = computed(() => project.value?.manifest.category === 'character'
  ? [{ key: 'setting', label: '角色设定' }, { key: 'extensions', label: '角色扩展' }]
  : project.value?.manifest.category === 'music'
    ? [{ key: 'voice', label: '人声训练' }, { key: 'songs', label: '歌曲创作' }, { key: 'mvs', label: 'MV创作' }]
    : project.value?.manifest.category === 'video' ? [{ key: 'storyboards', label: '故事板' }, { key: 'cast', label: '角色创作' }] : [])
const activeTab = computed(() => tabs.value.find(t => t.key === query('tab')) || tabs.value[0])
const entries = computed<Detail[]>(() => {
  switch (activeTab.value?.key) {
    case 'extensions': return character.value?.extensions || []
    case 'songs': return music.value?.songs || []
    case 'mvs': return music.value?.mvs || []
    case 'storyboards': return video.value?.storyboards || []
    default: return []
  }
})
const detail = computed(() => entries.value.find(d => d.key === query('item')))
const filtered = computed(() => items.value.filter(p => p.name.toLocaleLowerCase().includes(search.value.trim().toLocaleLowerCase())))
const resultKind = computed(() => activeTab.value?.key === 'extensions' ? 'image' : activeTab.value?.key === 'songs' ? 'audio' : 'video')
const fromProject = computed(() => isProjectID(query('from')) && isProjectID(query('fromRevision')) ? { id: query('from'), revision: query('fromRevision'), name: query('fromName') || '短视频' } : null)
const value = (text?: string | null) => text?.trim() ? text : '未填写'
const duration = (seconds?: number | null) => seconds == null ? '时长未填写' : `${seconds} 秒`
function link(change: Record<string, string | undefined>) { return { path: '/projects', query: { ...route.query, ...change } } }
const listLink = computed(() => link({ project: undefined, revision: undefined, tab: undefined, item: undefined, from: undefined, fromRevision: undefined, fromName: undefined }))
function openProject(item: ProjectItem) { return link({ project: item.project_id, revision: item.head_revision_id, tab: undefined, item: undefined }) }
function castLink(cast: Cast) { return link({ project: cast.project_id!, revision: cast.revision_id!, tab: 'setting', item: undefined, from: project.value!.project_id, fromRevision: project.value!.revision_id, fromName: project.value!.manifest.name }) }
const positions = new Map<string, number>()
const scrollStorageKey = 'studio.project.scroll'
try {
  const stored: unknown = JSON.parse(sessionStorage.getItem(scrollStorageKey) || '[]')
  if (Array.isArray(stored)) for (const entry of stored.slice(-60)) {
    if (Array.isArray(entry) && typeof entry[0] === 'string' && entry[0].length <= 4096 && typeof entry[1] === 'number' && Number.isFinite(entry[1]) && entry[1] >= 0) positions.set(entry[0], entry[1])
  }
} catch { /* Navigation works without browser storage. */ }
onBeforeRouteUpdate((_to, from) => {
  if (signedOut) return
  positions.delete(from.fullPath); positions.set(from.fullPath, window.scrollY)
  while (positions.size > 60) positions.delete(positions.keys().next().value!)
  try { sessionStorage.setItem(scrollStorageKey, JSON.stringify([...positions])) } catch { /* Keep in-memory positions. */ }
})
async function restoreScroll() { await nextTick(); if (alive) window.scrollTo(0, positions.get(route.fullPath) || 0) }
async function loadList(more = false) {
  if (signedOut) return
  const current = ++listRequest
  listLoading.value = true; listError.value = ''
  if (!more) { items.value = []; cursor.value = ''; listReady.value = false }
  try {
    const args: Record<string, unknown> = { limit: 50 }
    if (filter.value) args.category = filter.value
    if (more && cursor.value) args.cursor = cursor.value
    const response = await callTool<{ items: ProjectItem[]; next_cursor?: string | null }>('project.list', args)
    if (!alive || current !== listRequest) return
    if (!Array.isArray(response.items) || response.items.some(p => !isProjectID(p.project_id) || !isProjectID(p.head_revision_id) || typeof p.name !== 'string' || typeof p.category !== 'string')) throw new Error('项目列表返回内容不正确。')
    const previous = more ? items.value : []
    items.value = [...new Map([...previous, ...response.items].map(p => [p.project_id, p])).values()]
    if (response.next_cursor != null && typeof response.next_cursor !== 'string') throw new Error('项目分页信息不正确。')
    if (more && response.next_cursor && response.next_cursor === cursor.value) throw new Error('项目分页没有继续，请刷新后重试。')
    cursor.value = response.next_cursor || ''; listReady.value = true
  } catch (cause) { if (alive && current === listRequest) listError.value = readerError(cause) }
  finally { if (alive && current === listRequest) { listLoading.value = false; if (!more) void restoreScroll() } }
}
async function loadProject() {
  const current = ++request
  reader.value?.dispose(); reader.value = null; error.value = ''; loading.value = false
  if (signedOut) return
  if (!projectID.value) { if (!listReady.value && !listLoading.value) void loadList(); return }
  loading.value = true
  const next = new ProjectReader(projectID.value, revisionID.value || undefined)
  reader.value = next
  try {
    await next.load()
    if (!alive || current !== request) return
    triggerRef(reader)
    // Pin an unversioned deep link once; subsequent refreshes keep this revision.
    if (!revisionID.value) await router.replace(link({ revision: next.project!.revision_id }))
  } catch (cause) { if (alive && current === request) { next.dispose(); reader.value = null; error.value = readerError(cause) } }
  finally { if (alive && current === request) { loading.value = false; void restoreScroll() } }
}
function logout() {
  signedOut = true; request++; listRequest++; reader.value?.dispose(); reader.value = null
  items.value = []; listReady.value = false; loading.value = false; listLoading.value = false
  positions.clear()
  try { sessionStorage.removeItem(scrollStorageKey) } catch { /* Storage can be disabled. */ }
  error.value = '当前会话已退出，请登录后刷新。'; listError.value = error.value
}
function retry() { signedOut = false; if (projectID.value) void loadProject(); else void loadList() }
watch([projectID, revisionID], () => void loadProject(), { immediate: true })
watch(filter, () => { listReady.value = false; if (!projectID.value) void loadList() })
watch(() => route.fullPath, () => { if (!loading.value && !listLoading.value) void restoreScroll() })
window.addEventListener('studio:logout', logout)
onBeforeUnmount(() => { alive = false; request++; listRequest++; reader.value?.dispose(); window.removeEventListener('studio:logout', logout) })
</script>

<template>
  <main class="main project-browser">
    <div class="workspace">
      <nav v-if="projectID" class="breadcrumbs" aria-label="面包屑">
        <RouterLink :to="listLink">项目</RouterLink><span>/</span>
        <template v-if="fromProject"><RouterLink :to="link({ project: fromProject.id, revision: fromProject.revision, tab: 'cast', item: undefined, from: undefined, fromRevision: undefined, fromName: undefined })">{{ fromProject.name }}</RouterLink><span>/</span></template>
        <RouterLink :to="{ ...listLink, query: { ...listLink.query, category: project?.manifest.category || filter || undefined } }">{{ categoryName(project?.manifest.category || filter) }}</RouterLink><span>/</span>
        <template v-if="query('item')"><RouterLink :to="link({ item: undefined, tab: undefined })">{{ project?.manifest.name || '项目' }}</RouterLink><span>/</span><RouterLink :to="link({ item: undefined })">{{ activeTab?.label }}</RouterLink><span>/</span><span aria-current="page">{{ detail?.title || '详情' }}</span></template>
        <span v-else aria-current="page">{{ project?.manifest.name || '项目' }}</span>
      </nav>
      <header class="heading">
        <div class="heading-left"><div class="eyebrow">PROJECTS</div><h1>{{ projectID ? (detail?.title || project?.manifest.name || '项目') : '项目' }}</h1><p v-if="!projectID">角色、音乐与短视频创作</p></div>
        <button class="btn" :disabled="loading || listLoading" @click="retry">刷新</button>
      </header>

      <template v-if="!projectID">
        <div class="list-tools">
          <nav class="business-tabs" aria-label="项目分类"><RouterLink v-for="category in categories" :key="category.key" :class="{ active: filter === category.key }" :aria-current="filter === category.key ? 'page' : undefined" :to="link({ category: category.key || undefined })">{{ category.label }}</RouterLink></nav>
          <input :value="search" type="search" aria-label="搜索已加载项目" placeholder="搜索已加载项目" @input="router.replace(link({ q: ($event.target as HTMLInputElement).value || undefined }))">
        </div>
        <div v-if="listError" class="state error" role="alert">{{ listError }}<button class="btn" @click="retry">重试</button></div>
        <div v-if="listLoading && !items.length" class="state" role="status">正在读取项目…</div>
        <div v-else-if="!filtered.length && !listError" class="state">{{ search ? '已加载项目中没有匹配结果。' : '暂无项目。' }}</div>
        <div class="cards">
          <RouterLink v-for="item in filtered" :key="`${item.project_id}:${item.head_revision_id}`" class="project-card" :to="openProject(item)">
            <ProjectCover :project="item" /><div class="card-text"><small>{{ categoryName(item.category) }}</small><h2>{{ item.name }}</h2></div>
          </RouterLink>
        </div>
        <div v-if="cursor" class="more"><span>已加载 {{ items.length }} 个项目</span><button class="btn" :disabled="listLoading" @click="loadList(true)">{{ listLoading ? '正在读取…' : '加载更多' }}</button></div>
      </template>
      <div v-else-if="loading" class="state" role="status">正在读取项目…</div>
      <div v-else-if="error" class="state error" role="alert">{{ error }}<button class="btn" @click="retry">重试</button></div>
      <template v-else-if="project && reader">
        <nav v-if="!query('item')" class="business-tabs" aria-label="项目业务"><RouterLink v-for="tab in tabs" :key="tab.key" :class="{ active: activeTab?.key === tab.key }" :aria-current="activeTab?.key === tab.key ? 'page' : undefined" :to="link({ tab: tab.key, item: undefined })">{{ tab.label }}</RouterLink></nav>
        <div v-if="!content" class="state">暂无业务内容。</div>
        <div v-else-if="query('item') && !detail" class="state">当前项目修订中没有这项内容。</div>
        <article v-else-if="detail" :key="`${project.revision_id}:${activeTab?.key}:${detail.key}`" class="sections">
          <section class="panel"><h2>概述</h2><p class="prose">{{ value(detail.overview) }}</p><p v-if="detail.duration_seconds != null" class="muted">{{ duration(detail.duration_seconds) }}</p><dl v-if="detail.fields.length" class="fields"><template v-for="(field, i) in detail.fields" :key="i"><dt>{{ field.label }}</dt><dd>{{ value(field.value) }}</dd></template></dl></section>
          <section class="panel"><h2>参考图</h2><div v-if="detail.reference_file_ids.length" class="reference-row"><ProjectMedia v-for="(id, i) in detail.reference_file_ids" :key="`${id}:${i}`" :reader="reader" :file-id="id" kind="image" :label="`参考图 ${i + 1}`" /></div><p v-else class="muted">未添加图片</p><ProjectPrompt :text="detail.prompt" /></section>
          <section v-if="activeTab?.key === 'songs'" class="panel"><h2>歌词</h2><p class="prose">{{ value(detail.lyrics) }}</p></section>
          <section class="panel"><h2>{{ resultKind === 'image' ? '结果图' : resultKind === 'audio' ? '歌曲' : '结果视频' }}</h2><div class="sections"><ProjectMedia v-for="(id, i) in detail.result_file_ids" :key="`${id}:${i}`" :reader="reader" :file-id="id" :kind="resultKind" :label="`${detail.title} ${i + 1}`" /><ProjectMedia v-if="!detail.result_file_ids.length" :reader="reader" :kind="resultKind" :label="detail.title" /></div></section>
          <section v-if="detail.file_ids.length" class="panel"><h2>文件</h2><ProjectMedia v-for="id in detail.file_ids" :key="id" :reader="reader" :file-id="id" kind="file" :label="reader.file(id).path" /></section>
        </article>
        <div v-else :key="`${project.revision_id}:${activeTab?.key}`" class="sections">
          <template v-if="character && activeTab?.key === 'setting'">
            <section class="panel"><h2>基本信息</h2><div class="character-basic"><div><ProjectMedia :reader="reader" :file-id="character.portrait_file_id" kind="image" label="半身照" /><ProjectPrompt :text="character.portrait_prompt" /></div><div><dl class="fields"><dt>姓名</dt><dd>{{ value(character.name) }}</dd><dt>概述</dt><dd>{{ value(content.overview) }}</dd><template v-for="(field, i) in character.fields" :key="i"><dt>{{ field.label }}</dt><dd>{{ value(field.value) }}</dd></template></dl></div></div></section>
            <section class="panel"><h2>角色设定图</h2><ProjectMedia :reader="reader" :file-id="character.sheet_file_id" kind="image" label="角色设定图" /><ProjectPrompt :text="character.sheet_prompt" /></section>
          </template>
          <template v-else-if="music && activeTab?.key === 'voice'">
            <section class="panel"><h2>基本信息</h2><dl class="fields"><dt>姓名</dt><dd>{{ value(music.voice.name || music.artist) }}</dd><dt>专辑</dt><dd>{{ value(music.name) }}</dd><dt>概述</dt><dd>{{ value(music.voice.description || content.overview) }}</dd></dl></section>
            <section class="panel"><h2>干声录音</h2><p v-if="!music.voice.recordings.length" class="muted">未添加音频</p><article v-for="recording in music.voice.recordings" :key="recording.key" class="voice-item"><h3>{{ recording.title }}</h3><p class="prose">{{ value(recording.overview) }}</p><ProjectMedia v-for="id in recording.result_file_ids" :key="id" :reader="reader" :file-id="id" kind="audio" :label="recording.title" /><ProjectMedia v-if="!recording.result_file_ids.length" :reader="reader" kind="audio" :label="recording.title" /></article></section>
            <section class="panel"><h2>人声模型</h2><p v-if="!music.voice.models.length" class="muted">未添加人声模型</p><article v-for="model in music.voice.models" :key="model.key" class="voice-item"><h3>{{ model.title }}</h3><p class="prose">{{ value(model.overview) }}</p><dl v-if="model.fields.length" class="fields"><template v-for="(field, i) in model.fields" :key="i"><dt>{{ field.label }}</dt><dd>{{ value(field.value) }}</dd></template></dl><ProjectMedia v-for="id in model.file_ids" :key="id" :reader="reader" :file-id="id" kind="file" :label="reader.file(id).path" /><p v-if="!model.file_ids.length" class="muted">未添加模型文件</p><ProjectMedia v-for="id in model.result_file_ids" :key="id" :reader="reader" :file-id="id" kind="audio" :label="`${model.title}试听`" /></article></section>
          </template>
          <template v-else-if="video && activeTab?.key === 'cast'">
            <div v-if="!video.cast.length" class="state">未添加角色</div>
            <div class="cards"><article v-for="cast in video.cast" :key="cast.key" class="project-card"><component :is="cast.project_id ? 'RouterLink' : 'div'" v-bind="cast.project_id ? { to: castLink(cast) } : {}"><ProjectMedia :reader="reader" :file-id="cast.portrait_file_id" kind="image" :label="cast.name" thumbnail /><div class="card-text"><h2>{{ cast.name }}</h2><p>本片角色：{{ cast.role === 'lead' ? '主角' : '配角' }}</p></div></component></article></div>
          </template>
          <template v-else>
            <section v-if="video && activeTab?.key === 'storyboards'" class="panel"><h2>短视频</h2><p class="prose">{{ value(content.overview) }}</p><p class="muted video-meta">{{ duration(video.duration_seconds) }}<span v-if="video.aspect"> · {{ video.aspect }}</span></p><ProjectMedia :reader="reader" :file-id="video.output_file_id" kind="video" :label="project.manifest.name" /></section>
            <p v-else-if="music" class="prose">{{ value(content.overview) }}</p>
            <div v-if="!entries.length" class="state">暂无{{ activeTab?.label }}内容。</div>
            <div class="cards"><RouterLink v-for="(entry, index) in entries" :key="entry.key" class="project-card" :to="link({ tab: activeTab?.key, item: entry.key })"><ProjectMedia v-if="entry.image_file_id || activeTab?.key !== 'songs'" :reader="reader" :file-id="entry.image_file_id" kind="image" :label="entry.title" thumbnail /><div v-else class="song-glyph" aria-hidden="true">♫</div><div class="card-text"><small v-if="activeTab?.key === 'storyboards'">{{ String(index + 1).padStart(2, '0') }} · {{ duration(entry.duration_seconds) }}</small><h2>{{ entry.title }}</h2><p>{{ value(entry.overview) }}</p></div></RouterLink></div>
          </template>
        </div>
      </template>
    </div>
  </main>
</template>

<style scoped>
.project-browser { overflow-wrap: anywhere; }
.eyebrow { color: var(--accent); font-size: 11px; font-weight: 700; letter-spacing: .14em; }
.breadcrumbs { display: flex; align-items: center; flex-wrap: wrap; gap: 10px; margin-bottom: 20px; font-size: 13px; color: var(--muted); }
.breadcrumbs a:hover { color: var(--accent); }
.breadcrumbs [aria-current] { color: var(--ink); }
.heading { gap: 16px; }
.business-tabs { display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 22px; }
.business-tabs a { border: 1px solid transparent; padding: 9px 17px; border-radius: 8px; color: var(--muted); }
.business-tabs a.active { color: var(--accent); background: var(--panel); border-color: var(--line); }
.list-tools { display: flex; justify-content: space-between; gap: 18px; flex-wrap: wrap; align-items: start; margin-bottom: 22px; }
.list-tools .business-tabs { margin: 0; }
input { width: 250px; max-width: 100%; padding: 10px 14px; border: 1px solid var(--line); border-radius: 8px; color: var(--ink); background: var(--panel); }
.cards { display: grid; grid-template-columns: repeat(3, minmax(0, 1fr)); gap: 20px; }
.project-card { display: block; min-width: 0; border: 1px solid var(--line); border-radius: 14px; overflow: hidden; background: var(--panel); }
a.project-card:hover, .project-card:has(a:hover) { border-color: var(--accent); }
.card-text { padding: 18px; }
.card-text h2 { font-size: 18px; margin: 4px 0 8px; }
.card-text p { color: var(--muted); display: -webkit-box; -webkit-line-clamp: 2; -webkit-box-orient: vertical; overflow: hidden; }
small, .muted { color: var(--muted); }
.song-glyph { aspect-ratio: 16 / 10; display: grid; place-items: center; font-size: 56px; line-height: 1; color: var(--accent); background: var(--subtle); }
.sections { display: grid; gap: 24px; }
.panel { padding: 24px; min-width: 0; background: var(--panel); border: 1px solid var(--line); border-radius: 14px; }
.panel > h2 { font-size: 18px; margin-bottom: 20px; }
.prose { white-space: pre-wrap; line-height: 1.8; }
.character-basic { display: grid; grid-template-columns: minmax(180px, 280px) minmax(0, 1fr); gap: 28px; align-items: start; }
.character-basic :deep(.image-open img) { aspect-ratio: 4 / 5; object-fit: cover; object-position: center top; }
.fields { display: grid; grid-template-columns: minmax(80px, 140px) minmax(0, 1fr); }
.fields dt, .fields dd { padding: 12px 10px; border-bottom: 1px solid var(--line); white-space: pre-wrap; }
.fields dt { color: var(--muted); }
.reference-row { display: flex; gap: 16px; overflow-x: auto; padding-bottom: 8px; }
.reference-row > :deep(.project-media) { flex: 0 0 220px; }
.reference-row :deep(.image-open img) { height: 240px; object-fit: cover; object-position: center top; }
.voice-item + .voice-item { border-top: 1px solid var(--line); padding-top: 24px; margin-top: 24px; }
.voice-item h3, .voice-item .prose, .voice-item .fields { margin-bottom: 14px; }
.voice-item :deep(.project-media) + :deep(.project-media) { margin-top: 14px; }
.video-meta { margin: 8px 0 18px; }
.state { display: flex; align-items: center; gap: 12px; flex-wrap: wrap; padding: 30px; border: 1px dashed var(--line); border-radius: 12px; color: var(--muted); margin-bottom: 20px; }
.error { color: var(--danger); }
.more { display: flex; justify-content: center; gap: 16px; align-items: center; margin-top: 24px; color: var(--muted); }
@media (max-width: 1150px) { .cards { grid-template-columns: repeat(2, minmax(0, 1fr)); } .character-basic { grid-template-columns: minmax(140px, 220px) minmax(0, 1fr); gap: 18px; } }
@media (max-width: 700px) { .project-browser { padding: 20px 16px 40px; } .cards, .character-basic { grid-template-columns: minmax(0, 1fr); } .panel { padding: 18px; } .fields { grid-template-columns: 84px minmax(0, 1fr); } .business-tabs a { padding: 8px 10px; } .list-tools, input { width: 100%; } .reference-row > :deep(.project-media) { flex-basis: 190px; } }
</style>
