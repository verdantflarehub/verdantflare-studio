import { resourceState, resourceValue, formatResource, validResourceNumber } from './resource-metrics.js';
import { mountEditor } from './comfyui-editor.js';

const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
export function readDirectory(data) {
  if (data?.schema_version !== 1 || !Array.isArray(data.instances) || data.instances.length > 1000) throw Error('实例数据无效，请重试。');
  const aliases = new Set();
  for (const item of data.instances) {
    if (!item || !/^[a-zA-Z][a-zA-Z0-9]{0,63}$/.test(item.alias) || aliases.has(item.alias)
      || typeof item.name !== 'string' || !Array.isArray(item.allowed_actions)) throw Error('实例数据无效，请重试。');
    aliases.add(item.alias);
  }
  return data.instances;
}
export function resourceMetric(label, metric, unit, now = Date.now()) {
  const state = resourceState(metric, now);
  const fresh = state === 'fresh';
  const limit = fresh && validResourceNumber(metric?.limit) && metric.limit > 0 ? metric.limit : null;
  const percent = limit === null ? null : Math.min(100, metric.value / limit * 100);
  const caption = state === 'stale' ? '采样已过期' : !fresh ? '未采集' : '采样 ' + new Date(metric.sampled_at).toLocaleTimeString('zh-CN');
  return `<div class="metric"><div class="metric-label">${label}</div><div class="metric-value">${esc(resourceValue(metric,unit,now))}${limit === null ? '' : `<small>/ ${esc(formatResource(limit,unit))}</small>`}</div><div class="meter" aria-hidden="true">${percent === null ? '' : `<i style="width:${percent}%"></i>`}</div><div class="metric-foot">${esc(caption)}</div></div>`;
}
export function instanceCard(item, now = Date.now()) {
  const age = now - Date.parse(item.observed_at), fresh = Number.isFinite(age) && age >= -5000 && age <= 30000;
  const status = fresh ? ({running:'运行中',stopped:'已停止',starting:'启动中',failed:'异常'})[item.status] || '状态未知' : '状态过期';
  const r = item.resources || {};
  const canOpen = fresh && item.status === 'running' && item.editor_state === 'ready' && item.allowed_actions?.includes('open');
  return `<article class="panel instance" aria-label="${esc(item.name)}"><div class="instance-top"><div class="identity"><div class="app-icon" aria-hidden="true"><svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.6"><path d="M4 4h6v6H4zM14 14h6v6h-6zM14 4h6v6h-6zM7 10v7h7M10 7h4"/></svg></div><div><h2>${esc(item.name)}</h2><p>ComfyUI</p></div></div><span class="badge ${fresh && item.status === 'running' ? 'running' : ''}">${status}</span></div><div class="instance-body"><div class="instance-context"><span class="kicker">应用环境</span><h3>ComfyUI</h3><p>独立的工作环境。</p><dl><div><dt>基础包</dt><dd>${item.package_profile === 'base' ? '不预装模型和第三方节点' : '待确认'}</dd></div><div><dt>首次打开</dt><dd>空白工作流</dd></div></dl></div><section class="resource-area" aria-label="实例资源占用"><div class="resource-heading"><span>资源使用</span><small>实例采样</small></div><div class="metrics">${resourceMetric('CPU 使用',r.cpu,'cores',now)}${resourceMetric('内存工作集',r.memory,'bytes',now)}${resourceMetric('显存使用',r.gpu,'bytes',now)}${resourceMetric('工作区',r.storage,'bytes',now)}</div></section></div><div class="instance-bottom"><span>${canOpen ? "" : item.editor_state === "ready" ? "实例暂不可用" : "编辑器入口尚未接入"}</span><button class="btn primary" data-open-comfyui-instance="${esc(item.alias)}" ${canOpen ? "" : "disabled"}>打开 ComfyUI →</button></div></article>`;
}

