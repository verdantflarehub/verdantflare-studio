<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { ProjectReader, readerError } from '../../platform/project-reader'
import ProjectMedia from '../ProjectMedia.vue'
const props = defineProps<{ reader: ProjectReader; fileId: string; label: string }>()
const kind = ref<'image' | 'audio' | 'video' | 'file' | null>(null), error = ref('')
let serial = 0
async function inspect() {
  const current = ++serial; kind.value = null; error.value = ''
  try { const file = await props.reader.download(props.fileId); if (serial !== current) return; const mime = file.version.mime.split(';')[0]; kind.value = mime.startsWith('image/') ? 'image' : mime.startsWith('audio/') ? 'audio' : mime.startsWith('video/') ? 'video' : 'file' }
  catch (cause) { if (serial === current) error.value = readerError(cause) }
}
watch(() => [props.reader, props.fileId], inspect, { immediate: true })
onBeforeUnmount(() => { serial++ })
</script>
<template><div class="drama-file"><p class="muted">{{ label }}</p><ProjectMedia v-if="kind" :reader="reader" :file-id="fileId" :kind="kind" :label="label" /><p v-else-if="error" role="alert">{{ error }} <button class="btn" @click="inspect">重试</button></p><p v-else role="status">正在读取文件信息…</p></div></template>
<style scoped>.drama-file { min-width: 0; }.muted { color: var(--muted); margin-bottom: 8px; }</style>
