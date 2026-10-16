// Implements docs/design/contracts/app-dashboard-v1.md in the design repository.
// Public interfaces only: no Vue, Pinia, Wails or application business imports.
export type Unsubscribe = () => void
export interface MediaPreviewInput { bytes: ArrayBuffer; mime: string; title: string }
export type Lifecycle = 'active' | 'hidden' | 'dispose'
export type Theme = 'light' | 'dark'
export interface AppRoute { path: string; revision: number }
export type Json = null | boolean | number | string | Json[] | { [key: string]: Json }

export interface AppContext {
  userId: string
  organizationId: string
  stationId: string
  projectId: string | null
  appId: string
  installationId: string
  contextRevision: string
}

export interface DashboardLaunch {
  dashboardSessionId: string
  installationId: string
  appId: string
  appVersion: string
  artifactDigest: string
  adapter: 'wujie' | 'isolated-iframe'
  entryUrl: string
  sdkVersion: string
  grantedCapabilities: string[]
  contextRevision: string
  expiresAt: string
}

export interface AppRequest {
  path: string
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'
  body?: Json
}

export interface RequestOptions { signal?: AbortSignal }
export interface AppResponse { status: number; data: Json; requestId: string }
export interface SelectedFile { handle: string; name: string; size: number; mime: string }
export interface FileSelection { purpose: string; multiple?: boolean }
export interface InstanceEntry { instanceId: string; entryId: string }
export interface Navigation { path: string; target: 'app' | 'appcenter' }

export interface AppSDK {
  readonly version: string
  readonly context: {
    get(): Readonly<AppContext>
    subscribe(listener: (context: Readonly<AppContext>) => void): Unsubscribe
  }
  readonly theme: {
    get(): Theme
    subscribe(listener: (theme: Theme) => void): Unsubscribe
  }
  readonly navigation: {
    get(): Readonly<AppRoute>
    subscribe(listener: (route: Readonly<AppRoute>) => void): Unsubscribe
    request(input: Navigation): Promise<void>
  }
  readonly files: { select(input: FileSelection): Promise<readonly SelectedFile[]> }
  readonly capabilities: { call(name: string, input: Json, options?: RequestOptions): Promise<Json> }
  readonly app: { request(input: AppRequest, options?: RequestOptions): Promise<AppResponse> }
  readonly instances: { open(input: InstanceEntry): Promise<void> }
  readonly media: { preview(input: MediaPreviewInput): Promise<void>; close(): void }
  readonly lifecycle: { subscribe(listener: (event: Lifecycle) => void): Unsubscribe }
}

// Host-only transport. This object must never be injected into the Dashboard.
export interface SDKAdapters {
  previewMedia?(input: MediaPreviewInput, signal: AbortSignal): Promise<void>
  closeMedia?(): void
  now?: () => number
  navigate(input: Navigation, signal: AbortSignal): Promise<void>
  selectFiles(input: FileSelection, signal: AbortSignal): Promise<readonly SelectedFile[]>
  releaseFiles?(): void
  callCapability(name: string, input: Json, signal: AbortSignal): Promise<Json>
  requestApp(input: AppRequest, signal: AbortSignal): Promise<AppResponse>
  openInstance(input: InstanceEntry, signal: AbortSignal): Promise<void>
  hasUserActivation(): boolean
  reportListenerError(error: unknown): void
}
