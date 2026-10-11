import { defineStore } from 'pinia'
import { onScopeDispose, shallowRef, watch } from 'vue'
import { useHostStore } from '../platform/hostStore'
import { createAppCenterController } from './controller'
import { createAppCenterTransport } from './transport'
import { createSharedRefresh } from './refresh'

/** One snapshot and polling owner shared by directory, details and launchers. */
export const useAppCenterStore = defineStore('appcenter', () => {
  const host = useHostStore(), controller = createAppCenterController(createAppCenterTransport())
  const state = shallowRef(controller.state), consumers = new Set<string>()
  const openBindings = new Map<string, string>()
  function pinOpen(id: string, binding: string) { openBindings.set(id, binding) }
  function takeOpenBinding(id: string) { const binding = openBindings.get(id); openBindings.delete(id); return binding }
  let timer: ReturnType<typeof setTimeout> | undefined
  controller.subscribe(next => { state.value = next })
  const visible = () => consumers.size > 0 && !document.hidden
  const sharedRefresh = createSharedRefresh(() => controller.refresh(), schedule)
  function schedule() {
    clearTimeout(timer)
    if (!visible() || !host.connected) return
    const until = state.value.value ? Date.parse(state.value.value.validUntil) - Date.now() : 15000
    const delay = state.value.status === 'ready' && !state.value.stale ? Math.max(100, Math.min(15000, until)) : 15000
    timer = setTimeout(() => { controller.expire(); void refresh() }, delay)
  }
  function refresh(): Promise<void> {
    return visible() ? sharedRefresh.refresh() : Promise.resolve()
  }
  function changedVisibility() {
    clearTimeout(timer)
    if (visible()) void refresh()
    else { sharedRefresh.invalidate(); controller.suspend() }
  }
  function setVisible(owner: string, active: boolean) {
    const wasVisible = visible()
    if (active) consumers.add(owner); else consumers.delete(owner)
    if (wasVisible !== visible()) changedVisibility()
  }
  function clear() { clearTimeout(timer); sharedRefresh.invalidate(); openBindings.clear(); controller.clear() }
  watch(() => JSON.stringify([host.identity.user_id, host.identity.organization_id, host.identity.station_id, host.identity.session_id, host.identity.policy_version, host.identity.revocation_version, host.identity.roles, host.identity.scopes]), () => {
    const who = host.identity
    sharedRefresh.invalidate()
    openBindings.clear()
    controller.clear(who.user_id && who.organization_id && who.station_id ? { userId: who.user_id, organizationId: who.organization_id, stationId: who.station_id } : null)
    if (visible()) void refresh()
  }, { immediate: true })
  document.addEventListener('visibilitychange', changedVisibility)
  window.addEventListener('studio:logout', clear)
  onScopeDispose(() => { clearTimeout(timer); sharedRefresh.invalidate(); controller.dispose(); document.removeEventListener('visibilitychange', changedVisibility); window.removeEventListener('studio:logout', clear) })
  return { state, refresh, clear, setVisible, pinOpen, takeOpenBinding, canExecute: controller.canExecute }
})
