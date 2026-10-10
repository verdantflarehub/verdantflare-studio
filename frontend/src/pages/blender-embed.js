export function mountBlender({ listen, notice }) {
  const $ = id => document.getElementById(id), host = $('blenderHost'), frame = $('blenderFrame'), main = host.closest('.main');
  let active = false, items = [], route = null, epoch = 0, session = null, pending = null;
  let timeout, renewal, saving = false, listScroll = 0, marketScroll = 0;
  const alias = () => new URLSearchParams(location.hash.split('?')[1] || '').get('blender');
  const name = item => item.alias.replace(/^blender/, 'Blender ');
  const messages = {
    INSTANCE_EDIT_LEASE_HELD: '实例正在使用中，请稍后重试。', GUI_EDIT_LEASE_HELD: '实例正在使用中，请稍后重试。',
    PERMISSION_DENIED: '当前账号无权访问此工程。', GUI_ACCESS_DENIED: '当前账号无权编辑此实例。',
    REVISION_CONFLICT: '工程已有新修订。本次检查点已保留，请核对后再保存。',
    TURN_NOT_CONFIGURED: '桌面连接尚未配置，请联系管理员。', INITIAL_PROJECT_RESTORE_REQUIRED: '工程尚未恢复到此实例，请联系管理员。'
  };
  async function request(path, body, keepalive = false) {
    const response = await fetch(path, { method: body === undefined ? 'GET' : 'POST', credentials: 'same-origin', keepalive,
      headers: body === undefined ? {} : { 'Content-Type': 'application/json' }, body: body === undefined ? undefined : JSON.stringify(body) });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw Error(messages[data.code] || '暂时无法连接，请重试。');
    return data;
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
    const search = $('blenderSearch').value.trim().toLowerCase();
    const matches = items.filter(item => (name(item) + ' ' + item.alias).toLowerCase().includes(search));
    $('blenderRows').replaceChildren();
    for (const item of matches) {
      const row = document.createElement('tr'), first = document.createElement('td'), access = document.createElement('td'), action = document.createElement('td');
      const link = document.createElement('a'); link.href = '#/market?blender=' + encodeURIComponent(item.alias); link.className = 'blender-instance-name';
      const img = document.createElement('img'); img.src = '/assets/blender-logo.svg'; img.alt = '';
      link.append(img, document.createTextNode(name(item))); first.append(link);
      access.textContent = item.access === 'edit' ? '可编辑' : '只读';
      const open = document.createElement('a'); open.href = link.href; open.className = 'btn primary'; open.textContent = '打开'; open.setAttribute('aria-label', '打开 ' + name(item)); action.append(open);
      row.append(first, access, action); $('blenderRows').append(row);
    }
    $('blenderCount').textContent = matches.length + ' 个实例'; $('blenderTable').hidden = !matches.length;
    $('blenderListMessage').hidden = !!matches.length;
    $('blenderListMessage').textContent = items.length ? '没有匹配的实例。' : '当前账号暂无可访问的实例。';
  }
  function breadcrumb(target) {
    const nav = $('blenderBreadcrumb'); nav.replaceChildren();
    const market = document.createElement('a'); market.href = '#/market'; market.textContent = '应用市场'; nav.append(market, ' / ');
    const app = document.createElement(target === 'instances' ? 'span' : 'a'); app.textContent = 'Blender';
    if (target !== 'instances') app.href = '#/market?blender=instances'; else app.setAttribute('aria-current', 'page');
    nav.append(app);
    if (target !== 'instances') {
      const current = document.createElement('span'), item = items.find(i => i.alias === target);
      current.setAttribute('aria-current', 'page'); current.textContent = item ? name(item) : '实例不可用'; nav.append(' / ', current);
    }
  }
  async function sync(force = false) {
    const target = alias();
    if (!target) { close(); return; }
    if (active && target === route && !force) return;
    if (route === 'instances') listScroll = window.scrollY;
    const current = ++epoch, opening = pending;
    if (!active) marketScroll = window.scrollY;
    active = true; route = target;
    host.hidden = false; host.style.display = 'flex'; $('marketView').hidden = true; $('marketView').style.display = 'none';
    main.classList.add('embed-active', 'blender-active');
    $('blenderList').hidden = target !== 'instances'; $('blenderWorkspace').hidden = target === 'instances';
    $('blenderListRetry').hidden = true; $('blenderTable').hidden = true; $('blenderCount').textContent = '';
    $('blenderListMessage').hidden = false; $('blenderListMessage').textContent = '正在读取实例…';
    breadcrumb(target); status('正在打开', 'loading');
    await release().catch(() => {}); await opening?.catch(() => {});
    if (current !== epoch) return;
    try {
      const data = await request('/studio/apps/blender/instances');
      if (current !== epoch) return;
      items = data.instances; breadcrumb(target);
      if (target === 'instances') { renderList(); requestAnimationFrame(() => window.scrollTo(0, listScroll)); }
      else {
        window.scrollTo(0, 0);
        const selected = items.find(item => item.alias === target);
        if (!selected) status('实例不存在或当前账号无权访问。', 'empty', false);
        else if (selected.access !== 'edit') status('当前账号没有桌面编辑权限。', 'empty', false);
        else void connect();
      }
    } catch (error) {
      if (current !== epoch) return;
      if (target === 'instances') { $('blenderListMessage').textContent = error.message; $('blenderListRetry').hidden = false; }
      else status(error.message);
    }
  }
  function close() {
    if (!active) return;
    if (route === 'instances') listScroll = window.scrollY;
    active = false; route = null; ++epoch;
    void release().catch(() => {}); host.hidden = true; host.style.display = 'none'; main.classList.remove('blender-active');
    if (!location.hash.includes('video=') && !location.hash.includes('image=')) {
      main.classList.remove('embed-active'); $('marketView').hidden = false; $('marketView').style.display = '';
      $('pageTitle').textContent = '应用市场'; $('pageSubtitle').textContent = '为 Station 安装创作应用，管理模型与运行状态。';
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
      saving = false; if (current === epoch) $('blenderAction').disabled = false;
    }
  }
  listen($('blenderAction'), 'click', () => { if (host.dataset.state === 'loading') void stop(); else if (!items.some(item => item.alias === alias())) void sync(true); else void connect(); });
  listen($('blenderRelease'), 'click', () => void stop('已释放控制权。', 'released'));
  listen($('blenderSave'), 'click', () => void save());
  listen($('blenderCopy'), 'click', () => {
    const item = items.find(i => i.alias === alias());
    if (item) navigator.clipboard.writeText(location.origin + item.mcp_path).then(() => notice('已复制实例 MCP 地址')).catch(() => notice('复制失败，请检查浏览器权限。'));
  });
  listen($('blenderSearch'), 'input', renderList); listen($('blenderListRetry'), 'click', () => void sync(true));
  listen(window, 'hashchange', () => void sync()); listen(window, 'pagehide', () => { ++epoch; void release().catch(() => {}); });
  listen(window, 'message', event => {
    if (!active || !session || event.source !== frame.contentWindow || event.origin !== location.origin || event.data?.channel !== 'vf-blender' || event.data?.session !== session.id) return;
    const data = event.data;
    if (data.type === 'ready') { clearTimeout(timeout); status('', 'ready'); frame.contentWindow.postMessage({ channel: 'vf-studio', type: 'theme', theme: document.body.dataset.theme }, location.origin); }
    if (data.type === 'error') void stop('画面连接已中断，请重试。', 'error');
    if (data.type === 'wheel' && Number.isFinite(data.deltaY)) window.scrollBy(0, Math.max(-1000, Math.min(data.deltaY, 1000)));
  });
  return { sync, close, open: () => { location.hash = '/market?blender=instances'; }, dispose: close };
}
