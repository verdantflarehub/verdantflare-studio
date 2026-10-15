import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createRouter, createWebHashHistory } from 'vue-router'
import App from './App.vue'
import LegacyMarket from './pages/Market.vue'
import AppCenterPage from './app-center/AppCenterPage.vue'
import LegacyAppEntry from './pages/LegacyAppEntry.vue'
import './theme/market.css'

// One explicit release mode selects the whole AppCenter. A request failure
// must never silently select the legacy catalog or application-name branches.
const appCenterMode = import.meta.env.VITE_APPCENTER_MODE ?? 'legacy'
if (!['legacy', 'unified'].includes(appCenterMode)) throw new Error('INVALID_APPCENTER_MODE')
const Market = appCenterMode === 'unified' ? AppCenterPage : LegacyMarket

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/market' },
    { path: '/workbench', redirect: '/market' },
    { path: '/projects', component: LegacyAppEntry, props: { kind: 'project' } },
    { path: '/market', component: Market },
    { path: '/apps/:appId/', name: 'app-dashboard', component: Market, strict: true },
    { path: '/apps/:appId', strict: true, redirect: to => ({ path: `/apps/${String(to.params.appId)}/`, query: to.query }) },
    { path: '/apps/:installationId/dashboard', name: 'app-dashboard-legacy', component: Market },
    { path: '/apps/:installationId/instances/:workspaceSessionId', name: 'app-instance', component: Market },
    { path: '/models', redirect: '/market' },
    { path: '/resources', component: LegacyAppEntry, props: { kind: 'resources' } },
    { path: '/station/proxies', component: LegacyAppEntry, props: { kind: 'network' } },
    { path: '/:pathMatch(.*)*', redirect: '/market' }
  ]
})

createApp(App).use(createPinia()).use(router).mount('#app')
