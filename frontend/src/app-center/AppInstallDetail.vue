<script setup lang="ts">
import type { AppCenterCard, CommonAction } from './schema.generated'
import AppIcon from './AppIcon.vue'
import AppNavigationPreferences from '../components/AppNavigationPreferences.vue'
import { actionLabels, statusLabels, factLabel, reasonLabel } from './labels'
import { publisherLabel, observedTime, resourceLabel, serviceReasonLabel } from './presentation'
import AppOperationStatus from './AppOperationStatus.vue'
import type { OperationProgress } from './operation-progress'
import { computed } from 'vue'
import { useNavigationStore } from '../platform/navigationStore'
const props = defineProps<{ card: AppCenterCard; stale: boolean; groupLabel: string; operationProgress: OperationProgress; nativeOpen?: boolean }>()
const navigation = useNavigationStore()
const hasNavigation = computed(() => props.card.installation?.state === 'installed' && navigation.entries.some(entry => entry.installationId === props.card.installation?.installationId && entry.appId === props.card.appId))
const emit = defineEmits<{ back: []; action: [card: AppCenterCard, id: string]; refreshOperation: [] }>()
const origins: Record<string, string> = { installation: '安装记录', observation: '现场部署 · 待对账', catalog: '应用目录' }
const localAction = (action: CommonAction) => ['refresh', 'progress'].includes(action.kind)
function actionDisabled(action: CommonAction) { return !action.enabled || props.stale && !localAction(action) }
function actionTip(action: CommonAction) {
  if (!actionDisabled(action)) return ''
  if (!action.enabled) return reasonLabel(action.reasonCode)
  return '当前信息已过期，请刷新'
}
</script>

