<script setup lang="ts">
import { computed } from 'vue'
import type { AppCenterCard } from './schema.generated'
import { actionLabels, statusLabels, reasonLabel } from './labels'
const props = defineProps<{ card: AppCenterCard; stale: boolean }>()
const emit = defineEmits<{ action: [card: AppCenterCard, actionId: string] }>()
const primary = computed(() => props.card.actions.find(action => action.id === props.card.primaryActionId))
const local = computed(() => ['detail', 'progress', 'refresh'].includes(primary.value?.kind ?? ''))
const disabled = computed(() => !primary.value?.enabled || props.stale && !local.value)
const hint = computed(() => props.stale && !local.value ? '当前信息已过期，请刷新' : reasonLabel(primary.value?.reasonCode ?? null))
</script>

<template>
  <article class="app-card">
    <div class="card-top">
      <span class="app-icon appcenter-icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5"><rect x="3" y="3" width="7" height="7" rx="1.5" /><rect x="14" y="3" width="7" height="7" rx="1.5" /><rect x="3" y="14" width="7" height="7" rx="1.5" /><rect x="14" y="14" width="7" height="7" rx="1.5" /></svg></span>
      <div><button class="app-title" @click="emit('action', card, 'detail')">{{ card.display.name }}</button>
        <div class="brand-version"><span v-if="card.display.publisherId">{{ card.display.publisherId }}</span><span v-if="card.installedVersion || card.candidateVersion">{{ card.installedVersion ?? card.candidateVersion }}</span></div>
      </div>
    </div>
    <p class="desc">{{ card.display.description || '暂无应用说明' }}</p>
    <div v-if="card.display.resourceTags.length" class="appcenter-tags"><span v-for="tag in card.display.resourceTags" :key="tag.kind">{{ tag.kind.toUpperCase() }} {{ tag.value }} {{ tag.unit }}</span></div>
    <div class="card-foot">
      <span class="status" :class="{ running: card.statusCode === 'READY' && !stale }"><i class="dot" :class="{ on: card.statusCode === 'READY' && !stale }" />{{ stale ? '待刷新' : statusLabels[card.statusCode] ?? '状态待确认' }}</span>
      <button v-if="primary" class="btn" :disabled="disabled" :title="hint" @click="emit('action', card, primary.id)">{{ actionLabels[primary.kind] }}</button>
    </div>
  </article>
</template>

<style scoped>
.appcenter-icon { color: var(--accent); background: var(--panel); }
.appcenter-icon svg { width: 26px; height: 26px; }
.desc { flex: 1; }
.appcenter-tags { display: flex; flex-wrap: wrap; gap: 8px; color: var(--muted); font-size: 11px; }
</style>
