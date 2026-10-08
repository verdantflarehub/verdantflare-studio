import { callTool, MCPRequestError } from './mcp'

export type Category = 'character' | 'music' | 'video'
export interface ContentRef { store_id: string; artifact_id: string; version_id: string }
export interface ProjectItem { project_id: string; head_revision_id: string; name: string; category: string }
export interface ProjectFile { file_id: string; path: string; content_ref: ContentRef }
export interface ProjectOpen {
  project_id: string; revision_id: string
  manifest: { name: string; category: string; files: ProjectFile[]; domain_documents: { document_type: string; file_id: string }[] }
}
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
export interface ProjectContent {
  schema_version: 1; kind: 'project-content'; category: Category; overview: string | null; cover_file_id: string | null
  character?: { name: string | null; portrait_file_id: string | null; portrait_prompt: string | null; sheet_file_id: string | null; sheet_prompt: string | null; fields: Field[]; extensions: Detail[] }
  music?: { artist: string | null; name: string | null; voice: { name: string | null; description: string | null; recordings: Detail[]; models: Detail[] }; songs: Detail[]; mvs: Detail[] }
  video?: { duration_seconds: number | null; aspect: string | null; output_file_id: string | null; storyboards: Detail[]; cast: Cast[] }
}
interface Download { version: ContentRef & { mime: string; size: number; sha256: string }; content_path: string }
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
export function isProjectID(value: string): boolean { return uuid.test(value) }
export function readerError(error: unknown): string {
  if (error instanceof MCPRequestError && error.status === 401) return '当前会话已失效，请登录后重试。'
  if (error instanceof MCPRequestError && error.status === 403) return '当前账号没有读取此内容的权限。'
  return error instanceof Error ? error.message : '内容暂不可用，请重试。'
}

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
  if (d.schema_version !== 1 || d.kind !== 'project-content' || !['character', 'music', 'video'].includes(String(d.category)) || d.category !== project.manifest.category) invalid()
  const category = d.category as Category
  if (['character', 'music', 'video'].some(key => key !== category && d[key] != null)) invalid()
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
  return result
}

/** Download addresses must match this exact file and revision, with no redirects or extra scope. */
export function controlledContentPath(path: string, file: ProjectFile, project: ProjectOpen): string {
  const fail = (): never => { throw new Error('媒体读取地址与当前项目不匹配。') }
  if (!path.startsWith('/v2/artifacts/') || path.includes('#') || path.includes('\\')) fail()
  const url = new URL(path, location.origin)
  if (url.origin !== location.origin || url.pathname !== `/v2/artifacts/${file.content_ref.version_id}/content`) fail()
  const expected: Record<string, string> = { store_id: file.content_ref.store_id, artifact_id: file.content_ref.artifact_id, project_id: project.project_id, project_revision_id: project.revision_id }
  url.searchParams.forEach((_value, key) => { if (!(key in expected) || url.searchParams.getAll(key).length !== 1) fail() })
  for (const [key, value] of Object.entries(expected)) if (url.searchParams.get(key) !== value) fail()
  return url.pathname + url.search
}

