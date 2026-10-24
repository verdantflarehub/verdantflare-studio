export type DashboardLayout = 'document' | 'fill'

export const MIN_FILL_HEIGHT = 480

export function isDashboardLayout(value: unknown): value is DashboardLayout {
  return value === 'document' || value === 'fill'
}

/** Height of a fill-layout surface: the visible viewport below the surface top, never below a usable minimum. */
export function fillSurfaceHeight(viewportHeight: number, surfaceTop: number): number {
  if (!Number.isFinite(viewportHeight) || !Number.isFinite(surfaceTop)) return MIN_FILL_HEIGHT
  return Math.max(MIN_FILL_HEIGHT, Math.floor(viewportHeight - Math.max(0, surfaceTop)))
}

export interface LayoutSurface {
  /** Distance from the document top to the surface top, measured with the page scrolled to 0. */
  surfaceTop(): number
  viewportHeight(): number
  setHeight(height: number | null): void
  setFillClass(active: boolean): void
  scrollToTop(): void
  subscribeResize(listener: () => void): () => void
}

/**
 * Host-only layout bridge for trusted Dashboards (AppDashboard contract §3 `dashboard.layouts`).
 * `document` follows reported content height; `fill` pins the surface to the viewport and ignores resize reports.
 */
export function createDashboardLayoutBridge(surface: LayoutSurface) {
  let mode: DashboardLayout = 'document'
  let visible = false
  let disposed = false
  let reported: number | null = null
  let unsubscribe = () => {}

  function measure() {
    if (disposed || mode !== 'fill' || !visible) return
    surface.setHeight(fillSurfaceHeight(surface.viewportHeight(), surface.surfaceTop()))
  }
  function activateFill() {
    surface.setFillClass(true)
    surface.scrollToTop()
    unsubscribe()
    unsubscribe = surface.subscribeResize(measure)
    measure()
  }
  function deactivateFill() {
    unsubscribe()
    unsubscribe = () => {}
    surface.setFillClass(false)
    surface.setHeight(reported)
  }
  return {
    get mode() { return mode },
    setLayout(next: unknown) {
      if (disposed || !isDashboardLayout(next) || next === mode) return
      mode = next
      if (!visible) return
      if (mode === 'fill') activateFill()
      else deactivateFill()
    },
    resize(height: number) {
      if (disposed) return
      reported = height
      if (mode === 'document') surface.setHeight(height)
    },
    show() {
      if (disposed) return
      visible = true
      if (mode === 'fill') activateFill()
    },
    hide() {
      if (disposed) return
      visible = false
      if (mode === 'fill') { unsubscribe(); unsubscribe = () => {}; surface.setFillClass(false) }
    },
    dispose() {
      if (disposed) return
      if (visible && mode === 'fill') surface.setFillClass(false)
      disposed = true
      unsubscribe()
    },
  }
}
