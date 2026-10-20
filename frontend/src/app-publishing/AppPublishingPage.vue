<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useHostStore } from '../platform/hostStore'
import { useAppCenterStore } from '../app-center/store'

type Organization = { id: string; name: string }
type PublishedApp = {
  id: string; name: string; groupId?: string; category?: string; summary?: string; description?: string;
  developer?: string; brand?: string; iconRef?: string; publicIconUrl?: string; sortOrder?: number;
  publishedVersion?: string; version?: string; channel?: string; visibility?: string; enabled?: boolean;
  revision?: number; updatedAt?: string; showcase?: {
    screenshots?: string[]; highlights?: string[]; whatsNew?: string; permissions?: string[];
    websiteUrl?: string; docsUrl?: string; sourceUrl?: string; license?: string; languages?: string; platforms?: string;
  };
}
type CatalogApp = { appId: string; version?: string; chart?: string; chartVersion?: string; disabledReason?: string }
type AppDetail = { app: PublishedApp; published?: {
  version: string; chart?: { ociRef: string; version: string; manifestDigest: string; packageDigest: string };
  sourceRevision?: string; licenseId?: string; licenseUrl?: string; releaseNotes?: string;
} }
const router = useRouter()
const host = useHostStore()
const appCenter = useAppCenterStore()
const canOpen = computed(() => Boolean(host.connected && host.identity.roles?.includes('admin') && host.identity.scopes?.includes('app:manage')))
const appReady = computed(() => !appCenter.state.stale && Boolean(appCenter.state.value?.items.some(card => card.appId === 'app-publishing' && card.installation?.state === 'installed' && card.statusCode === 'READY')))
const view = ref<'list' | 'detail' | 'create'>('list')
const apps = ref<PublishedApp[]>([])
const catalogApps = ref<CatalogApp[]>([])
const readAt = ref('')
const listLoading = ref(false)
const detailLoading = ref(false)
const selectedAppId = ref('')
const detail = ref<AppDetail | null>(null)
const search = ref('')
const statusFilter = ref<'all' | 'published' | 'unpublished'>('all')
const page = ref(1)
const pageSize = ref(10)
const catalogById = computed(() => new Map(catalogApps.value.filter(item => item.version && item.chart && !item.disabledReason).map(item => [item.appId, item])))
const publishedCount = computed(() => apps.value.filter(app => app.publishedVersion).length)
const readyCount = computed(() => apps.value.filter(app => catalogById.value.has(app.id)).length)
const filteredApps = computed(() => apps.value.filter(app => {
  const ready = catalogById.value.has(app.id)
  if (statusFilter.value === 'published' && !ready || statusFilter.value === 'unpublished' && ready) return false
  return !search.value.trim() || `${app.name} ${app.id} ${app.category || ''} ${app.summary || ''}`.toLowerCase().includes(search.value.trim().toLowerCase())
}).sort((a, b) => Number(catalogById.value.has(b.id)) - Number(catalogById.value.has(a.id)) || (b.updatedAt || '').localeCompare(a.updatedAt || '') || a.id.localeCompare(b.id)))
const pages = computed(() => Math.max(1, Math.ceil(filteredApps.value.length / pageSize.value)))
const visibleApps = computed(() => filteredApps.value.slice((page.value - 1) * pageSize.value, page.value * pageSize.value))
watch([search, statusFilter, pageSize], () => { page.value = 1 })
watch(pages, value => { if (page.value > value) page.value = value })
watch(appReady, ready => { if (ready && canOpen.value && !readAt.value) void refreshDirectory() })
const organizations = ref<Organization[]>([])
const selectedOrganizations = ref<string[]>([])
const busy = ref(false)
const message = ref('')
const error = ref('')
const published = ref<{ appId: string; version: string; catalogETag: string } | null>(null)
const emptyForm = () => ({
  appId: '', name: '', groupId: 'desktop', category: '', summary: '', description: '', developer: '',
  brand: '', iconRef: '', publicIconUrl: '', sortOrder: 1000,
  screenshots: '', highlights: '', whatsNew: '', permissions: '', websiteUrl: '', docsUrl: '', sourceUrl: '', license: '', languages: '', platforms: '',
  version: '', chartVersion: '', ociRef: '', manifestDigest: '', packageDigest: '', sourceRevision: '', releaseNotes: '', upstreamVersion: '', publisher: '', versionSourceUrl: '', licenseId: '', licenseUrl: '',
  expectedRevision: 0,
})
const form = reactive(emptyForm())

