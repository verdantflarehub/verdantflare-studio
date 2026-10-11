import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import { after, test } from 'node:test'
const frontend = resolve(dirname(fileURLToPath(import.meta.url)), '..'), compiled = mkdtempSync(join(tmpdir(), 'vf-publication-test-'))
after(() => rmSync(compiled, { recursive: true, force: true }))
execFileSync(process.execPath, [join(frontend, 'node_modules/typescript/bin/tsc'), join(frontend, 'src/platform/drama-publication.ts'), '--outDir', compiled, '--target', 'es2022', '--module', 'commonjs', '--strict', '--skipLibCheck'], { stdio: 'pipe' })
const { publicationPackage, publishPrepared, canonical } = createRequire(import.meta.url)(join(compiled, 'drama-publication.js'))
const { parseProjectContent } = createRequire(import.meta.url)(join(compiled, 'project-content.js'))
const design = process.env.VERDANTFLARE_DESIGN_ROOT || resolve(frontend, '../..')
const load = name => JSON.parse(readFileSync(join(design, 'docs/design/contracts/drafts/examples', name), 'utf8'))
const id = n => `01900000-0000-7000-8000-${String(n).padStart(12, '0')}`
const ref = n => ({ store_id: id(100), artifact_id: id(n + 20), version_id: id(n + 21) })
const file = n => ({ file_id: id(n), path: `files/${n}.json`, role: 'domain', content_ref: ref(n) })
const snapshot = () => ({ project: { project_id: id(1), revision_id: id(2), manifest: { name: 'Test', category: 'ai_short_drama', files: Array.from({ length: 21 }, (_, i) => file(i + 10)), domain_documents: [{ document_type: 'project-content', file_id: id(29) }] } }, raw: JSON.stringify(load('ai-short-drama-versioned.json')) })

test('publishes L1 while L2 is selected and rejects extra or mislabeled files', () => {
  const s = snapshot(), d = parseProjectContent(s.raw, s.project).ai_short_drama, p = load('ai-short-drama-publication.json')
  const result = publicationPackage(JSON.stringify(p), d.world_candidates[0], d.globals[0], s.project)
  assert.deepEqual(result.files, [id(12), id(13), id(30)])
  assert.equal(result.selections[0].version_key, 'l1')
  for (const mutate of [
    p => { p.versions[0].version_key = 'l2' },
    p => { p.versions[0].file_refs[0].content_ref.version_id = id(999) },
    p => { p.supporting_file_refs = [] },
    p => { p.supporting_file_refs.push({ file_id: id(20), content_ref: ref(20), purpose: 'private' }) },
    p => { p.source.key = 'other' },
    p => { p.versions.push(p.versions[0]) }
  ]) { const value = structuredClone(p); mutate(value); assert.throws(() => publicationPackage(JSON.stringify(value), d.world_candidates[0], d.globals[0], s.project)) }
})

