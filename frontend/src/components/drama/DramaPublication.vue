<script setup lang="ts">
import DramaPublishCurrent from './DramaPublishCurrent.vue'
import DramaAsset from './DramaAsset.vue'
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useHostStore } from '../../platform/hostStore'
import { callTool, uuidv7 } from '../../platform/mcp'
import { ProjectReader, readerError, type DramaWorldCandidate, type DramaGlobal, type DramaDeliverable, type WorldRegistration, type DramaVersion } from '../../platform/project-reader'
import { canonical, publicationPackage, publishPrepared, type PublicationIntent, type PublicationPackage } from '../../platform/drama-publication'
const props = defineProps<{ reader: ProjectReader; candidates: DramaWorldCandidate[]; currentOwner?: DramaGlobal | DramaDeliverable; currentVersion?: DramaVersion }>()
const route = useRoute(), router = useRouter(), host = useHostStore()
const candidate = shallowRef<DramaWorldCandidate | null>(null), pack = shallowRef<PublicationPackage | null>(null), owner = shallowRef<DramaGlobal | DramaDeliverable | null>(null)
const busy = ref(false), error = ref(''), message = ref(''), acknowledged = ref(false), pending = shallowRef<PublicationIntent | null>(null)
const actor = computed(() => [host.identity.organization_id, host.identity.user_id].every(Boolean) ? `${host.identity.organization_id}:${host.identity.user_id}` : '')
let serial = 0
const keyFor = (c: DramaWorldCandidate) => `studio.drama.publication:${actor.value}:${props.reader.projectID}:${c.key}`
async function prepare(c: DramaWorldCandidate) {
  const current = ++serial; candidate.value = c; pack.value = null; owner.value = null; acknowledged.value = false; busy.value = true; error.value = ''; message.value = ''; pending.value = null
  try {
    if (!actor.value) throw new Error('请等待登录身份确认后再入库。')
    const stored = localStorage.getItem(keyFor(c))
    if (stored) {
      const intent = JSON.parse(stored) as PublicationIntent
      if (intent.project_id !== props.reader.projectID || intent.candidate_key !== c.key || c.registration_request && canonical(c.registration_request) !== canonical(intent.request)) throw new Error('本机有另一项未完成入库操作，请使用原项目修订恢复。')
      pending.value = intent
      if (intent.origin_revision_id !== props.reader.project!.revision_id) throw new Error('请打开原准备修订，核对该次范围后恢复。')
    }
    let sourceReader = props.reader
    const sourceRevision = c.registration_request?.source_revision_id
    if (sourceRevision && sourceRevision !== props.reader.project!.revision_id) {
      sourceReader = new ProjectReader(props.reader.projectID, sourceRevision)
      try { await sourceReader.load() } catch (cause) { sourceReader.dispose(); throw cause }
    }
    try {
      const d = sourceReader.content!.ai_short_drama!
      const source = c.source.kind === 'global' ? d.globals.find(g => g.key === c.source.key) : d.deliverables.find(g => g.key === c.source.key)
      if (!source || !c.publication_record_file_id || c.review.state !== 'approved') throw new Error('请先通过 MCP 准备此版入库说明与审核记录。')
      const record = await sourceReader.document(c.publication_record_file_id)
      if (serial !== current) return
      const package_ = publicationPackage(record.text, c, source, sourceReader.project!)
      const request = pending.value?.request || c.registration_request
      if (request && canonical([...request.file_ids].sort()) !== canonical(package_.files)) throw new Error('原请求的文件范围与入库说明不一致。')
      pack.value = package_; owner.value = source
    } finally { if (sourceReader !== props.reader) sourceReader.dispose() }
  } catch (cause) { if (serial === current) error.value = readerError(cause) }
  finally { if (serial === current) busy.value = false }
}
function versionLabel(key: string) { const version = owner.value?.versions.find(v => v.key === key); const state = owner.value && 'states' in owner.value ? owner.value.states.find(s => s.key === version?.state_key)?.name : ''; return [version?.label || '固定版本', state].filter(Boolean).join(' · ') }
async function publish() {
  if (!candidate.value || !pack.value || !owner.value || !acknowledged.value || busy.value || !actor.value) return
  const c = candidate.value, project = props.reader.project!, storageKey = keyFor(c), identity = actor.value, current = serial
  busy.value = true; error.value = ''; message.value = ''
  try {
    const r: WorldRegistration = c.registration_request || { commit_id: uuidv7(), source_project_id: project.project_id, source_revision_id: project.revision_id, file_ids: pack.value.files, name: c.name, asset_type: 'kind' in owner.value ? owner.value.kind : 'video-project', subjects: ['name' in owner.value ? owner.value.name : owner.value.title], relation: 'produced_in', review_file_id: c.review.record_file_id! }
    if (canonical([...r.file_ids].sort()) !== canonical(pack.value.files)) throw new Error('准备发布的文件与入库说明不一致。')
    const intent: PublicationIntent = pending.value || { project_id: project.project_id, origin_revision_id: project.revision_id, candidate_key: c.key, request: r, prepare_commit_id: uuidv7(), association_commit_id: uuidv7() }
    const save = (value: PublicationIntent) => { if (actor.value !== identity) throw new Error('登录身份已变化，请恢复原账号后继续。'); localStorage.setItem(storageKey, JSON.stringify(value)); pending.value = value }
    save(intent) // If recovery information cannot be persisted, no service write starts.
    const completed = await publishPrepared(intent, {
      newID: uuidv7, save,
      phase: value => { if (serial === current) message.value = value },
      call: <T,>(name: string, args: Record<string, unknown>) => { if (actor.value !== identity) throw new Error('登录身份已变化。'); return callTool<T>(name, args, name === 'world.commit_status' ? undefined : project.project_id) },
      read: async revision => { if (actor.value !== identity) throw new Error('登录身份已变化。'); const reader = new ProjectReader(project.project_id, revision); try { await reader.load(); if (!reader.project || !reader.rawContent) throw new Error('项目内容暂不可读。'); return { project: reader.project, raw: reader.rawContent } } finally { reader.dispose() } }
    })
    localStorage.removeItem(storageKey)
    if (serial === current) { message.value = '固定版本已入库，项目关联已保存。'; await router.replace({ path: route.path, query: { ...route.query, revision: completed.revision_id } }) }
  } catch (cause) { if (serial === current) error.value = readerError(cause) }
  finally { if (serial === current) busy.value = false }
}
watch(() => [props.reader, props.candidates, actor.value], () => { serial++; candidate.value = null; pack.value = null; owner.value = null; pending.value = null; busy.value = false; error.value = ''; message.value = '' })
onBeforeUnmount(() => { serial++ })
</script>
<template>
  <section class="panel">
    <h2>入库到数字世界</h2><p class="muted">选择固定版本和文件范围，由你确认入库。保留来源项目，入库版本可以与项目选用不同。</p>
    <DramaPublishCurrent v-if="currentOwner" :reader="reader" :owner="currentOwner" :version="currentVersion" :candidates="candidates" /><p v-if="!candidates.length" class="muted">尚无入库记录。</p><DramaAsset :reader="reader" :candidates="candidates" />
    <div v-for="c in candidates" :key="c.key" class="row"><strong>{{ c.name }}</strong><p class="muted">{{ c.registration_status?.state === 'committed' ? '已入库 · 固定版本已关联项目' : c.review.state === 'approved' && c.publication_record_file_id ? '入库范围已准备' : '说明或审核尚未准备' }}</p><RouterLink v-if="c.registration_status?.state === 'committed'" class="btn" :to="{ path: route.path, query: { ...route.query, publication: c.key } }">查看已入库版本</RouterLink><button v-else class="btn" :disabled="busy || c.review.state !== 'approved' || !c.publication_record_file_id" @click="prepare(c)">{{ c.registration_request ? '核对／恢复入库' : '入库到数字世界' }}</button></div>
    <div v-if="candidate" class="confirmation"><h3>{{ candidate.name }}</h3><template v-if="pack && owner"><p>本次入库：</p><ul><li v-for="selection in pack.selections" :key="selection.version_key">{{ versionLabel(selection.version_key) }} · {{ selection.file_ids.length }} 个成果文件</li></ul><p class="muted">共 {{ pack.files.length }} 个文件，包含入库说明及明确附带资料。来源为当前准备时固定的项目修订。</p><p v-if="pending" class="muted">检测到原操作记录，将按原请求恢复，不重复发布。</p><label><input v-model="acknowledged" type="checkbox" :disabled="busy">确认上述版本和范围入库</label><button class="btn primary" :disabled="!acknowledged || busy || !!error" @click="publish">{{ busy ? '正在处理…' : pending || candidate.registration_request ? '按原操作恢复' : '确认入库' }}</button><button v-if="error && pending" class="btn" :disabled="busy || !acknowledged" @click="error = ''; publish()">{{ pending.published_asset_id ? '恢复项目关联' : '重试原入库操作' }}</button></template><RouterLink v-if="pending && !pack" class="btn" :to="{ path: route.path, query: { ...route.query, revision: pending.origin_revision_id } }">打开原准备修订</RouterLink><p v-if="message" role="status" class="muted">{{ message }}</p><p v-if="error" role="alert" class="error">{{ error }}</p></div>
  </section>
</template>
<style scoped>
.panel { min-width: 0; padding: 20px; background: var(--shell-panel); border: 1px solid var(--line); border-radius: 8px; }h2 { font-size: 15px; margin-bottom: 14px; }h3 { margin: 14px 0; font-size: 15px; }.muted { color: var(--muted); font-size: 13px; line-height: 1.85; }.row,.confirmation { margin-top: 14px; padding-top: 14px; border-top: 1px solid var(--line); }.error { color: var(--danger); }label { display: flex; gap: 8px; align-items: start; margin: 14px 0; }ul { margin: 12px 0; padding-left: 20px; }.btn { margin-right: 8px; }
</style>
