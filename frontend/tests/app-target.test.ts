import test from 'node:test'
import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { appCenterReturnLocation, appCenterTargetLocation, appTargetLocation, decodeAppTarget, legacyAppTarget } from '../src/navigation/app-target.ts'
import { STUDIO_FRONTEND_VERSION, consumeBuildReloadTicket, hasServerBuildSkew } from '../src/platform/versionGuard.ts'
import type { AppCenterView } from '../src/app-center/schema.generated'
import type { NavigationEnvelope } from '../src/navigation/controller'

const now = Date.parse('2026-11-14T06:00:00Z')
const viewerCard = {
  cardKey: 'installation:01980000-0000-7000-8000-000000000099',
  appId: 'viewer',
  installationQuality: 'known',
  installation: { installationId: '01980000-0000-7000-8000-000000000099', revision: 1, appVersion: '0.1.3', state: 'installed', lastOperationId: null },
  entry: { entryId: 'dashboard-main', transport: 'dashboard', quality: 'known', availability: 'available', reasonCode: null, requiresServiceReady: false, minimumHostVersion: null },
  actions: [{ id: 'open', kind: 'open', enabled: true, reasonCode: null, entryId: 'dashboard-main', confirmation: 'none' }],
}
const appCenterSnapshot = () => ({
  generatedAt: new Date(now - 1000).toISOString(),
  validUntil: new Date(now + 5000).toISOString(),
  scope: { authorizationEpoch: 'epoch-1' },
  items: [structuredClone(viewerCard)],
}) as unknown as AppCenterView

test('cross-app navigation accepts product and path only and resolves an unpinned authorized install', () => {
  const target = decodeAppTarget({ appId: 'project', path: '/projects?revision=fixed' })
  const navigation = { mode: 'preinstalled', projection: { entries: [{ appId: 'project', installationId: '01980000-0000-7000-8000-000000000001', pinned: false }] } } as NavigationEnvelope
  assert.deepEqual(appTargetLocation(target, navigation), { path: '/apps/project/projects', query: { revision: 'fixed' } })
  for (const value of [{ appId: 'project', path: '//evil.invalid' }, { appId: 'project', path: '/', installationId: 'spoof' }, { appId: 'project', path: '/%2e%2e/other' }]) assert.throws(() => decodeAppTarget(value))
  assert.throws(() => appTargetLocation({ appId: 'missing', path: '/' }, navigation))
  assert.throws(() => appTargetLocation(target, { ...navigation, mode: 'legacy' }))
})

test('non-sidebar installed Dashboard apps resolve through the identity-bound AppCenter snapshot', () => {
  const viewerTarget = decodeAppTarget({ appId: 'viewer', path: '/?source=world&asset=01980000-0000-7000-8000-000000000010&version=01980000-0000-7000-8000-000000000011' })
  assert.deepEqual(appCenterTargetLocation(viewerTarget, appCenterSnapshot(), now, 'epoch-1'), {
    path: '/apps/viewer/',
    query: { source: 'world', asset: '01980000-0000-7000-8000-000000000010', version: '01980000-0000-7000-8000-000000000011' },
  })

  const tryonView = appCenterSnapshot()
  tryonView.items[0].appId = 'tryon'
  const tryonTarget = decodeAppTarget({ appId: 'tryon', path: '/?costume=01980000-0000-7000-8000-000000000020' })
  assert.deepEqual(appCenterTargetLocation(tryonTarget, tryonView, now, 'epoch-1'), {
    path: '/apps/tryon/',
    query: { costume: '01980000-0000-7000-8000-000000000020' },
  })

  assert.throws(() => appCenterTargetLocation(viewerTarget, appCenterSnapshot(), now + 10_000, 'epoch-1'), /APPCENTER_VIEW_EXPIRED/)
  assert.throws(() => appCenterTargetLocation(viewerTarget, appCenterSnapshot(), now, 'epoch-other'), /CONTEXT_CHANGED/)

  const uninstalled = appCenterSnapshot()
  uninstalled.items[0].installation = null
  assert.throws(() => appCenterTargetLocation(viewerTarget, uninstalled, now, 'epoch-1'), /APP_NAVIGATION_UNAVAILABLE/)

  const forbidden = appCenterSnapshot()
  forbidden.items[0].entry!.availability = 'denied'
  forbidden.items[0].entry!.reasonCode = 'PERMISSION_DENIED'
  forbidden.items[0].actions[0].enabled = false
  forbidden.items[0].actions[0].reasonCode = 'PERMISSION_DENIED'
  assert.throws(() => appCenterTargetLocation(viewerTarget, forbidden, now, 'epoch-1'), /PERMISSION_DENIED/)
})

