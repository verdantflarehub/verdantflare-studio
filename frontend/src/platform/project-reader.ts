import { callTool, MCPRequestError } from './mcp'
import { ProjectReads } from './project-reads'

export * from './project-content'
import { parseProjectContent, type ContentRef, type ProjectFile, type ProjectOpen, type ProjectContent, type WorldResult } from './project-content'
interface Download { version: ContentRef & { mime: string; size: number; sha256: string }; content_path: string }
const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-7[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/
export function isProjectID(value: string): boolean { return uuid.test(value) }
export function readerError(error: unknown): string {
  if (error instanceof MCPRequestError && error.status === 401) return '当前会话已失效，请登录后重试。'
  if (error instanceof MCPRequestError && error.status === 403) return '当前账号没有读取此内容的权限。'
  return error instanceof Error ? error.message : '内容暂不可用，请重试。'
}

/** Download addresses must match this exact file and revision, with no redirects or extra scope. */
export function controlledContentPath(path: string, file: ProjectFile, project: ProjectOpen, assetAccess?: { asset_id: string; asset_version_id: string }): string {
  const fail = (): never => { throw new Error('媒体读取地址与当前项目不匹配。') }
  if (!path.startsWith('/v2/artifacts/') || path.includes('#') || path.includes('\\')) fail()
  const url = new URL(path, location.origin)
  if (url.origin !== location.origin || url.pathname !== `/v2/artifacts/${file.content_ref.version_id}/content`) fail()
  const expected: Record<string, string> = { store_id: file.content_ref.store_id, artifact_id: file.content_ref.artifact_id, ...(assetAccess || { project_id: project.project_id, project_revision_id: project.revision_id }) }
  url.searchParams.forEach((_value, key) => { if (!(key in expected) || url.searchParams.getAll(key).length !== 1) fail() })
  for (const [key, value] of Object.entries(expected)) if (url.searchParams.get(key) !== value) fail()
  return url.pathname + url.search
}

export class ProjectReader {
  project: ProjectOpen | null = null
  content: ProjectContent | null = null
  rawContent: string | null = null
  private assetAccess?: { asset_id: string; asset_version_id: string }
  private disposed = false
  private controller = new AbortController()
  private downloads = new Map<string, Promise<Download>>()
  private blobs = new Map<string, Promise<string>>()
  private urls = new Set<string>()
  private documents = new Map<string, Promise<{ text: string; mime: string }>>()
  private reads = new ProjectReads(error => error instanceof MCPRequestError && [502, 503, 504].includes(error.status))
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
    if (project.head_revision_id !== undefined && !isProjectID(project.head_revision_id)) throw new Error('项目最新修订信息不正确。')
    const documents = project.manifest.domain_documents.filter(d => d.document_type === 'project-content')
    if (documents.length > 1) throw new Error('当前修订包含重复的项目业务文档。')
    if (!documents.length) return
    const read = await this.document(documents[0].file_id)
    this.content = parseProjectContent(read.text, project)
    this.rawContent = read.text
  }
  async loadAsset(assetID: string, versionID: string): Promise<WorldResult> {
    if (![assetID, versionID].every(isProjectID)) throw new Error('数字资产版本地址无效。')
    const result = await callTool<WorldResult>('world.get', { asset_id: assetID, asset_version_id: versionID })
    this.active()
    if (result.asset_id !== assetID || result.asset_version_id !== versionID || result.manifest.asset_id !== assetID || result.manifest.source.project_id !== this.projectID || !Array.isArray(result.manifest.files)) throw new Error('数字资产返回版本或来源项目不匹配。')
    this.assetAccess = { asset_id: assetID, asset_version_id: versionID }
    this.project = { project_id: this.projectID, revision_id: result.manifest.source.project_revision_id, manifest: { name: result.manifest.name, category: 'asset', files: result.manifest.files, domain_documents: [] } }
    return result
  }
  file(id: string): ProjectFile {
    this.active()
    const file = this.project?.manifest.files.find(f => f.file_id === id)
    if (!file || !isProjectID(file.file_id) || !file.content_ref || ![file.content_ref.store_id, file.content_ref.artifact_id, file.content_ref.version_id].every(isProjectID)) throw new Error('文件不在当前项目修订中。')
    return file
  }
  private access() { return this.assetAccess || { project_id: this.projectID, project_revision_id: this.project!.revision_id } }
  async document(id: string): Promise<{ text: string; mime: string }> {
    this.active()
    this.file(id)
    if (!this.documents.has(id)) {
      const promise = this.reads.run(() => this.readDocument(id)).catch(error => { this.documents.delete(id); throw error })
      this.documents.set(id, promise)
    }
    return this.documents.get(id)!
  }
  private async readDocument(id: string): Promise<{ text: string; mime: string }> {
    this.active()
    const file = this.file(id)
    const read = await callTool<{ mode: string; text: string; version: Download['version'] }>('artifact.read', { mode: 'text', content_ref: file.content_ref, access: this.access() }, this.assetAccess ? undefined : this.projectID)
    this.active()
    const v = read.version
    if (read.mode !== 'text' || typeof read.text !== 'string' || !v || v.store_id !== file.content_ref.store_id || v.artifact_id !== file.content_ref.artifact_id || v.version_id !== file.content_ref.version_id || typeof v.mime !== 'string' || !Number.isSafeInteger(v.size) || !/^[a-f0-9]{64}$/.test(v.sha256)) throw new Error('资料内容与当前固定版本不匹配。')
    const bytes = new TextEncoder().encode(read.text)
    if (bytes.length > 4 * 1024 * 1024 || bytes.length !== v.size) throw new Error('资料正文不完整，请下载查看。')
    const hash = await crypto.subtle.digest('SHA-256', bytes)
    this.active()
    if (Array.from(new Uint8Array(hash), b => b.toString(16).padStart(2, '0')).join('') !== v.sha256) throw new Error('资料内容校验失败，请重试。')
    return { text: read.text, mime: v.mime.split(';')[0].trim().toLowerCase() }
  }
  async download(id: string): Promise<Download> {
    this.active()
    if (!this.downloads.has(id)) {
      const file = this.file(id)
      const promise = this.reads.run(() => callTool<Download>('artifact.read', { mode: 'download', content_ref: file.content_ref, access: this.access() }, this.assetAccess ? undefined : this.projectID)).then(read => {
        this.active()
        if (!read.version || typeof read.version.mime !== 'string' || !Number.isSafeInteger(read.version.size) || read.version.size < 0 || !/^[a-f0-9]{64}$/.test(read.version.sha256) || typeof read.content_path !== 'string') throw new Error('媒体信息不完整。')
        if (read.version.store_id !== file.content_ref.store_id || read.version.artifact_id !== file.content_ref.artifact_id || read.version.version_id !== file.content_ref.version_id) throw new Error('媒体版本与当前项目不匹配。')
        return { ...read, content_path: controlledContentPath(read.content_path, file, this.project!, this.assetAccess) }
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
      const promise = this.reads.run(async () => {
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
      }).catch(error => { this.blobs.delete(id); throw error })
      this.blobs.set(id, promise)
    }
    return this.blobs.get(id)!
  }
  dispose() { this.disposed = true; this.reads.dispose(); this.controller.abort(); this.urls.forEach(url => URL.revokeObjectURL(url)); this.urls.clear(); this.blobs.clear(); this.downloads.clear(); this.documents.clear() }
}
