<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import vfLogo from '../assets/verdantflare-logo.png'
import { useHostStore } from '../platform/hostStore'
import { useNavigationStore } from '../platform/navigationStore'
import { browserDashboardPath, dashboardLocation } from '../app-dashboard/routes'
import { menuState } from '../navigation/menu'
import { navigationRouteNode } from '../app-dashboard/navigation-routes'
import { legacySidebarEntries, legacySidebarSelected } from '../app-dashboard/legacy-sidebar'
import { stationNavigationTarget, legacyStationLocation } from '../platform/sidebarActions'
import NavigationRow from './NavigationRow.vue'
import type { NavigationEntry } from '../navigation/controller'
import { useAppCenterStore } from '../app-center/store'
import { appCenterNavigationEntries } from '../app-center/navigation'
import { reasonLabel } from '../app-center/labels'
import { directoryLocation, directoryView, isDirectoryRoute } from '../app-center/directory-route'
import { selectCards } from '../app-center/controller'
import { hostTheme } from '../theme/hostTheme'
import { themeLabels } from '../theme/preference'
import type { ThemeMode } from '../theme/preference'
import { hostContentPreferences } from '../platform/contentPreferences'
import NavigationIcon from './NavigationIcon.vue'

const host = useHostStore(), navigation = useNavigationStore(), router = useRouter(), route = useRoute()
const appcenter = import.meta.env.VITE_APPCENTER_MODE === 'unified' ? useAppCenterStore() : null
// /market also hosts legacy application routes; directory ancestry stops at the directory.
const appCenterActive = computed(() => isDirectoryRoute(route))
const expanded = ref(new Set<string>(['directory']))
const branchKey = (entry: NavigationEntry) => entry.installationId + '/' + entry.entryId
function toggleBranch(key: string) { const next = new Set(expanded.value); next.has(key) ? next.delete(key) : next.add(key); expanded.value = next }
const directoryMenus = [{ view: 'installed', label: '我的应用' }, { view: 'all', label: '所有应用' }] as const
const directoryCounts = computed(() => selectCards(appcenter?.state.value ?? null, '', 'all', 'all', !appcenter?.state.stale).counts)
watch(() => expanded.value.has('directory'), value => appcenter?.setVisible('directory-menu', value), { immediate: true })
const currentAppId = computed(() => typeof route.params.appId === 'string' ? route.params.appId : navigation.entries.find(e => e.installationId === route.params.installationId)?.appId)
const appPath = computed(() => { try { return browserDashboardPath(route, currentAppId.value) } catch { return '/' } })
const entrySelected = (entry: NavigationEntry) => ['app-dashboard', 'app-dashboard-legacy', 'app-instance'].includes(String(route.name)) && (route.params.appId === entry.appId || route.params.installationId === entry.installationId)
const entryMenu = (entry: NavigationEntry) => menuState(navigationRouteNode(entry), appPath.value, entrySelected(entry) && ['app-dashboard', 'app-dashboard-legacy'].includes(String(route.name)))
const childSelected = (entry: NavigationEntry, entryId: string) => entryMenu(entry).selectedChild === entryId
let expandedForRoute = ''
watch(() => [route.fullPath, navigation.pinned] as const, () => {
  const keys = new Set(['directory', ...navigation.pinned.map(branchKey)])
  const next = new Set([...expanded.value].filter(key => keys.has(key)))
  const active = appCenterActive.value ? 'directory' : navigation.pinned.find(entry => entrySelected(entry) && entry.children?.length)
  const key = typeof active === 'string' ? active : active ? branchKey(active) : ''
  const signature = route.fullPath + '/' + key
  if (key && signature !== expandedForRoute) next.add(key)
  expandedForRoute = signature
  if (next.size !== expanded.value.size || [...next].some(k => !expanded.value.has(k))) {
    expanded.value = next
  }
}, { immediate: true })
const entryLocation = (entry: NavigationEntry, path = entry.route) => dashboardLocation(entry.appId, path)
function entryReason(entry: NavigationEntry) { return appcenter ? reasonLabel('openReason' in entry ? entry.openReason as string : null) : entry.availability === 'offline' ? '应用离线' : '应用状态待确认' }
const feedback = ref('')
async function openEntry(entry: NavigationEntry, target = entry.route) {
  feedback.value = ''
  if (!target || !(target === entry.route || entry.children?.some(child => child.route === target))) return
  try {
    if (appcenter) {
      const value = navigation.state.value
      if (!value) return
      const current = appCenterNavigationEntries(value, appcenter.state).find(item => item.installationId === entry.installationId && item.entryId === entry.entryId && item.appVersion === entry.appVersion)
      if (!current?.cardKey || !current.bindingRevision) { feedback.value = entryReason(entry); return }
      if (!(target === current.route || current.children?.some(child => child.route === target))) return
      appcenter.pinOpen(entry.installationId, current.bindingRevision)
      const failed = await router.push(entryLocation(current, target))
      if (failed) appcenter.takeOpenBinding(entry.installationId)
    } else await router.push(entryLocation(entry, target))
  } catch { feedback.value = '应用暂时无法打开，请刷新后重试。' }
}
function refreshNavigation() { if (host.connected) { void navigation.refresh(); void host.fetchCluster() } }
function clearNavigation() { navigation.clear(); expanded.value = new Set(['directory']); feedback.value = ''; dialog.value?.close() }
watch(() => JSON.stringify([host.identity.user_id, host.identity.organization_id, host.identity.station_id, host.identity.session_id, host.identity.policy_version, host.identity.revocation_version, host.identity.roles, host.identity.scopes]), authorizationKey => {
  const identity = host.identity
  hostContentPreferences.setSessionIdentity(
    identity.user_id && identity.organization_id ? { userId: identity.user_id, organizationId: identity.organization_id } : null,
  )
  navigation.clear(identity.user_id && identity.organization_id && identity.station_id ? { userId: identity.user_id, organizationId: identity.organization_id, stationId: identity.station_id } : null, authorizationKey)
  expanded.value = new Set(['directory']); feedback.value = ''
  refreshNavigation()
}, { immediate: true })

