import test from 'node:test';
import assert from 'node:assert/strict';
import { mergeCatalogWithLive } from '../src/pages/catalog.js';

test('ComfyUI and Blender share the creative tools category', () => {
  const apps = mergeCatalogWithLive([]);
  const group = id => apps.find(app => app.app_id === id).group_id;
  assert.equal(group('comfyui'), group('blender'));
  assert.notEqual(group('comfyui'), 'image');
});

test('legacy live categories cannot move ComfyUI or Blender out of creative tools', () => {
  const apps = mergeCatalogWithLive([
    { app_id: 'comfyui', group_id: 'image' },
    { app_id: 'blender', group_id: 'blender' },
    { app_id: 'image-mcp-server', group_id: 'image' },
  ]);
  const group = id => apps.find(app => app.app_id === id).group_id;
  assert.equal(group('comfyui'), 'desktop');
  assert.equal(group('blender'), 'desktop');
  assert.equal(group('image-mcp-server'), 'image');
});
