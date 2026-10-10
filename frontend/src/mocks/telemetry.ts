export const mockResourcesSummary = {
  summary: {
    total_gpus: 2,
    total_vram_mb: 65536,
    used_vram_mb: 18432,
    host_cpu_utilization: 12.5,
    host_cpu_cores: 24,
    host_cpu_load1: 0.28,
    host_mem_total_gb: 256,
    host_mem_used_gb: 34.2,
    host_mem_used_percent: 13.4,
    storage_total_tb: 7.3,
    storage_used_tb: 0.55,
    storage_used_percent: 7.5
  }
}

export const mockGpus = {
  gpus: [
    {
      index: 0,
      model_name: 'NVIDIA GeForce RTX 5090',
      total_vram_mb: 32768,
      used_vram_mb: 12288,
      utilization: 28,
      temperature_c: 48,
      power_watts: 185
    },
    {
      index: 1,
      model_name: 'NVIDIA GeForce RTX 5090',
      total_vram_mb: 32768,
      used_vram_mb: 6144,
      utilization: 8,
      temperature_c: 42,
      power_watts: 75
    }
  ]
}

export const mockNode = {
  node: {
    name: 'dev-gpu-worker-01',
    cpu_cores: 24,
    cpu_utilization_percent: 12.5,
    cpu_load1: 0.28,
    cpu_load5: 0.32,
    cpu_load15: 0.25,
    mem_total_bytes: 274877906944,
    mem_used_bytes: 36700160000,
    mem_available_bytes: 238177746944,
    mem_cached_bytes: 18874368000,
    mem_buffers_bytes: 4194304000,
    mem_used_percent: 13.4,
    storage_disks: [
      {
        mountpoint: '/data',
        total_bytes: 3700000000000,
        used_bytes: 320000000000,
        used_percent: 8.6
      },
      {
        mountpoint: '/',
        total_bytes: 3500000000000,
        used_bytes: 230000000000,
        used_percent: 6.5
      }
    ]
  }
}

// Used only when the user explicitly enables mock mode; never a live fallback.
export function mockWorkloads() {
  const sampled_at = new Date().toISOString()
  return {
    schema_version: 2,
    updated_at: sampled_at,
    summary: { total_pods: 1, gpu_pods: 1, infra_pods: 0, total_gpu_requested: 1, total_cpu_requested: 2, total_memory_requested: 4294967296 },
    workloads: [{
      name: 'blender-example', display_name: '示例 Blender · 非真实遥测', namespace: 'verdantflare-example',
      pod_name: 'blender-example', pod_uid: 'mock-pod-uid', instance_alias: 'blenderExample',
      status: 'Running', age: '示例', type: 'gpu', created_at: sampled_at,
      containers: [{
        name: 'blender', ready: true, restarts: 0, gpu_request: 1, gpu_limit: 1,
        cpu: { value: 0.25, request: 2, limit: 8, sampled_at, quality: 'fresh', scope: 'instance_container', unit: 'cores' },
        memory: { value: 1610612736, request: 4294967296, limit: 17179869184, sampled_at, quality: 'fresh', scope: 'instance_container', unit: 'bytes' }
      }]
    }]
  }
}

export const mockHealth = {
  status: 'healthy',
  components: {
    core: 'Ready',
    k8s: 'Ready',
    postgres: 'Ready',
    minio: 'Ready'
  }
}
