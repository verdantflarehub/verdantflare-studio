/**
 * Hedges an idempotent read. The public tunnel occasionally leaves a request
 * on a reused connection without any response; instead of waiting for a
 * timeout, a duplicate starts after `hedgeMs` and the first success wins.
 * Up to `attempts` run in parallel, each bounded by `attemptMs`. A real
 * failure (any rejection other than an attempt's own timeout) settles the
 * read at once; caller cancellation aborts every attempt.
 */
export function withHedge<T>(run: (signal: AbortSignal) => Promise<T>, signal: AbortSignal, hedgeMs = 600, attempts = 3, attemptMs = 3000): Promise<T> {
  return new Promise<T>((resolve, reject) => {
    if (signal.aborted) { reject(signal.reason); return }
    const controllers: AbortController[] = []
    let settled = false, started = 0, running = 0
    let hedge: ReturnType<typeof setTimeout> | undefined
    const finish = (settle: () => void) => {
      if (settled) return
      settled = true
      clearTimeout(hedge)
      signal.removeEventListener('abort', cancelled)
      for (const controller of controllers) controller.abort()
      settle()
    }
    const cancelled = () => finish(() => reject(signal.reason))
    signal.addEventListener('abort', cancelled, { once: true })
    const launch = () => {
      if (settled || started >= attempts) return
      started++; running++
      const controller = new AbortController()
      controllers.push(controller)
      const limit = setTimeout(() => controller.abort(new DOMException('Attempt timed out', 'TimeoutError')), attemptMs)
      run(AbortSignal.any([signal, controller.signal])).then(value => finish(() => resolve(value)), error => {
        running--
        if (settled) return
        if (controller.signal.aborted && !signal.aborted) {
          // This attempt stalled; start another now unless one is still running.
          if (running === 0) {
            if (started < attempts) launch()
            else finish(() => reject(error))
          }
          return
        }
        finish(() => reject(error))
      }).finally(() => clearTimeout(limit))
      clearTimeout(hedge)
      hedge = setTimeout(launch, hedgeMs)
    }
    launch()
  })
}
