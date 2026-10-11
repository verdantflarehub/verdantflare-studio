<script setup lang="ts">
import type { AppCenterCard } from './schema.generated'
import AppIcon from './AppIcon.vue'
import AppNavigationPreferences from '../components/AppNavigationPreferences.vue'
import { actionLabels, statusLabels, factLabel, reasonLabel } from './labels'
import { publisherLabel, observedTime, resourceLabel, serviceReasonLabel } from './presentation'
defineProps<{ card: AppCenterCard; stale: boolean; groupLabel: string }>()
const emit = defineEmits<{ back: []; action: [card: AppCenterCard, id: string] }>()
const origins: Record<string, string> = { installation: '安装记录', observation: '现场部署 · 待对账', catalog: '应用目录' }
</script>

<template>
  <section class="appcenter-detail" aria-label="应用详情">
    <nav class="detail-breadcrumb" aria-label="面包屑"><button @click="emit('back')">应用中心</button><span>/</span><span aria-current="page">{{ card.display.name }}</span></nav>
    <header class="panel detail-summary">
      <div class="detail-title"><AppIcon :icon-ref="card.display.iconRef" /><div><h2>{{ card.display.name }}</h2><p class="muted">{{ publisherLabel(card.display.publisherId) }} · {{ groupLabel }}</p></div></div>
      <p class="detail-description">{{ card.display.description || '应用说明待补充' }}</p>
      <p class="detail-state">{{ stale ? '信息已过期 · 请刷新' : statusLabels[card.statusCode] ?? '状态待确认' }}</p>
      <div class="action-row"><div v-for="action in card.actions.filter(item => item.kind !== 'detail')" :key="action.id" class="detail-action">
        <button class="btn" :disabled="!action.enabled || stale && !['refresh', 'progress'].includes(action.kind)" @click="emit('action', card, action.id)">{{ actionLabels[action.kind] }}</button>
        <p v-if="!action.enabled">{{ reasonLabel(action.reasonCode) }}</p>
      </div></div>
    </header>
    <div class="detail-columns">
      <section class="panel detail-section"><h3>安装与运行</h3><dl>
        <dt>应用标识</dt><dd>{{ card.appId }}</dd><dt>事实来源</dt><dd>{{ origins[card.origin] }}</dd>
        <dt>安装版本</dt><dd>{{ card.installedVersion ?? (card.installationQuality === 'known' ? '无已确认安装' : '安装版本待确认') }}</dd><dt>目录候选版本</dt><dd>{{ card.candidateVersion ?? '暂无可信候选' }}</dd>
        <dt>安装状态</dt><dd>{{ card.installationQuality !== 'known' ? '安装事实待确认' : card.installation ? factLabel(card.installation.state) : '无安装记录' }}</dd>
        <template v-if="card.installation"><dt>安装标识</dt><dd>{{ card.installation.installationId }}</dd></template>
        <dt>服务状态</dt><dd>{{ factLabel(card.service.state) }}<small v-if="card.service.reasonCode">{{ serviceReasonLabel(card.service.reasonCode) }}</small></dd>
        <dt>服务观测时间</dt><dd>{{ observedTime(card.service.observedAt) }}</dd>
        <dt>应用分组</dt><dd>{{ card.preinstalled ? '系统预装应用' : '普通应用' }}</dd>
      </dl></section>
      <section class="panel detail-section"><h3>应用入口</h3><dl>
        <dt>入口状态</dt><dd>{{ card.entry ? factLabel(card.entry.availability) : '尚无可用入口' }}</dd>
        <template v-if="card.entry"><dt>承载方式</dt><dd>{{ card.entry.transport === 'dashboard' ? '应用 Dashboard' : '兼容工作区' }}</dd>
          <dt>最低宿主版本</dt><dd>{{ card.entry.minimumHostVersion ?? '未声明' }}</dd></template>
      </dl><p v-if="card.entry?.reasonCode" class="muted">{{ reasonLabel(card.entry.reasonCode) }}</p><p class="muted detail-note">打开或关闭页面与后端实例运行独立。</p></section>
      <section class="panel detail-section"><h3>声明模型</h3><ul v-if="card.display.models?.length" class="detail-tags"><li v-for="model in card.display.models" :key="model">{{ model }}</li></ul><p v-else class="muted">{{ card.display.models ? '未声明模型' : '模型资料待补充' }}</p><p class="muted detail-note">模型名称来自应用资料，下载与运行状态请在应用内查看。</p></section>
      <section class="panel detail-section"><h3>声明能力</h3><ul v-if="card.display.capabilities?.length" class="detail-tags"><li v-for="capability in card.display.capabilities" :key="capability">{{ capability }}</li></ul><p v-else class="muted">{{ card.display.capabilities ? '未声明能力' : '能力资料待补充' }}</p><p class="muted detail-note">实际可用能力取决于服务状态与当前权限。</p></section>
      <section class="panel detail-section"><h3>管理服务声明资源</h3><dl v-if="card.display.resourceTags.length"><template v-for="tag in card.display.resourceTags" :key="tag.kind"><dt>{{ resourceLabel(tag.kind) }}</dt><dd>{{ tag.value }} {{ tag.unit }}</dd></template></dl><p v-else class="muted">未提供资源声明</p><p class="muted detail-note">实例的计算需求与实际占用由应用展示。</p></section>
      <section class="panel detail-section"><h3>最近操作</h3><dl v-if="card.operation.canRead"><dt>操作状态</dt><dd>{{ factLabel(card.operation.status) }}</dd><dt>当前阶段</dt><dd>{{ factLabel(card.operation.phase) }}</dd><dt>更新时间</dt><dd>{{ observedTime(card.operation.updatedAt) }}</dd><dt>操作标识</dt><dd>{{ card.operation.operationId }}</dd></dl><p v-else class="muted">{{ card.operation.quality !== 'known' ? '操作信息待确认' : card.operation.blocking ? '应用正在变更，请稍后刷新。' : '暂无可读取的操作记录' }}</p></section>
    </div>
    <section class="panel detail-section"><h3>导航偏好</h3><AppNavigationPreferences :app-id="card.appId" show-state /></section>
    <button class="btn ghost detail-back" @click="emit('back')">返回应用中心</button>
  </section>
