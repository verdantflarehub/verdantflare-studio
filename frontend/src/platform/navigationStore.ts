import { defineStore } from 'pinia'
import { computed, onScopeDispose, shallowRef, watch } from 'vue'
import { createNavigationController } from '../navigation/controller'
import { createNavigationTransport } from '../navigation/transport'
import { useAppCenterStore } from '../app-center/store'
import { appCenterNavigationEntries } from '../app-center/navigation'

export const useNavigationStore = defineStore('navigation', () => {
  const controller = createNavigationController(createNavigationTransport())
  const state = shallowRef(controller.state)
  const appcenter = import.meta.env.VITE_APPCENTER_MODE === 'unified' ? useAppCenterStore() : null
  controller.subscribe(next => { state.value = next })
  watch(() => state.value.value?.mode, mode => appcenter?.setVisible('navigation', mode === 'preinstalled'))
  onScopeDispose(() => appcenter?.setVisible('navigation', false))
  const entries = computed(() => {
    const value = state.value.status === 'ready' ? state.value.value : null
    if (!value) return []
    const projected = appcenter && value.mode === 'preinstalled' ? appCenterNavigationEntries(value, appcenter.state, appcenter.currentTime()) : value.projection.entries
    // Keep older Project declarations consistent with the current module name.
    return projected.map(entry => entry.appId === 'project' ? { ...entry, title: '项目管理' } : entry)
  })
  const pinned = computed(() => entries.value.filter(entry => entry.pinned))
  const legacy = computed(() => state.value.status === 'ready' && state.value.value?.mode === 'legacy')
  return { state, entries, pinned, legacy, clear: controller.clear, refresh: controller.refresh, setPreference: controller.setPreference }
})
