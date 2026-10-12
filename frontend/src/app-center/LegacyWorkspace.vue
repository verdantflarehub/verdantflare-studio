<script setup lang="ts">
import { computed, onBeforeUnmount, ref, shallowRef, watch } from 'vue'
import { useRouter } from 'vue-router'
import { useHostStore } from '../platform/hostStore'
import { useAppCenterStore } from './store'
import { createLegacyClient, legacyLocation, legacyTarget, selectLegacyCard } from './legacy'
import type { LegacyOpened, LegacyTarget } from './legacy'
import type { AppCenterCard } from './schema.generated'
import { reasonLabel } from './labels'
import LegacyMediaHost from './LegacyMediaHost.vue'
import LegacyBlenderHost from './LegacyBlenderHost.vue'

const props = defineProps<{ target: LegacyTarget | null; routeError: string }>()
const router = useRouter(), host = useHostStore(), store = useAppCenterStore(), client = createLegacyClient()
const selection = shallowRef<{ card: AppCenterCard; opened: LegacyOpened; route: string; scope: string; identity: string } | null>(null)
const error = ref(''), loading = ref(false), retry = ref(0)
let generation = 0, pending: AbortController | undefined
const identity = computed(() => JSON.stringify([host.connected, host.identity.user_id, host.identity.organization_id, host.identity.station_id,
  host.identity.session_id, host.identity.policy_version, host.identity.revocation_version, host.identity.roles, host.identity.scopes]))
const active = computed(() => !!props.target || !!props.routeError)
function clearSelection() { if (selection.value) void client.close(selection.value.opened); selection.value = null }
function fail(errorValue: unknown) { error.value = errorValue instanceof Error ? errorValue.message : 'APPCENTER_OPEN_UNAVAILABLE'; clearSelection() }
async function authorize() {
  const selected = selection.value
  if (!selected) throw new Error('APPCENTER_OPEN_UNAVAILABLE')
  if (store.state.stale || !store.state.value || Date.parse(store.state.value.validUntil) <= store.currentTime()) await store.refresh()
  const current = store.state.value
  if (selection.value !== selected || selected.identity !== identity.value || current && (JSON.stringify(current.scope) !== selected.scope
    || current.items.find(card => card.cardKey === selected.card.cardKey)?.bindingRevision !== selected.card.bindingRevision)) {
    const changed = new Error('APPCENTER_BINDING_CHANGED')
    if (selection.value === selected) fail(changed)
    throw changed
  }
  // A hidden page or failed read must block forwarding without erasing the
  // still-bound workspace or claiming that its identity changed.
  if (!current || store.state.stale || Date.parse(current.validUntil) <= store.currentTime()) throw new Error('APPCENTER_DEPENDENCY_UNAVAILABLE')
  if (!store.canExecute(selected.card.cardKey, 'open', selected.card.bindingRevision)) {
    const denied = new Error(current.items.find(card => card.cardKey === selected.card.cardKey)?.actions.find(action => action.kind === 'open')?.reasonCode ?? 'APPCENTER_OPEN_UNAVAILABLE')
    fail(denied); throw denied
  }
}
async function navigate(route: string) {
  if (route === '/market') { await router.push(route); return }
  const selected = selection.value
  if (!selected) return
  try {
    const next = legacyTarget(route)
    if (!next || next.adapterId !== selected.opened.adapterId) throw new Error('INVALID_LEGACY_ROUTE')
    await authorize()
    store.pinOpen(selected.card.cardKey, selected.card.bindingRevision)
    const failed = await router.push(legacyLocation({ ...selected.opened, route: next.route }, selected.card.cardKey))
    if (failed) store.takeOpenBinding(selected.card.cardKey)
  } catch (e) { fail(e) }
}
watch(() => [props.target?.route, props.target?.cardKey, props.routeError, identity.value, retry.value], async () => {
  const epoch = ++generation, target = props.target
  pending?.abort(); pending = new AbortController()
  const previous = selection.value
  if (previous && (previous.identity !== identity.value || target?.adapterId !== previous.opened.adapterId
    || target.cardKey && target.cardKey !== previous.card.cardKey || props.routeError)) clearSelection()
  const controller = pending
  if (!active.value) { clearSelection(); error.value = ''; loading.value = false; return }
  error.value = ''; loading.value = true
  let opened: LegacyOpened | undefined, adopted = false
  try {
    if (props.routeError) throw new Error(props.routeError)
    if (!target || !host.connected) throw new Error('UNAUTHENTICATED')
    await store.refresh()
    if (epoch !== generation || controller.signal.aborted) return
    const view = store.state.value
    if (!view) throw new Error('APPCENTER_OPEN_UNAVAILABLE')
    const card = selectLegacyCard(view, target), expected = store.takeOpenBinding(card.cardKey)
    if (expected && expected !== card.bindingRevision) throw new Error('APPCENTER_BINDING_CHANGED')
    if (!store.canExecute(card.cardKey, 'open', card.bindingRevision)) throw new Error(card.actions.find(action => action.kind === 'open')?.reasonCode ?? 'APPCENTER_OPEN_UNAVAILABLE')
    opened = await client.open(view, card, AbortSignal.any([controller.signal, AbortSignal.timeout(30000)]))
    if (epoch !== generation || controller.signal.aborted) return
    const current = store.state.value
    if (!current || JSON.stringify(current.scope) !== JSON.stringify(view.scope) || !store.canExecute(card.cardKey, 'open', card.bindingRevision)) throw new Error('APPCENTER_BINDING_CHANGED')
    clearSelection()
    selection.value = { card, opened, route: target.route, scope: JSON.stringify(view.scope), identity: identity.value }; adopted = true
  } catch (e) { if (epoch === generation) fail(e) }
  finally { if (opened && !adopted) void client.close(opened); if (epoch === generation) loading.value = false }
}, { immediate: true, flush: 'sync' })
watch(() => store.state.value, view => {
  const selected = selection.value
  if (selected && (!view || JSON.stringify(view.scope) !== selected.scope || view.items.find(card => card.cardKey === selected.card.cardKey)?.bindingRevision !== selected.card.bindingRevision)) {
    pending?.abort(); generation++; loading.value = false; fail(new Error('APPCENTER_BINDING_CHANGED'))
  }
})
onBeforeUnmount(() => { generation++; pending?.abort(); clearSelection() })
</script>

