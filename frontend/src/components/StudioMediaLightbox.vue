<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, shallowRef } from 'vue'
import { closeMediaPreview, subscribeMediaPreview, type MediaPreview } from '../platform/media-preview'
const current = shallowRef<MediaPreview | null>(null), dialog = ref<HTMLDialogElement>(), stage = ref<HTMLElement>(), picture = ref<HTMLImageElement>()
const scale = ref(1), x = ref(0), y = ref(0), dragging = ref(false), status = ref<'loading' | 'ready' | 'error'>('loading')
let start: { x: number; y: number; tx: number; ty: number } | null = null
let restoreFocus: HTMLElement | null = null, overflow: string | null = null, unsubscribe = () => {}
function activeControl(): HTMLElement | null {
  let element = document.activeElement
  while (element?.shadowRoot?.activeElement) element = element.shadowRoot.activeElement
  return element instanceof HTMLElement ? element : null
}
function close() { if (current.value) closeMediaPreview(current.value.owner) }
function unlock() {
  if (overflow !== null) { document.documentElement.style.overflow = overflow; overflow = null }
  if (restoreFocus?.isConnected) restoreFocus.focus({ preventScroll: true })
  restoreFocus = null
}
function reset() { scale.value = 1; x.value = y.value = 0; dragging.value = false; start = null }
function clamp() {
  if (!stage.value || !picture.value) return
  const maxX = Math.max(0, (picture.value.offsetWidth * scale.value - stage.value.clientWidth * .7) / 2)
  const maxY = Math.max(0, (picture.value.offsetHeight * scale.value - stage.value.clientHeight * .7) / 2)
  x.value = Math.max(-maxX, Math.min(maxX, x.value)); y.value = Math.max(-maxY, Math.min(maxY, y.value))
}
function zoom(next: number, event?: MouseEvent) {
  const rect = stage.value?.getBoundingClientRect()
  const ox = event && rect ? event.clientX - rect.left - rect.width / 2 : 0
  const oy = event && rect ? event.clientY - rect.top - rect.height / 2 : 0
  next = Math.max(.5, Math.min(5, next)); const ratio = next / scale.value
  x.value = ox - (ox - x.value) * ratio; y.value = oy - (oy - y.value) * ratio; scale.value = next; clamp()
}
function down(event: PointerEvent) {
  if (event.button !== 0 || status.value !== 'ready') return
  start = { x: event.clientX, y: event.clientY, tx: x.value, ty: y.value }
  stage.value?.setPointerCapture(event.pointerId)
}
function move(event: PointerEvent) {
  if (!start) return
  if (!dragging.value && Math.hypot(event.clientX - start.x, event.clientY - start.y) < 4) return
  dragging.value = true; x.value = start.tx + event.clientX - start.x; y.value = start.ty + event.clientY - start.y; clamp()
}
function up() { start = null; dragging.value = false }
onMounted(() => {
  unsubscribe = subscribeMediaPreview(async value => {
    current.value = value; reset(); status.value = 'loading'
    if (!value) { dialog.value?.close(); unlock(); return }
    if (overflow === null) {
      restoreFocus = activeControl()
      overflow = document.documentElement.style.overflow; document.documentElement.style.overflow = 'hidden'
    }
    await nextTick()
    if (current.value !== value || !dialog.value) return
    if (!dialog.value.open) dialog.value.showModal()
  })
  window.addEventListener('resize', clamp)
})
onBeforeUnmount(() => { close(); unsubscribe(); unlock(); window.removeEventListener('resize', clamp) })
</script>

<template>
  <Teleport to="body">
    <dialog ref="dialog" class="studio-media-lightbox" aria-label="图片预览" @cancel.prevent="close" @close="!dialog?.open && close()" @click="($event.target === dialog) && close()">
      <header><strong>{{ current?.title }}</strong><div class="media-tools">
        <button type="button" class="btn" aria-label="缩小图片" :disabled="status !== 'ready'" @click="zoom(scale / 1.25)">−</button>
        <output>{{ Math.round(scale * 100) }}%</output>
        <button type="button" class="btn" aria-label="放大图片" :disabled="status !== 'ready'" @click="zoom(scale * 1.25)">＋</button>
        <button type="button" class="btn" @click="reset">适配</button>
        <button type="button" class="btn" autofocus @click="close">关闭</button>
      </div></header>
      <div ref="stage" class="media-stage" :class="{ dragging }" @wheel.prevent.stop="zoom(scale * ($event.deltaY < 0 ? 1.15 : 1 / 1.15), $event)" @dblclick="scale > 1 ? reset() : zoom(2.5, $event)" @pointerdown="down" @pointermove="move" @pointerup="up" @pointercancel="up" @lostpointercapture="up">
        <p v-if="status === 'loading'" role="status">正在加载图片…</p>
        <p v-if="status === 'error'" role="alert">图片无法解码，请关闭后重试。</p>
        <img v-if="current && status !== 'error'" :key="current.url" ref="picture" :src="current.url" :alt="current.title" draggable="false" :style="{ visibility: status === 'ready' ? 'visible' : 'hidden', transform: `translate3d(${x}px, ${y}px, 0) scale(${scale})` }" @load="status = 'ready'; clamp()" @error="status = 'error'">
      </div>
    </dialog>
  </Teleport>
</template>

<style>
.studio-media-lightbox { position:fixed; inset:0; margin:auto; width:min(1120px,calc(100vw - 40px)); max-width:calc(100vw - 24px); max-height:calc(100dvh - 24px); padding:16px; border:.5px solid var(--line); border-radius:12px; color:var(--ink); background:var(--panel); box-shadow:var(--vf-shadow-card-hover); }
.studio-media-lightbox::backdrop { background:color-mix(in srgb,var(--ink) 55%,transparent); backdrop-filter:blur(6px); }
.studio-media-lightbox header { display:flex; align-items:center; justify-content:space-between; gap:16px; margin-bottom:12px; }
.studio-media-lightbox strong { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; font-size:14px; }
.studio-media-lightbox .media-tools { display:flex; align-items:center; gap:8px; flex-shrink:0; }
.studio-media-lightbox output { min-width:48px; text-align:center; color:var(--accent); font-variant-numeric:tabular-nums; }
.studio-media-lightbox .media-stage { position:relative; display:flex; align-items:center; justify-content:center; height:min(76dvh,800px); overflow:hidden; border-radius:8px; background:var(--subtle); cursor:grab; touch-action:none; user-select:none; }
.studio-media-lightbox .media-stage.dragging { cursor:grabbing; }
.studio-media-lightbox .media-stage img { display:block; width:auto; height:auto; max-width:100%; max-height:100%; object-fit:contain; pointer-events:none; transform-origin:center; }
.studio-media-lightbox .media-stage p { position:absolute; color:var(--muted); }
@media(max-width:600px) { .studio-media-lightbox { padding:10px; }.studio-media-lightbox header { flex-wrap:wrap; }.studio-media-lightbox .media-tools { width:100%; justify-content:flex-end; } }
</style>
