export interface CachedDashboard {
  hide(): void
  show(): void
  prepareClose(): Promise<boolean>
  dispose(): Promise<void>
}

/** Serialized, bounded LRU cache; instance lifecycle is deliberately absent. */
export class DashboardCache<T extends CachedDashboard> {
  private entries = new Map<string, T>()
  private activeKey: string | undefined
  private pending: Promise<unknown> = Promise.resolve()
  private generation = 0
  private creating = new Set<AbortController>()

  constructor(readonly capacity = 3) {
    if (!Number.isInteger(capacity) || capacity < 1 || capacity > 10) throw new Error('INVALID_CACHE_CAPACITY')
  }

  private serialize<R>(operation: () => Promise<R>): Promise<R> {
    const generation = this.generation
    const next = this.pending.then(() => {
      if (generation !== this.generation) throw new Error('CACHE_REVOKED')
      return operation()
    })
    this.pending = next.catch(() => undefined)
    return next
  }

  activate(key: string, create: (signal: AbortSignal) => Promise<T>, prepare?: (entry: T) => Promise<void>): Promise<T> {
    return this.serialize(async () => {
      const generation = this.generation
      let entry = this.entries.get(key)
      const newlyCreated = !entry
      if (!entry) {
        if (this.entries.size >= this.capacity) {
          // Select one oldest inactive entry; never silently discard dirty data.
          const oldest = [...this.entries.keys()].find(candidate => candidate !== this.activeKey)
            ?? this.activeKey!
          const victim = this.entries.get(oldest)!
          if (!await victim.prepareClose()) throw new Error('CACHE_CLOSE_CANCELLED')
          if (generation !== this.generation) throw new Error('CACHE_REVOKED')
          this.entries.delete(oldest)
          if (this.activeKey === oldest) this.activeKey = undefined
          await victim.dispose()
        }
        if (generation !== this.generation) throw new Error('CACHE_REVOKED')
        const controller = new AbortController()
        this.creating.add(controller)
        try { entry = await create(controller.signal) }
        finally { this.creating.delete(controller) }
        if (generation !== this.generation) {
          await entry.dispose()
          throw new Error('CACHE_REVOKED')
        }
        this.entries.set(key, entry)
      }
      const previousKey = this.activeKey
      const previous = this.entries.get(previousKey ?? '')
      let showing = false
      try {
        if (prepare) await prepare(entry)
        if (generation !== this.generation) throw new Error('CACHE_REVOKED')
        if (this.entries.get(key) !== entry) throw new Error('CACHE_REVOKED')
        showing = true
        if (previousKey !== key) previous?.hide()
        entry.show()
      } catch (error) {
        // Context veto on a cached page must preserve its unsaved state. A newly
        // mounted page has no previous state to restore and must release resources.
        if (generation === this.generation && this.entries.get(key) === entry && (showing || newlyCreated)) {
          this.entries.delete(key)
          if (this.activeKey === key) this.activeKey = undefined
          try { await entry.dispose() }
          finally {
            if (showing && generation === this.generation && previousKey !== key
              && previous && this.entries.get(previousKey!) === previous) previous.show()
          }
        }
        throw error
      }
      this.entries.delete(key)
      this.entries.set(key, entry)
      this.activeKey = key
      return entry
    })
  }

  close(key: string): Promise<boolean> {
    return this.serialize(async () => {
      const generation = this.generation
      const entry = this.entries.get(key)
      if (!entry) return true
      if (!await entry.prepareClose()) return false
      if (generation !== this.generation) return false
      this.entries.delete(key)
      if (this.activeKey === key) this.activeKey = undefined
      await entry.dispose()
      return true
    })
  }

  hideActive(): Promise<void> {
    return this.serialize(async () => { this.entries.get(this.activeKey ?? '')?.hide() })
  }

  async invalidate(key: string): Promise<void> {
    const entry = this.entries.get(key)
    this.entries.delete(key)
    if (this.activeKey === key) this.activeKey = undefined
    await entry?.dispose()
  }

  prepareLeave(): Promise<boolean> {
    return this.serialize(async () => {
      const generation = this.generation
      for (const entry of this.entries.values()) {
        if (!await entry.prepareClose() || generation !== this.generation) return false
      }
      return true
    })
  }

  // Identity loss must not wait for a dirty form, async factory or queued activation.
  async revoke(): Promise<void> {
    this.generation++
    for (const controller of this.creating) controller.abort()
    this.creating.clear()
    const entries = [...this.entries.values()]
    this.entries.clear()
    this.activeKey = undefined
    const results = await Promise.allSettled(entries.map(entry => entry.dispose()))
    const failures = results.filter(result => result.status === 'rejected')
    if (failures.length) throw new AggregateError(failures.map(result => result.reason), 'CACHE_DISPOSE_FAILED')
  }

  get size() { return this.entries.size }
}
