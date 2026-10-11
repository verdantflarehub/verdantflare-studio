export type Category = 'character' | 'music' | 'video' | 'ai_short_drama'
export interface ContentRef { store_id: string; artifact_id: string; version_id: string }
export interface ProjectItem { project_id: string; head_revision_id: string; name: string; category: string }
export interface ProjectFile { file_id: string; path: string; role?: string; content_ref: ContentRef }
export interface ProjectOpen {
  project_id: string; revision_id: string
  manifest: { name: string; category: string; files: ProjectFile[]; domain_documents: { document_type: string; file_id: string }[]; run_refs?: RunRef[]; asset_refs?: AssetRef[] }
}
export interface RunRef { service_id: string; run_id: string }
export interface AssetRef { asset_id: string; asset_version_id: string; purpose: string }
export interface Field { label: string; value: string | null }
export interface Detail {
  key: string; title: string; overview: string | null; fields: Field[]; image_file_id: string | null
  reference_file_ids: string[]; result_file_ids: string[]; file_ids: string[]
  prompt: string | null; lyrics: string | null; duration_seconds: number | null
}
export interface Cast {
  key: string; name: string; role: 'lead' | 'support'; portrait_file_id: string | null
  project_id: string | null; revision_id: string | null
}
export interface DramaReview { state: 'unreviewed' | 'in_review' | 'approved' | 'rejected'; record_file_id: string | null }
export interface DefinitionRef { file_id: string; anchor?: string }
export interface DramaState { key: string; name: string; definition_refs: DefinitionRef[]; marks: { key: string; kind: string; part: string; side: string; description: string; reference_refs: DefinitionRef[] }[] }
export interface DramaSegment { key: string; order: number; label: string }
export interface DramaVersion {
  key: string; label: string; basis_revision_id: string; parent_version_key: string | null; change_note: string
  file_refs: { file_id: string; content_ref: ContentRef; purpose: string }[]; generation_record_file_ids: string[]; review: DramaReview
  state_key?: string; purpose?: string; segment_key?: string | null
  spatial_record_file_id?: string | null; lighting_record_file_id?: string | null; change_record_file_id?: string | null; composition_file_id?: string | null
}
export interface DramaSelection { purpose: string; version_key: string; state_key?: string; segment_key?: string | null }
export interface DramaGlobal {
  key: string; kind: 'script' | 'character' | 'wardrobe' | 'prop' | 'location' | 'sound' | 'music'; name: string
  states: DramaState[]; versions: DramaVersion[]; selected_versions: DramaSelection[]
  description: string | null; file_ids: string[]; reference_file_ids: string[]; review: DramaReview
  asset_id: string | null; asset_version_id: string | null
  definition_refs: DefinitionRef[]; definition_review: DramaReview; input_global_keys: string[]; generation_record_file_ids: string[]
}
export interface DramaUsage { state_key: string | null; version_key: string | null; segment: DramaSegment | null; key: string; global_key: string; target: { episode_key: string; scene_key?: string; shot_key?: string }; description: string | null }
export interface DramaDeliverable { versions: DramaVersion[]; selected_version_key: string | null; delivery_record_file_ids: string[]; key: string; title: string; episode_key: string | null; description: string | null; file_ids: string[]; review: DramaReview }
export interface WorldRegistration {
  commit_id: string; source_project_id: string; source_revision_id: string; file_ids: string[]
  name: string; asset_type: string; subjects: string[]; relation: 'produced_in' | 'curated_in'
  asset_id?: string; expected_asset_version_id?: string; review_file_id?: string; depends_on?: AssetRef[]
}
export interface WorldResult {
  asset_id: string; asset_version_id: string; manifest_ref: ContentRef
  manifest: {
    schema_version: 1; kind: 'asset-version'; asset_id: string; name: string; asset_type: string; subjects: string[]
    files: (ProjectFile & { role: string })[]; depends_on: AssetRef[]
    source: { project_id: string; project_revision_id: string; relation: 'produced_in' | 'curated_in'; review_file_id?: string }
    extensions?: Record<string, unknown>
  }
}
export type WorldRegistrationStatus =
  | { request_sha256: string; state: 'preparing' }
  | { request_sha256: string; state: 'committed'; result: WorldResult }
  | { request_sha256: string; state: 'conflict'; error_code: 'REVISION_CONFLICT'; current_asset_version_id: string }
  | { request_sha256: string; state: 'failed'; error_code: 'INVALID_ARGUMENT' | 'NOT_FOUND' | 'PERMISSION_DENIED' | 'CONTENT_NOT_READY' }
