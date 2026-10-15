import test from 'node:test'
import assert from 'node:assert/strict'
import { nativeTaskLocation, nativeTaskTarget } from '../src/navigation/native-task-target.ts'
import type { AppCenterView } from '../src/app-center/schema.generated'

const now = Date.parse('2026-10-15T12:00:00Z')
const target = { appId: 'video-mcp', path: '/dashboard/tasks/video_task_original' }
const card = { appId: target.appId, cardKey: 'installation:01a130a5-2000-7000-8000-000000000003',
  entry: { transport: 'legacy', entryId: 'video-iframe-v1', quality: 'known', availability: 'available' },
  actions: [{ kind: 'open', enabled: true, entryId: 'video-iframe-v1' }] }
const snapshot = () => ({ generatedAt: new Date(now - 1000).toISOString(), validUntil: new Date(now + 1000).toISOString(),
  scope: { authorizationEpoch: 'epoch' }, items: [structuredClone(card)] }) as unknown as AppCenterView

test('native task navigation retains the exact task and selects one currently authorized binding', () => {
  assert.equal(nativeTaskLocation(target, snapshot(), now, 'epoch'), '/apps/video-mcp/?video=%2Fdashboard%2Ftasks%2Fvideo_task_original&legacyCard=' + encodeURIComponent(card.cardKey))
  assert.equal(nativeTaskTarget({ appId: 'project', path: '/' }), null)
  assert.equal(nativeTaskTarget({ appId: 'image-mcp', path: '/dashboard/tasks/imgtask_original' })?.adapterId, 'image-iframe-v1')
  assert.equal(nativeTaskTarget({ appId: 'image-mcp-server', path: '/dashboard/tasks/imgtask_original' })?.adapterId, 'image-iframe-v1')
  for (const path of ['/dashboard', '/dashboard/tasks/id?retry=true', '/dashboard/tasks/id#tasks', '/dashboard/tasks/../models', '/dashboard/tasks/%69d', '/dashboard/tasks/id/other']) {
    assert.throws(() => nativeTaskTarget({ ...target, path }), /INVALID_NAVIGATION/)
  }
})

test('stale, revoked, unavailable, wrong-product and ambiguous targets fail before navigation', () => {
  assert.throws(() => nativeTaskLocation(target, snapshot(), now + 1000, 'epoch'), /VIEW_EXPIRED/)
  assert.throws(() => nativeTaskLocation(target, snapshot(), now, 'other'), /CONTEXT_CHANGED/)
  for (const mutate of [
    (v: AppCenterView) => { v.items = [] },
    (v: AppCenterView) => { v.items[0].actions[0].enabled = false },
    (v: AppCenterView) => { v.items[0].appId = 'other' },
    (v: AppCenterView) => { v.items.push(structuredClone(v.items[0])) },
    (v: AppCenterView) => { v.items[0].entry!.availability = 'unavailable' },
  ]) { const view = snapshot(); mutate(view); assert.throws(() => nativeTaskLocation(target, view, now, 'epoch')) }
})
