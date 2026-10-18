import { instanceName, instanceURL, isFresh, canOpen, canStop, canDestroy, filterCounts, summary, instanceCard, detail } from './blender-management';
import { mountBlenderCreation } from './blender-create';
import { createBlenderStarter } from './blender-start';
import { blenderMCPAddress } from '../app-center/legacy-blender';

export function mountBlender({ listen, notice, root = /** @type {Document | HTMLElement} */ (document), location = /** @type {Pick<Location, 'hash' | 'origin'>} */ (window.location), authorize, fetcher = fetch, mcpOrigin = location.origin }) {
  const $ = id => root.querySelector('#' + id), host = $('blenderHost'), frame = $('blenderFrame'), main = host.closest('.main');
  const lifetime = new AbortController();
  const now = () => undefined;
  const PAGE_SIZE = 10;
  let active = false, items = [], route = null, epoch = 0, session = null, pending = null;
  let timeout, renewal, saving = false, listScroll = 0, marketScroll = 0;
  let polling, filter = 'all', page = 1, refreshing = false, hasSnapshot = false, createAvailable = false, operation = null, operationError = '';
  const alias = () => new URLSearchParams(location.hash.split('?')[1] || '').get('blender');
  const panel = () => new URLSearchParams(location.hash.split('?')[1] || '').get('panel') || '';
  const name = instanceName;
  const messages = {
    INSTANCE_EDIT_LEASE_HELD: '实例正在使用中，请稍后重试。', GUI_EDIT_LEASE_HELD: '实例正在使用中，请稍后重试。',
    PERMISSION_DENIED: '当前账号无权访问此工程。', GUI_ACCESS_DENIED: '当前账号无权编辑此实例。',
    REVISION_CONFLICT: '工程已有新修订。本次检查点已保留，请核对后再保存。',
    TURN_NOT_CONFIGURED: '桌面连接尚未配置，请联系管理员。', INITIAL_PROJECT_RESTORE_REQUIRED: '工程尚未恢复到此实例，请联系管理员。',
    MANAGEMENT_UNAVAILABLE:'实例管理暂未开放。', INSTANCE_NAME_CONFLICT:'已有同名实例，请修改名称。',
    PROFILE_UNAVAILABLE:'该运行规格已不可用，请重新选择。', CONTENT_TOO_LARGE:'来源工程超过当前规格的大小限制。',
    PROJECT_SOURCE_MISMATCH:'工程来源不一致，请重新核验固定修订。', STORAGE_CAPACITY_UNAVAILABLE:'工作区存储容量不足，正在等待可用容量。',
    WORKSPACE_BINDING_CONFLICT:'工作区归属发生变化，已暂停推进，请联系管理员。',
    COMPUTE_CAPACITY_UNAVAILABLE:'当前计算资源不足，正在等待可用容量。',
    COMPUTE_CAPACITY_UNKNOWN:'暂时无法确认可用计算资源，系统将继续检查。',
    WORKER_LOAD_UNVERIFIED:'正在确认 Blender 已加载工程。', WORKER_EXITED:'Blender 进程已退出，工作区仍保留，请联系管理员。',
    SAVE_IN_PROGRESS:'工程正在保存，请等待保存完成。', WORKER_BUSY:'Blender 仍有任务或操作进行中，正在等待空闲。',
    WORKER_ACTIVITY_UNKNOWN:'暂时无法确认 Blender 是否空闲，工作区已保留。',
    WORKER_DRAIN_UNVERIFIED:'正在确认工程已保存且 Blender 已停止接受编辑。',
    PROJECT_EXTERNAL_DEPENDENCIES:'工程含外链资源，请在 Blender 中打包资源后重试。',
    INSTANCE_STATE_CONFLICT:'实例状态已变化，请刷新后重试。', INSTANCE_BUSY:'实例已有操作进行中，请查看进度。',
    INSTANCE_NAME_MISMATCH:'实例名称不匹配，请输入完整名称。',
    WORKSPACE_PREPARATION_UNAVAILABLE:'工作区准备暂未确认，系统将继续核对原操作。',
    OPERATION_OUTCOME_UNKNOWN:'操作结果尚未确认，正在继续核对。', NOT_FOUND:'项目、修订或操作不存在，或当前账号无权访问。'
  };
  async function request(path, body, keepalive = false) {
    if (!keepalive && authorize) await authorize();
    const controller = new AbortController(), deadline = setTimeout(() => controller.abort(), 30000);
    try {
      const response = await fetcher(path, { method: body === undefined ? 'GET' : 'POST', credentials: 'same-origin', keepalive, signal: keepalive ? controller.signal : AbortSignal.any([controller.signal, lifetime.signal]),
        headers: body === undefined ? {} : { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body) });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) throw Object.assign(Error(messages[data.code] || '暂时无法连接，请重试。'), {code:data.code,status:response.status});
      if (path === '/studio/apps/blender/instances' && body === undefined) {
        if (data?.schema_version !== 1 || !Array.isArray(data.instances)) throw Error('实例管理服务需要更新，请联系管理员。');
      }
      return data;
    } catch (error) {
      if (controller.signal.aborted) throw Error('连接请求超时，请重试。');
      throw error;
    } finally { clearTimeout(deadline); }
  }
  function adoptSnapshot(data) {
    items = data.instances; hasSnapshot = true;
  }
  const creation = mountBlenderCreation({host:$('blenderCreateView'),request,listen,onAccepted:value=>{
    location.hash = '/market?blender='+encodeURIComponent(value.alias)+'&panel=overview&operation='+encodeURIComponent(value.operation_id);
  }});
  function creationAction() {
    $('blenderCreate').disabled = !createAvailable;
    $('blenderCreate').title = createAvailable ? '' : '当前账号或环境暂不允许创建实例';
  }
  async function readOperation(current) {
    const item = items.find(i=>i.alias===alias());
    const id = item?.management_operation_id || new URLSearchParams(location.hash.split('?')[1] || '').get('operation');
    operation = null; operationError = '';
    if (!id || !panel()) return;
    try {
      const result = await request('/studio/apps/blender/instance-operations/'+encodeURIComponent(id));
      if (current !== epoch) return;
      if (result.operation_id !== id || result.alias !== alias() || (item && result.instance_id !== item.instance_id)) throw Error('操作与实例不匹配，请刷新检查。');
      if(!item && result.action==='destroy' && result.state==='completed' && result.phase==='deleted'){
        notice('实例已销毁，项目修订与工作区已保留，继续占用存储。');location.hash='/market?blender=instances';return;
      }
      if(!item)throw Error('实例状态暂不可用，请刷新检查。');
      operation = result;
      if (result.state === 'completed') creation.finish(id);
    } catch(e) {if(current === epoch) operationError=e.message;}
  }
  function operationView() {
    if (!operation && !operationError) return;
    const section = document.createElement('section'); section.className = 'panel pad operation-progress';section.setAttribute('aria-live','polite');
    const heading = document.createElement('h2'), text = document.createElement('p');
    const starting = operation?.action === 'start', stopping = operation?.action === 'stop', destroying = operation?.action === 'destroy';
    heading.textContent = operation?.state === 'completed' ? (destroying ? '实例已销毁' : stopping ? '实例已停止' : starting ? '实例已启动' : '工作区已准备完成')
      : destroying ? '销毁进度' : stopping ? (operation?.state === 'failed' ? '停止未完成，实例继续运行' : '保存并停止') : (starting ? '启动进度' : '创建进度');
    const phases = {accepted:'请求已受理',reserved:'已预留工作区',claimed:'已绑定独立存储',preparing:'正在准备工作区',awaiting_content:'正在校验并恢复工程',removing:'工程已准备，正在确认准备服务退出',stopped:'已停止，可按需启动',launching:'正在启动 Blender',loading:'正在加载并校验工程',running:'工程已加载，可打开实例'};
    if (stopping) Object.assign(phases,{draining:'正在等待任务空闲并保存检查点',saving:'正在将工程保存到项目',stopping:'工程已保存，正在停止实例',removing:'正在确认计算资源释放',aborting:'保存未完成，正在恢复编辑',aborted:'本次停止已取消，实例继续运行'});
    if (destroying) Object.assign(phases,{revoking:'正在撤销实例访问并登记保留工作区',deleted:'实例已销毁，工作区与项目修订保留'});
    text.textContent = operationError || messages[operation?.code] || phases[operation?.phase] || '正在确认操作状态';
    if(operationError || operation?.code) text.className='state-note error';
    section.append(heading,text);$('blenderDetail').prepend(section);
  }
  const closeLease = value => request('/studio/apps/blender/' + value.alias + '/close', { editing_session_id: value.id }, true);
  async function release() {
    clearTimeout(timeout); clearTimeout(renewal);
    frame.removeAttribute('src'); frame.hidden = true;
    const old = session; session = null;
    if (old) await closeLease(old);
  }
  function status(text, state = 'error', canRetry = true) {
    host.dataset.state = state;
    $('blenderMessage').textContent = text;
    const selected = items.find(item => item.alias === alias());
    if ($('blenderWorkspaceTitle')) $('blenderWorkspaceTitle').textContent = selected ? name(selected) : 'Blender';
    $('blenderLoading').hidden = state === 'ready'; $('blenderToolbar').hidden = state !== 'ready';
    $('blenderProgress').hidden = state !== 'loading'; $('blenderAction').hidden = state === 'ready' || !canRetry;
    $('blenderAction').disabled = saving;
    $('blenderAction').textContent = state === 'loading' ? '取消连接' : '重新连接';
    frame.style.visibility = state === 'ready' ? 'visible' : 'hidden';
  }
  async function stop(text = '连接已取消', state = 'cancelled') {
    const current = ++epoch; status(text, state);
    $('blenderAction').disabled = true;
    const opening = pending;
    await release().catch(() => {}); await opening?.catch(() => {});
    if (current === epoch) $('blenderAction').disabled = saving;
    return current;
  }
  async function connect() {
    if (!active || saving || pending || session) return;
    const selected = items.find(item => item.alias === alias());
    if (!selected || selected.access !== 'edit') return;
    const target = selected.alias, current = ++epoch;
    status('正在打开', 'loading');
    timeout = setTimeout(() => { if (current === epoch) void stop('连接超时，请重试。', 'error'); }, 45000);
    pending = (async () => {
      try {
        // Reconnects and direct links must recheck the same current observation
        // as the cards; old snapshots cannot authorize a new editing lease.
        const data = await request('/studio/apps/blender/instances');
        if (current !== epoch || !active || alias() !== target) return;
        adoptSnapshot(data);
        if (!canOpen(items.find(item => item.alias === target) || {}, now())) {
          throw Error('实例状态或控制权暂不允许连接，请返回详情刷新检查。');
        }
        const value = await request('/studio/apps/blender/' + target + '/open', {});
        const opened = { alias: target, id: value.editing_session_id };
        if (current !== epoch || !active || alias() !== target) { await closeLease(opened); return; }
        session = opened; frame.hidden = false;
        frame.src = '/apps/blender/' + target + '/' + opened.id + '/';
        // Renew via a fresh authorized allocation; Blender keeps its in-memory scene.
        renewal = setTimeout(async () => {
          if (current !== epoch || session !== opened) return;
          const stopped = await stop('正在打开', 'loading');
          if (stopped === epoch && active && alias() === target) void connect();
        }, Math.max(60, Math.min(720, value.reconnect_after || 720)) * 1000);
      } catch (error) {
        if (current === epoch) { await release().catch(() => {}); status(error.message); }
      } finally { pending = null; }
    })();
    await pending;
  }
  function renderList() {
    if (!hasSnapshot) return;
    const currentTime = now();
    const counts = filterCounts(items, currentTime);
    for (const [k, v] of Object.entries(counts)) {
      const badgeEl = $('blenderFilters')?.querySelector(`[data-filter-count="${k}"]`);
      if (badgeEl) badgeEl.textContent = String(v);
    }
    const search = $('blenderSearch').value.trim().toLowerCase();
    const matches = items.filter(item => (name(item) + ' ' + item.alias + ' ' + (item.project_name || '')).toLowerCase().includes(search)
      && (filter === 'all'
        || filter === 'failed' && (!isFresh(item, currentTime) || ['unknown','failed'].includes(item.status))
        || filter === 'stopped' && isFresh(item, currentTime) && ['stopped','created'].includes(item.status)
        || isFresh(item, currentTime) && item.status === filter));
    const totalPages = Math.max(1, Math.ceil(matches.length / PAGE_SIZE));
    page = Math.min(Math.max(1, page), totalPages);
    const paged = matches.slice((page - 1) * PAGE_SIZE, page * PAGE_SIZE);
    if ($('blenderSkeleton')) $('blenderSkeleton').hidden = true;
    $('blenderRows').innerHTML = paged.map(item => instanceCard(item, currentTime)).join('');
    $('blenderSummary').innerHTML = summary(items, currentTime); $('blenderSummary').hidden = true;
    if ($('blenderTableWrap')) $('blenderTableWrap').hidden = !matches.length;
    if ($('blenderPageSummary')) $('blenderPageSummary').textContent = `共 ${matches.length} 个实例 · 第 ${page} / ${totalPages} 页（每页 ${PAGE_SIZE} 条）`;
    if ($('blenderPrevPage')) $('blenderPrevPage').disabled = page <= 1;
    if ($('blenderNextPage')) $('blenderNextPage').disabled = page >= totalPages;
    if ($('blenderEmpty')) $('blenderEmpty').hidden = !!matches.length;
    if ($('blenderEmptyTitle')) $('blenderEmptyTitle').textContent = items.length ? '没有匹配的实例' : '暂无 Blender 实例';
    if ($('blenderClearFilter')) $('blenderClearFilter').hidden = !items.length || (filter === 'all' && !search);
    $('blenderListMessage').hidden = !!matches.length;
    $('blenderListMessage').textContent = items.length ? '没有匹配的实例，请尝试其他关键词或清除筛选条件。' : '当前账号暂无可访问的实例。';
  }
  function renderDetail() {
    const selected = items.find(item => item.alias === alias());
    if (!selected || !['overview','resources','activity'].includes(panel())) {
      $('blenderDetail').textContent = '实例不存在或当前账号无权访问。'; return;
    }
    $('blenderDetail').innerHTML = detail(selected, panel(), now());
    operationView();
  }
  function detailError(message) {
    const text = document.createElement('p'), retry = document.createElement('button');
    text.textContent = message; text.setAttribute('role', 'status');
    retry.className = 'btn'; retry.textContent = '重试';
    retry.addEventListener('click', () => void sync(true), {once:true});
    $('blenderDetail').replaceChildren(text, retry);
  }
  function scheduleRefresh() {
    clearTimeout(polling);
    if (active && alias() !== 'create' && (alias() === 'instances' || panel())) polling = setTimeout(refresh, document.hidden ? 60000 : operation?.state === 'running' ? 2000 : 10000);
  }
  async function refresh() {
    if (!active || alias() === 'create' || (alias() !== 'instances' && !panel()) || refreshing) return;
    const current = epoch;
    refreshing = true; $('blenderRefresh').disabled = true;
    try {
      const data = await request('/studio/apps/blender/instances');
      if (current !== epoch) return;
      adoptSnapshot(data);
      createAvailable = data.capabilities?.create === true; creationAction();
      await readOperation(current); if(current !== epoch) return;
      breadcrumb(alias());
      const focus = document.activeElement;
      const href = (focus?.closest('#blenderRows') || focus?.closest('#blenderDetail')) ? focus.getAttribute('href') : null;
      if (alias() === 'instances') { renderList(); $('blenderListRetry').hidden = true; } else renderDetail();
      if (href) [...host.querySelectorAll('a[href]')].find(el => el.getAttribute('href') === href)?.focus({preventScroll:true});
    } catch(error) {
      if (current !== epoch) return;
      createAvailable = false; creationAction(); operation = null; operationError = '实例状态刷新失败，操作仍在后台核对。';
      // Never leave stale open links enabled after a failed refresh.
      items = items.map(item => ({...item,observed_at:null,status:'unknown',allowed_actions:['view'],resources_observed_at:null,
        resources:Object.fromEntries(Object.entries(item.resources || {}).map(([key,value]) => [key,{...value,value:null,quality:'unavailable'}]))}));
      if (alias() === 'instances') {
        renderList();
        if ($('blenderEmpty')) $('blenderEmpty').hidden = !!items.length;
        $('blenderListMessage').hidden = false; $('blenderListMessage').textContent = error.message; $('blenderListRetry').hidden = false;
      } else { if (hasSnapshot) {renderDetail();notice(error.message);} else detailError(error.message); }
    } finally {
      refreshing = false; $('blenderRefresh').disabled = false;
      scheduleRefresh();
    }
  }
  function breadcrumb(target) {
    const nav = $('blenderBreadcrumb'); nav.replaceChildren();
    const app = document.createElement('a');
    app.textContent = 'Blender';
    app.href = '#/market?blender=instances';
    const sep = document.createElement('span');
    sep.setAttribute('aria-hidden', 'true');
    sep.textContent = '/';
    const current = document.createElement('span');
    current.setAttribute('aria-current', 'page');
    if (target === 'instances') {
      current.textContent = '实例列表';
    } else {
      const item = items.find(i => i.alias === target);
      current.textContent = target === 'create' ? '创建实例' : item ? name(item) : '实例详情';
    }
    nav.append(app, sep, current);
  }
  async function sync(force = false) {
    const target = alias();
    if (!target) { close(); return; }
    const key = target === 'instances' ? target : target + '/' + panel();
    if (active && key === route && !force) return;
    if (route === 'instances') listScroll = window.scrollY;
    const current = ++epoch, opening = pending;
    if (!active) marketScroll = window.scrollY;
    const keepListForCreate = target === 'create' && hasSnapshot;
    active = true; route = key;
    if (!keepListForCreate) { items = []; hasSnapshot = false; }
    operation=null;operationError='';createAvailable=false;creationAction();clearTimeout(polling);creation.hide();
    host.hidden = false; host.style.display = 'flex'; if ($('marketView')) { $('marketView').hidden = true; $('marketView').style.display = 'none'; }
    main.classList.add('embed-active', 'blender-active');
    const management = target !== 'instances' && target !== 'create' && !!panel();
    const createDialog = $('blenderCreateDialog');
    if (createDialog) {
      if (target === 'create') {
        $('blenderCreateView').hidden = false;
        if (!createDialog.open && typeof createDialog.showModal === 'function') createDialog.showModal();
      } else {
        if (createDialog.open && typeof createDialog.close === 'function') createDialog.close();
        $('blenderCreateView').hidden = true;
      }
    } else {
      $('blenderCreateView').hidden = target !== 'create';
    }
    if ($('blenderAppHeader')) $('blenderAppHeader').hidden = target !== 'instances' && target !== 'create';
    $('blenderList').hidden = target !== 'instances' && target !== 'create';
    $('blenderWorkspace').hidden = target === 'instances' || target === 'create' || management;
    $('blenderDetail').hidden = !management; $('blenderDetail').textContent = '正在读取实例…';
    $('blenderManageLink').hidden = target === 'instances' || target === 'create' || management;
    $('blenderCreate').hidden = target !== 'instances' && target !== 'create';
    $('blenderManageLink').href = instanceURL({alias:target}, 'overview');
    if (!keepListForCreate) {
      $('blenderListRetry').hidden = true; $('blenderRows').replaceChildren(); $('blenderSummary').hidden = true;
      if ($('blenderTableWrap')) $('blenderTableWrap').hidden = true;
      if ($('blenderSkeleton')) $('blenderSkeleton').hidden = target !== 'instances';
      if ($('blenderEmpty')) $('blenderEmpty').hidden = true;
      $('blenderListMessage').hidden = false; $('blenderListMessage').textContent = '正在读取实例…';
    }
    breadcrumb(target); status('正在打开', 'loading');
    await release().catch(() => {}); await opening?.catch(() => {});
    if (current !== epoch) return;
    if (target === 'create') {
      if (!keepListForCreate) {
        void request('/studio/apps/blender/instances').then(data => {
          if (current !== epoch) return;
          adoptSnapshot(data);
          createAvailable = data.capabilities?.create === true; creationAction();
          renderList();
        }).catch(() => {});
      }
      await creation.show();
      return;
    }
    try {
      const data = await request('/studio/apps/blender/instances');
      if (current !== epoch) return;
      adoptSnapshot(data); breadcrumb(target);
      createAvailable=data.capabilities?.create===true;creationAction();
      await readOperation(current);if(current!==epoch)return;
      if (target === 'instances') { renderList(); requestAnimationFrame(() => window.scrollTo(0, listScroll)); }
      else if (management) { renderDetail(); window.scrollTo(0, 0); }
      else {
        window.scrollTo(0, 0);
        const selected = items.find(item => item.alias === target);
        if (!selected) status('实例不存在或当前账号无权访问。', 'empty', false);
        else if (selected.access !== 'edit') status('当前账号没有桌面编辑权限。', 'empty', false);
        else if (!canOpen(selected, now())) status('实例状态或控制权暂不允许连接，请返回详情刷新检查。', 'empty', false);
        else if (saving) status('正在保存工程…', 'saving', false);
        else void connect();
      }
    } catch (error) {
      if (current !== epoch) return;
      if (target === 'instances') {
        if ($('blenderSkeleton')) $('blenderSkeleton').hidden = true;
        if ($('blenderEmpty')) $('blenderEmpty').hidden = false;
        if ($('blenderEmptyTitle')) $('blenderEmptyTitle').textContent = '实例列表读取失败';
        $('blenderListMessage').hidden = false; $('blenderListMessage').textContent = error.message; $('blenderListRetry').hidden = false;
      }
      else if (management) detailError(error.message);
      else status(error.message);
    }
    if (current === epoch) scheduleRefresh();
  }
  function close() {
    if (!active) return;
    if (route === 'instances') listScroll = window.scrollY;
    active = false; route = null; ++epoch;
    clearTimeout(polling);creation.hide();
    void release().catch(() => {}); host.hidden = true; host.style.display = 'none'; main.classList.remove('blender-active');
    if (!location.hash.includes('video=') && !location.hash.includes('image=')) {
      main.classList.remove('embed-active'); if ($('marketView')) { $('marketView').hidden = false; $('marketView').style.display = ''; }
      if ($('pageTitle')) $('pageTitle').textContent = '应用中心'; if ($('pageSubtitle')) $('pageSubtitle').textContent = '为 Station 安装创作应用，管理模型与运行状态。';
      requestAnimationFrame(() => window.scrollTo(0, marketScroll));
    }
  }
  async function rpc(tool, args, target) {
    const data = await request('/mcp/' + target, { jsonrpc: '2.0', id: crypto.randomUUID(), method: 'tools/call', params: { name: tool, arguments: args } });
    if (data.error) throw Error('操作未完成，请重试。');
    const value = JSON.parse(data.result.content[0].text);
    if (data.result.isError) throw Error(messages[value.code || value.result?.code] || '操作未完成，已保留当前工程。');
    return value;
  }
  async function save() {
    if (saving || !session) return;
    saving = true;
    const target = alias(), selected = items.find(item => item.alias === target);
    const current = await stop('正在保存工程…', 'loading'); let edit;
    try {
      edit = await rpc('session.open', { project_id: selected.project_id, mode: 'edit' }, target);
      const args = { editing_session_id: edit.editing_session_id }, scene = await rpc('scene.get', args, target);
      const op = await rpc('project.save', { ...args, scene_version: scene.scene_version, idempotency_key: crypto.randomUUID() }, target);
      let result;
      for (let count = 0; count < 90; count++) {
        result = await rpc('operation.get', { operation_id: op.operation_id }, target);
        if (result.state === 'completed') break;
        if (result.state === 'failed' || result.result?.code === 'REVISION_CONFLICT') throw Error(messages[result.result?.code] || '保存未完成，检查点已保留。');
        await new Promise(resolve => setTimeout(resolve, 1000));
      }
      if (result?.state !== 'completed') throw Error('保存尚未确认，操作编号：' + op.operation_id);
      notice('Blender 工程已保存'); if (current === epoch) status('已保存到项目。', 'saved');
    } catch (error) { if (current === epoch) status(error.message); }
    finally {
      if (edit) await rpc('session.close', { editing_session_id: edit.editing_session_id }, target).catch(() => {});
      saving = false;
      if (current === epoch) $('blenderAction').disabled = false;
      else if (active && !['instances','create'].includes(alias())) void sync(true);
    }
  }
  listen($('blenderAction'), 'click', () => { if (host.dataset.state === 'loading') void stop(); else if (!items.some(item => item.alias === alias())) void sync(true); else void connect(); });
  listen($('blenderRelease'), 'click', () => void stop('已释放控制权。', 'released'));
  listen($('blenderSave'), 'click', () => void save());
  listen($('blenderCopy'), 'click', () => {
    const item = items.find(i => i.alias === alias());
    if (item) {
      try { navigator.clipboard.writeText(blenderMCPAddress(mcpOrigin, item.alias, item.mcp_path)).then(() => notice('已复制实例 MCP 地址')).catch(() => notice('复制失败，请检查浏览器权限。')); }
      catch { notice('实例 MCP 地址无效，请刷新后重试。'); }
    }
  });
  listen($('blenderSearch'), 'input', () => { page = 1; renderList(); }); listen($('blenderListRetry'), 'click', () => void sync(true));
  if ($('blenderPrevPage')) listen($('blenderPrevPage'), 'click', () => { if (page > 1) { page--; renderList(); } });
  if ($('blenderNextPage')) listen($('blenderNextPage'), 'click', () => { page++; renderList(); });
  if ($('blenderClearFilter')) listen($('blenderClearFilter'), 'click', () => {
    filter = 'all'; page = 1; $('blenderSearch').value = '';
    for (const el of $('blenderFilters').querySelectorAll('[data-status]')) el.setAttribute('aria-pressed', String(el.dataset.status === 'all'));
    renderList();
  });
  if ($('blenderCreateDialog')) listen($('blenderCreateDialog'), 'cancel', event => { event.preventDefault(); location.hash = '/market?blender=instances'; });
  const startInstance = createBlenderStarter({request,now,storage:sessionStorage});
  const stopInstance = createBlenderStarter({request,now,storage:sessionStorage,action:'stop'});
  const destroyInstance = createBlenderStarter({request,now,storage:sessionStorage,action:'destroy'});
  const stopDialog = document.createElement('dialog');stopDialog.className='blender-management blender-stop-dialog';
  stopDialog.setAttribute('aria-labelledby','blenderStopTitle');
  stopDialog.innerHTML='<h2 id="blenderStopTitle"></h2><p data-description></p><ul><li data-retention></li><li data-access></li></ul><label class="field" data-name-field hidden><span>输入实例名称以确认</span><input autocomplete="off" data-name></label><p class="state-note" role="status"></p><div class="actions"><button class="btn" type="button" data-cancel>取消</button><button class="btn primary" type="button" data-confirm>保存并停止</button></div>';
  host.append(stopDialog);
  let stopTarget = null, stopTrigger = null, dialogAction = 'stop';
  const stopConfirm = stopDialog.querySelector('[data-confirm]');
  const confirmName = stopDialog.querySelector('[data-name]');
  listen(confirmName,'input',()=>{stopConfirm.disabled=confirmName.value!==instanceName(items.find(value=>value.alias===stopTarget)||{alias:''});});
  listen(stopDialog.querySelector('[data-cancel]'),'click',()=>stopDialog.close());
  listen(stopDialog,'close',()=>{if(stopTrigger?.isConnected)stopTrigger.focus();stopTarget=null;});
  listen(stopConfirm,'click',async()=>{
    const item=items.find(value=>value.alias===stopTarget),current=epoch;
    const note=stopDialog.querySelector('[role=status]');
    if(!item || !(dialogAction==='destroy'?canDestroy(item, now()):canStop(item, now()))){note.textContent='实例状态已变化，请关闭后刷新检查。';return;}
    stopConfirm.disabled=true;
    try{
      const result=await (dialogAction==='destroy'?destroyInstance(item,confirmName.value):stopInstance(item));
      if(result){stopDialog.close();if(current===epoch)location.hash='/market?blender='+encodeURIComponent(result.alias)+'&panel=overview&operation='+encodeURIComponent(result.operation_id);}
    }catch(e){note.textContent=e.message;}
    finally{stopConfirm.disabled=dialogAction==='destroy' && confirmName.value!==instanceName(item);}
  });
  listen(host,'click',async event=>{
    const stopButton=event.target.closest('[data-blender-stop],[data-blender-destroy]');
    if(stopButton && !stopButton.disabled){
      const destroying=stopButton.hasAttribute('data-blender-destroy');
      const item=items.find(value=>value.alias===(destroying?stopButton.dataset.blenderDestroy:stopButton.dataset.blenderStop));
      if(!item || !(destroying?canDestroy(item, now()):canStop(item, now())))return;
      dialogAction=destroying?'destroy':'stop';stopTarget=item.alias;stopTrigger=stopButton;stopConfirm.disabled=destroying;
      stopConfirm.textContent=destroying?'确认销毁':'保存并停止';stopConfirm.className=destroying?'btn danger solid':'btn primary';
      confirmName.value='';stopDialog.querySelector('[data-name-field]').hidden=!destroying;
      stopDialog.querySelector('h2').textContent=(destroying?'销毁 ':'保存并停止 ')+instanceName(item)+'？';
      stopDialog.querySelector('[data-description]').textContent=destroying?'销毁后，该实例及其访问入口将不可用。':'先将工程保存到项目，确认成功后再停止 Blender 并释放 CPU、内存与 GPU。';
      stopDialog.querySelector('[data-retention]').textContent=destroying?'保留正式项目修订、Artifact 和工作区，继续占用存储。':'保留工作区和项目修订，下次使用可重新启动。';
      stopDialog.querySelector('[data-access]').textContent=destroying?'工作区清理是独立操作，本次不会删除工作区数据。':'停止期间不接受新的桌面或 MCP 编辑连接。';
      stopDialog.querySelector('[role=status]').textContent='';stopDialog.showModal();return;
    }
    const button = event.target.closest('[data-blender-start]');
    if (!button || button.disabled) return;
    const item = items.find(value=>value.alias===button.dataset.blenderStart), current = epoch;
    if (!item) return;
    const prevHtml = button.innerHTML;
    button.disabled = true; button.setAttribute('aria-busy', 'true');
    try {
      const result = await startInstance(item);
      if (result && current === epoch) location.hash = '/market?blender='+encodeURIComponent(result.alias)+'&panel=overview&operation='+encodeURIComponent(result.operation_id);
    } catch(e) { if(current===epoch) {notice(e.message);await refresh();} }
    finally { if(button.isConnected) {button.disabled=false;button.removeAttribute('aria-busy');button.innerHTML=prevHtml;} }
  });
  listen($('blenderCreate'),'click',()=>{if(createAvailable)location.hash='/market?blender=create';});
  listen($('blenderRefresh'), 'click', () => void refresh());
  listen($('blenderFilters'), 'click', event => {
    const button = event.target.closest('[data-status]'); if (!button) return;
    filter = button.dataset.status; page = 1;
    for (const el of $('blenderFilters').querySelectorAll('[data-status]')) el.setAttribute('aria-pressed', String(el === button));
    renderList();
  });
  listen(document, 'visibilitychange', () => { if (active && (alias() === 'instances' || panel())) { if (!document.hidden) void refresh(); else { ++epoch; scheduleRefresh(); } } });
  listen(window, 'hashchange', () => void sync()); listen(window, 'pagehide', () => { ++epoch; void release().catch(() => {}); });
  listen(window, 'message', event => {
    if (!active || !session || event.source !== frame.contentWindow || event.origin !== location.origin || event.data?.channel !== 'vf-blender' || event.data?.session !== session.id) return;
    const data = event.data;
    if (data.type === 'ready') { clearTimeout(timeout); status('', 'ready'); frame.contentWindow.postMessage({ channel: 'vf-studio', type: 'theme', theme: document.body.dataset.theme }, location.origin); }
    if (data.type === 'error') void stop('画面连接已中断，请重试。', 'error');
    if (data.type === 'wheel' && Number.isFinite(data.deltaY)) window.scrollBy(0, Math.max(-1000, Math.min(data.deltaY, 1000)));
  });
  return { sync, close, open: () => { location.hash = '/market?blender=instances'; }, dispose: () => { close(); lifetime.abort(); stopDialog.remove(); } };
}
