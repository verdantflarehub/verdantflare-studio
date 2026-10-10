const messages = {
  EDITOR_IN_USE: '此实例已在另一窗口打开，请先关闭该窗口。',
  EDITOR_SESSION_EXPIRED: '编辑会话已失效，请重新打开。',
  DRAFT_VERSION_CONFLICT: '草稿版本已变化，请重新打开以恢复最新草稿。',
};

export function mountEditor({ alias, container, onStatus }) {
  const controller = new AbortController();
  let bridge, session, version, stopped = false, ready = false, saving, savedGraph, saveTimer, renewTimer, loadTimer, frame, releaseLock;
  let editGeneration = 0, savedGeneration = 0, editTimer, closing;
  const endpoint = `/studio/apps/comfyui/${encodeURIComponent(alias)}/editor/`;
  async function call(action, value, signal = controller.signal) {
    const response = await fetch(endpoint + action, { method:'POST', credentials:'same-origin',
      headers:{'Content-Type':'application/json'}, body:JSON.stringify(value), signal });
    if (!response.ok) {
      const data = await response.json().catch(()=>({}));
      throw Object.assign(Error(messages[data.code] || (response.status === 401 || response.status === 403 ? '登录或实例授权已失效，请重新登录。' : '暂时无法连接 ComfyUI，请重试。')), {status:response.status});
    }
    return response;
  }
  function theme() { bridge?.sendTheme(document.documentElement.dataset.theme === 'light' ? 'light' : 'dark'); }
  const observer = new MutationObserver(theme);
  observer.observe(document.documentElement,{attributes:true,attributeFilter:['data-theme']});
  function fail(error) {
    if (stopped) return;
    const message = error?.message || 'ComfyUI 连接中断，请重新打开。';
    dispose(); onStatus('error', message);
  }
  async function save() {
    // A leave/refresh flush waits for an older request, then captures again.
    // This prevents a change made during that request from being discarded.
    if (stopped || !ready) return;
    const operation = (saving || Promise.resolve()).catch(()=>{}).then(async () => {
      if (stopped) return;
      const generation = editGeneration;
      const exported = await bridge.exportWorkflow(), graph = exported.workflow;
      const serialized = JSON.stringify(graph);
      if (stopped) return;
      if (serialized !== savedGraph) {
        onStatus('saving', '正在保存实例草稿…');
        const result = await (await call('draft',{session_id:session,version,graph})).json();
        version = result.version; savedGraph = serialized;
      }
      savedGeneration = generation;
      onStatus('ready','实例草稿已保存');
    });
    saving = operation;
    try { await operation; }
    finally { if (saving === operation) saving = null; }
  }
  function saveError(error) {
    if (stopped) return;
    if ([401,403,409].includes(error.status)) { fail(error); return; }
    onStatus('error','草稿暂未保存，请保持此页并重试；也可在 ComfyUI 中导出工作流。');
  }
  function beforeUnload(event) {
    if (!stopped && ready && (saving || editGeneration !== savedGeneration)) {
      event.preventDefault(); event.returnValue = '';
    }
  }
  function close() {
    if (closing) return closing;
    closing = (async () => {
      clearInterval(saveTimer); clearTimeout(editTimer);
      if (frame) frame.inert = true;
      try {
        await save();
        if (session && !stopped) await call('close',{session_id:session});
        dispose();
      }
      catch (error) {
        if (frame) frame.inert = false;
        saveError(error);
        if (!stopped) saveTimer = setInterval(()=>void save().catch(saveError),1500);
        throw error;
      } finally { closing = null; }
    })();
    return closing;
  }
  async function events(emit) {
    while (!stopped) {
      const response = await call('events',{session_id:session});
      const reader = response.body.getReader(), decoder = new TextDecoder();
      let text = '';
      try {
        while (!stopped) {
          const chunk = await reader.read();
          if (chunk.done) break;
          text += decoder.decode(chunk.value,{stream:true});
          if (text.length > 65536) throw Error('ComfyUI 事件数据无效。');
          let end;
          while ((end = text.indexOf('\n')) !== -1) {
            const value = JSON.parse(text.slice(0,end)); text = text.slice(end+1);
            if (value.kind === 'socket') emit(value.payload);
            if (value.kind === 'error') throw Error('ComfyUI 连接中断，请重新打开。');
          }
        }
      } finally { await reader.cancel().catch(()=>{}); }
    }
  }
  let attach;
  function hello(event) {
    if (stopped || bridge || event.source !== frame?.contentWindow || event.data?.channel !== 'vf-comfyui' || event.data?.kind !== 'hello') return;
    bridge = attach(frame, {
      initialWorkflow: JSON.parse(savedGraph),
      request: async (request, signal) => (await call('request',{session_id:session,request},signal)).json(),
      subscribe: emit => { void events(emit).catch(fail); return ()=>{}; },
      onEvent: async event => {
        if (event.type === 'ready' && !ready) {
          try {
            if (stopped) return;
            ready = true; clearTimeout(loadTimer); frame.hidden = false; theme();
            onStatus('ready','实例草稿已恢复');
            saveTimer = setInterval(()=>void save().catch(saveError), 1500);
          } catch (error) { fail(error); }
        }
        if (event.type === 'editing' && ready) {
          ++editGeneration; clearTimeout(editTimer);
          editTimer = setTimeout(()=>void save().catch(saveError),250);
        }
        if (event.type === 'wheel') window.scrollBy(0,event.deltaY * (event.deltaMode === 1 ? 16 : event.deltaMode === 2 ? innerHeight : 1));
        if (event.type === 'resize' && ready) {
          const border = frame.offsetHeight - frame.clientHeight;
          frame.style.height = (Math.max(680,Math.min(event.height,10000)) + border) + 'px';
        }
      }
    });
  }
  window.addEventListener('message',hello);
  async function start() {
    onStatus('loading','正在打开 ComfyUI…');
    // A duplicated tab may inherit sessionStorage. The browser lock prevents it
    // from reusing the original tab's reconnect identity as a second writer.
    await new Promise((resolve, reject) => {
      if (!navigator.locks) { reject(Error('此浏览器不支持安全编辑会话。')); return; }
      navigator.locks.request('vf-comfyui-editor-'+alias,{ifAvailable:true},async lock => {
        if (!lock) { reject(Error(messages.EDITOR_IN_USE)); return; }
        await new Promise(release=>{releaseLock=release;resolve();});
      }).catch(reject);
    });
    if (stopped) { releaseLock?.(); return; }
    // The bridge is versioned and served with the pinned App frontend, never copied into Studio.
    const bridgeURL = '/apps/comfyui/static/vf/host-bridge.mjs';
    const module = await import(/* @vite-ignore */ bridgeURL); attach = module.attachHostBridge;
    const key = 'vf-comfyui-client-' + alias;
    let client = sessionStorage.getItem(key);
    if (!client) { client = crypto.randomUUID(); sessionStorage.setItem(key,client); }
    const opened = await (await call('open',{client_id:client})).json();
    if (stopped) return;
    session = opened.session_id; version = opened.version; savedGraph = JSON.stringify(opened.graph);
    // Reopening is an idempotent read of an existing lease. Explicitly renew
    // before loading: it may otherwise expire before the first 30s timer fires.
    await call('renew',{session_id:session});
    if (stopped) return;
    renewTimer = setInterval(()=>void call('renew',{session_id:session}).catch(fail),30000);
    loadTimer = setTimeout(()=>fail(Error('打开超时，请重试。')),30000);
    frame = document.createElement('iframe'); frame.title = 'ComfyUI'; frame.className = 'comfyui-frame';
    frame.setAttribute('sandbox','allow-scripts allow-downloads'); frame.setAttribute('referrerpolicy','no-referrer');
    frame.src = '/apps/comfyui/static/?parent_origin=' + encodeURIComponent(location.origin);
    container.replaceChildren(frame);
  }
  function dispose() {
    if (stopped) return;
    stopped = true; clearTimeout(loadTimer); clearTimeout(editTimer); clearInterval(saveTimer); clearInterval(renewTimer);
    observer.disconnect(); window.removeEventListener('message',hello); window.removeEventListener('pagehide',dispose);
    window.removeEventListener('beforeunload',beforeUnload);
    bridge?.dispose(); controller.abort(); frame?.remove();
    releaseLock?.();
    // Hard navigation retains the short lease for this tab's reconnect ID.
    // A late pagehide close must never invalidate the refreshed page's lease.
  }
  window.addEventListener('pagehide',dispose);
  window.addEventListener('beforeunload',beforeUnload);
  void start().catch(fail);
  return {dispose,save,close};
}
