import type { ContentRef, DramaGlobal, DramaDeliverable, DramaWorldCandidate, ProjectOpen, WorldRegistration, WorldRegistrationStatus, WorldResult } from './project-content'

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
const fail = (message = '入库说明与固定版本不一致，请经 MCP 核对。'): never => { throw new Error(message) }
export const canonical = (value: unknown): string => JSON.stringify(value, (_key, v) => v && typeof v === 'object' && !Array.isArray(v) ? Object.fromEntries(Object.entries(v).sort(([a], [b]) => a.localeCompare(b))) : v)
const obj = (value: unknown, keys: string[]) => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return fail()
  const o = value as Record<string, unknown>
  if (Object.keys(o).some(k => !keys.includes(k))) fail()
  return o
}
export interface PublicationSelection { version_key: string; file_ids: string[] }
export interface PublicationPackage { files: string[]; selections: PublicationSelection[] }

/** Validates only the package scope. Server authority and World outcomes remain authoritative. */
export function publicationPackage(raw: string, candidate: DramaWorldCandidate, owner: DramaGlobal | DramaDeliverable, project: ProjectOpen): PublicationPackage {
  let value: unknown
  try { value = JSON.parse(raw) } catch { return fail() }
  const r = obj(value, ['schema_version', 'kind', 'source', 'versions', 'supporting_file_refs'])
  if (r.schema_version !== 1 || r.kind !== 'short-drama-publication' || !Array.isArray(r.versions) || !r.versions.length || !Array.isArray(r.supporting_file_refs)) fail()
  if (canonical(r.source) !== canonical(candidate.source) || !candidate.publication_record_file_id) fail()
  const files = new Set<string>([candidate.publication_record_file_id!]), seen = new Set<string>(), support = new Set<string>(), required = new Set<string>(), selections: PublicationSelection[] = []
  const readFile = (raw: unknown) => {
    const file = obj(raw, ['file_id', 'content_ref', 'purpose'])
    if (typeof file.file_id !== 'string' || typeof file.purpose !== 'string' || !/^[a-zA-Z0-9_-]+$/.test(file.purpose)) return fail()
    const actual = project.manifest.files.find(f => f.file_id === file.file_id)
    if (!actual || files.has(file.file_id) || canonical(actual.content_ref) !== canonical(file.content_ref)) return fail()
    files.add(file.file_id)
    return { file_id: file.file_id, purpose: file.purpose, content_ref: file.content_ref as ContentRef }
  }
  for (const entry of r.versions as unknown[]) {
    const selection = obj(entry, ['version_key', 'file_refs'])
    const version = owner.versions.find(v => v.key === selection.version_key)
    if (!version || seen.has(version.key) || !Array.isArray(selection.file_refs) || !selection.file_refs.length) fail()
    seen.add(version!.key)
    const ids = (selection.file_refs as unknown[]).map(raw => {
      const file = readFile(raw)
      if (!version!.file_refs.some(f => canonical(f) === canonical(file))) fail()
      if (file.purpose === 'whitebox') required.add(version!.spatial_record_file_id || '')
      if (file.purpose === 'lighting') required.add(version!.lighting_record_file_id || '')
      return file.file_id
    })
    selections.push({ version_key: version!.key, file_ids: ids })
    for (const id of [version!.change_record_file_id, version!.spatial_record_file_id, version!.lighting_record_file_id, version!.composition_file_id, version!.review.record_file_id, ...version!.generation_record_file_ids]) if (id) support.add(id)
    if ('states' in owner) for (const ref of owner.states.find(s => s.key === version!.state_key)?.definition_refs || []) support.add(ref.file_id)
  }
  if ('definition_refs' in owner) for (const ref of owner.definition_refs) support.add(ref.file_id)
  for (const entry of r.supporting_file_refs as unknown[]) if (!support.has(readFile(entry).file_id)) fail()
  if ([...required].some(id => !id || !files.has(id)) || !project.manifest.files.some(f => f.file_id === candidate.publication_record_file_id)) fail()
  return { files: [...files].sort(), selections }
}

