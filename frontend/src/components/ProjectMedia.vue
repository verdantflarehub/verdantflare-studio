<script setup lang="ts">
import { onBeforeUnmount, ref, watch } from 'vue'
import { ProjectReader, readerError } from '../platform/project-reader'
const props = withDefaults(defineProps<{ reader: ProjectReader; fileId?: string | null; kind: 'image' | 'audio' | 'video' | 'file'; label: string; thumbnail?: boolean }>(), { thumbnail: false })
const source = ref(''), error = ref(''), busy = ref(false), downloadPath = ref('')
const viewer = ref<HTMLDialogElement | null>(null)
let generation = 0
async function load() {
  const current = ++generation
  source.value = ''; error.value = ''; downloadPath.value = ''; busy.value = true
  try {
    if (!props.fileId) return
    if (props.kind === 'file') {
      const read = await props.reader.download(props.fileId)
      if (current === generation) downloadPath.value = read.content_path
    } else {
      const result = await props.reader.media(props.fileId, props.kind)
      if (current === generation) source.value = result
    }
  } catch (cause) { if (current === generation) error.value = readerError(cause) }
  finally { if (current === generation) busy.value = false }
}
async function prepareDownload() {
  if (!props.fileId) return
  const current = generation
  try { const read = await props.reader.download(props.fileId); if (current === generation) downloadPath.value = read.content_path }
  catch (cause) { if (current === generation) error.value = readerError(cause) }
}
watch(() => [props.reader, props.fileId, props.kind], () => {
  generation++; source.value = ''; error.value = ''; downloadPath.value = ''; busy.value = false
  viewer.value?.close()
  if (props.kind === 'image' && props.fileId) void load()
}, { immediate: true })
onBeforeUnmount(() => { generation++; viewer.value?.close() })
</script>

<template>
  <div class="project-media" :class="[kind, { thumbnail }]" :aria-label="label">
    <template v-if="source">
      <img v-if="kind === 'image' && thumbnail" :src="source" :alt="label" @error="error = '图片解码失败。'; source = ''">
      <button v-else-if="kind === 'image'" class="image-open" :aria-label="`放大${label}`" @click="viewer?.showModal()"><img :src="source" :alt="label" @error="error = '图片解码失败。'; source = ''"></button>
      <audio v-else-if="kind === 'audio'" :src="source" controls preload="metadata" :aria-label="label" @error="error = '浏览器无法播放此音频。'; source = ''" />
      <video v-else :src="source" controls playsinline preload="metadata" :aria-label="label" @error="error = '浏览器无法播放此视频。'; source = ''" />
    </template>
    <div v-else class="media-state">
      <span v-if="!fileId">{{ kind === 'image' ? '未添加图片' : kind === 'audio' ? '未添加音频' : kind === 'video' ? '未添加视频' : '未添加文件' }}</span>
      <span v-else-if="busy" role="status">正在读取…</span>
      <template v-else-if="error"><span role="alert">{{ error }}</span><button v-if="!thumbnail" class="btn" @click="load">重试</button><button v-if="!thumbnail && !downloadPath" class="btn" @click="prepareDownload">下载查看</button></template>
      <button v-else-if="!downloadPath" class="btn" @click="load">{{ kind === 'file' ? `准备下载 ${label}` : kind === 'audio' ? '加载音频' : '加载视频' }}</button>
      <a v-if="downloadPath" class="btn" :href="downloadPath" download>{{ kind === 'file' ? `下载 ${label}` : '下载文件' }}</a>
    </div>
    <dialog v-if="kind === 'image' && !thumbnail" ref="viewer" :aria-label="label" @click="($event.target === viewer) && viewer?.close()">
      <button class="btn close" autofocus aria-label="关闭图片" @click="viewer?.close()">关闭</button>
      <img :src="source || undefined" :alt="label">
    </dialog>
  </div>
</template>

<style scoped>
.project-media { min-width: 0; }
.image, .video { border-radius: 12px; background: var(--subtle); overflow: hidden; }
.image-open { display: block; width: 100%; cursor: zoom-in; }
img { display: block; width: 100%; height: auto; }
.thumbnail { aspect-ratio: 16 / 10; }
.thumbnail img { width: 100%; height: 100%; object-fit: cover; object-position: center top; }
.media-state { min-height: 72px; display: flex; align-items: center; justify-content: center; gap: 10px; flex-wrap: wrap; padding: 16px; color: var(--muted); overflow-wrap: anywhere; }
.image .media-state, .video .media-state { min-height: 180px; height: 100%; }
audio, video { display: block; width: 100%; }
video { max-height: 70vh; }
dialog { margin: auto; border: 1px solid var(--line); background: var(--panel); color: var(--ink); border-radius: 12px; max-width: 95vw; max-height: 95vh; padding: 12px; }
dialog::backdrop { background: var(--aside); }
dialog img { width: auto; max-width: 90vw; max-height: 80vh; object-fit: contain; }
.close { display: block; margin: 0 0 10px auto; }
</style>
