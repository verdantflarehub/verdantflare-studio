/**
 * Bounds each attempt of an idempotent read and retries a stalled one. The
 * public tunnel occasionally leaves a reused connection's next streams
 * without a response after a page reload; a fresh attempt recovers it.
 * Caller cancellation and non-timeout failures are never retried here.
 */
export async function withAttemptTimeout<T>(run: (signal: AbortSignal) => Promise<T>, signal: AbortSignal, attemptMs = 3000, attempts = 3): Promise<T> {
  for (let attempt = 1; ; attempt++) {
    const timeout = AbortSignal.timeout(attemptMs)
    try { return await run(AbortSignal.any([signal, timeout])) }
    catch (error) { if (signal.aborted || !timeout.aborted || attempt >= attempts) throw error }
  }
}