const sidebar = ref<HTMLElement>(), sticky = ref(false)
let resizeObserver: ResizeObserver | undefined
function measureSidebar() { sticky.value = !!sidebar.value && sidebar.value.scrollHeight <= window.innerHeight }
const loginStatus = computed(() => host.connected ? '已登录' : host.sessionStatus === 'loading' ? '读取中' : host.sessionStatus === 'error' ? '读取失败' : '未登录')
const stationStatus = computed(() => !host.connected ? '未连接' : host.coreStatus === '就绪' ? '已就绪' : host.coreStatus === '未就绪' ? '未就绪' : host.healthLoading ? '检查中' : '待确认')
const dialog = ref<HTMLDialogElement>()
const dialogKind = ref<'help' | 'station'>('help')
const dialogTitle = computed(() => ({ help: '帮助', station: 'Station 状态' })[dialogKind.value])
function showDialog(kind: typeof dialogKind.value) {
  dialogKind.value = kind
  dialog.value?.showModal()
}
function account() { void router.push('/settings') }
function station() {
  const entry = stationNavigationTarget(navigation.entries)
  if (host.connected && entry) void openEntry(entry, entry.route || entry.children?.[0]?.route)
  else if (host.connected && navigation.legacy) void router.push(legacyStationLocation)
  else showDialog('station')
}
const themeButton = ref<HTMLButtonElement>(), themeMenu = ref<HTMLElement>(), themeExpanded = ref(false)
const modes: ThemeMode[] = ['light', 'dark', 'system']
function positionTheme() {
  const menu = themeMenu.value, button = themeButton.value
  if (!menu || !button || !menu.matches(':popover-open')) return
  const rect = button.getBoundingClientRect()
  menu.style.width = Math.min(220, innerWidth - 16) + 'px'
  menu.style.left = Math.max(8, Math.min(rect.left, innerWidth - menu.offsetWidth - 8)) + 'px'
  menu.style.top = Math.max(8, rect.top - menu.offsetHeight - 8 >= 8 ? rect.top - menu.offsetHeight - 8 : Math.min(rect.bottom + 8, innerHeight - menu.offsetHeight - 8)) + 'px'
}
function toggleTheme() {
  const menu = themeMenu.value
  if (!menu) return
  if (menu.matches(':popover-open')) menu.hidePopover()
  else { menu.showPopover(); positionTheme(); menu.querySelector<HTMLElement>('[aria-checked="true"]')?.focus() }
}
function themeToggled(event: Event) {
  themeExpanded.value = (event as ToggleEvent).newState === 'open'
  if (!themeExpanded.value) themeButton.value?.focus()
}
function chooseTheme(mode: ThemeMode) { hostTheme.select(mode); themeMenu.value?.hidePopover() }
function themeKeys(event: KeyboardEvent) {
  const items = Array.from(themeMenu.value?.querySelectorAll<HTMLButtonElement>('[role="menuitemradio"]') ?? [])
  const index = items.indexOf(document.activeElement as HTMLButtonElement)
  if (['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(event.key)) {
    event.preventDefault()
    items[event.key === 'Home' ? 0 : event.key === 'End' ? 2 : (index + (event.key === 'ArrowDown' ? 1 : 2)) % 3]?.focus()
  }
  if (event.key === 'Escape') { event.preventDefault(); themeMenu.value?.hidePopover() }
}
function resized() { measureSidebar(); positionTheme() }
onMounted(() => {
  void host.init()
  window.addEventListener('studio:logout', clearNavigation)
  window.addEventListener('resize', resized)
  window.addEventListener('scroll', positionTheme, { passive: true })
  resizeObserver = new ResizeObserver(measureSidebar)
  if (sidebar.value) resizeObserver.observe(sidebar.value)
  measureSidebar()
})
onBeforeUnmount(() => {
  appcenter?.setVisible('directory-menu', false); resizeObserver?.disconnect()
  window.removeEventListener('studio:logout', clearNavigation)
  window.removeEventListener('resize', resized)
  window.removeEventListener('scroll', positionTheme); clearNavigation()
})
</script>

<template>
  <aside ref="sidebar" class="host-sidebar" :class="{ sticky }" aria-label="Studio 全局导航">
    <a class="brand" :href="router.resolve(directoryLocation('installed')).href" aria-label="青焰 VerdantFlare Studio，回到我的应用" @click.prevent="router.push(directoryLocation('installed'))">
      <img :src="vfLogo" alt="" width="30" height="37"><span class="brand-copy"><span class="brand-name">青焰 · VerdantFlare</span><small>STUDIO OS</small></span>
    </a>
    <nav aria-label="业务导航">
      <NavigationRow label="应用中心" icon="app" children :parent-current="appCenterActive" :expanded="expanded.has('directory')" controls="directory-children" @toggle="toggleBranch('directory')" />
      <div v-show="expanded.has('directory')" id="directory-children" class="children">
        <NavigationRow v-for="item in directoryMenus" :key="item.view" child :label="item.label" :href="router.resolve(directoryLocation(item.view)).href"
          :current="appCenterActive && directoryView(route.query.view) === item.view" :count="directoryCounts[item.view] ?? '—'" @activate="router.push(directoryLocation(item.view))" />
      </div>
      <div class="dynamic-menu">
        <template v-if="navigation.legacy">
          <NavigationRow v-for="item in legacySidebarEntries" :key="item.title" :label="item.title" :icon="item.icon"
            :current="legacySidebarSelected(item, route)" :href="router.resolve(item).href" @activate="router.push(item)" />
        </template>
        <template v-else v-for="entry in navigation.pinned" :key="branchKey(entry)">
          <NavigationRow :label="entry.title" :icon="entry.iconRef" :children="entryMenu(entry).hasChildren"
            :href="entryMenu(entry).navigable ? router.resolve(entryLocation(entry)).href : undefined"
            :current="entryMenu(entry).parentCurrent" :parent-current="entrySelected(entry)" :expanded="expanded.has(branchKey(entry))"
            :controls="'children-' + entry.installationId" :badge="entry.availability !== 'ready' ? (entry.availability === 'offline' ? '离线' : '待确认') : undefined"
            @activate="openEntry(entry)" @toggle="toggleBranch(branchKey(entry))" />
          <div v-if="entry.children?.length" :hidden="!expanded.has(branchKey(entry))" :inert="!expanded.has(branchKey(entry))" :id="'children-' + entry.installationId" class="children">
            <NavigationRow v-for="child in entry.children" :key="child.entryId" child :label="child.title" :href="router.resolve(entryLocation(entry, child.route)).href"
              :current="childSelected(entry, child.entryId)" @activate="openEntry(entry, child.route)" />
          </div>
        </template>
        <div v-if="navigation.state.status === 'loading'" class="nav-state" role="status" aria-busy="true">正在加载应用菜单…</div>
        <div v-else-if="navigation.state.value?.mode === 'preinstalled' && !navigation.pinned.length && !navigation.state.error" class="nav-state">暂无可显示的应用菜单</div>
        <div v-if="navigation.state.error" role="status" class="nav-state"><p>{{ navigation.state.error }}</p><button class="small-button" @click="navigation.refresh()">重试</button></div>
        <p v-if="feedback" class="nav-state" role="status">{{ feedback }}</p>
      </div>
    </nav>
    <div class="side-spacer"></div>
    <div class="utilities">
      <button class="utility account" :title="host.displayName + '，' + loginStatus" :aria-label="host.displayName + '，' + loginStatus + '，账号与设置'" :aria-current="route.path === '/settings' ? 'page' : undefined" @click="account">
        <span class="avatar">{{ host.avatar }}</span><span class="utility-label username">{{ host.displayName }}</span><span class="status-text">{{ loginStatus }}</span><NavigationIcon name="chevron" />
      </button>
      <button class="utility station" :title="host.clusterName + '，' + stationStatus" :aria-label="host.clusterName + '，' + stationStatus + '，查看状态'" @click="station">
        <NavigationIcon name="server" /><span class="utility-label station-name">{{ host.clusterName }}</span><span class="status-text">{{ stationStatus }}</span><i class="status-dot" :class="{ ready: stationStatus === '已就绪' }" aria-hidden="true"></i>
      </button>
      <button ref="themeButton" class="utility theme-row" :title="'主题：' + themeLabels[hostTheme.mode.value]" :aria-label="'主题：' + themeLabels[hostTheme.mode.value] + '，更改主题'"
              aria-haspopup="menu" :aria-expanded="themeExpanded" aria-controls="theme-menu" @click="toggleTheme">
        <NavigationIcon name="theme" /><span class="utility-label">主题</span><span class="status-text">{{ themeLabels[hostTheme.mode.value] }}</span><NavigationIcon name="chevron" />
      </button>
      <p v-if="hostTheme.mode.value === 'system'" class="system-hint">当前{{ themeLabels[hostTheme.actual.value] }}</p>
      <p v-if="hostTheme.warning.value" class="nav-state" role="status">{{ hostTheme.warning.value }}</p>
      <button class="utility help" aria-label="帮助" title="帮助" @click="showDialog('help')"><NavigationIcon name="help" /><span class="utility-label">帮助</span></button>
    </div>
  </aside>
  <Teleport to="body">
    <div ref="themeMenu" id="theme-menu" class="theme-popover" popover role="menu" aria-label="主题" @toggle="themeToggled" @keydown="themeKeys">
      <p>主题</p><button v-for="mode in modes" :key="mode" role="menuitemradio" :aria-checked="hostTheme.mode.value === mode" @click="chooseTheme(mode)">{{ themeLabels[mode] }}<span v-if="hostTheme.mode.value === mode" aria-hidden="true">✓</span></button>
    </div>
    <dialog ref="dialog" class="navigation-dialog" aria-labelledby="navigation-dialog-title">
      <div class="dialog-head"><h2 id="navigation-dialog-title">{{ dialogTitle }}</h2><button class="small-button" aria-label="关闭" @click="dialog?.close()">关闭</button></div>
      <template v-if="dialogKind === 'station'">
        <p>{{ host.clusterName }}：{{ stationStatus }}</p><p>{{ host.connected ? '资源管理入口当前不可用，请在应用中心检查安装与访问权限。' : '请登录后查看 Station 的当前状态。' }}</p>
        <div class="dialog-actions"><button class="small-button" @click="refreshNavigation">重新检查</button></div>
      </template>
      <template v-else>
        <h3>找到你的工作</h3><p>应用中心提供“我的应用”和“所有应用”。侧栏应用可直接打开，有子菜单时点击箭头展开或收起。</p>
        <h3>管理侧栏</h3><p>在已安装应用的详情中使用“在侧栏显示”。隐藏入口不会停止应用。</p>
        <h3>了解当前状态</h3><p>“已就绪”表示 Station 健康检查通过；“已登录”表示账号会话有效。</p>
        <h3>调整外观</h3><p>使用侧栏主题入口选择浅色、深色或跟随系统。业务页面统一跟随。</p>
        <h3>键盘操作</h3><p>Tab 移动焦点，Enter 打开入口。主题选项可用方向键选择，Escape 关闭浮层。</p>
      </template>
    </dialog>
  </Teleport>
</template>

<style scoped>
.host-sidebar { align-self:start; min-height:100dvh; padding:30px 14px 17px; background:var(--aside); border-right:0.5px solid var(--vf-hairline-outer); box-shadow:inset -0.5px 0 0 0 var(--vf-hairline-inner); display:flex; flex-direction:column; }
.host-sidebar.sticky { position:sticky; top:0; }
.brand { display:grid; grid-template-columns:30px minmax(0,1fr); align-items:center; gap:6px; padding:0 12px 0 6px; margin-bottom:38px; }
.brand img { width:30px; height:37px; object-fit:contain; justify-self:center; }
.brand-name { display:block; font-weight:650; white-space:nowrap; font-size:14px; letter-spacing:-.35px; }
.brand small { display:block; font-size:9px; letter-spacing:2.5px; color:var(--muted); margin-top:3px; }
.utility { display:grid; grid-template-columns:30px minmax(0,1fr) auto; align-items:center; column-gap:6px; width:100%; min-height:40px; padding:9px 12px 9px 6px; border-radius:7px; text-align:left; color:var(--muted); line-height:22px; transition:background var(--vf-dur-fast,140ms) var(--vf-ease-out),color var(--vf-dur-fast,140ms) var(--vf-ease-out); }
.utility[aria-current="page"],.utility:hover { background:var(--shell-button); color:var(--ink); }
.utility>.icon,.avatar { justify-self:center; }
.children[hidden] { display:none; }
.children { padding:2px 0 7px; }
.dynamic-menu { margin-top:6px; }
.side-spacer { flex:1; min-height:38px; }
.utilities { border-top:0.5px solid var(--vf-hairline-outer); padding-top:13px; }
.utility { min-height:36px; font-size:12px; grid-template-columns:30px minmax(0,1fr) 44px 14px; }
.utility>.icon { width:17px; height:17px; }
.utility>.chevron { width:14px; height:14px; }
.utility-label { min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.username,.station-name { color:var(--ink); }
.account { margin:4px 0 10px; }
.avatar { width:30px; height:30px; border-radius:50%; background:var(--shell-button); display:grid; place-items:center; color:var(--ink); font-size:12px; }
.status-text { font-size:11px; text-align:left; line-height:16px; }
.status-dot { justify-self:center; width:5px; height:5px; border-radius:50%; background:var(--muted); }
.status-dot.ready { background:var(--accent); }
.system-hint { font-size:10px; color:var(--muted); padding:0 12px 5px 42px; }
.nav-state { font-size:12px; color:var(--muted); padding:14px 12px; }
.nav-state p { margin-bottom:8px; }
.small-button { border:0.5px solid var(--vf-hairline-outer); border-radius:5px; padding:5px 10px; font-size:12px; background:var(--shell-button); color:var(--ink); }
.theme-popover { position:fixed; inset:auto; margin:0; padding:6px; background:var(--panel); color:var(--ink); border:0.5px solid var(--vf-hairline-outer); border-radius:10px; box-shadow:inset 0 0 0 0.5px var(--vf-hairline-inner), var(--shadow); }
.theme-popover p { padding:7px 11px; font-size:10px; color:var(--muted); }
.theme-popover button { width:100%; display:flex; justify-content:space-between; padding:9px 11px; border-radius:5px; font-size:12px; }
.theme-popover button:hover,.theme-popover button:focus { background:var(--shell-button); }
.theme-popover button span { color:var(--accent); }
.navigation-dialog { width:min(480px,calc(100vw - 32px)); padding:28px; border:0.5px solid var(--vf-hairline-outer); border-radius:14px; background:var(--panel); color:var(--ink); margin:auto; box-shadow:inset 0 0 0 0.5px var(--vf-hairline-inner), var(--shadow); }
.navigation-dialog::backdrop { background:var(--shell-button); backdrop-filter:blur(2px); }
.dialog-head { display:flex; align-items:center; justify-content:space-between; gap:20px; margin-bottom:22px; }
.dialog-head h2 { font-size:20px; font-weight:600; }
.navigation-dialog p { font-size:13px; margin:10px 0; color:var(--muted); }
.navigation-dialog h3 { font-size:13px; margin:20px 0 5px; }
.dialog-actions { display:flex; justify-content:flex-end; gap:10px; margin-top:24px; }
@media(max-width:700px) {
 .host-sidebar { padding:22px 7px 12px; }
 .brand { display:flex; padding:0; justify-content:center; margin-bottom:28px; }
 .brand img { width:28px; height:34px; }
 .brand-copy,.utility-label,.status-text,.utility>.chevron,.status-dot,.system-hint { display:none; }
 .utility { display:flex; justify-content:center; padding:10px 8px; }
 .side-spacer { min-height:28px; }
 .nav-state { font-size:10px; padding:10px 2px; overflow-wrap:anywhere; }
 .nav-state .small-button { padding:3px; }
}
@media(prefers-reduced-motion:reduce) { * { transition:none!important; } }
</style>
