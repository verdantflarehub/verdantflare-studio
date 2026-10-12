import { canonical, publicationPackage, publishPrepared, type PublicationIntent, type PublicationIO, type PublicationSnapshot } from './drama-publication'
import { parseProjectContent, type DramaGlobal, type DramaDeliverable, type DramaVersion, type WorldRegistration } from './project-content'

type Owner = DramaGlobal | DramaDeliverable
export interface PagePublication {
  project_id: string; viewed_revision_id: string; source: { kind: 'global' | 'deliverable'; key: string }
  candidate_key: string; name: string; actor: string; confirmed_at: string; version: DramaVersion
  files: DramaVersion['file_refs']; support: DramaVersion['file_refs']; asset_type: string; subject: string
  target?: { asset_id: string; expected_asset_version_id: string }
  record_files?: { file_id: string; path: string; content_ref: DramaVersion['file_refs'][number]['content_ref'] }[]
  records_revision_id?: string; candidate_revision_id?: string
  pending?: { phase: 'records' | 'candidate'; args: Record<string, unknown> }
  publication?: PublicationIntent
}
export interface PagePublicationIO extends PublicationIO { saveDraft(draft: PagePublication): void }
export function publicationSupport(owner: Owner, version: DramaVersion, ids: string[], snapshot: PublicationSnapshot) {
  const required = new Set<string>()
  for (const file of version.file_refs.filter(f => ids.includes(f.file_id))) {
    if (file.purpose === 'whitebox') { if (!version.spatial_record_file_id) throw Error('此白模版本缺少空间说明，请先补齐。'); required.add(version.spatial_record_file_id) }
    if (file.purpose === 'lighting') { if (!version.lighting_record_file_id) throw Error('此灯光版本缺少灯光说明，请先补齐。'); required.add(version.lighting_record_file_id) }
  }
  return [...required].filter(id => !ids.includes(id)).map(id => {
    const file = snapshot.project.manifest.files.find(f => f.file_id === id)
    if (!file) throw Error('本次必需说明文件缺失。')
    return { file_id: file.file_id, content_ref: file.content_ref, purpose: 'supporting_record' }
  })
}
export function createPagePublication(snapshot: PublicationSnapshot, owner: Owner, version: DramaVersion, ids: string[], name: string, actor: string, now: string, newID: () => string, target?: PagePublication['target']): PagePublication {
  if (!name.trim() || !actor || !ids.length || new Set(ids).size !== ids.length || ids.some(id => !version.file_refs.some(f => f.file_id === id))) throw Error('请核对资产名称、登录身份与本次文件范围。')
  const files = version.file_refs.filter(f => ids.includes(f.file_id))
  if (files.some(f => canonical(snapshot.project.manifest.files.find(x => x.file_id === f.file_id)?.content_ref) !== canonical(f.content_ref))) throw Error('所选成果与项目固定文件不匹配。')
  return { project_id: snapshot.project.project_id, viewed_revision_id: snapshot.project.revision_id, source: { kind: 'kind' in owner ? 'global' : 'deliverable', key: owner.key }, candidate_key: 'page_' + newID().replaceAll('-', ''), name: name.trim(), actor, confirmed_at: now, version: structuredClone(version), files: structuredClone(files), support: publicationSupport(owner, version, ids, snapshot), asset_type: 'kind' in owner ? owner.kind : 'video-project', subject: 'name' in owner ? owner.name : owner.title, ...(target ? { target } : {}) }
}
function scope(draft: PagePublication, snapshot: PublicationSnapshot) {
  if (snapshot.project.project_id !== draft.project_id) throw Error('来源项目不匹配。')
  const content = parseProjectContent(snapshot.raw, snapshot.project).ai_short_drama!
  const owner = draft.source.kind === 'global' ? content.globals.find(g => g.key === draft.source.key) : content.deliverables.find(g => g.key === draft.source.key)
  const version = owner?.versions.find(v => v.key === draft.version.key)
  if (!owner || !version || canonical(version) !== canonical(draft.version)) throw Error('所选版本已变化，请返回素材重新核对入库。')
  for (const f of [...draft.files, ...draft.support]) if (canonical(snapshot.project.manifest.files.find(x => x.file_id === f.file_id)?.content_ref) !== canonical(f.content_ref)) throw Error('所选固定文件已变化，请重新核对。')
  return owner
}
function domainUpdate(snapshot: PublicationSnapshot, text: string) {
  const id = snapshot.project.manifest.domain_documents.find(d => d.document_type === 'project-content')?.file_id
  const f = snapshot.project.manifest.files.find(f => f.file_id === id)
  if (!f?.role) throw Error('项目内容入口不完整。')
  return { file_id: f.file_id, path: f.path, role: f.role, mime: 'application/json', text }
}
/** Preparation writes only explicit records/candidate; preserves all production reviews and selections.
 * Each request is persisted before send. Unknown outcomes replay identical arguments; known CAS
 * conflicts re-read the head and revalidate the approved scope before forming a new request. */
