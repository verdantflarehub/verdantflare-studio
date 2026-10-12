import test from 'node:test';
import assert from 'node:assert/strict';
import { CATALOG_GROUPS, mergeCatalogWithLive, selectCatalogGroups } from '../src/pages/catalog.js';

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

const systemApps = [
  { app_id: 'project', display_name: '项目管理', description: 'Project revisions', group_id: 'system', capabilities: ['project.commit'], deployment: { state: 'ready' } },
  { app_id: 'world', display_name: '数字世界', description: 'World assets', group_id: 'system', capabilities: ['world.publish'], deployment: { state: 'not_installed' } },
  { app_id: 'network', display_name: '网络出口', description: 'Network proxies', group_id: 'system', capabilities: ['network.probe'], deployment: { state: 'starting' } },
  { app_id: 'resources', display_name: '资源管理', description: 'Station resources', group_id: 'system', capabilities: ['resources.read'], deployment: { state: 'unknown' } },
];

test('live system apps appear in the same ordered sections used by the catalog renderer', () => {
  const apps = mergeCatalogWithLive(systemApps);
  const sections = selectCatalogGroups(apps);
  assert.deepEqual(sections.map(section => section.id), CATALOG_GROUPS.map(group => group.id));
  assert.deepEqual(sections.find(section => section.id === 'system').apps.map(app => app.app_id), ['project', 'world', 'network', 'resources']);
  assert.equal(sections.reduce((count, section) => count + section.apps.length, 0), apps.length);
  assert.deepEqual(sections.find(section => section.id === 'desktop').apps.map(app => app.app_id), ['comfyui', 'blender']);
});

test('system filtering composes with name, description, model and capability searches', () => {
  const apps = mergeCatalogWithLive(systemApps);
  const ids = options => selectCatalogGroups(apps, options).flatMap(section => section.apps.map(app => app.app_id));
  assert.deepEqual(ids({ group: 'system' }), ['project', 'world', 'network', 'resources']);
  assert.deepEqual(ids({ group: 'system', query: '资源管理' }), ['resources']);
  assert.deepEqual(ids({ query: ' WORLD ASSETS ' }), ['world']);
  assert.deepEqual(ids({ group: 'system', query: 'NETWORK.PROBE' }), ['network']);
  assert.deepEqual(ids({ group: 'system', query: 'Whisper' }), []);
  assert.deepEqual(ids({ group: 'music', query: 'Whisper' }), ['music-lyrics-aligner-api']);
});

test('system sections obey the installed and in-progress tab predicates', () => {
  const apps = mergeCatalogWithLive(systemApps);
  const installed = app => !['not_installed', 'absent', 'unknown'].includes(app.deployment.state);
  const active = app => app.deployment.state === 'starting';
  const ids = matches => selectCatalogGroups(apps, { group: 'system', matches }).flatMap(section => section.apps.map(app => app.app_id));
  assert.deepEqual(ids(installed), ['project', 'network']);
  assert.deepEqual(ids(active), ['network']);
  assert.deepEqual(selectCatalogGroups(apps, { group: 'system', query: 'World', matches: installed }), []);
});

test('system cards are sourced from live catalog data and retain its order', () => {
  assert.deepEqual(selectCatalogGroups(mergeCatalogWithLive([]), { group: 'system' }), []);
  const apps = mergeCatalogWithLive([systemApps[3], systemApps[0]]);
  const section = selectCatalogGroups(apps, { group: 'system' });
  assert.deepEqual(section[0].apps.map(app => app.app_id), ['resources', 'project']);
  assert.equal(section[0].apps[0].deployment.state, 'unknown');
  assert.deepEqual(selectCatalogGroups(apps, { query: 'no matching application' }), []);
});
