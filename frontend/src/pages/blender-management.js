// Production views of the approved Blender v0.2 ClaudeStyle prototype. No fixture fallback.
import { formatResource, resourceState, resourceValue } from './resource-metrics.js';
const esc = value => String(value ?? '').replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
const states = {running:'运行中',stopped:'已停止',created:'未启动',starting:'启动中',stopping:'停止中',creating:'创建中',deleting:'销毁中',failed:'异常',unknown:'状态未知'};
export const instanceName = item => item.name || item.alias.replace(/^blender/, 'Blender ');
export const instanceURL = (item, panel = '') => '#/market?blender=' + encodeURIComponent(item.alias) + (panel ? '&panel=' + panel : '');
export function isFresh(item, now) {
  const sampled = Date.parse(item?.observed_at);
  if (!Number.isFinite(sampled)) return false;
  if (now === undefined) return true;
  return Number.isFinite(now) && now - sampled >= -5000 && now - sampled < 30000;
}
export function canOpen(item, now) {
  return isFresh(item, now) && item.status === 'running' && item.control?.state === 'idle' && item.access === 'edit' && item.allowed_actions?.includes('open');
}
export function canStart(item, now) {
  return isFresh(item, now) && item.status === 'stopped' && item.can_manage === true && item.access === 'edit'
    && !item.management_operation_id && Number.isSafeInteger(item.state_version) && item.state_version > 0
    && item.allowed_actions?.includes('start');
}
export function canStop(item, now) {
  return isFresh(item, now) && item.status === 'running' && item.control?.state === 'idle'
    && item.can_manage === true && item.access === 'edit' && !item.management_operation_id
    && Number.isSafeInteger(item.state_version) && item.state_version > 0 && item.allowed_actions?.includes('stop');
}
export function canDestroy(item, now) {
  return isFresh(item, now) && item.status === 'stopped' && item.can_manage === true && item.access === 'edit'
    && !item.management_operation_id && Number.isSafeInteger(item.state_version) && item.state_version > 0
    && item.allowed_actions?.includes('destroy');
}
const status = (item, now) => isFresh(item, now) ? states[item.status] || states.unknown : states.unknown;
const role = item => item.can_manage ? '管理员' : item.access === 'edit' ? '可编辑' : '只读';
const project = item => item.project_name || '关联项目名称暂不可用';
const when = value => value && Number.isFinite(Date.parse(value)) ? new Date(value).toLocaleString('zh-CN') : '暂无记录';
function badgeClass(item, now) {
  if (!isFresh(item, now)) return 'failed';
  if (item.status === 'running') return 'running';
  if (['starting','stopping','creating','deleting'].includes(item.status)) return 'working ' + item.status;
  if (['failed','unknown'].includes(item.status)) return 'failed';
  return '';
}
function badge(item, now) {
  const cls = badgeClass(item, now);
  const busyExtra = isFresh(item, now) && item.status === 'running' && item.control?.state === 'busy'
    ? ` · ${item.control.mode === 'gui' ? '桌面使用中' : 'MCP 使用中'}`
    : '';
  return `<span data-vf="Pill" class="badge ${cls}"><span class="dot"></span>${status(item, now)}${esc(busyExtra)}</span>`;
}
function control(item, now) {
  if (isFresh(item, now) && ['stopped','creating','starting','stopping','deleting'].includes(item.status)) return '实例未开放连接';
  if (!isFresh(item, now) || item.control?.state === 'unknown') return '控制权待确认';
  return item.control?.state === 'busy' ? (item.control.mode === 'gui' ? '桌面正在使用' : 'MCP 正在使用') : '空闲 · 可连接';
}
const actionIcons = {
  destroy: '<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M3 6h18M9 6V3h6v3M5 6l1 15h12l1-15M10 10v7M14 10v7"/></svg>',
  stop: '<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect x="6" y="6" width="12" height="12" rx="2"/></svg>',
  play: '<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="m8 5 11 7-11 7V5Z"/></svg>',
  spin: '<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21 12a9 9 0 1 1-6.2-8.56"/></svg>',
  open: '<svg class="icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/></svg>',
};
function openAction(item, now) {
  if (item.access !== 'edit') return '<span class="muted">只读</span>';
  if (canOpen(item, now)) return `<a data-vf="Button" class="btn g-icon-button primary" href="${instanceURL(item)}" title="打开 ${esc(instanceName(item))}" aria-label="打开 ${esc(instanceName(item))}">${actionIcons.open}</a>`;
  return `<button type="button" data-vf="Button" class="btn g-icon-button" disabled title="实例状态、编辑权限或控制权暂不允许连接" aria-label="打开">${actionIcons.open}</button>`;
}
function lifecycle(item, now) {
  if (!item.can_manage) return '';
  const name = esc(instanceName(item));
  const destroy = canDestroy(item, now) ? `<button type="button" data-vf="Button" class="btn g-icon-button quiet danger" data-blender-destroy="${esc(item.alias)}" title="销毁 ${name}" aria-label="销毁 ${name}">${actionIcons.destroy}</button>` : `<button type="button" data-vf="Button" class="btn g-icon-button quiet danger" disabled title="实例销毁暂未开放" aria-label="销毁">${actionIcons.destroy}</button>`;
  if (canStart(item, now)) return destroy+`<button type="button" data-vf="Button" class="btn g-icon-button primary" data-blender-start="${esc(item.alias)}" title="启动 ${name}" aria-label="启动 ${name}">${actionIcons.play}</button>`;
  if (canDestroy(item, now)) return destroy+`<button type="button" data-vf="Button" class="btn g-icon-button" disabled title="启动" aria-label="启动">${actionIcons.play}</button>`;
  if (canStop(item, now)) return `<button type="button" data-vf="Button" class="btn g-icon-button quiet danger" disabled title="请先停止实例，再销毁" aria-label="销毁">${actionIcons.destroy}</button><button type="button" data-vf="Button" class="btn g-icon-button" data-blender-stop="${esc(item.alias)}" title="停止 ${name}" aria-label="停止 ${name}">${actionIcons.stop}</button>`;
  const busy = ['starting','stopping','creating','deleting'].includes(item.status);
  const label = item.status === 'stopped' ? '启动' : '停止';
  return `<button type="button" data-vf="Button" class="btn g-icon-button quiet danger" disabled title="实例销毁暂未开放" aria-label="销毁">${actionIcons.destroy}</button><button type="button" data-vf="Button" class="btn g-icon-button" disabled title="实例生命周期管理暂未开放" aria-label="${label}"${busy ? ' aria-busy="true"' : ''}>${busy ? actionIcons.spin : item.status === 'stopped' ? actionIcons.play : actionIcons.stop}</button>`;
}
function meterBar(metric, label, now) {
  if (resourceState(metric, now) !== 'fresh') return '';
  const limit = Number.isFinite(metric?.limit) && metric.limit > 0
    ? metric.limit
    : (Number.isFinite(metric?.request) && metric.request > 0 ? metric.request : null);
  if (!limit || typeof metric?.value !== 'number' || !Number.isFinite(metric.value) || metric.value < 0) return '';
  const pct = Math.min(100, Math.max(0, Math.round((metric.value / limit) * 100)));
  const cls = pct >= 90 ? 'danger' : pct >= 75 ? 'warn' : '';
  return `<div class="vf-meter ${cls}" role="progressbar" aria-label="${esc(label)}" aria-valuemin="0" aria-valuemax="100" aria-valuenow="${pct}"><span style="transform:scaleX(${Math.min(1, pct / 100)})"></span></div>`;
}
function resourceCell(metric, unit, label, now) {
  const val = esc(resourceValue(metric, unit, now));
  return `<td class="cell-resource num"><strong>${val}</strong>${meterBar(metric, label, now)}</td>`;
}
function metric(label, metric, unit, now) {
  const state = resourceState(metric, now);
  const caption = state === 'stale' ? '采样已过期' : state === 'unavailable' ? '未采集' : '采样 ' + when(metric.sampled_at);
  return `<div><div class="metric-label">${label}</div><div class="metric-value">${esc(resourceValue(metric,unit, now))}</div><div class="meter" aria-hidden="true"></div><div class="metric-foot">${esc(caption)}</div></div>`;
}
function metrics(item, now) {
  const r = item.resources || {};
  return metric('CPU 使用', r.cpu, 'cores', now) + metric('内存工作集', r.memory, 'bytes', now) + metric('显存使用', r.gpu, 'bytes', now) + metric('工作区', r.storage, 'bytes', now);
}
function configuration(item, now) {
  if (!isFresh({observed_at:item.resources_observed_at}, now)) return '待资源绑定确认';
  const r = item.resources || {};
  return '请求 ' + formatResource(r.cpu?.request) + ' / ' + formatResource(r.memory?.request,'bytes');
}
function resourceNote(item) {
  return item.resources_observed_at ? '配置观测 ' + when(item.resources_observed_at) : '资源绑定待确认';
}
export function filterCounts(items, now) {
  return {
    all: items.length,
    running: items.filter(i => isFresh(i, now) && i.status === 'running').length,
    stopped: items.filter(i => isFresh(i, now) && ['stopped', 'created'].includes(i.status)).length,
    failed: items.filter(i => !isFresh(i, now) || ['unknown', 'failed'].includes(i.status)).length,
  };
}
export function summary(items, now) {
  const count = items.length, known = items.every(i => isFresh(i, now) && i.status !== 'unknown');
  const gpuKnown = items.every(i => isFresh(i, now) && Number.isInteger(i.allocated_gpu_count) && i.allocated_gpu_count >= 0);
  const copiesKnown = items.every(i => typeof i.workspace?.registered === 'boolean');
  const values = [['可访问实例',count,'个工作环境'],['正在运行',known ? items.filter(i => i.status === 'running').length : '—','场景就绪观测'],['已分配 GPU',gpuKnown ? items.reduce((n,i) => n+i.allocated_gpu_count,0) : '—','独占 GPU 数量'],['已登记工作区',copiesKnown ? items.filter(i => i.workspace.registered).length : '—','持久工作副本']];
  return values.map(([label,value,note]) => `<div><dt>${label}</dt><dd><strong>${value}</strong><small>${note}</small></dd></div>`).join('');
}
export function instanceCard(item, now) {
  const r = item.resources || {};
  return `<tr aria-label="${esc(instanceName(item))}">` +
    `<td><div class="cell-name"><div class="app-icon"><img src="/assets/blender-logo.svg" alt=""></div><div class="cell-name-text"><a class="instance-name" href="${instanceURL(item,'overview')}">${esc(instanceName(item))}</a><small class="instance-sub num">${esc(item.alias)} · ${esc(project(item))} · ${esc(configuration(item, now))} · ${role(item)}</small></div></div></td>` +
    `<td>${badge(item, now)}</td>` +
    resourceCell(r.cpu, 'cores', 'CPU 使用率', now) +
    resourceCell(r.memory, 'bytes', '内存使用率', now) +
    resourceCell(r.gpu, 'bytes', 'GPU 显存使用率', now) +
    `<td class="col-actions"><div class="cell-actions">${lifecycle(item, now)}${openAction(item, now)}</div></td>` +
    `</tr>`;
}
export function detail(item, panel, now) {
  const row = (k,v) => `<div><dt>${k}</dt><dd>${esc(v)}</dd></div>`;
  const tabs = `<div class="tabs">${[['overview','概览'],['resources','资源'],['activity','活动']].map(([value,label]) => `<a href="${instanceURL(item,value)}" ${panel === value ? 'aria-current="page"' : ''}>${label}</a>`).join('')}</div>`;
  if (panel === 'resources') return tabs+`<section class="panel pad"><div class="section-head"><h2>资源配置与使用</h2><small>${esc(resourceNote(item))}</small></div><div class="metrics">${metrics(item, now)}</div><dl class="definition">${row('CPU 请求 / 上限',isFresh({observed_at:item.resources_observed_at}, now) ? formatResource(item.resources?.cpu?.request) + ' / ' + formatResource(item.resources?.cpu?.limit) : '待确认')}${row('内存请求 / 上限',isFresh({observed_at:item.resources_observed_at}, now) ? formatResource(item.resources?.memory?.request,'bytes') + ' / ' + formatResource(item.resources?.memory?.limit,'bytes') : '待确认')}${row('存储预留',formatResource(item.workspace?.reserved_bytes,'bytes'))}${row('计量范围','实例容器 CPU、内存工作集、独占显存与工作区文件')}</dl><p class="state-note">显存按已绑定的独占 GPU 采集；工作区统计文件字节，不含用户目录。存储预留不是硬配额，停止和销毁后继续保留。缺少或过期样本显示“—”。</p></section>`;
  if (panel === 'activity') return tabs+`<section class="panel pad"><div class="section-head"><h2>实例活动</h2></div>${item.save?.revision_id ? `<p>工程已保存到项目</p><dl class="definition">${row('确认时间',when(item.save.last_saved_at))}${row('项目修订',item.save.revision_id)}</dl>` : '<p class="muted">暂无已确认的保存记录。</p>'}<p class="state-note">完整生命周期活动尚未接入。</p></section>`;
  return tabs+`<div class="detail-grid"><section class="panel pad"><div class="section-head"><h2>运行概况</h2>${badge(item, now)}</div><dl class="definition">${row('关联项目',project(item))}${row('访问权限',role(item))}${row('当前控制权',control(item, now))}${row('状态来源',item.status_source === 'instance_ledger' ? '实例管理操作' : '实时场景检查')}${row('观测时间',when(item.observed_at))}</dl><p class="state-note">${item.status === 'unknown' ? '无法确认实例运行状态，请刷新重试。' : '关闭页面或释放编辑控制权不会停止实例。'} ${(canStart(item, now) || canStop(item, now) || canDestroy(item, now)) ? "" : "当前状态不支持启停或销毁。"}</p><div class="actions">${lifecycle(item, now)}${openAction(item, now)}</div></section><section class="panel pad"><div class="section-head"><h2>工程与保存</h2></div><dl class="definition">${row('当前工程状态','待确认')}${row('最近确认保存',when(item.save?.last_saved_at))}${row('工作副本',item.workspace?.registered === true ? '已登记' : '待确认')}${row('工作副本基线',item.workspace?.revision_id || '暂无记录')}</dl><p class="state-note">历史保存记录不代表当前内存中的更改已经保存。</p></section></div>`;
}

