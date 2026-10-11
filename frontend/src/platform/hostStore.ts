import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { request } from './client'

export interface HostIdentity {
  user_id?: string
  username?: string
  organization_id?: string
  organization_name?: string
  station_id?: string
  roles?: string[]
  scopes?: string[]
}

export const useHostStore = defineStore('host', () => {
  const connected = ref(false)
  const identity = ref<HostIdentity>({})
  const clusterName = ref('Station')
  const clusterHost = computed(() => identity.value.station_id || '未连接')
  const coreStatus = ref<string | null>(null)
  const loginOpen = ref(false)
  let identityEpoch = 0

  const displayName = computed(() => {
    return identity.value.username || identity.value.user_id || '等待连接'
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
    coreStatus.value = null
  }

  async function fetchSession() {
    const epoch = identityEpoch
    // A transient gateway failure must not permanently strand direct project
    // links without the identity needed for user-scoped local Tips.
    for (let attempt = 0; attempt < 3; attempt++) {
      try {
        const res = await request({ path: 'me', method: 'GET' }, { realOnly: true })
        if (epoch !== identityEpoch) return null
        if (res.status === 200) {
          const data = await res.json()
          if (epoch !== identityEpoch) return null
          setIdentity(data)
          return data
        }
        if (res.status < 500) break
      } catch {
        if (epoch !== identityEpoch) return null
        // Retry only this idempotent session read; never replay a mutation.
      }
    }
    if (epoch === identityEpoch) setIdentity({})
    return null
  }

  async function fetchCluster() {
    const epoch = identityEpoch, stationId = identity.value.station_id
    if (!connected.value || !stationId) return
    try {
      const res = await request({ path: 'health', method: 'GET' }, { realOnly: true })
      const data = res.status === 200 ? await res.json() : null
      if (epoch !== identityEpoch) return
      coreStatus.value = data?.station_id === stationId && data?.components?.core === 'Ready' ? '就绪'
        : data?.station_id === stationId && data?.components?.core === 'NotReady' ? '未就绪' : null
    } catch { if (epoch === identityEpoch) coreStatus.value = null }
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
    loginOpen,
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
