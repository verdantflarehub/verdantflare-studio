<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'
import type { RouteLocationNormalized, RouteLocationRaw } from 'vue-router'
import { useHostStore } from '../platform/hostStore'
import AppDashboardHost from './AppDashboardHost.vue'
import AppInstanceHost from './AppInstanceHost.vue'
import { createDashboardWebClient } from './web-client'
import type { OpenedDashboard } from './web-client'
import { DashboardSessions } from './sessions'
import { dashboardLocation, dashboardRoutePath, installationPattern, instanceWorkspaceLocation } from './routes'
import type { SDKAdapters } from './types'
import { createNavigationTransport } from '../navigation/transport'
import { decodeNavigation, sameNavigationPath } from '../navigation/controller'
import { useNavigationStore } from '../platform/navigationStore'
import { appTargetLocation, decodeAppTarget } from '../navigation/app-target'
import { createFileAccess } from './files'

const props = defineProps<{
  returnLocation?: RouteLocationRaw;
  active: boolean; installationId: string; titles: Record<string, string>;
  openDashboard?: (installationId: string, signal?: AbortSignal) => Promise<OpenedDashboard>;
  authorizeDashboard?: (installationId: string, signal: AbortSignal) => Promise<void>;
}>()
const route = useRoute(), router = useRouter(), identity = useHostStore()
const navigation = useNavigationStore()
const navigationChild = computed(() => navigation.entries.find(entry => entry.installationId === props.installationId)?.children?.find(child => sameNavigationPath(child.route, typeof route.query.path === 'string' ? route.query.path : '/')))
const client = ['http:', 'https:'].includes(location.protocol) ? createDashboardWebClient(location.origin) : undefined
const sessions = client ? new DashboardSessions({ ...client, open: (id, signal) => props.openDashboard ? props.openDashboard(id, signal) : client.open(id, signal) }) : undefined
const host = ref<InstanceType<typeof AppDashboardHost>>()
const instanceHost = ref<InstanceType<typeof AppInstanceHost>>()
const workspaceId = computed(() => route.name === 'app-instance' && typeof route.params.workspaceSessionId === 'string' ? route.params.workspaceSessionId : '')
type Selection = OpenedDashboard & { authorizationEpoch: string; adapters: SDKAdapters; route: string; releaseSession(): Promise<void> }
const selection = shallowRef<Selection | null>(null)
let previousSelection: Selection | null = null
const state = ref<'idle' | 'resolving' | 'resolved' | 'ready' | 'error'>('idle')
const error = ref(''), retry = ref(0)
let revision = 0, scope = '', pending: AbortController | undefined
const actorScope = computed(() => !identity.connected ? '' : JSON.stringify([identity.identity.user_id,
  identity.identity.organization_id, identity.identity.station_id,
  identity.identity.session_id, identity.identity.policy_version, identity.identity.revocation_version,
  [...(identity.identity.roles ?? [])].sort(), [...(identity.identity.scopes ?? [])].sort()]))
const visible = computed(() => props.active && !workspaceId.value && ['resolved', 'ready'].includes(state.value))
const workspaceAppId = ref(''), workspaceInstanceName = ref('')
const title = computed(() => {
  const appId = selection.value?.launch.installationId === props.installationId
    ? selection.value.context.appId
    : workspaceAppId.value
  return appId ? (props.titles[appId] ?? appId) : '应用控制台'
})
const confirmDiscard = async () => window.confirm('控制台有未保存的修改，确定放弃并关闭吗？')
const messages: Record<string, string> = {
  UNAUTHENTICATED: '请先登录，再打开应用控制台。',
  DASHBOARD_DENIED: '当前账号无权打开此控制台，或安装授权已改变。',
  DASHBOARD_UNAVAILABLE: '应用控制台暂时不可用，请稍后重试。',
  DASHBOARD_UNSUPPORTED: '此应用控制台需要尚未提供的接入能力。',
  INVALID_DASHBOARD_BINDING: '应用控制台版本校验失败，请联系管理员核对安装。',
  DASHBOARD_SESSION_EXPIRED: '控制台会话已过期，请重新打开。',
  SESSION_REVOKED: '控制台会话已失效，请重新打开。',
  DESKTOP_DASHBOARD_UNAVAILABLE: '桌面客户端的应用控制台接入尚未完成。',
  INVALID_PATH: '控制台地址无效，请从应用中心重新打开。',
  APPCENTER_BINDING_CHANGED: '应用状态或入口已变化，请返回应用中心刷新后重新打开。',
  APPCENTER_OPEN_UNAVAILABLE: '此应用当前不可打开，请返回应用中心查看状态。',
}
function showError(value: unknown) {
  const code = value instanceof Error ? value.message : String(value)
  error.value = messages[code] ?? '应用控制台加载失败，请检查连接后重试。'
  state.value = 'error'
}

