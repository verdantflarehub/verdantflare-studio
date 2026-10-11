<script setup lang="ts">
import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ProjectReader, type DramaVersion, type DramaSelection } from '../../platform/project-reader'
import DramaProse from './DramaProse.vue'
import DramaDocument from './DramaDocument.vue'
import DramaFile from './DramaFile.vue'
const props = defineProps<{ reader: ProjectReader; versions: DramaVersion[]; selections?: DramaSelection[]; selectedKey?: string | null }>()
const route = useRoute(), router = useRouter()
const purposeName: Record<string, string> = { whitebox: '白模工程', lighting: '环境与灯光方案', storyboard: '分镜图', keyframe: '关键帧', segment_video: '片段视频', shot_video: '整镜视频', video: '视频', image: '图像', preview: '预览', audio: '音频', project: '可编辑工程', subtitle: '字幕' }
const selectedKeys = computed(() => new Set([props.selectedKey, ...(props.selections || []).map(s => s.version_key)].filter(Boolean)))
const version = computed(() => route.query.version ? props.versions.find(v => v.key === route.query.version) : props.versions.find(v => selectedKeys.value.has(v.key)) || props.versions.at(-1))
const comparable = computed(() => props.versions.filter(v => v.key !== version.value?.key && v.state_key === version.value?.state_key && v.purpose === version.value?.purpose && v.segment_key === version.value?.segment_key))
const comparison = computed(() => comparable.value.find(v => v.key === route.query.compare))
const displayed = computed(() => [version.value, comparison.value].filter((v): v is DramaVersion => !!v))
const reviewName = (state: string) => ({ unreviewed: '未审核', in_review: '审核中', approved: '此版已有审核记录', rejected: '需修改' }[state] || '未审核')
function select(field: string, value: string) { void router.replace({ path: route.path, query: { ...route.query, [field]: value || undefined, ...(field === 'version' ? { compare: undefined } : {}) } }) }
</script>
<template>
  <section class="panel">
    <h2>制作版本与每轮修改</h2><slot />
    <p v-if="!versions.length" class="muted">尚未登记制作版本。已有文件保留原记录，不自动视为第一版或已批准成果。</p>
    <template v-else>
      <p v-if="!version" class="muted">当前修订或状态没有指定的版本，请重新选择。</p>
      <div class="toolbar"><label>当前查看版本<select :value="version?.key" @change="select('version', ($event.target as HTMLSelectElement).value)"><option v-for="v in versions" :key="v.key" :value="v.key">{{ v.label }}{{ selectedKeys.has(v.key) ? ' · 项目选用' : '' }}{{ v.purpose ? ' · ' + (purposeName[v.purpose] || '制作成果') : '' }}</option></select></label><label>与同用途版本比较<select :value="comparison?.key || ''" @change="select('compare', ($event.target as HTMLSelectElement).value)"><option value="">不比较</option><option v-for="v in comparable" :key="v.key" :value="v.key">{{ v.label }}</option></select></label></div>
      <p class="muted">切换查看不会更改项目选用。返修、换选和保存由 AI 经 MCP 执行。</p>
      <div class="version-grid" :class="{ comparing: comparison }"><article v-for="v in displayed" :key="v.key"><h3>{{ v.label }} <span class="muted">{{ selectedKeys.has(v.key) ? '项目选用' : '未选用候选' }}</span></h3><div class="outputs"><DramaFile v-for="f in v.file_refs" :key="f.file_id" :reader="reader" :file-id="f.file_id" :label="`${v.label} · ${purposeName[f.purpose] || '成果文件'}`" /></div><DramaProse :text="v.change_note" :drama="reader.content?.ai_short_drama" /><RouterLink class="basis-link" :to="{ path: route.path, query: { ...route.query, revision: v.basis_revision_id, version: undefined, compare: undefined } }">查看本版制作依据 →</RouterLink><p class="muted">{{ reviewName(v.review.state) }}{{ v.parent_version_key ? ' · 基于 ' + (versions.find(p => p.key === v.parent_version_key)?.label || '此前版本') : '' }}</p><p v-if="(v.purpose === 'whitebox' || v.file_refs.some(f => f.purpose === 'whitebox')) && !v.file_refs.some(f => f.purpose === 'preview' || f.purpose === 'image')" class="muted">本版预览待生成；旧版预览不会代替本版。</p><DramaDocument v-if="v.change_record_file_id" :reader="reader" :file-id="v.change_record_file_id" title="完整修改记录" /><DramaDocument v-if="v.lighting_record_file_id" :reader="reader" :file-id="v.lighting_record_file_id" title="场景状态与灯光方案" /><DramaDocument v-if="v.spatial_record_file_id" :reader="reader" :file-id="v.spatial_record_file_id" title="白模空间与相机说明" /><DramaDocument v-if="v.composition_file_id" :reader="reader" :file-id="v.composition_file_id" title="固定剪辑来源" /><DramaDocument v-if="v.review.record_file_id" :reader="reader" :file-id="v.review.record_file_id" title="此版审核记录" /><DramaDocument v-for="(id, index) in v.generation_record_file_ids" :key="id" :reader="reader" :file-id="id" :title="`制作记录 ${index + 1}`" /></article></div>
      <details class="version-history"><summary>完整版本与修改历史 · {{ versions.length }} 版</summary><div v-for="v in versions" :key="v.key" class="history-row"><RouterLink :to="{ path: route.path, query: { ...route.query, version: v.key, compare: undefined } }"><strong>{{ v.label }}</strong> · {{ selectedKeys.has(v.key) ? '项目选用' : '未选用' }} · {{ reviewName(v.review.state) }} →</RouterLink><DramaProse :text="v.change_note" :drama="reader.content?.ai_short_drama" /></div></details>
    </template>
  </section>
</template>
<style scoped>
.version-grid :deep(.image-open img) { max-height: 570px; object-fit: contain; }
.version-grid.comparing :deep(.image-open img) { height: 260px; object-fit: contain; }
.version-history { border-top: 1px solid var(--line); margin-top: 22px; padding-top: 16px; font-size: 13px; }
summary { cursor: pointer; color: var(--muted); }.history-row { border-top: 1px solid var(--line); padding-top: 12px; margin-top: 14px; }.history-row a { color: var(--accent); display: block; margin-bottom: 8px; }

.panel { background: var(--shell-panel); border: 1px solid var(--line); border-radius: 8px; padding: 20px; min-width: 0; }h2 { font-size: 15px; margin-bottom: 14px; }h3 { font-size: 15px; margin: 18px 0 10px; }.muted { color: var(--muted); font-size: 13px; line-height: 1.85; }.prose { white-space: pre-wrap; line-height: 1.85; }.toolbar { margin-top:16px; display: flex; flex-wrap: wrap; gap: 16px; margin-bottom: 12px; }label { display: grid; gap: 6px; max-width: 100%; }select { max-width: 100%; min-width: 180px; border: 1px solid var(--line); border-radius: 8px; padding: 9px; background: var(--panel); color: var(--ink); }.version-grid, .outputs { display: grid; gap: 18px; min-width: 0; }.version-grid > *, .outputs > * { min-width: 0; }.comparing { grid-template-columns: repeat(2, minmax(0, 1fr)); }.outputs { margin: 18px 0; }@media(max-width:800px) { .comparing { grid-template-columns: minmax(0, 1fr); } }
</style>
