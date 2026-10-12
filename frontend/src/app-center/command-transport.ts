import { AppCenterError } from './controller.ts'
import { validAppCommand, validCommandCore, validCommandOperation, validCommandPrepare, validCommandSubmit } from './schema.generated.ts'
import type { AppCommand, CommandCore, CommandOperation, CommandPrepare } from './schema.generated'

export interface CommandTransport {
  prepare(input: CommandPrepare, signal: AbortSignal): Promise<CommandCore>
  submit(body: string, signal: AbortSignal): Promise<CommandOperation>
  read(id: string, signal: AbortSignal): Promise<CommandOperation>
  legacy(body: string, signal: AbortSignal): Promise<CommandOperation>
}
const invalid = (): never => { throw new AppCenterError(502, 'APPCENTER_SUBMISSION_UNKNOWN') }
export async function commandResponse(response: Response, status: number): Promise<unknown> {
  if (!response.body || response.headers.get('Content-Type')?.split(';')[0].trim() !== 'application/json') return invalid()
  const reader = response.body.getReader(), decoder = new TextDecoder('utf-8', { fatal: true })
  let size = 0, text = ''
  try {
    for (;;) {
      const part = await reader.read(); if (part.done) break
      size += part.value.byteLength; if (size > 16 << 10) return invalid()
      text += decoder.decode(part.value, { stream: true })
    }
    text += decoder.decode()
    const value: unknown = JSON.parse(text)
    if (response.status !== status) {
      const code = value && typeof value === 'object' && 'code' in value && typeof value.code === 'string' && /^[A-Z][A-Z0-9_]{0,127}$/.test(value.code) ? value.code : 'APPCENTER_SUBMISSION_UNKNOWN'
      throw new AppCenterError(response.status, code)
    }
    return value
  } finally { await reader.cancel().catch(() => undefined); reader.releaseLock() }
}
export function operationMatches(operation: CommandOperation, prepared: { command: AppCommand }): boolean {
  const command = prepared.command
  return operation.organization_id === command.organization_id && operation.station_id === command.station_id
    && operation.app_id === command.app_id && operation.app_version === command.app_version && operation.action === command.action
}
export function createCommandTransport(fetcher: typeof fetch = fetch,
  readOperation: (id: string, signal: AbortSignal) => Promise<Response> = (id, signal) => fetcher(`/api/operations/${id}`, { credentials: 'same-origin', cache: 'no-store', redirect: 'error', signal }),
  submitLegacy?: (body: string, signal: AbortSignal) => Promise<Response>,
): CommandTransport {
  const post = (path: string, body: string, signal: AbortSignal) => fetcher(path, {
    method: 'POST', credentials: 'same-origin', cache: 'no-store', redirect: 'error', headers: { 'Content-Type': 'application/json' }, body, signal,
  })
  return {
    async legacy(body, signal) {
      const command: unknown = JSON.parse(body)
      if (new TextEncoder().encode(body).length > 16 << 10 || !validAppCommand(command)) throw new AppCenterError(400, 'INVALID_APPCENTER_REQUEST')
      const response = submitLegacy ? await submitLegacy(body, signal) : await post('/api/commands', body, signal)
      const value = await commandResponse(response, 202)
      if (!validCommandOperation(value) || !operationMatches(value, { command })) return invalid()
      return value
    },
    async prepare(input, signal) {
      if (!validCommandPrepare(input)) throw new AppCenterError(400, 'INVALID_APPCENTER_REQUEST')
      const value = await commandResponse(await post('/studio/appcenter/v1/commands/prepare', JSON.stringify(input), signal), 200)
      if (!validCommandCore(value) || value.command.request_id !== input.requestId || value.command.idempotency_key !== input.idempotencyKey || value.command.action !== input.actionId) return invalid()
      return value
    },
    async submit(body, signal) {
      const parsed: unknown = JSON.parse(body)
      if (new TextEncoder().encode(body).length > 16 << 10 || !validCommandSubmit(parsed)) throw new AppCenterError(400, 'INVALID_APPCENTER_REQUEST')
      const value = await commandResponse(await post('/studio/appcenter/v1/commands', body, signal), 202)
      if (!validCommandOperation(value) || !operationMatches(value, parsed.prepared)) return invalid()
      return value
    },
    async read(id, signal) {
      if (!/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(id)) return invalid()
      const value = await commandResponse(await readOperation(id, signal), 200)
      if (!validCommandOperation(value) || value.operation_id !== id) return invalid()
      return value
    },
  }
}
