import { formatResource } from './resource-metrics.js';

const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const validID = value => typeof value === 'string' && /^[0-9a-f]{8}-[0-9a-f]{4}-[47][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/.test(value);
function commitID() {
  const hex = Date.now().toString(16).padStart(12,'0') + crypto.randomUUID().replaceAll('-','').slice(12);
  return `${hex.slice(0,8)}-${hex.slice(8,12)}-7${hex.slice(13,16)}-${hex.slice(16,20)}-${hex.slice(20)}`;
}
const base = '/studio/apps/blender/';
const sourceText = source => source?.kind === 'restore' ? '恢复项目中的 Blender 工程' : source?.kind === 'empty' ? '空白 Blender 场景' : '选择项目后核验来源';

export function mountBlenderCreation({ host, request, listen, onAccepted }) {
  let epoch = 0, selection = 0, options, projects = [], nextCursor = '', scope = '', model = {}, source = null;
  let busy = false, checking = false, error = '', pending = null;
  const key = value => 'vf-blender-create:' + value;
  const save = (value, targetScope = scope) => {
    try { sessionStorage.setItem(key(targetScope), JSON.stringify(value)); }
    catch { throw Error('浏览器无法保留恢复记录，请检查存储设置后重试。'); }
  };
  const read = () => {
    const raw = sessionStorage.getItem(key(scope));
    if (raw === null) return null;
    try {
      const value = JSON.parse(raw);
      if (value && value.scope === scope && typeof value.idempotency_key === 'string' && typeof value.name === 'string') return value;
    } catch { /* An unreadable pending request must not silently become a new create. */ }
    throw Error('本次创建记录无法读取，请先在实例列表核对结果。');
  };
  function render() {
    if (!options) return;
    const locked = !!pending || busy;
    const profile = options.profiles.find(p => p.profile_id === model.profile_id);
    const projectOptions = projects.map(p => `<option value="${esc(p.project_id)}" ${p.project_id === model.project_id ? 'selected' : ''}>${esc(p.name)}</option>`).join('');
    const currentMissing = model.project_id && !projects.some(p => p.project_id === model.project_id);
    host.innerHTML = `<div class="page-heading"><h1>准备一个独立工作区</h1><p>项目保存你的成果，实例提供 Blender 的运行环境。</p></div>
      <form id="blenderCreateForm"><div class="form-layout"><div class="panel"><fieldset ${locked?'disabled':''}>
      <section class="form-section"><div class="step-heading"><span class="step-number">01</span><h2>名称与项目</h2></div>
      <label class="field"><span>实例名称</span><input name="name" value="${esc(model.name)}" maxlength="40" required placeholder="例如：产品建模工作区" autocomplete="off"><small>1–40 个字符；当前组织内名称不重复。</small></label>
      <label class="field"><span>关联项目</span><select name="project" required><option value="">选择可编辑的项目</option>${projectOptions}${currentMissing?`<option selected value="${esc(model.project_id)}">已选择项目 · ${esc(model.project_id)}</option>`:''}<option value="new" ${model.new_project?'selected':''}>新建项目 · 空白场景</option></select><small>实例固定关联该项目，保存时生成新的项目修订。</small></label>
      ${nextCursor?'<button class="btn" type="button" data-create-action="more">加载更多项目</button>':''}
      ${model.new_project?`<label class="field"><span>新项目名称</span><input name="project_name" maxlength="256" value="${esc(model.project_name)}" required placeholder="例如：产品空间"><small>创建实例失败时，已经建立的项目仍会保留。</small></label>`:
      `<label class="field"><span>来源修订</span><input name="revision" value="${esc(model.source_revision_id)}" placeholder="选择项目后自动填入，也可指定旧修订" ${model.project_id?'':'disabled'} required><small>固定使用此修订，后续项目更新不会替换工作副本。</small></label><button class="btn" data-create-action="verify" type="button" ${model.project_id && !checking?'':'disabled'}>核验来源修订</button>`}
      <div class="source-summary" role="status"><span class="kicker">初始工程</span><p>${checking?'正在核验修订与工程…':esc(sourceText(source))}</p>${source?.kind==='restore'?`<small>${esc(formatResource(source.size,'bytes'))} · 已核验工程元数据</small>`:''}</div>
      </section><section class="form-section"><div class="step-heading"><span class="step-number">02</span><h2>运行位置与规格</h2></div>
      <label class="field"><span>Station</span><input value="当前 Station" disabled></label><label class="field"><span>运行规格</span><select name="profile" required>${options.profiles.map(p=>`<option value="${esc(p.profile_id)}" ${p.profile_id===model.profile_id?'selected':''}>${esc(p.profile_id)}</option>`).join('')}</select></label>
      <div class="profile"><div class="profile-grid"><div><small>存储预留</small>${esc(formatResource(profile?.storage_reserved_bytes,'bytes'))}</div><div><small>单工程大小上限</small>${esc(formatResource(profile?.max_file_bytes,'bytes'))}</div></div><p class="muted">创建完成后保持停止，启动时检查并分配计算资源。</p></div></section></fieldset>
      <div class="form-actions"><a class="btn" href="#/market?blender=instances">返回实例</a><button class="btn primary" type="submit" ${busy || checking || !options.profiles.length || (!pending && !model.new_project && !source)?'disabled':''}>${busy?'正在确认…':pending?.operation?'查看创建进度':pending?'重试确认创建':'创建实例'}</button></div></div>
      <aside class="panel pad"><h2>创建后的步骤</h2><ol class="creation-steps"><li><strong>准备独立工作区</strong><p>校验并恢复所选工程，或建立空白工作副本。</p></li><li><strong>按需启动</strong><p>启动后分配计算资源，确认场景加载完成。</p></li><li><strong>打开并创作</strong><p>保存到项目；停止或销毁后保留工程与工作区。</p></li></ol><div class="state-note">你将拥有新实例的管理和编辑权限。创建操作不授予其他项目的访问权。</div></aside></div>
      <p id="blenderCreateError" class="state-note error" role="alert" ${error?'':'hidden'}>${esc(error)}</p>
      ${pending?'<p class="state-note" role="status">已保留本次请求。离开页面不会取消已受理的操作；重试沿用同一次创建。</p>':''}</form>`;
  }
  async function loadProjects(cursor) {
    const data = await request(base+'instance-projects', cursor ? {cursor} : {});
    if (!Array.isArray(data.items) || data.items.some(p=>!validID(p.project_id)||!validID(p.head_revision_id)||typeof p.name!=='string') || (data.next_cursor !== '' && !validID(data.next_cursor))) throw Error('项目列表暂不可用，请重试。');
    return data;
  }
  async function show() {
    const current = ++epoch; ++selection; options = null; busy = false; pending = null; error = ''; source = null; checking = false;
    host.innerHTML = '<section class="panel pad"><p role="status">正在读取创建权限与规格…</p></section>';
    try {
      const value = await request(base+'instance-options');
      if (current !== epoch) return;
      if (value.can_manage !== true || ![value.station_id,value.user_id,value.organization_id].every(validID) || !Array.isArray(value.profiles)) throw Error('当前账号暂时无法创建实例。');
      scope = [value.station_id,value.organization_id,value.user_id].join(':'); options = value;
      pending = read();
      model = pending ? {...pending} : {name:'',project_id:'',source_revision_id:'',new_project:false,project_name:'',profile_id:value.profiles[0]?.profile_id || ''};
      if (pending?.operation) { onAccepted(pending.operation); return; }
      const data = await loadProjects();
      if (current !== epoch) return;
      projects = data.items; nextCursor = data.next_cursor;
      if (pending) source = pending.source || null;
      render();
    } catch (e) {
      if (current !== epoch) return;
      options = null;
      host.innerHTML = `<section class="panel pad"><h2>暂时无法创建实例</h2><p class="state-note error" role="status">${esc(e.message)}</p><div class="actions"><a class="btn" href="#/market?blender=instances">返回实例</a><button class="btn" data-create-action="reload">重试</button></div></section>`;
    }
  }
  async function verify() {
    const current = epoch, selected = ++selection;
    source = null; error = ''; checking = true; render();
    try {
      if (!validID(model.project_id) || !validID(model.source_revision_id)) throw Error('请输入有效的来源修订编号。');
      const fixed = {project_id:model.project_id,source_revision_id:model.source_revision_id};
      const value = await request(base+'instance-source', fixed);
      if (current !== epoch || selected !== selection) return;
      if (value.project_id !== fixed.project_id || value.source_revision_id !== fixed.source_revision_id || !['empty','restore'].includes(value.kind)) throw Error('工程来源未能确认，请重试。');
      source = value;
    } catch (e) { if (current === epoch && selected === selection) error = e.message; }
    finally { if (current === epoch && selected === selection) {checking = false; render();} }
  }
  async function submit(event) {
    event.preventDefault();
    if (!options || busy || checking) return;
    if (pending?.operation) { onAccepted(pending.operation); return; }
    if (!pending && !model.new_project && !source) return;
    const current = epoch, targetScope = scope;
    let draft = pending;
    try {
      if (!draft) {
        draft = {...model, name:model.name.trim(), project_name:model.project_name.trim(), scope, source,
          idempotency_key:crypto.randomUUID(), commit_id:commitID()};
        if (!draft.name || (draft.new_project && !draft.project_name)) throw Error('请填写实例名称和项目。');
        save(draft); // A failed storage write must precede, and prevent, network effects.
        pending = draft;
      }
      busy = true; error = ''; render();
      if (draft.new_project && !draft.project_id) {
        const value = await request(base+'instance-project-create', {name:draft.project_name,commit_id:draft.commit_id});
        if (!validID(value.project_id) || !validID(value.source_revision_id) || value.kind !== 'empty') throw Error('新项目尚未确认，请重试原请求。');
        draft = {...draft,new_project:false,project_id:value.project_id,source_revision_id:value.source_revision_id,source:value};
        save(draft,targetScope);
        if (current === epoch) {pending = draft; model = {...draft};}
      }
      const value = await request(base+'instances', {name:draft.name,project_id:draft.project_id,source_revision_id:draft.source_revision_id,
        profile_id:draft.profile_id,idempotency_key:draft.idempotency_key});
      if (!validID(value.operation_id) || !/^blender[A-Za-z0-9_-]{1,57}$/.test(value.alias)) throw Error('创建结果尚未确认，请重试原请求。');
      draft = {...draft,operation:value}; save(draft,targetScope);
      if (current === epoch) {pending = draft; onAccepted(value);}
    } catch (e) {
      if (current !== epoch) return;
      // These rejections occur before admission and permit correcting the form.
      if (['INSTANCE_NAME_CONFLICT','PROFILE_UNAVAILABLE','CONTENT_TOO_LARGE'].includes(e.code)) {
        sessionStorage.removeItem(key(targetScope)); pending = null;
      }
      error = e.message;
    } finally { if (current === epoch) {busy = false; render();} }
  }
  listen(host,'input',event=>{
    if (pending || busy) return;
    const field = event.target.name;
    if (field === 'name') model.name = event.target.value;
    if (field === 'project_name') model.project_name = event.target.value;
    if (field === 'revision') {model.source_revision_id = event.target.value.trim();source = null;++selection;checking = false;host.querySelector('button[type=submit]').disabled = true;host.querySelector('.source-summary').textContent='来源修订已更改，请重新核验。';host.querySelector('[data-create-action=verify]').disabled=false;}
  });
  listen(host,'change',event=>{
    if (pending || busy) return;
    if (event.target.name === 'project') {
      ++selection; checking = false; source = null; error = ''; model.new_project = event.target.value === 'new';
      model.project_id = model.new_project ? '' : event.target.value;
      model.source_revision_id = projects.find(p=>p.project_id===model.project_id)?.head_revision_id || '';
      if (model.new_project) {source = {kind:'empty'};render();} else if (model.project_id) void verify(); else render();
    }
    if (event.target.name === 'profile') {model.profile_id = event.target.value;render();}
  });
  listen(host,'submit',submit);
  listen(host,'click',async event=>{
    const action = event.target.closest('[data-create-action]')?.dataset.createAction;
    if (action === 'reload') void show();
    if (action === 'verify') void verify();
    if (action === 'more' && !busy) {
      const current = epoch; busy = true; render();
      try {const data = await loadProjects(nextCursor); if (current === epoch) {projects = [...new Map([...projects,...data.items].map(p=>[p.project_id,p])).values()];nextCursor = data.next_cursor;}}
      catch(e) {if(current === epoch) error=e.message;}
      finally {if(current === epoch) {busy=false;render();}}
    }
  });
  return { show, hide() {++epoch;++selection;}, finish(operation) {
    // A detail deep link may finish after a full reload, before this form has loaded its scope.
    for (const name of Object.keys(sessionStorage)) {
      if (!name.startsWith('vf-blender-create:')) continue;
      try { if (JSON.parse(sessionStorage.getItem(name))?.operation?.operation_id === operation) sessionStorage.removeItem(name); } catch { /* Unrelated drafts remain intact. */ }
    }
    if (pending?.operation?.operation_id === operation) {sessionStorage.removeItem(key(pending.scope));pending=null;}
  }};
}
