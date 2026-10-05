// Opaque-origin iframe; all data access stays on the authenticated host.
export function mountImage({ listen, api, notice, login }) {
  const host = document.getElementById('imageHost'), frame = document.getElementById('imageFrame');
  const getMarket = () => document.getElementById('marketView') || document.getElementById('marketContent');
  const getResource = () => document.getElementById('resourceView');
  const getVideo = () => document.getElementById('videoHost');
  let active = false, generation = 0, pending = new Set(), viewState = null, currentUrl = null;
  const pathPattern = /^\/(api\/(tasks(?:\/[a-zA-Z0-9_-]+)?|tasks\/stats|artifacts\/upload|mcp\/status)|artifacts\/[a-zA-Z0-9_-]+\/content)$/;
  const theme = () => document.body.dataset.theme === 'light' ? 'light' : 'dark';

  function route() {
    const params = new URLSearchParams(location.hash.split('?')[1] || '');
    return /^\/dashboard(?:\/tasks\/[a-zA-Z0-9_-]+)?(?:#(?:tasks|models|mcp))?$/.test(params.get('image') || '') ? params.get('image') : null;
  }

  function breadcrumbs(path) {
    const items = [['应用市场', '#/market'], ['Image MCP', '#/market?image=' + encodeURIComponent('/dashboard#tasks')]];
    if (path.startsWith('/dashboard/tasks/')) items.push(['任务', '#/market?image=' + encodeURIComponent('/dashboard#tasks')], ['任务详情', null]);
    else items.push([{ tasks: '任务', models: '模型', mcp: 'MCP' }[path.split('#')[1]] || '任务', null]);
    const list = document.getElementById('imageBreadcrumb');
    if (list) {
      list.replaceChildren();
      for (const [label, href] of items) {
        const li = document.createElement('li'), item = document.createElement(href ? 'a' : 'span');
        item.textContent = label;
        if (href) item.href = href;
        else item.setAttribute('aria-current', 'page');
        li.append(item);
        list.append(li);
      }
    }
  }

  const loader = document.getElementById('viewportLoader');
  const loaderText = document.getElementById('loaderText');
  let loaderTimer = null;
  function showLoader(text) {
    if (loader) {
      if (loaderText) loaderText.textContent = text || '正在启动 Image MCP 工作台…';
      loader.classList.remove('fading');
      loader.hidden = false;
      loader.style.display = 'flex';
      clearTimeout(loaderTimer);
      loaderTimer = setTimeout(() => {
        hideLoader();
      }, 2500);
    }
  }
  function hideLoader() {
    clearTimeout(loaderTimer);
    if (loader && !loader.hidden) {
      loader.classList.add('fading');
      setTimeout(() => {
        loader.hidden = true;
        loader.style.display = 'none';
        loader.classList.remove('fading');
      }, 280);
    }
  }
  if (frame) {
    frame.addEventListener('load', () => setTimeout(hideLoader, 150));
  }

  function close() {
    hideLoader();
    const wasActive = active || currentUrl !== null;
    currentUrl = null;
    generation++;
    active = false;
    pending.clear();
    if (frame) frame.removeAttribute('src');
    if (host) {
      host.hidden = true;
      host.style.display = 'none';
    }
    if (!wasActive) return;
    if (location.hash.includes('video=')) return;

    const m = getMarket();
    if (m) {
      m.hidden = false;
      m.style.display = '';
    }
    const t = document.getElementById('pageTitle');
    if (t) t.textContent = '应用市场';
    const s = document.getElementById('pageSubtitle');
    if (s) s.textContent = '为 Station 安装创作应用，管理模型与运行状态。';
    const main = document.querySelector('.main');
    if (main) main.classList.remove('embed-active');
  }

  async function sync() {
    const path = route();
    if (!path) {
      hideLoader();
      close();
      return;
    }
    const [url, hash] = path.split('#');
    if (active && currentUrl === url && frame) {
      const m = getMarket(), res = getResource(), vHost = getVideo();
      if (m) { m.hidden = true; m.style.display = 'none'; }
      if (res) { res.hidden = true; res.style.display = 'none'; }
      if (vHost) { vHost.hidden = true; vHost.style.display = 'none'; }
      if (host) { host.hidden = false; host.style.display = 'flex'; }
      const main = document.querySelector('.main');
      if (main) main.classList.add('embed-active');
      const t = document.getElementById('pageTitle');
      if (t) t.textContent = 'Image MCP';
      const s = document.getElementById('pageSubtitle');
      if (s) s.textContent = '图像生成、模型图层与 MCP 规约工作台';
      breadcrumbs(path);
      frame.contentWindow?.postMessage({ channel: 'vf-studio', type: 'route', hash: hash || 'tasks' }, '*');
      return;
    }
    currentUrl = url;
    const epoch = ++generation;
    pending.clear();

    // First-Paint: immediate viewport lockdown before async network
    const m = getMarket(), res = getResource(), vHost = getVideo();
    if (m) { m.hidden = true; m.style.display = 'none'; }
    if (res) { res.hidden = true; res.style.display = 'none'; }
    if (vHost) { vHost.hidden = true; vHost.style.display = 'none'; }
    if (host) { host.hidden = false; host.style.display = 'flex'; }
    const main = document.querySelector('.main');
    if (main) main.classList.add('embed-active');
    breadcrumbs(path);
    const t = document.getElementById('pageTitle');
    if (t) t.textContent = 'Image MCP';
    const s = document.getElementById('pageSubtitle');
    if (s) s.textContent = '图像生成、模型图层与 MCP 规约工作台';
    showLoader('正在启动 Image MCP 工作台…');

    try {
      await api('me');
      if (epoch !== generation) return;
      active = true;
      if (frame) frame.src = '/apps/image' + url + '?embed=1&theme=' + theme() + '&view=' + epoch + (hash ? '#' + hash : '');
      const msg = document.getElementById('imageMessage');
      if (msg) msg.textContent = '';
    } catch (e) {
      hideLoader();
      currentUrl = null;
      close();
      notice(e.message);
    }
  }

  function navigate(path) {
    location.hash = '/market?image=' + encodeURIComponent(path);
  }

  listen(window, 'hashchange', sync);
  listen(window, 'message', async e => {
    if (!active || e.source !== frame.contentWindow || e.origin !== 'null' || !e.data || e.data.channel !== 'vf-image') return;
    const d = e.data;
    if (d.view !== String(generation)) return;
    if (d.type === 'ready') {
      hideLoader();
      const msg = document.getElementById('imageMessage');
      if (msg) msg.textContent = '';
      frame.contentWindow.postMessage({ channel: 'vf-studio', type: 'theme', theme: theme() }, '*');
      frame.contentWindow.postMessage({ channel: 'vf-studio', type: 'restore', state: viewState }, '*');
      return;
    }
    if (d.type === 'navigate' && typeof d.path === 'string' && /^\/dashboard(?:\/tasks\/[a-zA-Z0-9_-]+)?(?:#(?:tasks|models|mcp))?$/.test(d.path)) {
      if (d.state && typeof d.state === 'object') viewState = d.state;
      navigate(d.path);
      return;
    }
    if (d.type === 'resize' && typeof d.height === 'number') {
      if (frame) frame.style.height = Math.max(d.height, 600) + 'px';
      return;
    }
    if (d.type === 'wheel' && typeof d.deltaY === 'number') {
      window.scrollBy({ top: d.deltaY, behavior: 'auto' });
      return;
    }
    if (d.type !== 'request' || !Number.isSafeInteger(d.id) || typeof d.path !== 'string' || d.path.length > 2048 || pending.has(d.id) || pending.size >= 32) return;
    const epoch = generation, method = d.method || 'GET';
    const reply = value => {
      if (epoch === generation && active) {
        frame.contentWindow.postMessage({ channel: 'vf-studio', type: 'response', id: d.id, ...value }, '*');
      }
    };
    if (!['GET', 'POST'].includes(method) || !pathPattern.test(d.path.split('?')[0]) || d.path.split('?')[0].includes('%') || d.path.includes('#') || (d.body != null && (typeof d.body !== 'string' || d.body.length > 1048576))) {
      reply({ status: 403, body: new ArrayBuffer(0) });
      return;
    }
    pending.add(d.id);
    try {
      const r = await fetch('/apps/image' + d.path, {
        method,
        body: method === 'POST' ? d.body : undefined,
        headers: method === 'POST' ? { 'Content-Type': 'application/json' } : {},
        credentials: 'same-origin',
        cache: 'no-store',
        signal: AbortSignal.timeout(90000)
      });
      if (r.status === 401) {
        close();
        login();
        return;
      }
      reply({
        status: r.status,
        contentType: r.headers.get('Content-Type') || 'application/octet-stream',
        body: await r.arrayBuffer()
      });
    } catch {
      reply({ status: 502, body: new ArrayBuffer(0) });
    } finally {
      if (epoch === generation) pending.delete(d.id);
    }
  });

  const observer = new MutationObserver(() => {
    if (active) frame.contentWindow?.postMessage({ channel: 'vf-studio', type: 'theme', theme: theme() }, '*');
  });
  observer.observe(document.body, { attributes: true, attributeFilter: ['data-theme'] });

  const retryBtn = document.getElementById('imageRetry');
  if (retryBtn) retryBtn.onclick = sync;

  return {
    open: () => route() === '/dashboard#tasks' ? sync() : navigate('/dashboard#tasks'),
    sync,
    close,
    dispose: () => {
      close();
      observer.disconnect();
    }
  };
}
