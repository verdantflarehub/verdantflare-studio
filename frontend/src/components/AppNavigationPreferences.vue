<script setup lang="ts">
import { useNavigationStore } from '../platform/navigationStore'
import { computed } from 'vue'
import type { NavigationEntry } from '../navigation/controller'
const navigation = useNavigationStore()
const props = defineProps<{ appId?: string; showState?: boolean }>()
const entries = computed(() => navigation.entries.filter(item => !props.appId || item.appId === props.appId))
function change(entry: NavigationEntry, event: Event) {
  const control = event.target as HTMLInputElement, desired = control.checked
  control.checked = entry.pinned
  void navigation.setPreference(entry, desired)
}
</script>

<template>
  <div class="navigation-preferences" v-if="navigation.state.value?.mode === 'preinstalled'">
    <div v-for="entry in entries" :key="`${entry.appId}/${entry.entryId}`" class="navigation-preference">
      <label><input type="checkbox" role="switch" :checked="entry.pinned" :disabled="navigation.state.saving || navigation.state.status !== 'ready'" @change="change(entry, $event)"> 在侧栏显示</label>
    </div>
    <p v-if="entries.length" class="navigation-empty">隐藏入口不会停止应用。</p>
  </div>
  <p v-if="navigation.state.error" role="status" class="navigation-feedback">{{ navigation.state.error }}</p>
  <p v-else-if="showState && navigation.state.value?.mode === 'legacy'" class="navigation-empty">当前使用系统默认菜单，应用固定设置尚未开放。</p>
  <p v-else-if="showState && !navigation.state.value" role="status" class="navigation-empty">正在读取导航偏好…</p>
</template>

<style scoped>
.navigation-preferences { display: grid; gap: 8px; }
.navigation-preference { display: flex; gap: 8px; align-items: center; justify-content: space-between; }
.navigation-preference > span { overflow-wrap: anywhere; }
.navigation-preference label { display:flex; align-items:center; gap:8px; cursor:pointer; }
.navigation-preference input { accent-color:var(--accent); }
.navigation-empty, .navigation-feedback { color: var(--muted); font-size: 12px; margin: 6px 0; }
</style>
