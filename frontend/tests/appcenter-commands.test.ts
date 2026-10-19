import assert from 'node:assert/strict'
import test from 'node:test'
import { readFileSync } from 'node:fs'
import { validCommandCore, validCommandSubmit, validCommandPrepare } from '../src/app-center/schema.generated.ts'

const cases = JSON.parse(readFileSync(new URL('./fixtures/appcenter-commands.generated.json', import.meta.url), 'utf8')) as { name: string; valid: boolean; input: unknown }[]
for (const c of cases) test(`command contract: ${c.name}`, () => {
  assert.equal(validCommandCore(c.input), c.valid)
  assert.equal(validCommandSubmit({ schemaVersion: 1, prepared: c.input }), c.valid)
})
test('prepare accepts only the closed lifecycle selection', () => {
  const input = { schemaVersion: 1, requestId: 'trace', idempotencyKey: 'command-key', cardKey: 'catalog:test-app', actionId: 'install', expectedBindingRevision: 'sha256:' + 'a'.repeat(64) }
  assert.equal(validCommandPrepare(input), true)
  assert.equal(validCommandPrepare({ ...input, actionId: 'upgrade' }), true)
  for (const actionId of ['open', 'adopt', 'rollback', 'arbitrary']) assert.equal(validCommandPrepare({ ...input, actionId }), false)
  for (const key of Object.keys(input)) {
    const missing = { ...input } as Record<string, unknown>
    delete missing[key]
    assert.equal(validCommandPrepare(missing), false)
  }
  assert.equal(validCommandPrepare({ ...input, endpoint: 'https://other.invalid' }), false)
})