export interface DramaWorldCandidate {
  key: string; name: string; source: { kind: 'global' | 'deliverable'; key: string }; description: string | null
  publication_record_file_id: string | null; review: DramaReview; registration_request: WorldRegistration | null; registration_status: WorldRegistrationStatus | null
}
export interface DramaShot {
  segments: DramaSegment[]; versions: DramaVersion[]; selected_versions: DramaSelection[]
  key: string; title: string; overview: string | null; script_anchor: string | null; duration_seconds: number | null
  image_file_id: string | null; reference_file_ids: string[]; result_file_ids: string[]; selected_result_file_id: string | null
  continuity_keys: string[]; run_ref: RunRef | null; review: DramaReview
  definition_refs: DefinitionRef[]; generation_record_file_ids: string[]
}
export interface DramaScene { key: string; title: string; overview: string | null; definition_refs: DefinitionRef[]; shots: DramaShot[] }
export interface DramaEpisode { key: string; title: string; overview: string | null; script_key: string | null; script_file_id: string | null; output_file_id: string | null; edit_file_id: string | null; scenes: DramaScene[] }
export interface DramaContinuity { key: string; name: string; kind: 'character' | 'wardrobe' | 'location' | 'prop' | 'voice' | 'general'; description: string | null; reference_file_ids: string[]; definition_refs: DefinitionRef[]; global_keys: string[]; asset_id: string | null; asset_version_id: string | null }
export interface ProjectContent {
  schema_version: 1; kind: 'project-content'; category: Category; overview: string | null; cover_file_id: string | null
  character?: { name: string | null; portrait_file_id: string | null; portrait_prompt: string | null; sheet_file_id: string | null; sheet_prompt: string | null; fields: Field[]; extensions: Detail[] }
  music?: { artist: string | null; name: string | null; voice: { name: string | null; description: string | null; recordings: Detail[]; models: Detail[] }; songs: Detail[]; mvs: Detail[] }
  video?: { duration_seconds: number | null; aspect: string | null; output_file_id: string | null; storyboards: Detail[]; cast: Cast[] }
  ai_short_drama?: { script_file_id: string | null; aspect: string | null; globals: DramaGlobal[]; usages: DramaUsage[]; deliverables: DramaDeliverable[]; world_candidates: DramaWorldCandidate[]; continuity: DramaContinuity[]; episodes: DramaEpisode[] }
}

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/

