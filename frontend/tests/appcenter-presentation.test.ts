import assert from 'node:assert/strict'
import test from 'node:test'
import { serviceReasonLabel } from '../src/app-center/presentation.ts'

test('successful and stopped observations do not become generic action-denial errors', () => {
  assert.equal(serviceReasonLabel('replicas_ready'), '服务副本已就绪')
  assert.equal(serviceReasonLabel('scaled_to_zero'), '服务副本已停止')
  assert.equal(serviceReasonLabel('unavailable'), '暂时无法取得服务观测')
  assert.equal(serviceReasonLabel('new-observation-code'), '观测说明待确认')
  assert.equal(serviceReasonLabel(null), '')
})
