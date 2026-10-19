import assert from 'node:assert/strict'
import test from 'node:test'
import { readFile } from 'node:fs/promises'

test('AppIcon handles brand:blender and brand:comfyui correctly', async () => {
  const iconSfc = await readFile(new URL('../src/app-center/AppIcon.vue', import.meta.url), 'utf8')
  assert.ok(iconSfc.includes("import blenderLogo from '../assets/blender.svg'"), 'must import blender logo')
  assert.ok(iconSfc.includes("import comfyuiLogo from '../assets/comfyui.svg'"), 'must import comfyui logo')
  assert.ok(iconSfc.includes("iconRef === 'brand:blender'"), 'must handle brand:blender')
  assert.ok(iconSfc.includes("iconRef === 'brand:comfyui'"), 'must handle brand:comfyui')

  const blenderSvg = await readFile(new URL('../src/assets/blender.svg', import.meta.url), 'utf8')
  assert.ok(blenderSvg.includes('<svg') && blenderSvg.includes('#E87D0D'), 'blender svg must be valid')

  const comfyuiSvg = await readFile(new URL('../src/assets/comfyui.svg', import.meta.url), 'utf8')
  assert.ok(comfyuiSvg.includes('<svg') && comfyuiSvg.includes('#172DD7') && comfyuiSvg.includes('#F0FF41'), 'comfyui svg must be valid')

  const channelIconSfc = await readFile(new URL('../src/app-center/ChannelBrandIcon.vue', import.meta.url), 'utf8')
  assert.ok(channelIconSfc.includes('color: var(--ink)'), 'ChannelBrandIcon must use var(--ink) instead of undefined var(--text)')
  assert.ok(!channelIconSfc.includes('var(--text)'), 'ChannelBrandIcon must not reference undefined var(--text)')
  assert.ok(iconSfc.includes("'brand:chatgpt'") && iconSfc.includes("'brand:grok'") && iconSfc.includes('is-mono'), 'AppIcon must apply monochrome ink class to ChatGPT and Grok')
  assert.ok(iconSfc.includes('#E2167E') && iconSfc.includes('#FE603C'), 'AppIcon must use MiniMax official gradient')
})
