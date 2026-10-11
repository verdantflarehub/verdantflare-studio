import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createRouter, createWebHashHistory } from 'vue-router'
import App from './App.vue'
import Workbench from './pages/Workbench.vue'
import Market from './pages/Market.vue'
import Models from './pages/Models.vue'
import LegacyAppEntry from './pages/LegacyAppEntry.vue'
import './theme/market.css'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/market' },
    { path: '/workbench', component: Workbench },
    { path: '/projects', component: LegacyAppEntry, props: { kind: 'project' } },
    { path: '/market', component: Market },
    { path: '/apps/:installationId/dashboard', name: 'app-dashboard', component: Market },
    { path: '/apps/:installationId/instances/:workspaceSessionId', name: 'app-instance', component: Market },
    { path: '/models', component: Models },
    { path: '/resources', component: LegacyAppEntry, props: { kind: 'resources' } },
    { path: '/station/proxies', component: LegacyAppEntry, props: { kind: 'network' } },
    { path: '/:pathMatch(.*)*', redirect: '/market' }
  ]
})

createApp(App).use(createPinia()).use(router).mount('#app')
