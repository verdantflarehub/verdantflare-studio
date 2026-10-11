import type { OpenedDashboard } from './web-client'

interface SessionTransport {
  open(id: string, signal?: AbortSignal): Promise<OpenedDashboard>
  inspect(sessionId: string, installationId: string, signal?: AbortSignal): Promise<OpenedDashboard>
  close(sessionId: string, signal?: AbortSignal): Promise<void>
}

/** Owned by one AppCenter view. Wujie eviction releases the matching session. */
export class DashboardSessions {
  private entries = new Map<string, OpenedDashboard>()
  private generation = 0
  private closing = new Map<string, Promise<void>>()
  private selecting = new Map<string, symbol>()
  constructor(private transport: SessionTransport) {}

  async select(installationId: string, signal: AbortSignal): Promise<OpenedDashboard> {
    const generation = this.generation, previous = this.entries.get(installationId)
    const selection = Symbol()
    this.selecting.set(installationId, selection)
    try {
      const opened = previous
        ? await this.transport.inspect(previous.launch.dashboardSessionId, installationId, signal)
        : await this.transport.open(installationId, signal)
      if (generation !== this.generation || signal.aborted || this.selecting.get(installationId) !== selection) {
        if (!previous) await this.close(opened.launch.dashboardSessionId)
        throw new Error('REQUEST_CANCELLED')
      }
      this.entries.set(installationId, opened)
      return opened
    } finally {
      if (this.selecting.get(installationId) === selection) this.selecting.delete(installationId)
    }
  }

  async release(installationId: string, sessionId: string): Promise<void> {
    if (this.entries.get(installationId)?.launch.dashboardSessionId === sessionId) {
      this.entries.delete(installationId)
      this.selecting.delete(installationId)
    }
    await this.close(sessionId)
  }

  private close(id: string): Promise<void> {
    // SDK is already revoked by its owner. Offline server sessions expire under
    // their original deadline; a failed close must not retain local capabilities.
    const existing = this.closing.get(id)
    if (existing) return existing
    const operation = this.transport.close(id, AbortSignal.timeout(5000)).catch(() => undefined)
      .finally(() => { this.closing.delete(id) })
    this.closing.set(id, operation)
    return operation
  }

  async revoke(): Promise<void> {
    this.generation++
    this.selecting.clear()
    const previous = [...this.entries.values()]
    this.entries.clear()
    await Promise.all(previous.map(entry => this.close(entry.launch.dashboardSessionId)))
  }
}