export interface PublicationSnapshot { project: ProjectOpen; raw: string }
export interface PublicationIntent {
  project_id: string; origin_revision_id: string; candidate_key: string
  request: WorldRegistration; prepare_commit_id: string; association_commit_id: string
  association_base_revision_id?: string; published_asset_id?: string; published_version_id?: string
}
export interface PublicationIO {
  call<T>(name: string, args: Record<string, unknown>): Promise<T>
  read(revision?: string): Promise<PublicationSnapshot>
  save(intent: PublicationIntent): void
  newID(): string
  phase(message: string): void
}
function sourceCandidate(snapshot: PublicationSnapshot, key: string) {
  const root = JSON.parse(snapshot.raw)
  const candidate = root.ai_short_drama?.world_candidates?.find((c: { key: string }) => c.key === key)
  if (!candidate) fail('项目中的入库候选已经变化，请经 MCP 核对后恢复关联。')
  return { root, candidate }
}
function commitArgs(snapshot: PublicationSnapshot, text: string, commitID: string) {
  const domain = snapshot.project.manifest.domain_documents.find(d => d.document_type === 'project-content')
  const file = snapshot.project.manifest.files.find(f => f.file_id === domain?.file_id)
  if (!file || typeof file.role !== 'string' || !file.role) return fail('项目内容文件信息不完整，无法保存入库对应关系。')
  return { project_id: snapshot.project.project_id, expected_revision_id: snapshot.project.revision_id, commit_id: commitID, changes: { upsert_files: [{ file_id: file.file_id, path: file.path, role: file.role, text, mime: 'application/json' }] } }
}
function checkResult(result: ProjectOpen, projectID: string) { if (result.project_id !== projectID || !uuid.test(result.revision_id)) fail('项目保存结果不完整，请按原操作恢复。') }

/** All retries reuse the original World request. Only a known CAS conflict may
 * create a new association commit, after re-reading the head and matching the candidate. */
export async function publishPrepared(intent: PublicationIntent, io: PublicationIO): Promise<{ revision_id: string; result: WorldResult }> {
  if (![intent.project_id, intent.origin_revision_id, intent.prepare_commit_id, intent.association_commit_id, intent.request.commit_id].every(id => uuid.test(id)) || intent.request.source_project_id !== intent.project_id) fail('入库恢复信息无效。')
  const original = await io.read(intent.origin_revision_id)
  if (original.project.project_id !== intent.project_id || original.project.revision_id !== intent.origin_revision_id) fail()
  const { root, candidate } = sourceCandidate(original, intent.candidate_key)
  if (candidate.registration_request) {
    if (canonical(candidate.registration_request) !== canonical(intent.request)) fail('已有入库请求与恢复操作不一致。')
  } else {
    io.phase('正在保存入库对应关系…')
    candidate.registration_request = intent.request
    delete candidate.registration_status
    const prepared = await io.call<ProjectOpen>('project.commit', commitArgs(original, JSON.stringify(root), intent.prepare_commit_id))
    checkResult(prepared, intent.project_id)
  }
  io.phase('正在按固定版本入库…')
  const result = await io.call<WorldResult>('world.register', { ...intent.request })
  if (!uuid.test(result.asset_id) || !uuid.test(result.asset_version_id) || result.manifest.source.project_id !== intent.project_id || result.manifest.source.project_revision_id !== intent.request.source_revision_id || result.manifest.files.length !== intent.request.file_ids.length || result.manifest.files.some(f => !intent.request.file_ids.includes(f.file_id))) fail('入库返回来源不匹配，请按原操作核对。')
  const status = await io.call<WorldRegistrationStatus>('world.commit_status', { asset_id: result.asset_id, commit_id: intent.request.commit_id })
  if (status.state !== 'committed' || canonical(status.result) !== canonical(result)) fail('入库结果尚未核实，请按原操作恢复。')
  intent.published_asset_id = result.asset_id; intent.published_version_id = result.asset_version_id
  io.save(intent); io.phase('已入库，正在同步项目关联…')
  if (intent.association_base_revision_id) {
    const status = await io.call<{ state: string; result?: ProjectOpen }>('project.commit_status', { project_id: intent.project_id, commit_id: intent.association_commit_id }).catch(() => null)
    if (status?.state === 'committed' && status.result) { checkResult(status.result, intent.project_id); return { revision_id: status.result.revision_id, result } }
    if (status?.state === 'conflict') { intent.association_commit_id = io.newID(); delete intent.association_base_revision_id; io.save(intent) }
    if (status?.state === 'failed') fail('入库已成功，项目关联被拒绝；请经 MCP 核对后恢复，不要再次发布。')
  }
  const current = await io.read(intent.association_base_revision_id)
  if (current.project.project_id !== intent.project_id || intent.association_base_revision_id && current.project.revision_id !== intent.association_base_revision_id) fail()
  const update = sourceCandidate(current, intent.candidate_key)
  if (canonical(update.candidate.registration_request) !== canonical(intent.request)) fail('入库已成功，但项目候选已变化，请经 MCP 核对关联。')
  if (canonical(update.candidate.registration_status) === canonical(status)) return { revision_id: current.project.revision_id, result }
  update.candidate.registration_status = status
  intent.association_base_revision_id = current.project.revision_id
  io.save(intent)
  const saved = await io.call<ProjectOpen>('project.commit', commitArgs(current, JSON.stringify(update.root), intent.association_commit_id))
  checkResult(saved, intent.project_id)
  return { revision_id: saved.revision_id, result }
}
