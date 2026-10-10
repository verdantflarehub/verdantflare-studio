import test from 'node:test';
import assert from 'node:assert/strict';
import { randomUUID } from 'node:crypto';
import { createBlenderStarter } from '../src/pages/blender-start.js';
import { canStart, canStop, canDestroy, instanceCard, instanceName } from '../src/pages/blender-management.js';

function fixture() {
  const item = {alias:'blenderDynamic',instance_id:randomUUID(),state_version:6,status:'stopped',access:'edit',
    can_manage:true,allowed_actions:['view','start'],observed_at:new Date().toISOString()};
  const actor = {can_manage:true,station_id:randomUUID(),organization_id:randomUUID(),user_id:randomUUID()};
  const map = new Map(), calls = [];
  const storage = {getItem:k=>map.get(k)??null,setItem:(k,v)=>map.set(k,v),removeItem:k=>map.delete(k)};
  const result = {alias:item.alias,instance_id:item.instance_id,operation_id:randomUUID(),action:'start',state:'running'};
  const request = async (path,body) => {calls.push({path,body});return body ? result : actor;};
  return {item,actor,map,calls,storage,result,request};
}

test('start requires fresh server permission and a stopped, versioned instance',()=>{
  const f = fixture();
  assert.ok(canStart(f.item));
  assert.match(instanceCard(f.item),/data-blender-start="blenderDynamic"/);
  for (const change of [{can_manage:false},{access:'read'},{status:'running'},{allowed_actions:['view']},
    {observed_at:null},{management_operation_id:randomUUID()},{state_version:0},{state_version:1.5}]) {
    assert.ok(!canStart({...f.item,...change}));
    assert.doesNotMatch(instanceCard({...f.item,...change}),/data-blender-start=/);
  }
});

test('lost responses survive reload and reuse the original version and idempotency key',async()=>{
  const f = fixture(); let lost = true;
  const request = async (path,body)=>{const result=await f.request(path,body);if(body&&lost){lost=false;throw Error('network');}return result;};
  const first = createBlenderStarter({...f,request,newID:randomUUID});
  await assert.rejects(first(f.item),/network/);
  assert.equal(f.map.size,1);
  const reloaded = createBlenderStarter({...f,request,newID:randomUUID});
  assert.equal(await reloaded({...f.item,state_version:9}),f.result);
  const posts = f.calls.filter(call=>call.body);
  assert.deepEqual(posts[0].body,posts[1].body);
  assert.equal(posts[1].body.expected_version,6);
  assert.equal(f.map.size,0);
});

test('unwritable or corrupt browser storage prevents command submission',async()=>{
  const f=fixture();
  const start=createBlenderStarter({...f,newID:randomUUID,storage:{...f.storage,setItem(){throw Error('blocked');}}});
  await assert.rejects(start(f.item),/blocked/);
  assert.equal(f.calls.filter(call=>call.body).length,0);
  const key='vf-blender-start:'+ [f.actor.station_id,f.actor.organization_id,f.actor.user_id,f.item.instance_id].join(':');
  f.map.set(key,'bad-json');
  await assert.rejects(createBlenderStarter({...f,newID:randomUUID})(f.item),/恢复记录/);
  assert.equal(f.calls.filter(call=>call.body).length,0);
});

test('concurrent clicks submit once and actor scopes never reuse another users pending request',async()=>{
  const f=fixture();
  const start=createBlenderStarter({...f,newID:randomUUID});
  const results=await Promise.all([start(f.item),start(f.item)]);
  assert.equal(results[1],null);
  assert.equal(f.calls.filter(call=>call.body).length,1);
  const other='vf-blender-start:'+ [f.actor.station_id,f.actor.organization_id,randomUUID(),f.item.instance_id].join(':');
  f.map.set(other,'other-user-draft');
  await start(f.item);
  assert.equal(f.map.get(other),'other-user-draft');
});

test('mismatched acknowledgements preserve the request until its outcome is known',async()=>{
  const f=fixture(); f.result.instance_id=randomUUID();
  await assert.rejects(createBlenderStarter({...f,newID:randomUUID})(f.item),/尚未确认/);
  assert.equal(f.map.size,1);
});

test('stop requires current permission and idle control, preserves one request after response loss',async()=>{
  const f=fixture();Object.assign(f.item,{status:'running',control:{state:'idle'},allowed_actions:['view','stop']});f.result.action='stop';
  assert.ok(canStop(f.item));assert.match(instanceCard(f.item),/data-blender-stop=/);
  for(const change of [{status:'stopped'},{control:{state:'busy'}},{control:{state:'unknown'}},{can_manage:false},{allowed_actions:['view']},{management_operation_id:randomUUID()},{observed_at:null}]){
    assert.ok(!canStop({...f.item,...change}));assert.doesNotMatch(instanceCard({...f.item,...change}),/data-blender-stop=/);
  }
  let lost=true;
  const request=async(path,body)=>{const result=await f.request(path,body);if(body&&lost){lost=false;throw Error('lost');}return result;};
  await assert.rejects(createBlenderStarter({...f,action:'stop',request,newID:randomUUID})(f.item),/lost/);
  const pending=[...f.map.keys()][0];assert.match(pending,/^vf-blender-stop:/);
  assert.equal(await createBlenderStarter({...f,action:'stop',request,newID:randomUUID})({...f.item,state_version:10}),f.result);
  const posts=f.calls.filter(c=>c.body);assert.deepEqual(posts[0],posts[1]);assert.match(posts[0].path,/\/stop$/);
  assert.equal(f.map.size,0);
});

test('destroy requires exact name and preserves a scoped pending command',async()=>{
  const f=fixture();f.item.allowed_actions=['view','start','destroy'];f.result.action='destroy';
  assert.ok(canDestroy(f.item));assert.match(instanceCard(f.item),/data-blender-destroy=/);
  assert.ok(!canDestroy({...f.item,status:'running'}));
  const destroy=createBlenderStarter({...f,action:'destroy',newID:randomUUID});
  await assert.rejects(destroy(f.item,'wrong'),/完整的实例名称/);assert.equal(f.calls.length,0);
  let lost=true;
  const request=async(path,body)=>{const result=await f.request(path,body);if(body&&lost){lost=false;throw Error('lost');}return result;};
  const name=instanceName(f.item);
  await assert.rejects(createBlenderStarter({...f,action:'destroy',request,newID:randomUUID})(f.item,name),/lost/);
  assert.match([...f.map.keys()][0],/^vf-blender-destroy:/);
  await createBlenderStarter({...f,action:'destroy',request,newID:randomUUID})(f.item,name);
  const posts=f.calls.filter(c=>c.body);assert.deepEqual(posts[0],posts[1]);assert.equal(posts[0].body.confirm_name,name);
  assert.equal(f.map.size,0);
});
