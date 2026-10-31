import { defineStore } from 'pinia'
import { computed, onScopeDispose, shallowRef, watch } from 'vue'
import { createNavigationController } from '../navigation/controller'
import { createNavigationTransport, prefetchNavigation } from '../navigation/transport'
import { useAppCenterStore } from '../app-center/store'
import { appCenterNavigationEntries } from '../app-center/navigation'

export const useNavigationStore = defineStore('navigation', () => {
  const controller = createNavigationController(prefetchNavigation(createNavigationTransport()))
  const state = shallowRef(controller.state)
  // Menu structure only (no App state) is kept per tab under the full session
  // and authorization key, so a refresh can draw the last menu immediately.
  const cacheName = 'vf.navigation.menu.v1'
  let cacheKey = ''
  controller.subscribe(next => {
    if (!cacheKey || next.status !== 'ready' || !next.value) return
    try { sessionStorage.setItem(cacheName, JSON.stringify({ key: cacheKey, value: next.value })) } catch { /* storage is optional */ }
  })
  function clear(identity: Parameters<typeof controller.clear>[0] = null, key = '') {
    cacheKey = identity ? key : ''
    controller.clear(identity)
    if (!identity) { try { sessionStorage.removeItem(cacheName) } catch { /* storage is optional */ } return }
    if (!key) return
    try {
      const saved = JSON.parse(sessionStorage.getItem(cacheName) ?? 'null')
      if (saved?.key === key) controller.seed(saved.value)
    } catch { /* malformed or unavailable storage falls back to the network */ }
  }
  const appcenter = import.meta.env.VITE_APPCENTER_MODE === 'unified' ? useAppCenterStore() : null
  controller.subscribe(next => { state.value = next })
  watch(() => state.value.value?.mode, mode => appcenter?.setVisible('navigation', mode === 'preinstalled'))
  const stopNavHint = appcenter?.onNavigationHint(() => {
    if (state.value.value?.mode === 'preinstalled') void controller.refresh()
  })
  let lastInstallSignature = ''
  watch(() => {
    const items = appcenter?.state.value?.items
    if (!items) return ''
    return items
      .filter(card => card.installation?.state === 'installed')
      .map(card => `${card.appId}@${card.installedVersion}:${card.installation?.installationId}`)
      .sort()
      .join('|')
  }, sig => {
    if (lastInstallSignature && sig && sig !== lastInstallSignature && state.value.value?.mode === 'preinstalled') {
      void controller.refresh()
    }
    if (sig) lastInstallSignature = sig
  })
  onScopeDispose(() => {
    stopNavHint?.()
    appcenter?.setVisible('navigation', false)
  })
  const entries = computed(() => {
    const value = state.value.value
    if (!value) return []
    const projected = appcenter && value.mode === 'preinstalled' ? appCenterNavigationEntries(value, appcenter.state, appcenter.currentTime()) : value.projection.entries
    // Keep older Project declarations consistent with the current module name.
    return projected.map(entry => entry.appId === 'project' ? { ...entry, title: '项目管理' } : entry)
  })
  const pinned = computed(() => entries.value.filter(entry => entry.pinned))
  const legacy = computed(() => state.value.status === 'ready' && state.value.value?.mode === 'legacy')
  return { state, entries, pinned, legacy, clear, refresh: controller.refresh, setPreference: controller.setPreference }
})
