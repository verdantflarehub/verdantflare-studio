import assert from 'node:assert/strict'
import test from 'node:test'
import { createThemePreference } from '../src/theme/preference.ts'
import type { ThemeMode, Theme } from '../src/theme/preference.ts'

test('old preferences, system changes and manual overrides share one actual theme', () => {
  let dark = false, saved: unknown = 'dark'
  const states: [ThemeMode, Theme, boolean][] = []
  const preference = createThemePreference({ read: () => saved, write: value => { saved = value }, dark: () => dark, apply: (...state) => states.push(state) })
  assert.deepEqual(states.at(-1), ['dark', 'dark', true])
  preference.select('system'); assert.deepEqual(states.at(-1), ['system', 'light', true])
  dark = true; preference.systemChanged(); assert.deepEqual(states.at(-1), ['system', 'dark', true])
  preference.select('light'); dark = false; preference.systemChanged(); assert.deepEqual(states.at(-1), ['light', 'light', true])
  preference.storageChanged('broken'); assert.deepEqual(states.at(-1), ['light', 'light', true])
  assert.equal(saved, 'light')
})
test('unavailable storage defaults to light and never blocks immediate appearance changes', () => {
  const states: [ThemeMode, Theme, boolean][] = []
  const preference = createThemePreference({ read: () => { throw Error('blocked') }, write: () => { throw Error('blocked') }, dark: () => false, apply: (...state) => states.push(state) })
  assert.deepEqual(states.at(-1), ['light', 'light', true])
  preference.select('dark'); assert.deepEqual(states.at(-1), ['dark', 'dark', false])
})
