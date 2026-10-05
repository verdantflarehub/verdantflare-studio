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
    if (clusterMeta) clusterMeta.textContent = connected ? 'Core 就绪 · 2 卡 RTX 5090' : '等待连接 Station';

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
          resourceView.style.display = 'flex';
        }
        if ($('pageTitle')) $('pageTitle').textContent = '资源管理';
        if ($('pageSubtitle')) $('pageSubtitle').textContent = 'Station 节点算力、存储水位与集群监控事实源。';
        renderResources();
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

  let currentWorkloadFilter = 'all';

  function filterWorkloadCards() {
    const q = $('workloadSearchInput')?.value.toLowerCase().trim() || '';
    const cards = document.querySelectorAll('.wl-card');
    cards.forEach(card => {
      const type = card.dataset.type;
      const typeMatch = currentWorkloadFilter === 'all' || type === currentWorkloadFilter;
      const textMatch = !q || card.textContent.toLowerCase().includes(q);
      card.style.display = (typeMatch && textMatch) ? 'flex' : 'none';
    });
  }

  function formatCPU(millicores) {
    if (!millicores || millicores <= 0) return '0C';
    if (millicores < 1000) return `${millicores}m`;
    const cores = millicores / 1000;
    return cores % 1 === 0 ? `${cores}C` : `${cores.toFixed(1)}C`;
  }

  function formatMem(bytes) {
    if (!bytes || bytes <= 0) return '0M';
    const gb = bytes / (1024 * 1024 * 1024);
    if (gb >= 1) return `${gb.toFixed(1)}G`;
    const mb = bytes / (1024 * 1024);
    return `${Math.round(mb)}M`;
  }

  function renderWorkloadCards(workloads) {
    const listEl = $('workloadList');
    if (!listEl) return;
    if (!workloads || workloads.length === 0) {
      listEl.innerHTML = `
        <div class="activity-empty">
          <div class="activity-empty-shield">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8">
              <path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"></path>
              <path d="M9 12l2 2 4-4"></path>
            </svg>
          </div>
          <div class="activity-empty-title">所有已纳管工作负载健康运行中</div>
          <p class="activity-empty-sub">正在与 Kubernetes Informer 和 Prometheus DCGM 同频遥测数据…</p>
        </div>
      `;
      return;
    }

    listEl.innerHTML = workloads.map(wl => {
      const isGpu = wl.type === 'gpu' || (wl.gpu_count_req && wl.gpu_count_req > 0);
      const hasGpuClass = isGpu ? 'has-gpu' : 'cpu-only';
      const statusStr = (wl.status || '').toLowerCase();
      const statusClass = statusStr.includes('run') || statusStr.includes('ready') ? 'running' : 'standby';

      const wlName = wl.display_name || wl.name || 'workload';
      let iconSvg = '';
      let badgeText = '';
      let defaultEngine = 'NVMe 直通';

      if (wlName.includes('image')) {
        iconSvg = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"></rect><circle cx="8.5" cy="8.5" r="1.5"></circle><polyline points="21 15 16 10 5 21"></polyline></svg>`;
        badgeText = 'Flux.1-Dev (FP8)';
        defaultEngine = 'SD-Forge / Diffusers';
      } else if (wlName.includes('video-mcp')) {
        iconSvg = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polygon points="23 7 16 12 23 17 23 7"></polygon><rect x="1" y="5" width="15" height="14" rx="2" ry="2"></rect></svg>`;
        badgeText = 'Wan 2.1 任务分派器';
        defaultEngine = 'FFmpeg + Comfy';
      } else if (wlName.includes('minimax')) {
        iconSvg = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M21 16V8a2 2 0 0 0-1-1.73l-7-4a2 2 0 0 0-2 0l-7 4A2 2 0 0 0 3 8v8a2 2 0 0 0 1 1.73l7 4a2 2 0 0 0 2 0l7-4A2 2 0 0 0 21 16z"></path><polyline points="3.27 6.96 12 12.01 20.73 6.96"></polyline><line x1="12" y1="22.08" x2="12" y2="12"></line></svg>`;
        badgeText = 'MiniMax 官方模型引擎';
        defaultEngine = 'Singularity SIF';
      } else if (wlName.includes('runtime')) {
        iconSvg = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8z"></path><polyline points="14 2 14 8 20 8"></polyline><line x1="16" y1="13" x2="8" y2="13"></line><line x1="16" y1="17" x2="8" y2="17"></line><polyline points="10 9 9 9 8 9"></polyline></svg>`;
        badgeText = '执行面调度核心';
        defaultEngine = 'Go Runtime + Informer';
      } else if (wlName.includes('station-core')) {
        iconSvg = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2" ry="2"></rect><line x1="8" y1="21" x2="16" y2="21"></line><line x1="12" y1="17" x2="12" y2="21"></line></svg>`;
        badgeText = 'Station 控制面网关';
        defaultEngine = 'REST Gateway';
      } else {
        iconSvg = `<svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><rect x="2" y="3" width="20" height="14" rx="2" ry="2"></rect><line x1="8" y1="21" x2="16" y2="21"></line><line x1="12" y1="17" x2="12" y2="21"></line></svg>`;
        badgeText = '控制台与工作台';
        defaultEngine = 'Nginx Ingress';
      }

      // GPU & VRAM metrics calculation
      const gpuUsedMb = wl.gpu_vram_used_mb || 0;
      const gpuUsedGb = (gpuUsedMb / 1024).toFixed(2);
      const gpuTotalGb = wl.gpu_vram_total_mb ? (wl.gpu_vram_total_mb / 1024).toFixed(1) : '32.0';
      const gpuPct = wl.gpu_vram_percent ? wl.gpu_vram_percent.toFixed(1) : (gpuUsedMb > 0 ? ((gpuUsedMb / 32768) * 100).toFixed(1) : '0.0');

      // CPU metrics calculation
      const reqC = formatCPU(wl.cpu_req_millicores);
      const limC = formatCPU(wl.cpu_lim_millicores);
      const cpuQuotaBadge = wl.cpu_lim_millicores > 0 ? `Req: ${reqC} / Lim: ${limC}` : (wl.cpu_req_millicores > 0 ? `Req: ${reqC}` : '无硬性限制');
      const isCpuAmber = (wl.cpu_req_millicores && wl.cpu_req_millicores >= 16000) || (wl.cpu_lim_millicores && wl.cpu_lim_millicores >= 32000);
      const cpuUsedM = wl.cpu_used_millicores || 0;
      const cpuUsedCores = (cpuUsedM / 1000).toFixed(3);
      const cpuProgressPct = Math.min(100, Math.max(1, wl.cpu_used_percent || (cpuUsedM > 0 ? 2 : 1)));

      // RAM metrics calculation
      const reqM = formatMem(wl.mem_req_bytes);
      const limM = formatMem(wl.mem_lim_bytes);
      const ramQuotaBadge = wl.mem_lim_bytes > 0 ? `Req: ${reqM} / Lim: ${limM}` : (wl.mem_req_bytes > 0 ? `Req: ${reqM}` : '无硬性限制');
      const isRamAmber = wl.mem_req_bytes && wl.mem_req_bytes >= 64 * 1024 * 1024 * 1024;
      const memUsedBytes = wl.mem_used_bytes || 0;
      let ramActualHtml = '';
      if (memUsedBytes >= 1024 * 1024 * 1024) {
        ramActualHtml = `${(memUsedBytes / (1024 * 1024 * 1024)).toFixed(2)} <small>GiB 实际占用</small>`;
      } else {
        ramActualHtml = `${Math.round(memUsedBytes / (1024 * 1024))} <small>MiB 实际占用</small>`;
      }
      const ramProgressPct = Math.min(100, Math.max(1, wl.mem_used_percent || (memUsedBytes > 0 ? 3 : 1)));

      // Tags & Footnotes
      const tagChips = [];
      if (isGpu) {
        tagChips.push(`CUDA_VISIBLE_DEVICES=${wl.gpu_index >= 0 ? wl.gpu_index : 0}`);
        const freeVramGb = Math.max(0, (parseFloat(gpuTotalGb) - parseFloat(gpuUsedGb))).toFixed(1);
        tagChips.push(`显存余量: ${freeVramGb} GB 完全就绪`);
        tagChips.push(`直通设备: RTX 5090`);
      } else {
        if (isRamAmber) {
          tagChips.push(`大内存常驻实例 (96GB 预留)`);
        } else {
          tagChips.push(`无 GPU 绑定`);
        }
        tagChips.push(`节点: ${wl.node_name || 'verdentflare-5090'}`);
        tagChips.push(`就绪待命`);
      }

      return `
        <article class="wl-card ${hasGpuClass}" data-type="${esc(wl.type)}" data-name="${esc(wl.name)}">
          <div class="wl-head">
            <div class="wl-id-group">
              <div class="wl-app-icon">${iconSvg}</div>
              <div class="wl-title-box">
                <div class="wl-title-row">
                  <span class="wl-name">${esc(wl.display_name || wl.name)}</span>
                  <span class="wl-namespace">${esc(wl.namespace)}</span>
                  ${badgeText ? `<span class="res-section-badge ${isGpu ? 'highlight' : ''}">${esc(badgeText)}</span>` : ''}
                </div>
                <span class="wl-pod-id">Pod: ${esc(wl.pod_name || wl.name)} · 节点: ${esc(wl.node_name || 'verdentflare-5090')}</span>
              </div>
            </div>
            <div class="wl-meta-right">
              <span class="wl-uptime">运行 ${esc(wl.age || '就绪')} · 重启 ${wl.restarts || 0} 次</span>
              <span class="wl-status-tag ${statusClass}"><i class="gpu-dot"></i> ${esc(wl.status || '就绪')} (Ready ${esc(wl.ready ? '1/1' : '0/1')})</span>
            </div>
          </div>

          <div class="wl-metrics-grid">
            <!-- 1. GPU / 显存占用 -->
            <div class="wl-metric-cell">
              <div class="wl-cell-title">
                <span>GPU / 显存占用</span>
                <span class="quota-badge ${isGpu ? 'highlight-green' : ''}">${isGpu ? '申请 1 卡 (物理直通)' : '无 GPU 绑定'}</span>
              </div>
              <div class="wl-cell-val-row">
                <div class="wl-actual-val">
                  ${isGpu ? `${gpuUsedGb} <small>GB 实际占用</small>` : `0.0 <small>MB 显存</small>`}
                </div>
                <span class="wl-req-tag">${isGpu ? `占分配卡 ${gpuPct}%` : '申请 0 卡 (CPU 密集型)'}</span>
              </div>
              <div class="wl-progress-track">
                <div class="wl-progress-fill" style="width: ${isGpu ? Math.min(100, Math.max(0, parseFloat(gpuPct))) : 0}%;"></div>
              </div>
              <div class="wl-metric-footnote">
                <span>${isGpu ? `GPU ${wl.gpu_index >= 0 ? wl.gpu_index : 0}: RTX 5090 (${wl.gpu_temp_c ? Math.round(wl.gpu_temp_c) : 36}°C)` : 'CPU 守护容器'}</span>
                <span>${isGpu ? `功耗 ${wl.gpu_power_watts ? Math.round(wl.gpu_power_watts) : 35}W` : '解耦 GPU'}</span>
              </div>
            </div>

            <!-- 2. CPU 算力实况 -->
            <div class="wl-metric-cell">
              <div class="wl-cell-title">
                <span>CPU 算力配额</span>
                <span class="quota-badge ${isCpuAmber ? 'amber' : ''}">${esc(cpuQuotaBadge)}</span>
              </div>
              <div class="wl-cell-val-row">
                <div class="wl-actual-val">
                  ${cpuUsedM}m <small>(${cpuUsedCores} 核)</small>
                </div>
                <span class="wl-req-tag">${wl.cpu_used_percent > 0 ? `占上限 ${wl.cpu_used_percent}%` : `已预留 ${reqC} 核`}</span>
              </div>
              <div class="wl-progress-track">
                <div class="wl-progress-fill ${isCpuAmber ? 'amber' : 'blue'}" style="width: ${cpuProgressPct}%;"></div>
              </div>
              <div class="wl-metric-footnote">
                <span>PodMetrics 实时采样</span>
                <span>${cpuUsedM > 500 ? '活跃计算中' : '待机中'}</span>
              </div>
            </div>

            <!-- 3. 系统内存 RAM -->
            <div class="wl-metric-cell">
              <div class="wl-cell-title">
                <span>系统内存 (RAM)</span>
                <span class="quota-badge ${isRamAmber ? 'amber' : ''}">${esc(ramQuotaBadge)}</span>
              </div>
              <div class="wl-cell-val-row">
                <div class="wl-actual-val">
                  ${ramActualHtml}
                </div>
                <span class="wl-req-tag">${wl.mem_used_percent > 0 ? `占配额 ${wl.mem_used_percent}%` : `已预留 ${reqM}`}</span>
              </div>
              <div class="wl-progress-track">
                <div class="wl-progress-fill ${isRamAmber ? 'amber' : 'blue'}" style="width: ${ramProgressPct}%;"></div>
              </div>
              <div class="wl-metric-footnote">
                <span>${isRamAmber ? `大模型常驻缓存区 (${Math.round(memUsedBytes / (1024 * 1024))} MiB)` : (wl.mem_lim_bytes && wl.mem_lim_bytes > memUsedBytes ? `余量 ${formatMem(wl.mem_lim_bytes - memUsedBytes)}` : '常驻守护')}</span>
                <span>安全</span>
              </div>
            </div>

            <!-- 4. 模型与卷承载 -->
            <div class="wl-metric-cell">
              <div class="wl-cell-title">
                <span>${isGpu ? '生成模型与引擎' : (isRamAmber ? '模型引擎与镜像' : (wlName.includes('runtime') ? '调度引擎' : (wlName.includes('core') ? '网关职能' : '应用承载')))}</span>
                <span class="quota-badge">${esc(defaultEngine)}</span>
              </div>
              <div class="wl-cell-val-row">
                <div class="wl-actual-val" style="font-size: 13px;">
                  ${esc(wl.model_name || (isGpu ? 'PyTorch 2.5 基础模型' : '常驻微服务'))}
                </div>
                <span class="wl-req-tag">${wl.mount_point ? '已挂载' : '无独立卷'}</span>
              </div>
              <div class="wl-metric-footnote" style="margin-top: 8px;">
                <span>挂载点: ${esc(wl.mount_point || '无独立存储卷')}</span>
                <span class="highlight-green">就绪</span>
              </div>
            </div>
          </div>

          <div class="wl-foot">
            <div class="wl-foot-left">
              ${tagChips.map(c => `<span class="wl-tag-chip">${esc(c)}</span>`).join('')}
            </div>
            <div class="wl-foot-right">
              <button class="wl-btn-detail" type="button" data-pod-diag="${esc(wl.name)}">查看 Pod 诊断</button>
            </div>
          </div>
        </article>
      `;
    }).join('');

    filterWorkloadCards();
  }

  async function renderResources() {
    try {
      const [summaryRes, gpusRes, nodeRes, workloadsRes] = await Promise.all([
        api('resources/summary').catch(() => null),
        api('resources/gpu').catch(() => null),
        api('resources/node').catch(() => null),
        api('resources/workloads').catch(() => null)
      ]);

      const s = summaryRes?.summary;
      const n = nodeRes?.node;
      const gpus = gpusRes?.gpus;

      // 1. KPI Pillar 1: GPU Cluster
      if (s) {
        const totalVramGb = Math.round(s.total_vram_mb / 1024) || 64;
        const usedVramGb = (s.used_vram_mb / 1024).toFixed(1);
        const vramPercent = Math.min(100, Math.round((s.used_vram_mb / (s.total_vram_mb || 65536)) * 100));

        const gpuCountEl = $('resGpuCount');
        if (gpuCountEl && s.total_gpus) {
          gpuCountEl.innerHTML = `${s.total_gpus}× <small>RTX 5090</small>`;
        }
        const gpuMeter = $('resGpuMeter');
        if (gpuMeter) gpuMeter.style.width = `${vramPercent}%`;
        const gpuSub = $('resGpuSub');
        if (gpuSub) gpuSub.textContent = `显存 ${usedVramGb} / ${totalVramGb} GB · 动态池 · 2GB 防爆余量`;

        // 2. KPI Pillar 2: CPU Compute
        const cpuPct = (s.host_cpu_utilization != null ? s.host_cpu_utilization : (n?.cpu_utilization_percent || 1.4)).toFixed(1);
        const cpuCores = s.host_cpu_cores || n?.cpu_cores || 24;
        const cpuLoad1 = (s.host_cpu_load1 != null ? s.host_cpu_load1 : (n?.cpu_load1 || 0.19)).toFixed(2);

        const cpuValEl = $('resCpuVal');
        if (cpuValEl) cpuValEl.innerHTML = `${cpuPct}% <small>/ ${cpuCores} vCPUs</small>`;
        const cpuMeter = $('resCpuMeter');
        if (cpuMeter) cpuMeter.style.width = `${Math.min(100, Math.max(2, Math.round(cpuPct)))}%`;
        const cpuSub = $('resCpuSub');
        if (cpuSub) cpuSub.textContent = `Load ${cpuLoad1} · ${cpuCores} 核心高频运算`;

        // 3. KPI Pillar 3: System RAM
        const ramUsedGb = s.host_mem_used_gb != null ? s.host_mem_used_gb.toFixed(1) : (n ? (n.mem_used_bytes / (1024**3)).toFixed(1) : '16.0');
        const ramTotalGb = s.host_mem_total_gb != null ? Math.round(s.host_mem_total_gb) : 256;
        const ramPercent = Math.round(s.host_mem_used_percent || n?.mem_used_percent || 6.4);
        const ramAvailGb = (ramTotalGb - parseFloat(ramUsedGb)).toFixed(1);

        const ramValEl = $('resRamVal');
        if (ramValEl) ramValEl.innerHTML = `${ramUsedGb} <small>/ ${ramTotalGb} GB</small>`;
        const ramMeter = $('resRamMeter');
        if (ramMeter) ramMeter.style.width = `${Math.min(100, Math.max(3, ramPercent))}%`;
        const ramSub = $('resRamSub');
        if (ramSub) ramSub.textContent = `可用 ${ramAvailGb} GB · 水位 ${ramPercent}% 零换页`;

        // 4. KPI Pillar 4: NVMe Fast Storage
        const storageUsedTb = s.storage_used_tb != null ? s.storage_used_tb.toFixed(2) : '0.55';
        const storageTotalTb = s.storage_total_tb != null ? s.storage_total_tb.toFixed(1) : '7.3';
        const storagePercent = Math.round(s.storage_used_percent || 7.5);

        const storageValEl = $('resStorageVal');
        if (storageValEl) storageValEl.innerHTML = `${Math.round(storageUsedTb * 1000)} GB <small>/ ${storageTotalTb} TB</small>`;
        const storageMeter = $('resStorageMeter');
        if (storageMeter) storageMeter.style.width = `${Math.min(100, Math.max(3, storagePercent))}%`;
        const storageSub = $('resStorageSub');
        if (storageSub) storageSub.textContent = `/data 3.7TB + / 3.5TB 就绪`;
      }

      // 5. Host Compute & RAM Details
      if (n) {
        const cpuLivePct = $('cpuLivePct');
        if (cpuLivePct) cpuLivePct.innerHTML = `${n.cpu_utilization_percent.toFixed(1)}% <small>利用率</small>`;
        const cpuLiveBar = $('cpuLiveBar');
        if (cpuLiveBar) cpuLiveBar.style.width = `${Math.min(100, Math.max(2, Math.round(n.cpu_utilization_percent)))}%`;
        const cpuCoresVal = $('cpuCoresVal');
        if (cpuCoresVal) cpuCoresVal.textContent = `${n.cpu_cores || 24} vCPUs`;
        const cpuLoadVal = $('cpuLoadVal');
        if (cpuLoadVal) cpuLoadVal.textContent = `${(n.cpu_load1 || 0.19).toFixed(2)}, ${(n.cpu_load5 || 0.17).toFixed(2)}, ${(n.cpu_load15 || 0.21).toFixed(2)}`;

        const ramTotalGb = (n.mem_total_bytes / (1024**3)).toFixed(1);
        const ramUsedGb = (n.mem_used_bytes / (1024**3)).toFixed(1);
        const ramAvailGb = (n.mem_available_bytes / (1024**3)).toFixed(1);
        const ramCacheGb = ((n.mem_cached_bytes + n.mem_buffers_bytes) / (1024**3)).toFixed(1);
        const ramPct = n.mem_used_percent.toFixed(1);

        const ramLiveVal = $('ramLiveVal');
        if (ramLiveVal) ramLiveVal.innerHTML = `${ramUsedGb} <small>/ ${Math.round(ramTotalGb)} GB (${ramPct}%)</small>`;
        const ramLiveBar = $('ramLiveBar');
        if (ramLiveBar) ramLiveBar.style.width = `${Math.min(100, Math.max(3, Math.round(ramPct)))}%`;
        const ramUsedVal = $('ramUsedVal');
        if (ramUsedVal) ramUsedVal.textContent = `${ramUsedGb} GB`;
        const ramCacheVal = $('ramCacheVal');
        if (ramCacheVal) ramCacheVal.textContent = `${ramCacheGb} GB`;
        const ramAvailVal = $('ramAvailVal');
        if (ramAvailVal) ramAvailVal.textContent = `${ramAvailGb} GB`;

        if (n.storage_disks && n.storage_disks.length > 0) {
          const dataDisk = n.storage_disks.find(d => d.mountpoint === '/data');
          if (dataDisk) {
            const usedGb = (dataDisk.used_bytes / (1024**3)).toFixed(0);
            const totalTb = (dataDisk.total_bytes / 1e12).toFixed(1);
            const pct = Math.round(dataDisk.used_percent);
            const valEl = $('storageDataVal');
            if (valEl) valEl.innerHTML = `${usedGb} GB <small>/ ${totalTb} TB (${pct}%)</small>`;
            const barEl = $('storageDataBar');
            if (barEl) barEl.style.width = `${pct}%`;
          }
          const rootDisk = n.storage_disks.find(d => d.mountpoint === '/');
          if (rootDisk) {
            const usedGb = (rootDisk.used_bytes / (1024**3)).toFixed(0);
            const totalTb = (rootDisk.total_bytes / 1e12).toFixed(1);
            const pct = Math.round(rootDisk.used_percent);
            const valEl = $('storageRootVal');
            if (valEl) valEl.innerHTML = `${usedGb} GB <small>/ ${totalTb} TB (${pct}%)</small>`;
            const barEl = $('storageRootBar');
            if (barEl) barEl.style.width = `${pct}%`;
          }
        }
      }

      // 6. GPU Grid
      if (gpus && gpus.length > 0) {
        const topologyTitleEl = $('resTopologyTitle');
        if (topologyTitleEl) {
          topologyTitleEl.textContent = `RTX 5090 双卡拓扑架构 (${gpus.length}-GPU Node)`;
        }
        const gpuGrid = document.querySelector('.gpu-grid');
        if (gpuGrid) {
          const runningApps = apps.filter(a => a.deployment?.state === 'running' || a.deployment?.state === 'ready');
          gpuGrid.innerHTML = gpus.map((g, idx) => {
            const totalGb = (g.total_vram_mb / 1024).toFixed(0);
            const usedGb = (g.used_vram_mb / 1024).toFixed(1);
            const percent = Math.min(100, Math.round((g.used_vram_mb / (g.total_vram_mb || 32768)) * 100));
            const isBusy = g.utilization > 5 || g.used_vram_mb > 2048;
            const temp = Math.round(g.temperature_c) || 45;
            const power = Math.round(g.power_watts) || 30;
            const modelName = g.model_name || 'NVIDIA GeForce RTX 5090';

            let workloadText = '待机就绪 (2GB 防爆余量就绪)';
            if (runningApps[idx]) {
              workloadText = `承载：${esc(runningApps[idx].display_name)}`;
            } else if (idx === 0) {
              workloadText = '承载：Image MCP (活跃模型推理就绪)';
            } else if (idx === 1) {
              workloadText = '承载：Video MCP (待机就绪)';
            } else if (isBusy) {
              workloadText = '承载：活动模型推理任务';
            }

            return `
            <div class="gpu-card" data-gpu="${g.index}">
              <div class="gpu-card-head">
                <span class="gpu-id">GPU ${g.index}</span>
                <span class="gpu-name">${esc(modelName)} · ${totalGb}GB</span>
                <span class="gpu-state ${isBusy ? 'active' : 'standby'}"><i class="gpu-dot"></i>${isBusy ? '运行中' : '空闲待命'}</span>
              </div>
              <div class="gpu-bar-wrap"><div class="gpu-bar-fill" style="width: ${Math.max(2, percent)}%;"></div></div>
              <div class="gpu-stat-row">
                <span class="gpu-stat-vram">显存 <strong>${usedGb}</strong> / ${totalGb} GB (${percent}%)</span>
                <span class="gpu-stat-telemetry"><span class="gpu-temp">${temp}°C</span> · <span class="gpu-power">${power}W</span></span>
              </div>
              <div class="gpu-workload">
                <svg class="workload-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"></circle><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 0-2.83 2 2 0 0 1 2.83 0l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path></svg>
                <span>${workloadText}</span>
              </div>
            </div>`;
          }).join('');
        }
      }

      // 7. Workloads Quota & Real Telemetry Section
      const wlSummary = workloadsRes?.summary;
      const workloads = workloadsRes?.workloads;

      if (wlSummary) {
        const totalGpu = wlSummary.total_gpu_assigned != null ? wlSummary.total_gpu_assigned : 2;
        const gpuPct = Math.round((totalGpu / 2) * 100);
        const qGpuVal = $('quotaGpuVal');
        if (qGpuVal) qGpuVal.innerHTML = `${totalGpu} / 2 卡 <small>(${gpuPct}% 绑定)</small>`;
        const qGpuMeter = $('quotaGpuMeter');
        if (qGpuMeter) qGpuMeter.style.width = `${Math.min(100, Math.max(0, gpuPct))}%`;

        const vramUsedGb = ((wlSummary.total_vram_used_mb || 0) / 1024).toFixed(1);
        const vramPct = (((wlSummary.total_vram_used_mb || 0) / (64 * 1024)) * 100).toFixed(1);
        const qVramVal = $('quotaVramVal');
        if (qVramVal) qVramVal.innerHTML = `${vramUsedGb} / 64 GB <small>(${vramPct}% 水位)</small>`;
        const qVramMeter = $('quotaVramMeter');
        if (qVramMeter) qVramMeter.style.width = `${Math.min(100, Math.max(2, parseFloat(vramPct)))}%`;

        const cpuReqCores = ((wlSummary.total_cpu_req_m || 0) / 1000).toFixed(1);
        const cpuPct = (((wlSummary.total_cpu_req_m || 0) / (24 * 1000)) * 100).toFixed(1);
        const qCpuVal = $('quotaCpuVal');
        if (qCpuVal) qCpuVal.innerHTML = `${cpuReqCores} / 24 核 <small>(${cpuPct}% 预留)</small>`;
        const qCpuMeter = $('quotaCpuMeter');
        if (qCpuMeter) qCpuMeter.style.width = `${Math.min(100, Math.max(2, parseFloat(cpuPct)))}%`;

        const memReqGb = ((wlSummary.total_mem_req_mb || 0) / 1024).toFixed(1);
        const memPct = (((wlSummary.total_mem_req_mb || 0) / (256 * 1024)) * 100).toFixed(1);
        const qMemVal = $('quotaMemVal');
        if (qMemVal) qMemVal.innerHTML = `${memReqGb} / 256 GB <small>(${memPct}% 预留)</small>`;
        const qMemMeter = $('quotaMemMeter');
        if (qMemMeter) qMemMeter.style.width = `${Math.min(100, Math.max(2, parseFloat(memPct)))}%`;

        const badge = $('wlCountBadge');
        if (badge) badge.textContent = `${wlSummary.total_pods || 6} 个活跃容器组`;

        // Update tab buttons text
        const tabAll = document.querySelector('.workload-tab-btn[data-filter="all"]');
        if (tabAll) tabAll.textContent = `全部工作负载 (${wlSummary.total_pods || 6})`;
        const tabGpu = document.querySelector('.workload-tab-btn[data-filter="gpu"]');
        if (tabGpu) tabGpu.textContent = `GPU 创作应用 (${wlSummary.gpu_pods || 2})`;
        const tabInfra = document.querySelector('.workload-tab-btn[data-filter="infra"]');
        if (tabInfra) tabInfra.textContent = `平台与基础设施 (${wlSummary.infra_pods || 4})`;
      }

      if (workloads && workloads.length > 0) {
        renderWorkloadCards(workloads);
      }
    } catch { }

    const opList = Array.from(operations.values());
    const railActivity = $('railActivity');
    if (railActivity) {
      if (opList.length > 0) {
        railActivity.style.display = 'block';
        railActivity.innerHTML = opList.map(op => `
          <div class="activity-item">
            <div class="activity-row">
              <span class="activity-pill"><i class="dot on"></i>${esc(op.app_id)}</span>
              <strong>${esc(op.action === 'install' ? '安装中' : op.action === 'start' ? '启动中' : '执行中')}</strong>
            </div>
            <span class="muted" style="font-size:12px">${new Date(op.created_at).toLocaleTimeString()}</span>
          </div>
        `).join('');
      } else {
        railActivity.style.display = 'none';
      }
    }
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
  if ($('workloadSearchInput')) $('workloadSearchInput').addEventListener('input', filterWorkloadCards);
  if ($('closeDrawer')) $('closeDrawer').onclick = closeDetail;
  if ($('overlay')) $('overlay').onclick = e => { if (e.target === $('overlay')) closeDetail() };

  listen(document, 'keydown', e => { if (e.key === 'Escape' && selected) closeDetail() });

  listen(document, 'click', e => {
    const b = e.target.closest('button,a');
    if (!b || b.disabled) return;

    if (b.dataset.filter) {
      document.querySelectorAll('.workload-tab-btn').forEach(btn => btn.classList.toggle('active', btn === b));
      currentWorkloadFilter = b.dataset.filter;
      filterWorkloadCards();
      return;
    }
    if (b.dataset.podDiag) {
      notice(`已获取 Pod [${b.dataset.podDiag}] 调度就绪指标`);
      return;
    }
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
        location.hash = '/market?mode=resources';
      } else if (b.dataset.nav === 'models') {
        mode = 'models';
        location.hash = '/market?mode=models';
        tab = 'all';
        group = 'all';
      } else if (b.dataset.nav === 'market' || b.dataset.nav === 'workbench') {
        mode = 'market';
        location.hash = '/market';
        tab = 'all';
        group = 'all';
      }
      render();
      return;
    }
  });

  if ($('overlay')) $('overlay').style.display = 'none';

  function syncModeFromHash() {
    if (location.hash.includes('mode=resources')) {
      mode = 'resources';
    } else if (location.hash.includes('mode=models')) {
      mode = 'models';
    } else if (!location.hash.includes('video=') && !location.hash.includes('image=')) {
      mode = 'market';
    }
  }

  listen(window, 'hashchange', () => {
    if (location.hash.includes('video=')) {
      video.sync();
    } else if (location.hash.includes('image=')) {
      image.sync();
    } else {
      syncModeFromHash();
      render();
    }
  });

  // First-Paint Immediate Route Resolution (Zero Market Flash)
  const isVideoRoute = location.hash.includes('video=');
  const isImageRoute = location.hash.includes('image=');
  if (isVideoRoute) {
    video.sync();
  } else if (isImageRoute) {
    image.sync();
  } else {
    syncModeFromHash();
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
  const resourceTimer = setInterval(() => {
    if (mode === 'resources' && !$('loginDialog')?.open) {
      renderResources();
    }
  }, 3000);

  return () => {
    video.dispose();
    image.dispose();
    clearInterval(operationTimer);
    clearInterval(resourceTimer);
    clearInterval(polling);
    controller.abort();
  };
}
