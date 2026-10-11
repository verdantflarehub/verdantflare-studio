import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdtempSync, readFileSync, rmSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { dirname, join, resolve } from 'node:path'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'
import { after, test } from 'node:test'

const frontend = resolve(dirname(fileURLToPath(import.meta.url)), '..')
const compiled = mkdtempSync(join(tmpdir(), 'vf-project-content-test-'))
after(() => rmSync(compiled, { recursive: true, force: true }))
execFileSync(process.execPath, [join(frontend, 'node_modules/typescript/bin/tsc'),
  join(frontend, 'src/platform/project-content.ts'), join(frontend, 'src/platform/drama-presentation.ts'), '--outDir', compiled,
  '--target', 'es2022', '--module', 'commonjs', '--strict', '--skipLibCheck'], { cwd: frontend, stdio: 'pipe' })
const { materialPreview, dramaText } = createRequire(import.meta.url)(join(compiled, 'drama-presentation.js'))
const { parseProjectContent } = createRequire(import.meta.url)(join(compiled, 'project-content.js'))
// Design remains the contract source; do not duplicate its fixtures in the application.
const design = process.env.VERDANTFLARE_DESIGN_ROOT || resolve(frontend, '../..')
const load = name => JSON.parse(readFileSync(join(design, 'docs/design/contracts/drafts/examples', name), 'utf8'))
const sample = () => load('ai-short-drama-globals.json')
const id = n => `01900000-0000-7000-8000-${String(n).padStart(12, '0')}`
const ref = n => ({ store_id: id(100), artifact_id: id(n), version_id: id(n + 1) })
const file = n => ({ file_id: id(n), path: `files/${n}.md`, content_ref: ref(n + 20) })
const project = () => ({ project_id: id(1), revision_id: id(2), manifest: {
  name: 'Test', category: 'ai_short_drama', files: [file(10), file(11), file(12)],
  domain_documents: [], asset_refs: [], run_refs: []
} })
const parse = (value, p = project()) => parseProjectContent(JSON.stringify(value), p)
const bad = mutate => { const value = sample(); mutate(value.ai_short_drama); assert.throws(() => parse(value)) }

function registered() {
  const value = sample(), p = project(), candidate = value.ai_short_drama.world_candidates[0]
  candidate.review = { state: 'approved', record_file_id: id(10) }
  candidate.registration_request = {
    commit_id: id(3), source_project_id: p.project_id, source_revision_id: id(4), file_ids: [id(30)],
    name: 'Ring package', asset_type: 'prop', subjects: ['Ring'], relation: 'produced_in', review_file_id: id(10)
  }
  const result = {
    asset_id: id(5), asset_version_id: id(6), manifest_ref: ref(40),
    manifest: { schema_version: 1, kind: 'asset-version', asset_id: id(5), name: 'Ring package', asset_type: 'prop', subjects: ['Ring'],
      files: [{ ...file(30), role: 'reference' }], depends_on: [],
      source: { project_id: p.project_id, project_revision_id: id(4), relation: 'produced_in', review_file_id: id(10) } }
  }
  candidate.registration_status = { state: 'committed', request_sha256: 'a'.repeat(64), result }
  return { value, p, candidate }
}

test('reads all seven kinds and explicit cross-episode references without data loss', () => {
  const parsed = parse(sample())
  assert.equal(parsed.ai_short_drama.globals.length, 7)
  assert.equal(parsed.ai_short_drama.usages.length, 4)
  assert.equal(parsed.ai_short_drama.episodes[0].script_key, 'script')
  assert.deepEqual(parsed.ai_short_drama.continuity[0].global_keys, ['ring'])
  assert.equal(parsed.ai_short_drama.world_candidates[0].registration_status, null)
  assert.deepEqual(parse(parsed), parsed)
})

test('can read materials before any episodes exist', () => {
  const value = sample()
  value.ai_short_drama = { globals: value.ai_short_drama.globals, episodes: [] }
  assert.equal(parse(value).ai_short_drama.globals.length, 7)
})

test('legacy first episode and three existing categories remain readable', () => {
  const drama = parse(load('ai-short-drama-project-content.json')).ai_short_drama
  assert.equal(drama.episodes[0].scenes[0].shots.length, 10)
  assert.deepEqual(drama.globals, [])
  assert.equal(drama.continuity[0].key, 'bronze_ring')
  for (const category of ['character', 'music', 'video']) {
    const p = project(); p.manifest.category = category
    assert.ok(parse({ schema_version: 1, kind: 'project-content', category, [category]: {} }, p)[category])
  }
})