async function callTool(name: string, args: Record<string, unknown>): Promise<any> {
  const response = await fetch('/mcp', {
    method: 'POST', credentials: 'same-origin', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ jsonrpc: '2.0', id: crypto.randomUUID(), method: 'tools/call', params: { name, arguments: args } }),
  })
  const payload = await response.json().catch(() => null)
  if (!response.ok || !payload?.result) throw new Error(payload?.error?.message || `请求失败（${response.status}）`)
  const result = payload.result
  if (result.isError) throw new Error(result.structuredContent?.Message || result.structuredContent?.message || '发布检查失败')
  return result.structuredContent
}

const lines = (value: string) => value.split(/\r?\n/).map(item => item.trim()).filter(Boolean)
async function refreshDirectory() {
  if (listLoading.value) return
  listLoading.value = true; error.value = ''
  try {
    const result = await callTool('app_publish.list', {})
    if (!Array.isArray(result?.items) || !Array.isArray(result?.catalogApps)) throw new Error('应用目录响应不完整')
    apps.value = result.items
    catalogApps.value = result.catalogApps
    readAt.value = new Date().toLocaleString('zh-CN')
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '无法读取应用目录' }
  finally { listLoading.value = false }
}

async function openDetail(appId: string) {
  selectedAppId.value = appId; detail.value = null; detailLoading.value = true; error.value = ''; view.value = 'detail'
  try {
    const result = await callTool('app_publish.get', { app_id: appId })
    if (!result?.app?.id) throw new Error('应用详情响应不完整')
    detail.value = result as AppDetail
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '无法读取应用详情' }
  finally { detailLoading.value = false }
}

async function openCreate() {
  Object.assign(form, emptyForm()); selectedOrganizations.value = []; published.value = null
  error.value = ''; message.value = ''; view.value = 'create'
  await loadOrganizations()
}

function backToList() { view.value = 'list'; detail.value = null; selectedAppId.value = ''; error.value = '' }

async function loadOrganizations() {
  try {
    const items = await callTool('app_publish.organizations', {})
    organizations.value = (Array.isArray(items) ? items : []).filter(item => item.id && item.name)
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '无法读取组织' }
}

