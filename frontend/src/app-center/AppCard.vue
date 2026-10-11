<script setup lang="ts">
import { computed } from 'vue'
import type { AppCenterCard } from './schema.generated'
import { actionLabels, statusLabels, reasonLabel } from './labels'
import AppIcon from './AppIcon.vue'
import { publisherLabel, resourceLabel } from './presentation'
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
      <button class="appcenter-icon-button" :aria-label="`查看 ${card.display.name} 详情`" @click="emit('action', card, 'detail')"><AppIcon :icon-ref="card.display.iconRef" /></button>
      <div><button class="app-title" @click="emit('action', card, 'detail')">{{ card.display.name }}</button>
        <div class="brand-version"><span v-if="card.display.publisherId">{{ publisherLabel(card.display.publisherId) }}</span><span v-if="card.installedVersion || card.candidateVersion">{{ card.installedVersion ?? card.candidateVersion }}</span></div>
      </div>
    </div>
    <p class="card-desc">{{ card.display.description || '暂无应用说明' }}</p>
    <div v-if="card.display.resourceTags.length || card.display.models?.length" class="tags appcenter-tags"><span v-for="tag in card.display.resourceTags" :key="tag.kind" class="tag">{{ resourceLabel(tag.kind) }} {{ tag.value }} {{ tag.unit }}</span><span v-if="card.display.models?.length" class="tag">{{ card.display.models.length }} 项声明模型</span></div>
    <div class="card-foot">
      <span class="status" :class="{ running: card.statusCode === 'READY' && !stale }"><i class="dot" :class="{ on: card.statusCode === 'READY' && !stale }" />{{ stale ? '待刷新' : statusLabels[card.statusCode] ?? '状态待确认' }}</span>
      <button v-if="primary" class="btn" :disabled="disabled" :title="hint" @click="emit('action', card, primary.id)">{{ actionLabels[primary.kind] }}</button>
    </div>
  </article>
</template>

<style scoped>
.appcenter-icon-button { padding: 0; border: 0; background: none; cursor: pointer; }
.card-desc { flex: 1; }
.appcenter-tags { display: flex; flex-wrap: wrap; gap: 8px; color: var(--muted); font-size: 11px; }
</style>
