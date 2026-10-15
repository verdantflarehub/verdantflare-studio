import assert from 'node:assert/strict'
import test from 'node:test'
import { stationHealth } from '../src/platform/stationHealth.ts'
test('Core readiness alone never claims the whole Station is ready', () => {
  assert.deepEqual(stationHealth({ station_id: 'a', status: 'Degraded', components: { core: 'Ready' } }, 'a'), { name: null, status: '未就绪' })
  assert.deepEqual(stationHealth({ station_id: 'a', cluster_name: '创作站', status: 'Ready' }, 'a'), { name: '创作站', status: '就绪' })
  assert.deepEqual(stationHealth({ station_id: 'b', cluster_name: '其他站', status: 'Ready' }, 'a'), { name: null, status: null })
  assert.deepEqual(stationHealth(null, 'a'), { name: null, status: null })
})