function adapters(opened: OpenedDashboard): SDKAdapters {
  const id = opened.launch.dashboardSessionId
  const now = () => Math.max(opened.policy.now?.() ?? -Infinity, Number.isFinite(client!.now()) ? client!.now() : -Infinity)
  const files = createFileAccess({ now, expiresAt: opened.launch.expiresAt, async authorize(capability, signal) {
    const current = await client!.inspect(id, opened.launch.installationId, signal)
    if (['userId', 'organizationId', 'stationId', 'appId', 'installationId', 'contextRevision'].some(key =>
      current.context[key as keyof typeof current.context] !== opened.context[key as keyof typeof opened.context])) throw new Error('CONTEXT_CHANGED')
    if (!current.launch.grantedCapabilities.includes(capability)) throw new Error('CAPABILITY_DENIED')
  } })
  return {
    now,
    async navigate(input, signal) {
      await client!.inspect(id, opened.launch.installationId, signal)
      if (input.target === 'appcenter') {
        if (input.path !== '/') throw new Error('INVALID_PATH')
        await router.push(props.returnLocation ?? '/market')
      } else await router.push(dashboardLocation(opened.launch.installationId, input.path))
    },
    requestApp: (input, signal) => client!.request(id, input, signal),
    async callCapability(name, input, signal) {
      if (name === 'files.read') return files.read(input, signal)
      if (name === 'workspace.local' && new URLSearchParams(location.search).get('host') !== 'desktop') throw new Error('DESKTOP_REQUIRED')
      if (name !== 'navigation.openApp') return client!.capability(id, name, input, signal)
      const target = decodeAppTarget(input)
      const current = await client!.inspect(id, opened.launch.installationId, signal)
      if (!current.launch.grantedCapabilities.includes(name)) throw new Error('CAPABILITY_DENIED')
      const navigation = decodeNavigation(await createNavigationTransport().read(signal), current.context)
      if (signal.aborted) throw new Error('REQUEST_CANCELLED')
      if (navigation.projection.scope.authorizationEpoch !== current.context.contextRevision) throw new Error('CONTEXT_CHANGED')
      const failed = await router.push(appTargetLocation(target, navigation))
      if (failed) throw new Error('NAVIGATION_CANCELLED')
      return null
    },
    selectFiles: files.select,
    releaseFiles: files.clear,
    async openInstance(input, signal) {
      const launch = await client!.openInstance(id, opened.launch.installationId, input, signal)
      try {
        if (signal.aborted || opened.context.contextRevision !== selection.value?.context.contextRevision) throw new Error('REQUEST_CANCELLED')
        workspaceAppId.value = launch.appId
        workspaceInstanceName.value = launch.instanceName || launch.instanceId
        const failed = await router.push(instanceWorkspaceLocation(launch.installationId, launch.workspaceSessionId))
        if (failed) throw new Error('NAVIGATION_CANCELLED')
      } catch (cause) {
        await client!.closeWorkspace(launch.workspaceSessionId, AbortSignal.timeout(5000)).catch(()=>undefined)
        throw cause
      }
    },
    hasUserActivation: () => navigator.userActivation?.isActive === true,
    reportListenerError: () => { console.warn('APP_DASHBOARD_LISTENER_FAILED') },
  }
}

watch([() => props.active, () => props.installationId, () => route.query.path, workspaceId, actorScope, retry], async () => {
  const current = ++revision
  pending?.abort()
  if (scope !== actorScope.value) {
    scope = actorScope.value
    selection.value = null; previousSelection = null
    await Promise.allSettled([host.value?.revoke(), sessions?.revoke()])
  }
  if (current !== revision) return
  if (!props.active) { state.value = 'idle'; return }
  if (!scope) { showError(new Error('UNAUTHENTICATED')); return }
  if (workspaceId.value) { state.value = 'idle'; error.value = ''; return }
  workspaceInstanceName.value = ''
  state.value = 'resolving'; error.value = ''
  const controller = new AbortController(); pending = controller
  try {
    if (!sessions) throw new Error('DESKTOP_DASHBOARD_UNAVAILABLE')
    if (!installationPattern.test(props.installationId)) throw new Error('INVALID_PATH')
    const appRoute = dashboardRoutePath(route.query.path)
    await props.authorizeDashboard?.(props.installationId, controller.signal)
    if (current !== revision || controller.signal.aborted) return
    const opened = await sessions.select(props.installationId, controller.signal)
    if (current !== revision || controller.signal.aborted) return
    selection.value = { ...opened, authorizationEpoch: opened.context.contextRevision, route: appRoute,
      adapters: adapters(opened), releaseSession: () => sessions.release(opened.launch.installationId, opened.launch.dashboardSessionId) }
    state.value = 'resolved'
  } catch (cause) {
    if (current !== revision || controller.signal.aborted) return
    if (cause instanceof Error && ['DASHBOARD_DENIED', 'DASHBOARD_SESSION_EXPIRED', 'SESSION_EXPIRED', 'UNAUTHENTICATED'].includes(cause.message)) {
      selection.value = null; previousSelection = null
      await Promise.allSettled([host.value?.revoke(), sessions?.revoke()])
      if (current !== revision) return
    }
    showError(cause)
  }
}, { immediate: true })