/** Reads the central project-content document; never infers content from MD or filenames. */
export function parseProjectContent(raw: string, project: ProjectOpen): ProjectContent {
  const invalid = (): never => { throw new Error('项目业务内容格式不正确，暂时无法展示。') }
  const object = (value: unknown): Record<string, unknown> => value && typeof value === 'object' && !Array.isArray(value) ? value as Record<string, unknown> : invalid()
  const text = (value: unknown): string | null => value == null ? null : typeof value === 'string' ? value : invalid()
  const required = (value: unknown): string => text(value)?.trim() ? text(value)! : invalid()
  const number = (value: unknown): number | null => value == null ? null : typeof value === 'number' && Number.isFinite(value) && value >= 0 ? value : invalid()
  const array = <T>(value: unknown, parse: (item: unknown) => T): T[] => value == null ? [] : Array.isArray(value) && value.length <= 1000 ? value.map(parse) : invalid()
  const files = new Set(project.manifest.files.map(f => f.file_id))
  const reference = (value: unknown): string | null => {
    const id = text(value)
    if (id !== null && (!uuid.test(id) || !files.has(id))) throw new Error('业务内容引用的文件不在当前项目修订中。')
    return id
  }
  const references = (value: unknown) => array(value, item => reference(item) || invalid())
  const fields = (value: unknown): Field[] => array(value, item => { const f = object(item); return { label: required(f.label), value: text(f.value) } })
  const unique = <T extends { key: string }>(items: T[]): T[] => new Set(items.map(i => i.key)).size === items.length ? items : invalid()
  const details = (value: unknown): Detail[] => unique(array(value, item => {
    const d = object(item), key = required(d.key)
    if (!/^[a-zA-Z0-9_-]+$/.test(key)) invalid()
    return { key, title: required(d.title), overview: text(d.overview), fields: fields(d.fields), image_file_id: reference(d.image_file_id), reference_file_ids: references(d.reference_file_ids), result_file_ids: references(d.result_file_ids), file_ids: references(d.file_ids), prompt: text(d.prompt), lyrics: text(d.lyrics), duration_seconds: number(d.duration_seconds) }
  }))
  let json: unknown
  try { json = JSON.parse(raw) } catch { return invalid() }
  const d = object(json)
  if (d.schema_version !== 1 || d.kind !== 'project-content' || !['character', 'music', 'video', 'ai_short_drama'].includes(String(d.category)) || d.category !== project.manifest.category) invalid()
  const category = d.category as Category
  if (['character', 'music', 'video', 'ai_short_drama'].some(key => key !== category && d[key] != null)) invalid()
  const result: ProjectContent = { schema_version: 1, kind: 'project-content', category, overview: text(d.overview), cover_file_id: reference(d.cover_file_id) }
  const body = object(d[category])
  if (category === 'character') result.character = { name: text(body.name), portrait_file_id: reference(body.portrait_file_id), portrait_prompt: text(body.portrait_prompt), sheet_file_id: reference(body.sheet_file_id), sheet_prompt: text(body.sheet_prompt), fields: fields(body.fields), extensions: details(body.extensions) }
  if (category === 'music') {
    const voice = body.voice == null ? {} : object(body.voice)
    result.music = { artist: text(body.artist), name: text(body.name), voice: { name: text(voice.name), description: text(voice.description), recordings: details(voice.recordings), models: details(voice.models) }, songs: details(body.songs), mvs: details(body.mvs) }
  }
  if (category === 'video') result.video = {
    duration_seconds: number(body.duration_seconds), aspect: text(body.aspect), output_file_id: reference(body.output_file_id), storyboards: details(body.storyboards),
    cast: unique(array(body.cast, item => {
      const c = object(item), projectID = text(c.project_id), revisionID = text(c.revision_id)
      if (c.role !== 'lead' && c.role !== 'support') invalid()
      if ((projectID === null) !== (revisionID === null) || (projectID && (!uuid.test(projectID) || !uuid.test(revisionID!)))) invalid()
      return { key: required(c.key), name: required(c.name), role: c.role as Cast['role'], portrait_file_id: reference(c.portrait_file_id), project_id: projectID, revision_id: revisionID }
    }))
  }
  if (category === 'ai_short_drama') {
    const key = (value: unknown) => { const k = required(value); if (!/^[a-zA-Z0-9_-]+$/.test(k)) invalid(); return k }
    const keys = (value: unknown) => unique(array(value, item => ({ key: key(item) }))).map(item => item.key)
    // New collections are optional, but explicit null is not an empty collection.
    const collection = <T>(value: unknown, parse: (item: unknown) => T, limit = 1000): T[] => value === undefined ? [] : Array.isArray(value) && value.length <= limit ? value.map(parse) : invalid()
    const definitions = (value: unknown): DefinitionRef[] => collection(value, item => {
      const d = object(item), file_id = reference(d.file_id) || invalid()
      return d.anchor === undefined ? { file_id } : { file_id, anchor: required(d.anchor) }
    })
    const reviewOf = (value: unknown): DramaReview => {
      if (value === undefined) return { state: 'unreviewed', record_file_id: null }
      const r = object(value)
      if (!['unreviewed', 'in_review', 'approved', 'rejected'].includes(String(r.state))) invalid()
      const record = reference(r.record_file_id)
      if (r.state === 'approved' && !record) invalid()
      return { state: r.state as DramaReview['state'], record_file_id: record }
    }
    const assetOf = (value: Record<string, unknown>) => {
      const asset_id = text(value.asset_id), asset_version_id = text(value.asset_version_id)
      if ((asset_id === null) !== (asset_version_id === null)) invalid()
      if (asset_id && (!uuid.test(asset_id) || !uuid.test(asset_version_id!) || !project.manifest.asset_refs?.some(a => a.asset_id === asset_id && a.asset_version_id === asset_version_id))) invalid()
      return { asset_id, asset_version_id }
    }
    const optionalKey = (v: unknown) => v == null ? null : key(v)
    const segmentOf = (value: unknown): DramaSegment => {
      const r = recordObject(value, ['key', 'order', 'label'])
      if (!Number.isInteger(r.order) || Number(r.order) < 1) invalid()
      return { key: key(r.key), order: Number(r.order), label: required(r.label) }
    }
    const statesOf = (value: unknown): DramaState[] => unique(collection(value, item => {
      const r = recordObject(item, ['key', 'name', 'definition_refs', 'marks'])
      if (!Array.isArray(r.definition_refs) || !Array.isArray(r.marks)) invalid()
      return { key: key(r.key), name: required(r.name), definition_refs: definitions(r.definition_refs), marks: unique(collection(r.marks, item => {
        const m = recordObject(item, ['key', 'kind', 'part', 'side', 'description', 'reference_refs'])
        return { key: key(m.key), kind: required(m.kind), part: required(m.part), side: required(m.side), description: required(m.description), reference_refs: definitions(m.reference_refs) }
      })) }
    }))
    const versionsOf = (value: unknown, kind: 'global' | 'shot' | 'deliverable'): DramaVersion[] => {
      const common = ['key', 'label', 'basis_revision_id', 'parent_version_key', 'change_note', 'file_refs', 'generation_record_file_ids', 'review']
      const extra = kind === 'global' ? ['state_key', 'spatial_record_file_id', 'lighting_record_file_id'] : kind === 'shot' ? ['purpose', 'segment_key', 'change_record_file_id', 'spatial_record_file_id', 'lighting_record_file_id'] : ['change_record_file_id', 'composition_file_id']
      const values = unique(collection(value, item => {
        const v = recordObject(item, [...common, ...extra])
        if (!Array.isArray(v.file_refs) || !v.file_refs.length || !Array.isArray(v.generation_record_file_ids) || v.review === undefined) invalid()
        const files = collection(v.file_refs, item => {
          const f = recordObject(item, ['file_id', 'content_ref', 'purpose']), file_id = reference(f.file_id) || invalid(), content_ref = recordContentRef(f.content_ref)
          const actual = project.manifest.files.find(f => f.file_id === file_id)!.content_ref
          if (actual.store_id !== content_ref.store_id || actual.artifact_id !== content_ref.artifact_id || actual.version_id !== content_ref.version_id) invalid()
          return { file_id, content_ref, purpose: key(f.purpose) }
        })
        distinct(files.map(f => f.file_id))
        const result: DramaVersion = { key: key(v.key), label: required(v.label), basis_revision_id: recordID(v.basis_revision_id), parent_version_key: optionalKey(v.parent_version_key), change_note: required(v.change_note), file_refs: files, generation_record_file_ids: references(v.generation_record_file_ids), review: reviewOf(v.review) }
        if (kind === 'global') result.state_key = key(v.state_key)
        if (kind === 'shot') {
          if (!['whitebox', 'storyboard', 'keyframe', 'segment_video', 'shot_video'].includes(String(v.purpose))) invalid()
          result.purpose = String(v.purpose); result.segment_key = optionalKey(v.segment_key)
          if ((v.purpose === 'shot_video' && result.segment_key) || (v.purpose === 'segment_video' && !result.segment_key)) invalid()
        }
        for (const name of extra.filter(k => k.endsWith('_file_id'))) {
          const id = reference(v[name])
          if ((name === 'change_record_file_id' || name === 'composition_file_id') && !id) invalid()
          ;(result as unknown as Record<string, unknown>)[name] = id
        }
        if ((result.purpose === 'whitebox' || files.some(f => f.purpose === 'whitebox')) && !result.spatial_record_file_id) invalid()
        if (files.some(f => f.purpose === 'lighting') && !result.lighting_record_file_id) invalid()
        return result
      }))
      distinct(values.map(v => JSON.stringify({ ...v, key: null, label: null, parent_version_key: null, change_note: null, review: null,
        file_refs: [...v.file_refs].sort((a, b) => a.file_id.localeCompare(b.file_id)), generation_record_file_ids: [...v.generation_record_file_ids].sort()
      })))
      const byKey = new Map(values.map(v => [v.key, v]))
      for (const v of values) {
        const seen = new Set([v.key]); let parent = v.parent_version_key
        while (parent) { if (seen.has(parent) || !byKey.has(parent)) invalid(); seen.add(parent); parent = byKey.get(parent)!.parent_version_key }
      }
      return values
    }
    const selectionsOf = (value: unknown, versions: DramaVersion[], kind: 'global' | 'shot'): DramaSelection[] => {
      const selections = collection(value, item => {
        const r = recordObject(item, ['purpose', 'version_key', kind === 'global' ? 'state_key' : 'segment_key'])
        const result: DramaSelection = { purpose: key(r.purpose), version_key: key(r.version_key) }
        const version = versions.find(v => v.key === result.version_key); if (!version) invalid()
        if (kind === 'global') { result.state_key = key(r.state_key); if (version!.state_key !== result.state_key || !version!.file_refs.some(f => f.purpose === result.purpose)) invalid() }
        else { result.segment_key = optionalKey(r.segment_key); if (version!.purpose !== result.purpose || version!.segment_key !== result.segment_key) invalid() }
        return result
      })
      distinct(selections.map(s => [s.state_key || '', s.segment_key || '', s.purpose].join('/')))
      return selections
    }
    const sameFiles = (ids: string[], other: string[]) => ids.length === other.length && new Set(ids).size === ids.length && ids.every(id => other.includes(id))
    const globals: DramaGlobal[] = unique(collection(body.globals, item => {
      const g = object(item), asset = assetOf(g), review = reviewOf(g.review), selected = references(g.file_ids)
      const definition_refs = definitions(g.definition_refs), definition_review = reviewOf(g.definition_review)
      if (!['script', 'character', 'wardrobe', 'prop', 'location', 'sound', 'music'].includes(String(g.kind))) invalid()
      if (review.state === 'approved' && !selected.length && !asset.asset_id) invalid()
      if (definition_review.state === 'approved' && !definition_refs.length) invalid()
      const states = statesOf(g.states), versions = versionsOf(g.versions, 'global'), selected_versions = selectionsOf(g.selected_versions, versions, 'global')
      if (versions.some(v => !states.some(s => s.key === v.state_key))) invalid()
      return { states, versions, selected_versions, key: key(g.key), kind: g.kind as DramaGlobal['kind'], name: required(g.name), description: text(g.description), file_ids: selected, reference_file_ids: references(g.reference_file_ids), review, definition_refs, definition_review, input_global_keys: keys(g.input_global_keys), generation_record_file_ids: references(g.generation_record_file_ids), ...asset }
    }))
    const globalByKey = new Map(globals.map(g => [g.key, g]))
    if (globals.some(g => g.input_global_keys.some(k => k === g.key || !globalByKey.has(k)))) invalid()
    const bodyContinuity = (value: unknown): DramaContinuity[] => unique(array(value, item => {
      const c = object(item), asset = assetOf(c), global_keys = keys(c.global_keys)
      if (!['character', 'wardrobe', 'location', 'prop', 'voice', 'general'].includes(String(c.kind)) || global_keys.some(k => !globalByKey.has(k))) invalid()
      return { key: key(c.key), name: required(c.name), kind: c.kind as DramaContinuity['kind'], description: text(c.description), reference_file_ids: references(c.reference_file_ids), definition_refs: definitions(c.definition_refs), global_keys, ...asset }
    }))
    const continuity = bodyContinuity(body.continuity), continuityKeys = new Set(continuity.map(c => c.key))
    if (!Array.isArray(body.episodes)) invalid()
    const episodes = unique(array(body.episodes, episode => {
      const ep = object(episode)
      if (!Array.isArray(ep.scenes)) invalid()
      const scenes = unique(array(ep.scenes, scene => {
        const sc = object(scene)
        if (!Array.isArray(sc.shots)) invalid()
        const shots = unique(array(sc.shots, shot => {
          const sh = object(shot), resultFileIDs = references(sh.result_file_ids), selected = reference(sh.selected_result_file_id)
          if (selected && !resultFileIDs.includes(selected)) invalid()
          const linkedContinuity = keys(sh.continuity_keys)
          if (linkedContinuity.some(k => !continuityKeys.has(k))) invalid()
          let run: RunRef | null = null
          if (sh.run_ref != null) {
            const r = object(sh.run_ref)
            run = { service_id: required(r.service_id), run_id: required(r.run_id) }
            if (!project.manifest.run_refs?.some(ref => ref.service_id === run!.service_id && ref.run_id === run!.run_id)) invalid()
          }
          const review = reviewOf(sh.review)
          if (review.state === 'approved' && !selected) invalid()
          const segments = unique(collection(sh.segments, segmentOf)), versions = versionsOf(sh.versions, 'shot'), selected_versions = selectionsOf(sh.selected_versions, versions, 'shot')
          distinct(segments.map(s => s.order))
          if (versions.some(v => v.segment_key && !segments.some(s => s.key === v.segment_key))) invalid()
          if (versions.length) {
            const whole = versions.filter(v => v.purpose === 'shot_video'), ids = whole.map(v => {
              const outputs = v.file_refs.filter(f => f.purpose === 'video'); if (outputs.length !== 1) invalid(); return outputs[0].file_id
            })
            const chosen = whole.find(v => v.key === selected_versions.find(s => s.purpose === 'shot_video')?.version_key)
            if (!sameFiles(resultFileIDs, ids) || selected !== (chosen?.file_refs.find(f => f.purpose === 'video')?.file_id || null) || chosen && JSON.stringify(chosen.review) !== JSON.stringify(review)) invalid()
          }
          return { segments, versions, selected_versions, key: key(sh.key), title: required(sh.title), overview: text(sh.overview), script_anchor: text(sh.script_anchor), duration_seconds: number(sh.duration_seconds), image_file_id: reference(sh.image_file_id), reference_file_ids: references(sh.reference_file_ids), result_file_ids: resultFileIDs, selected_result_file_id: selected, continuity_keys: linkedContinuity, run_ref: run, review, definition_refs: definitions(sh.definition_refs), generation_record_file_ids: references(sh.generation_record_file_ids) }
        }))
        return { key: key(sc.key), title: required(sc.title), overview: text(sc.overview), definition_refs: definitions(sc.definition_refs), shots }
      }))
      const script_key = ep.script_key == null ? null : key(ep.script_key), script_file_id = reference(ep.script_file_id)
      if (script_key) {
        const script = globalByKey.get(script_key)
        if (!script || script.kind !== 'script' || (script_file_id && !script.file_ids.includes(script_file_id))) invalid()
      }
      return { key: key(ep.key), title: required(ep.title), overview: text(ep.overview), script_key, script_file_id, output_file_id: reference(ep.output_file_id), edit_file_id: reference(ep.edit_file_id), scenes }
    }))
    const episodeByKey = new Map(episodes.map(e => [e.key, e]))
    const usages: DramaUsage[] = unique(collection(body.usages, item => {
      const u = object(item), t = object(u.target), global_key = key(u.global_key)
      if (!globalByKey.has(global_key)) invalid()
      const target: DramaUsage['target'] = { episode_key: key(t.episode_key) }
      const episode = episodeByKey.get(target.episode_key)
      if (!episode) invalid()
      if (t.scene_key !== undefined) {
        target.scene_key = key(t.scene_key)
        const scene = episode!.scenes.find(s => s.key === target.scene_key)
        if (!scene) invalid()
        if (t.shot_key !== undefined) {
          target.shot_key = key(t.shot_key)
          if (!scene!.shots.some(s => s.key === target.shot_key)) invalid()
        }
      } else if (t.shot_key !== undefined) invalid()
      const state_key = optionalKey(u.state_key), version_key = optionalKey(u.version_key), segment = u.segment == null ? null : segmentOf(u.segment), global = globalByKey.get(global_key)!
      if (state_key && !global.states.some(s => s.key === state_key)) invalid()
      if (version_key && (!state_key || !global.versions.some(v => v.key === version_key && v.state_key === state_key))) invalid()
      if (segment) {
        const actual = episode!.scenes.find(s => s.key === target.scene_key)?.shots.find(s => s.key === target.shot_key)?.segments.find(s => s.key === segment.key)
        if (!actual || actual.order !== segment.order || actual.label !== segment.label) invalid()
      }
      return { state_key, version_key, segment, key: key(u.key), global_key, target, description: text(u.description) }
    }, 10000))
    const deliverables: DramaDeliverable[] = unique(collection(body.deliverables, item => {
      const d = object(item), episode_key = d.episode_key == null ? null : key(d.episode_key), file_ids = references(d.file_ids), review = reviewOf(d.review)
      if ((episode_key && !episodeByKey.has(episode_key)) || (review.state === 'approved' && !file_ids.length)) invalid()
      const versions = versionsOf(d.versions, 'deliverable'), selected_version_key = optionalKey(d.selected_version_key), delivery_record_file_ids = references(d.delivery_record_file_ids)
      if (selected_version_key) {
        const version = versions.find(v => v.key === selected_version_key)
        if (!version || !sameFiles(file_ids, version.file_refs.map(f => f.file_id)) || JSON.stringify(review) !== JSON.stringify(version.review)) invalid()
      } else if (versions.length && file_ids.length) invalid()
      return { versions, selected_version_key, delivery_record_file_ids, key: key(d.key), title: required(d.title), episode_key, description: text(d.description), file_ids, review }
    }))
    const deliveryKeys = new Set(deliverables.map(d => d.key))
    const world_candidates: DramaWorldCandidate[] = unique(collection(body.world_candidates, item => {
      const c = object(item), s = object(c.source), sourceKey = key(s.key), review = reviewOf(c.review)
      if (s.kind === 'global' ? !globalByKey.has(sourceKey) : s.kind === 'deliverable' ? !deliveryKeys.has(sourceKey) : true) invalid()
      const registration_request = c.registration_request == null ? null : parseWorldRegistration(c.registration_request)
      if (registration_request && (review.state !== 'approved' || registration_request.source_project_id !== project.project_id || (registration_request.review_file_id && registration_request.review_file_id !== review.record_file_id))) invalid()
      const publication_record_file_id = reference(c.publication_record_file_id)
      // Historical source files belong to the saved source revision, not necessarily this revision.
      if (registration_request?.source_revision_id === project.revision_id) {
        const sourceItem = s.kind === 'global' ? globalByKey.get(sourceKey)! : deliverables.find(d => d.key === sourceKey)!
        // A versioned publication is resolved from its pinned publication record by
        // the service. Do not misinterpret the current selection as the publish scope.
        if (sourceItem.versions.length) {
          if (!publication_record_file_id || !registration_request.file_ids.includes(publication_record_file_id) || registration_request.file_ids.some(id => !files.has(id))) invalid()
        } else if (registration_request.file_ids.some(id => !files.has(id) || !sourceItem.file_ids.includes(id))) invalid()
      }
      const registration_status = c.registration_status == null ? null : parseWorldRegistrationStatus(c.registration_status, registration_request)
      return { publication_record_file_id, key: key(c.key), name: required(c.name), source: { kind: s.kind as 'global' | 'deliverable', key: sourceKey }, description: text(c.description), review, registration_request, registration_status }
    }))
    result.ai_short_drama = { script_file_id: reference(body.script_file_id), aspect: text(body.aspect), globals, usages, deliverables, world_candidates, continuity, episodes }
  }
  return result
}

