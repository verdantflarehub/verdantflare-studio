<script setup lang="ts">
import { computed, defineAsyncComponent, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useNavigationStore } from '../platform/navigationStore'
import { useHostStore } from '../platform/hostStore'
import { appTargetLocation, legacyAppTarget } from '../navigation/app-target'

const props = defineProps<{ kind: 'project' | 'network' | 'resources' }>()
const navigation = useNavigationStore(), identity = useHostStore(), route = useRoute(), router = useRouter()
const legacyViews = {
  project: defineAsyncComponent(() => import('./ProjectWorld.vue')),
  network: defineAsyncComponent(() => import('./StationProxy.vue')),
  resources: defineAsyncComponent(() => import('./Resources.vue')),
}
const error = ref('')
const legacy = computed(() => navigation.legacy)
watch([() => navigation.state, () => route.fullPath], () => {
  error.value = ''
  if (navigation.state.status !== 'ready' || navigation.state.value?.mode !== 'preinstalled') return
  try {
    const target = legacyAppTarget(route.fullPath)
    if (!target) throw new Error('INVALID_NAVIGATION')
    void router.replace(appTargetLocation(target, navigation.state.value))
  } catch { error.value = '此应用尚未安装或当前账号无权访问，请到应用中心查看。' }
}, { immediate: true })
</script>

<template>
  <component v-if="legacy" :is="legacyViews[kind]" />
  <main v-else class="main"><div class="workspace">
    <header class="heading"><div class="heading-left"><h1>打开应用</h1></div></header>
    <p v-if="!identity.connected" role="status">请先登录，再打开此应用。</p>
    <p v-else-if="error || navigation.state.error" role="alert">{{ error || navigation.state.error }}</p>
    <p v-else role="status">正在确认应用安装与访问权限…</p>
    <div class="heading-actions"><button class="btn" @click="navigation.refresh()">重试</button><button class="btn ghost" @click="router.push('/market')">前往应用中心</button></div>
  </div></main>
</template>
