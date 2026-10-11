<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useHostStore } from '../../platform/hostStore'
const props = defineProps<{ projectId: string; revisionId: string; target: string }>()
const host = useHostStore(), note = ref(''), message = ref('')
const storageKey = computed(() => host.identity.user_id && host.identity.organization_id ? ['studio.drama.tips', host.identity.organization_id, host.identity.user_id, props.projectId, props.revisionId, props.target].join(':') : '')
watch(storageKey, key => { note.value = ''; message.value = ''; if (key) { try { note.value = localStorage.getItem(key) || '' } catch { message.value = '无法读取本机提示。' } } }, { immediate: true })
function save() { try { if (!storageKey.value) throw new Error(); localStorage.setItem(storageKey.value, note.value); message.value = '提示已保存在本机。' } catch { message.value = '保存失败，输入仍保留，请复制备份后重试。' } }
</script>
<template><section class="panel review"><h2>修改提示（Tips）</h2><p class="muted">记录给 AI 的修改建议。提示仅保存到本机；剧本和素材修改需由 AI 经 MCP 执行并保存新修订。</p><form @submit.prevent="save"><label>提示内容<textarea v-model="note" rows="6" maxlength="20000" placeholder="例如：补充人物动机，或核对某一场的衣着变化。" /></label><button class="btn primary" type="submit" :disabled="!storageKey">保存提示</button><p v-if="message" role="status" class="muted">{{ message }}</p></form></section></template>
<style scoped>
.panel { background:var(--shell-panel); border:1px solid var(--line); border-radius:8px; padding:20px; min-width:0; }h2 { font-size:15px; margin-bottom:14px; }p { font-size:12px; line-height:1.8; margin-bottom:12px; }.muted { color:var(--muted); }form { display:grid; gap:10px; }label { display:grid; gap:10px; font-size:12px; }textarea { width:100%; resize:vertical; padding:10px; border-radius:5px; border:1px solid var(--line); background:var(--panel); color:var(--ink); font:inherit; }textarea:focus-visible { outline:2px solid var(--accent); outline-offset:3px; }.btn { font-size:12px; }
</style>
