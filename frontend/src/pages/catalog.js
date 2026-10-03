// VerdantFlare Market Catalog Data
// Source of truth: docs/htmls/verdantflare_studio_market_v1.1.html

export const CATALOG_APPS = [
  {
    app_id: 'image-mcp-server',
    display_name: 'Image MCP',
    group_id: 'image',
    brand: 'vf',
    version: '0.1.22',
    gpu: 0,
    models: [],
    tagline: '图像创作，从一个想法开始。',
    description: '连接 Codex 与 Gemini，完成图像生成、编辑和局部重绘。',
    capabilities: ['image.generate', 'image.edit', 'image.inpaint', 'image.faceswap'],
    dashboard: '/apps/image/dashboard'
  },
  {
    app_id: 'image-face-fusion-api',
    display_name: 'Face Fusion',
    group_id: 'image',
    brand: 'github',
    version: '0.1.2',
    gpu: 1,
    models: ['InsightFace'],
    tagline: '人脸融合与面部修复。',
    description: '为 Image MCP 提供本地换脸能力，支持面部融合及修复。',
    capabilities: ['image.faceswap'],
    dashboard: null
  },
  {
    app_id: 'music-mcp-server',
    display_name: 'Music MCP',
    group_id: 'music',
    brand: 'vf',
    version: '0.6.0',
    gpu: 0,
    models: [],
    tagline: '连接完整的音乐制作流程。',
    description: '统一调用音乐生成、分轨、人声、歌词对齐与母带处理。',
    capabilities: [
      'music.generate',
      'stems.separate',
      'voice.prepare',
      'voice.train',
      'voice.convert',
      'lyrics.align',
      'mix.master'
    ],
    dashboard: 'verdantflare_app_music_dashboard_v1.0.html'
  },
  {
    app_id: 'music-minimax-music3-api',
    display_name: 'MiniMax Music3',
    group_id: 'music',
    brand: 'minimax',
    version: '0.1.1',
    gpu: 2,
    models: ['MiniMax Music3'],
    tagline: '把歌词与描述变成音乐。',
    description: '本地音乐生成服务，为 Music MCP 提供高品质歌曲生成能力。',
    capabilities: ['music.generate'],
    dashboard: null
  },
  {
    app_id: 'music-uvr5-api',
    display_name: 'UVR5',
    group_id: 'music',
    brand: 'github',
    version: '0.1.7',
    gpu: 1,
    models: ['MelBand RoFormer', '去混响模型'],
    tagline: '分离人声，保留音乐细节。',
    description: '提供高精度伴奏与干声分离，以及去混响处理。',
    capabilities: ['stems.separate'],
    dashboard: null
  },
  {
    app_id: 'music-rvc-api',
    display_name: 'RVC',
    group_id: 'music',
    brand: 'github',
    version: '0.2.1',
    gpu: 1,
    models: ['RVC 基础模型', '项目声音模型（按需）'],
    tagline: '构建属于项目的人声。',
    description: '提供声音材料准备、声音模型训练与人声转换。',
    capabilities: ['voice.prepare', 'voice.train', 'voice.convert'],
    dashboard: null
  },
  {
    app_id: 'music-lyrics-aligner-api',
    display_name: 'Lyrics Aligner',
    group_id: 'music',
    brand: 'github',
    version: '0.1.0',
    gpu: 1,
    models: ['Whisper small'],
    tagline: '让每一句歌词落在对的时刻。',
    description: '将已批准歌词与实际音频对齐，生成歌词时间轴。',
    capabilities: ['lyrics.align'],
    dashboard: null
  },
  {
    app_id: 'music-audio-mixer-api',
    display_name: 'Audio Mixer',
    group_id: 'music',
    brand: 'github',
    version: '0.1.0',
    gpu: 0,
    models: [],
    tagline: '完成最后一道声音处理。',
    description: '音轨混合与母带制作，输出 WAV、MP3 和歌词文件。',
    capabilities: ['mix.master'],
    dashboard: null
  },
  {
    app_id: 'video-mcp-server',
    display_name: 'Video MCP',
    group_id: 'video',
    brand: 'vf',
    version: '0.11.9',
    gpu: 0,
    models: [],
    tagline: '让视频能力在 Station 汇合。',
    description: '统一视频工具与渠道。安装后查看能力，按需连接本地模型服务。',
    capabilities: ['video.generate', 'video.status', 'video.result'],
    dashboard: 'verdantflare_app_vedio_dashboard_v1.0.html'
  },
  {
    app_id: 'video-minimax-h3-vdn',
    display_name: 'MiniMax H3 VDN',
    group_id: 'video',
    brand: 'minimax',
    version: '0.3.4',
    gpu: 2,
    models: ['VDN H3 Ref2VA'],
    tagline: '为视频创作准备本地引擎。',
    description: '独立安装 H3 VDN 与所需模型，启动预热后接入 Video MCP 本地渠道。',
    capabilities: ['video.generate'],
    dashboard: null
  },
  {
    app_id: 'video-minimax-h3-api',
    display_name: 'MiniMax H3',
    group_id: 'video',
    brand: 'minimax',
    version: '0.3.1',
    gpu: 2,
    models: ['MiniMax H3'],
    tagline: 'MiniMax H3 本地推理。',
    description: '为 Video MCP 提供 H3 模型服务，独立管理部署与运行。',
    capabilities: ['video.generate'],
    dashboard: null
  },
  {
    app_id: 'video-depth-anything-api',
    display_name: 'Depth Anything',
    group_id: 'video',
    brand: 'github',
    version: '0.2.1',
    gpu: 1,
    models: ['Depth Anything'],
    tagline: '从视频中提取空间深度。',
    description: 'Video MCP 使用的内部深度转换服务。',
    capabilities: ['深度转换'],
    dashboard: null
  }
];

export function mergeCatalogWithLive(liveItems = []) {
  const liveMap = new Map();
  for (const item of liveItems) {
    if (item.app_id) liveMap.set(item.app_id, item);
  }

  const seenIds = new Set();
  const merged = CATALOG_APPS.map(cat => {
    seenIds.add(cat.app_id);
    const live = liveMap.get(cat.app_id);
    if (live) {
      return {
        ...cat,
        ...live,
        display_name: live.display_name || cat.display_name,
        group_id: live.group_id || cat.group_id,
        brand: live.brand || cat.brand,
        version: live.version || cat.version,
        gpu: live.gpu !== undefined ? live.gpu : cat.gpu,
        models: (live.models && live.models.length) ? live.models : cat.models,
        description: cat.description || live.description,
        deployment: live.deployment || {
          state: 'ready',
          ready_replicas: 1,
          desired_replicas: 1,
          images: []
        }
      };
    }
    return {
      ...cat,
      deployment: {
        state: 'not_installed',
        ready_replicas: 0,
        desired_replicas: 0,
        images: []
      },
      images: []
    };
  });

  // Preserve any live app returned by Station that is not explicitly in CATALOG_APPS
  for (const item of liveItems) {
    if (item.app_id && !seenIds.has(item.app_id)) {
      merged.push({
        ...item,
        tagline: item.description || '',
        description: item.description || '提供应用入口，无本地模型依赖',
        models: item.models || [],
        capabilities: item.capabilities || [],
        deployment: item.deployment || {
          state: 'ready',
          ready_replicas: 1,
          desired_replicas: 1,
          images: []
        }
      });
    }
  }

  return merged;
}