</template>

<style scoped>
.appcenter-detail { display: grid; gap: 20px; }
.detail-breadcrumb { display: flex; flex-wrap: wrap; gap: 10px; color: var(--muted); font-size: 13px; }
.detail-breadcrumb button { border: 0; padding: 0; background: none; color: inherit; cursor: pointer; }
.detail-summary, .detail-section { padding: 24px; min-width: 0; }
.detail-title { display: flex; align-items: center; gap: 14px; }
.detail-title h2 { margin: 0; font-size: 22px; }
.detail-title p { margin: 5px 0 0; font-size: 12px; }
.detail-description { line-height: 1.7; }
.detail-state { color: var(--accent); font-size: 13px; }
.action-row { flex-wrap: wrap; align-items: start; }
.detail-action { max-width: 220px; }
.detail-action p { color: var(--muted); margin: 6px 0 0; font-size: 12px; }
.detail-columns { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 20px; }
.detail-section h3 { font-size: 16px; margin: 0 0 20px; }
dl { display: grid; grid-template-columns: minmax(100px, .5fr) 1fr; gap: 14px 20px; font-size: 13px; margin: 0; }
dt { color: var(--muted); } dd { margin: 0; overflow-wrap: anywhere; } dd small { display: block; margin-top: 5px; color: var(--muted); }
.detail-tags { display: flex; flex-wrap: wrap; gap: 8px; list-style: none; padding: 0; margin: 0; }
.detail-tags li { padding: 6px 10px; border: 1px solid var(--line); border-radius: 6px; font-size: 12px; overflow-wrap: anywhere; max-width: 100%; }
.detail-note { font-size: 12px; line-height: 1.6; margin-top: 18px; }
.detail-back { justify-self: start; }
@media (max-width: 900px) { .detail-columns { grid-template-columns: 1fr; } }
</style>
