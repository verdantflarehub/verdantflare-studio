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
})
