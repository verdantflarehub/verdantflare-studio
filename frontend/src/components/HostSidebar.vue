<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, ref, watch } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import vfLogo from '../assets/verdantflare-logo.png'
import { useHostStore } from '../platform/hostStore'
import { request } from '../platform/client'
import { useNavigationStore } from '../platform/navigationStore'
import { browserDashboardPath, dashboardLocation } from '../app-dashboard/routes'
import { sameNavigationPath, selectedNavigationChild } from '../navigation/controller'
import type { NavigationEntry } from '../navigation/controller'
import { useAppCenterStore } from '../app-center/store'
import { appCenterNavigationEntries } from '../app-center/navigation'
import { reasonLabel } from '../app-center/labels'
import { directoryLocation, directoryView } from '../app-center/directory-route'
import { selectCards } from '../app-center/controller'
import { hostTheme } from '../theme/hostTheme'
import { themeLabels } from '../theme/preference'
import type { ThemeMode } from '../theme/preference'
import { formatRemainingTime, hostContentPreferences } from '../platform/contentPreferences'
import NavigationIcon from './NavigationIcon.vue'

const host = useHostStore(), navigation = useNavigationStore(), router = useRouter(), route = useRoute()
const appcenter = import.meta.env.VITE_APPCENTER_MODE === 'unified' ? useAppCenterStore() : null
// /market also hosts legacy application routes; directory ancestry stops at the directory.
const appCenterActive = computed(() => route.path === '/market' && !['video', 'image', 'music', 'speech', 'blender', 'comfyui'].some(key => route.query[key] !== undefined))
const expanded = ref(new Set<string>(['directory']))
const branchKey = (entry: NavigationEntry) => entry.installationId + '/' + entry.entryId
function toggleBranch(key: string) { const next = new Set(expanded.value); next.has(key) ? next.delete(key) : next.add(key); expanded.value = next }
const directoryMenus = [{ view: 'installed', label: '我的应用' }, { view: 'all', label: '所有应用' }] as const
const directoryCounts = computed(() => selectCards(appcenter?.state.value ?? null, '', 'all', 'all', !appcenter?.state.stale).counts)
watch(() => expanded.value.has('directory'), value => appcenter?.setVisible('directory-menu', value), { immediate: true })
const currentAppId = computed(() => typeof route.params.appId === 'string' ? route.params.appId : navigation.entries.find(e => e.installationId === route.params.installationId)?.appId)
const appPath = computed(() => { try { return browserDashboardPath(route, currentAppId.value) } catch { return '/' } })
const entrySelected = (entry: NavigationEntry) => ['app-dashboard', 'app-dashboard-legacy', 'app-instance'].includes(String(route.name)) && (route.params.appId === entry.appId || route.params.installationId === entry.installationId)
const childSelected = (entry: NavigationEntry, path: string) => ['app-dashboard', 'app-dashboard-legacy'].includes(String(route.name)) && entrySelected(entry) && selectedNavigationChild(entry.children, appPath.value)?.route === path
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
// A parent opens its own page only when it declares a route that no child
// repeats (for example a World overview); otherwise clicking it expands.
const ownsPage = (entry: NavigationEntry) => !!entry.route && !!entry.children?.length && !entry.children.some(child => sameNavigationPath(child.route, entry.route))
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
// Explicit legacy mode alone retains the old direct entry compatibility.
const legacyEntries = [
  { title: '项目管理', icon: 'folder', path: '/projects', query: {} },
  { title: '数字世界', icon: 'globe', path: '/projects', query: { mode: 'world' } },
  { title: '资源管理', icon: 'server', path: '/resources', query: {} },
  { title: '网络出口', icon: 'network', path: '/station/proxies', query: {} },
]
function legacySelected(item: typeof legacyEntries[number]) { return route.path === item.path && (route.query.mode === 'world') === (item.query.mode === 'world') }