// Recorded responses do not prove live publication or authorization. Query World before acting.
const recordInvalid = (): never => { throw new Error('数字世界入库记录格式或来源不一致。') }
const recordObject = (value: unknown, allowed: string[]): Record<string, unknown> => {
  if (!value || typeof value !== 'object' || Array.isArray(value)) return recordInvalid()
  const result = value as Record<string, unknown>
  if (Object.keys(result).some(key => !allowed.includes(key))) recordInvalid()
  return result
}
const recordID = (value: unknown): string => typeof value === 'string' && uuid.test(value) ? value : recordInvalid()
const recordName = (value: unknown): string => typeof value === 'string' && value.length > 0 && value.length <= 256 ? value : recordInvalid()
const recordToken = (value: unknown): string => typeof value === 'string' && /^[a-z][a-z0-9_-]{0,63}$/.test(value) ? value : recordInvalid()
const recordArray = <T>(value: unknown, parse: (item: unknown) => T, limit = 10000): T[] => Array.isArray(value) && value.length <= limit ? value.map(parse) : recordInvalid()
const distinct = <T>(items: T[]): T[] => new Set(items).size === items.length ? items : recordInvalid()
const recordRelation = (value: unknown): WorldRegistration['relation'] => value === 'produced_in' || value === 'curated_in' ? value : recordInvalid()
const recordContentRef = (value: unknown): ContentRef => {
  const r = recordObject(value, ['store_id', 'artifact_id', 'version_id'])
  return { store_id: recordID(r.store_id), artifact_id: recordID(r.artifact_id), version_id: recordID(r.version_id) }
}
const recordDependencies = (value: unknown): AssetRef[] => recordArray(value, item => {
  const r = recordObject(item, ['asset_id', 'asset_version_id', 'purpose'])
  return { asset_id: recordID(r.asset_id), asset_version_id: recordID(r.asset_version_id), purpose: recordName(r.purpose) }
})

