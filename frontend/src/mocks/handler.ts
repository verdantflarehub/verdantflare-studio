import { mockIdentity } from './identity'
import { mockResourcesSummary, mockGpus, mockNode, mockWorkloads, mockHealth } from './telemetry'
import { mockLiveApps } from './apps'
import { mockProxies } from './proxies'
import { mockAssets, mockAssetGet, mockCreatedProjectID, mockEntryTexts, mockProjectOpen, mockProjects, mockSavedRevision } from './project-world'

export interface MockRequestInput {
  path: string
  method: string
  body?: unknown
}

function jsonResponse(data: unknown, status = 200): Response {
  return new Response(JSON.stringify(data), {
    status,
    headers: {
      'Content-Type': 'application/json',
      'X-Mock-Data': 'true'
    }
  })
}

/**
 * Handle API request using mock data.
 * Returns a Response if the endpoint is mocked, or null to delegate to real backend.
 */
export async function handleMockRequest(input: MockRequestInput): Promise<Response | null> {
  const method = (input.method || 'GET').toUpperCase()
  const rawPath = input.path.replace(/^\/+/, '')
  const [basePath, queryStr] = rawPath.split('?')

  // Project / World MCP tool calls used by the live Studio pages.
  if (basePath === 'mcp' && method === 'POST') {
    const payload = input.body as { id?: string; params?: { name?: string; arguments?: Record<string, unknown> } } | undefined
    const name = payload?.params?.name
    const args = payload?.params?.arguments || {}
    let result: unknown
    if (name === 'project.list') {
      result = { items: mockProjects, next_cursor: '' }
    } else if (name === 'project.open') {
      result = mockProjectOpen(String(args.project_id || ''))
    } else if (name === 'project.create') {
      if (!mockProjects.some(item => item.project_id === mockCreatedProjectID)) {
        mockProjects.unshift({ project_id: mockCreatedProjectID, head_revision_id: '0192f3d4-4333-7aaa-8bbb-1234567890ab', name: String(args.name || '新建项目'), category: String(args.category || 'music'), status: 'draft', created_at: new Date().toISOString() })
        mockEntryTexts.set(mockCreatedProjectID, String(args.entry_text || '# 制作审核记录'))
      }
      result = mockProjectOpen(mockCreatedProjectID)
    } else if (name === 'project.commit') {
      const projectID = String(args.project_id || '')
      const changes = args.changes as { upsert_files?: Array<{ file_id?: string; text?: string }> } | undefined
      const text = changes?.upsert_files?.find(file => typeof file.text === 'string')?.text
      if (text !== undefined) mockEntryTexts.set(projectID, text)
      const project = mockProjects.find(item => item.project_id === projectID)
      if (project) project.head_revision_id = mockSavedRevision
      result = mockProjectOpen(projectID)
    } else if (name === 'project.use_asset') {
      const opened = mockProjectOpen(String(args.project_id || ''))
      opened.manifest.asset_refs = [{ asset_id: String(args.asset_id || ''), asset_version_id: String(args.asset_version_id || ''), purpose: String(args.purpose || 'asset-reference') }]
      result = opened
    } else if (name === 'world.list') {
      const wanted = typeof args.asset_type === 'string' ? args.asset_type : ''
      result = { items: mockAssets.filter(item => !wanted || item.asset_type === wanted), next_cursor: '' }
    } else if (name === 'world.get') {
      result = mockAssetGet(String(args.asset_id || ''), String(args.asset_version_id || ''))
    } else if (name === 'artifact.read') {
      const access = args.access as { project_id?: string } | undefined
      const text = mockEntryTexts.get(String(access?.project_id || '')) || '# 制作审核记录'
      result = { mode: 'text', version: { content_ref: args.content_ref }, text }
    } else {
      return jsonResponse({ jsonrpc: '2.0', id: payload?.id, error: { code: -32601, message: 'Unknown mock tool' } }, 404)
    }
    return jsonResponse({ jsonrpc: '2.0', id: payload?.id, result })
  }

  // 1. Identity & Session
  if (basePath === 'me') {
    return jsonResponse(mockIdentity)
  }

  if (basePath === 'login' && method === 'POST') {
    return jsonResponse({
      token: 'mock-session-token',
      identity: mockIdentity
    })
  }

  if (basePath === 'logout' && method === 'POST') {
    return new Response(null, { status: 204 })
  }

  // 2. Health & Cluster Status
  if (basePath === 'health') {
    return jsonResponse(mockHealth)
  }

  // 3. Telemetry & Hardware
  if (basePath === 'resources/summary') {
    return jsonResponse(mockResourcesSummary)
  }

  if (basePath === 'resources/gpu') {
    return jsonResponse(mockGpus)
  }

  if (basePath === 'resources/node') {
    return jsonResponse(mockNode)
  }

  if (basePath === 'resources/workloads') {
    return jsonResponse(mockWorkloads())
  }

  // 4. Apps & Catalog
  if (basePath === 'apps' && method === 'GET') {
    return jsonResponse({ items: mockLiveApps })
  }

  if (basePath.startsWith('apps/')) {
    const appId = decodeURIComponent(basePath.slice('apps/'.length))
    const app = mockLiveApps.find(a => a.app_id === appId)
    if (app) {
      return jsonResponse({ app })
    }
    return jsonResponse({
      app: {
        app_id: appId,
        display_name: appId,
        deployment: { state: 'not_installed', ready_replicas: 0, desired_replicas: 0, images: [] }
      }
    })
  }

  // 5. Network Egress / Station Proxies
  if (basePath === 'proxies' || basePath === 'station/proxies') {
    return jsonResponse({ items: mockProxies })
  }

  // 6. Operations & Commands
  if (basePath === 'commands' && method === 'POST') {
    return jsonResponse({
      operation_id: 'op-mock-' + Date.now(),
      status: 'accepted',
      phase: 'checking',
      download: { downloaded_bytes: 0, total_bytes: 104857600, percent: 0 }
    }, 202)
  }

  if (basePath.startsWith('operations/')) {
    const opId = basePath.slice('operations/'.length)
    return jsonResponse({
      operation_id: opId,
      status: 'succeeded',
      phase: 'completed',
      download: { downloaded_bytes: 104857600, total_bytes: 104857600, percent: 100 }
    })
  }

  // Not matched by mock handler
  return null
}
