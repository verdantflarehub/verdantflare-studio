// Opaque-origin iframe; all data access stays on the authenticated host.
export function mountVideo({listen, api, notice, login}) {
 const host=document.getElementById('videoHost'), frame=document.getElementById('videoFrame');
 const market=document.getElementById('marketContent');
 let active=false, generation=0, pending=new Set(), viewState=null;
 const pathPattern=/^\/(api\/(dashboard|mcp\/status|models|models\/[a-z0-9-]+\/instances(\/[a-zA-Z0-9._-]+(\/gpus\/[a-zA-Z0-9._:-]+)?)?|tasks\/[a-zA-Z0-9_-]+(\/thumbnail|\/result)?|tasks|artifacts\/import)|artifacts\/[a-zA-Z0-9_-]+\/content)$/;
 const theme=()=>document.body.dataset.theme==='light'?'light':'dark';
 function route(){
  const params=new URLSearchParams(location.hash.split('?')[1]||'');
  return /^\/dashboard(?:\/tasks\/[a-zA-Z0-9_-]+)?(?:#(?:tasks|models|mcp))?$/.test(params.get('video')||'')?params.get('video'):null;
 }
 function close(){generation++;active=false;pending.clear();frame.removeAttribute('src');host.hidden=true;market.hidden=false;document.getElementById('pageTitle').textContent='应用市场'}
 async function sync(){
  const path=route();if(!path){close();return}
  const epoch=++generation;pending.clear();
  try {await api('me');if(epoch!==generation)return;active=true;market.hidden=true;host.hidden=false;document.getElementById('pageTitle').textContent='Video MCP';document.getElementById('pageSubtitle').textContent='视频任务、模型与 MCP 服务工作区';const [url,hash]=path.split('#');frame.src='/studio/apps/video'+url+'?embed=1&theme='+theme()+'&view='+epoch+(hash?'#'+hash:'');document.getElementById('videoMessage').textContent='正在连接 Video…'} catch(e){close();notice(e.message)}
 }
 function navigate(path){location.hash='/market?video='+encodeURIComponent(path)}
 listen(window,'hashchange',sync);
 listen(window,'message',async e=>{
  if(!active||e.source!==frame.contentWindow||e.origin!=='null'||!e.data||e.data.channel!=='vf-video')return;
  const d=e.data;if(d.view!==String(generation))return;
  if(d.type==='ready'){document.getElementById('videoMessage').textContent='';frame.contentWindow.postMessage({channel:'vf-studio',type:'theme',theme:theme()},'*');frame.contentWindow.postMessage({channel:'vf-studio',type:'restore',state:viewState},'*');return}
  if(d.type==='navigate'&&typeof d.path==='string'&&/^\/dashboard(?:\/tasks\/[a-zA-Z0-9_-]+)?(?:#(?:tasks|models|mcp))?$/.test(d.path)){if(d.state && typeof d.state==='object')viewState=d.state;navigate(d.path);return}
  if(d.type!=='request'||!Number.isSafeInteger(d.id)||typeof d.path!=='string'||d.path.length>2048||pending.has(d.id)||pending.size>=32)return;
  const epoch=generation,method=d.method||'GET';
  const reply=value=>{if(epoch===generation&&active)frame.contentWindow.postMessage({channel:'vf-studio',type:'response',id:d.id,...value},'*')};
  if(!['GET','POST'].includes(method)||!pathPattern.test(d.path.split('?')[0])||d.path.split('?')[0].includes('%')||d.path.includes('#')||(d.body!=null&&(typeof d.body!=='string'||d.body.length>1048576))){reply({status:403,body:new ArrayBuffer(0)});return}
  pending.add(d.id);
  try {
   const r=await fetch('/studio/apps/video'+d.path,{method,body:method==='POST'?d.body:undefined,headers:method==='POST'?{'Content-Type':'application/json'}:{},credentials:'same-origin',cache:'no-store',signal:AbortSignal.timeout(90000)});
   if(r.status===401){close();login();return}
   reply({status:r.status,contentType:r.headers.get('Content-Type')||'application/octet-stream',body:await r.arrayBuffer()});
  }catch{reply({status:502,body:new ArrayBuffer(0)})}finally{if(epoch===generation)pending.delete(d.id)}
 });
 const observer=new MutationObserver(()=>{if(active)frame.contentWindow?.postMessage({channel:'vf-studio',type:'theme',theme:theme()},'*')});observer.observe(document.body,{attributes:true,attributeFilter:['data-theme']});
 document.getElementById('videoBack').onclick=()=>{location.hash='/market'};
 document.getElementById('videoRetry').onclick=sync;
 return {open:()=>navigate('/dashboard#tasks'),sync,close,dispose:()=>{close();observer.disconnect()}};
}
