import { mountVideo } from './video-embed'
import { request } from '../platform/client'
import { brandMarks } from './brands'
// Temporary adapter for approved Market interactions; migrate into Vue components next.
export function mountMarket() {
const controller = new AbortController();
const listen = (target, event, handler) => target.addEventListener(event, handler, {signal:controller.signal});

const $=id=>document.getElementById(id), labels={image:'Image',music:'Music',video:'Video'};
const statusText={ready:'已就绪',stopped:'已关闭',stopping:'关闭中',starting:'启动中',not_installed:'未安装',degraded:'运行异常',unknown:'状态未知'};
const esc=x=>String(x??'').replace(/[&<>"']/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
let identity={};
const operations = new Map();
let operationPolling=false;
const terminal=op=>op && ['succeeded','failed'].includes(op.status);
const phases={checking:'检查中',downloading:'下载中',verifying:'校验中',installing:'安装中',starting:'启动中',warming:'预热中',stopping:'关闭中',deleting:'删除中',completed:'完成'};
let apps=[],group='all',tab='all',selected=null,connected=false,mode='market',refreshing=false;
const installed=a=>!['not_installed','unknown'].includes(a.deployment.state);
const active=a=>['starting','stopping'].includes(a.deployment.state);
function brand(a){return {vf:'VF 官方',minimax:'MiniMax',github:'GitHub'}[a.brand]||'GitHub'}
function icon(a){return `<span class="app-brand-mark" data-logo="${esc(a.brand)}">${brandMarks[a.brand]||brandMarks.github}</span>`}
function badge(a){return `<span class="status ${a.deployment.state==='ready'?'running':a.deployment.state==='degraded'?'failed':''}"><i class="dot ${a.deployment.state==='ready'?'on':''}"></i>${statusText[a.deployment.state]||'状态未知'}</span>`}
function notice(message){$('toast').textContent=message;$('toast').classList.add('show');setTimeout(()=>$('toast').classList.remove('show'),3500)}
function login(){video.close();connected=false;identity={};operations.clear();apps=[];render();if(!$('loginDialog').open)$('loginDialog').showModal()}
async function api(path,options={}){
 const r=await request({path,method:options.method||'GET',body:options.body?JSON.parse(options.body):undefined});
 const data=r.status===204?{}:await r.json();
 if(r.status===401 && path!=='login'){login();const error=Error('请先登录');error.unauthorized=true;throw error}
 if(!r.ok){const error=Error(({ACCOUNT_LOCKED:'登录失败次数过多，请稍后再试',UNAUTHENTICATED:'用户名或密码不正确',SERVICE_UNAVAILABLE:'Station 暂时无法连接'}[data.code])||data.message||'请求失败');error.status=r.status;throw error;}
 return data;
}
const video=mountVideo({listen,api,notice,login});
function render(){
 const q=$('search').value.trim().toLowerCase();
 const shown=apps.filter(a=>(group==='all'||a.group_id===group)&&(tab==='all'||tab==='installed'&&installed(a)||tab==='active'&&active(a))&&[a.display_name,...a.models].join(' ').toLowerCase().includes(q));
 const count=apps.filter(installed).length,running=apps.filter(a=>a.deployment.state==='ready').length;
 $('allCount').textContent=connected?apps.length:'—';$('installedCount').textContent=connected?count:'—';$('activeCount').textContent=connected?apps.filter(active).length:'—';
 $('navInstalledCount').textContent=connected?count:'—';$('railInstalled').textContent=connected?count:'—';$('railRunning').textContent=connected?running:'—';
 $('resultCount').textContent=connected?`${shown.length} 个应用`:'等待连接';
 $('statusSummary').textContent=connected?`已安装 ${count} · 就绪 ${running}`:'状态未知';
 document.querySelector('.statusbar>span').innerHTML=`<i class="dot"></i> ${connected?'Station Core 已连接':'Station 未连接'}`;
 document.querySelector('.strip-right .demo').textContent=connected?'实时应用目录':'未连接 Station';
 document.querySelector('.rail-heading .pill').textContent=connected?'已连接':'未连接';
 document.querySelector('.station p').textContent=connected?'Core 已连接 · 真实应用目录':'等待连接 Station';
 document.querySelectorAll('[data-group]').forEach(b=>b.classList.toggle('active',b.dataset.group===group));
 document.querySelectorAll('[data-tab]').forEach(b=>b.classList.toggle('active',b.dataset.tab===tab));
 $('groupNav').innerHTML=Object.keys(labels).map(g=>`<button class="group-nav" data-group="${g}">${labels[g]}<span class="count">${apps.filter(a=>a.group_id===g).length}</span></button>`).join('');
 $('catalog').innerHTML=!connected?'<div class="empty">登录后查看 Station 中的真实应用。</div>':!shown.length?'<div class="empty">没有符合条件的应用</div>':Object.keys(labels).map(g=>{
  const list=shown.filter(a=>a.group_id===g);if(!list.length)return '';
  return `<section><div class="section-title"><h2>${labels[g]}</h2><small>${list.length} 个应用</small><span class="line"></span></div><div class="grid">${list.map(a=>`<article class="app-card" data-card="${esc(a.app_id)}"><div class="card-top"><div class="app-icon ${g}">${icon(a)}</div><div><button class="app-title" data-detail="${esc(a.app_id)}">${esc(a.display_name)}</button><small class="version brand-version">${brand(a)} · v${esc(a.version)}</small></div></div><p class="card-desc">${mode==='models'?(a.models.length?esc(a.models.join(' · ')):'无需本地模型'):a.models.length?'模型：'+esc(a.models.join(' · ')):'提供应用入口，无本地模型依赖'}</p><div class="tags">${badge(a)}<span class="tag">${a.model_status==='not_required'?'无需模型':'模型状态未知'}</span></div><div class="card-foot"><span>${a.deployment.ready_replicas??'—'}/${a.deployment.desired_replicas??'—'} 就绪副本</span>${a.app_id==='video-mcp-server'&&a.deployment.state==='ready'?'<button class="btn primary" data-open-video="true">打开</button>':''}<button class="btn" data-detail="${esc(a.app_id)}">查看详情</button></div></article>`).join('')}</div></section>`;
 }).join('');
 $('mcpNav').innerHTML=apps.filter(a=>a.brand==='vf').map(a=>`<button class="nav-link" data-detail="${esc(a.app_id)}">${esc(a.display_name)} ${badge(a)}</button>`).join('');
 $('railActivity').textContent=`${[...operations.values()].filter(x=>!terminal(x.op)).length} 个待完成操作`;$('railModels').textContent='状态未知';
 if(document.getElementById('videoHost').hidden)$('pageTitle').textContent=mode==='models'?'模型市场':'应用市场';
 if(document.getElementById('videoHost').hidden)$('pageSubtitle').textContent=mode==='models'?'查看应用声明的模型依赖':'Image · Music · Video，统一查看本地应用';
}
async function refresh(){
 if(refreshing)return;refreshing=true;$('reset').disabled=true;
 try{const data=await api('apps');apps=data.items;const nextIdentity=await api('me');const changed=identity.user_id!==nextIdentity.user_id||identity.organization_id!==nextIdentity.organization_id||identity.station_id!==nextIdentity.station_id;identity=nextIdentity;if(changed){operations.clear();restoreOperations()}connected=true;render();if(selected){const a=apps.find(x=>x.app_id===selected);if(a)drawDetail(a)}}
 catch(e){connected=false;apps=[];render();if(selected)closeDetail();if(!e.unauthorized)notice(e.message)}
 finally{refreshing=false;$('reset').disabled=false}
}
function saveOperations(){
 // Store command IDs, never credentials; scope persisted recovery to this identity.
 try{sessionStorage.setItem('studio.operations',JSON.stringify({user:identity.user_id,organization:identity.organization_id,station:identity.station_id,items:[...operations]}))}catch{}
}
function restoreOperations(){
 try{const saved=JSON.parse(sessionStorage.getItem('studio.operations')||'null');if(saved&&saved.user===identity.user_id&&saved.organization===identity.organization_id&&saved.station===identity.station_id){operations.clear();for(const [id,entry] of saved.items)operations.set(id,{...entry,sending:false})}}catch{}
}
async function submit(entry){
 entry.sending=true;
 try{entry.op=await api('commands',{method:'POST',body:JSON.stringify(entry.command)});entry.error=''}
 catch(e){entry.error=e.message;entry.rejected=!!e.status&&e.status<500&&e.status!==401;throw e}
 finally{entry.sending=false;saveOperations()}
}
async function command(action){
 if(!selected)return;const a=apps.find(x=>x.app_id===selected);if(!a)return;
 let entry=operations.get(a.app_id);
 if(entry?.sending)return;
 if(entry&&!terminal(entry.op)&&!entry.rejected){notice('已有待完成操作，请查询原操作');return}
 entry={command:{request_id:crypto.randomUUID(),idempotency_key:'studio-'+crypto.randomUUID(),organization_id:identity.organization_id,station_id:identity.station_id,app_id:a.app_id,app_version:a.version,action}};
 operations.set(a.app_id,entry);saveOperations();drawDetail(a);
 try{await submit(entry);notice(`${a.display_name}：命令已记录，等待执行`)}catch(e){notice(e.message)}
 if(selected===a.app_id)drawDetail(a);
}
async function pollOperations(){
 if(operationPolling||!connected)return;operationPolling=true;
 try{for(const entry of operations.values()){
  if(entry.sending||entry.rejected||terminal(entry.op))continue;
  try{if(entry.op){entry.op=await api('operations/'+entry.op.operation_id);entry.error=''}else{await submit(entry)}}catch(e){entry.error=e.message;if(e.unauthorized)break}
 }saveOperations();if(selected){const a=apps.find(x=>x.app_id===selected);if(a)drawDetail(a)}}finally{operationPolling=false}
}
function drawDetail(a){
 const entry=operations.get(a.app_id),op=entry?.op,busy=entry&&!entry.rejected&&!terminal(op);
 $('detailHead').innerHTML=`<div class="card-top"><div class="app-icon ${esc(a.group_id)}">${icon(a)}</div><div><h2 id="detailTitle">${esc(a.display_name)}</h2><p>${brand(a)} · v${esc(a.version)} ${badge(a)}</p></div></div>`;
 $('actions').innerHTML=[['安装','install'],['启动','start'],['关闭','stop'],['重启','restart'],['删除','delete']].map(([label,action])=>{
  const unavailable=['stop','restart','delete'].includes(action);
  const disabled=a.deployment.state==='unknown'||busy||unavailable||(action==='install'?a.deployment.state!=='not_installed':!installed(a));
  return `<button class="btn" data-command="${action}" ${disabled?'disabled':''} ${unavailable?'title="暂不可用：尚未支持安全结束进行中的任务"':''}>${label}</button>`;
 }).join('');
 $('progress').innerHTML=entry?`<p>${op?op.status==='accepted'?'命令已记录，等待执行':op.status==='failed'?'操作失败':op.status==='succeeded'?'操作完成':esc(phases[op.phase]||op.phase):entry.rejected?'命令被拒绝':'响应待确认，正在使用原幂等键恢复'}</p>${op?`<p>阶段：${esc(phases[op.phase]||op.phase)} · 操作 ID：${esc(op.operation_id)}</p><p>下载：${op.download.downloaded_bytes} / ${op.download.total_bytes??'未知'} 字节 · ${op.download.percent==null?'进度未知':esc(op.download.percent)+'%'}</p>`:''}${op?.error?`<p role="alert">${esc(op.error.message)} (${esc(op.error.code)})</p>`:''}${entry.error?`<p role="alert">${esc(entry.error)}</p>`:''}`:'';
 $('detailContent').innerHTML=`<p>命令状态与部署就绪分别展示；下载完成不代表安装或模型预热完成。</p><h3>部署</h3><p>关闭、重启和删除暂不可用：尚未支持安全结束进行中的任务。</p><p>状态：${statusText[a.deployment.state]}</p><p>就绪副本：${a.deployment.ready_replicas??'—'} / ${a.deployment.desired_replicas??'—'}</p><p>观测时间：${new Date(a.deployment.observed_at).toLocaleString()}</p><h3>模型</h3><p>${a.models.length?esc(a.models.join(' · ')):'无需本地模型'}</p><p>${a.model_status==='unknown'?'模型下载与预热状态尚未接入':'不需要模型预热'}</p><details><summary>部署信息</summary><p>${esc(a.namespace)} / ${esc(a.workload_name)}</p><h4>当前镜像</h4>${a.deployment.images.map(i=>`<p style="overflow-wrap:anywhere">${esc(i.component)}：${esc(i.image)}</p>`).join('')}<h4>清单镜像</h4>${a.images.map(i=>`<p style="overflow-wrap:anywhere">${esc(i.image)}</p>`).join('')}</details>`;
 document.querySelectorAll('[data-detail-tab]').forEach(b=>{b.hidden=b.dataset.detailTab!=='overview'});
}
async function openDetail(id){try{const data=await api('apps/'+encodeURIComponent(id));selected=id;drawDetail(data.app);$('overlay').classList.add('open');$('overlay').style.display='flex';$('closeDrawer').focus()}catch(e){notice(e.message)}}
function closeDetail(){selected=null;$('overlay').classList.remove('open');$('overlay').style.display='none'}
$('loginDialog').addEventListener('cancel',e=>e.preventDefault());
$('loginForm').addEventListener('submit',async e=>{e.preventDefault();const form=e.currentTarget,b=form.querySelector('button');b.disabled=true;$('loginError').textContent='';try{await api('login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:form.elements.username.value,password:form.elements.password.value})});form.elements.password.value='';$('toast').classList.remove('show');$('loginDialog').close();await refresh();await video.sync()}catch(err){$('loginError').textContent=err.message}finally{b.disabled=false}});
$('reset').textContent='刷新状态';$('reset').onclick=refresh;
$('theme').onclick=()=>{const light=document.body.dataset.theme!=='light';document.body.dataset.theme=light?'light':'dark';$('theme').textContent=light?'深色主题':'浅色主题'};
const logout=document.createElement('button');logout.className='btn';logout.textContent='退出登录';logout.onclick=async()=>{try{await api('logout',{method:'POST'});closeDetail();operations.clear();sessionStorage.removeItem('studio.operations');login()}catch(e){notice(e.message)}};$('reset').after(logout);
$('vfHeaderLogo').src=brandMarks.vf.match(/src="([^"]+)"/)[1];
$('newTask').disabled=true;$('newTask').title='创作任务开发中';
$('sideCollapse').onclick=()=>{document.body.classList.toggle('sidebar-collapsed');$('sideCollapse').setAttribute('aria-expanded',String(!document.body.classList.contains('sidebar-collapsed')))};
$('search').addEventListener('input',render);$('closeDrawer').onclick=closeDetail;$('overlay').onclick=e=>{if(e.target===$('overlay'))closeDetail()};
listen(document,'keydown',e=>{if(e.key==='Escape'&&selected)closeDetail()});
listen(document,'click',e=>{const b=e.target.closest('button,a');if(!b||b.disabled)return;if(b.dataset.openVideo){e.preventDefault();const app=apps.find(a=>a.app_id==='video-mcp-server');if(app?.deployment.state==='ready')video.open();else notice('Video 暂未就绪');return}if(b.dataset.placeholder){e.preventDefault();notice('该功能正在开发');return}if(b.dataset.command){command(b.dataset.command);return}if(b.dataset.detail){openDetail(b.dataset.detail);return}if(b.dataset.group){group=b.dataset.group;render();return}if(b.dataset.tab){if(!document.getElementById('videoHost').hidden)location.hash='/market';tab=b.dataset.tab;render();return}if(b.dataset.nav){if(!document.getElementById('videoHost').hidden)location.hash='/market';mode=b.dataset.nav==='models'?'models':'market';tab=b.dataset.nav==='installed'?'installed':'all';group='all';render()}});
$('overlay').style.display='none';render();refresh().then(()=>video.sync());const polling=setInterval(()=>{if(!$('loginDialog').open)refresh()},30000);

const operationTimer=setInterval(pollOperations,2000);
return () => { video.dispose();clearInterval(operationTimer);clearInterval(polling); controller.abort(); };
}
