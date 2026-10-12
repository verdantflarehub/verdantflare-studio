<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref, useId, watch } from 'vue'
import type { AppCenterCard } from './schema.generated'
import { actionLabels, statusLabels, reasonLabel } from './labels'
import AppIcon from './AppIcon.vue'
import { publisherLabel, resourceLabel } from './presentation'
import { managementActions, versionLabel } from './card-actions'
const props = defineProps<{ card: AppCenterCard; stale: boolean; menuOpen: boolean }>()
const emit = defineEmits<{ action: [card: AppCenterCard, actionId: string]; menu: [key: string] }>()
const menuId = useId(), root = ref<HTMLElement>(), trigger = ref<HTMLButtonElement>(), menu = ref<HTMLElement>()
const management = computed(() => managementActions(props.card, props.stale))
function close(restore = false) { emit('menu', ''); if (restore) trigger.value?.focus() }
async function open(last = false) {
  emit('menu', props.card.cardKey)
  await nextTick()
  const items = menu.value?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]')
  items?.[last ? items.length - 1 : 0]?.focus()
}
function menuKey(event: KeyboardEvent) {
  const items = Array.from(menu.value?.querySelectorAll<HTMLButtonElement>('[role="menuitem"]') ?? [])
  const index = items.indexOf(document.activeElement as HTMLButtonElement)
  if (event.key === 'Escape') { event.preventDefault(); event.stopPropagation(); close(true) }
  else if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
    event.preventDefault()
    items[event.key === 'Home' ? 0 : event.key === 'End' ? items.length - 1 : (index + (event.key === 'ArrowUp' ? -1 : 1) + items.length) % items.length]?.focus()
  } else if (event.key === 'Tab') close()
}
function outside(event: Event) { if (props.menuOpen && !root.value?.contains(event.target as Node)) close() }
document.addEventListener('pointerdown', outside)
document.addEventListener('focusin', outside)
onBeforeUnmount(() => { document.removeEventListener('pointerdown', outside); document.removeEventListener('focusin', outside) })
watch(() => management.value.length, length => { if (!length && props.menuOpen) close() })
function manage(item: ReturnType<typeof managementActions>[number]) {
  if (!item.enabled || !item.id) return
  close(true); emit('action', props.card, item.id)
}
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
        <div class="brand-version"><span>{{ publisherLabel(card.display.publisherId) }}</span><span>{{ versionLabel(card) }}</span></div>
      </div>
    </div>
    <p class="card-desc">{{ card.display.description || '暂无应用说明' }}</p>
    <div v-if="card.display.resourceTags.length || card.display.models?.length" class="tags appcenter-tags"><span v-for="tag in card.display.resourceTags" :key="tag.kind" class="tag">{{ resourceLabel(tag.kind) }} {{ tag.value }} {{ tag.unit }}</span><span v-if="card.display.models?.length" class="tag">{{ card.display.models.length }} 项声明模型</span></div>
    <div class="card-foot">
      <span class="status" :class="{ running: card.statusCode === 'READY' && !stale }"><i class="dot" :class="{ on: card.statusCode === 'READY' && !stale }" />{{ stale ? '待刷新' : statusLabels[card.statusCode] ?? '状态待确认' }}</span>
      <div ref="root" class="card-actions">
        <button v-if="primary" class="btn" :disabled="disabled" :title="hint" @click="emit('action', card, primary.id)">{{ actionLabels[primary.kind] }}</button>
        <button v-if="management.length" ref="trigger" class="btn menu-trigger" :aria-label="`${card.display.name} 管理操作`" aria-haspopup="menu" :aria-expanded="menuOpen" :aria-controls="menuId" @click="menuOpen ? close(true) : open()" @keydown.down.prevent="open()" @keydown.up.prevent="open(true)" @keydown.esc.prevent="close(true)">▾</button>
        <div v-if="menuOpen && management.length" :id="menuId" ref="menu" class="management-menu" role="menu" :aria-label="`${card.display.name} 管理操作`" @keydown="menuKey">
          <button v-for="item in management" :key="item.kind" role="menuitem" :aria-disabled="!item.enabled" :title="item.reason" @click="manage(item)"><span>{{ actionLabels[item.kind] }}</span><small v-if="!item.enabled">{{ item.reason }}</small></button>
        </div>
      </div>
    </div>
  </article>
</template>

<style scoped>
.appcenter-icon-button { padding: 0; border: 0; background: none; cursor: pointer; }
.card-desc { flex: 1; }
.appcenter-tags { display: flex; flex-wrap: wrap; gap: 8px; color: var(--muted); font-size: 11px; }
.app-card { overflow: visible; }
.app-card:has(.management-menu) { z-index: 21; }
.card-actions { position: relative; display: flex; align-items: stretch; gap: 0; }
.card-actions:has(.menu-trigger) > .btn:first-child { border-radius: 10px 0 0 10px; }
.menu-trigger { border-radius: 0 10px 10px 0; padding-inline: 10px; margin-left: -1px; }
.management-menu { position: absolute; right: 0; top: calc(100% + 6px); z-index: 20; min-width: 180px; max-width: 260px; padding: 6px; border: 1px solid var(--line); border-radius: 12px; background: var(--panel); box-shadow: var(--shadow); }
.management-menu button { display: block; width: 100%; text-align: left; padding: 10px 12px; border: 0; border-radius: 8px; background: transparent; color: var(--ink); cursor: pointer; }
.management-menu button:hover, .management-menu button:focus-visible { background: var(--subtle); }
.management-menu button[aria-disabled="true"] { color: var(--muted); cursor: default; }
.management-menu small { display: block; margin-top: 4px; line-height: 1.5; }
</style>
