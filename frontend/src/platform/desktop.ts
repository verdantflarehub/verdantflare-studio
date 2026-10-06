import { MCPRequestError } from './mcp'

const serviceName = 'github.com/verdantflarehub/verdantflare-studio/internal/transport/desktop.Service'

export interface DesktopWorkspaceState {
  schema_version: number
  kind: string
  connection_alias: string
  project_id: string
  base_revision_id: string
  materialized_file_ids: string[]
}

export interface DesktopWorkspaceFileStatus {
  file_id: string
  path: string
  state: 'clean' | 'not_materialized' | 'missing' | 'modified' | 'conflict' | string
}

export interface DesktopWorkspaceEnvelope {
  status: number
  data: unknown
  request_id: string
}

export interface DesktopWorkspaceFileInput {
  file_id?: string
  path: string
  role: string
  mime: string
}

function isDesktopHost(): boolean {
  return new URLSearchParams(location.search).get('host') === 'desktop'
}

function decodeData(data: unknown): unknown {
  if (typeof data !== 'string') return data
  try {
    return JSON.parse(data) as unknown
  } catch {
    return data
  }
}

function messageFrom(data: unknown, fallback: string): string {
  if (data && typeof data === 'object') {
    const value = data as { message?: unknown; code?: unknown }
    if (typeof value.message === 'string' && value.message.trim()) return value.message
    if (typeof value.code === 'string' && value.code.trim()) return value.code
  }
  return fallback
}

async function callHost<T>(method: string, argument: Record<string, unknown>): Promise<T> {
  if (!isDesktopHost()) throw new MCPRequestError('桌面工作副本只能在 Studio 桌面端使用。', 400)
  const { Call } = await import('@wailsio/runtime')
  const result = await Call.ByName(`${serviceName}.${method}`, argument) as DesktopWorkspaceEnvelope
  const data = decodeData(result?.data)
  const status = typeof result?.status === 'number' ? result.status : 503
  if (status < 200 || status >= 300) {
    throw new MCPRequestError(messageFrom(data, `桌面工作副本请求失败（${status}）。`), status)
  }
  return data as T
}

export async function chooseWorkspaceDirectory(): Promise<string> {
  if (!isDesktopHost()) return ''
  const { Dialogs } = await import('@wailsio/runtime')
  const selected = await Dialogs.OpenFile({
    Title: '选择 Project 工作副本目录',
    Message: '选择一个已有项目目录，Studio 会在其中创建 .vf 工作副本目录。',
    ButtonText: '选择此目录',
    CanChooseDirectories: true,
    CanChooseFiles: false,
    CanCreateDirectories: true,
    AllowsMultipleSelection: false,
    ShowHiddenFiles: true
  })
  return Array.isArray(selected) ? (selected[0] || '') : selected
}

export function openWorkspace(projectID: string, directory: string, alias = 'station') {
  return callHost<{ workspace_id: string; state: DesktopWorkspaceState; manifest: unknown }>('WorkspaceOpen', {
    project_id: projectID,
    directory,
    connection_alias: alias
  })
}

export function fetchWorkspaceFile(workspaceID: string, fileID: string, maxBytes = 0) {
  return callHost<{ workspace_id: string; file_id: string }>('WorkspaceFetch', {
    workspace_id: workspaceID,
    file_id: fileID,
    max_bytes: maxBytes
  })
}

export function getWorkspaceStatus(workspaceID: string) {
  return callHost<{ workspace_id: string; files: DesktopWorkspaceFileStatus[] }>('WorkspaceStatus', {
    workspace_id: workspaceID
  })
}

export function saveWorkspaceTexts(workspaceID: string, fileIDs: string[]) {
  return callHost<{ workspace_id: string; result: unknown }>('WorkspaceSaveTexts', {
    workspace_id: workspaceID,
    file_ids: fileIDs
  })
}

export function saveWorkspaceFiles(workspaceID: string, files: DesktopWorkspaceFileInput[]) {
  return callHost<{ workspace_id: string; result: unknown }>('WorkspaceSaveFiles', {
    workspace_id: workspaceID,
    files
  })
}

export function resumeWorkspace(workspaceID: string) {
  return callHost<{ workspace_id: string; result: unknown }>('WorkspaceResume', {
    workspace_id: workspaceID
  })
}

export function closeWorkspace(workspaceID: string) {
  return callHost<{ workspace_id: string }>('WorkspaceClose', { workspace_id: workspaceID })
}

export { isDesktopHost }
