import test from 'node:test'
import assert from 'node:assert/strict'
import { appTargetLocation, decodeAppTarget, legacyAppTarget } from '../src/navigation/app-target.ts'
import type { NavigationEnvelope } from '../src/navigation/controller'

test('cross-app navigation accepts product and path only and resolves an unpinned authorized install', () => {
  const target = decodeAppTarget({ appId: 'project', path: '/projects?revision=fixed' })
  const navigation = { mode: 'preinstalled', projection: { entries: [{ appId: 'project', installationId: '01980000-0000-7000-8000-000000000001', pinned: false }] } } as NavigationEnvelope
  assert.deepEqual(appTargetLocation(target, navigation), { path: '/apps/01980000-0000-7000-8000-000000000001/dashboard', query: { path: target.path } })
  for (const value of [{ appId: 'project', path: '//evil.invalid' }, { appId: 'project', path: '/', installationId: 'spoof' }, { appId: 'project', path: '/%2e%2e/other' }]) assert.throws(() => decodeAppTarget(value))
  assert.throws(() => appTargetLocation({ appId: 'missing', path: '/' }, navigation))
  assert.throws(() => appTargetLocation(target, { ...navigation, mode: 'legacy' }))
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
  assert.deepEqual(legacyAppTarget('/station/proxies'), { appId: 'network', path: '/' })
  assert.throws(() => legacyAppTarget('/projects?mode=world&mode=projects'))
})
