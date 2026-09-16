import { createApp } from 'vue'
import { createPinia } from 'pinia'
import { createRouter, createWebHashHistory } from 'vue-router'
import App from './App.vue'
import Market from './pages/Market.vue'
import './theme/market.css'
const router = createRouter({history:createWebHashHistory(),routes:[{path:'/',redirect:'/market'},{path:'/market',component:Market},{path:'/:pathMatch(.*)*',redirect:'/market'}]})
createApp(App).use(createPinia()).use(router).mount('#app')
