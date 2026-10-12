import assert from 'node:assert/strict'
import test from 'node:test'
import { readHostSession } from '../src/platform/session.ts'

test('missing or expired sessions require login without retrying denied requests', async () => {
  for (const status of [401, 403]) {
    let calls = 0
    assert.deepEqual(await readHostSession(async () => { calls++; return new Response('', { status }) }, () => true), { status: 'anonymous' })
    assert.equal(calls, 1)
  }
})
test('transient failures recover identity; sustained failures remain connection errors', async () => {
  let calls = 0
  const identity = { user_id: 'test-user', organization_id: 'test-org', station_id: 'test-station' }
  const recovered = await readHostSession(async () => {
    calls++
    if (calls === 1) throw new Error('network')
    return calls === 2 ? new Response('', { status: 503 }) : Response.json(identity)
  }, () => true)
  assert.deepEqual(recovered, { status: 'authenticated', identity }); assert.equal(calls, 3)
  for (const response of [() => new Response('', { status: 503 }), () => Response.json({}), () => new Response('invalid', { status: 200 })]) {
    calls = 0
    assert.deepEqual(await readHostSession(async () => { calls++; return response() }, () => true), { status: 'error' })
    assert.equal(calls, 3)
  }
})
test('late identity responses cannot replace a newer login or logout', async () => {
  let current = true
  const result = await readHostSession(async () => {
    current = false
    return Response.json({ user_id: 'previous-user' })
  }, () => current)
  assert.deepEqual(result, { status: 'superseded' })
})
