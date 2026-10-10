// Production views of the approved Blender v0.1 prototype. No fixture fallback.
import { formatResource, resourceState, resourceValue } from './resource-metrics.js';
const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const states = {running:'运行中',stopped:'已停止',starting:'启动中',stopping:'停止中',creating:'创建中',deleting:'销毁中',failed:'异常',unknown:'状态未知'};
export const instanceName = item => item.name || item.alias.replace(/^blender/, 'Blender ');
export const instanceURL = (item, panel = '') => '#/market?blender=' + encodeURIComponent(item.alias) + (panel ? '&panel=' + panel : '');
export function isFresh(item, now = Date.now()) {
  const sampled = Date.parse(item.observed_at);
  return Number.isFinite(sampled) && now - sampled >= -5000 && now - sampled <= 30000;
}
export function canOpen(item) {
  return isFresh(item) && item.status === 'running' && item.control?.state === 'idle' && item.access === 'edit' && item.allowed_actions?.includes('open');
}
const status = item => isFresh(item) ? states[item.status] || states.unknown : '状态过期';
const role = item => item.can_manage ? '管理员' : item.access === 'edit' ? '可编辑' : '只读';
const project = item => item.project_name || '关联项目名称暂不可用';
const when = value => value && Number.isFinite(Date.parse(value)) ? new Date(value).toLocaleString('zh-CN') : '暂无记录';
const badge = item => `<span class="badge ${isFresh(item) && item.status === 'running' ? 'running' : ''}">${status(item)}</span>`;
function control(item) {
  if (isFresh(item) && ['stopped','creating','starting','stopping','deleting'].includes(item.status)) return '实例未开放连接';
  if (!isFresh(item) || item.control?.state === 'unknown') return '控制权待确认';
  return item.control?.state === 'busy' ? (item.control.mode === 'gui' ? '桌面正在使用' : 'MCP 正在使用') : '空闲 · 可连接';
}
function openAction(item) {
  if (item.access !== 'edit') return '';
  if (isFresh(item) && ['stopped','creating','starting','stopping','deleting'].includes(item.status)) return '';
  if (canOpen(item)) return `<a class="btn primary" href="${instanceURL(item)}" aria-label="打开 ${esc(instanceName(item))}">打开 →</a>`;
  return '<button class="btn" disabled title="实例状态、编辑权限或控制权暂不允许连接">打开</button>';
}
function lifecycle(item) {
  if (!item.can_manage) return '';
  // Command endpoints are not implemented yet. Never infer a command from status.
  return `<button class="btn danger" disabled title="实例销毁暂未开放">销毁</button><button class="btn" disabled title="实例生命周期管理暂未开放">${item.status === 'stopped' ? '启动' : '停止'}</button>`;
}
function metric(label, metric, unit) {
  const state = resourceState(metric);
  const caption = state === 'stale' ? '采样已过期' : state === 'unavailable' ? '未采集' : '采样 ' + when(metric.sampled_at);
  return `<div><div class="metric-label">${label}</div><div class="metric-value">${esc(resourceValue(metric,unit))}</div><div class="meter" aria-hidden="true"></div><div class="metric-foot">${esc(caption)}</div></div>`;
}
function metrics(item) {
  const r = item.resources || {};
  return metric('CPU 使用', r.cpu, 'cores') + metric('内存工作集', r.memory, 'bytes') + metric('显存使用', r.gpu, 'bytes') + metric('工作区', r.storage, 'bytes');
}
function configuration(item) {
  if (!isFresh({observed_at:item.resources_observed_at})) return '待资源绑定确认';
  const r = item.resources || {};
  return '请求 ' + formatResource(r.cpu?.request) + ' / ' + formatResource(r.memory?.request,'bytes');
}
function resourceNote(item) {
  return item.resources_observed_at ? '配置观测 ' + when(item.resources_observed_at) : '资源绑定待确认';
}
export function summary(items) {
  const count = items.length, known = items.every(i => isFresh(i) && i.status !== 'unknown');
  const gpuKnown = items.every(i => isFresh(i) && Number.isInteger(i.allocated_gpu_count) && i.allocated_gpu_count >= 0);
  const copiesKnown = items.every(i => typeof i.workspace?.registered === 'boolean');
  const values = [['可访问实例',count,'个工作环境'],['正在运行',known ? items.filter(i => i.status === 'running').length : '—','场景就绪观测'],['已分配 GPU',gpuKnown ? items.reduce((n,i) => n+i.allocated_gpu_count,0) : '—','实际分配待确认'],['已登记工作区',copiesKnown ? items.filter(i => i.workspace.registered).length : '—','持久工作副本']];
  return values.map(([label,value,note]) => `<div><dt>${label}</dt><dd><strong>${value}</strong><small>${note}</small></dd></div>`).join('');
}
export function instanceCard(item) {
  return `<article class="panel instance" aria-label="${esc(instanceName(item))}"><div class="instance-top"><div class="identity"><div class="app-icon"><img src="/assets/blender-logo.svg" alt=""></div><div><a class="instance-name" href="${instanceURL(item,'overview')}">${esc(instanceName(item))}</a><small>${role(item)}</small></div></div>${badge(item)}</div><div class="instance-body"><div class="instance-context"><div class="kicker">关联项目</div><div class="project-line">${esc(project(item))}</div><dl class="context-meta"><div><dt>控制权</dt><dd>${esc(control(item))}</dd></div><div><dt>计算配置</dt><dd>${esc(configuration(item))}</dd></div><div><dt>工程状态</dt><dd>当前保存状态待确认</dd></div></dl></div><div class="resource-area"><div class="resource-heading"><span>资源使用</span><small>${esc(resourceNote(item))}</small></div><div class="metrics">${metrics(item)}</div></div></div><div class="instance-bottom"><span class="save-status">最近确认保存 ${esc(when(item.save?.last_saved_at))}</span><div class="actions">${lifecycle(item)}${openAction(item)}</div></div></article>`;
}
export function detail(item, panel) {
  const row = (k,v) => `<div><dt>${k}</dt><dd>${esc(v)}</dd></div>`;
  const tabs = `<div class="tabs">${[['overview','概览'],['resources','资源'],['activity','活动']].map(([value,label]) => `<a href="${instanceURL(item,value)}" ${panel === value ? 'aria-current="page"' : ''}>${label}</a>`).join('')}</div>`;
  if (panel === 'resources') return tabs+`<section class="panel pad"><div class="section-head"><h2>资源配置与使用</h2><small>${esc(resourceNote(item))}</small></div><div class="metrics">${metrics(item)}</div><dl class="definition">${row('CPU 请求 / 上限',isFresh({observed_at:item.resources_observed_at}) ? formatResource(item.resources?.cpu?.request) + ' / ' + formatResource(item.resources?.cpu?.limit) : '待确认')}${row('内存请求 / 上限',isFresh({observed_at:item.resources_observed_at}) ? formatResource(item.resources?.memory?.request,'bytes') + ' / ' + formatResource(item.resources?.memory?.limit,'bytes') : '待确认')}${row('计量范围','实例容器 CPU 与内存工作集')}</dl><p class="state-note">显存用量待独占设备绑定确认；工作区存储尚未采集。缺少配置或使用量时显示“—”。</p></section>`;
  if (panel === 'activity') return tabs+`<section class="panel pad"><div class="section-head"><h2>实例活动</h2></div>${item.save?.revision_id ? `<p>工程已保存到项目</p><dl class="definition">${row('确认时间',when(item.save.last_saved_at))}${row('项目修订',item.save.revision_id)}</dl>` : '<p class="muted">暂无已确认的保存记录。</p>'}<p class="state-note">完整生命周期活动尚未接入。</p></section>`;
  return tabs+`<div class="detail-grid"><section class="panel pad"><div class="section-head"><h2>运行概况</h2>${badge(item)}</div><dl class="definition">${row('关联项目',project(item))}${row('访问权限',role(item))}${row('当前控制权',control(item))}${row('状态来源',item.status_source === 'instance_ledger' ? '实例管理操作' : '实时场景检查')}${row('观测时间',when(item.observed_at))}</dl><p class="state-note">${item.status === 'unknown' ? '无法确认实例运行状态，请刷新重试。' : '关闭页面或释放编辑控制权不会停止实例。'} 实例生命周期管理暂未开放。</p><div class="actions">${lifecycle(item)}${openAction(item)}</div></section><section class="panel pad"><div class="section-head"><h2>工程与保存</h2></div><dl class="definition">${row('当前工程状态','待确认')}${row('最近确认保存',when(item.save?.last_saved_at))}${row('工作副本',item.workspace?.registered === true ? '已登记' : '待确认')}${row('工作副本基线',item.workspace?.revision_id || '暂无记录')}</dl><p class="state-note">历史保存记录不代表当前内存中的更改已经保存。</p></section></div>`;
}