test('rejects duplicate keys within each new collection and missing material references', () => {
  for (const collection of ['globals', 'usages', 'deliverables', 'world_candidates']) {
    bad(d => d[collection].push(structuredClone(d[collection][0])))
    bad(d => { d[collection] = null })
  }
  bad(d => { d.globals = d.globals.filter(g => g.key !== 'ring') })
  bad(d => { d.usages[0].global_key = 'missing' })
  bad(d => { d.continuity[0].global_keys = ['missing'] })
})

test('validates the complete usage path, with parent-scoped keys', () => {
  // The sample deliberately reuses scene01/shot01 in different episodes.
  assert.doesNotThrow(() => parse(sample()))
  for (const target of [ {}, { episode_key: 'missing' }, { episode_key: 'ep01', shot_key: 'shot01' },
    { episode_key: 'ep01', scene_key: 'missing' }, { episode_key: 'ep01', scene_key: 'scene01', shot_key: 'missing' } ]) {
    bad(d => { d.usages[0].target = target })
  }
  const value = sample()
  value.ai_short_drama.usages = Array.from({ length: 1001 }, (_, i) => ({ key: `u${i}`, global_key: 'ring', target: { episode_key: 'ep01' } }))
  assert.equal(parse(value).ai_short_drama.usages.length, 1001)
})

test('checks script kinds, fixed files and delivery episode references', () => {
  bad(d => { d.episodes[0].script_key = 'ring' })
  bad(d => { d.episodes[0].script_key = 'missing' })
  bad(d => { d.globals[0].file_ids = [id(99)] })
  bad(d => { d.globals[0].reference_file_ids = [id(99)] })
  bad(d => { d.deliverables[0].episode_key = 'missing' })
  bad(d => { d.deliverables[0].file_ids = [id(99)] })
  const value = sample()
  value.ai_short_drama.globals[0].file_ids = [id(11)]
  value.ai_short_drama.episodes[0].script_file_id = id(11)
  assert.doesNotThrow(() => parse(value))
  value.ai_short_drama.episodes[0].script_file_id = id(12)
  assert.throws(() => parse(value))
})

test('approved materials and deliveries require readable evidence and selected content', () => {
  for (const collection of ['globals', 'deliverables']) {
    const value = sample(), item = value.ai_short_drama[collection][0]
    item.review = { state: 'approved', record_file_id: id(10) }
    assert.throws(() => parse(value))
    item.file_ids = [id(11)]
    assert.doesNotThrow(() => parse(value))
    item.review.record_file_id = id(99)
    assert.throws(() => parse(value))
    item.review = {}
    assert.throws(() => parse(value))
  }
})

test('World-backed material requires the exact declared asset/version pair', () => {
  const value = sample(), p = project(), g = value.ai_short_drama.globals[0]
  g.asset_id = id(50); g.asset_version_id = id(51)
  g.review = { state: 'approved', record_file_id: id(10) }
  assert.throws(() => parse(value, p))
  p.manifest.asset_refs.push({ asset_id: id(50), asset_version_id: id(51), purpose: 'script' })
  assert.doesNotThrow(() => parse(value, p))
  g.asset_version_id = id(52)
  assert.throws(() => parse(value, p))
})

test('candidate source must exist and registration needs approval', () => {
  bad(d => { d.world_candidates[0].source.key = 'missing' })
  bad(d => { d.world_candidates[0].source = { kind: 'deliverable', key: 'missing' } })
  const { value, candidate } = registered()
  candidate.review.state = 'in_review'
  assert.throws(() => parse(value))
  candidate.review.state = 'approved'
  candidate.registration_request.source_project_id = id(99)
  assert.throws(() => parse(value))
})

test('historical publication is retained without misreading source files as current files', () => {
  const { value, p, candidate } = registered()
  assert.equal(p.manifest.files.some(f => f.file_id === id(30)), false)
  const parsed = parse(value, p)
  assert.deepEqual(parsed.ai_short_drama.world_candidates[0].registration_status, candidate.registration_status)
  assert.deepEqual(parse(parsed, p), parsed)
  candidate.registration_request.source_revision_id = p.revision_id
  assert.throws(() => parse(value, p))
})

