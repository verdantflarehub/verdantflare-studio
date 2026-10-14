import test from 'node:test';
import assert from 'node:assert/strict';
import { canOpen, canStart, canStop, canDestroy, instanceCard, detail, summary } from '../src/pages/blender-management.js';

const server = Date.parse('2026-10-12T03:00:00Z');
const stamp = new Date(server).toISOString();
const item = {alias:'blenderA',status:'running',observed_at:stamp,resources_observed_at:stamp,
  access:'edit',can_manage:true,state_version:4,control:{state:'idle'},allowed_actions:['view','open','stop'],
  allocated_gpu_count:1,resources:{cpu:{quality:'fresh',sampled_at:stamp,value:0,request:2},
    memory:{quality:'fresh',sampled_at:stamp,value:1024**3,request:4*1024**3,limit:8*1024**3}}};

test('all Blender projections and actions are immune to local wall clock skew and elapsed client time', t => {
  for (const skew of [-3600000, -10000, 31000, 3600000]) {
    t.mock.method(Date, 'now', () => server + skew);
    assert.equal(canOpen(item), true);
    assert.equal(canStop(item), true);
    const stopped = {...item,status:'stopped',allowed_actions:['start','destroy']};
    assert.equal(canStart(stopped), true);
    assert.equal(canDestroy(stopped), true);
    assert.match(instanceCard(item), /aria-label="打开/);
    assert.doesNotMatch(instanceCard(item), /状态过期/);
    assert.match(instanceCard(item), /0 核/);
    assert.match(detail(item, 'overview'), /空闲 · 可连接/);
    assert.match(detail(item, 'resources'), /4 GiB \/ 8 GiB/);
    assert.match(summary([item]), /正在运行<\/dt><dd><strong>1/);
  }
});

test('missing observation or capability, access, or control constraints still gate actions', () => {
  for (const change of [{observed_at:null},{status:'unknown'},{allowed_actions:['view']},
    {access:'read'},{control:{state:'busy'}},{control:{state:'unknown'}}]) {
    assert.equal(canOpen({...item,...change}),false);
  }
});

