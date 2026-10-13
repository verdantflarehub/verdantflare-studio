/** Compatibility environment for a single trusted Dashboard Window. */
export function installDashboardRuntime(target: Window & typeof globalThis, _now?: () => number): () => void {
  const timers = new Set<number>(), intervals = new Set<number>()
  const setTimeout = target.setTimeout.bind(target), clearTimeout = target.clearTimeout.bind(target)
  const setInterval = target.setInterval.bind(target), clearInterval = target.clearInterval.bind(target)
  let disposed = false
  target.setTimeout = ((handler: TimerHandler, delay?: number, ...args: unknown[]) => {
    if (disposed) return 0
    let handle: number
    handle = setTimeout(typeof handler === 'function' ? (...values: unknown[]) => {
      timers.delete(handle)
      if (!disposed) Reflect.apply(handler, target, values)
    } : handler, delay, ...args)
    timers.add(handle)
    return handle
  }) as typeof target.setTimeout
  target.setInterval = ((handler: TimerHandler, delay?: number, ...args: unknown[]) => {
    if (disposed) return 0
    const handle = setInterval(handler, delay, ...args)
    intervals.add(handle)
    return handle
  }) as typeof target.setInterval
  // Native clearTimeout and clearInterval share the same handle space.
  target.clearTimeout = target.clearInterval = ((handle?: number) => {
    timers.delete(handle!); intervals.delete(handle!); clearTimeout(handle)
  }) as typeof target.clearTimeout
  return () => {
    disposed = true
    for (const handle of timers) clearTimeout(handle)
    for (const handle of intervals) clearInterval(handle)
    timers.clear(); intervals.clear()
  }
}