function parseWorldRegistration(value: unknown): WorldRegistration {
  const r = recordObject(value, ['commit_id', 'source_project_id', 'source_revision_id', 'file_ids', 'name', 'asset_type', 'subjects', 'relation', 'asset_id', 'expected_asset_version_id', 'review_file_id', 'depends_on'])
  const file_ids = distinct(recordArray(r.file_ids, recordID))
  if (!file_ids.length) recordInvalid()
  const result: WorldRegistration = {
    commit_id: recordID(r.commit_id), source_project_id: recordID(r.source_project_id), source_revision_id: recordID(r.source_revision_id), file_ids,
    name: recordName(r.name), asset_type: recordToken(r.asset_type), subjects: distinct(recordArray(r.subjects, recordName, 256)), relation: recordRelation(r.relation)
  }
  if ((r.asset_id === undefined) !== (r.expected_asset_version_id === undefined)) recordInvalid()
  if (r.asset_id !== undefined) {
    result.asset_id = recordID(r.asset_id)
    result.expected_asset_version_id = recordID(r.expected_asset_version_id)
  }
  if (r.review_file_id !== undefined) result.review_file_id = recordID(r.review_file_id)
  if (r.depends_on !== undefined) result.depends_on = recordDependencies(r.depends_on)
  return result
}

