import test from 'node:test'
import assert from 'node:assert/strict'
import { dashboardAppId, dashboardLocation, dashboardOwnsBreadcrumb } from '../src/app-dashboard/routes.ts'

test('World root stays in World while the prior Project dashboard is cached or its replacement fails', () => {
  const project = { appId: 'project', installationId: '00000000-0000-7000-8000-000000000001' }
  for (const previous of [undefined, project, { ...project, appId: 'resources' }]) {
    const appId = dashboardAppId('world', previous)
    assert.equal(dashboardLocation(appId!).path, '/apps/world/')
    assert.equal(dashboardOwnsBreadcrumb(appId, '/music'), false)
  }
  assert.equal(dashboardLocation(dashboardAppId('project', { ...project, appId: 'world' })!).path, '/apps/project/')
})

test('legacy installation links wait for their own resolved identity rather than another cached app', () => {
  const world = { appId: 'world', installationId: '00000000-0000-7000-8000-000000000002' }
  const project = { appId: 'project', installationId: '00000000-0000-7000-8000-000000000001' }
  assert.equal(dashboardAppId(world.installationId), undefined)
  assert.equal(dashboardAppId(world.installationId, project), undefined)
  assert.equal(dashboardLocation(dashboardAppId(world.installationId, world)!).path, '/apps/world/')
  for (const target of ['', '../world', 'https://outside.invalid', '/apps/world']) assert.equal(dashboardAppId(target, world), undefined)
  assert.equal(dashboardAppId(world.installationId, { ...world, appId: '../world' }), undefined)
})

test('Project lists, searches and fixed objects own one complete breadcrumb', () => {
  for (const path of ['/image', '/image?project=id&revision=fixed', '/character', '/video?q=abc', '/ai_short_drama?project=id&revision=fixed&tab=character&material=hero', '/projects?category=character', '/projects?category=video&q=abc']) {
    assert.equal(dashboardOwnsBreadcrumb('project', path), true)
  }
})

test('other applications, fallback routes and invalid navigation keep host breadcrumb', () => {
  for (const app of [undefined, 'resources', 'world']) assert.equal(dashboardOwnsBreadcrumb(app, '/projects'), false)
  for (const path of [undefined, '/', '/workspace', '/create', '//outside.invalid/projects', '/projects#fragment', '/projects-extra', ['projects']]) assert.equal(dashboardOwnsBreadcrumb('project', path), false)
})