function service() {
  const original = snapshot(), requests = [], snapshots = new Map([[id(2), original]]), commits = new Map(), worlds = new Map()
  let head = id(2), serial = 40, loseWorld = false, loseAssociation = false, conflictAssociation = false, published = 0
  const intent = { project_id: id(1), origin_revision_id: id(2), candidate_key: 'hall-package', prepare_commit_id: id(3), association_commit_id: id(4), request: { commit_id: id(5), source_project_id: id(1), source_revision_id: id(2), file_ids: [id(12), id(13), id(30)], name: 'Hall', asset_type: 'location', subjects: ['Hall'], relation: 'produced_in', review_file_id: id(10) } }
  const io = {
    read: async revision => structuredClone(snapshots.get(revision || head)), save: () => {}, newID: () => id(++serial), phase: () => {},
    call: async (name, args) => {
      requests.push({ name, args: structuredClone(args) })
      if (name === 'project.commit_status') { const saved = commits.get(args.commit_id); if (!saved) throw Error('not found'); return saved.status }
      if (name === 'project.commit') {
        if (commits.has(args.commit_id)) { const saved = commits.get(args.commit_id); assert.equal(canonical(saved.args), canonical(args), 'retry payload drifted'); if (saved.status.state === 'conflict') throw Error('conflict'); return structuredClone(saved.status.result) }
        if (conflictAssociation && args.commit_id !== intent.prepare_commit_id) {
          conflictAssociation = false
          const other = structuredClone(snapshots.get(head)), root = JSON.parse(other.raw); root.overview = 'Another editor saved this'; other.raw = JSON.stringify(root); head = id(++serial); other.project.revision_id = head; snapshots.set(head, other)
          commits.set(args.commit_id, { args: structuredClone(args), status: { state: 'conflict' } }); throw Error('conflict')
        }
        assert.equal(args.expected_revision_id, head)
        const next = structuredClone(snapshots.get(head)); next.raw = args.changes.upsert_files[0].text; head = id(++serial); next.project.revision_id = head; snapshots.set(head, next)
        commits.set(args.commit_id, { args: structuredClone(args), status: { state: 'committed', result: structuredClone(next.project) } })
        if (loseAssociation && args.commit_id !== intent.prepare_commit_id) { loseAssociation = false; throw Error('lost association response') }
        return structuredClone(next.project)
      }
      if (name === 'world.register') {
        if (!worlds.has(args.commit_id)) { published++; worlds.set(args.commit_id, { asset_id: id(6), asset_version_id: id(7), manifest_ref: ref(40), manifest: { schema_version: 1, kind: 'asset-version', asset_id: id(6), name: 'Hall', asset_type: 'location', subjects: ['Hall'], depends_on: [], source: { project_id: id(1), project_revision_id: id(2), relation: 'produced_in', review_file_id: id(10) }, files: args.file_ids.map(n => original.project.manifest.files.find(f => f.file_id === n)) } }) }
        if (loseWorld) { loseWorld = false; throw Error('lost World response') }
        return structuredClone(worlds.get(args.commit_id))
      }
      if (name === 'world.commit_status') return { state: 'committed', request_sha256: 'a'.repeat(64), result: structuredClone(worlds.get(args.commit_id)) }
      throw Error('unexpected tool ' + name)
    }
  }
  return { intent, io, requests, snapshot: () => snapshots.get(head), published: () => published, loseWorld: () => { loseWorld = true }, loseAssociation: () => { loseAssociation = true }, conflict: () => { conflictAssociation = true } }
}
test('records the request before publishing and saves authoritative result to the same project', async () => {
  const s = service(), result = await publishPrepared(s.intent, s.io)
  assert.equal(s.published(), 1)
  assert.deepEqual(s.requests.map(r => r.name), ['project.commit', 'world.register', 'world.commit_status', 'project.commit'])
  const candidate = JSON.parse(s.snapshot().raw).ai_short_drama.world_candidates[0]
  assert.deepEqual(candidate.registration_request, s.intent.request)
  assert.equal(candidate.registration_status.result.asset_version_id, result.result.asset_version_id)
})
test('lost World response reuses the same registration and never makes a second asset', async () => {
  const s = service(); s.loseWorld()
  await assert.rejects(publishPrepared(s.intent, s.io), /lost World/)
  await publishPrepared(s.intent, s.io)
  assert.equal(s.published(), 1)
  assert.deepEqual(s.requests.filter(r => r.name === 'world.register').map(r => r.args.commit_id), [id(5), id(5)])
})
test('lost project association response queries the original commit instead of rewriting it', async () => {
  const s = service(); s.loseAssociation()
  await assert.rejects(publishPrepared(s.intent, s.io), /lost association/)
  const saved = await publishPrepared(s.intent, s.io)
  assert.equal(saved.revision_id, s.snapshot().project.revision_id)
  assert.equal(s.requests.filter(r => r.name === 'project.commit' && r.args.commit_id === id(4)).length, 1)
  assert.equal(s.published(), 1)
})
test('known CAS conflict merges only the association and preserves another editors text', async () => {
  const s = service(); s.conflict()
  await assert.rejects(publishPrepared(s.intent, s.io), /conflict/)
  await publishPrepared(s.intent, s.io)
  assert.equal(JSON.parse(s.snapshot().raw).overview, 'Another editor saved this')
  assert.equal(s.published(), 1)
  assert.notEqual(s.intent.association_commit_id, id(4))
})
test('changed candidate prevents associating a successful publication with a different operation', async () => {
  const s = service(); s.conflict()
  await assert.rejects(publishPrepared(s.intent, s.io))
  const latest = s.snapshot(), raw = JSON.parse(latest.raw); raw.ai_short_drama.world_candidates[0].registration_request.commit_id = id(99); latest.raw = JSON.stringify(raw)
  await assert.rejects(publishPrepared(s.intent, s.io), /候选已变化/)
  assert.equal(s.published(), 1)
})
