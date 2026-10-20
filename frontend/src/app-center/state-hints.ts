/** Notifications only ask for a read. Revision tokens are never ordered. */
export function subscribeStateHints(notify: (revision?: string) => void, factory: () => EventSource = () => new EventSource('/studio/appcenter/v1/events')): () => void {
  let stopped = false, current: EventSource | undefined, retry: ReturnType<typeof setTimeout> | undefined
  function connect() {
    if (stopped) return
    try {
      const stream = factory(); current = stream
      const failed = () => {
        if (stopped || current !== stream) return
        current = undefined; stream.close(); notify()
        retry = setTimeout(connect, 10000)
      }
      stream.addEventListener('refresh', event => {
        if (stopped || current !== stream) return
        try {
          if (typeof event.data !== 'string' || event.data.length > 512) return failed()
          const value = JSON.parse(event.data)
          if (!value || typeof value !== 'object' || Object.keys(value).length !== 2 || value.schemaVersion !== 1 || typeof value.revision !== 'string' || !/^[A-Za-z0-9:._-]{0,128}$/.test(value.revision)) return failed()
          notify(value.revision)
        } catch { failed() }
      })
      stream.onerror = failed
    } catch {
      // An unavailable EventSource implementation must not disable calibration.
      retry = setTimeout(connect, 10000)
    }
  }
  connect()
  return () => { stopped = true; retry !== undefined && clearTimeout(retry); current?.close(); current = undefined }
}

/** This clock is independent of ordinary expiry polling and every hint. */
export function createStateCalibration(read: () => Promise<void>, subscribe?: (notify: () => void) => () => void) {
  let scope: string | null = null, epoch = 0, stop: (() => void) | undefined, timer: ReturnType<typeof setInterval> | undefined
  function activate(next: string | null) {
    if (next === scope) return
    epoch++; scope = next; timer !== undefined && clearInterval(timer); stop?.(); stop = undefined
    if (!next) return
    const generation = epoch
    const notify = () => { if (generation === epoch) void read().catch(() => undefined) }
    timer = setInterval(notify, 30000)
    notify()
    try { stop = subscribe?.(notify) } catch { /* Periodic reads remain active. */ }
  }
  return { activate, dispose: () => activate(null) }
}
