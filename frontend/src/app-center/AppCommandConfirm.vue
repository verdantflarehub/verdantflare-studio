<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
defineProps<{ title: string; name: string; version: string; installationId: string | null; impact: string; busy: boolean }>()
const emit = defineEmits<{ confirm: []; cancel: [] }>()
const dialog = ref<HTMLDialogElement>()
onMounted(() => dialog.value?.showModal())
onBeforeUnmount(() => dialog.value?.close())
</script>

<template>
  <Teleport to="body">
    <dialog ref="dialog" class="app-command-confirm" aria-labelledby="app-command-title" aria-describedby="app-command-impact" @cancel.prevent="!busy && emit('cancel')">
      <h2 id="app-command-title">{{ title }}「{{ name }}」？</h2>
      <p class="muted">版本：{{ version }}</p>
      <p v-if="installationId" class="muted target">安装标识：{{ installationId }}</p>
      <p id="app-command-impact">{{ impact }}</p>
      <div class="confirm-actions"><button class="btn ghost" autofocus :disabled="busy" @click="emit('cancel')">取消</button><button class="btn" :disabled="busy" @click="emit('confirm')">{{ busy ? '提交中…' : `确认${title}` }}</button></div>
    </dialog>
  </Teleport>
</template>

<style scoped>
.app-command-confirm { width: min(480px, calc(100vw - 48px)); box-sizing: border-box; padding: 24px; border: 1px solid var(--line); border-radius: 16px; background: var(--panel); color: var(--ink); }
.app-command-confirm::backdrop { background: rgb(0 0 0 / 40%); }
h2 { margin: 0 0 16px; font-size: 20px; }
p { line-height: 1.6; }
.target { overflow-wrap: anywhere; font-size: 12px; }
.confirm-actions { display: flex; justify-content: flex-end; gap: 12px; margin-top: 24px; }
</style>