async function submit() {
  if (busy.value) return
  busy.value = true; error.value = ''; message.value = ''; published.value = null
  try {
    const result = await callTool('app_publish.publish', {
      appId: form.appId.trim(), name: form.name.trim(), groupId: form.groupId, category: form.category.trim(),
      summary: form.summary.trim(), description: form.description.trim(), developer: form.developer.trim(),
      brand: form.brand.trim(), iconRef: form.iconRef.trim(), publicIconUrl: form.publicIconUrl.trim(), sortOrder: form.sortOrder,
      showcase: {
        screenshots: lines(form.screenshots), highlights: lines(form.highlights), whatsNew: form.whatsNew.trim(),
        permissions: lines(form.permissions), websiteUrl: form.websiteUrl.trim(), docsUrl: form.docsUrl.trim(),
        sourceUrl: form.sourceUrl.trim(), license: form.license.trim(), languages: form.languages.trim(), platforms: form.platforms.trim(),
      },
      version: {
        version: form.version.trim(), chart: { ociRef: form.ociRef.trim(), version: form.chartVersion.trim(), manifestDigest: form.manifestDigest.trim(), packageDigest: form.packageDigest.trim() },
        sourceRevision: form.sourceRevision.trim(), releaseNotes: form.releaseNotes.trim(), upstreamVersion: form.upstreamVersion.trim(),
        publisher: form.publisher.trim(), sourceUrl: form.versionSourceUrl.trim(), licenseId: form.licenseId.trim(), licenseUrl: form.licenseUrl.trim(),
      },
      organizationIds: selectedOrganizations.value, expectedRevision: form.expectedRevision,
    })
    if (!result?.app?.publishedVersion || !result.catalogETag) throw new Error('未取得 Station 目录回读证据，请检查应用状态')
    form.expectedRevision = result.app.revision
    published.value = { appId: result.app.id, version: result.app.publishedVersion, catalogETag: result.catalogETag }
    message.value = '已发布到 Station 目录；用户可自行安装。'
    await refreshDirectory()
    await openDetail(result.app.id)
  } catch (cause) {
    error.value = cause instanceof Error ? cause.message : '发布失败；请先回读应用状态，避免重复提交。'
    // A failure after Control created the draft leaves a real app revision.
    // Refresh it before the operator retries; never repeat a create with 0.
    if (form.appId.trim()) {
      try {
        const current = await callTool('app_publish.get', { app_id: form.appId.trim() }) as AppDetail
        if (current?.app?.id === form.appId.trim() && Number.isInteger(current.app.revision) && (current.app.revision || 0) > 0) {
          form.expectedRevision = current.app.revision!
          message.value = `已回读应用修订 ${form.expectedRevision}。请检查错误后重新提交。`
        }
      } catch { /* No persisted app yet; the next attempt remains a create. */ }
    }
  }
  finally { busy.value = false }
}
onMounted(async () => {
  appCenter.setVisible('app-publishing', true)
  if (!host.connected) await host.init()
  await appCenter.refresh()
  if (canOpen.value && appReady.value) await refreshDirectory()
})
onBeforeUnmount(() => appCenter.setVisible('app-publishing', false))
</script>