<template>
  <section class="appcenter-detail" aria-label="应用详情">
    <nav class="detail-breadcrumb" aria-label="面包屑"><button @click="emit('back')">应用中心</button><span>/</span><span aria-current="page">{{ card.display.name }}</span></nav>
    <header class="panel detail-summary">
      <div class="detail-title"><AppIcon :icon-ref="card.display.iconRef" /><div><h2>{{ card.display.name }}</h2><p class="muted">{{ publisherLabel(card.display.publisherId) }} · {{ groupLabel }}</p></div></div>
      <p class="detail-description">{{ card.display.description || '应用说明待补充' }}</p>
      <p class="detail-state">{{ stale ? '信息已过期 · 请刷新' : statusLabels[card.statusCode] ?? '状态待确认' }}</p>
      <div class="action-row"><button v-if="nativeOpen" class="btn" @click="emit('action', card, 'native-open')">打开</button><span v-for="action in card.actions.filter(item => item.kind !== 'detail')" :key="action.id" class="detail-action" :tabindex="actionTip(action) ? 0 : undefined">
        <button class="btn" :disabled="actionDisabled(action)" @click="emit('action', card, action.id)">{{ actionLabels[action.kind] }}</button>
        <span v-if="actionTip(action)" class="action-tip" role="tooltip">{{ actionTip(action) }}</span>
      </span></div>
    </header>
    <slot />
    <div class="detail-columns">
      <section class="panel detail-section"><h3>安装与运行</h3><dl>
        <dt>应用标识</dt><dd>{{ card.appId }}</dd><dt>事实来源</dt><dd>{{ origins[card.origin] }}</dd>
        <dt>安装版本</dt><dd>{{ card.installedVersion ?? (card.installationQuality === 'known' ? '无已确认安装' : '安装版本待确认') }}</dd><dt>目录候选版本</dt><dd>{{ card.candidateVersion ?? '暂无可信候选' }}</dd>
        <dt>安装状态</dt><dd>{{ card.installationQuality !== 'known' ? '安装事实待确认' : card.installation ? factLabel(card.installation.state) : '无安装记录' }}</dd>
        <template v-if="card.installation"><dt>安装标识</dt><dd>{{ card.installation.installationId }}</dd></template>
        <dt>服务状态</dt><dd>{{ factLabel(card.service.state) }}<small v-if="card.service.reasonCode">{{ serviceReasonLabel(card.service.reasonCode) }}</small></dd>
        <dt>服务观测时间</dt><dd>{{ observedTime(card.service.observedAt) }}</dd>
        <dt>应用分组</dt><dd>{{ groupLabel }}</dd>
        <template v-if="card.display.disabledReason"><dt>未开放原因</dt><dd>{{ card.display.disabledReason }}</dd></template>
      </dl></section>
      <section v-if="card.helm" class="panel detail-section"><h3>Helm OCI 安装输入</h3><dl>
        <dt>Chart OCI</dt><dd><code>{{ card.helm.ociRef }}</code></dd>
        <dt>Chart 版本</dt><dd>{{ card.helm.chartName }} @ {{ card.helm.chartVersion }}</dd>
        <dt>Manifest Digest</dt><dd><code>{{ card.helm.manifestDigest }}</code></dd>
        <dt>Package Digest</dt><dd><code>{{ card.helm.packageDigest }}</code></dd>
        <dt>目标 Namespace</dt><dd><code>{{ card.helm.targetNamespace }}</code></dd>
        <dt>Helm Release</dt><dd><code>{{ card.helm.releaseName }}</code></dd>
        <dt>Values 来源</dt><dd><code>{{ card.helm.valuesSource }}</code></dd>
      </dl></section>
      <section class="panel detail-section"><h3>应用入口</h3><dl>
        <dt>入口状态</dt><dd>{{ card.entry ? factLabel(card.entry.availability) : '尚无可用入口' }}</dd>
        <template v-if="card.entry"><dt>承载方式</dt><dd>{{ card.entry.transport === 'dashboard' ? '应用 Dashboard' : '兼容工作区' }}</dd>
          <dt>最低宿主版本</dt><dd>{{ card.entry.minimumHostVersion ?? '未声明' }}</dd></template>
      </dl><p v-if="card.entry?.reasonCode" class="muted">{{ reasonLabel(card.entry.reasonCode) }}</p><p class="muted detail-note">打开或关闭页面与后端实例运行独立。</p></section>
      <section class="panel detail-section"><h3>声明模型</h3><ul v-if="card.display.models?.length" class="detail-tags"><li v-for="model in card.display.models" :key="model">{{ model }}</li></ul><p v-else class="muted">{{ card.display.models ? '未声明模型' : '模型资料待补充' }}</p><p class="muted detail-note">模型名称来自应用资料，下载与运行状态请在应用内查看。</p></section>
      <section class="panel detail-section"><h3>声明能力</h3><ul v-if="card.display.capabilities?.length" class="detail-tags"><li v-for="capability in card.display.capabilities" :key="capability">{{ capability }}</li></ul><p v-else class="muted">{{ card.display.capabilities ? '未声明能力' : '能力资料待补充' }}</p><p class="muted detail-note">实际可用能力取决于服务状态与当前权限。</p></section>
      <section class="panel detail-section"><h3>管理服务声明资源</h3><dl v-if="card.display.resourceTags.length"><template v-for="tag in card.display.resourceTags" :key="tag.kind"><dt>{{ resourceLabel(tag.kind) }}</dt><dd>{{ tag.value }} {{ tag.unit }}</dd></template></dl><p v-else class="muted">未提供资源声明</p><p class="muted detail-note">实例的计算需求与实际占用由应用展示。</p></section>
      <section class="panel detail-section"><h3>最近操作</h3>
        <AppOperationStatus v-if="operationProgress.value" :operation="operationProgress.value" :stale="operationProgress.stale" />
        <dl v-else-if="card.operation.canRead && !['UNAUTHENTICATED', 'PERMISSION_DENIED'].includes(operationProgress.error)"><dt>操作状态</dt><dd>{{ factLabel(card.operation.status) }}</dd><dt>当前阶段</dt><dd>{{ factLabel(card.operation.phase) }}</dd><dt>更新时间</dt><dd>{{ observedTime(card.operation.updatedAt) }}</dd><dt>操作标识</dt><dd>{{ card.operation.operationId }}</dd></dl>
        <p v-else class="muted">{{ card.operation.quality !== 'known' ? '操作信息待确认' : card.operation.blocking ? '应用正在变更，请稍后刷新。' : '暂无可读取的操作记录' }}</p>
        <p v-if="operationProgress.error" role="alert">{{ reasonLabel(operationProgress.error) }}</p>
        <button v-if="card.operation.canRead && card.operation.protocol === 'lifecycle'" class="btn operation-refresh" :disabled="operationProgress.busy" @click="emit('refreshOperation')">{{ operationProgress.busy ? '正在读取…' : '刷新进度' }}</button>
      </section>
    </div>
    <section v-if="hasNavigation" class="panel detail-section"><h3>侧栏显示</h3><AppNavigationPreferences :app-id="card.appId" show-state /></section>
    <button class="btn ghost detail-back" @click="emit('back')">返回应用中心</button>
  </section>