test('rejects committed snapshots with mismatched source, selected files or asset identity', () => {
  for (const mutate of [
    c => { c.registration_status.result.manifest.source.project_revision_id = id(99) },
    c => { c.registration_status.result.manifest.files[0].file_id = id(99) },
    c => { c.registration_status.result.manifest.asset_id = id(99) },
    c => { c.registration_status.result.manifest.name = 'Another package' },
    c => { c.registration_request.review_file_id = id(11) },
    c => { delete c.registration_status.result },
    c => { c.registration_request = null }
  ]) {
    const { value, candidate } = registered(); mutate(candidate)
    assert.throws(() => parse(value))
  }
})

test('pending, failed and conflicting registrations retain distinct recovery states', () => {
  const { value, candidate } = registered()
  for (const status of [
    { state: 'preparing', request_sha256: 'a'.repeat(64) },
    { state: 'failed', request_sha256: 'a'.repeat(64), error_code: 'CONTENT_NOT_READY' }
  ]) {
    candidate.registration_status = status
    assert.deepEqual(parse(value).ai_short_drama.world_candidates[0].registration_status, status)
  }
  candidate.registration_status.result = {}
  assert.throws(() => parse(value))
  candidate.registration_request.asset_id = id(5)
  candidate.registration_request.expected_asset_version_id = id(6)
  candidate.registration_status = { state: 'conflict', request_sha256: 'a'.repeat(64), error_code: 'REVISION_CONFLICT', current_asset_version_id: id(7) }
  assert.doesNotThrow(() => parse(value))
})

test('existing shot file, run, candidate and approval checks remain effective', () => {
  bad(d => { d.episodes[0].scenes[0].shots[0].continuity_keys = ['missing'] })
  bad(d => { d.episodes[0].scenes[0].shots[0].run_ref = { service_id: 'video', run_id: 'missing' } })
  bad(d => { d.episodes[0].scenes[0].shots[0].selected_result_file_id = id(11) })
  bad(d => { d.episodes[0].scenes[0].shots[0].review = { state: 'approved', record_file_id: id(10) } })
})

test('reads fixed script, character, Look, location, prop and continuity document locators', () => {
  const value = load('ai-short-drama-production-basis.json'), p = project()
  p.manifest.files = [10, 11, 12, 13, 14, 15].map(file)
  const parsed = parse(value, p), d = parsed.ai_short_drama
  assert.deepEqual(d.globals.find(g => g.key === 'W_XE_05').input_global_keys, ['XE'])
  assert.equal(d.episodes[0].scenes[0].definition_refs[0].anchor, 'LOC05 / S13')
  assert.ok(d.episodes[0].scenes[0].shots[0].definition_refs.some(r => r.file_id === id(15) && r.anchor === 'SH026'))
  assert.deepEqual(parse(parsed, p), parsed)
  value.ai_short_drama.globals.find(g => g.key === 'W_XE_05').input_global_keys = ['missing']
  assert.throws(() => parse(value, p))
})

test('definition approval does not approve a generated image and invalid basis references are rejected', () => {
  const value = sample(), item = value.ai_short_drama.globals[0]
  item.definition_refs = [{ file_id: id(11), anchor: 'Character' }]
  item.definition_review = { state: 'approved', record_file_id: id(10) }
  const parsed = parse(value).ai_short_drama.globals[0]
  assert.equal(parsed.definition_review.state, 'approved')
  assert.equal(parsed.review.state, 'unreviewed')
  item.review = { state: 'approved', record_file_id: id(10) }
  assert.throws(() => parse(value))
  delete item.review
  item.definition_refs = []
  assert.throws(() => parse(value))
  bad(d => { d.globals[0].definition_refs = [{ file_id: id(99), anchor: 'Character' }] })
  bad(d => { d.globals[0].input_global_keys = ['script'] })
  bad(d => { d.globals[0].generation_record_file_ids = [id(99)] })
  bad(d => { d.episodes[0].scenes[0].shots[0].definition_refs = [{ file_id: id(99) }] })
})

