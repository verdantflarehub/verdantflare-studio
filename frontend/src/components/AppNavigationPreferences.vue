<script setup lang="ts">
import { useNavigationStore } from '../platform/navigationStore'
import { computed } from 'vue'
const navigation = useNavigationStore()
const props = defineProps<{ appId?: string; showState?: boolean }>()
const entries = computed(() => navigation.entries.filter(item => !props.appId || item.appId === props.appId))
</script>

<template>
  <div class="navigation-preferences" v-if="navigation.state.value?.mode === 'preinstalled'">
    <div v-for="entry in entries" :key="`${entry.appId}/${entry.entryId}`" class="navigation-preference">
      <span>{{ entry.title }}</span>
      <button type="button" :disabled="navigation.state.saving" :aria-pressed="entry.pinned" @click="navigation.setPreference(entry, !entry.pinned)">
        {{ entry.pinned ? '取消固定' : '固定到菜单' }}
      </button>
    </div>
    <p v-if="entries.length === 0" class="navigation-empty">暂无可固定的应用入口。</p>
  </div>
  <p v-if="navigation.state.error" role="status" class="navigation-feedback">{{ navigation.state.error }}</p>
  <p v-else-if="showState && navigation.state.value?.mode === 'legacy'" class="navigation-empty">当前使用系统默认菜单，应用固定设置尚未开放。</p>
  <p v-else-if="showState && !navigation.state.value" role="status" class="navigation-empty">正在读取导航偏好…</p>
</template>

<style scoped>
.navigation-preferences { display: grid; gap: 8px; }
.navigation-preference { display: flex; gap: 8px; align-items: center; justify-content: space-between; }
.navigation-preference > span { overflow-wrap: anywhere; }
.navigation-preference button { flex-shrink: 0; color: var(--accent); font-size: 12px; padding: 4px; }
.navigation-empty, .navigation-feedback { color: var(--muted); font-size: 12px; margin: 6px 0; }
</style>
