import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createRouter, createWebHashHistory } from 'vue-router'
import App from './App.vue'
import Workbench from './pages/Workbench.vue'
import Market from './pages/Market.vue'
import Models from './pages/Models.vue'
import Resources from './pages/Resources.vue'
import StationProxy from './pages/StationProxy.vue'
import './theme/market.css'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/market' },
    { path: '/workbench', component: Workbench },
    { path: '/market', component: Market },
    { path: '/models', component: Models },
    { path: '/resources', component: Resources },
    { path: '/station/proxies', component: StationProxy },
    { path: '/:pathMatch(.*)*', redirect: '/market' }
  ]
})

createApp(App).use(createPinia()).use(router).mount('#app')

