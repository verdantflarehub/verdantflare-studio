import type { FileSelection, Json, SelectedFile } from './types'

const limit = 1024 * 1024
const handlePattern = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
let pickerBusy = false
const fail = (code: string): never => { throw new Error(code) }

/** Called synchronously from the SDK's verified user gesture, in the host DOM. */
export function pickJSONFiles(input: FileSelection, signal: AbortSignal): Promise<readonly File[]> {
  if (signal.aborted) return Promise.reject(new Error('REQUEST_CANCELLED'))
  if (pickerBusy) return Promise.reject(new Error('FILE_PICKER_BUSY'))
  pickerBusy = true
  return new Promise((resolve, reject) => {
    const element = document.createElement('input')
    element.type = 'file'; element.accept = '.json,application/json'; element.multiple = input.multiple === true
    element.hidden = true
    const cleanup = () => {
      pickerBusy = false; element.remove(); signal.removeEventListener('abort', cancelled)
      element.removeEventListener('change', changed); element.removeEventListener('cancel', empty)
    }
    const cancelled = () => { cleanup(); reject(new Error('REQUEST_CANCELLED')) }
    const empty = () => { cleanup(); resolve([]) }
    const changed = () => {
      const files = Array.from(element.files ?? [])
      cleanup(); resolve(files)
    }
    element.addEventListener('change', changed, { once: true })
    element.addEventListener('cancel', empty, { once: true })
    signal.addEventListener('abort', cancelled, { once: true })
    try {
      document.body.append(element)
      if (element.showPicker) element.showPicker()
      else element.click()
    } catch { cleanup(); reject(new Error('FILE_PICKER_UNAVAILABLE')) }
  })
}

function readBytes(file: File, signal: AbortSignal): Promise<ArrayBuffer> {
  return new Promise((resolve, reject) => {
    if (signal.aborted) { reject(new Error('REQUEST_CANCELLED')); return }
    const reader = new FileReader()
    const cleanup = () => { signal.removeEventListener('abort', abort); reader.onload = reader.onerror = reader.onabort = null }
    const abort = () => { cleanup(); reader.abort(); reject(new Error('REQUEST_CANCELLED')) }
    reader.onload = () => { const value = reader.result; cleanup(); value instanceof ArrayBuffer ? resolve(value) : reject(new Error('FILE_READ_FAILED')) }
    reader.onerror = () => { cleanup(); reject(new Error('FILE_READ_FAILED')) }
    reader.onabort = abort
    signal.addEventListener('abort', abort, { once: true })
    try { reader.readAsArrayBuffer(file) } catch { cleanup(); reject(new Error('FILE_READ_FAILED')) }
  })
}

export function createFileAccess(options: {
  expiresAt: string
  authorize(capability: 'files.select' | 'files.read', signal: AbortSignal): Promise<void>
  pick?: typeof pickJSONFiles
  read?: typeof readBytes
  now?: () => number
}) {
  const now = options.now ?? Date.now, expiry = Date.parse(options.expiresAt)
  if (!Number.isFinite(expiry)) fail('INVALID_FILE_SCOPE')
  const entries = new Map<string, { file: File; expires: number }>()
  let generation = 0, timer: ReturnType<typeof setTimeout> | undefined
  const pending = new Set<AbortController>()
  function sweep() {
    timer !== undefined && clearTimeout(timer)
    for (const [handle, entry] of entries) if (entry.expires <= now()) entries.delete(handle)
    if (entries.size) timer = setTimeout(sweep, Math.max(1, Math.min(...[...entries.values()].map(entry => entry.expires)) - now()))
  }
  function clear() {
    generation++; entries.clear(); timer !== undefined && clearTimeout(timer)
    for (const controller of pending) controller.abort()
  }
  async function operation<T>(signal: AbortSignal, fn: (signal: AbortSignal, check: () => void) => Promise<T>) {
    const current = generation, controller = new AbortController()
    const abort = () => controller.abort()
    const check = () => {
      if (signal.aborted || controller.signal.aborted || generation !== current) fail('REQUEST_CANCELLED')
      if (now() >= expiry) { clear(); fail('SESSION_EXPIRED') }
    }
    check(); signal.addEventListener('abort', abort, { once: true }); pending.add(controller)
    try { return await fn(controller.signal, check) }
    finally { signal.removeEventListener('abort', abort); pending.delete(controller) }
  }
  return {
    clear,
    select(input: FileSelection, signal: AbortSignal): Promise<readonly SelectedFile[]> {
      if (input.purpose !== 'json.import' || (input.multiple !== undefined && typeof input.multiple !== 'boolean')) fail('FILE_PURPOSE_UNSUPPORTED')
      return operation(signal, async (signal, check) => {
        // Do not await network before opening: browsers require transient activation.
        const files = [...await (options.pick ?? pickJSONFiles)(input, signal)]
        check()
        if (!files.length) return []
        if (files.length > (input.multiple ? 8 : 1)) fail('FILE_SELECTION_LIMIT')
        const metadata = files.map(file => {
          const name = file.name.split(/[/\\]/).pop() ?? ''
          if (!name || name.length > 255 || /[\x00-\x1f\x7f]/.test(name) || !/\.json$/i.test(name)
            || !Number.isSafeInteger(file.size) || file.size < 1 || file.size > limit) fail('INVALID_SELECTED_FILE')
          return { name, size: file.size, mime: 'application/json' }
        })
        await options.authorize('files.select', signal); check(); sweep()
        if (entries.size + files.length > 16) fail('FILE_HANDLE_LIMIT')
        const result = metadata.map((meta, index) => {
          const handle = crypto.randomUUID()
          entries.set(handle, { file: files[index]!, expires: Math.min(expiry, now() + 300_000) })
          return { ...meta, handle }
        })
        sweep(); return result
      })
    },
    read(input: Json, signal: AbortSignal): Promise<Json> {
      if (!input || typeof input !== 'object' || Array.isArray(input) || Object.keys(input).sort().join(',') !== 'handle,purpose'
        || input.purpose !== 'json.import' || typeof input.handle !== 'string' || !handlePattern.test(input.handle)) return fail('INVALID_FILE_REQUEST')
      const handle = input.handle as string
      return operation(signal, async (signal, check) => {
        sweep()
        const entry = entries.get(handle)
        const current = () => { check(); if (!entry || entries.get(handle) !== entry || entry.expires <= now()) fail('FILE_HANDLE_EXPIRED') }
        current(); await options.authorize('files.read', signal); current()
        let bytes: ArrayBuffer
        try { bytes = await (options.read ?? readBytes)(entry!.file, signal) } catch { check(); return fail('FILE_READ_FAILED') }
        current()
        if (bytes.byteLength !== entry!.file.size || bytes.byteLength > limit) fail('INVALID_SELECTED_FILE')
        let text: string
        try { text = new TextDecoder('utf-8', { fatal: true }).decode(bytes) } catch { return fail('INVALID_FILE_ENCODING') }
        await options.authorize('files.read', signal); current()
        entries.delete(handle); sweep()
        return { text }
      })
    },
  }
}