<template>
  <main v-if="active" class="main embed-active legacy-workspace"><div class="workspace">
    <header class="heading"><div class="heading-left"><h1>{{ selection?.card.display.name || '应用入口' }}</h1><p>{{ selection?.card.display.description }}</p></div><button class="btn" @click="router.push('/market')">返回应用中心</button></header>
    <nav aria-label="面包屑"><a href="#/market" @click.prevent="router.push('/market')">应用中心</a><span v-if="selection"> / {{ selection.card.display.name }}</span></nav>
    <p v-if="loading" role="status">正在确认应用入口…</p>
    <div v-if="error" class="panel legacy-error" role="alert"><p>{{ reasonLabel(error) }}</p><button class="btn" @click="retry++">重试</button></div>
    <template v-if="selection">
      <LegacyBlenderHost v-if="selection.opened.adapterId === 'blender-host-v1'" :key="selection.card.bindingRevision + (selection.opened.resourceBase ?? '')" :route="selection.route" :resource-base="selection.opened.resourceBase" :mcp-origin="selection.opened.mcpOrigin" :authorize="authorize" @navigate="navigate" />
      <LegacyMediaHost v-else :key="selection.card.bindingRevision + (selection.opened.resourceBase ?? '')" :adapter-id="selection.opened.adapterId" :resource-base="selection.opened.resourceBase" :route="selection.route" :title="selection.card.display.name" :authorize="authorize" @navigate="navigate" @error="fail" @reconnect="retry++" />
    </template>
  </div></main>
</template>

<style scoped>
.legacy-error { margin: 20px 0; padding: 20px; color: var(--muted); }
nav { margin-bottom: 20px; color: var(--muted); }
</style>