export async function publishFromPage(draft: PagePublication, io: PagePublicationIO) {
  const recordPath = `publications/${draft.candidate_key}/scope.json`, reviewPath = `publications/${draft.candidate_key}/review.md`
  async function stage(phase: 'records' | 'candidate', build: (s: PublicationSnapshot) => Record<string, unknown>) {
    for (let attempt = 0; attempt < 2; attempt++) {
      if (!draft.pending) {
        const snapshot = await io.read(); scope(draft, snapshot)
        draft.pending = { phase, args: { project_id: draft.project_id, expected_revision_id: snapshot.project.revision_id, commit_id: io.newID(), changes: build(snapshot) } }
        io.saveDraft(draft)
      }
      if (draft.pending.phase !== phase) throw Error('入库恢复步骤不匹配。')
      try {
        const saved = await io.call<{ project_id: string; revision_id: string }>('project.commit', draft.pending.args)
        if (saved.project_id !== draft.project_id || !saved.revision_id) throw Error('项目保存结果不匹配。')
        const actual = await io.read(saved.revision_id)
        if (actual.project.revision_id !== saved.revision_id) throw Error('保存修订回读不匹配。')
        if (phase === 'records') { draft.records_revision_id = saved.revision_id; draft.record_files = actual.project.manifest.files.filter(f => f.path === recordPath || f.path === reviewPath) }
        else draft.candidate_revision_id = saved.revision_id
        delete draft.pending; io.saveDraft(draft)
        return
      } catch (error) {
        // Status lookup is read-only. Never reissue with a new ID after an unknown outcome.
        const status = await io.call<{ state: string }>('project.commit_status', { project_id: draft.project_id, commit_id: draft.pending?.args.commit_id }).catch(() => null)
        if (status?.state !== 'conflict') throw error
        delete draft.pending; io.saveDraft(draft)
        if (attempt === 1) throw Error('项目正在被其他操作修改，请稍后恢复本次入库。')
      }
    }
  }
  if (!draft.records_revision_id) {
    io.phase('正在保存本次入库范围与人工确认…')
    const record = JSON.stringify({ schema_version: 1, kind: 'short-drama-publication', source: draft.source, versions: [{ version_key: draft.version.key, file_refs: draft.files }], supporting_file_refs: draft.support })
    const review = `# 页面入库确认\n\n## 1. 确认范围\n\n用户通过页面勾选并点击确认入库。此记录仅批准下列固定范围入库，不改变制作审核或项目选用。\n\n${JSON.stringify({ actor: draft.actor, confirmed_at: draft.confirmed_at, project_id: draft.project_id, viewed_revision_id: draft.viewed_revision_id, source: draft.source, version: draft.version.key, name: draft.name, target: draft.target || 'new', files: draft.files, supporting_files: draft.support }, null, 2)}\n`
    await stage('records', () => ({ upsert_files: [{ path: recordPath, role: 'publication', mime: 'application/json', text: record }, { path: reviewPath, role: 'review', mime: 'text/markdown', text: review }] }))
  }
  if (!draft.candidate_revision_id) {
    io.phase('正在保存来源项目的入库候选…')
    await stage('candidate', snapshot => {
      const owner = scope(draft, snapshot), root = JSON.parse(snapshot.raw)
      root.ai_short_drama.world_candidates ||= []
      if (draft.record_files?.length !== 2 || draft.record_files.some(f => canonical(snapshot.project.manifest.files.find(x => x.file_id === f.file_id)?.content_ref) !== canonical(f.content_ref))) throw Error('本次入库确认文件已变化，请核对原操作。')
      const record = snapshot.project.manifest.files.find(f => f.path === recordPath), review = snapshot.project.manifest.files.find(f => f.path === reviewPath)
      if (!record || !review || root.ai_short_drama.world_candidates.some((c: { key: string }) => c.key === draft.candidate_key)) throw Error('入库准备记录已变化，请核对原操作。')
      const candidate = { key: draft.candidate_key, name: draft.name, source: draft.source, publication_record_file_id: record.file_id, review: { state: 'approved' as const, record_file_id: review.file_id }, description: '页面人工确认入库；制作审核及项目选用保持原记录。', registration_request: null, registration_status: null }
      publicationPackage(JSON.stringify({ schema_version: 1, kind: 'short-drama-publication', source: draft.source, versions: [{ version_key: draft.version.key, file_refs: draft.files }], supporting_file_refs: draft.support }), candidate, owner, snapshot.project)
      root.ai_short_drama.world_candidates.push(candidate)
      return { upsert_files: [domainUpdate(snapshot, JSON.stringify(root))] }
    })
  }
  if (!draft.publication) {
    const snapshot = await io.read(draft.candidate_revision_id), owner = scope(draft, snapshot)
    const candidate = parseProjectContent(snapshot.raw, snapshot.project).ai_short_drama!.world_candidates.find(c => c.key === draft.candidate_key)!
    const record = snapshot.project.manifest.files.find(f => f.path === recordPath)!
    const request: WorldRegistration = { commit_id: io.newID(), source_project_id: draft.project_id, source_revision_id: draft.candidate_revision_id!, name: draft.name, asset_type: draft.asset_type, subjects: [draft.subject], relation: 'produced_in', review_file_id: candidate.review.record_file_id!, file_ids: [record.file_id, ...draft.files.map(f => f.file_id), ...draft.support.map(f => f.file_id)].sort(), ...draft.target }
    if (!owner || !candidate) throw Error('保存的入库范围缺失。')
    draft.publication = { project_id: draft.project_id, origin_revision_id: draft.candidate_revision_id!, candidate_key: draft.candidate_key, request, prepare_commit_id: io.newID(), association_commit_id: io.newID() }
    io.saveDraft(draft)
  }
  return publishPrepared(draft.publication, { ...io, save: intent => { draft.publication = intent; io.saveDraft(draft) } })
}
