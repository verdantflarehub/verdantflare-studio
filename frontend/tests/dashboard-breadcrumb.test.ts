import test from 'node:test'
import assert from 'node:assert/strict'
import { dashboardOwnsBreadcrumb } from '../src/app-dashboard/routes.ts'

test('Project lists, searches and fixed objects own one complete breadcrumb', () => {
  for (const path of ['/character', '/video?q=abc', '/ai_short_drama?project=id&revision=fixed&tab=character&material=hero', '/projects?category=character', '/projects?category=video&q=abc']) {
    assert.equal(dashboardOwnsBreadcrumb('project', path), true)
  }
})

test('other applications, fallback routes and invalid navigation keep host breadcrumb', () => {
  for (const app of [undefined, 'resources', 'world']) assert.equal(dashboardOwnsBreadcrumb(app, '/projects'), false)
  for (const path of [undefined, '/', '/workspace', '/create', '//outside.invalid/projects', '/projects#fragment', '/projects-extra', ['projects']]) assert.equal(dashboardOwnsBreadcrumb('project', path), false)
})
