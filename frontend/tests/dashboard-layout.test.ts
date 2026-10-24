import test from 'node:test'
import assert from 'node:assert/strict'
import { MIN_FILL_HEIGHT, createDashboardLayoutBridge, fillSurfaceHeight } from '../src/app-dashboard/layout.ts'

function setup(top = 24, viewport = 900) {
  const heights: Array<number | null> = []
  let fill = false, scrolled = 0, listener: (() => void) | null = null
  const state = { top, viewport }
  const bridge = createDashboardLayoutBridge({
    surfaceTop: () => state.top,
    viewportHeight: () => state.viewport,
    setHeight: h => { heights.push(h) },
    setFillClass: active => { fill = active },
    scrollToTop: () => { scrolled++ },
    subscribeResize: cb => { listener = cb; return () => { listener = null } },
  })
  return { bridge, heights, state, fill: () => fill, scrolled: () => scrolled, fire: () => listener?.(), listening: () => listener !== null }
}

test('fill height is the viewport below the surface with a usable minimum', () => {
  assert.equal(fillSurfaceHeight(1080, 24), 1056)
  assert.equal(fillSurfaceHeight(400, 24), MIN_FILL_HEIGHT)
  assert.equal(fillSurfaceHeight(Number.NaN, 0), MIN_FILL_HEIGHT)
})

test('document layout follows reported content height', () => {
  const s = setup(); s.bridge.show(); s.bridge.resize(1800)
  assert.deepEqual(s.heights, [1800]); assert.equal(s.fill(), false)
})

test('fill layout pins to viewport, ignores resize reports and tracks window resize', () => {
  const s = setup(); s.bridge.show(); s.bridge.resize(1800); s.bridge.setLayout('fill')
  assert.equal(s.fill(), true); assert.equal(s.scrolled(), 1); assert.equal(s.heights.at(-1), 876)
  s.bridge.resize(2400); assert.equal(s.heights.at(-1), 876)
  s.state.viewport = 1080; s.fire(); assert.equal(s.heights.at(-1), 1056)
})

test('switching back to document restores the last reported height', () => {
  const s = setup(); s.bridge.show(); s.bridge.resize(1500); s.bridge.setLayout('fill'); s.bridge.resize(1700)
  s.bridge.setLayout('document')
  assert.equal(s.fill(), false); assert.equal(s.heights.at(-1), 1700); assert.equal(s.listening(), false)
})

test('unknown modes are ignored and hidden surfaces release the fill class', () => {
  const s = setup(); s.bridge.show(); s.bridge.setLayout('fullscreen'); assert.equal(s.bridge.mode, 'document')
  s.bridge.setLayout('fill'); s.bridge.hide(); assert.equal(s.fill(), false); assert.equal(s.listening(), false)
  s.bridge.show(); assert.equal(s.fill(), true)
  s.bridge.dispose(); assert.equal(s.fill(), false); s.bridge.setLayout('document'); assert.equal(s.bridge.mode, 'fill')
})

test('layout requested before activation applies on first show', () => {
  const s = setup(); s.bridge.setLayout('fill'); assert.equal(s.fill(), false); assert.equal(s.heights.length, 0)
  s.bridge.show(); assert.equal(s.fill(), true); assert.equal(s.heights.at(-1), 876)
})
