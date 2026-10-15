import type { AppSDKSession } from './sdk'
import type { AppContext, AppRoute, DashboardLaunch } from './types'
import type { CachedDashboard } from './cache'
import { installDashboardRuntime } from './runtime'
import { createDashboardScrollBridge } from './scroll'

export interface DashboardMountOptions {
  container: HTMLElement
  launch: DashboardLaunch
  session: AppSDKSession
  signal: AbortSignal
  confirmDiscard(): Promise<boolean>
  onInvalidated(): void
  releaseSession?(): Promise<void>
  now: () => number
}

export interface WujieDashboard extends CachedDashboard {
  updateContext(context: AppContext): Promise<void>
  updateRoute(route: AppRoute): void
}

/** Trusted Dashboards only. Wujie is a loader, not a security boundary. */
export async function mountWujieDashboard(options: DashboardMountOptions): Promise<WujieDashboard> {
  const { container, launch, session, signal } = options
  if (launch.adapter !== 'wujie') throw new Error('ADAPTER_UNSUPPORTED')
  const { startApp, destroyApp } = await import('wujie')
  if (signal.aborted) { session.dispose(); throw new Error('LOAD_CANCELLED') }
  // Opaque per-load identity prevents Wujie from reusing an old authorized sandbox.
  const name = `dashboard-${crypto.randomUUID()}`
  const element = document.createElement('div')
  element.className = 'app-dashboard-surface'
  element.hidden = true
  container.append(element)
  const controller = new AbortController()
  let disposed = false, dirty = true, ready = false
  const scroll = createDashboardScrollBridge({
    read: () => window.scrollY,
    write: top => window.scrollTo(0, top),
    rendered: () => element.getClientRects().length > 0,
    subscribe(listener) { window.addEventListener('scroll', listener, { passive: true }); return () => window.removeEventListener('scroll', listener) },
    schedule: listener => requestAnimationFrame(listener),
    cancel: handle => cancelAnimationFrame(handle),
  })
  let cleanup: Promise<void> | undefined
  let clearRuntime = () => {}
  let settleReady!: () => void
  let rejectReady!: (reason: Error) => void
  const readiness = new Promise<void>((resolve, reject) => { settleReady = resolve; rejectReady = reject })
  // Handles an early load failure before startApp has returned.
  void readiness.catch(() => undefined)
  let timer: ReturnType<typeof setTimeout> | undefined
  let unsubscribe = () => {}
  // Only host code reads the outer document. App code consumes AppSDK.theme.
  const updateTheme = () => session.setTheme(document.body.dataset.theme === 'dark' ? 'dark' : 'light')
  updateTheme()
  const themeObserver = new MutationObserver(updateTheme)
  themeObserver.observe(document.body, { attributes: true, attributeFilter: ['data-theme'] })

  function dispose(): Promise<void> {
    if (cleanup) return cleanup
    if (disposed) return Promise.resolve()
    disposed = true
    scroll.dispose()
    clearTimeout(timer)
    signal.removeEventListener('abort', cancel)
    unsubscribe()
    themeObserver.disconnect()
    session.dispose()
    clearRuntime()
    controller.abort()
    rejectReady(new Error('LOAD_CANCELLED'))
    element.remove()
    cleanup = Promise.allSettled([destroyApp(name), options.releaseSession?.()]).then(results => {
      const failed = results.find(result => result.status === 'rejected')
      if (failed?.status === 'rejected') throw failed.reason
    })
    return cleanup
  }
  function cancel() { void dispose().catch(() => undefined) }
  signal.addEventListener('abort', cancel, { once: true })
  unsubscribe = session.sdk.lifecycle.subscribe(event => {
    if (event === 'dispose') { cancel(); options.onInvalidated() }
  })

  // Loading metadata is a transport handshake, not an App business interface.
  const dashboardHost = Object.freeze({
    ready() {
      if (disposed || ready) return
      ready = true
      dirty = false
      settleReady()
    },
    setDirty(value: boolean) {
      if (!disposed && ready && typeof value === 'boolean') dirty = value
    },
    resize(height: number) {
      if (!disposed && Number.isSafeInteger(height) && height >= 0 && height <= 1_000_000) {
        element.style.height = `${height}px`
      }
    },
    getScroll() { return scroll.get() },
    restoreScroll(top: number) { scroll.restore(top) },
  })
  const fetchAssets: typeof fetch = async (input, init) => {
    const response = await fetch(input, { ...init, credentials: 'same-origin', signal: controller.signal })
    if (!response.ok) throw new Error('DASHBOARD_ASSET_FAILED')
    return response
  }
  timer = setTimeout(() => { rejectReady(new Error('DASHBOARD_LOAD_TIMEOUT')); cancel() }, 30_000)
  try {
    const loading = startApp({ name, url: launch.entryUrl, el: element, alive: true, sync: false,
      props: { appSDK: session.sdk, dashboardHost }, fetch: fetchAssets,
      beforeLoad: appWindow => { clearRuntime = installDashboardRuntime(appWindow as Window & typeof globalThis, options.now) },
      loadError: () => rejectReady(new Error('DASHBOARD_ASSET_FAILED')),
    })
    // A late Wujie completion must never revive a cancelled page.
    void loading.then(async () => { if (disposed) await destroyApp(name) }, () => undefined).catch(() => undefined)
    await Promise.race([
      Promise.all([loading, readiness]),
      new Promise<never>((_, reject) => {
        if (controller.signal.aborted) reject(new Error('LOAD_CANCELLED'))
        else controller.signal.addEventListener('abort', () => reject(new Error('LOAD_CANCELLED')), { once: true })
      }),
    ])
    if (disposed) throw new Error('LOAD_CANCELLED')
    clearTimeout(timer)
  } catch (error) {
    await dispose()
    throw error
  }
  return {
    show() { if (disposed) throw new Error('DASHBOARD_DISPOSED'); element.hidden = false; scroll.show(); session.setVisibility(true) },
    hide() { if (!disposed) { scroll.hide(); session.setVisibility(false); element.hidden = true } },
    async prepareClose() { return !dirty || await options.confirmDiscard() },
    async updateContext(context: AppContext) {
      const previous = session.sdk.context.get()
      if (context.projectId !== previous.projectId && dirty) {
        if (!await options.confirmDiscard()) throw new Error('CONTEXT_CHANGE_CANCELLED')
        dirty = false
      }
      session.updateContext(context)
    },
    updateRoute(route: AppRoute) { session.setRoute(route) },
    dispose,
  }
}
