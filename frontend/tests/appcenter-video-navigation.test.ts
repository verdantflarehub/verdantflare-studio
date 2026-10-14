import assert from 'node:assert/strict'
import test from 'node:test'
import { videoPage, videoTaskLabel, videoTaskName } from '../src/app-center/legacy-video-navigation.ts'

const route = '/market?video=' + encodeURIComponent('/dashboard/tasks/task-A')
const task = { video_task_id: 'task-A', project_id: '纸船', idempotency_key: '河上漂流' }

test('task name matches the existing list title regardless of status', () => {
  for (const status of ['queued', 'running', 'succeeded', 'failed', 'cancelled']) {
    assert.equal(videoTaskName({ ...task, status }), '纸船 / 河上漂流')
  }
  assert.equal(videoTaskName({ project_id: '项目' }), '未命名任务')
  assert.equal(videoTaskName({ idempotency_key: '镜头' }), '镜头')
})

test('only the current authorized task response supplies a name', () => {
  assert.equal(videoTaskLabel(route, '/api/tasks/task-A', 200, task), '纸船 / 河上漂流')
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
