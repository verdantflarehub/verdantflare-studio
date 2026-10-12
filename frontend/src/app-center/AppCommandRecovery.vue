<script setup lang="ts">
import { computed } from 'vue'
import type { CommandState } from './commands'
import type { AppCommand } from './schema.generated'
import { actionLabels, reasonLabel } from './labels'
import AppOperationStatus from './AppOperationStatus.vue'
const props = defineProps<{ state: CommandState; titles: Record<string, string>; appId?: string; exceptOperationId?: string }>()
const emit = defineEmits<{ resume: [key: string] }>()
const rows = computed(() => props.state.items.map(item => {
  const input = JSON.parse(item.record.body)
  const command: AppCommand = item.record.protocol === 'legacy' ? input : input.prepared.command
  return { ...item, command }
}).filter(item => (!props.appId || item.command.app_id === props.appId) && (!props.exceptOperationId || item.record.operationId !== props.exceptOperationId || item.error)))
</script>

<template>
  <section v-if="rows.length" class="panel recovery-panel" aria-label="操作与恢复">
    <h2>操作与恢复</h2>
    <article v-for="item in rows" :key="item.record.key" class="recovery-item">
      <h3>{{ titles[item.command.app_id] ?? item.command.app_id }} · {{ actionLabels[item.command.action] ?? '应用管理' }}</h3>
      <AppOperationStatus v-if="item.operation" :operation="item.operation" :stale="Boolean(item.error)" />
      <p v-else>{{ item.record.operationId ? '等待确认原操作状态。' : '提交结果待确认。恢复将继续原请求，请勿重复发起。' }}</p>
      <p v-if="item.error" role="alert">{{ reasonLabel(item.error) }}</p>
      <button v-if="!item.operation || !['succeeded', 'failed'].includes(item.operation.status) || item.error" class="btn" :disabled="item.busy" @click="emit('resume', item.record.key)">{{ item.busy ? '正在确认…' : item.record.operationId ? '刷新进度' : '恢复原请求' }}</button>
    </article>
  </section>
</template>

<style scoped>
.recovery-panel { padding: 20px; margin: 16px 0; background: var(--panel); border: 1px solid var(--line); border-radius: 12px; }
h2 { font-size: 18px; margin: 0 0 12px; }
h3 { font-size: 15px; margin: 0 0 16px; overflow-wrap: anywhere; }
.recovery-item + .recovery-item { border-top: 1px solid var(--line); margin-top: 16px; padding-top: 16px; }
.recovery-item .btn { margin-top: 16px; }
.recovery-item p { color: var(--muted); line-height: 1.6; }
</style>
