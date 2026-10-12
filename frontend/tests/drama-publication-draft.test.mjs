import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import { after, test } from 'node:test'
const frontend = resolve(dirname(fileURLToPath(import.meta.url)), '..'), compiled = mkdtempSync(join(tmpdir(), 'vf-page-publication-'))
after(() => rmSync(compiled, { recursive: true, force: true }))
execFileSync(process.execPath, [join(frontend, 'node_modules/typescript/bin/tsc'), join(frontend, 'src/platform/drama-publication-draft.ts'), '--outDir', compiled, '--target', 'es2022', '--module', 'commonjs', '--strict', '--skipLibCheck'], { stdio: 'pipe' })
const require = createRequire(import.meta.url), { createPagePublication, publishFromPage } = require(join(compiled, 'drama-publication-draft.js')), { parseProjectContent } = require(join(compiled, 'project-content.js'))
const design = process.env.VERDANTFLARE_DESIGN_ROOT || resolve(frontend, '../..')
const id = n => `01900000-0000-7000-8000-${String(n).padStart(12, '0')}`
const ref = n => ({ store_id: id(100), artifact_id: id(n + 20), version_id: id(n + 21) })
function service(fault) {
  const root = JSON.parse(readFileSync(join(design, 'docs/design/contracts/drafts/examples/ai-short-drama-versioned.json')))
  root.ai_short_drama.world_candidates = []
  const project = { project_id: id(1), revision_id: id(2), manifest: { name: 'Synthetic test', category: 'ai_short_drama', files: Array.from({ length: 21 }, (_, i) => ({ file_id: id(i + 10), path: `files/${i}.json`, role: 'domain', content_ref: ref(i + 10) })), domain_documents: [{ document_type: 'project-content', file_id: id(29) }] } }
  let head = { project, raw: JSON.stringify(root) }, serial = 1000, lost = false, published = 0, saved
  const snapshots = new Map([[id(2), structuredClone(head)]]), commits = new Map(), worlds = new Map(), requests = []
  const owner = parseProjectContent(head.raw, project).ai_short_drama.globals[0]
  const version = owner.versions.find(v => v.key === 'l1')
  const draft = createPagePublication(head, owner, version, [version.file_refs[0].file_id], 'Page test L1', 'synthetic actor', '2026-10-11T00:00:00Z', () => id(++serial))
  const initial = structuredClone(head)
  const io = {
    newID: () => id(++serial), read: async revision => structuredClone(revision ? snapshots.get(revision) : head),
    save: () => {}, saveDraft: value => { saved = structuredClone(value) }, phase: () => {},
    call: async (name, args) => {
      requests.push({ name, args: structuredClone(args) })
      if (name === 'project.commit_status') return commits.get(args.commit_id)?.status || { state: 'not_found' }
      if (name === 'project.commit') {
        assert(saved, 'intent persisted before writes')
        if (fault === 'permission') throw Error('permission denied')
        const prior = commits.get(args.commit_id)
        if (prior) { assert.deepEqual(args, prior.args); if (prior.status.state === 'conflict') throw Error('conflict'); return structuredClone(prior.status.result) }
        const updates = args.changes.upsert_files
        const stage = updates.length === 2 ? 'records' : JSON.parse(updates[0].text).ai_short_drama.world_candidates.at(-1)?.registration_status ? 'association' : JSON.parse(updates[0].text).ai_short_drama.world_candidates.at(-1)?.registration_request ? 'request' : 'candidate'
        if (fault === 'conflict-' + stage && !lost) {
          lost = true; head = structuredClone(head); const r = JSON.parse(head.raw); r.overview = 'Concurrent editor'; head.raw = JSON.stringify(r); head.project.revision_id = id(++serial); snapshots.set(head.project.revision_id, structuredClone(head)); commits.set(args.commit_id, { args: structuredClone(args), status: { state: 'conflict' } }); throw Error('conflict')
        }
        assert.equal(args.expected_revision_id, head.project.revision_id)
        head = structuredClone(head)
        for (const f of updates) {
          if (f.file_id === id(29)) head.raw = f.text
          else head.project.manifest.files.push({ file_id: id(++serial), path: f.path, role: f.role, content_ref: ref(++serial) })
        }
        head.project.revision_id = id(++serial); snapshots.set(head.project.revision_id, structuredClone(head)); commits.set(args.commit_id, { args: structuredClone(args), status: { state: 'committed', result: structuredClone(head.project) } })
        if (fault === 'lose-' + stage && !lost) { lost = true; throw Error('response lost') }
        return structuredClone(head.project)
      }
      if (name === 'world.register') {
        if (!worlds.has(args.commit_id)) {
          published++; const source = snapshots.get(args.source_revision_id)
          worlds.set(args.commit_id, { asset_id: id(50), asset_version_id: id(51), manifest_ref: ref(60), manifest: { schema_version: 1, kind: 'asset-version', asset_id: id(50), name: args.name, asset_type: args.asset_type, subjects: args.subjects, depends_on: [], source: { project_id: id(1), project_revision_id: args.source_revision_id, relation: 'produced_in', review_file_id: args.review_file_id }, files: args.file_ids.map(id => source.project.manifest.files.find(f => f.file_id === id)) } })
        }
        if (fault === 'lose-world' && !lost) { lost = true; throw Error('response lost') }
        return structuredClone(worlds.get(args.commit_id))
      }
      if (name === 'world.commit_status') return { state: 'committed', request_sha256: 'a'.repeat(64), result: structuredClone(worlds.get(args.commit_id)) }
      throw Error(name)
    }
  }
  return { draft, io, requests, initial, head: () => head, published: () => published, saved: () => saved }
}
test('page confirmation preserves selected L2 while publishing chosen L1 and mandatory lighting record only', async () => {
  const s = service(); await publishFromPage(s.draft, s.io)
  const after = JSON.parse(s.head().raw), before = JSON.parse(s.initial.raw)
  assert.deepEqual(after.ai_short_drama.globals, before.ai_short_drama.globals)
  assert.equal(s.published(), 1)
  const r = s.requests.find(r => r.name === 'world.register').args
  assert(r.file_ids.includes(id(12))); assert(r.file_ids.includes(id(13))); assert(!r.file_ids.includes(id(14)))
  assert.equal(after.ai_short_drama.world_candidates[0].registration_status.state, 'committed')
})
for (const phase of ['records', 'candidate', 'request', 'association', 'world']) test('page recovery after lost ' + phase + ' response preserves exact request and avoids duplicate publication', async () => {
  const s = service('lose-' + phase); await assert.rejects(publishFromPage(s.draft, s.io), /response lost/)
  await publishFromPage(s.saved(), s.io)
  assert.equal(s.published(), 1)
  assert.equal(JSON.parse(s.head().raw).ai_short_drama.world_candidates.length, 1)
})
for (const phase of ['records', 'candidate', 'request', 'association']) test('page recovery merges known ' + phase + ' conflict without losing another edit', async () => {
  const s = service('conflict-' + phase)
  try { await publishFromPage(s.draft, s.io) } catch { await publishFromPage(s.saved(), s.io) }
  assert.equal(s.published(), 1); assert.equal(JSON.parse(s.head().raw).overview, 'Concurrent editor')
})
test('read-only project cannot publish by bypassing preparation', async () => {
  const s = service('permission'); await assert.rejects(publishFromPage(s.draft, s.io), /permission/); assert.equal(s.published(), 0)
})
test('changed fixed version stops a saved draft before any write', async () => {
  const s = service(); s.draft.version.file_refs[0].content_ref = ref(999)
  await assert.rejects(publishFromPage(s.draft, s.io), /版本已变化/); assert.equal(s.requests.length, 0)
})
