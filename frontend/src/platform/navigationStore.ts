import { defineStore } from 'pinia'
import { computed, shallowRef } from 'vue'
import { createNavigationController } from '../navigation/controller'
import { createNavigationTransport } from '../navigation/transport'

export const useNavigationStore = defineStore('navigation', () => {
  const controller = createNavigationController(createNavigationTransport())
  const state = shallowRef(controller.state)
  controller.subscribe(next => { state.value = next })
  const entries = computed(() => state.value.status === 'ready' ? state.value.value?.projection.entries ?? [] : [])
  const pinned = computed(() => entries.value.filter(entry => entry.pinned))
  const legacy = computed(() => state.value.status === 'ready' && state.value.value?.mode === 'legacy')
  return { state, entries, pinned, legacy, clear: controller.clear, refresh: controller.refresh, setPreference: controller.setPreference }
})
