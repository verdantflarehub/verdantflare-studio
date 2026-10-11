<script setup lang="ts">
import { useNavigationStore } from '../platform/navigationStore'
const navigation = useNavigationStore()
defineProps<{ appId?: string }>()
</script>

<template>
  <div class="navigation-preferences" v-if="navigation.state.value?.mode === 'preinstalled'">
    <div v-for="entry in navigation.entries.filter(item => !appId || item.appId === appId)" :key="`${entry.appId}/${entry.entryId}`" class="navigation-preference">
      <span>{{ entry.title }}</span>
      <button type="button" :disabled="navigation.state.saving" :aria-pressed="entry.pinned" @click="navigation.setPreference(entry, !entry.pinned)">
        {{ entry.pinned ? '取消固定' : '固定到菜单' }}
      </button>
    </div>
    <p v-if="navigation.entries.length === 0" class="navigation-empty">暂无可固定的应用。</p>
  </div>
  <p v-if="navigation.state.error" role="status" class="navigation-feedback">{{ navigation.state.error }}</p>
</template>

<style scoped>
.navigation-preferences { display: grid; gap: 8px; }
.navigation-preference { display: flex; gap: 8px; align-items: center; justify-content: space-between; }
.navigation-preference > span { overflow-wrap: anywhere; }
.navigation-preference button { flex-shrink: 0; color: var(--accent); font-size: 12px; padding: 4px; }
.navigation-empty, .navigation-feedback { color: var(--muted); font-size: 12px; margin: 6px 0; }
</style>
