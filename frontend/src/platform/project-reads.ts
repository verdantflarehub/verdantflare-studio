/** Bound read pressure per fixed project reader. This queue is never used for writes. */
export class ProjectReads {
  private running = 0
  private stopped = false
  private waiting: Array<{ resolve: () => void; reject: (error: Error) => void }> = []
  constructor(private retryable: (error: unknown) => boolean, private limit = 4) {}
  private active() { if (this.stopped) throw new Error('项目已切换。') }
  async run<T>(read: () => Promise<T>): Promise<T> {
    this.active()
    if (this.running >= this.limit) await new Promise<void>((resolve, reject) => this.waiting.push({ resolve, reject }))
    else this.running++
    try {
      for (let attempt = 0; ; attempt++) {
        this.active()
        try { return await read() }
        catch (error) {
          if (attempt >= 2 || !this.retryable(error)) throw error
          await new Promise(resolve => setTimeout(resolve, 250 * (attempt + 1)))
        }
      }
    } finally {
      const next = this.waiting.shift()
      if (next) next.resolve()
      else this.running--
    }
  }
  dispose() {
    this.stopped = true
    for (const pending of this.waiting.splice(0)) pending.reject(new Error('项目已切换。'))
  }
}
