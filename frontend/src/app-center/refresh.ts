/** Consumers share one refresh; invalidation prevents old work owning new scope. */
export function createSharedRefresh(read: () => Promise<void>, settled: () => void) {
  let epoch = 0, pending: { promise: Promise<void>; again: boolean } | undefined
  function invalidate() { epoch++; pending = undefined }
  function refresh(): Promise<void> {
    if (pending) return pending.promise
    const generation = epoch
    const job = { promise: Promise.resolve(), again: false }
    job.promise = Promise.resolve().then(async () => {
      if (generation !== epoch) return
      do { job.again = false; await read() } while (job.again && generation === epoch)
    }).finally(() => {
      if (pending === job) { pending = undefined; settled() }
    })
    pending = job
    return job.promise
  }
  // An in-flight read may predate a hint. Keep one following read, without
  // canceling the current request or letting old scope work restart itself.
  function refreshLatest(): Promise<void> {
    if (pending) { pending.again = true; return pending.promise }
    return refresh()
  }
  return { refresh, refreshLatest, invalidate }
}
