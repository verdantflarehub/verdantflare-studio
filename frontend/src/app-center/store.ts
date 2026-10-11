import { defineStore } from 'pinia'
import { onScopeDispose, shallowRef, watch } from 'vue'
import { useHostStore } from '../platform/hostStore'
import { createAppCenterController } from './controller'
import { createAppCenterTransport } from './transport'
import { createSharedRefresh } from './refresh'
import { createCommands } from './commands'
import { commandScope, createCommandStorage } from './command-storage'
import { createCommandTransport } from './command-transport'
import { request } from '../platform/client'
import type { AppCenterCard } from './schema.generated'

/** One snapshot and polling owner shared by directory, details and launchers. */
export const useAppCenterStore = defineStore('appcenter', () => {
  const host = useHostStore(), controller = createAppCenterController(createAppCenterTransport())
  const state = shallowRef(controller.state), consumers = new Set<string>()
  // IndexedDB is optional at runtime, but persistence is mandatory for sending.
  const commands = createCommands(createCommandStorage(globalThis.indexedDB), createCommandTransport(fetch,
    (id, signal) => { if (signal.aborted) return Promise.reject(new Error('REQUEST_CANCELLED')); return request({ path: `operations/${id}`, method: 'GET' }, { realOnly: true }) }))
  const commandState = shallowRef(commands.state)
  commands.subscribe(next => { commandState.value = next })
  let lastScope = ''
  const channel = typeof BroadcastChannel === 'undefined' ? null : new BroadcastChannel('studio.appcenter.commands.v1')
  async function logoutCommands() {
    const target = lastScope
    if (!target) return
    channel?.postMessage({ kind: 'logout', scope: target })
    await commands.clearScope(target).catch(() => undefined)
  }
  if (channel) channel.onmessage = event => {
    if (event.data?.kind === 'logout' && event.data.scope === lastScope) {
      void commands.setScope(null)
      clear()
    }
  }
  async function execute(card: AppCenterCard, action: string) {
    await commands.execute(card, action, () => controller.canExecute(card.cardKey, action, card.bindingRevision))
    await refresh()
  }
  async function resumeCommand(key: string) { await commands.resume(key); await refresh() }
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
    const identity = who.user_id && who.organization_id && who.station_id ? { userId: who.user_id, organizationId: who.organization_id, stationId: who.station_id } : null
    if (identity) lastScope = commandScope(identity)
    void commands.setScope(identity)
    if (visible()) void refresh()
  }, { immediate: true, flush: 'sync' })
  document.addEventListener('visibilitychange', changedVisibility)
  window.addEventListener('studio:logout', clear)
  window.addEventListener('studio:logout', logoutCommands)
  onScopeDispose(() => { clearTimeout(timer); sharedRefresh.invalidate(); controller.dispose(); commands.dispose(); channel?.close(); document.removeEventListener('visibilitychange', changedVisibility); window.removeEventListener('studio:logout', clear); window.removeEventListener('studio:logout', logoutCommands) })
  return { state, commandState, execute, resumeCommand, refresh, clear, setVisible, pinOpen, takeOpenBinding, canExecute: controller.canExecute }
})
