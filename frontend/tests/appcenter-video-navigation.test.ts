import assert from 'node:assert/strict'
import test from 'node:test'
import { videoPage, videoTaskLabel, videoTaskName, videoTasksLocation } from '../src/app-center/legacy-video-navigation.ts'
import { legacyTarget } from '../src/app-center/legacy.ts'

const route = '/market?video=' + encodeURIComponent('/dashboard/tasks/task-A')
const task = { video_task_id: 'task-A', project_id: '纸船', idempotency_key: 'shot-key', title: '河上漂流的纸船' }

test('task name matches the persisted title regardless of status', () => {
  for (const status of ['queued', 'running', 'succeeded', 'failed', 'cancelled']) {
    assert.equal(videoTaskName({ ...task, status }), '河上漂流的纸船')
  }
  assert.equal(videoTaskName({ project_id: '项目' }), '视频任务')
  assert.equal(videoTaskName({ idempotency_key: '镜头' }), '视频任务')
})

test('only the current authorized task response supplies a name', () => {
  assert.equal(videoTaskLabel(route, '/api/tasks/task-A', 200, task), '河上漂流的纸船')
  for (const path of ['/api/tasks/task-B', '/api/tasks/task-A/thumbnail', '/api/tasks/task-A/content', '/api/dashboard']) {
    assert.equal(videoTaskLabel(route, path, 200, task), null)
  }
  for (const status of [401, 403, 409, 410, 503]) assert.equal(videoTaskLabel(route, '/api/tasks/task-A', status, task), '任务不可用')
  assert.equal(videoTaskLabel(route, '/api/tasks/task-A', 404, task), '未找到任务')
  assert.equal(videoTaskLabel(route, '/api/tasks/task-A', 200, { ...task, video_task_id: 'task-B' }), '任务不可用')
  assert.equal(videoTaskLabel(route, '/api/tasks/task-A', 200, null), '任务不可用')
})

test('root routes and direct task routes retain the correct breadcrumb ancestry', () => {
  assert.deepEqual(videoPage(route), { path: '/dashboard/tasks/task-A', taskId: 'task-A', section: 'tasks' })
  for (const section of ['tasks', 'models', 'mcp']) {
    const root = '/market?video=' + encodeURIComponent('/dashboard#' + section)
    assert.equal(videoPage(root).section, section)
    assert.equal(videoTaskLabel(root, '/api/tasks/task-A', 200, task), null)
  }
  assert.equal(videoPage('/market?image=%2Fdashboard').path, '')
})

test('ancestor href retains the exact object binding when opened directly or in another tab', () => {
  const cardKey = 'installation:01a130a5-2000-7000-8000-000000000003'
  const target = legacyTarget(videoTasksLocation(cardKey))
  assert.equal(target?.cardKey, cardKey)
  assert.equal(videoPage(target!.route).section, 'tasks')
  assert.equal(videoPage(target!.route).taskId, '')
})
