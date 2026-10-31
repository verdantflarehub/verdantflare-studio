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

/**
 * State changes arrive as server hints; there is no periodic read. One read
 * happens on activation and whenever the stream reconnects (the server sends
 * an initial hint per connection). Without a stream (desktop) the optional
 * fallback interval keeps the previous calibration behaviour.
 */
export function createStateCalibration(read: () => Promise<void>, subscribe?: (notify: () => void) => () => void, fallbackInterval = 30000) {
  let scope: string | null = null, epoch = 0, stop: (() => void) | undefined, timer: ReturnType<typeof setInterval> | undefined
  function activate(next: string | null) {
    if (next === scope) return
    epoch++; scope = next; timer !== undefined && clearInterval(timer); timer = undefined; stop?.(); stop = undefined
    if (!next) return
    const generation = epoch
    const notify = () => { if (generation === epoch) void read().catch(() => undefined) }
    notify()
    if (!subscribe) { timer = setInterval(notify, fallbackInterval); return }
    try { stop = subscribe(notify) } catch { timer = setInterval(notify, fallbackInterval) }
  }
  return { activate, dispose: () => activate(null) }
}
