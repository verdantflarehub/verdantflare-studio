<script setup lang="ts">
import { ref } from 'vue'
defineProps<{ text?: string | null }>()
const message = ref('')
async function copy(text: string) {
  try { await navigator.clipboard.writeText(text); message.value = '已复制' }
  catch { message.value = '复制失败，请选择文字复制。' }
}
</script>

<template>
  <details class="project-prompt">
    <summary>提示词</summary>
    <p>{{ text?.trim() ? text : '未填写' }}</p>
    <button v-if="text?.trim()" class="btn" @click="copy(text)">复制提示词</button>
    <span role="status">{{ message }}</span>
  </details>
</template>

<style scoped>
.project-prompt { margin-top: 16px; border-top: 1px solid var(--line); padding-top: 12px; }
summary { cursor: pointer; color: var(--muted); }
p { white-space: pre-wrap; overflow-wrap: anywhere; margin: 14px 0; }
span { margin-left: 12px; color: var(--muted); }
</style>
