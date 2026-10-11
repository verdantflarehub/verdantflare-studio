<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { useHostStore } from '../platform/hostStore'
import { useNavigationStore } from '../platform/navigationStore'
import { createPreinstalledClient, createPreinstalledIntentStore, preinstalledIntent } from '../navigation/preinstalled'
import type { PreinstalledItem, PreinstalledIntent, PreinstalledStatus } from '../navigation/preinstalled'

const props = defineProps<{ active: boolean; titles: Record<string, string> }>()
const host = useHostStore(), navigation = useNavigationStore(), client = createPreinstalledClient()
const status = ref<PreinstalledStatus | null>(null), error = ref(''), busy = ref(false)
const pendingIntent = ref<PreinstalledIntent | null>(null)
const scope = computed(() => host.connected && host.identity.user_id && host.identity.organization_id && host.identity.station_id ? JSON.stringify([host.identity.user_id, host.identity.organization_id, host.identity.station_id]) : '')
const canRead = computed(() => Boolean(host.identity.roles?.includes('admin') && host.identity.scopes?.includes('app:read')))
const permitted = computed(() => canRead.value && host.identity.scopes?.includes('app:manage'))
// Access can throw in a restricted WebView. Never submit before recovery state
// has been saved, so a refresh cannot silently turn a retry into a new intent.
const intents = () => createPreinstalledIntentStore(sessionStorage)
const labels: Record<string, string> = { ready: '已安装', pending: '待安装', waiting: '等待依赖', blocked: '暂不可用', upgrade_required: '待升级', suppressed: '已按你的选择卸载', unknown: '正在确认原操作', failed: '安装失败', accepted: '已受理', running: '安装中', confirming: '正在确认安装结果', adoption_required: '待接管已有服务' }
let generation = 0, pending: AbortController | undefined, timer: ReturnType<typeof setTimeout> | undefined
function reset() { generation++; pending?.abort(); clearTimeout(timer); status.value = null; pendingIntent.value = null; busy.value = false; error.value = '' }
async function refresh() {
  if (!scope.value || !canRead.value || !props.active || busy.value || document.hidden) return
  const current = ++generation, controller = new AbortController(); pending?.abort(); pending = controller
  try {
    const value = await client.read(AbortSignal.any([controller.signal, AbortSignal.timeout(15000)]))
    if (current !== generation) return
    status.value = value; error.value = ''
    if (value.enabled && value.items.some(item => ['accepted', 'running', 'confirming', 'unknown'].includes(item.state))) {
      clearTimeout(timer); timer = setTimeout(() => void refresh(), 10000)
    }
    void navigation.refresh()
  } catch { if (current === generation) { status.value = null; error.value = '预装应用状态暂时无法读取，请重试。' } }
}
async function execute(kind: 'fill' | 'retry' | 'reinstall' | 'adopt', item?: PreinstalledItem) {
  if (busy.value || !permitted.value || !scope.value || !status.value?.enabled && !pendingIntent.value) return
  const originalScope = scope.value
  let intent: PreinstalledIntent
  try {
    intent = pendingIntent.value ?? intents().read(originalScope) ?? preinstalledIntent(kind, item)
    intents().save(originalScope, intent)
  } catch { error.value = '操作恢复记录暂时无法保存，请刷新后重试。'; return }
  const current = ++generation, controller = new AbortController(); pending?.abort(); pending = controller
  // A lost reply is resolved by replaying the same intent. Never silently make
  // another retry/reinstall intent before the first response is known.
  pendingIntent.value = intent; busy.value = true; error.value = ''
  try {
    const value = await client.execute(intent, AbortSignal.any([controller.signal, AbortSignal.timeout(70000)]))
    if (current !== generation) return
    intents().clear(originalScope, intent.intentId)
    status.value = value; pendingIntent.value = null
    void navigation.refresh()
    timer = setTimeout(() => void refresh(), 1000)
  } catch { if (current === generation) error.value = '上次操作结果尚未确认，请点击“确认上次操作”继续查询原操作。' }
  finally { if (current === generation) busy.value = false }
}
watch([scope, canRead, permitted], () => {
  reset()
  if (scope.value && canRead.value) {
    try { pendingIntent.value = intents().read(scope.value) }
    catch { error.value = '操作恢复记录暂时无法读取，请刷新后重试。' }
    void refresh()
  }
}, { immediate: true })
watch(() => props.active, active => { if (active) void refresh(); else clearTimeout(timer) })
function onVisibility() { if (document.hidden) clearTimeout(timer); else void refresh() }
document.addEventListener('visibilitychange', onVisibility)
onBeforeUnmount(() => { reset(); document.removeEventListener('visibilitychange', onVisibility) })
</script>

<template>
  <section v-if="active && canRead && (status?.enabled || error || pendingIntent)" class="preinstalled-apps" aria-label="系统预装应用">
    <div class="preinstalled-heading"><h2>系统预装应用</h2><div class="action-row">
      <button class="btn ghost" :disabled="busy" @click="refresh">刷新</button>
      <button v-if="permitted && (status?.enabled || pendingIntent)" class="btn" :disabled="busy" @click="execute('fill')">{{ busy ? '正在处理…' : pendingIntent ? '确认上次操作' : '补齐与恢复' }}</button>
    </div></div>
    <p v-if="error" role="status">{{ error }}</p>
    <div v-for="item in status?.items || []" :key="item.appId" class="preinstalled-row">
      <div><strong>{{ titles[item.appId] || item.appId }}</strong><span>{{ labels[item.state] }}</span>
        <details v-if="item.adoptionEvidence"><summary>查看已有服务</summary><p>服务标识：{{ item.adoptionEvidence.expectedWorkloadUid }}</p><p>观测时间：{{ item.adoptionEvidence.observedAt }}</p><p>接管时会再次核对现有服务配置；配置变化将停止接管。</p></details>
      </div>
      <div v-if="permitted && !pendingIntent" class="action-row">
        <button v-if="item.state === 'failed'" class="btn ghost" :disabled="busy" @click="execute('retry', item)">重试</button>
        <button v-if="item.state === 'suppressed'" class="btn ghost" :disabled="busy" @click="execute('reinstall', item)">重新安装</button>
        <button v-if="item.state === 'adoption_required' && item.adoptionEvidence" class="btn ghost" :disabled="busy" @click="execute('adopt', item)">接管此服务</button>
      </div>
    </div>
  </section>
</template>

<style scoped>
.preinstalled-apps { padding: 20px; margin-bottom: 24px; border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
.preinstalled-heading, .preinstalled-row { display: flex; justify-content: space-between; align-items: center; gap: 16px; }
.preinstalled-heading h2 { font-size: 16px; margin: 0; }
.action-row { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
.preinstalled-row { padding-top: 12px; margin-top: 12px; border-top: 1px solid var(--line); }
.preinstalled-row span { margin-left: 16px; color: var(--muted); font-size: 13px; }
.preinstalled-row details { color: var(--muted); font-size: 12px; margin-top: 8px; overflow-wrap: anywhere; }
</style>