function versioned() {
  const value = load('ai-short-drama-versioned.json'), p = project()
  p.manifest.files = Array.from({ length: 21 }, (_, i) => file(i + 10))
  return { value, p }
}
test('retains scene states, lighting, whitebox, shot and delivery versions across read round-trips', () => {
  const { value, p } = versioned(), parsed = parse(value, p), d = parsed.ai_short_drama
  assert.equal(d.globals[0].selected_versions[0].version_key, 'l2')
  assert.equal(d.usages[0].version_key, 'l1')
  assert.equal(d.usages[0].segment.label, '雨夜闪回，灯灭')
  assert.equal(d.episodes[0].scenes[0].shots[0].versions[0].spatial_record_file_id, id(17))
  assert.equal(d.deliverables[0].versions[0].composition_file_id, id(23))
  assert.equal(d.world_candidates[0].publication_record_file_id, id(30))
  assert.deepEqual(parse(parsed, p), parsed)
})
test('rejects invalid version/state/segment bindings and rewritten immutable file content', () => {
  for (const mutate of [
    d => { d.globals[0].versions.push({ ...structuredClone(d.globals[0].versions[0]), key: 'duplicate', label: 'Another name' }) },
    d => { d.globals[0].versions[0].state_key = 'missing' },
    d => { d.globals[0].versions[0].parent_version_key = 'l2' },
    d => { d.globals[0].versions[0].file_refs[0].content_ref.version_id = id(99) },
    d => { d.globals[0].selected_versions[0].state_key = 'day' },
    d => { d.globals[0].selected_versions.push(d.globals[0].selected_versions[0]) },
    d => { d.usages[0].version_key = 'foreign' },
    d => { d.usages[0].segment.order = 9 },
    d => { d.episodes[0].scenes[0].shots[0].versions[0].spatial_record_file_id = null },
    d => { d.episodes[0].scenes[0].shots[0].versions[1].segment_key = 'flashback' },
    d => { d.episodes[0].scenes[0].shots[0].result_file_ids.push(id(16)) },
    d => { d.deliverables[0].file_ids = [id(19)] },
    d => { d.deliverables[0].versions[0].composition_file_id = id(99) }
  ]) {
    const { value, p } = versioned(); mutate(value.ai_short_drama)
    assert.throws(() => parse(value, p))
  }
})
test('current source publication uses an explicit package record instead of current selected outputs', () => {
  const { value, p } = versioned(), c = value.ai_short_drama.world_candidates[0]
  c.review = { state: 'approved', record_file_id: id(10) }
  c.registration_request = { commit_id: id(3), source_project_id: p.project_id, source_revision_id: p.revision_id, file_ids: [id(30), id(12), id(13)], name: c.name, asset_type: 'location', subjects: ['Hall'], relation: 'produced_in', review_file_id: id(10) }
  assert.doesNotThrow(() => parse(value, p))
  delete c.publication_record_file_id
  assert.throws(() => parse(value, p))
})


test('material previews include unselected and rejected attempts without changing adoption', () => {
  const candidate = (key, state) => ({ key, label: key, state_key: 'wet', purpose: 'image', review: { state }, file_refs: [{ file_id: key, purpose: 'image' }] })
  const material = { versions: [candidate('first', 'approved'), candidate('retry', 'rejected')], selected_versions: [], file_ids: [] }
  const before = JSON.stringify(material)
  assert.equal(materialPreview(material).fileId, 'retry')
  assert.equal(materialPreview(material).version.state_key, 'wet')
  assert.equal(materialPreview(material).version.key, 'retry')
  assert.equal(materialPreview(material).selected, false)
  assert.equal(JSON.stringify(material), before)
  material.selected_versions.push({ version_key: 'first', state_key: 'wet', purpose: 'image' })
  assert.equal(materialPreview(material).fileId, 'first')
  assert.equal(materialPreview(material).selected, true)
  material.versions.push({ key: 'engineering', file_refs: [{ file_id: 'blend', purpose: 'whitebox' }] })
  material.selected_versions = []
  assert.equal(materialPreview(material).fileId, 'retry')
  assert.equal(materialPreview({ versions: [], selected_versions: [], file_ids: [] }).fileId, undefined)
})

test('readable display resolves known codes without changing unknown identifiers or source text', () => {
  const text = '**动作：** `P01` 交给 CH_A；P010 保留。阅读[场景](../scene.md)。'
  const drama = { globals: [{ key: 'P01', name: '铜戒' }, { key: 'CH_A', name: '谢小娥' }], episodes: [] }
  assert.equal(dramaText(text, drama), '**动作：** 铜戒 交给 谢小娥；P010 保留。阅读场景。')
  assert(text.includes('`P01`'))
  assert.equal(dramaText('<script>unsafe</script>', drama), '<script>unsafe</script>') // Vue renders this as escaped text, never HTML.
})
