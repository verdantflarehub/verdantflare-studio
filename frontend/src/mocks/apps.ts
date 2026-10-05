export interface MockLiveApp {
  app_id: string
  display_name?: string
  group_id?: string
  brand?: string
  version?: string
  gpu?: number
  models?: string[]
  description?: string
  tagline?: string
  deployment: {
    state: string
    ready_replicas: number
    desired_replicas: number
    observed_at?: string
    images?: Array<{ component: string; image: string }>
  }
}

export const mockLiveApps: MockLiveApp[] = [
  {
    app_id: 'image-mcp-server',
    display_name: 'Image MCP',
    group_id: 'image',
    brand: 'vf',
    version: '0.1.23',
    gpu: 1,
    models: ['Flux.1-Dev (FP8)'],
    description: '连接 Codex 与 Gemini，完成图像生成、编辑和局部重绘。',
    deployment: {
      state: 'ready',
      ready_replicas: 1,
      desired_replicas: 1,
      observed_at: '2026-10-05T12:00:00Z',
      images: [
        {
          component: 'image-mcp-server',
          image: 'registry.dev.verdantflarehub.com/studio/image-mcp:v0.1.23'
        }
      ]
    }
  },
  {
    app_id: 'video-mcp-server',
    display_name: 'Video MCP',
    group_id: 'video',
    brand: 'vf',
    version: '0.1.35',
    gpu: 1,
    models: ['Wan 2.1 14B'],
    description: '视频创作与生成引擎，支持图生视频、文生视频与长时序渲染。',
    deployment: {
      state: 'ready',
      ready_replicas: 1,
      desired_replicas: 1,
      observed_at: '2026-10-05T12:00:00Z',
      images: [
        {
          component: 'video-mcp-server',
          image: 'registry.dev.verdantflarehub.com/studio/video-mcp:v0.1.35'
        }
      ]
    }
  },
  {
    app_id: 'music-mcp-server',
    display_name: 'Music MCP',
    group_id: 'music',
    brand: 'vf',
    version: '0.6.0',
    gpu: 0,
    models: [],
    description: '统一调用音乐生成、分轨、人声、歌词对齐与母带处理。',
    deployment: {
      state: 'stopped',
      ready_replicas: 0,
      desired_replicas: 0,
      observed_at: '2026-10-05T10:00:00Z',
      images: []
    }
  }
]
