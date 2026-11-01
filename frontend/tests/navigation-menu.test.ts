import assert from 'node:assert/strict'
import test from 'node:test'
import { menuState } from '../src/navigation/menu.ts'
import { sameNavigationPath, selectedNavigationChild } from '../src/navigation/controller.ts'
import { navigationRouteNode } from '../src/app-dashboard/navigation-routes.ts'
import type { NavigationEntry } from '../src/navigation/controller.ts'

const child = (route: string, entryId = route) => ({ route, entryId, title: entryId, order: 0 })
test('menu capabilities and non-root parent selection depend only on declared routes', () => {
  const node = { route: '/overview', children: [child('/items')] }
  assert.deepEqual(menuState(node, '/overview?q=one', true), { hasChildren: true, navigable: true, selectedChild: undefined, parentCurrent: true })
  assert.equal(menuState(node, '/items/123', true).selectedChild, '/items')
  assert.equal(menuState(node, '/items/123', true).parentCurrent, false)
  assert.equal(menuState(node, '/overview', false).parentCurrent, false)
  assert.equal(menuState({ ...node, route: '' }, '/', true).navigable, false)
  assert.equal(menuState({ ...node, route: '/items' }, '/items', true).navigable, false)
  assert.equal(menuState({ route: '/overview' }, '/overview/123', true).parentCurrent, true)
})
test('details select the most specific segment-boundary match without business exceptions', () => {
  const nodes = [child('/'), child('/items'), child('/items/special')]
  assert.equal(selectedNavigationChild(nodes, '/items/special/123')?.route, '/items/special')
  assert.equal(selectedNavigationChild(nodes, '/items-other'), undefined)
  assert.equal(selectedNavigationChild(nodes, '/unknown'), undefined)
  assert.equal(selectedNavigationChild(nodes, '/')?.route, '/')
  assert.equal(selectedNavigationChild([child('/items'), child('/items?kind=a')], '/items/123?kind=a')?.route, '/items?kind=a')
  assert.equal(selectedNavigationChild([child('/items'), child('/items', 'duplicate')], '/items/123'), undefined)
  assert.equal(selectedNavigationChild(nodes, '/items?q=a&q=b'), undefined)
  assert.equal(selectedNavigationChild(nodes, '//external/items'), undefined)
})
test('arbitrary app identities and titles do not change capabilities or rewrite business paths', () => {
  for (const appId of ['sample-alpha', 'sample-beta']) {
    const entry = { appId, title: '来自服务端的名称', route: '/overview', children: [child('/monitor'), child('/?view=tasks')] } as NavigationEntry
    assert.deepEqual(navigationRouteNode(entry), { route: entry.route, children: entry.children })
    assert.equal(menuState(navigationRouteNode(entry), '/overview', true).parentCurrent, true)
    assert.equal(entry.title, '来自服务端的名称')
  }
  assert.equal(sameNavigationPath('/monitor', '/'), false)
  assert.equal(sameNavigationPath('/?view=tasks', '/tasks'), false)
  assert.equal(sameNavigationPath('/items?a=1&b=2', '/items?b=2&a=1'), true)
})
test('old declarations normalize only in their app route adapter without mutating source data', () => {
  const entry = { appId: 'resources', route: '/', children: [child('/?view=monitor'), child('/?view=tasks')] } as NavigationEntry
  const node = navigationRouteNode(entry)
  assert.equal(node.children?.[1].route, '/tasks')
  assert.equal(menuState(node, '/tasks/123', true).selectedChild, '/?view=tasks')
  assert.equal(menuState(node, '/', true).navigable, false)
  assert.equal(entry.children?.[1].route, '/?view=tasks')
})
