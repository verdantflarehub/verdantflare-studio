import { mountVideo } from './video-embed'
import { mountImage } from './image-embed'
import { request } from '../platform/client'
import { brandMarks } from './brands'
import { mergeCatalogWithLive } from './catalog'

// Studio Market interaction adapter aligned with verdantflare_studio_market_v1.1.html
export function mountMarket() {
  const controller = new AbortController();
  const listen = (target, event, handler) => target.addEventListener(event, handler, { signal: controller.signal });

  const $ = id => document.getElementById(id), labels = { image: 'Image', music: 'Music', video: 'Video' };
  const statusText = {
    ready: '已就绪',
    running: '运行中',
    stopped: '已关闭',
    stopping: '关闭中',
    starting: '启动中',
    warming: '预热中',
    installing: '部署中',
    not_installed: '未安装',
    absent: '未安装',
    degraded: '运行异常',
    failed: '异常',
    unknown: '状态未知'
  };
  const esc = x => String(x ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]));

  let identity = {};
  const operations = new Map();
  let operationPolling = false;
  const terminal = op => op && ['succeeded', 'failed'].includes(op.status);
  const phases = {
    checking: '检查中',
    downloading: '下载中',
    verifying: '校验中',
    installing: '安装中',
    starting: '启动中',
    warming: '预热中',
    stopping: '关闭中',
    deleting: '删除中',
    completed: '完成'
  };

  let apps = mergeCatalogWithLive([]), group = 'all', tab = 'all', selected = null, connected = false, mode = 'market', refreshing = false;
  const installed = a => !['not_installed', 'absent', 'unknown'].includes(a.deployment?.state);
  const active = a => ['starting', 'stopping', 'installing', 'warming'].includes(a.deployment?.state) || (operations.has(a.app_id) && !terminal(operations.get(a.app_id)?.op));

  function brandInfo(a) {
    if (a.brand === 'vf' || a.app_id.endsWith('-mcp-server')) return { key: 'vf', name: 'VF 官方', title: 'VerdantFlare 官方微服务应用' };
    if (a.brand === 'minimax' || ['music-minimax-music3-api', 'video-minimax-h3-api', 'video-minimax-h3-vdn', 'video-minimax-h3-singularity'].includes(a.app_id)) {
      return { key: 'minimax', name: 'MiniMax', title: 'MiniMax 官方模型引擎' };
    }
    return { key: 'github', name: 'GitHub', title: '开源算法应用' };
  }

  function brandLabel(a) {
    const b = brandInfo(a);
    const contents = `<span>${esc(b.name)}</span>`;
    return b.key === 'vf'
      ? `<a class="publisher" data-brand="vf" title="${esc(b.title)}" href="https://github.com/verdantflarehub" target="_blank" rel="noopener">${contents}</a>`
      : `<span class="publisher" data-brand="${esc(b.key)}" title="${esc(b.title)}">${contents}</span>`;
  }

  function icon(a) {
    const b = brandInfo(a);
    return `<span class="app-brand-mark" data-logo="${esc(b.key)}" role="img" aria-label="${esc(b.title)}">${brandMarks[b.key] || brandMarks.github}</span>`;
  }

  function badge(a) {
    const state = a.deployment?.state || 'not_installed';
    const isRunning = state === 'ready' || state === 'running';
    const isDegraded = state === 'degraded' || state === 'failed';
    const isBusy = ['starting', 'stopping', 'installing', 'warming'].includes(state);
    return `<span class="status ${isRunning ? 'running' : isDegraded ? 'failed' : isBusy ? 'busy' : ''}"><i class="dot ${isRunning ? 'on' : ''}"></i>${esc(statusText[state] || '未安装')}</span>`;
  }

  function primaryBtn(a) {
    const op = operations.get(a.app_id);
    const isBusy = op && !op.rejected && !terminal(op.op);
    if (isBusy) {
      const phaseName = phases[op.op?.phase] || op.op?.phase || '处理中';
      return `<button class="btn" disabled>${esc(phaseName)}</button>`;
    }
    const state = a.deployment?.state || 'not_installed';
    if (state === 'ready' || state === 'running') {
      if (a.app_id === 'video-mcp-server') {
        return '<button class="btn primary" data-open-video="true">打开</button>';
      }
      if (a.app_id === 'image-mcp-server') {
        return '<button class="btn primary" data-open-image="true">打开</button>';
      }
      return `<button class="btn" data-detail="${esc(a.app_id)}">管理</button>`;
    }
    if (state === 'stopped') {
      return `<button class="btn primary" data-action="start" data-id="${esc(a.app_id)}">启动</button>`;
    }
    if (state === 'degraded' || state === 'failed') {
      return `<button class="btn primary" data-action="start" data-id="${esc(a.app_id)}">重试</button>`;
    }
    // not_installed or absent
    return `<button class="btn primary" data-action="install" data-id="${esc(a.app_id)}">安装</button>`;
  }

  function notice(message) {
    $('toast').textContent = message;
    $('toast').classList.add('show');
    setTimeout(() => $('toast').classList.remove('show'), 3500);
  }

  function login() {
    video.close();
    image.close();
    connected = false;
    identity = {};
    operations.clear();
    apps = mergeCatalogWithLive([]);
    render();
    if (!$('loginDialog').open) $('loginDialog').showModal();
  }

  async function api(path, options = {}) {
    const r = await request({ path, method: options.method || 'GET', body: options.body ? JSON.parse(options.body) : undefined });
    const data = r.status === 204 ? {} : await r.json();
    if (r.status === 401 && path !== 'login') {
      login();
      const error = Error('请先登录');
      error.unauthorized = true;
      throw error;
    }
    if (!r.ok) {
      const error = Error(({ ACCOUNT_LOCKED: '登录失败次数过多，请稍后再试', UNAUTHENTICATED: '用户名或密码不正确', SERVICE_UNAVAILABLE: 'Station 暂时无法连接' }[data.code]) || data.message || '请求失败');
      error.status = r.status;
      throw error;
    }
    return data;
  }

  const video = mountVideo({ listen, api, notice, login });
  const image = mountImage({ listen, api, notice, login });

  function render() {
    const searchInput = $('search');
    const q = searchInput ? searchInput.value.trim().toLowerCase() : '';
    const shown = apps.filter(a =>
      (group === 'all' || a.group_id === group) &&
      (tab === 'all' || (tab === 'installed' && installed(a)) || (tab === 'active' && active(a))) &&
      [a.display_name, a.description, ...a.models, ...(a.capabilities || [])].join(' ').toLowerCase().includes(q)
    );
    const count = apps.filter(installed).length;

    if ($('allCount')) $('allCount').textContent = connected ? apps.length : '—';
    if ($('installedCount')) $('installedCount').textContent = connected ? count : '—';
    if ($('activeCount')) $('activeCount').textContent = connected ? apps.filter(active).length : '—';
    if ($('resultCount')) $('resultCount').textContent = connected ? `${shown.length} 个应用` : '等待连接';

    // Left Sidebar Host Pod
    const clusterPill = document.querySelector('.side-cluster-pill');
    if (clusterPill) clusterPill.textContent = connected ? '在线' : '未连接';
    const clusterDot = document.querySelector('.side-cluster-lead i.dot');
    if (clusterDot) clusterDot.className = `dot ${connected ? 'on' : ''}`;
    const clusterMeta = document.querySelector('.side-cluster-meta');
    if (clusterMeta) clusterMeta.textContent = connected ? 'Core 就绪 · 8 卡 RTX 5090' : '等待连接 Station';

    const userName = document.querySelector('.side-user-name');
    if (userName) userName.textContent = identity.user_id || 'admin';
    const userTeam = document.querySelector('.side-user-team');
    if (userTeam) userTeam.textContent = identity.organization_name || '青岚创意工作室';

    // Tabs & Filters
    document.querySelectorAll('[data-group]').forEach(b => b.classList.toggle('active', b.dataset.group === group));
    document.querySelectorAll('.tabs [data-tab]').forEach(b => b.classList.toggle('active', b.dataset.tab === tab));

    if ($('groupNav')) {
      $('groupNav').innerHTML = Object.keys(labels).map(g => `<button class="group-nav ${group === g ? 'active' : ''}" data-group="${g}">${labels[g]}<span class="count">${apps.filter(a => a.group_id === g).length}</span></button>`).join('');
    }

    // Catalog Rendering strictly matching verdantflare_studio_market_v1.1.html
    if ($('catalog')) {
      if (!connected) {
        $('catalog').innerHTML = '<div class="empty">登录后查看 Station 中的真实应用。</div>';
      } else if (!shown.length) {
        $('catalog').innerHTML = '<div class="empty"><h3>没有符合条件的应用</h3><p>试试切换分类标签，或清空搜索关键词。</p></div>';
      } else {
        $('catalog').innerHTML = Object.keys(labels).map(g => {
          const list = shown.filter(a => a.group_id === g);
          if (!list.length) return '';
          return `<section>
            <div class="section-title">
              <h2>${labels[g]}</h2>
              <small>${list.length} 个应用</small>
              <span class="line"></span>
            </div>
            <div class="grid">${list.map(a => {
              const gpuTag = a.gpu ? `<span class="tag gpu">${a.gpu} GPU 节点</span>` : '<span class="tag">1 CPU 节点</span>';
              const modelTag = (a.models && a.models.length) ? `<span class="tag">${a.models.length} 项本地模型</span>` : '';

              return `<article class="app-card" data-card="${esc(a.app_id)}">
                <div class="card-top">
                  <button type="button" class="app-icon app-detail-trigger ${esc(g)}" data-detail="${esc(a.app_id)}" aria-label="查看 ${esc(a.display_name)} 详情">${icon(a)}</button>
                  <div>
                    <button class="app-title" data-detail="${esc(a.app_id)}">${esc(a.display_name)}</button>
                    <div class="brand-version">
                      ${brandLabel(a)}
                      <span class="version-divider">·</span>
                      <span>v${esc(a.version)}</span>
                    </div>
                  </div>
                </div>
                <p class="card-desc">${esc(mode === 'models' ? (a.models && a.models.length ? a.models.join(' · ') : '无需本地模型') : a.description || (a.models && a.models.length ? '模型：' + a.models.join(' · ') : '提供应用入口，无本地模型依赖'))}</p>
                <div class="tags">${gpuTag}${modelTag}</div>
                <div class="card-foot">
                  ${badge(a)}
                  <div class="card-buttons">
                    ${primaryBtn(a)}
                  </div>
                </div>
              </article>`;
            }).join('')}</div>
          </section>`;
        }).join('');
      }
    }

    // Viewport Mode Switching: Market vs Resources vs Embedded Viewports
    const marketView = $('marketView');
    const resourceView = $('resourceView');
    const videoHost = $('videoHost');
    const imageHost = $('imageHost');
    const isEmbedActive = location.hash.includes('video=') ||
                          location.hash.includes('image=') ||
                          (videoHost && !videoHost.hidden) ||
                          (imageHost && !imageHost.hidden);

    if (isEmbedActive) {
      if (marketView) {
        marketView.hidden = true;
        marketView.style.display = 'none';
      }
      if (resourceView) {
        resourceView.hidden = true;
        resourceView.style.display = 'none';
      }
    } else {
      if (mode === 'resources') {
        if (marketView) {
          marketView.hidden = true;
          marketView.style.display = 'none';
        }
        if (resourceView) {
          resourceView.hidden = false;
          resourceView.style.display = 'block';
        }
        if ($('pageTitle')) $('pageTitle').textContent = '资源管理';
        if ($('pageSubtitle')) $('pageSubtitle').textContent = 'Station 节点算力、存储水位与集群监控事实源。';
      } else {
        if (marketView) {
          marketView.hidden = false;
          marketView.style.display = '';
        }
        if (resourceView) {
          resourceView.hidden = true;
          resourceView.style.display = 'none';
        }
        if ($('pageTitle')) $('pageTitle').textContent = mode === 'models' ? '模型市场' : '应用市场';
        if ($('pageSubtitle')) $('pageSubtitle').textContent = mode === 'models' ? '查看应用声明的模型依赖' : '为 Station 安装创作应用，管理模型与运行状态。';
      }
    }

    // Update Left Sidebar Nav Active Classes
    document.querySelectorAll('.side .nav-link').forEach(link => {
      link.classList.toggle('selected', link.dataset.nav === mode);
    });
  }

  async function refresh() {
    if (refreshing) return;
    refreshing = true;
    if ($('reset')) $('reset').disabled = true;
    try {
      const data = await api('apps').catch(() => ({ items: [] }));
      apps = mergeCatalogWithLive(data.items || []);
      const nextIdentity = await api('me');
      const changed = identity.user_id !== nextIdentity.user_id || identity.organization_id !== nextIdentity.organization_id || identity.station_id !== nextIdentity.station_id;
      identity = nextIdentity;
      if (changed) {
        operations.clear();
        restoreOperations();
      }
      connected = true;
      render();
      if (selected) {
        const a = apps.find(x => x.app_id === selected);
        if (a) drawDetail(a);
      }
    } catch (e) {
      connected = false;
      apps = mergeCatalogWithLive([]);
      render();
      if (selected) closeDetail();
      if (!e.unauthorized) notice(e.message);
    } finally {
      refreshing = false;
      if ($('reset')) $('reset').disabled = false;
    }
  }

  function saveOperations() {
    try {
      sessionStorage.setItem('studio.operations', JSON.stringify({
        user: identity.user_id,
        organization: identity.organization_id,
        station: identity.station_id,
        items: [...operations]
      }));
    } catch { }
  }

  function restoreOperations() {
    try {
      const saved = JSON.parse(sessionStorage.getItem('studio.operations') || 'null');
      if (saved && saved.user === identity.user_id && saved.organization === identity.organization_id && saved.station === identity.station_id) {
        operations.clear();
        for (const [id, entry] of saved.items) operations.set(id, { ...entry, sending: false });
      }
    } catch { }
  }

  async function submit(entry) {
    entry.sending = true;
    try {
      entry.op = await api('commands', { method: 'POST', body: JSON.stringify(entry.command) });
      entry.error = '';
    } catch (e) {
      entry.error = e.message;
      entry.rejected = !!e.status && e.status < 500 && e.status !== 401;
      throw e;
    } finally {
      entry.sending = false;
      saveOperations();
    }
  }

  async function command(action) {
    if (!selected) return;
    const a = apps.find(x => x.app_id === selected);
    if (!a) return;
    let entry = operations.get(a.app_id);
    if (entry?.sending) return;
    if (entry && !terminal(entry.op) && !entry.rejected) {
      notice('已有待完成操作，请查询原操作');
      return;
    }
    entry = {
      command: {
        request_id: crypto.randomUUID(),
        idempotency_key: 'studio-' + crypto.randomUUID(),
        organization_id: identity.organization_id,
        station_id: identity.station_id,
        app_id: a.app_id,
        app_version: a.version,
        action
      }
    };
    operations.set(a.app_id, entry);
    saveOperations();
    drawDetail(a);
    try {
      await submit(entry);
      notice(`${a.display_name}：命令已记录，等待执行`);
    } catch (e) {
      notice(e.message);
    }
    if (selected === a.app_id) drawDetail(a);
  }

  async function pollOperations() {
    if (operationPolling || !connected) return;
    operationPolling = true;
    try {
      for (const entry of operations.values()) {
        if (entry.sending || entry.rejected || terminal(entry.op)) continue;
        try {
          if (entry.op) {
            entry.op = await api('operations/' + entry.op.operation_id);
            entry.error = '';
          } else {
            await submit(entry);
          }
        } catch (e) {
          entry.error = e.message;
          if (e.unauthorized) break;
        }
      }
      saveOperations();
      if (selected) {
        const a = apps.find(x => x.app_id === selected);
        if (a) drawDetail(a);
      }
    } finally {
      operationPolling = false;
    }
  }

  function drawDetail(a) {
    const entry = operations.get(a.app_id), op = entry?.op, busy = entry && !entry.rejected && !terminal(op);
    const dep = a.deployment || { state: 'not_installed', images: [] };

    $('detailHead').innerHTML = `<div class="card-top">
      <div class="app-icon ${esc(a.group_id)}">${icon(a)}</div>
      <div>
        <h2 id="detailTitle">${esc(a.display_name)}</h2>
        <div class="brand-version" style="font-size:12px; margin-top:6px;">
          ${brandLabel(a)}
          <span class="version-divider">·</span>
          <span>v${esc(a.version)} ${badge(a)}</span>
        </div>
      </div>
    </div>`;

    $('actions').innerHTML = [['安装', 'install'], ['启动', 'start'], ['关闭', 'stop'], ['重启', 'restart'], ['删除', 'delete']].map(([label, action]) => {
      const unavailable = ['stop', 'restart', 'delete'].includes(action);
      const disabled = dep.state === 'unknown' || busy || unavailable || (action === 'install' ? dep.state !== 'not_installed' && dep.state !== 'absent' : !installed(a));
      return `<button class="btn ${action === 'install' && (dep.state === 'not_installed' || dep.state === 'absent') ? 'primary' : ''}" data-command="${action}" ${disabled ? 'disabled' : ''} ${unavailable ? 'title="暂不可用：尚未支持安全结束进行中的任务"' : ''}>${label}</button>`;
    }).join('');

    $('progress').innerHTML = entry ? `<p>${op ? op.status === 'accepted' ? '命令已记录，等待执行' : op.status === 'failed' ? '操作失败' : op.status === 'succeeded' ? '操作完成' : esc(phases[op.phase] || op.phase) : entry.rejected ? '命令被拒绝' : '响应待确认，正在使用原幂等键恢复'}</p>${op ? `<p>阶段：${esc(phases[op.phase] || op.phase)} · 操作 ID：${esc(op.operation_id)}</p><p>下载：${op.download.downloaded_bytes} / ${op.download.total_bytes ?? '未知'} 字节 · ${op.download.percent == null ? '进度未知' : esc(op.download.percent) + '%'}</p>` : ''}${op?.error ? `<p role="alert">${esc(op.error.message)} (${esc(op.error.code)})</p>` : ''}${entry.error ? `<p role="alert">${esc(entry.error)}</p>` : ''}` : '';

    $('detailContent').innerHTML = `
      <p>命令状态与部署就绪分别展示；下载完成不代表安装或模型预热完成。</p>
      <p class="description" style="color:var(--muted); margin: 8px 0 16px;">${esc(a.description || '')}</p>
      <h3>部署</h3>
      <p>关闭、重启和删除暂不可用：尚未支持安全结束进行中的任务。</p>
      <p>状态：${statusText[dep.state] || '未安装'}</p>
      <p>就绪副本：${dep.ready_replicas ?? '—'} / ${dep.desired_replicas ?? '—'}</p>
      <p>观测时间：${dep.observed_at ? new Date(dep.observed_at).toLocaleString() : '尚未部署'}</p>
      <h3>模型</h3>
      <p>${a.models && a.models.length ? esc(a.models.join(' · ')) : '无需本地模型'}</p>
      <p>${a.model_status === 'unknown' ? '模型下载与预热状态尚未接入' : a.models && a.models.length ? '随应用自动下载与预热' : '不需要模型预热'}</p>
      ${dep.images && dep.images.length ? `<details><summary>部署信息</summary><p>${esc(a.namespace || 'verdantflare-station')} / ${esc(a.workload_name || a.app_id)}</p><h4>当前镜像</h4>${dep.images.map(i => `<p style="overflow-wrap:anywhere">${esc(i.component)}：${esc(i.image)}</p>`).join('')}</details>` : ''}
    `;

    document.querySelectorAll('[data-detail-tab]').forEach(b => {
      b.hidden = b.dataset.detailTab !== 'overview';
    });
  }

  async function openDetail(id) {
    try {
      const data = await api('apps/' + encodeURIComponent(id));
      selected = id;
      const local = apps.find(x => x.app_id === id);
      const appData = data.app ? { ...local, ...data.app } : local;
      drawDetail(appData);
      $('overlay').classList.add('open');
      $('overlay').style.display = 'flex';
      $('closeDrawer').focus();
    } catch {
      const localApp = apps.find(x => x.app_id === id);
      if (localApp) {
        selected = id;
        drawDetail(localApp);
        $('overlay').classList.add('open');
        $('overlay').style.display = 'flex';
        $('closeDrawer').focus();
      }
    }
  }

  function closeDetail() {
    selected = null;
    $('overlay').classList.remove('open');
    $('overlay').style.display = 'none';
  }

  $('loginDialog').addEventListener('cancel', e => e.preventDefault());
  $('loginForm').addEventListener('submit', async e => {
    e.preventDefault();
    const form = e.currentTarget, b = form.querySelector('button');
    b.disabled = true;
    $('loginError').textContent = '';
    try {
      await api('login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ username: form.elements.username.value, password: form.elements.password.value })
      });
      form.elements.password.value = '';
      $('toast').classList.remove('show');
      $('loginDialog').close();
      await refresh();
      await video.sync();
    } catch (err) {
      $('loginError').textContent = err.message;
    } finally {
      b.disabled = false;
    }
  });

  if ($('reset')) $('reset').onclick = refresh;
  if ($('theme')) {
    const savedTheme = localStorage.getItem('vf_studio_theme') || 'light';
    document.body.dataset.theme = savedTheme;
    document.documentElement.dataset.theme = savedTheme;
    $('theme').textContent = savedTheme === 'dark' ? '浅色主题' : '深色主题';

    $('theme').onclick = () => {
      const isDark = document.body.dataset.theme === 'dark';
      const nextTheme = isDark ? 'light' : 'dark';
      document.body.dataset.theme = nextTheme;
      document.documentElement.dataset.theme = nextTheme;
      localStorage.setItem('vf_studio_theme', nextTheme);
      $('theme').textContent = nextTheme === 'dark' ? '浅色主题' : '深色主题';
    };
  }

  if ($('vfSideLogo')) {
    const vfSrc = brandMarks.vf.match(/src="([^"]+)"/);
    if (vfSrc) $('vfSideLogo').src = vfSrc[1];
  }

  const userCard = $('sideUserCard');
  if (userCard) {
    userCard.style.cursor = 'pointer';
    userCard.onclick = async () => {
      if (confirm('是否退出当前登录状态？')) {
        try {
          await api('logout', { method: 'POST' });
          closeDetail();
          operations.clear();
          sessionStorage.removeItem('studio.operations');
          login();
        } catch (e) {
          notice(e.message);
        }
      }
    };
  }

  if ($('newTask')) {
    $('newTask').disabled = true;
    $('newTask').title = '创作任务开发中';
  }

  if ($('search')) $('search').addEventListener('input', render);
  if ($('closeDrawer')) $('closeDrawer').onclick = closeDetail;
  if ($('overlay')) $('overlay').onclick = e => { if (e.target === $('overlay')) closeDetail() };

  listen(document, 'keydown', e => { if (e.key === 'Escape' && selected) closeDetail() });

  listen(document, 'click', e => {
    const b = e.target.closest('button,a');
    if (!b || b.disabled) return;

    if (b.dataset.openVideo) {
      e.preventDefault();
      const app = apps.find(a => a.app_id === 'video-mcp-server');
      if (app?.deployment.state === 'ready') video.open();
      else notice('Video 暂未就绪');
      return;
    }
    if (b.dataset.openImage) {
      e.preventDefault();
      const app = apps.find(a => a.app_id === 'image-mcp-server');
      if (app?.deployment.state === 'ready' || app?.deployment.state === 'running') image.open();
      else notice('Image 暂未就绪');
      return;
    }
    if (b.dataset.placeholder) {
      e.preventDefault();
      notice('该功能正在开发');
      return;
    }
    if (b.dataset.action) {
      const appId = b.dataset.id;
      const action = b.dataset.action;
      openDetail(appId).then(() => {
        command(action);
      });
      return;
    }
    if (b.dataset.command) {
      command(b.dataset.command);
      return;
    }
    if (b.dataset.detail) {
      openDetail(b.dataset.detail);
      return;
    }
    if (b.dataset.group) {
      group = b.dataset.group;
      render();
      return;
    }
    if (b.dataset.tab) {
      if (!document.getElementById('videoHost').hidden) location.hash = '/market';
      if (document.getElementById('imageHost') && !document.getElementById('imageHost').hidden) location.hash = '/market';
      tab = b.dataset.tab;
      render();
      return;
    }
    if (b.dataset.nav) {
      if (['resources', 'models', 'market', 'workbench'].includes(b.dataset.nav)) {
        e.preventDefault();
      }
      if (!document.getElementById('videoHost').hidden) location.hash = '/market';
      if (document.getElementById('imageHost') && !document.getElementById('imageHost').hidden) location.hash = '/market';
      if (b.dataset.nav === 'resources') {
        mode = 'resources';
      } else if (b.dataset.nav === 'models') {
        mode = 'models';
        tab = 'all';
        group = 'all';
      } else if (b.dataset.nav === 'market' || b.dataset.nav === 'workbench') {
        mode = 'market';
        tab = 'all';
        group = 'all';
      }
      render();
      return;
    }
  });

  if ($('overlay')) $('overlay').style.display = 'none';

  // First-Paint Immediate Route Resolution (Zero Market Flash)
  const isVideoRoute = location.hash.includes('video=');
  const isImageRoute = location.hash.includes('image=');
  if (isVideoRoute) {
    video.sync();
  } else if (isImageRoute) {
    image.sync();
  } else {
    render();
  }

  refresh().then(() => {
    if (location.hash.includes('video=')) video.sync();
    else if (location.hash.includes('image=')) image.sync();
  });
  const polling = setInterval(() => {
    if (!$('loginDialog').open) {
      refresh().then(() => {
        if (location.hash.includes('video=')) video.sync();
        else if (location.hash.includes('image=')) image.sync();
      });
    }
  }, 30000);
  const operationTimer = setInterval(pollOperations, 2000);

  return () => {
    video.dispose();
    image.dispose();
    clearInterval(operationTimer);
    clearInterval(polling);
    controller.abort();
  };
}