async function cancelled() {
  const unused = selection.value
  selection.value = previousSelection
  if (unused && unused.launch.dashboardSessionId !== previousSelection?.launch.dashboardSessionId) await unused.releaseSession()
  await router.replace(previousSelection ? dashboardLocation(previousSelection.launch.installationId, previousSelection.route) : props.returnLocation ?? '/market')
}
function ready() { previousSelection = selection.value; state.value = 'ready' }
function dashboardError(code: string) { if (!workspaceId.value) showError(code) }
async function prepareNavigation(to: RouteLocationNormalized) {
  const staysInAppCenter = to.path === '/market' || to.name === 'app-dashboard' || to.name === 'app-instance'
  if (!staysInAppCenter && !await (host.value?.prepareLeave() ?? true)) return false
  if (workspaceId.value && to.params.workspaceSessionId !== workspaceId.value) return await instanceHost.value?.prepareClose() ?? true
  return true
}
onBeforeRouteLeave(prepareNavigation)
onBeforeRouteUpdate(prepareNavigation)
onBeforeUnmount(() => { revision++; pending?.abort(); void sessions?.revoke() })
</script>

<template>
  <main v-show="active" class="main embed-active" :class="{ 'workspace-active': Boolean(workspaceId) }">
    <div class="workspace">
      <div class="dashboard-context"><nav class="dashboard-breadcrumb" aria-label="面包屑">
        <a v-if="workspaceId || navigationChild" :href="router.resolve(dashboardLocation(installationId)).href" @click.prevent="router.push(dashboardLocation(installationId))">{{ title }}</a>
        <span v-else class="breadcrumb-current">{{ title }}</span>
        <template v-if="!workspaceId && navigationChild"><span aria-hidden="true" class="breadcrumb-sep"> / </span><span class="breadcrumb-current" aria-current="page">{{ navigationChild.title }}</span></template>
        <template v-if="workspaceId && workspaceInstanceName">
          <span aria-hidden="true" class="breadcrumb-sep"> / </span>
          <span class="breadcrumb-current">{{ workspaceInstanceName }}</span>
        </template>
      </nav></div>
      <header v-if="!workspaceId && !visible" class="heading">
        <div class="heading-left">
          <h1>{{ navigationChild?.title ?? title }}</h1>
        </div>
      </header>
      <p v-if="state === 'resolving'" role="status">正在确认应用版本与访问权限…</p>
      <div v-if="state === 'error'" class="dashboard-error" role="alert"><p>{{ error }}</p><button class="btn" @click="retry++">重试</button></div>
      <AppDashboardHost ref="host" v-show="visible" :selection="selection" :visible="visible"
        :confirm-discard="confirmDiscard"
        @ready="ready" @error="dashboardError" @cancelled="cancelled" @retry="retry++" />
      <AppInstanceHost v-if="workspaceId && actorScope" :key="actorScope + workspaceId" ref="instanceHost"
        :workspace-session-id="workspaceId" :installation-id="installationId"
        @loaded="info => { workspaceAppId = info.appId; workspaceInstanceName = info.instanceName }" />
    </div>
  </main>
</template>

<style scoped>
.dashboard-context{display:flex;align-items:center;justify-content:space-between;gap:16px;margin-bottom:6px}.dashboard-context .dashboard-breadcrumb{margin:0}
.main.embed-active.workspace-active {
  padding: 16px 24px 16px 24px;
}
.dashboard-breadcrumb {
  display: flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  color: var(--muted);
  margin-bottom: 6px;
}
.workspace-active .dashboard-breadcrumb {
  margin-bottom: 10px;
}
.dashboard-breadcrumb a {
  color: var(--muted);
  text-decoration: none;
  transition: color 0.15s ease;
}
.dashboard-breadcrumb a:hover {
  color: var(--ink);
}
.dashboard-breadcrumb .breadcrumb-sep {
  color: var(--muted-2);
  font-size: 12px;
}
.dashboard-breadcrumb .breadcrumb-current {
  color: var(--ink);
  font-weight: 500;
}
.heading {
  margin-bottom: 12px;
}
.heading-left h1 {
  font-size: 26px;
  font-weight: 650;
  letter-spacing: -0.4px;
  line-height: 1.25;
  color: var(--ink);
  margin: 0;
}
.heading-left p {
  margin: 4px 0 0 0;
  font-size: 13px;
  color: var(--muted);
}
.dashboard-error { padding: 20px; border: 1px solid var(--line); border-radius: 12px; background: var(--card); }
</style>
