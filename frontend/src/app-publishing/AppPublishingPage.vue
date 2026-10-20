<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useRouter } from 'vue-router'
import { useHostStore } from '../platform/hostStore'

type Organization = { id: string; name: string }
const router = useRouter()
const host = useHostStore()
const canOpen = computed(() => Boolean(host.connected && host.identity.roles?.includes('admin') && host.identity.scopes?.includes('app:manage')))
const organizations = ref<Organization[]>([])
const selectedOrganizations = ref<string[]>([])
const busy = ref(false)
const message = ref('')
const error = ref('')
const published = ref<{ appId: string; version: string; catalogETag: string } | null>(null)
const form = reactive({
  appId: '', name: '', groupId: 'desktop', category: '', summary: '', description: '', developer: '',
  brand: '', iconRef: '', publicIconUrl: '', sortOrder: 1000,
  screenshots: '', highlights: '', whatsNew: '', permissions: '', websiteUrl: '', docsUrl: '', sourceUrl: '', license: '', languages: '', platforms: '',
  version: '', chartVersion: '', ociRef: '', manifestDigest: '', packageDigest: '', sourceRevision: '', releaseNotes: '', upstreamVersion: '', publisher: '', versionSourceUrl: '', licenseId: '', licenseUrl: '',
  expectedRevision: 0,
})

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
async function loadOrganizations() {
  try {
    const items = await callTool('app_publish.organizations', {})
    organizations.value = (Array.isArray(items) ? items : []).filter(item => item.id && item.name)
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '无法读取组织' }
}

async function loadExisting() {
  if (!form.appId.trim() || busy.value) return
  busy.value = true; error.value = ''; message.value = ''
  try {
    const managed = await callTool('app_publish.get', { app_id: form.appId.trim() })
    const app = managed.app
    if (!app) throw new Error('应用资料不可用')
    form.expectedRevision = app.revision ?? 0
    form.name = app.name ?? ''; form.groupId = app.groupId ?? 'desktop'; form.category = app.category ?? ''
    form.summary = app.summary ?? ''; form.description = app.description ?? ''; form.developer = app.developer ?? ''
    form.brand = app.brand ?? ''; form.iconRef = app.iconRef ?? ''; form.publicIconUrl = app.publicIconUrl ?? ''; form.sortOrder = app.sortOrder ?? 1000
    form.screenshots = (app.showcase?.screenshots ?? []).join('\n'); form.highlights = (app.showcase?.highlights ?? []).join('\n')
    form.whatsNew = app.showcase?.whatsNew ?? ''; form.permissions = (app.showcase?.permissions ?? []).join('\n')
    form.websiteUrl = app.showcase?.websiteUrl ?? ''; form.docsUrl = app.showcase?.docsUrl ?? ''; form.sourceUrl = app.showcase?.sourceUrl ?? ''
    form.license = app.showcase?.license ?? ''; form.languages = app.showcase?.languages ?? ''; form.platforms = app.showcase?.platforms ?? ''
    message.value = `已读取应用修订 ${form.expectedRevision}；请核对后发布。`
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '读取失败' }
  finally { busy.value = false }
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
  } catch (cause) { error.value = cause instanceof Error ? cause.message : '发布失败；请先回读应用状态，避免重复提交。' }
  finally { busy.value = false }
}
onMounted(async () => { if (!host.connected) await host.init(); if (canOpen.value) await loadOrganizations() })
</script>

<template>
  <main class="publish-page">
    <button class="back" @click="router.push('/market')">← 返回应用中心</button>
    <header><span class="eyebrow">系统应用 · 管理员工具</span><h1>应用发布</h1><p>填写 Hub 目录资料，选择已发行的固定 OCI Chart，发布到 Station 目录。发布后由用户自行安装。</p></header>
    <p v-if="!canOpen" class="feedback error">需要 Station 管理员身份；Control 还会核对绑定的应用运营角色。</p>
    <template v-else>
    <p class="notice">第一版接收已由 Market Actions 发行的 OCI Chart。Chart 源码上传与自动发行尚未接入；临时 .tgz 预检不能作为可安装制品。</p>
    <p v-if="error" class="feedback error" role="alert">{{ error }}</p>
    <p v-if="message" class="feedback" role="status">{{ message }}</p>
    <form @submit.prevent="submit">
      <section class="panel"><h2>1. 应用目录资料</h2><p>与 Hub 发布字段一致；标 * 的字段为发布必填。</p>
        <div class="grid"><label>应用 ID * <input v-model="form.appId" required pattern="[a-z0-9][a-z0-9-]{0,61}[a-z0-9]" placeholder="my-app" /></label><button type="button" class="load" :disabled="busy" @click="loadExisting">读取已有资料</button>
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
  </main>
</template>

<style scoped>
.publish-page{max-width:1080px;margin:0 auto;padding:28px 32px 80px;color:var(--ink)}
.back{border:0;background:none;color:var(--accent);cursor:pointer;padding:0 0 24px}
header{margin-bottom:20px} h1{font-size:32px;margin:8px 0} h2{font-size:17px;margin:0 0 8px} p{color:var(--muted);line-height:1.55}.eyebrow{font-size:11px;color:var(--accent);letter-spacing:.08em}
.notice,.feedback,.result{border:1px solid var(--line);border-radius:12px;background:var(--panel);padding:15px 18px;margin:16px 0}.notice{border-color:var(--amber)}.feedback{color:var(--accent)}.error{color:var(--danger)}
form{display:grid;gap:18px}.panel{border:1px solid var(--line);border-radius:14px;background:var(--panel);padding:22px}.panel>p{margin:0 0 20px;font-size:13px}.grid{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:16px}.wide{grid-column:1/-1}
label{display:flex;flex-direction:column;gap:7px;font-size:12px;color:var(--muted)}label small{font-size:11px;color:var(--muted-2)}input,textarea,select{width:100%;box-sizing:border-box;border:1px solid var(--line-strong);border-radius:8px;padding:10px 12px;background:var(--subtle);color:var(--ink);font:inherit;font-size:13px}textarea{resize:vertical}input:focus,textarea:focus,select:focus{outline:2px solid var(--accent);outline-offset:1px}
details{margin-top:20px;border-top:1px solid var(--line);padding-top:15px}summary{cursor:pointer;color:var(--accent);font-size:13px}.optional{padding-top:18px}.orgs{display:grid;grid-template-columns:repeat(2,minmax(0,1fr));gap:10px}.orgs label{flex-direction:row;align-items:center;color:var(--ink)}.orgs input{width:auto}.orgs small{color:var(--muted-2)}
footer{display:flex;align-items:center;gap:16px}footer button,.load{border:1px solid var(--accent);border-radius:8px;background:var(--accent);color:var(--button-ink);padding:10px 15px;cursor:pointer}.load{align-self:end;background:transparent;color:var(--accent)}button:disabled{opacity:.5;cursor:not-allowed}footer span{color:var(--muted);font-size:12px}.result small{overflow-wrap:anywhere;color:var(--muted)}
@media(max-width:680px){.publish-page{padding:20px 16px 60px}.grid,.orgs{grid-template-columns:1fr}.wide{grid-column:auto}footer{align-items:flex-start;flex-direction:column}}
</style>
