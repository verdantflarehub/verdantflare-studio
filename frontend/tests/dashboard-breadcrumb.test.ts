import test from 'node:test'
import assert from 'node:assert/strict'
import { dashboardAppId, dashboardLocation, dashboardOwnsBreadcrumb } from '../src/app-dashboard/routes.ts'

test('World root stays in World while the prior Project dashboard is cached or its replacement fails', () => {
  const project = { appId: 'project', installationId: '00000000-0000-7000-8000-000000000001' }
  for (const previous of [undefined, project, { ...project, appId: 'resources' }]) {
    const appId = dashboardAppId('world', previous)
    assert.equal(dashboardLocation(appId!).path, '/apps/world/')
    assert.equal(dashboardOwnsBreadcrumb(appId, '/music'), true)
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
  for (const path of [
    '/image',
    '/image?project=id&revision=fixed',
    '/character',
    '/video?q=abc',
    '/ai_short_drama?project=id&revision=fixed&tab=character&material=hero',
    '/ai_short_drama/00000000-0000-7000-8000-000000000001/materials/location/bathroom/lights_on',
    '/ai_short_drama/00000000-0000-7000-8000-000000000001/materials/location/courtyard/review?revision=00000000-0000-7000-8000-000000000002',
    '/projects?category=character',
    '/projects?category=video&q=abc',
  ]) {
    assert.equal(dashboardOwnsBreadcrumb('project', path), true, `expected project to own breadcrumb for ${path}`)
  }
})

test('other applications, fallback routes and invalid navigation keep host breadcrumb', () => {
  for (const app of [undefined, 'resources', 'network', 'viewer', 'tryon', 'world']) assert.equal(dashboardOwnsBreadcrumb(app, '/projects'), false)
  for (const path of [undefined, '/', '/workspace', '/create', '//outside.invalid/projects', '/projects#fragment', '/projects-extra', ['projects']]) assert.equal(dashboardOwnsBreadcrumb('project', path), false)
})

test('World owns complete trails across all valid domain, gallery, collection, review and result sub-routes', () => {
  for (const path of [
    '/',
    '/?asset=id&version=fixed',
    '/item/hero?asset=id&version=fixed',
    '/character',
    '/character/item/hero?asset=id&version=fixed',
    '/pose',
    '/pose?sys=standard&v=diag',
    '/pose/asset/0199b501-0000-7000-8000-000000000001?version=0199b501-0000-7000-8000-000000000002&v=diag',
    '/costume',
    '/costume?tab=item&kind=dress',
    '/costume/asset/0199b501-0000-7000-8000-000000000003?version=0199b501-0000-7000-8000-000000000004',
    '/scene?asset=id&version=fixed',
    '/scene/01900000-0000-7000-8000-000000000001/versions/01900000-0000-7000-8000-000000000002/review',
    '/sound',
    '/music',
    '/shot',
    '/script',
    '/knowledge',
    '/collections',
    '/collections?trash=1',
    '/collections/01900000-0000-7000-8000-000000000001?revision=01900000-0000-7000-8000-000000000002',
    '/learn',
    '/learn?task=id',
    '/learn/item/prompt.123?task=id',
    '/publish?project=01900000-0000-7000-8000-000000000001',
  ]) {
    assert.equal(dashboardOwnsBreadcrumb('world', path), true, `expected world to own breadcrumb for ${path}`)
  }
  for (const path of ['/item', '/learn-extra', '/pose-extra', '/costume-extra', '/publish/extra', '//outside.invalid/learn', '/learn#fragment']) {
    assert.equal(dashboardOwnsBreadcrumb('world', path), false, `expected world not to own invalid path ${path}`)
  }
})