test('compatibility bookmarks preserve fixed versions and route local copies through Project', () => {
  assert.deepEqual(legacyAppTarget('/projects?mode=world&asset=asset&version=fixed'), { appId: 'world', path: '/?asset=asset&version=fixed' })
  assert.deepEqual(legacyAppTarget('/projects?project=project&revision=fixed&tab=music'), { appId: 'project', path: '/projects?project=project&revision=fixed&tab=music' })
  assert.deepEqual(legacyAppTarget('/projects?view=files'), { appId: 'project', path: '/projects' })
  const project = '01980000-0000-7000-8000-000000000001'
  assert.deepEqual(legacyAppTarget(`/projects?view=files&project=${project}&revision=fixed`), { appId: 'project', path: `/workspace?project=${project}&revision=fixed` })
  assert.throws(() => legacyAppTarget('/projects?view=files&project=invalid'))
  assert.deepEqual(legacyAppTarget('/projects?mode=world&view=files&asset=asset&version=fixed'), { appId: 'world', path: '/?view=files&asset=asset&version=fixed' })
  assert.deepEqual(legacyAppTarget('/resources'), { appId: 'resources', path: '/' })
  assert.equal(legacyAppTarget('/station/proxies')?.appId, 'network')
  assert.throws(() => legacyAppTarget('/projects?mode=world&mode=projects'))
})

test('appcenter return navigation supports plain root and ?app=<appId> filter while rejecting invalid paths', () => {
  assert.equal(appCenterReturnLocation('/', '/market?view=installed'), '/market?view=installed')
  assert.deepEqual(appCenterReturnLocation('/?app=viewer', '/market'), {
    path: '/market',
    query: { view: 'all', q: 'viewer' },
  })
  for (const invalid of ['/viewer', '/?app=viewer&extra=1', '/?other=viewer', '/?app=', '/?app=../bad', '//evil.invalid']) {
    assert.throws(() => appCenterReturnLocation(invalid), /INVALID_PATH/)
  }
})

test('frontend version guard stays aligned with package.json and buildinfo and reloads at most once per target build', () => {
  const pkg = JSON.parse(readFileSync(new URL('../package.json', import.meta.url), 'utf8')) as { version: string }
  const buildInfo = readFileSync(new URL('../../internal/buildinfo/version.go', import.meta.url), 'utf8')
  assert.equal(STUDIO_FRONTEND_VERSION, pkg.version)
  assert.match(buildInfo, new RegExp(`Version\\s*=\\s*"${STUDIO_FRONTEND_VERSION.replaceAll('.', '\\.')}"`))

  assert.equal(hasServerBuildSkew(STUDIO_FRONTEND_VERSION, STUDIO_FRONTEND_VERSION), false)
  assert.equal(hasServerBuildSkew(STUDIO_FRONTEND_VERSION, '0.5.999'), true)
  assert.equal(hasServerBuildSkew(STUDIO_FRONTEND_VERSION, 'dev'), false)

  const store = new Map<string, string>()
  const storage = { getItem: (k: string) => store.get(k) ?? null, setItem: (k: string, v: string) => { store.set(k, v) } }
  assert.equal(consumeBuildReloadTicket('0.5.999', storage), true)
  assert.equal(consumeBuildReloadTicket('0.5.999', storage), false)
  assert.equal(consumeBuildReloadTicket('0.6.000', storage), true)
})