export class ProjectReader {
  project: ProjectOpen | null = null
  content: ProjectContent | null = null
  private disposed = false
  private controller = new AbortController()
  private downloads = new Map<string, Promise<Download>>()
  private blobs = new Map<string, Promise<string>>()
  private urls = new Set<string>()
  constructor(readonly projectID: string, readonly revisionID?: string) {}
  private active() { if (this.disposed) throw new Error('项目已切换。') }
  async load(): Promise<void> {
    if (!isProjectID(this.projectID) || (this.revisionID && !isProjectID(this.revisionID))) throw new Error('项目地址无效。')
    const args: Record<string, string> = { project_id: this.projectID }
    if (this.revisionID) args.revision_id = this.revisionID
    const project = await callTool<ProjectOpen>('project.open', args, this.projectID)
    this.active()
    if (project.project_id !== this.projectID || !isProjectID(project.revision_id) || (this.revisionID && project.revision_id !== this.revisionID) || !Array.isArray(project.manifest?.files) || !Array.isArray(project.manifest?.domain_documents)) throw new Error('项目修订返回内容不正确。')
    if (typeof project.manifest.name !== 'string' || typeof project.manifest.category !== 'string' || new Set(project.manifest.files.map(f => f.file_id)).size !== project.manifest.files.length) throw new Error('项目文件清单不正确。')
    this.project = project
    const documents = project.manifest.domain_documents.filter(d => d.document_type === 'project-content')
    if (documents.length > 1) throw new Error('当前修订包含重复的项目业务文档。')
    if (!documents.length) return
    const file = this.file(documents[0].file_id)
    const read = await callTool<{ text?: string }>('artifact.read', { mode: 'text', content_ref: file.content_ref, access: this.access() }, this.projectID)
    this.active()
    if (typeof read.text !== 'string') throw new Error('项目业务文档没有可读取的正文。')
    this.content = parseProjectContent(read.text, project)
  }
  file(id: string): ProjectFile {
    this.active()
    const file = this.project?.manifest.files.find(f => f.file_id === id)
    if (!file || !isProjectID(file.file_id) || !file.content_ref || ![file.content_ref.store_id, file.content_ref.artifact_id, file.content_ref.version_id].every(isProjectID)) throw new Error('文件不在当前项目修订中。')
    return file
  }
  private access() { return { project_id: this.projectID, project_revision_id: this.project!.revision_id } }
  async download(id: string): Promise<Download> {
    this.active()
    if (!this.downloads.has(id)) {
      const file = this.file(id)
      const promise = callTool<Download>('artifact.read', { mode: 'download', content_ref: file.content_ref, access: this.access() }, this.projectID).then(read => {
        this.active()
        if (!read.version || typeof read.version.mime !== 'string' || !Number.isSafeInteger(read.version.size) || read.version.size < 0 || !/^[a-f0-9]{64}$/.test(read.version.sha256) || typeof read.content_path !== 'string') throw new Error('媒体信息不完整。')
        if (read.version.store_id !== file.content_ref.store_id || read.version.artifact_id !== file.content_ref.artifact_id || read.version.version_id !== file.content_ref.version_id) throw new Error('媒体版本与当前项目不匹配。')
        return { ...read, content_path: controlledContentPath(read.content_path, file, this.project!) }
      }).catch(error => { this.downloads.delete(id); throw error })
      this.downloads.set(id, promise)
    }
    return this.downloads.get(id)!
  }
  async media(id: string, kind: 'image' | 'audio' | 'video'): Promise<string> {
    const read = await this.download(id)
    const mime = read.version.mime.split(';')[0].trim().toLowerCase()
    const supported = { image: ['image/png', 'image/jpeg', 'image/webp', 'image/gif'], audio: ['audio/mpeg', 'audio/mp3', 'audio/wav', 'audio/x-wav', 'audio/flac', 'audio/ogg', 'audio/mp4'], video: ['video/mp4', 'video/webm', 'video/ogg'] }
    if (!supported[kind].includes(mime)) throw new Error('文件格式不支持此类预览。')
    if (read.version.size > 128 * 1024 * 1024) throw new Error('文件超过浏览器预览大小，请下载查看。')
    if (!this.blobs.has(id)) {
      const promise = (async () => {
        const response = await fetch(read.content_path, { credentials: 'same-origin', cache: 'no-store', redirect: 'error', signal: this.controller.signal })
        if (!response.ok) throw new MCPRequestError('媒体读取失败，请重试。', response.status)
        if (!response.body) throw new Error('媒体没有可读取的内容。')
        const stream = response.body.getReader(), chunks: BlobPart[] = []
        let size = 0
        try {
          for (;;) {
            const { done, value } = await stream.read()
            if (done) break
            size += value.byteLength
            if (size > read.version.size || size > 128 * 1024 * 1024) { await stream.cancel(); throw new Error('媒体大小与声明不匹配。') }
            chunks.push(new Uint8Array(value))
          }
        } finally { stream.releaseLock() }
        const blob = new Blob(chunks, { type: mime })
        this.active()
        if (blob.size !== read.version.size) throw new Error('媒体未完整读取，请重试。')
        const digest = await crypto.subtle.digest('SHA-256', await blob.arrayBuffer())
        if (Array.from(new Uint8Array(digest), n => n.toString(16).padStart(2, '0')).join('') !== read.version.sha256) throw new Error('媒体内容校验失败，请重试。')
        this.active()
        const url = URL.createObjectURL(new Blob([blob], { type: mime })); this.urls.add(url); return url
      })().catch(error => { this.blobs.delete(id); throw error })
      this.blobs.set(id, promise)
    }
    return this.blobs.get(id)!
  }
  dispose() { this.disposed = true; this.controller.abort(); this.urls.forEach(url => URL.revokeObjectURL(url)); this.urls.clear(); this.blobs.clear(); this.downloads.clear() }
}
