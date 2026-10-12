/* Go-owned, bounded WebSocket adaptation for the trusted legacy editor only. */
(() => {
  'use strict';
  const config = document.currentScript.dataset;
  const workspace = config.workspace, alias = config.alias, session = config.session;
  const uuid = '[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}';
  const valid = new RegExp('^/studio/legacy-blender/' + uuid + '$').test(workspace)
    && /^blender[A-Za-z0-9_-]{1,57}$/.test(alias) && /^[A-Za-z0-9_-]{32,128}$/.test(session)
    && location.pathname === '/apps/blender/' + alias + '/' + session + '/';
  const expected = location.origin.replace(/^http/, 'ws') + location.pathname + 'webrtc/signalling/';
  const encode = new TextEncoder(), fetchLocal = window.fetch.bind(window);
  const sockets = new Set();
  let used = false;
  const options = { credentials: 'omit', cache: 'no-store', redirect: 'error' };
  function remove(path) {
    void fetchLocal(path, { ...options, method: 'DELETE', keepalive: true, signal: AbortSignal.timeout(5000) }).catch(() => {});
  }
  async function json(response, limit) {
    if (!response.ok || response.headers.get('Content-Type')?.split(';')[0].trim() !== 'application/json' || !response.body) throw Error('signal response');
    const reader = response.body.getReader(), decoder = new TextDecoder('utf-8', { fatal: true });
    let size = 0, text = '';
    try {
      for (;;) {
        const part = await reader.read();
        if (part.done) break;
        size += part.value.byteLength;
        if (size > limit) throw Error('signal response limit');
        text += decoder.decode(part.value, { stream: true });
      }
      return JSON.parse(text + decoder.decode());
    } catch (error) { await reader.cancel().catch(() => {}); throw error; }
    finally { reader.releaseLock(); }
  }
  class BoundWebSocket extends EventTarget {
    #state = 0; #path; #queue = []; #bytes = 0; #sending = false;
    #lifetime = new AbortController(); #binaryType = 'blob'; #url;
    onopen = null; onmessage = null; onerror = null; onclose = null;
    constructor(url, protocols) {
      super();
      if (url instanceof URL) url = url.href;
      // Compare original spelling: URL normalization must not admit encoded paths.
      if (!valid || typeof url !== 'string' || url !== expected || (protocols !== undefined && !(Array.isArray(protocols) && protocols.length === 0))) throw new DOMException('Unsupported editor signal URL', 'SyntaxError');
      if (used) throw new DOMException('Editor signal already opened', 'InvalidStateError');
      used = true; this.#url = url; sockets.add(this);
      for (const type of ['open', 'message', 'error', 'close']) this.addEventListener(type, event => {
        const handler = this['on' + type];
        if (typeof handler === 'function') handler.call(this, event);
      });
      void this.#open();
    }
    get url() { return this.#url; }
    get readyState() { return this.#state; }
    get bufferedAmount() { return this.#bytes; }
    get protocol() { return ''; }
    get extensions() { return ''; }
    get binaryType() { return this.#binaryType; }
    set binaryType(value) { if (value === 'blob' || value === 'arraybuffer') this.#binaryType = value; }
    #emit(event) {
      this.dispatchEvent(event);
    }
    #finish(failed, code = 1000, reason = '') {
      if (this.#state === 3) return;
      this.#state = 3; this.#lifetime.abort(); this.#queue = []; this.#bytes = 0; sockets.delete(this);
      if (this.#path) remove(this.#path);
      queueMicrotask(() => {
        if (failed) this.#emit(new Event('error'));
        this.#emit(new CloseEvent('close', { code: failed ? 1006 : code, reason, wasClean: false }));
      });
    }
    async #open() {
      try {
        // Keep a bounded opening request alive after close so a late handle can be deleted.
        const response = await fetchLocal(workspace + '/signals', { ...options, method: 'POST', headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ alias, editingSession: session }), signal: AbortSignal.timeout(20000) });
        const value = await json(response, 1024);
        if (!value || Object.keys(value).join(',') !== 'signalId' || typeof value.signalId !== 'string' || !new RegExp('^' + uuid + '$').test(value.signalId)) throw Error('invalid signal');
        this.#path = workspace + '/signals/' + value.signalId;
        if (this.#state !== 0) { remove(this.#path); return; }
        this.#state = 1; this.#emit(new Event('open'));
        await this.#poll();
      } catch { if (this.#state !== 3) this.#finish(true); }
    }
    async #poll() {
      while (this.#state === 1) {
        const response = await fetchLocal(this.#path, { ...options, method: 'GET', signal: AbortSignal.any([this.#lifetime.signal, AbortSignal.timeout(40000)]) });
        if (this.#state !== 1) return;
        if (response.status === 204) continue;
        const value = await json(response, 2 << 20);
        if (!value || Object.keys(value).join(',') !== 'message' || typeof value.message !== 'string' || !value.message.isWellFormed() || encode.encode(value.message).length > 256 << 10) throw Error('invalid message');
        if (this.#state === 1) this.#emit(new MessageEvent('message', { data: value.message, origin: location.origin }));
      }
    }
    send(data) {
      if (this.#state === 0) throw new DOMException('Signal connecting', 'InvalidStateError');
      if (this.#state !== 1) return;
      if (typeof data !== 'string' || !data.isWellFormed()) { this.#finish(true); return; }
      const size = encode.encode(data).length;
      if (size > 256 << 10 || this.#queue.length >= 32 || this.#bytes + size > 8 << 20) { this.#finish(true); return; }
      this.#queue.push({ data, size }); this.#bytes += size;
      void this.#flush();
    }
    async #flush() {
      if (this.#sending) return;
      this.#sending = true;
      try {
        while (this.#state === 1 && this.#queue.length) {
          const item = this.#queue[0];
          const response = await fetchLocal(this.#path, { ...options, method: 'POST', headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ message: item.data }), signal: AbortSignal.any([this.#lifetime.signal, AbortSignal.timeout(20000)]) });
          if (this.#state !== 1) return;
          if (response.status !== 204) throw Error('send failed');
          this.#queue.shift(); this.#bytes -= item.size;
        }
      } catch { this.#finish(true); }
      finally { this.#sending = false; }
    }
    close(code = 1000, reason = '') {
      if (code !== 1000 && (!Number.isInteger(code) || code < 3000 || code > 4999)) throw new DOMException('Invalid close code', 'InvalidAccessError');
      if (typeof reason !== 'string' || encode.encode(reason).length > 123) throw new DOMException('Invalid close reason', 'SyntaxError');
      this.#finish(false, code, reason);
    }
  }
  for (const [name, value] of Object.entries({ CONNECTING: 0, OPEN: 1, CLOSING: 2, CLOSED: 3 })) {
    Object.defineProperty(BoundWebSocket, name, { value, enumerable: true });
    Object.defineProperty(BoundWebSocket.prototype, name, { value, enumerable: true });
  }
  window.WebSocket = BoundWebSocket;
  window.addEventListener('pagehide', () => { for (const socket of sockets) socket.close(); }, { once: true });
})();
