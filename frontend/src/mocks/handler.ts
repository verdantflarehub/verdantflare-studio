import { mockIdentity } from './identity'
import { mockResourcesSummary, mockGpus, mockNode, mockWorkloads, mockHealth } from './telemetry'
import { mockLiveApps } from './apps'
import { mockProxies } from './proxies'

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
    return jsonResponse(mockWorkloads)
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
