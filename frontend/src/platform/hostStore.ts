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
  const clusterName = ref('5090集群')
  const clusterHost = ref('k8s.dev.verdantflarehub.com')
  const gpuCount = ref(2)
  const gpuModel = ref('RTX 5090')
  const coreStatus = ref('就绪')
  const loginOpen = ref(false)

  const displayName = computed(() => {
    return identity.value.username || identity.value.user_id || (connected.value ? 'admin' : '等待连接')
  })

  const teamName = computed(() => {
    return identity.value.organization_name || (connected.value ? '未分配组织' : '等待连接 Station')
  })

  const avatar = computed(() => {
    const name = displayName.value
    if (!name || name === '等待连接' || name === 'admin') return 'VF'
    return name.slice(0, 2).toUpperCase()
  })

  const clusterMeta = computed(() => {
    if (!connected.value) return '等待连接 Station'
    const gpuText = gpuCount.value > 0 ? `${gpuCount.value} 卡 ${gpuModel.value}` : gpuModel.value
    return `Core ${coreStatus.value} · ${gpuText}`
  })

  function setIdentity(nextIdentity: HostIdentity) {
    identity.value = nextIdentity || {}
    connected.value = Boolean(nextIdentity && (nextIdentity.username || nextIdentity.user_id))
  }

  function setGpuCount(count: number) {
    if (count != null && count > 0) {
      gpuCount.value = count
    }
  }

  async function fetchSession() {
    try {
      const res = await request({ path: 'me', method: 'GET' })
      if (res.status === 200) {
        const data = await res.json()
        setIdentity(data)
        return data
      } else {
        connected.value = false
        return null
      }
    } catch {
      connected.value = false
      return null
    }
  }

  async function fetchCluster() {
    try {
      const [sumRes, gpuRes, healthRes] = await Promise.all([
        request({ path: 'resources/summary', method: 'GET' }).catch(() => null),
        request({ path: 'resources/gpu', method: 'GET' }).catch(() => null),
        request({ path: 'health', method: 'GET' }).catch(() => null)
      ])
      if (sumRes && sumRes.status === 200) {
        const data = await sumRes.json()
        if (data.summary?.total_gpus != null) {
          gpuCount.value = data.summary.total_gpus
        }
      }
      if (gpuRes && gpuRes.status === 200) {
        const data = await gpuRes.json()
        if (data.gpus && data.gpus.length > 0) {
          gpuCount.value = data.gpus.length
          const model = data.gpus[0].model_name
          if (model && model.includes('5090')) {
            gpuModel.value = 'RTX 5090'
          } else if (model) {
            gpuModel.value = model
          }
        }
      }
      if (healthRes && healthRes.status === 200) {
        const data = await healthRes.json()
        if (data.components?.core) {
          coreStatus.value = data.components.core === 'Ready' ? '就绪' : data.components.core
        }
      }
    } catch {}
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
    gpuCount,
    gpuModel,
    coreStatus,
    loginOpen,
    displayName,
    teamName,
    avatar,
    clusterMeta,
    setIdentity,
    setGpuCount,
    fetchSession,
    fetchCluster,
    init
  }
})
