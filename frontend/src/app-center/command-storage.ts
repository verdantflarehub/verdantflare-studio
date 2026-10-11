import { validAppCommand, validCommandSubmit } from './schema.generated.ts'
import type { Identity } from './controller'
import type { AppCommand, CommandCore } from './schema.generated'

export interface PendingCommand { key: string; body: string; operationId: string | null; protocol?: 'legacy' }
export interface CommandLease { scope: string; epoch: number }
export interface CommandStorage {
  load(scope: string): Promise<{ lease: CommandLease; records: PendingCommand[] }>
  put(lease: CommandLease, record: PendingCommand): Promise<void>
  remove(lease: CommandLease, key: string): Promise<void>
  clear(scope: string): Promise<void>
  importLegacy(lease: CommandLease, records: PendingCommand[]): Promise<void>
}
export const commandScope = (identity: Identity) => JSON.stringify([identity.userId, identity.organizationId, identity.stationId])
export const storageError = () => new Error('APPCENTER_RECOVERY_STORAGE_UNAVAILABLE')
export function parsePending(record: PendingCommand, scope: string): CommandCore | { command: AppCommand } {
  if (!record || Object.keys(record).sort().join(',') !== (record.protocol === 'legacy' ? 'body,key,operationId,protocol' : 'body,key,operationId') || typeof record.body !== 'string'
    || new TextEncoder().encode(JSON.stringify(record)).length > 16 << 10) throw storageError()
  const value: unknown = JSON.parse(record.body)
  const input = record.protocol === 'legacy' ? validAppCommand(value) ? { command: value } : null : validCommandSubmit(value) ? value.prepared : null
  if (!input || JSON.stringify(value) !== record.body || input.command.idempotency_key !== record.key) throw storageError()
  const who: unknown = JSON.parse(scope)
  if (!Array.isArray(who) || who.length !== 3 || who[1] !== input.command.organization_id || who[2] !== input.command.station_id
    || record.operationId !== null && !/^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(record.operationId)) throw storageError()
  return input
}

/** Scope epochs make logout invalidate writes from other tabs and late replies. */
export function createCommandStorage(factory: IDBFactory = globalThis.indexedDB): CommandStorage {
  let opened: Promise<IDBDatabase> | undefined
  function open(): Promise<IDBDatabase> {
    if (!factory) return Promise.reject(storageError())
    if (!opened) opened = new Promise((resolve, reject) => {
      const request = factory.open('studio.appcenter.commands.v1', 1)
      request.onupgradeneeded = () => request.result.createObjectStore('scopes')
      request.onerror = () => { opened = undefined; reject(storageError()) }
      request.onblocked = () => { opened = undefined; reject(storageError()) }
      request.onsuccess = () => {
        const db = request.result
        db.onversionchange = () => { db.close(); opened = undefined }
        resolve(db)
      }
    })
    return opened
  }
  interface Bucket { epoch: number; records: PendingCommand[] }
  async function transaction<T>(scope: string, mode: IDBTransactionMode, apply: (bucket: Bucket) => T, validateRecords = true): Promise<T> {
    const db = await open()
    return new Promise((resolve, reject) => {
      let result: T, failure: unknown
      // Resolve only after transaction completion, never after request success.
      const tx = db.transaction('scopes', mode, mode === 'readwrite' ? { durability: 'strict' } : undefined)
      const store = tx.objectStore('scopes'), get = store.get(scope)
      get.onsuccess = () => {
        try {
          const bucket: Bucket = get.result ?? { epoch: 0, records: [] }
          if (!Number.isSafeInteger(bucket.epoch) || bucket.epoch < 0 || bucket.epoch >= Number.MAX_SAFE_INTEGER) throw storageError()
          if (validateRecords && (!Array.isArray(bucket.records) || bucket.records.length > 100)) throw storageError()
          const keys = new Set<string>()
          if (validateRecords) for (const record of bucket.records) { parsePending(record, scope); if (keys.has(record.key)) throw storageError(); keys.add(record.key) }
          result = apply(bucket)
          if (mode === 'readwrite') store.put(bucket, scope)
        } catch (error) { failure = error; tx.abort() }
      }
      tx.oncomplete = () => resolve(result)
      tx.onerror = tx.onabort = () => reject(failure ?? storageError())
    })
  }
  const check = (bucket: Bucket, lease: CommandLease) => { if (bucket.epoch !== lease.epoch) throw new Error('APPCENTER_RECOVERY_SCOPE_CHANGED') }
  return {
    load: scope => transaction(scope, 'readonly', bucket => ({ lease: { scope, epoch: bucket.epoch }, records: bucket.records })),
    put: (lease, record) => transaction(lease.scope, 'readwrite', bucket => {
      check(bucket, lease); const prepared = parsePending(record, lease.scope)
      const index = bucket.records.findIndex(item => item.key === record.key)
      if (index >= 0) {
        const old = bucket.records[index]
        if (old.body !== record.body || old.operationId && old.operationId !== record.operationId) throw storageError()
        bucket.records[index] = record
      } else {
        if (bucket.records.length >= 100) throw new Error('APPCENTER_RECOVERY_LIMIT')
        if (bucket.records.some(item => parsePending(item, lease.scope).command.app_id === prepared.command.app_id)) throw new Error('APPCENTER_RECOVERY_PENDING')
        bucket.records.push(record)
      }
    }),
    remove: (lease, key) => transaction(lease.scope, 'readwrite', bucket => { check(bucket, lease); bucket.records = bucket.records.filter(item => item.key !== key) }),
    clear: scope => transaction(scope, 'readwrite', bucket => { bucket.records = []; bucket.epoch++ }, false),
    importLegacy: (lease, records) => transaction(lease.scope, 'readwrite', bucket => {
      check(bucket, lease)
      for (const record of records) {
        if (record.protocol !== 'legacy') throw storageError()
        parsePending(record, lease.scope)
        const index = bucket.records.findIndex(item => item.key === record.key)
        if (index < 0) { bucket.records.push(record); continue }
        const old = bucket.records[index]
        if (old.protocol !== 'legacy' || old.body !== record.body || old.operationId && record.operationId && old.operationId !== record.operationId) throw new Error('APPCENTER_LEGACY_CONFLICT')
        if (!old.operationId && record.operationId) bucket.records[index] = record
      }
      if (bucket.records.length > 100) throw new Error('APPCENTER_RECOVERY_LIMIT')
    }),
  }
}
