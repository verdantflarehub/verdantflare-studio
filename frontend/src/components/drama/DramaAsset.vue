<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ProjectReader, readerError, type DramaWorldCandidate, type WorldResult } from '../../platform/project-reader'
import DramaFile from './DramaFile.vue'
const props = defineProps<{ reader: ProjectReader; candidates: DramaWorldCandidate[] }>()
const route = useRoute(), router = useRouter(), dialog = ref<HTMLDialogElement>()
const assetReader = shallowRef<ProjectReader | null>(null), result = shallowRef<WorldResult | null>(null), error = ref(''), busy = ref(false)
const candidate = computed(() => props.candidates.find(c => c.key === route.query.publication))
let serial = 0
async function load() {
  const token = ++serial; assetReader.value?.dispose(); assetReader.value = null; result.value = null; error.value = ''; busy.value = false
  if (!route.query.publication) { dialog.value?.close(); return }
  dialog.value?.showModal(); busy.value = true
  try {
    const status = candidate.value?.registration_status
    if (status?.state !== 'committed') throw Error('本修订尚无此入库结果，请返回素材查看入库记录。')
    const r = new ProjectReader(props.reader.projectID)
    assetReader.value = r
    const actual = await r.loadAsset(status.result.asset_id, status.result.asset_version_id)
    if (serial === token) result.value = actual
  } catch (e) { if (serial === token) error.value = readerError(e) }
  finally { if (serial === token) busy.value = false }
}
function close() {
  dialog.value?.close()
  const back = router.options.history.state.back
  if (typeof back === 'string' && router.resolve(back).query.project === route.query.project && !router.resolve(back).query.publication) router.back()
  else void router.replace({ path: route.path, query: { ...route.query, publication: undefined } })
}
watch(() => [route.query.publication, props.reader, dialog.value], load, { immediate: true, flush: 'post' })
onBeforeUnmount(() => { serial++; assetReader.value?.dispose(); dialog.value?.close() })
</script>
<template>
  <dialog ref="dialog" aria-label="已入库的固定资产版本" @cancel.prevent="close" @click="($event.target === dialog) && close()">
    <header><h2>{{ result?.manifest.name || '已入库版本' }}</h2><button class="btn" @click="close">返回来源素材</button></header>
    <p v-if="busy" role="status">正在读取已入库版本…</p><p v-if="error" role="alert">{{ error }} <button class="btn" @click="load">重试</button></p>
    <template v-if="result && assetReader"><p class="muted">此处读取本次入库的固定版本。来源：{{ reader.project?.manifest.name }}。后续生成或换选不会改变本包。</p><div v-for="(f, i) in result.manifest.files" :key="f.file_id" class="asset-file"><DramaFile :reader="assetReader" :file-id="f.file_id" :label="f.role === 'publication' ? '入库范围说明' : `已入库文件 ${i + 1}`" /></div><button class="btn" @click="close">返回来源素材</button></template>
  </dialog>
</template>
<style scoped>
dialog { margin:auto; width:min(820px,calc(100vw - 32px)); max-height:calc(100dvh - 32px); overflow:auto; border:1px solid var(--line); border-radius:8px; padding:24px; background:var(--panel); color:var(--ink); }header { display:flex; align-items:center; justify-content:space-between; gap:12px; flex-wrap:wrap; }h2 { font-size:18px; }p { margin:16px 0; line-height:1.8; }.muted { color:var(--muted); }.asset-file { margin:18px 0; }@media(max-width:600px){dialog{padding:16px}}
</style>
