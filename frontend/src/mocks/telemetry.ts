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

export const mockWorkloads = {
  summary: {
    total_pods: 6,
    gpu_pods: 2,
    infra_pods: 4,
    total_gpu_assigned: 2,
    total_vram_used_mb: 18432,
    total_cpu_req_millicores: 16000,
    total_mem_req_mb: 32768
  },
  workloads: [
    {
      name: 'image-mcp-server-pod-01',
      display_name: 'Image MCP 工作台',
      type: 'gpu',
      status: 'Running',
      gpu_count_req: 1,
      vram_used_mb: 12288,
      cpu_cores_req: 4,
      mem_used_mb: 16384,
      mount_point: '/data/models/flux'
    },
    {
      name: 'video-mcp-server-pod-01',
      display_name: 'Video MCP 任务分派器',
      type: 'gpu',
      status: 'Ready',
      gpu_count_req: 1,
      vram_used_mb: 6144,
      cpu_cores_req: 4,
      mem_used_mb: 8192,
      mount_point: '/data/models/wan'
    },
    {
      name: 'station-core-runtime',
      display_name: 'Station Core 调度核心',
      type: 'infra',
      status: 'Running',
      gpu_count_req: 0,
      vram_used_mb: 0,
      cpu_cores_req: 2,
      mem_used_mb: 2048,
      mount_point: ''
    },
    {
      name: 'openclash-egress-gateway',
      display_name: 'OpenClash 海外网络出口',
      type: 'infra',
      status: 'Running',
      gpu_count_req: 0,
      vram_used_mb: 0,
      cpu_cores_req: 1,
      mem_used_mb: 1024,
      mount_point: ''
    }
  ]
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
