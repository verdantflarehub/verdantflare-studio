<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { ProjectReader, readerError } from '../../platform/project-reader'
import { documentSection } from '../../platform/drama-presentation'
import DramaProse from './DramaProse.vue'
import ProjectMedia from '../ProjectMedia.vue'
const props = defineProps<{ reader: ProjectReader; fileId?: string | null; title: string; anchor?: string; initiallyExpanded?: boolean }>()
const text = ref(''), expanded = ref(false), busy = ref(false), error = ref(''), structured = ref(false), loaded = ref(false)
let serial = 0
async function open() {
  expanded.value = !expanded.value
  if (!expanded.value || loaded.value || !props.fileId) return
  const current = ++serial; busy.value = true; error.value = ''
  try {
    const result = await props.reader.document(props.fileId)
    if (serial !== current) return
    structured.value = !['text/plain', 'text/markdown'].includes(result.mime)
    text.value = structured.value ? '' : documentSection(result.text, props.anchor)
    loaded.value = true
  } catch (cause) { if (serial === current) error.value = readerError(cause) }
  finally { if (serial === current) busy.value = false }
}
watch(() => [props.reader, props.fileId, props.anchor], () => { serial++; text.value = ''; expanded.value = false; loaded.value = false; busy.value = false; error.value = ''; structured.value = false; if (props.initiallyExpanded) void open() }, { immediate: true })
onBeforeUnmount(() => { serial++ })
</script>
<template>
  <div class="drama-document">
    <button v-if="fileId" class="btn" :aria-expanded="expanded" @click="open">{{ expanded ? '收起' : '查看' }}{{ title }}</button>
    <p v-else class="muted">{{ title }}尚未登记</p>
    <div v-if="expanded">
      <p v-if="busy" role="status" class="muted">正在读取固定版本…</p>
      <p v-else-if="error" role="alert" class="error">{{ error }}<button class="btn" @click="expanded = false; open()">重试</button></p>
      <template v-else-if="structured"><p class="muted">这份资料为工程记录，可下载查看；页面不展开内部字段。</p><ProjectMedia :reader="reader" :file-id="fileId" kind="file" :label="title" /></template>
      <DramaProse v-else class="document-text" :text="text" :drama="reader.content?.ai_short_drama" />
    </div>
  </div>
</template>
<style scoped>
.drama-document { min-width: 0; margin: 10px 0; }
.document-text { font: inherit; white-space: pre-wrap; overflow-wrap: anywhere; line-height: 1.9; margin-top: 16px; }
.muted { color: var(--muted); }.error { color: var(--danger); }
</style>
