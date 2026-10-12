<script setup lang="ts">
import type { CommandOperation } from './schema.generated'
import { actionLabels, factLabel, reasonLabel } from './labels'
import { observedTime } from './presentation'
defineProps<{ operation: CommandOperation; stale?: boolean }>()
const bytes = (value: number) => value < 1024 ? `${value} B` : value < 1024 ** 2 ? `${(value / 1024).toFixed(1)} KiB` : value < 1024 ** 3 ? `${(value / 1024 ** 2).toFixed(1)} MiB` : `${(value / 1024 ** 3).toFixed(1)} GiB`
</script>

<template>
  <div class="operation-status">
    <p v-if="stale" class="muted">以下为上次读取的操作信息，等待重新确认。</p>
    <dl>
      <dt>操作</dt><dd>{{ actionLabels[operation.action] ?? '应用管理' }}</dd>
      <dt>操作版本</dt><dd>{{ operation.app_version }}</dd>
      <dt>状态</dt><dd>{{ factLabel(operation.status) }}</dd>
      <dt>阶段</dt><dd>{{ factLabel(operation.phase) }}</dd>
      <dt>下载</dt><dd>{{ bytes(operation.download.downloaded_bytes) }} / {{ operation.download.total_bytes === null ? '总大小待确认' : bytes(operation.download.total_bytes) }}<span v-if="operation.download.percent !== null"> · {{ operation.download.percent }}%</span></dd>
      <dt>更新时间</dt><dd>{{ observedTime(operation.updated_at) }}</dd>
      <dt>操作标识</dt><dd>{{ operation.operation_id }}</dd>
    </dl>
    <progress v-if="operation.status === 'running' && operation.phase === 'downloading'" :value="operation.download.percent ?? undefined" max="100" aria-label="下载进度" />
    <p v-if="operation.error" class="operation-error" role="alert">{{ reasonLabel(operation.error.code) }}</p>
  </div>
</template>

<style scoped>
dl { display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: 10px 16px; margin: 0; font-size: 13px; }
dt { color: var(--muted); }
dd { margin: 0; overflow-wrap: anywhere; }
progress { width: 100%; accent-color: var(--accent); margin-top: 16px; }
.operation-error { color: var(--danger); line-height: 1.6; }
</style>
