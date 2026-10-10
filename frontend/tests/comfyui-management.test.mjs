import {test} from 'node:test';
import assert from 'node:assert/strict';
import {instanceCard,resourceMetric,readDirectory} from '../src/pages/comfyui-management.js';
const now=Date.now(),stamp=new Date(now).toISOString();
test('zero is a fresh measurement, unknown and stale never masquerade as zero',()=>{
  const metric={value:0,limit:8,quality:'fresh',sampled_at:stamp};
  assert.match(resourceMetric('CPU',metric,'cores',now),/0 核/);
  assert.match(resourceMetric('CPU',metric,'cores',now),/width:0%/);
  for(const m of [{...metric,quality:'unavailable'},{...metric,sampled_at:new Date(now-31000).toISOString()},{...metric,value:-1},{...metric,value:NaN}]){
    const result=resourceMetric('CPU',m,'cores',now);
    assert.match(result,/—/);assert.doesNotMatch(result,/style="width:/);
  }
});
test('directory rejects ambiguous or malformed instance identities',()=>{
  const item={alias:'comfyuiA',name:'A',allowed_actions:[]};
  assert.equal(readDirectory({schema_version:1,instances:[item]}).length,1);
  for(const instances of [[item,item],[{...item,alias:'../escape'}],[{...item,allowed_actions:null}]])assert.throws(()=>readDirectory({schema_version:1,instances}));
});
test('directory escapes untrusted display names and does not follow an editor URL',()=>{
  const html=instanceCard({alias:'comfyuiA',name:'<img src=x onerror=alert(1)>',observed_at:stamp,status:'running',allowed_actions:['open'],editor_url:'https://untrusted.invalid'},now);
  assert.doesNotMatch(html,/<img|https:\/\/untrusted|href=/);
  assert.match(html,/&lt;img/);assert.match(html,/disabled/);
  assert.match(html,/未采集/);assert.doesNotMatch(html,/0\.2|SDXL|林间/);
});
