export interface ScrollViewport {
  read(): number
  write(top: number): void
  rendered(): boolean
  subscribe(listener: () => void): () => void
  schedule(listener: () => void): number
  cancel(handle: number): void
}

// Host-only bridge: a hidden surface must not replace saved scroll with the
// browser's clamped zero. Route restoration can arrive before activation.
export function createDashboardScrollBridge(viewport: ScrollViewport) {
  let visible = false, disposed = false, saved = 0, pending: number | null = null
  let frame: number | undefined
  function capture() { if (visible && !disposed && viewport.rendered()) saved = viewport.read() }
  const unsubscribe = viewport.subscribe(capture)
  function cancel() { if (frame !== undefined) viewport.cancel(frame); frame = undefined }
  function restore() {
    cancel()
    frame = viewport.schedule(() => {
      frame = undefined
      if (!visible || disposed || !viewport.rendered()) return
      const target = pending ?? saved
      viewport.write(target); saved = target; pending = null
    })
  }
  return {
    get() { if (disposed) return 0; capture(); return saved },
    restore(top: number) {
      if (disposed || !Number.isFinite(top) || top < 0 || top > 1_000_000) return
      pending = top
      if (visible) restore()
    },
    show() { if (!disposed) { visible = true; restore() } },
    hide() { capture(); visible = false; cancel() },
    dispose() { disposed = true; visible = false; saved = 0; pending = null; cancel(); unsubscribe() },
  }
}
