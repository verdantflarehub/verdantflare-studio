import test from 'node:test';
import assert from 'node:assert/strict';
import { canOpen, isFresh, instanceCard, summary, detail } from '../src/pages/blender-management.js';
import { formatResource, resourceState, resourceValue } from '../src/pages/resource-metrics.js';

const instance = (overrides = {}) => ({
  alias:'blenderA', name:'Blender A', status:'running', access:'edit',
  observed_at:new Date().toISOString(), control:{state:'idle'},
  allowed_actions:['view','open'], ...overrides,
});

test('telemetry preserves zero and formats CPU and memory with sample freshness', () => {
  const stamp = new Date().toISOString();
  const metric = {value:0,request:2,limit:8,quality:'fresh',sampled_at:stamp};
  assert.equal(resourceValue(metric,'cores'),'0 核');
  assert.equal(formatResource(4*1024**3,'bytes'),'4 GiB');
  assert.equal(formatResource(null,'bytes'),'—');
  for (const value of [null,undefined,-1,NaN,Infinity,'0']) assert.equal(resourceValue({...metric,value},'cores'),'—');
  const stale = {...metric,sampled_at:new Date(Date.now()-31000).toISOString()};
  assert.equal(resourceState(stale),'stale');
  assert.equal(resourceValue(stale,'cores'),'—');
  assert.equal(resourceValue({...metric,sampled_at:new Date(Date.now()+60000).toISOString()},'cores'),'—');
  const item = instance({resources_observed_at:stamp,resources:{cpu:metric,memory:{...metric,value:1024**3,request:4*1024**3,limit:16*1024**3}}});
  assert.match(instanceCard(item),/请求 2 核 \/ 4 GiB/);
  assert.match(detail(item,'resources'),/4 GiB \/ 16 GiB/);
  assert.match(detail(item,'resources'),/1 GiB/);
  assert.ok(!detail(instance({...item,resources_observed_at:null}),'resources').includes('4 GiB / 16 GiB'));
});

test('opening requires fresh observations, edit access, idle control and server capability', () => {
  assert.equal(canOpen(instance()), true);
  for (const changes of [
    {access:'read'}, {status:'unknown'}, {control:{state:'busy'}},
    {allowed_actions:['view']}, {observed_at:null},
    {observed_at:new Date(Date.now()-31000).toISOString()},
    {observed_at:new Date(Date.now()+60000).toISOString()},
  ]) assert.ok(!canOpen(instance(changes)), JSON.stringify(changes));
  assert.equal(isFresh(instance({observed_at:'bad'})), false);
});

test('summaries cannot invent GPU allocation or workspace counts from running instances', () => {
  const html = summary([instance()]);
  assert.equal((html.match(/<strong>—<\/strong>/g)||[]).length,2);
  assert.equal((summary([instance({status:'unknown'})]).match(/<strong>—<\/strong>/g)||[]).length,3);
  assert.ok(summary([]).includes('<strong>0</strong>'));
});

test('untrusted names are escaped and lifecycle commands remain disabled without backend support', () => {
  const html = instanceCard(instance({name:'<img src=x onerror=alert(1)>',can_manage:true}));
  assert.ok(html.includes('&lt;img'));
  assert.ok(!html.includes('<img src=x'));
  assert.ok(html.includes('disabled title="实例销毁暂未开放"'));
  assert.ok(html.includes('disabled title="实例生命周期管理暂未开放"'));
  assert.ok(!instanceCard(instance()).includes('>停止</button>'));
  assert.ok(!html.includes('已保存到项目'));
});