</template>

<style scoped>
.appcenter-detail { display: grid; gap: 20px; }
.detail-breadcrumb { display: flex; flex-wrap: wrap; gap: 10px; color: var(--muted); font-size: 13px; }
.detail-breadcrumb button { border: 0; padding: 0; background: none; color: inherit; cursor: pointer; }
.detail-summary, .detail-section { padding: 24px; min-width: 0; background: var(--panel); border: 0.5px solid var(--vf-hairline-outer); box-shadow: inset 0 0 0 0.5px var(--vf-hairline-inner), var(--vf-shadow-card); border-radius: 12px; }
.detail-summary { overflow: visible; }
.detail-title { display: flex; align-items: center; gap: 14px; }
.detail-title h2 { margin: 0; font-size: 22px; }
.detail-title p { margin: 5px 0 0; font-size: 12px; }
.detail-description { line-height: 1.7; }
.detail-state { color: var(--accent); font-size: 13px; }
.operation-refresh { margin-top: 16px; }
.action-row { flex-wrap: wrap; align-items: center; overflow: visible; }
.detail-action { position: relative; display: inline-flex; outline: none; }
.action-tip { position: absolute; left: 50%; bottom: calc(100% + 8px); z-index: 8; width: max-content; max-width: 240px; transform: translateX(-50%); padding: 8px 10px; border: 0.5px solid var(--vf-hairline-outer); border-radius: 8px; background: var(--panel); box-shadow: inset 0 0 0 0.5px var(--vf-hairline-inner), var(--shadow); color: var(--ink); font-size: 12px; line-height: 1.5; text-align: left; pointer-events: none; opacity: 0; visibility: hidden; }
.detail-action:hover .action-tip, .detail-action:focus-visible .action-tip { opacity: 1; visibility: visible; }
.detail-columns { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 20px; }
.detail-section h3 { font-size: 16px; margin: 0 0 20px; }
dl { display: grid; grid-template-columns: minmax(100px, .5fr) 1fr; gap: 14px 20px; font-size: 13px; margin: 0; }
dt { color: var(--muted); } dd { margin: 0; overflow-wrap: anywhere; font-variant-numeric: tabular-nums; } dd small { display: block; margin-top: 5px; color: var(--muted); }
.detail-tags { display: flex; flex-wrap: wrap; gap: 8px; list-style: none; padding: 0; margin: 0; }
.detail-tags li { padding: 6px 10px; border: 0.5px solid var(--vf-hairline-outer); border-radius: 6px; font-size: 12px; overflow-wrap: anywhere; max-width: 100%; font-variant-numeric: tabular-nums; }
.detail-note { font-size: 12px; line-height: 1.6; margin-top: 18px; }
.detail-back { justify-self: start; }
@media (max-width: 900px) { .detail-columns { grid-template-columns: 1fr; } }
</style>
