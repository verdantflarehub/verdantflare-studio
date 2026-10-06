import { handleMockRequest } from '../mocks'
import { isMockEnabled } from '../mocks/config'

interface MCPError {
  code?: number
  message?: string
}

interface MCPEnvelope<T> {
  result?: T
  error?: MCPError
}

export class MCPRequestError extends Error {
  readonly status: number
  readonly code?: number

  constructor(message: string, status: number, code?: number) {
    super(message)
    this.name = 'MCPRequestError'
    this.status = status
    this.code = code
  }
}

function requestID(): string {
  const bytes = new Uint8Array(16)
  crypto.getRandomValues(bytes)
  const now = BigInt(Date.now())
  bytes[0] = Number((now >> 40n) & 0xffn)
  bytes[1] = Number((now >> 32n) & 0xffn)
  bytes[2] = Number((now >> 24n) & 0xffn)
  bytes[3] = Number((now >> 16n) & 0xffn)
  bytes[4] = Number((now >> 8n) & 0xffn)
  bytes[5] = Number(now & 0xffn)
  bytes[6] = (bytes[6] & 0x0f) | 0x70
  bytes[8] = (bytes[8] & 0x3f) | 0x80
  const hex = Array.from(bytes, byte => byte.toString(16).padStart(2, '0')).join('')
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`
}

/** Create a UUIDv7 for a business idempotency key. */
export function uuidv7(): string {
  return requestID()
}

export async function callTool<T>(name: string, arguments_: Record<string, unknown>, projectID?: string): Promise<T> {
  const payload = {
    jsonrpc: '2.0',
    id: requestID(),
    method: 'tools/call',
    params: { name, arguments: arguments_ }
  }
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    Accept: 'application/json'
  }
  if (projectID) headers['X-Project-Id'] = projectID

  let response: Response
  if (isMockEnabled()) {
    const mocked = await handleMockRequest({ path: 'mcp', method: 'POST', body: payload })
    if (mocked) response = mocked
    else throw new MCPRequestError('MCP mock route is unavailable', 503)
  } else {
    response = await fetch('/mcp', {
      method: 'POST',
      credentials: 'same-origin',
      cache: 'no-store',
      headers,
      body: JSON.stringify(payload)
    })
  }

  let envelope: MCPEnvelope<T>
  try {
    envelope = await response.json() as MCPEnvelope<T>
  } catch {
    throw new MCPRequestError('Studio MCP returned an invalid response', response.status)
  }
  if (!response.ok || envelope.error) {
    throw new MCPRequestError(envelope.error?.message || `Studio MCP request failed (${response.status})`, response.status, envelope.error?.code)
  }
  if (envelope.result === undefined) {
    throw new MCPRequestError('Studio MCP response has no result', response.status)
  }
  return envelope.result
}
