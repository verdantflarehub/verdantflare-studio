<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useHostStore } from '../../platform/hostStore'
import { callTool, uuidv7 } from '../../platform/mcp'
import { ProjectReader, readerError, type DramaGlobal, type DramaDeliverable, type DramaVersion, type DramaWorldCandidate } from '../../platform/project-reader'
import { createPagePublication, publicationSupport, publishFromPage, type PagePublication } from '../../platform/drama-publication-draft'
const props = defineProps<{ reader: ProjectReader; owner: DramaGlobal | DramaDeliverable; version?: DramaVersion; candidates: DramaWorldCandidate[] }>()
const route = useRoute(), router = useRouter(), host = useHostStore()
const actor = computed(() => host.identity.user_id && host.identity.organization_id ? `${host.identity.organization_id}:${host.identity.user_id}` : '')
const dialog = ref<HTMLDialogElement>(), name = ref(''), ids = ref<string[]>([]), acknowledged = ref(false), target = ref(''), busy = ref(false), error = ref(''), phase = ref('')
const pending = shallowRef<PagePublication | null>(null)
let alive = true
const storageKey = computed(() => `studio.drama.page-publication:${actor.value}:${props.reader.projectID}:${props.owner.key}`)
const ownerName = computed(() => 'name' in props.owner ? props.owner.name : props.owner.title)
const stateName = computed(() => 'states' in props.owner ? props.owner.states.find(s => s.key === (pending.value?.version.state_key || props.version?.state_key))?.name : '')
const choices = computed(() => props.version?.file_refs.map((f, i) => ({ ...f, label: `${purpose(f.purpose)} ${i + 1}` })) || [])
const targets = computed(() => [...new Map(props.candidates.filter(c => c.registration_status?.state === 'committed').map(c => { const r = c.registration_status!; if (r.state !== 'committed') throw Error(); return [r.result.asset_id, { id: r.result.asset_id, version: r.result.asset_version_id, name: r.result.manifest.name }] })).values()])
const snapshot = () => ({ project: props.reader.project!, raw: props.reader.rawContent! })
const support = computed(() => { try { return props.version ? publicationSupport(props.owner, props.version, ids.value, snapshot()) : [] } catch { return [] } })
function purpose(value: string) { return ({ image: '图像', preview: '预览', text_layer: '后期字层', audio: '音频', video: '视频', whitebox: '白模工程', lighting: '灯光方案', subtitle: '字幕', project: '可编辑工程' } as Record<string, string>)[value] || '成果文件' }
function readPending() { error.value = ''; pending.value = null; if (!actor.value) return; try { const raw = localStorage.getItem(storageKey.value); if (raw) { const p = JSON.parse(raw) as PagePublication; if (p.project_id !== props.reader.projectID || p.source.key !== props.owner.key || p.actor !== actor.value) throw Error('恢复记录与当前项目或登录身份不符。'); pending.value = p } } catch (e) { error.value = readerError(e) } }
function open() { acknowledged.value = false; name.value = pending.value?.name || `${ownerName.value} · ${stateName.value || '成果'} · ${props.version?.label || ''}`; ids.value = props.version?.file_refs.map(f => f.file_id) || []; target.value = ''; phase.value = ''; void router.push({ path: route.path, query: { ...route.query, publish: 'current' } }) }
function close() {
  if (busy.value) return
  dialog.value?.close()
  const previous = router.options.history.state.back
  if (typeof previous === 'string' && router.resolve(previous).query.project === route.query.project && !router.resolve(previous).query.publish) router.back()
  else void router.replace({ path: route.path, query: { ...route.query, publish: undefined } })
}
async function submit() {
  if (busy.value || !acknowledged.value || !actor.value || !props.version && !pending.value) return
  const key = storageKey.value, identity = actor.value, projectID = props.reader.projectID
  busy.value = true; error.value = ''
  try {
    if (!pending.value && localStorage.getItem(key)) { readPending(); throw Error('另一个页面已保存本素材的入库操作，请核对后按原操作继续。') }
    const chosen = targets.value.find(t => t.id === target.value)
    const draft = pending.value || createPagePublication(snapshot(), props.owner, props.version!, ids.value, name.value, identity, new Date().toISOString(), uuidv7, chosen ? { asset_id: chosen.id, expected_asset_version_id: chosen.version } : undefined)
    const saveDraft = (value: PagePublication) => { if (actor.value !== identity) throw Error('登录身份已变化，请恢复原账号后继续。'); localStorage.setItem(key, JSON.stringify(value)); pending.value = value }
    saveDraft(draft)
    const result = await publishFromPage(draft, {
      newID: uuidv7, saveDraft, save: intent => { draft.publication = intent; saveDraft(draft) }, phase: value => { phase.value = value },
      call: <T,>(tool: string, args: Record<string, unknown>) => { if (actor.value !== identity) throw Error('登录身份已变化。'); return callTool<T>(tool, args, projectID) },
      read: async revision => { if (actor.value !== identity) throw Error('登录身份已变化。'); const r = new ProjectReader(projectID, revision); try { await r.load(); if (!r.rawContent) throw Error('项目内容不可读。'); return { project: r.project!, raw: r.rawContent } } finally { r.dispose() } }
    })
    localStorage.removeItem(key); pending.value = null
    if (alive) { dialog.value?.close(); await router.replace({ path: route.path, query: { ...route.query, revision: result.revision_id, publish: undefined, publication: draft.candidate_key } }) }
  } catch (e) { error.value = readerError(e) }
  finally { busy.value = false }
}
watch(() => [props.reader, props.owner.key, actor.value], readPending, { immediate: true })
watch(() => [route.query.publish, dialog.value], () => { if (route.query.publish === 'current') { if (!name.value) { name.value = pending.value?.name || `${ownerName.value} · ${stateName.value || '成果'} · ${props.version?.label || ''}`; ids.value = props.version?.file_refs.map(f => f.file_id) || [] } dialog.value?.showModal() } else dialog.value?.close() }, { flush: 'post', immediate: true })
watch(() => [props.version?.key, route.query.state], () => { acknowledged.value = false })
onBeforeUnmount(() => { alive = false; dialog.value?.close() })
</script>
<template>
  <div class="page-publication">
    <p v-if="version" class="muted">当前查看：{{ stateName }} · {{ version.label }}</p>
    <button class="btn primary" :disabled="busy || !actor || (!version && !pending)" @click="open">{{ pending ? '恢复本次入库' : '将当前版本入库' }}</button>
    <p v-if="!version && !pending" class="muted">先选择一个已有制作版本，再确认入库。</p>
    <p v-if="error && route.query.publish !== 'current'" role="alert">{{ error }}</p>
    <dialog ref="dialog" aria-label="确认当前版本入库" @cancel.prevent="close" @click="($event.target === dialog) && close()">
      <header><h2>入库到数字世界</h2><button class="btn" :disabled="busy" @click="close">返回素材版本</button></header>
      <p>{{ ownerName }} · {{ pending?.version.label || version?.label }}<template v-if="stateName"> · {{ stateName }}</template></p>
      <p class="muted">只保存本次确认的固定版本与文件。素材制作审核、项目选用和其他版本保留原记录。</p>
      <template v-if="pending"><p>本次范围已固定：{{ pending.name }} · {{ pending.files.length }} 个成果文件。</p><p class="muted">{{ pending.publication?.published_asset_id ? '已入库，项目关联待同步。恢复将核对原结果并补关联。' : '将使用原操作记录继续，不另建重复资产。' }}</p></template>
      <template v-else>
        <label>资产名称<input v-model="name" :disabled="busy" maxlength="256"></label>
        <label>入库目标<select v-model="target" :disabled="busy"><option value="">创建新数字资产</option><option v-for="t in targets" :key="t.id" :value="t.id">更新已有资产：{{ t.name }}</option></select></label>
        <p v-if="target" class="muted">为选定资产新增一个版本；本次只包含下列文件，不自动继承原资产文件。</p>
        <fieldset :disabled="busy"><legend>本次成果文件</legend><label v-for="f in choices" :key="f.file_id" class="check"><input v-model="ids" type="checkbox" :value="f.file_id">{{ f.label }}</label></fieldset>
        <p v-if="support.length" class="muted">另含 {{ support.length }} 份必需的空间／灯光说明。</p>
        <p class="muted">入库说明随成果保存；私人参考、制作清单和其他候选不自动入库。</p>
        <p v-if="version?.review.state === 'rejected'" class="warning">此制作版本原标记为“需修改”。请确认它是否适合本次入库用途。</p>
      </template>
      <label class="check"><input v-model="acknowledged" type="checkbox" :disabled="busy">我已核对并批准本次入库范围</label>
      <p v-if="phase" role="status">{{ phase }}</p><p v-if="error" role="alert" class="error">{{ error }}</p>
      <footer><button class="btn" :disabled="busy" @click="close">取消并返回</button><button class="btn primary" :disabled="busy || !acknowledged || !pending && (!ids.length || !name.trim())" @click="submit">{{ busy ? '正在处理…' : pending ? '按原操作继续' : '确认入库' }}</button></footer>
    </dialog>
  </div>
</template>
<style scoped>
.muted { color:var(--muted); font-size:13px; line-height:1.85; }.error,.warning { color:var(--danger); }dialog { margin:auto; width:min(640px,calc(100vw - 32px)); max-height:calc(100dvh - 32px); overflow:auto; border:1px solid var(--line); border-radius:8px; padding:24px; background:var(--panel); color:var(--ink); }header,footer { display:flex; flex-wrap:wrap; gap:12px; justify-content:space-between; align-items:center; }h2 { font-size:18px; }p { margin:14px 0; line-height:1.8; }label { display:grid; gap:8px; margin:14px 0; }label.check { display:flex; align-items:start; gap:10px; }input:not([type=checkbox]),select { width:100%; min-width:0; border:1px solid var(--line); border-radius:6px; padding:10px; background:var(--panel); color:var(--ink); font:inherit; }fieldset { border:1px solid var(--line); padding:12px; margin:16px 0; }footer { margin-top:20px; }button:disabled { opacity:.55; }@media(max-width:600px){dialog { padding:16px; }}
</style>
