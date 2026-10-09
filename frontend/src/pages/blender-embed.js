export function mountBlender({listen,notice}) {
 const $=id=>document.getElementById(id),host=$('blenderHost'),frame=$('blenderFrame');
 let active=false,busy=false,session=null,items=[],epoch=0,timeout=null,scroll=0;
 const alias=()=>new URLSearchParams(location.hash.split('?')[1]||'').get('blender');
 const messages={INSTANCE_EDIT_LEASE_HELD:'实例正在由另一编辑会话使用，请先释放控制权。',GUI_EDIT_LEASE_HELD:'图形会话尚未释放，请稍后重试。',PERMISSION_DENIED:'当前账号无权访问此工程。',REVISION_CONFLICT:'工程已有新修订。本次检查点已保留，请核对后再保存。',GUI_LEASE_EXPIRED:'图形会话已过期，请重新连接。'};
 const status=(text,state='ready')=>{$('blenderMessage').textContent=text;host.dataset.state=state};
 async function request(path,body){
  const response=await fetch(path,{method:body===undefined?'GET':'POST',credentials:'same-origin',headers:body===undefined?{}:{'Content-Type':'application/json'},body:body===undefined?undefined:JSON.stringify(body)});
  const data=await response.json().catch(()=>({}));
  if(!response.ok)throw Error(messages[data.code]||data.error?.message||'暂时无法连接 Blender，请重试。');
  return data;
 }
 async function rpc(name,args,target){
  const data=await request('/mcp/'+target,{jsonrpc:'2.0',id:crypto.randomUUID(),method:'tools/call',params:{name,arguments:args}});
  if(data.error)throw Error(data.error.message);
  const value=JSON.parse(data.result.content[0].text);
  if(data.result.isError)throw Error(messages[value.code||value.result?.code]||'操作未完成，已保留当前工程。');
  return value;
 }
 function buttons(){
  const selected=items.find(i=>i.alias===alias());
  $('blenderConnect').disabled=busy||!selected||selected.access!=='edit'||!!session;
  $('blenderRelease').disabled=busy||!session;
  $('blenderSave').disabled=busy||!selected||selected.access!=='edit';
  $('blenderInstance').disabled=busy||!!session;
  $('blenderMCP').value=selected?location.origin+selected.mcp_path:'';
 }
 async function release(){
  clearTimeout(timeout);frame.removeAttribute('src');frame.hidden=true;
  const previous=session;session=null;buttons();
  if(previous)await request('/studio/apps/blender/'+previous.alias+'/close',{editing_session_id:previous.id});
 }
 async function connect(){
  if(busy||session)return;busy=true;buttons();const target=alias(),current=epoch;
  status('正在取得图形编辑控制权…','loading');
  try{
   const value=await request('/studio/apps/blender/'+target+'/open',{});
   if(current!==epoch){await request('/studio/apps/blender/'+target+'/close',{editing_session_id:value.editing_session_id});return}
   session={alias:target,id:value.editing_session_id};frame.hidden=false;
   frame.style.height=Math.max(240,Math.round(host.clientWidth*9/16))+'px';
   frame.src='/apps/blender/'+target+'/'+session.id+'/';
   status('正在连接 Blender 画面…','loading');
   timeout=setTimeout(()=>{if(host.dataset.state==='loading')status('画面连接超时。请释放控制权后重新连接。','error')},25000);
  }catch(error){status(error.message,'error')}finally{busy=false;buttons()}
 }
 async function save(){
  if(busy)return;busy=true;buttons();const target=alias();let edit=null;
  try{
   status('正在保存工程…','loading');await release();
   edit=await rpc('session.open',{project_id:items.find(i=>i.alias===target).project_id,mode:'edit'},target);
   const args={editing_session_id:edit.editing_session_id};
   const scene=await rpc('scene.get',args,target);
   const operation=await rpc('project.save',{...args,scene_version:scene.scene_version,idempotency_key:crypto.randomUUID()},target);
   let result;
   for(let count=0;count<90;count++){
    result=await rpc('operation.get',{operation_id:operation.operation_id},target);
    if(result.state==='completed')break;
    await new Promise(resolve=>setTimeout(resolve,1000));
   }
   if(result?.state!=='completed')throw Error('保存仍在进行，可通过 MCP 查询原操作：'+operation.operation_id);
   status('已保存到项目，新修订已生成。');notice('Blender 工程已保存');
  }catch(error){status(error.message,'error')}finally{
   if(edit)await rpc('session.close',{editing_session_id:edit.editing_session_id},target).catch(()=>{});
   busy=false;buttons();
  }
 }
 async function close(){
  if(!active)return;active=false;epoch++;
  release().catch(()=>{});host.hidden=true;host.style.display='none';
  if(!location.hash.includes('video=')&&!location.hash.includes('image=')){
   $('marketView').hidden=false;$('marketView').style.display='';document.querySelector('.main')?.classList.remove('embed-active');
   $('pageTitle').textContent='应用市场';$('pageSubtitle').textContent='为 Station 安装创作应用，管理模型与运行状态。';
   requestAnimationFrame(()=>window.scrollTo(0,scroll));
  }
 }
 async function sync(){
  const target=alias();if(!target){close();return}
  if(active){buttons();return}active=true;scroll=window.scrollY;const current=++epoch;
  host.hidden=false;host.style.display='flex';$('marketView').hidden=true;$('marketView').style.display='none';
  document.querySelector('.main')?.classList.add('embed-active');$('pageTitle').textContent='Blender';
  $('pageSubtitle').textContent='选择实例，用图形界面或专用 MCP 编辑三维工程。';status('正在读取可访问实例…','loading');
  try{
   const data=await request('/studio/apps/blender/instances');if(current!==epoch)return;items=data.instances;
   $('blenderInstance').replaceChildren();
   for(const item of items){const option=document.createElement('option');option.value=item.alias;option.textContent=item.alias+(item.access==='read'?' · 只读':'');$('blenderInstance').append(option)}
   if(!items.length){status('当前账号暂无可访问的 Blender 实例。','empty');buttons();return}
   if(!items.some(i=>i.alias===target))location.hash='/market?blender='+encodeURIComponent(items[0].alias);
   $('blenderInstance').value=alias();status('实例已选定。连接画面后取得图形编辑控制权；释放后可交给 MCP。');buttons();
  }catch(error){status(error.message,'error');buttons()}
 }
 listen($('blenderConnect'),'click',connect);
 listen($('blenderRelease'),'click',async()=>{if(busy)return;busy=true;buttons();try{await release();status('已释放图形控制权，可通过 MCP 编辑。')}catch(error){status(error.message,'error')}finally{busy=false;buttons()}});
 listen($('blenderSave'),'click',save);
 listen($('blenderCopy'),'click',()=>navigator.clipboard.writeText($('blenderMCP').value).then(()=>notice('已复制实例 MCP 地址')).catch(()=>notice('请选中地址手动复制')));
 listen($('blenderInstance'),'change',()=>{location.hash='/market?blender='+encodeURIComponent($('blenderInstance').value);buttons()});
 listen(window,'hashchange',sync);
 listen(window,'message',event=>{
  if(!active||event.source!==frame.contentWindow||event.origin!==location.origin||event.data?.channel!=='vf-blender')return;
  const data=event.data;
  if(data.type==='ready'){clearTimeout(timeout);status('图形编辑中。Shift + 滚轮可滚动 Studio 页面。');frame.contentWindow.postMessage({channel:'vf-studio',type:'theme',theme:document.body.dataset.theme},location.origin)}
  if(data.type==='error')status('画面连接已中断，请释放控制权后重新连接。','error');
  if(data.type==='resize'&&Number.isFinite(data.height))frame.style.height=Math.max(240,Math.min(data.height,2000))+'px';
  if(data.type==='wheel'&&Number.isFinite(data.deltaY))window.scrollBy(0,Math.max(-1000,Math.min(data.deltaY,1000)));
 });
 return {sync,close,open:()=>{location.hash='/market?blender=instances'},dispose:()=>{close();clearTimeout(timeout)}};
}
