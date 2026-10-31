import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { request } from './client'
import { readHostSession } from './session'
import { stationHealth } from './stationHealth'
import type { HostIdentity } from './session'
export type { HostIdentity } from './session'

export const useHostStore = defineStore('host', () => {
  const connected = ref(false)
  const identity = ref<HostIdentity>({})
  const clusterName = ref('名称待确认')
  const healthLoading = ref(false)
  let healthRequest = 0
  const clusterHost = computed(() => identity.value.station_id || '未连接')
  const coreStatus = ref<string | null>(null)
  const loginOpen = ref(false)
  const sessionStatus = ref<'loading' | 'authenticated' | 'anonymous' | 'error'>('loading')
  let identityEpoch = 0

  const displayName = computed(() => {
    return identity.value.username || (connected.value ? '当前用户' : sessionStatus.value === 'loading' ? '读取中' : sessionStatus.value === 'error' ? '身份读取失败' : '未登录')
  })

  const teamName = computed(() => {
    return identity.value.organization_name || (connected.value ? '未分配组织' : '等待连接 Station')
  })

  const avatar = computed(() => {
    const name = displayName.value
    if (!name || name === '等待连接') return 'VF'
    return name.slice(0, 2).toUpperCase()
  })

  const clusterMeta = computed(() => {
    if (!connected.value) return '等待连接 Station'
    return `Core ${coreStatus.value ?? '状态未知'}`
  })

  function setIdentity(nextIdentity: HostIdentity) {
    identityEpoch++
    identity.value = nextIdentity || {}
    connected.value = Boolean(nextIdentity && (nextIdentity.username || nextIdentity.user_id))
    sessionStatus.value = connected.value ? 'authenticated' : 'anonymous'
    if (connected.value) loginOpen.value = false
    coreStatus.value = null
    clusterName.value = nextIdentity.station_name?.trim() || '名称待确认'
    healthRequest++; healthLoading.value = false
  }

  async function fetchSession() {
    const epoch = identityEpoch
    sessionStatus.value = 'loading'
    const result = await readHostSession(() => request({ path: 'me', method: 'GET' }, { realOnly: true, signal: AbortSignal.timeout(4000) }), () => epoch === identityEpoch)
    if (epoch !== identityEpoch || result.status === 'superseded') return null
    if (result.status === 'authenticated') { setIdentity(result.identity); return result.identity }
    setIdentity({}); sessionStatus.value = result.status
    if (result.status === 'anonymous') loginOpen.value = true
    return null
  }

  async function fetchCluster() {
    const epoch = identityEpoch, stationId = identity.value.station_id
    if (!connected.value || !stationId || healthLoading.value) return
    const current = ++healthRequest
    healthLoading.value = true
    const abort = new AbortController(), timeout = setTimeout(() => abort.abort(), 10000)
    try {
      const res = await request({ path: 'health', method: 'GET' }, { realOnly: true, signal: abort.signal })
      const data = res.status === 200 ? await res.json() : null
      if (epoch !== identityEpoch || current !== healthRequest) return
      const health = stationHealth(data, stationId)
      if (health.name) clusterName.value = health.name
      coreStatus.value = health.status
    } catch { if (epoch === identityEpoch && current === healthRequest) coreStatus.value = null }
    finally { clearTimeout(timeout); if (current === healthRequest) healthLoading.value = false }
  }

  async function init() {
    const s = await fetchSession()
    if (s) {
      await fetchCluster()
    }
  }

  return {
    connected,
    identity,
    clusterName,
    clusterHost,
    coreStatus,
    healthLoading,
    loginOpen,
    sessionStatus,
    displayName,
    teamName,
    avatar,
    clusterMeta,
    setIdentity,
    fetchSession,
    fetchCluster,
    init
  }
})
