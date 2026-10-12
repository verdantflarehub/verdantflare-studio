import test from 'node:test';
import assert from 'node:assert/strict';
import { responseClock } from '../src/pages/response-clock.js';
import { canOpen, canStart, canStop, canDestroy, instanceCard, detail, summary } from '../src/pages/blender-management.js';

const server = Date.parse('2026-10-12T03:00:00Z');
const stamp = new Date(server).toISOString();
const item = {alias:'blenderA',status:'running',observed_at:stamp,resources_observed_at:stamp,
  access:'edit',can_manage:true,state_version:4,control:{state:'idle'},allowed_actions:['view','open','stop'],
  allocated_gpu_count:1,resources:{cpu:{quality:'fresh',sampled_at:stamp,value:0,request:2},
    memory:{quality:'fresh',sampled_at:stamp,value:1024**3,request:4*1024**3,limit:8*1024**3}}};

test('all Blender projections and actions use response time despite local wall clock changes', t => {
  let elapsed = 250;
  const now = responseClock(new Date(server).toUTCString(), 100, () => elapsed);
  for (const skew of [-3600000,-10000,3600000]) {
    t.mock.method(Date, 'now', () => server + skew);
    assert.equal(canOpen(item, now()), true);
    assert.equal(canStop(item, now()), true);
    const stopped = {...item,status:'stopped',allowed_actions:['start','destroy']};
    assert.equal(canStart(stopped, now()), true);
    assert.equal(canDestroy(stopped, now()), true);
    assert.match(instanceCard(item, now()), /aria-label="打开/);
    assert.match(instanceCard(item, now()), /0 核/);
    assert.match(detail(item, 'overview', now()), /空闲 · 可连接/);
    assert.match(detail(item, 'resources', now()), /4 GiB \/ 8 GiB/);
    assert.match(summary([item], now()), /正在运行<\/dt><dd><strong>1/);
  }
  elapsed += 31000;
  assert.equal(canOpen(item, now()), false);
  assert.equal(canStop(item, now()), false);
  assert.match(instanceCard(item, now()), /状态过期/);
  assert.doesNotMatch(detail(item, 'resources', now()), /4 GiB \/ 8 GiB/);
  assert.match(summary([item], now()), /正在运行<\/dt><dd><strong>—/);
});

test('response time charges transport and body delay and rejects missing or malformed headers', () => {
  let elapsed = 30000;
  const now = responseClock(new Date(server).toUTCString(), 0, () => elapsed);
  assert.equal(canOpen(item, now()), false);
  for (const date of [null,undefined,'','bad',stamp]) assert.throws(() => responseClock(date,0));
  const earlier = responseClock(new Date(server-31000).toUTCString(),0,()=>0);
  assert.equal(canOpen(item, earlier()), false); // observation implausibly in the future
});

test('valid server time never overrides failed reads, capability, access or control constraints', () => {
  const now = responseClock(new Date(server).toUTCString(),0,()=>0);
  for (const change of [{observed_at:null},{status:'unknown'},{allowed_actions:['view']},
    {access:'read'},{control:{state:'busy'}},{control:{state:'unknown'}}]) {
    assert.equal(canOpen({...item,...change},now()),false);
  }
  assert.equal(canOpen(item,NaN),false);
  assert.doesNotMatch(instanceCard(item,NaN),/aria-label="打开/);
});
