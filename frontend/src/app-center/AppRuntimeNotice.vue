<script setup lang="ts">
import { computed } from 'vue'
import { statusLabels } from './labels'
import type { AppCenterCard } from './schema.generated'

// Opening no longer waits for a ready service; the App page explains a stopped
// or unknown service here and offers start when the user may manage it.
const props = defineProps<{ card: AppCenterCard | null }>()
const emit = defineEmits<{ start: [actionId: string] }>()
const visible = computed(() => {
  const card = props.card
  return !!card?.entry?.requiresServiceReady && !(card.service.quality === 'known' && card.service.state === 'ready')
})
const start = computed(() => props.card?.actions.find(action => action.kind === 'start' && action.enabled) ?? null)
const text = computed(() => {
  const card = props.card
  if (!card) return ''
  if (card.service.quality !== 'known' || card.service.state === 'unknown') return '应用运行状态暂未确认，部分功能可能暂时不可用。'
  return `应用当前未运行（${statusLabels[card.statusCode] ?? '状态待确认'}），部分功能暂时不可用。`
})
</script>

<template>
  <div v-if="visible" class="runtime-notice" role="status">
    <span>{{ text }}</span>
    <button v-if="start" class="btn" type="button" @click="emit('start', start.id)">启动</button>
  </div>
</template>

<style scoped>
.runtime-notice { display:flex; align-items:center; justify-content:space-between; gap:12px; margin:0 0 12px; padding:10px 14px; border:1px solid var(--line); border-radius:8px; background:var(--panel); color:var(--muted); font-size:14px; }
</style>
