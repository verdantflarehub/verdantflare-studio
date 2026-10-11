/** Consumers share one refresh; invalidation prevents old work owning new scope. */
export function createSharedRefresh(read: () => Promise<void>, settled: () => void) {
  let epoch = 0, pending: Promise<void> | undefined
  function invalidate() { epoch++; pending = undefined }
  function refresh(): Promise<void> {
    if (pending) return pending
    const generation = epoch
    const task = Promise.resolve().then(() => generation === epoch ? read() : undefined).finally(() => {
      if (pending === task) { pending = undefined; settled() }
    })
    pending = task
    return task
  }
  return { refresh, invalidate }
}