<template>
  <main class="publish-page">
    <header class="heading">
      <div class="heading-left">
        <button v-if="view !== 'list'" class="back" @click="backToList">← 应用发布</button>
        <button v-else class="back" @click="router.push('/market')">← 返回应用中心</button>
        <h1>{{ view === 'list' ? '应用发布' : view === 'detail' ? (detail?.app.name || '应用详情') : '新增应用' }}</h1>
        <p>{{ view === 'list' ? '查看已登记应用的发布状态，并管理 Station 目录。' : view === 'detail' ? 'Hub 资料、固定版本与目录状态。' : '填写 Hub 资料和已发行的固定 OCI Chart，发布后由用户自行安装。' }}</p>
      </div>
      <div v-if="canOpen && view === 'list'" class="heading-actions">
        <button class="btn" :disabled="listLoading" @click="refreshDirectory">{{ listLoading ? '读取中…' : '刷新状态' }}</button>
        <button class="btn primary" @click="openCreate">＋ 新增应用</button>
      </div>
      <div v-else-if="canOpen && view === 'detail'" class="heading-actions">
        <button class="btn" :disabled="detailLoading" @click="openDetail(selectedAppId)">{{ detailLoading ? '读取中…' : '刷新详情' }}</button>
      </div>
    </header>
    <p v-if="!canOpen" class="feedback error">需要 Station 管理员身份；Control 还会核对绑定的应用运营角色。</p>
    <p v-else-if="!appReady" class="feedback error">应用发布尚未安装或运行就绪。请返回应用中心查看状态、安装或启动。</p>
    <template v-else>
    <p v-if="error" class="feedback error" role="alert">{{ error }}</p>
    <p v-if="message" class="feedback" role="status">{{ message }}</p>
    <template v-if="view === 'list'">
      <section class="publish-metrics" aria-label="应用发布概览">
        <article><span>登记应用</span><strong>{{ readAt ? apps.length : '—' }}</strong></article>
        <article><span>固定发布指针</span><strong>{{ readAt ? publishedCount : '—' }}</strong></article>
        <article><span>Station 目录收录</span><strong class="ok">{{ readAt ? readyCount : '—' }}</strong></article>
        <article><span>未进入目录</span><strong>{{ readAt ? apps.length - readyCount : '—' }}</strong></article>
      </section>
      <p class="recent">最近读取：{{ readAt || '尚未读取' }}</p>
      <section class="list-panel">
        <div class="list-toolbar">
          <label class="list-search"><span aria-hidden="true">⌕</span><input v-model="search" aria-label="搜索应用名称、ID 或分类" placeholder="搜索应用名称、ID 或分类" /></label>
          <select v-model="statusFilter" aria-label="发布状态筛选"><option value="all">状态：全部</option><option value="published">Station 目录</option><option value="unpublished">未进入目录</option></select>
        </div>
        <div v-if="listLoading && !readAt" class="empty" role="status">正在读取应用…</div>
        <div v-else-if="error && !readAt" class="empty" role="alert"><h2>无法读取应用目录</h2><button class="btn" @click="refreshDirectory">重试</button></div>
        <div v-else-if="!filteredApps.length" class="empty"><h2>{{ apps.length ? '没有匹配结果' : '暂无已登记应用' }}</h2><p>{{ apps.length ? '调整搜索或筛选条件。' : '点击右上角“新增应用”开始发布。' }}</p></div>
        <div v-else class="table-wrap"><table><thead><tr><th>应用</th><th>分类</th><th>发布版本</th><th>Station 目录</th><th>更新时间</th><th>操作</th></tr></thead>
          <tbody><tr v-for="app in visibleApps" :key="app.id"><td><strong>{{ app.name }}</strong><small>{{ app.id }}</small></td><td>{{ app.category || '—' }}</td><td>{{ catalogById.get(app.id)?.version || app.publishedVersion || '—' }}</td><td><span class="directory-status" :class="{ ready: catalogById.has(app.id) }">{{ catalogById.has(app.id) ? '已收录' : '未进入目录' }}</span></td><td>{{ app.updatedAt ? new Date(app.updatedAt).toLocaleString('zh-CN') : '—' }}</td><td><button class="btn small table-action" :aria-label="`查看 ${app.name} 详情`" @click="openDetail(app.id)">查看详情</button></td></tr></tbody>
        </table></div>
        <footer class="table-footer"><span>共 {{ filteredApps.length }} 个应用<span v-if="filteredApps.length"> · 当前显示 {{ (page - 1) * pageSize + 1 }}–{{ Math.min(page * pageSize, filteredApps.length) }}</span></span>
          <div class="pagination"><label>每页 <select v-model.number="pageSize" aria-label="每页条数"><option :value="10">10</option><option :value="20">20</option><option :value="50">50</option></select></label><button class="btn small" :disabled="page <= 1" aria-label="上一页" @click="page--">‹</button><span>{{ page }} / {{ pages }}</span><button class="btn small" :disabled="page >= pages" aria-label="下一页" @click="page++">›</button></div>
        </footer>
      </section>
      <p class="footnote">Station 目录状态来自 Control 的当前 apps.yaml 回读；实际安装还须符合组织权益与本站条件。</p>
    </template>
    <template v-else-if="view === 'detail'">
      <div v-if="detailLoading" class="panel empty" role="status">正在读取应用详情…</div>
      <template v-else-if="detail">
        <section class="detail-head panel"><div><span class="eyebrow">{{ detail.app.id }}</span><h2>{{ detail.app.name }}</h2><p>{{ detail.app.summary || '暂无简介' }}</p></div><span class="directory-status" :class="{ ready: catalogById.has(detail.app.id) }">{{ catalogById.has(detail.app.id) ? 'Station 目录已收录' : '未进入 Station 目录' }}</span></section>
        <div class="detail-columns">
          <div class="detail-main">
            <section class="panel"><h2>应用资料</h2><dl class="detail-fields"><div><dt>分类</dt><dd>{{ detail.app.category || '—' }}</dd></div><div><dt>主分组</dt><dd>{{ detail.app.groupId || '—' }}</dd></div><div><dt>开发者</dt><dd>{{ detail.app.developer || '—' }}</dd></div><div><dt>资料修订</dt><dd>{{ detail.app.revision || '—' }}</dd></div></dl><p class="detail-description">{{ detail.app.description || '暂无应用介绍' }}</p></section>
            <section class="panel"><h2>展示内容</h2><h3>功能亮点</h3><ul v-if="detail.app.showcase?.highlights?.length"><li v-for="item in detail.app.showcase.highlights" :key="item">{{ item }}</li></ul><p v-else>暂无功能亮点</p><h3>展示截图</h3><ul v-if="detail.app.showcase?.screenshots?.length" class="url-list"><li v-for="url in detail.app.showcase.screenshots" :key="url">{{ url }}</li></ul><p v-else>暂无展示截图</p><h3 v-if="detail.app.showcase?.whatsNew">新功能</h3><p v-if="detail.app.showcase?.whatsNew">{{ detail.app.showcase.whatsNew }}</p><h3 v-if="detail.app.showcase?.permissions?.length">所需权限</h3><ul v-if="detail.app.showcase?.permissions?.length"><li v-for="item in detail.app.showcase.permissions" :key="item">{{ item }}</li></ul></section>
          </div>
          <div class="detail-side">
            <section class="panel"><h2>发布版本</h2><dl class="detail-fields"><div><dt>应用版本</dt><dd>{{ catalogById.get(detail.app.id)?.version || detail.app.publishedVersion || '—' }}</dd></div><div><dt>Chart 版本</dt><dd>{{ detail.published?.chart?.version || '—' }}</dd></div><div><dt>OCI Chart</dt><dd class="break">{{ detail.published?.chart?.ociRef || '—' }}</dd></div><div><dt>Manifest SHA-256</dt><dd class="break">{{ detail.published?.chart?.manifestDigest || '—' }}</dd></div><div><dt>Package SHA-256</dt><dd class="break">{{ detail.published?.chart?.packageDigest || '—' }}</dd></div><div><dt>源码 revision</dt><dd class="break">{{ detail.published?.sourceRevision || '—' }}</dd></div><div><dt>许可证</dt><dd>{{ detail.published?.licenseId || '—' }}</dd></div><div><dt>许可证证据</dt><dd class="break">{{ detail.published?.licenseUrl || '—' }}</dd></div></dl><h3 v-if="detail.published?.releaseNotes">更新说明</h3><p v-if="detail.published?.releaseNotes">{{ detail.published.releaseNotes }}</p></section>
            <section class="panel"><h2>其他信息</h2><dl class="detail-fields"><div><dt>网站</dt><dd class="break">{{ detail.app.showcase?.websiteUrl || '—' }}</dd></div><div><dt>文档</dt><dd class="break">{{ detail.app.showcase?.docsUrl || '—' }}</dd></div><div><dt>源码</dt><dd class="break">{{ detail.app.showcase?.sourceUrl || '—' }}</dd></div><div><dt>展示许可证</dt><dd>{{ detail.app.showcase?.license || '—' }}</dd></div><div><dt>语言</dt><dd>{{ detail.app.showcase?.languages || '—' }}</dd></div><div><dt>平台</dt><dd>{{ detail.app.showcase?.platforms || '—' }}</dd></div></dl></section>
          </div>
        </div>
      </template>
    </template>
    <template v-else>
    <p class="notice">第一版接收已由 Market Actions 发行的 OCI Chart。Chart 源码上传与自动发行尚未接入；临时 .tgz 预检不能作为可安装制品。</p>
    <form @submit.prevent="submit">
      <section class="panel"><h2>1. 应用目录资料</h2><p>与 Hub 发布字段一致；标 * 的字段为发布必填。</p>
        <div class="grid"><label>应用 ID * <input v-model="form.appId" required pattern="[a-z0-9][a-z0-9-]{0,61}[a-z0-9]" placeholder="my-app" /></label>
          <label>应用名称 * <input v-model="form.name" required minlength="2" maxlength="128" /></label>
          <label>主分组 * <select v-model="form.groupId"><option value="desktop">创作工具</option><option value="image">图像</option><option value="video">视频</option><option value="music">音乐</option><option value="channel">渠道</option><option value="system">系统应用</option></select></label>
          <label>市场分类 * <input v-model="form.category" required /></label><label>开发者／发行方 * <input v-model="form.developer" required /></label>
          <label class="wide">一句话简介 * <input v-model="form.summary" required maxlength="512" /></label><label class="wide">完整介绍 * <textarea v-model="form.description" required rows="5" maxlength="16384" /></label>
          <label class="wide">展示截图 HTTPS URL * <small>每行一张，1–8 张</small><textarea v-model="form.screenshots" required rows="4" /></label>
          <label class="wide">功能亮点 * <small>每行一条，1–12 条</small><textarea v-model="form.highlights" required rows="4" /></label>
          <label>品牌标识 <input v-model="form.brand" /></label><label>图标引用 <input v-model="form.iconRef" placeholder="brand:vf" /></label>
          <label class="wide">官方图标 HTTPS URL <input v-model="form.publicIconUrl" type="url" /></label><label>排序 <input v-model.number="form.sortOrder" type="number" min="0" max="1000000" /></label>
        </div>
        <details><summary>选填的客户详情</summary><div class="grid optional">
          <label class="wide">新功能 <textarea v-model="form.whatsNew" rows="3" /></label><label class="wide">权限说明 <small>每行一条</small><textarea v-model="form.permissions" rows="3" /></label>
          <label>网站 URL <input v-model="form.websiteUrl" type="url" /></label><label>文档 URL <input v-model="form.docsUrl" type="url" /></label>
          <label>源码 URL <input v-model="form.sourceUrl" type="url" /></label><label>展示许可证 <input v-model="form.license" /></label>
          <label>语言 <input v-model="form.languages" /></label><label>平台 <input v-model="form.platforms" /></label>
        </div></details>
      </section>
      <section class="panel"><h2>2. 已发行的固定版本</h2><p>OCI 地址和双摘要必须来自 Market Actions 的实际制品；源码 revision、许可证证据必须可核对。</p>
        <div class="grid"><label>应用版本 * <input v-model="form.version" required placeholder="1.0.0" /></label><label>Chart 版本 * <input v-model="form.chartVersion" required placeholder="1.0.0" /></label>
          <label class="wide">OCI Chart 引用 * <input v-model="form.ociRef" required placeholder="oci://registry.../charts/my-app" /></label>
          <label class="wide">OCI Manifest SHA-256 * <input v-model="form.manifestDigest" required placeholder="sha256:..." /></label>
          <label class="wide">Chart Package SHA-256 * <input v-model="form.packageDigest" required placeholder="sha256:..." /></label>
          <label>源码 revision * <input v-model="form.sourceRevision" required /></label><label>许可证标识 * <input v-model="form.licenseId" required placeholder="MIT" /></label>
          <label class="wide">许可证证据 HTTPS URL * <input v-model="form.licenseUrl" required type="url" /></label>
          <label>上游版本 <input v-model="form.upstreamVersion" /></label><label>版本发行方 <input v-model="form.publisher" /></label>
          <label class="wide">上游源码／发行页 URL <input v-model="form.versionSourceUrl" type="url" /></label>
          <label class="wide">更新说明 <small>替换现有版本时必填</small><textarea v-model="form.releaseNotes" rows="3" /></label>
        </div>
      </section>
      <section class="panel"><h2>3. 目标组织</h2><p>至少选择一个获准安装的组织。Hub 资料公开可浏览，Station 安装按组织权益复核。</p>
        <div v-if="organizations.length" class="orgs"><label v-for="org in organizations" :key="org.id"><input v-model="selectedOrganizations" type="checkbox" :value="org.id" /> {{ org.name }} <small>{{ org.id }}</small></label></div>
        <p v-else>当前没有可选择的组织；请检查应用运营权限与 Control 连接。</p>
      </section>
      <footer><button type="submit" :disabled="busy || !selectedOrganizations.length">{{ busy ? '正在核验并发布…' : '发布到 Station 目录' }}</button><span>不会自动安装到 Station。</span></footer>
    </form>
    <aside v-if="published" class="result"><h2>目录回读通过</h2><p>应用 {{ published.appId }} · 版本 {{ published.version }}</p><small>目录 ETag：{{ published.catalogETag }}</small></aside>
    </template>
    </template>
  </main>