export function mountComfyUI({listen}) {
  const $ = id => document.getElementById(id), host = $('comfyuiHost'), main = host.closest('.main');
  let active = false, items = [], current = 0, pending, poll, scroll = 0, marketScroll = 0, selected = null, editor, navigation = 0;
  const route = () => new URLSearchParams(location.hash.split('?')[1] || '').get('comfyui');
  function render() {
    const q = $('comfyuiSearch').value.trim().toLowerCase();
    const visible = items.filter(item => (item.name+' '+item.alias).toLowerCase().includes(q));
    $('comfyuiCount').textContent = `我的实例 · ${items.length}`;
    $('comfyuiRows').innerHTML = visible.length ? visible.map(item => instanceCard(item)).join('')
      : `<div class="panel empty"><h2>${q ? '没有匹配的实例' : '暂无可用实例'}</h2><p>${q ? '请修改搜索条件。' : '实例准备完成并获得授权后，会显示在这里。'}</p></div>`;
  }
  async function refresh() {
    if (!active || selected) return;
    clearTimeout(poll); pending?.abort();
    const epoch = ++current, controller = new AbortController(); pending = controller;
    const timeout = setTimeout(() => controller.abort(), 12000);
    $('comfyuiRefresh').disabled = true;
    $('comfyuiMessage').hidden = false; $('comfyuiMessage').textContent = '正在获取实例…';
    try {
      const response = await fetch('/studio/apps/comfyui/instances', {credentials:'same-origin', signal:controller.signal});
      if (!response.ok) throw Error(response.status === 401 || response.status === 403 ? '登录已失效或无权访问实例。' : '暂时无法获取实例，请重试。');
      const next = readDirectory(await response.json());
      if (!active || epoch !== current) return;
      items = next; render(); $('comfyuiMessage').hidden = true;
    } catch (error) {
      if (!active || epoch !== current) return;
      items = []; $('comfyuiRows').replaceChildren(); $('comfyuiCount').textContent = '我的实例 · —';
      $('comfyuiMessage').textContent = controller.signal.aborted ? '请求超时，请刷新重试。' : error instanceof SyntaxError ? '实例数据无效，请重试。' : error.message;
    } finally {
      clearTimeout(timeout);
      if (active && !selected && epoch === current) { pending = null; $('comfyuiRefresh').disabled = false; if (!document.hidden) poll = setTimeout(refresh, 15000); }
    }
  }
  function close() {
    if (!active) return;
    if (!selected) scroll = window.scrollY;
    editor?.dispose(); editor = null; selected = null;
    active = false; ++current; clearTimeout(poll); pending?.abort();
    host.hidden = true; main.classList.remove('comfyui-active');
    if (!/[?&](blender|image|video)=/.test(location.hash)) {
      $('marketView').hidden = false; $('marketView').style.display = '';
      $('pageTitle').textContent = '应用市场'; $('pageSubtitle').textContent = '为 Station 安装创作应用，管理模型与运行状态。';
      requestAnimationFrame(() => { if (!active) window.scrollTo(0,marketScroll); });
    }
  }
  async function sync() {
    const target = route();
    const epoch = ++navigation;
    if (editor && target !== selected) {
      const previous = editor;
      try { await previous.close(); }
      catch {
        if (epoch === navigation) location.hash = '/market?comfyui=' + encodeURIComponent(selected);
        return;
      }
      if (editor === previous) editor = null;
      if (epoch !== navigation) return;
    }
    if (target !== 'instances' && !/^comfyui[A-Za-z0-9]{1,57}$/.test(target || '')) { close(); return; }
    if (!active) marketScroll = window.scrollY;
    active = true; host.hidden = false; main.classList.add('comfyui-active');
    $('marketView').hidden = true; $('marketView').style.display = 'none';
    $('pageTitle').textContent = 'ComfyUI'; $('pageSubtitle').textContent = '选择实例，打开 ComfyUI。';
    const instance = target === 'instances' ? null : target;
    if (selected !== instance) {
      if (!selected) scroll = window.scrollY;
      editor?.dispose(); editor = null;
      ++current; pending?.abort(); clearTimeout(poll); selected = instance;
    }
    $('comfyuiDirectory').hidden = !!instance; $('comfyuiWorkspace').hidden = !instance;
    $('comfyuiInstanceCrumb').hidden = !instance;
    $('comfyuiInstanceName').textContent = instance ? (items.find(item=>item.alias===instance)?.name || 'ComfyUI '+instance.slice(7)) : '';
    $('comfyuiRefresh').disabled = false;
    if (instance) {
      if (!editor) {
        $('comfyuiFrameHost').replaceChildren();
        editor = mountEditor({alias:instance,container:$('comfyuiFrameHost'),onStatus:(state,message)=>{
          $('comfyuiEditorMessage').textContent = message;
          $('comfyuiEditorMessage').dataset.state = state;
        }});
        window.scrollTo(0,0);
      }
    } else {
      requestAnimationFrame(()=>{if(active&&!selected)window.scrollTo(0,scroll);});
      void refresh();
    }
  }
  listen($('comfyuiSearch'),'input',render);
  listen($('comfyuiRefresh'),'click',async()=>{
    if (!selected) { void refresh(); return; }
    const previous = editor;
    $('comfyuiRefresh').disabled = true;
    try { await previous?.close(); if (editor === previous) editor = null; await sync(); }
    catch { /* The editor preserves the graph and explains the failed save. */ }
    finally { $('comfyuiRefresh').disabled = false; }
  });
  listen($('comfyuiRows'),'click',event=>{const button=event.target.closest('[data-open-comfyui-instance]');if(button&&!button.disabled)location.hash='/market?comfyui='+encodeURIComponent(button.dataset.openComfyuiInstance);});
  listen($('comfyuiTheme'),'click',() => $('theme').click());
  listen(window,'hashchange',sync);
  listen(document,'visibilitychange',() => { if (!active || selected) return; if (document.hidden) clearTimeout(poll); else void refresh(); });
  return {sync,open:()=>{location.hash='/market?comfyui=instances';},dispose:close};
}