function refreshNavigation() { if (host.connected) { void navigation.refresh(); void host.fetchCluster() } }
function clearNavigation() { navigation.clear(); expanded.value = new Set(['directory']); feedback.value = ''; dialog.value?.close() }
const nsfwSnap = ref(hostContentPreferences.get())
const nsfwStatusText = computed(() => nsfwSnap.value.includeNsfw ? `已开启，${formatRemainingTime(nsfwSnap.value.remainingMs)}` : '关闭')
let unsubNsfw: (() => void) | undefined

watch(() => JSON.stringify([host.identity.user_id, host.identity.organization_id, host.identity.station_id, host.identity.session_id, host.identity.policy_version, host.identity.revocation_version, host.identity.roles, host.identity.scopes]), authorizationKey => {
  const identity = host.identity
  nsfwSnap.value = hostContentPreferences.setSessionIdentity(
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
const dialogKind = ref<'account' | 'help' | 'station' | 'logout'>('help')
const dialogTitle = computed(() => ({ account: '账号详情', help: '帮助', station: 'Station 状态', logout: '退出登录？' })[dialogKind.value])
const logoutBusy = ref(false), logoutError = ref('')
function showDialog(kind: typeof dialogKind.value) {
  dialogKind.value = kind
  logoutError.value = ''
  nsfwSnap.value = hostContentPreferences.get()
  dialog.value?.showModal()
}
function openSettings() {
  dialog.value?.close()
  void router.push('/settings')
}
async function account() {
  if (host.connected) showDialog('account')
  else if (host.sessionStatus === 'error') await host.init()
  else host.loginOpen = true
}
function station() {
  const entry = navigation.entries.find(item => item.appId === 'resources')
  if (host.connected && entry) void openEntry(entry, entry.route || entry.children?.[0]?.route)
  else if (host.connected && navigation.legacy) void router.push('/resources')
  else showDialog('station')
}
async function logout() {
  if (logoutBusy.value) return
  logoutBusy.value = true; logoutError.value = ''
  nsfwSnap.value = hostContentPreferences.disable('logout')
  try {
    const result = await request({ path: 'logout', method: 'POST' }, { realOnly: true })
    if (!result.ok && result.status !== 401) throw new Error('logout')
    host.setIdentity({}); try { sessionStorage.removeItem('studio.operations') } catch { /* optional browser cache */ }
    window.dispatchEvent(new CustomEvent('studio:logout'))
    dialog.value?.close(); host.loginOpen = true
  } catch { logoutError.value = '退出未能确认，请重试。' }
  finally { logoutBusy.value = false }
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
  unsubNsfw = hostContentPreferences.subscribe(next => { nsfwSnap.value = next })
  window.addEventListener('studio:logout', clearNavigation)
  window.addEventListener('resize', resized)
  window.addEventListener('scroll', positionTheme, { passive: true })
  resizeObserver = new ResizeObserver(measureSidebar)
  if (sidebar.value) resizeObserver.observe(sidebar.value)
  measureSidebar()
})
onBeforeUnmount(() => {
  unsubNsfw?.()
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
      <button class="nav-row" :class="{ 'parent-current': appCenterActive }" aria-label="应用中心" title="应用中心" :aria-expanded="expanded.has('directory')" aria-controls="directory-children" @click="toggleBranch('directory')">
        <NavigationIcon name="app" /><span class="label">应用中心</span><NavigationIcon name="chevron" />
      </button>
      <div v-show="expanded.has('directory')" id="directory-children" class="children">
        <a v-for="item in directoryMenus" :key="item.view" class="nav-row" :href="router.resolve(directoryLocation(item.view)).href"
           :aria-current="appCenterActive && directoryView(route.query.view) === item.view ? 'page' : undefined" @click.prevent="router.push(directoryLocation(item.view))">
          <span class="label">{{ item.label }}</span><span class="count">{{ directoryCounts[item.view] ?? '—' }}</span>
        </a>
      </div>
      <div class="dynamic-menu">
        <template v-if="navigation.legacy">
          <a v-for="item in legacyEntries" :key="item.title" class="nav-row" :title="item.title" :aria-label="item.title" :aria-current="legacySelected(item) ? 'page' : undefined"
             :href="router.resolve(item).href" @click.prevent="router.push(item)"><NavigationIcon :name="item.icon" /><span class="label">{{ item.title }}</span></a>
        </template>
        <template v-else v-for="entry in navigation.pinned" :key="branchKey(entry)">
          <div v-if="ownsPage(entry)" class="branch-row">
            <a class="nav-row" :href="router.resolve(entryLocation(entry)).href" :class="{ 'parent-current': entrySelected(entry) }" :aria-current="entrySelected(entry) && appPath === '/' ? 'page' : undefined" @click.prevent="openEntry(entry)"><NavigationIcon :name="entry.iconRef" /><span class="label">{{ entry.title }}</span><span v-if="entry.availability !== 'ready'" class="offline-badge">{{ entry.availability === 'offline' ? '离线' : '待确认' }}</span></a>
            <button class="branch-toggle" :aria-label="(expanded.has(branchKey(entry)) ? '收起' : '展开') + entry.title" :aria-expanded="expanded.has(branchKey(entry))" :aria-controls="'children-' + entry.installationId" @click="toggleBranch(branchKey(entry))"><NavigationIcon name="chevron" /></button>
          </div>
          <button v-else-if="entry.children?.length" class="nav-row" :class="{ 'parent-current': entrySelected(entry) }" :title="entry.title" :aria-label="entry.title"
                  :aria-expanded="expanded.has(branchKey(entry))" :aria-controls="'children-' + entry.installationId" @click="toggleBranch(branchKey(entry))">
            <NavigationIcon :name="entry.iconRef" /><span class="label">{{ entry.title }}</span><span v-if="entry.availability !== 'ready'" class="offline-badge">{{ entry.availability === 'offline' ? '离线' : '待确认' }}</span><NavigationIcon name="chevron" />
          </button>
          <a v-else class="nav-row" :href="router.resolve(entryLocation(entry)).href" :title="entry.title" :aria-label="entry.title"
             :class="{ 'parent-current': entrySelected(entry) }" :aria-current="entrySelected(entry) && (!entry.children?.length || appPath === entry.route) ? 'page' : undefined" @click.prevent="openEntry(entry)">
            <NavigationIcon :name="entry.iconRef" /><span class="label">{{ entry.title }}</span><span v-if="entry.availability !== 'ready'" class="offline-badge">{{ entry.availability === 'offline' ? '离线' : '待确认' }}</span>
          </a>
          <div v-if="entry.children?.length" :hidden="!expanded.has(branchKey(entry))" :inert="!expanded.has(branchKey(entry))" :id="'children-' + entry.installationId" class="children">
            <a v-for="child in entry.children" :key="child.entryId" class="nav-row" :href="router.resolve(entryLocation(entry, child.route)).href"
               :aria-current="childSelected(entry, child.route) ? 'page' : undefined" @click.prevent="openEntry(entry, child.route)"><span class="label">{{ child.title }}</span></a>
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
      <button class="utility account" :title="host.displayName + '，' + loginStatus" :aria-label="host.displayName + '，' + loginStatus + '，查看账号详情'" aria-haspopup="dialog" @click="account">
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
    <dialog ref="dialog" class="navigation-dialog" aria-labelledby="navigation-dialog-title" @cancel="logoutBusy && $event.preventDefault()">
      <div class="dialog-head"><h2 id="navigation-dialog-title">{{ dialogTitle }}</h2><button class="small-button" :disabled="logoutBusy" aria-label="关闭" @click="dialog?.close()">关闭</button></div>
      <template v-if="dialogKind === 'account'">
        <p>用户：{{ host.displayName }}</p><p>组织：{{ host.identity.organization_name || '未配置组织' }}</p><p>登录状态：{{ loginStatus }}</p>
        <p>NSFW 显示：{{ nsfwStatusText }}</p>
        <div class="dialog-actions"><button class="small-button" @click="openSettings">设置</button><button class="small-button" @click="dialogKind = 'logout'">退出登录</button></div>
      </template>
      <template v-else-if="dialogKind === 'logout'">
        <p>确认退出当前账号？</p><p v-if="logoutError" role="alert">{{ logoutError }}</p>
        <div class="dialog-actions"><button class="small-button" :disabled="logoutBusy" @click="dialogKind = 'account'">取消</button><button class="small-button" :disabled="logoutBusy" @click="logout">{{ logoutBusy ? '正在退出…' : '确认退出' }}</button></div>
      </template>
      <template v-else-if="dialogKind === 'station'">
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
.nav-row,.utility { display:grid; grid-template-columns:30px minmax(0,1fr) auto; align-items:center; column-gap:6px; width:100%; min-height:40px; padding:9px 12px 9px 6px; border-radius:7px; text-align:left; color:var(--muted); line-height:22px; transition:background var(--vf-dur-fast, 140ms) var(--vf-ease-out), color var(--vf-dur-fast, 140ms) var(--vf-ease-out); }
.nav-row:hover,.utility:hover { background:var(--shell-button); color:var(--ink); }
.nav-row>.icon,.utility>.icon,.avatar { justify-self:center; }
.nav-row.parent-current { color:var(--ink); font-weight:600; }
.nav-row[aria-current=page] { background:var(--shell-button); color:var(--ink); font-weight:600; }
.nav-row[aria-current=page]>.icon { color:var(--accent); }
.nav-row:has(.offline-badge):has(.chevron) { grid-template-columns:30px minmax(0,1fr) auto 14px; }
.nav-row>.chevron { transition:transform var(--vf-dur-Spring, 340ms) var(--vf-ease-spring); }
.nav-row[aria-expanded=true]>.chevron { transform:rotate(180deg); }
.label { min-width:0; overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }
.children[hidden] { display:none; }
.branch-row { display:flex; align-items:center; min-width:0; }
.branch-row>.nav-row { flex:1; min-width:0; }
/* Same right inset as .nav-row (12px) so this chevron lines up with the others. */
.branch-toggle { flex:0 0 38px; height:40px; padding:0 12px 0 0; border-radius:6px; color:var(--muted); display:grid; place-items:center end; }
.branch-toggle>.chevron { transition:transform var(--vf-dur-Spring, 340ms) var(--vf-ease-spring); }
.branch-toggle[aria-expanded=true]>.chevron { transform:rotate(180deg); }
.branch-toggle:hover { background:var(--shell-button); color:var(--ink); }
.branch-toggle:focus-visible { outline:2px solid var(--accent); outline-offset:1px; }
.children { padding:2px 0 7px; }
.children .nav-row { display:flex; justify-content:space-between; min-height:35px; padding:6px 12px 6px 42px; font-size:13px; position:relative; }
.children .nav-row[aria-current=page]:before { content:""; position:absolute; width:4px; height:4px; left:24px; border-radius:50%; background:var(--accent); }
.count,.offline-badge { color:var(--muted); font-size:11px; font-weight:400; font-variant-numeric:tabular-nums; font-feature-settings:"tnum" 1, "zero" 1; }
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
 .branch-toggle { flex-basis:20px; }
 .branch-row>.nav-row { padding:10px 0; }
 .host-sidebar { padding:22px 7px 12px; }
 .brand { display:flex; padding:0; justify-content:center; margin-bottom:28px; }
 .brand img { width:28px; height:34px; }
 .brand-copy,.nav-row .label,.count,.nav-row .chevron,.utility-label,.status-text,.utility>.chevron,.status-dot,.system-hint,.offline-badge { display:none; }
 .host-sidebar .nav-row,.utility { display:flex; justify-content:center; padding:10px 8px; }
 .children .nav-row { padding:8px 3px; min-height:34px; font-size:10px; line-height:15px; text-align:center; }
 .children .nav-row .label { display:block; white-space:normal; }
 .children .nav-row[aria-current=page]:before { display:none; }
 .side-spacer { min-height:28px; }
 .nav-state { font-size:10px; padding:10px 2px; overflow-wrap:anywhere; }
 .nav-state .small-button { padding:3px; }
}
@media(prefers-reduced-motion:reduce) { * { transition:none!important; } }
</style>