</template>

<style scoped>
.publish-page { width: 100%; max-width: 1440px; margin: 0 auto; padding: 28px 32px 80px; color: var(--ink); box-sizing: border-box; }
.heading { display: flex; justify-content: space-between; align-items: flex-end; gap: 20px; margin-bottom: 24px; }
.heading-left h1 { margin: 0; font-size: 28px; line-height: 1.2; }
.heading-left p { margin: 8px 0 0; }
.heading-actions { display: flex; gap: 8px; align-items: center; flex-shrink: 0; }
.back { display: block; border: 0; background: none; color: var(--accent); cursor: pointer; padding: 0; margin-bottom: 13px; font-size: 12px; }
h2 { font-size: 17px; margin: 0 0 12px; } h3 { font-size: 13px; margin: 18px 0 6px; }
p { color: var(--muted); line-height: 1.55; }.eyebrow { font-size: 11px; color: var(--accent); letter-spacing: .08em; }
.notice,.feedback,.result { border: 1px solid var(--line); border-radius: 12px; background: var(--panel); padding: 15px 18px; margin: 16px 0; }
.notice { border-color: var(--amber); }.feedback { color: var(--accent); }.error { color: var(--danger); }
.publish-metrics { display: grid; grid-template-columns: repeat(4, minmax(0,1fr)); gap: 12px; }
.publish-metrics article { padding: 16px 20px; border: 1px solid var(--line); border-radius: 10px; background: var(--shell-panel); }
.publish-metrics span { display: block; color: var(--muted); font-size: 12px; }
.publish-metrics strong { display: block; margin-top: 8px; font-size: 27px; font-weight: 600; line-height: 1.15; }
.publish-metrics .ok { color: var(--accent); }
.recent,.footnote { font-size: 12px; color: var(--muted); margin: 14px 2px 20px; }
.list-panel { border: 1px solid var(--line); border-radius: 12px; background: var(--shell-panel); overflow: hidden; }
.list-toolbar { display: flex; align-items: center; gap: 8px; padding: 14px 16px; border-bottom: 1px solid var(--line); }
.list-search { display: flex; align-items: center; gap: 8px; flex: 1; min-width: 220px; min-height: 36px; border: 1px solid var(--line); border-radius: 7px; padding: 0 12px; background: var(--subtle); color: var(--muted); }
.list-search input { width: 100%; border: 0; outline: 0; background: transparent; color: var(--ink); padding: 0; }
.list-toolbar select,.pagination select { border: 1px solid var(--line); border-radius: 7px; padding: 6px 10px; background: var(--panel); color: var(--ink); font-size: 12px; min-height: 36px; }
.table-wrap { overflow-x: auto; }
table { width: 100%; border-collapse: collapse; font-size: 12px; text-align: left; }
th,td { padding: 13px 16px; border-bottom: 1px solid var(--line); vertical-align: middle; }
th { color: var(--muted); font-weight: 500; white-space: nowrap; }
tbody tr:last-child td { border-bottom: 0; }
td strong,td small { display: block; } td strong { font-size: 13px; font-weight: 600; color: var(--ink); } td small { color: var(--muted); margin-top: 3px; }
.table-action { min-height: 30px; padding: 5px 10px; border: 1px solid var(--line); border-radius: 8px; background: var(--shell-panel); color: var(--ink); font: inherit; font-size: 12px; white-space: nowrap; cursor: pointer; }
.table-action:hover { border-color: var(--accent); }
.table-action:focus-visible { outline: 2px solid var(--accent); outline-offset: 2px; }
.directory-status { display: inline-flex; color: var(--muted); border: 1px solid var(--line); border-radius: 999px; padding: 4px 9px; font-size: 11px; white-space: nowrap; }
.directory-status.ready { color: var(--accent); border-color: var(--accent); }
.empty { padding: 48px 20px; text-align: center; color: var(--muted); }.empty h2 { color: var(--ink); }
.table-footer { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; border-top: 1px solid var(--line); padding: 12px 16px; color: var(--muted); font-size: 12px; }
.pagination { display: flex; align-items: center; gap: 10px; }.pagination label { display: flex; flex-direction: row; align-items: center; gap: 8px; }.pagination select { min-height: 30px; }
.panel { border: 1px solid var(--line); border-radius: 14px; background: var(--panel); padding: 22px; min-width: 0; }.panel>p { margin: 0 0 20px; font-size: 13px; }
.detail-head { display: flex; justify-content: space-between; align-items: flex-start; gap: 18px; margin-bottom: 18px; }.detail-head h2 { margin: 5px 0; font-size: 22px; }
.detail-columns { display: grid; grid-template-columns: minmax(0, 2fr) minmax(260px, 1fr); gap: 18px; }.detail-main,.detail-side { display: grid; align-content: start; gap: 18px; }
.detail-fields { margin: 0; }.detail-fields div { display: flex; justify-content: space-between; gap: 16px; padding: 9px 0; border-bottom: 1px solid var(--line); font-size: 12px; }.detail-fields div:last-child { border-bottom: 0; }
.detail-fields dt { color: var(--muted); flex-shrink: 0; }.detail-fields dd { margin: 0; text-align: right; }.detail-fields .break { overflow-wrap: anywhere; }
.detail-description { white-space: pre-wrap; margin: 16px 0 0 !important; color: var(--ink); }
.url-list { overflow-wrap: anywhere; }.panel ul { padding-left: 20px; color: var(--muted); font-size: 13px; line-height: 1.6; }
form { display: grid; gap: 18px; max-width: 1080px; }.grid { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 16px; }.wide { grid-column: 1/-1; }
form label { display: flex; flex-direction: column; gap: 7px; font-size: 12px; color: var(--muted); } form label small { font-size: 11px; color: var(--muted-2); }
form input,form textarea,form select { width: 100%; box-sizing: border-box; border: 1px solid var(--line-strong); border-radius: 8px; padding: 10px 12px; background: var(--subtle); color: var(--ink); font: inherit; font-size: 13px; }
textarea { resize: vertical; } input:focus,textarea:focus,select:focus { outline: 2px solid var(--accent); outline-offset: 1px; }
details { margin-top: 20px; border-top: 1px solid var(--line); padding-top: 15px; } summary { cursor: pointer; color: var(--accent); font-size: 13px; }.optional { padding-top: 18px; }
.orgs { display: grid; grid-template-columns: repeat(2,minmax(0,1fr)); gap: 10px; }.orgs label { flex-direction: row; align-items: center; color: var(--ink); }.orgs input { width: auto; }.orgs small { color: var(--muted-2); }
form footer { display: flex; align-items: center; gap: 16px; } form footer button { border: 1px solid var(--accent); border-radius: 8px; background: var(--accent); color: var(--button-ink); padding: 10px 15px; cursor: pointer; }
button:disabled { opacity: .5; cursor: not-allowed; } form footer span { color: var(--muted); font-size: 12px; }.result small { overflow-wrap: anywhere; color: var(--muted); }
@media(max-width:900px) { .publish-metrics { grid-template-columns: repeat(2,minmax(0,1fr)); }.detail-columns { grid-template-columns: 1fr; } }
@media(max-width:680px) { .publish-page { padding: 20px 16px 60px; }.heading { align-items: flex-start; flex-direction: column; }.list-toolbar { flex-wrap: wrap; }.grid,.orgs { grid-template-columns: 1fr; }.wide { grid-column: auto; } form footer { align-items: flex-start; flex-direction: column; } }
</style>
