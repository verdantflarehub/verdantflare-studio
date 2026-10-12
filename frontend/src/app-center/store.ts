import { defineStore } from 'pinia'
import { onScopeDispose, shallowRef, watch } from 'vue'
import { useHostStore } from '../platform/hostStore'
import { createAppCenterController } from './controller'
import { createAppCenterTransport } from './transport'
import { createSharedRefresh } from './refresh'
import { createCommands } from './commands'
import { commandScope, createCommandStorage } from './command-storage'
import { createCommandTransport } from './command-transport'
import { createLegacyRecoverySource } from './legacy-commands'
import { createOperationProgress, createOperationReads } from './operation-progress'
import { request } from '../platform/client'
import type { AppCenterCard } from './schema.generated'

/** One snapshot and polling owner shared by directory, details and launchers. */
export const useAppCenterStore = defineStore('appcenter', () => {
  const host = useHostStore(), controller = createAppCenterController(createAppCenterTransport())
  const state = shallowRef(controller.state), consumers = new Set<string>()
  // IndexedDB is optional at runtime, but persistence is mandatory for sending.
  const transport = createCommandTransport(fetch,
    (id, signal) => request({ path: `operations/${id}`, method: 'GET' }, { realOnly: true, signal }),
    (body, signal) => request({ path: 'commands', method: 'POST', body: JSON.parse(body) }, { realOnly: true, signal }))
  const reads = createOperationReads(transport.read), progress = createOperationProgress(reads.read)
  const commands = createCommands(createCommandStorage(globalThis.indexedDB), { ...transport, read: reads.read }, undefined, createLegacyRecoverySource())
  const commandState = shallowRef(commands.state)
  const operationState = shallowRef(progress.state)
  commands.subscribe(next => { commandState.value = next })
  progress.subscribe(next => { operationState.value = next })
  let detailKey = '', progressVisible = true, readCycle: AbortController | null = null, forceOperations = false
  function stopReads() { readCycle?.abort(); readCycle = null; reads.invalidate(); progress.suspend() }
  function setDetail(key: string) { detailKey = key; progress.select(state.value.value?.items.find(card => card.cardKey === key)); if (key && progressVisible) void refresh() }
  function setProgressVisible(active: boolean) { progressVisible = active; if (!active) { stopReads(); sharedRefresh.invalidate() } else if (visible()) void refresh() }
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
  controller.subscribe(next => { state.value = next; progress.select(next.value?.items.find(card => card.cardKey === detailKey)) })
  const visible = () => consumers.size > 0 && !document.hidden
  const sharedRefresh = createSharedRefresh(async () => {
    const current = new AbortController(); readCycle = current
    try {
      await controller.refresh()
      if (current.signal.aborted || !visible() || !progressVisible) return
      const force = forceOperations; forceOperations = false
      await reads.cycle(async () => {
        await Promise.all([commands.refreshKnown(current.signal), progress.refresh(force, current.signal)])
      })
    } finally { if (readCycle === current) readCycle = null }
  }, schedule)
  function schedule() {
    clearTimeout(timer)
    if (!visible() || !host.connected) return
    const until = state.value.value ? Date.parse(state.value.value.validUntil) - controller.currentTime() : 15000
    const delay = state.value.status === 'ready' && !state.value.stale ? Math.max(100, Math.min(15000, until)) : 15000
    timer = setTimeout(() => { controller.expire(); void refresh() }, delay)
  }
  function refresh(force = false): Promise<void> {
    forceOperations ||= force
    return visible() ? sharedRefresh.refresh() : Promise.resolve()
  }
  function changedVisibility() {
    clearTimeout(timer)
    if (visible()) void refresh()
    else { sharedRefresh.invalidate(); controller.suspend(); stopReads() }
  }
  function setVisible(owner: string, active: boolean) {
    const wasVisible = visible()
    if (active) consumers.add(owner); else consumers.delete(owner)
    if (wasVisible !== visible()) changedVisibility()
  }
  function clear() { clearTimeout(timer); sharedRefresh.invalidate(); stopReads(); progress.clear(null); openBindings.clear(); controller.clear() }
  watch(() => JSON.stringify([host.identity.user_id, host.identity.organization_id, host.identity.station_id, host.identity.session_id, host.identity.policy_version, host.identity.revocation_version, host.identity.roles, host.identity.scopes]), () => {
    const who = host.identity
    sharedRefresh.invalidate()
    stopReads()
    openBindings.clear()
    controller.clear(who.user_id && who.organization_id && who.station_id ? { userId: who.user_id, organizationId: who.organization_id, stationId: who.station_id } : null)
    const identity = who.user_id && who.organization_id && who.station_id ? { userId: who.user_id, organizationId: who.organization_id, stationId: who.station_id } : null
    progress.clear(identity)
    progress.select(state.value.value?.items.find(card => card.cardKey === detailKey))
    if (identity) lastScope = commandScope(identity)
    void commands.setScope(identity).then(() => { if (visible()) void refresh() })
    if (visible()) void refresh()
  }, { immediate: true, flush: 'sync' })
  document.addEventListener('visibilitychange', changedVisibility)
  window.addEventListener('studio:logout', clear)
  window.addEventListener('studio:logout', logoutCommands)
  onScopeDispose(() => { clearTimeout(timer); sharedRefresh.invalidate(); stopReads(); controller.dispose(); commands.dispose(); progress.dispose(); channel?.close(); document.removeEventListener('visibilitychange', changedVisibility); window.removeEventListener('studio:logout', clear); window.removeEventListener('studio:logout', logoutCommands) })
  return { state, commandState, operationState, setDetail, setProgressVisible, execute, resumeCommand, refresh, clear, setVisible, pinOpen, takeOpenBinding, canExecute: controller.canExecute, currentTime: controller.currentTime }
})
