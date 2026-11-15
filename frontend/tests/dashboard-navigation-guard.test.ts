import test from 'node:test'
import assert from 'node:assert/strict'
import { inspectNavigation } from '../src/app-dashboard/navigation-guard.ts'

test('browser back while permission inspection is pending cancels late navigation', async () => {
  let location = '/list?filter=unassessed', resolve!: () => void, committed = false
  const inspection = new Promise<void>(r => { resolve = r })
  const pending = inspectNavigation(new AbortController().signal, () => location, () => inspection).then(() => { committed = true })
  location = '/list?filter=excluded'
  resolve()
  await assert.rejects(pending, /NAVIGATION_SUPERSEDED/)
  assert.equal(committed, false)
})

test('stable navigation still requires successful permission inspection', async () => {
  let checked = false
  await inspectNavigation(new AbortController().signal, () => '/list', async () => { checked = true })
  assert.equal(checked, true)
  await assert.rejects(inspectNavigation(new AbortController().signal, () => '/list', async () => { throw Error('DENIED') }), /DENIED/)
})

test('cancelled inspection cannot complete a pending navigation', async () => {
  const controller = new AbortController()
  await assert.rejects(inspectNavigation(controller.signal, () => '/list', async () => { controller.abort() }), { name: 'AbortError' })
  await assert.rejects(inspectNavigation(controller.signal, () => '/list', async () => assert.fail('must not inspect')), { name: 'AbortError' })
})
