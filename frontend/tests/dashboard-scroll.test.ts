import test from 'node:test'
import assert from 'node:assert/strict'
import { createDashboardScrollBridge } from '../src/app-dashboard/scroll.ts'

function setup() {
  let top = 0, rendered = true, listener = () => {}, serial = 0
  const frames = new Map<number, () => void>(), writes: number[] = []
  const bridge = createDashboardScrollBridge({ read: () => top, write: value => { top = value; writes.push(value) }, rendered: () => rendered,
    subscribe: callback => { listener = callback; return () => { listener = () => {} } },
    schedule: callback => { frames.set(++serial, callback); return serial }, cancel: id => { frames.delete(id) } })
  return { bridge, frames, writes, render: (value: boolean) => { rendered = value },
    scroll: (value: number) => { top = value; listener() }, flush: () => { const callbacks = [...frames.values()]; frames.clear(); callbacks.forEach(f => f()) } }
}
test('temporary parent hiding does not replace saved scroll with browser clamping', () => {
  const s = setup();s.bridge.show();s.flush();s.scroll(300)
  s.render(false);s.scroll(0);s.bridge.hide()
  assert.equal(s.bridge.get(),300)
  s.bridge.restore(120);s.render(true);s.bridge.show();s.flush()
  assert.equal(s.bridge.get(),120);assert.equal(s.writes.at(-1),120)
})
test('hidden navigation restores the latest requested page after activation', () => {
  const s = setup();s.bridge.restore(200);s.bridge.restore(400)
  assert.equal(s.frames.size,0);s.bridge.show();s.flush()
  assert.deepEqual(s.writes,[400])
  s.bridge.hide();s.scroll(900);assert.equal(s.bridge.get(),400)
})
test('invalid targets and disposal cannot scroll the outer document', () => {
  const s = setup();s.bridge.show();s.flush();s.writes.length=0
  for (const top of [-1,NaN,Infinity,1_000_001])s.bridge.restore(top)
  assert.equal(s.frames.size,0)
  s.bridge.restore(200);s.bridge.dispose();s.scroll(300);s.bridge.show();s.bridge.restore(500);s.flush()
  assert.equal(s.frames.size,0);assert.equal(s.bridge.get(),0);assert.deepEqual(s.writes,[])
})
