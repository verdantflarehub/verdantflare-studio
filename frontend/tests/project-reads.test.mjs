import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdtempSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import { after, test } from 'node:test'
const frontend = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const compiled = mkdtempSync(join(tmpdir(), 'vf-project-reads-test-'))
after(() => rmSync(compiled, { recursive: true, force: true }))
execFileSync(process.execPath, [join(frontend, 'node_modules/typescript/bin/tsc'), join(frontend, 'src/platform/project-reads.ts'), '--outDir', compiled, '--target', 'es2022', '--module', 'commonjs', '--strict'], { stdio: 'pipe' })
const { ProjectReads } = createRequire(import.meta.url)(join(compiled, 'project-reads.js'))
const tick = () => new Promise(resolve => setImmediate(resolve))
test('many material reads remain bounded, drain in order, and return every result', async () => {
  const queue = new ProjectReads(() => false), releases = [], started = []
  let concurrent = 0, peak = 0
  const all = Array.from({ length: 21 }, (_, n) => queue.run(async () => {
    started.push(n); peak = Math.max(peak, ++concurrent)
    await new Promise(resolve => releases.push(resolve)); concurrent--; return n
  }))
  assert.equal(started.length, 4)
  while (started.length < all.length || concurrent) { releases.splice(0).forEach(resolve => resolve()); await tick() }
  assert.deepEqual(await Promise.all(all), Array.from({ length: 21 }, (_, n) => n))
  assert.deepEqual(started, Array.from({ length: 21 }, (_, n) => n)); assert.equal(peak, 4)
})
test('transient reads retry within a bound while denied or invalid content fails immediately', async () => {
  const queue = new ProjectReads(error => error.status === 502)
  let attempts = 0
  assert.equal(await queue.run(async () => { if (++attempts < 3) throw { status: 502 }; return 'fixed bytes' }), 'fixed bytes')
  assert.equal(attempts, 3)
  for (const status of [403, 401, 400]) {
    attempts = 0; await assert.rejects(queue.run(async () => { attempts++; throw { status } }), e => e.status === status); assert.equal(attempts, 1)
  }
  attempts = 0; await assert.rejects(queue.run(async () => { attempts++; throw { status: 502 } }), e => e.status === 502); assert.equal(attempts, 3)
})
test('switching project rejects queued work before it can read another artifact', async () => {
  const queue = new ProjectReads(() => false, 1)
  let release, calls = 0
  const first = queue.run(() => new Promise(resolve => { release = resolve }))
  const waiting = queue.run(async () => ++calls)
  const rejected = assert.rejects(waiting, /项目已切换/)
  queue.dispose(); await rejected; release('old request'); await first
  assert.equal(calls, 0); await assert.rejects(queue.run(async () => ++calls), /项目已切换/); assert.equal(calls, 0)
})
