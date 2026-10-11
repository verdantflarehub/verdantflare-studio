import { validAppCommand, validCommandOperation } from './schema.generated.ts'
import type { Identity } from './controller'
import { commandScope, parsePending } from './command-storage.ts'
import type { CommandLease, CommandStorage, PendingCommand } from './command-storage'
import { operationMatches } from './command-transport.ts'

const invalid = (): never => { throw new Error('APPCENTER_LEGACY_RECOVERY_INVALID') }
const object = (value: unknown): value is Record<string, unknown> => value !== null && typeof value === 'object' && !Array.isArray(value)
export function legacyRecords(raw: string, identity: Identity): PendingCommand[] | null {
  if (new TextEncoder().encode(raw).length > 2 << 20) return invalid()
  let value: unknown
  try { value = JSON.parse(raw) } catch { return invalid() }
  if (!object(value)) return invalid()
  // Another identity's records are neither imported, shown, nor erased.
  if (value.user !== identity.userId || value.organization !== identity.organizationId || value.station !== identity.stationId) return null
  if (Object.keys(value).sort().join(',') !== 'items,organization,station,user' || !Array.isArray(value.items) || value.items.length > 100) return invalid()
  const keys = new Set<string>(), apps = new Set<string>()
  return value.items.map(item => {
    if (!Array.isArray(item) || item.length !== 2 || typeof item[0] !== 'string' || !object(item[1])) return invalid()
    const entry = item[1], command = entry.command
    if (Object.keys(entry).some(key => !['command', 'op', 'error', 'rejected', 'sending'].includes(key)) || !validAppCommand(command)
      || command.app_id !== item[0] || command.organization_id !== identity.organizationId || command.station_id !== identity.stationId
      || keys.has(command.idempotency_key) || apps.has(command.app_id)) return invalid()
    keys.add(command.idempotency_key); apps.add(command.app_id)
    let operationId: string | null = null
    if (entry.op !== undefined && entry.op !== null) {
      if (!validCommandOperation(entry.op) || !operationMatches(entry.op, { command })) return invalid()
      operationId = entry.op.operation_id
    }
    const record: PendingCommand = { key: command.idempotency_key, body: JSON.stringify(command), operationId, protocol: 'legacy' }
    parsePending(record, commandScope(identity))
    return record
  })
}

export interface LegacyRecoverySource { import(storage: CommandStorage, lease: CommandLease, identity: Identity, current: () => void): Promise<void> }
export function createLegacyRecoverySource(source: () => Pick<Storage, 'getItem' | 'removeItem'> = () => sessionStorage): LegacyRecoverySource {
  return { async import(storage, lease, identity, current) {
    let session: Pick<Storage, 'getItem' | 'removeItem'>, raw: string | null
    try { session = source(); raw = session.getItem('studio.operations') } catch { throw new Error('APPCENTER_LEGACY_STORAGE_UNAVAILABLE') }
    if (raw === null) return
    const records = legacyRecords(raw, identity)
    if (records === null) return
    current()
    await storage.importLegacy(lease, records)
    current()
    // A concurrent old page may replace its entry while migration is pending.
    // Do not erase that newer value or send commands until it is imported too.
    if (session.getItem('studio.operations') !== raw) throw new Error('APPCENTER_LEGACY_CONFLICT')
    session.removeItem('studio.operations')
  } }
}