function parseWorldRegistrationStatus(value: unknown, request: WorldRegistration | null): WorldRegistrationStatus {
  if (!request) return recordInvalid()
  const r = recordObject(value, ['request_sha256', 'state', 'result', 'error_code', 'current_asset_version_id'])
  const request_sha256 = typeof r.request_sha256 === 'string' && /^[0-9a-f]{64}$/.test(r.request_sha256) ? r.request_sha256 : recordInvalid()
  if (r.state === 'preparing') {
    recordObject(value, ['request_sha256', 'state'])
    return { state: 'preparing', request_sha256 }
  }
  if (r.state === 'failed') {
    recordObject(value, ['request_sha256', 'state', 'error_code'])
    if (!['INVALID_ARGUMENT', 'NOT_FOUND', 'PERMISSION_DENIED', 'CONTENT_NOT_READY'].includes(String(r.error_code))) recordInvalid()
    return { state: 'failed', request_sha256, error_code: r.error_code as Extract<WorldRegistrationStatus, { state: 'failed' }>['error_code'] }
  }
  if (r.state === 'conflict') {
    recordObject(value, ['request_sha256', 'state', 'error_code', 'current_asset_version_id'])
    if (r.error_code !== 'REVISION_CONFLICT' || !request.asset_id) recordInvalid()
    return { state: 'conflict', request_sha256, error_code: 'REVISION_CONFLICT', current_asset_version_id: recordID(r.current_asset_version_id) }
  }
  if (r.state !== 'committed') return recordInvalid()
  recordObject(value, ['request_sha256', 'state', 'result'])
  const output = recordObject(r.result, ['asset_id', 'asset_version_id', 'manifest_ref', 'manifest'])
  const m = recordObject(output.manifest, ['schema_version', 'kind', 'asset_id', 'name', 'asset_type', 'subjects', 'files', 'source', 'depends_on', 'extensions'])
  const s = recordObject(m.source, ['project_id', 'project_revision_id', 'relation', 'review_file_id'])
  const source: WorldResult['manifest']['source'] = { project_id: recordID(s.project_id), project_revision_id: recordID(s.project_revision_id), relation: recordRelation(s.relation) }
  if (s.review_file_id !== undefined) source.review_file_id = recordID(s.review_file_id)
  const asset_id = recordID(output.asset_id), asset_version_id = recordID(output.asset_version_id)
  const files = recordArray(m.files, item => {
    const f = recordObject(item, ['file_id', 'path', 'role', 'content_ref'])
    if (typeof f.path !== 'string' || !f.path.length || f.path.length > 1024 || !/^(?!\/)(?!.*\/\/)(?!.*(?:^|\/)\.{1,2}(?:\/|$))(?!\.vf(?:\/|$))(?!.*[. ](?:\/|$))(?!.*\/$)[^\u0000-\u001f\u007f\\<>:"|?*]+$/.test(f.path)) recordInvalid()
    return { file_id: recordID(f.file_id), path: f.path as string, role: recordToken(f.role), content_ref: recordContentRef(f.content_ref) }
  })
  distinct(files.map(f => f.file_id))
  if (!files.length || m.schema_version !== 1 || m.kind !== 'asset-version' || m.asset_id !== asset_id) recordInvalid()
  if ((request.asset_id && request.asset_id !== asset_id) || source.project_id !== request.source_project_id || source.project_revision_id !== request.source_revision_id || source.relation !== request.relation || source.review_file_id !== request.review_file_id) recordInvalid()
  if (files.length !== request.file_ids.length || files.some(f => !request.file_ids.includes(f.file_id))) recordInvalid()
  const subjects = distinct(recordArray(m.subjects, recordName, 256))
  if (m.name !== request.name || m.asset_type !== request.asset_type || subjects.length !== request.subjects.length || subjects.some(s => !request.subjects.includes(s))) recordInvalid()
  const depends_on = recordDependencies(m.depends_on)
  if (request.depends_on && (depends_on.length !== request.depends_on.length || depends_on.some(d => !request.depends_on!.some(r => d.asset_id === r.asset_id && d.asset_version_id === r.asset_version_id && d.purpose === r.purpose)))) recordInvalid()
  const manifest: WorldResult['manifest'] = { schema_version: 1, kind: 'asset-version', asset_id, name: recordName(m.name), asset_type: recordToken(m.asset_type), subjects, files, source, depends_on }
  if (m.extensions !== undefined) {
    if (!m.extensions || typeof m.extensions !== 'object' || Array.isArray(m.extensions) || Object.keys(m.extensions).some(k => !/^[a-z][a-z0-9_-]*\.[a-z][a-z0-9_.-]*$/.test(k))) recordInvalid()
    manifest.extensions = m.extensions as Record<string, unknown>
  }
  return { state: 'committed', request_sha256, result: { asset_id, asset_version_id, manifest_ref: recordContentRef(output.manifest_ref), manifest } }
}

