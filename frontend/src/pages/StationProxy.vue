<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import vfLogo from '../assets/verdantflare-logo.png'

type ProxyStatus = 'active' | 'pending' | 'off' | 'testing'
type ProxyRow = { id: string; name: string; protocol: string; host: string; port: string; region: string; latency: string; expiry: string; status: ProxyStatus; refs: string[]; tag: string; tested: string; exitIp?: string; probeTarget?: string; probeVersion?: string }

const rows = ref<ProxyRow[]>([
  { id: 'proxy_01HOMNI', name: 'Google Omni 出口', protocol: 'HTTP', host: 'proxy-us.example.net', port: '8080', region: '美国 · 洛杉矶', latency: '142 ms', expiry: '2027-03-12', status: 'active', refs: ['Video MCP / Google Omni · 渠道 01', 'Video MCP / Google Omni · 渠道 02'], tag: 'google, primary', tested: '2 分钟前', exitIp: '198.51.100.24', probeTarget: 'egress_probe · ipify', probeVersion: 'probe-2026.09' },
  { id: 'proxy_01IMGCDN', name: 'Image CDN', protocol: 'SOCKS5H', host: 'proxy-sg.example.net', port: '7891', region: '新加坡', latency: '188 ms', expiry: '2026-12-31', status: 'active', refs: ['Image MCP / Image CDN · 主出口'], tag: 'image', tested: '18 分钟前', exitIp: '203.0.113.18', probeTarget: 'egress_probe · ipify', probeVersion: 'probe-2026.09' },
  { id: 'proxy_01MUSIC', name: 'Music Provider', protocol: 'HTTPS', host: 'proxy-jp.example.net', port: '443', region: '日本 · 东京', latency: '—', expiry: '长期有效', status: 'pending', refs: [], tag: 'music', tested: '尚未测试' },
  { id: 'proxy_01BACKUP', name: '备用出口', protocol: 'SOCKS5', host: 'proxy-hk.example.net', port: '1080', region: '香港', latency: '—', expiry: '2026-09-01', status: 'off', refs: [], tag: 'backup', tested: '2026-08-20' },
])

const query = ref('')
const protocol = ref('')
const status = ref('')
const binding = ref('')
const selected = ref<string[]>([])
const drawer = ref<ProxyRow | null>(null)
const modal = ref<'add' | 'edit' | 'import' | 'delete' | null>(null)
const editingId = ref<string | null>(null)
const deleteIds = ref<string[]>([])
const notice = ref('')
const importText = ref('')
const importPreview = ref('')
const form = reactive({ name: '', protocol: 'HTTP', host: '', port: '', region: '', expiry: '长期有效', tag: '', username: '', password: '', note: '' })
const currentTheme = ref('light')

const statusText: Record<ProxyStatus, string> = { active: '正常', pending: '待测试', off: '已停用', testing: '测试中' }
const probeFixtures: Record<string, { latency: string; ip: string; region: string }> = {
  proxy_01HOMNI: { latency: '142 ms', ip: '198.51.100.24', region: '美国 · 洛杉矶' },
  proxy_01IMGCDN: { latency: '188 ms', ip: '203.0.113.18', region: '新加坡' },
  proxy_01MUSIC: { latency: '241 ms', ip: '192.0.2.44', region: '日本 · 东京' },
  proxy_01BACKUP: { latency: '—', ip: '', region: '香港' }
}

const filtered = computed(() => rows.value.filter((row) => {
  const needle = query.value.trim().toLowerCase()
  return (!needle || `${row.name} ${row.host} ${row.tag}`.toLowerCase().includes(needle)) &&
    (!protocol.value || row.protocol === protocol.value) &&
    (!status.value || row.status === status.value) &&
    (!binding.value || (binding.value === 'bound' ? row.refs.length > 0 : binding.value === 'unbound' ? row.refs.length === 0 : true))
}))

const activeCount = computed(() => rows.value.filter((row) => row.status === 'active').length)
const pendingCount = computed(() => rows.value.filter((row) => row.status === 'pending').length)
const referenceCount = computed(() => rows.value.reduce((total, row) => total + row.refs.length, 0))
const allSelected = computed(() => filtered.value.length > 0 && filtered.value.every((row) => selected.value.includes(row.id)))

function flash(message: string) {
  notice.value = message
  window.setTimeout(() => { notice.value = '' }, 2600)
}

function resetForm(row?: ProxyRow) {
  Object.assign(form, row
    ? { ...row, username: '', password: '', note: '' }
    : { name: '', protocol: 'HTTP', host: '', port: '', region: '', expiry: '长期有效', tag: '', username: '', password: '', note: '' })
}

function openAdd() {
  editingId.value = null
  resetForm()
  modal.value = 'add'
}

function openEdit(row: ProxyRow) {
  editingId.value = row.id
  resetForm(row)
  drawer.value = null
  modal.value = 'edit'
}

function save() {
  if (!form.name || !form.host || !form.port) return
  if (editingId.value) {
    const row = rows.value.find((item) => item.id === editingId.value)
    if (row) Object.assign(row, { ...form, status: 'pending', latency: '—', tested: '尚未测试', exitIp: '', probeTarget: '', probeVersion: '' })
    flash('代理已保存，等待重新测试')
  } else {
    rows.value.unshift({
      id: `proxy_${Math.random().toString(36).slice(2, 9).toUpperCase()}`,
      name: form.name,
      protocol: form.protocol,
      host: form.host,
      port: form.port,
      region: form.region || '未探测',
      latency: '—',
      expiry: form.expiry,
      status: 'pending',
      refs: [],
      tag: form.tag || 'untagged',
      tested: '尚未测试',
      exitIp: '',
      probeTarget: '',
      probeVersion: ''
    })
    flash('代理已保存，待连接测试')
  }
  modal.value = null
}

function test(row: ProxyRow) {
  if (row.status === 'testing') return
  row.status = 'testing'
  flash(`正在探测 ${row.name}…`)
  window.setTimeout(() => {
    const fixture = probeFixtures[row.id] ?? { latency: '203 ms', ip: '192.0.2.88', region: row.region }
    row.status = 'active'
    row.latency = fixture.latency
    row.exitIp = fixture.ip
    row.region = fixture.region
    row.probeTarget = 'egress_probe · ipify'
    row.probeVersion = 'probe-2026.09'
    row.tested = '刚刚'
    flash(`${row.name} Core 探测通过，出口 ${row.exitIp || '未返回'}`)
  }, 900)
}

function toggle(row: ProxyRow) {
  row.status = row.status === 'off' ? 'pending' : 'off'
  flash(row.status === 'off' ? '代理已停用' : '代理已启用')
}

function askDelete(rowsToDelete: ProxyRow[]) {
  if (rowsToDelete.some((row) => row.refs.length)) {
    flash('已被渠道引用，不能直接删除；请先解除引用或停用')
    return
  }
  deleteIds.value = rowsToDelete.map((row) => row.id)
  editingId.value = deleteIds.value[0] ?? null
  modal.value = 'delete'
}

function confirmDelete() {
  rows.value = rows.value.filter((row) => !deleteIds.value.includes(row.id))
  selected.value = selected.value.filter((id) => !deleteIds.value.includes(id))
  deleteIds.value = []
  modal.value = null
  drawer.value = null
  flash('代理已删除')
}

function toggleAll() {
  selected.value = allSelected.value
    ? selected.value.filter((id) => !filtered.value.some((row) => row.id === id))
    : [...new Set([...selected.value, ...filtered.value.map((row) => row.id)])]
}

function batchTest() {
  selected.value.map((id) => rows.value.find((row) => row.id === id)).filter(Boolean).forEach((row) => test(row as ProxyRow))
}

function parseImport() {
  const lines = importText.value.split('\n').map((line) => line.trim()).filter(Boolean)
  const valid = lines.filter((line) => /^(https?|socks5h?):\/\/.*:\d+$/.test(line)).length
  importPreview.value = `解析结果：${valid} 条有效，${lines.length - valid} 条需检查，0 条重复`
}

function confirmImport() {
  modal.value = null
  importText.value = ''
  importPreview.value = ''
  flash('已导入代理，待测试')
}

function syncTheme(theme: string) {
  currentTheme.value = theme
  document.body.dataset.theme = theme
  document.documentElement.dataset.theme = theme
  localStorage.setItem('vf_studio_theme', theme)
}

function toggleTheme() {
  syncTheme(currentTheme.value === 'dark' ? 'light' : 'dark')
}

onMounted(() => {
  const saved = localStorage.getItem('vf_studio_theme') || document.body.dataset.theme || 'light'
  syncTheme(saved)
})
</script>

<template>
  <div class="proxy-page">
    <!-- Left Sidebar: Complete Integrated Brand & Primary Navigation (Aligned with Studio OS / Market v1.1) -->
    <aside class="side" aria-label="Studio 主菜单">
      <a class="side-brand" href="#/market">
        <img id="vfSideLogo"
             :src="vfLogo"
             alt="VerdantFlare"
             width="26"
             height="26">
        <div class="brand-text">
          <span>青焰 · VerdantFlare</span>
          <small>STUDIO OS</small>
        </div>
      </a>

      <button class="side-create-btn"
              id="newTask"
              aria-label="新建创作任务"
              @click="flash('创作任务功能开发中')">
        <svg width="15"
             height="15"
             viewBox="0 0 24 24"
             fill="none"
             stroke="currentColor"
             stroke-width="2.2"
             stroke-linecap="round"
             stroke-linejoin="round">
          <line x1="12" y1="5" x2="12" y2="19"></line>
          <line x1="5" y1="12" x2="19" y2="12"></line>
        </svg>
        <span>新建创作任务</span>
      </button>

      <div class="nav-label">创作空间</div>
      <a class="nav-link"
         href="#/market"
         data-nav="workbench">
        <span class="nav-icon">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
            <path d="M3 9l9-7 9 7v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"></path>
            <polyline points="9 22 9 12 15 12 15 22"></polyline>
          </svg>
        </span>
        <span>工作台</span>
      </a>

      <a class="nav-link"
         href="#/market"
         data-nav="market">
        <span class="nav-icon">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
            <rect x="3" y="3" width="7" height="7"></rect>
            <rect x="14" y="3" width="7" height="7"></rect>
            <rect x="14" y="14" width="7" height="7"></rect>
            <rect x="3" y="14" width="7" height="7"></rect>
          </svg>
        </span>
        <span>应用市场</span>
      </a>

      <a class="nav-link"
         href="#/market?mode=models"
         data-nav="models">
        <span class="nav-icon">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
            <circle cx="12" cy="12" r="8"></circle>
            <line x1="12" y1="2" x2="12" y2="6"></line>
            <line x1="12" y1="18" x2="12" y2="22"></line>
            <line x1="4.93" y1="4.93" x2="7.76" y2="7.76"></line>
            <line x1="16.24" y1="16.24" x2="19.07" y2="19.07"></line>
            <line x1="2" y1="12" x2="6" y2="12"></line>
            <line x1="18" y1="12" x2="22" y2="12"></line>
            <line x1="4.93" y1="19.07" x2="7.76" y2="16.24"></line>
            <line x1="16.24" y1="7.76" x2="19.07" y2="4.93"></line>
          </svg>
        </span>
        <span>模型市场</span>
      </a>

      <a class="nav-link"
         href="#/market?mode=resources"
         data-nav="resources">
        <span class="nav-icon">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
            <rect x="2" y="2" width="20" height="8" rx="2" ry="2"></rect>
            <rect x="2" y="14" width="20" height="8" rx="2" ry="2"></rect>
            <line x1="6" y1="6" x2="6.01" y2="6"></line>
            <line x1="6" y1="18" x2="6.01" y2="18"></line>
          </svg>
        </span>
        <span>资源管理</span>
      </a>

      <a class="nav-link selected"
         href="#/station/proxies"
         data-nav="proxies">
        <span class="nav-icon">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
            <path d="M13 2L3 14h9l-1 8 10-12h-9l1-8z"></path>
          </svg>
        </span>
        <span>网络出口</span>
      </a>

      <div class="side-bottom">
        <!-- 1. Station Cluster Online Card (Clickable to switch to Resource Center) -->
        <a href="#/market?mode=resources"
           class="side-cluster-card"
           data-nav="resources"
           id="sideClusterCard"
           title="当前连接：5090集群 (k8s.dev.verdantflarehub.com) · 点击查看资源看板">
          <div class="side-cluster-row">
            <div class="side-cluster-lead">
              <i class="dot on"></i>
              <span class="side-cluster-name">5090集群</span>
            </div>
            <span class="side-cluster-pill">在线</span>
          </div>
          <div class="side-cluster-meta">Core 就绪 · 8 卡 RTX 5090</div>
        </a>

        <!-- 2. User Login Card -->
        <div class="side-user-card"
             id="sideUserCard"
             title="当前操作员：admin · 青岚创意工作室">
          <div class="avatar">VF</div>
          <div class="side-user-info">
            <span class="side-user-name">admin</span>
            <span class="side-user-team">青岚创意工作室</span>
          </div>
        </div>

        <a class="nav-link side-help-link"
           href="#/market"
           @click.prevent="flash('帮助与设计说明正在完善中')"
           title="帮助与设计说明">
          <span class="nav-icon">
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
              <circle cx="12" cy="12" r="9"></circle>
              <path d="M9.8 9a2.4 2.4 0 1 1 3.2 2.3c-.7.3-1 1-1 1.7v.3M12 17h.01"></path>
            </svg>
          </span>
          <span>帮助与设计说明</span>
        </a>
      </div>
    </aside>

    <!-- Main Content Area: Pure Workspace -->
    <main class="main">
      <div class="workspace proxy-workspace">
        <header class="heading">
          <div class="heading-left">
            <span class="eyebrow">STATION · NETWORK EGRESS</span>
            <h1>网络出口</h1>
            <p>维护 Station 的代理出口，供 Video、Image 和 Music MCP 渠道引用。</p>
          </div>
          <div class="heading-actions">
            <button class="btn ghost" @click="toggleTheme">{{ currentTheme === 'dark' ? '浅色主题' : '深色主题' }}</button>
            <button class="btn" @click="flash('代理列表已刷新')">刷新状态</button>
            <button class="btn primary" @click="openAdd">＋ 添加代理</button>
          </div>
        </header>

        <div class="review-note">
          <span class="review-dot"></span>
          <b>开发预览</b> 示例数据 · Core API 接入后替换为真实代理事实
        </div>

        <section class="proxy-metrics">
          <article>
            <span>代理总数</span>
            <strong>{{ rows.length }}</strong>
            <small>当前 Station</small>
          </article>
          <article>
            <span>可用代理</span>
            <strong class="ok">{{ activeCount }}</strong>
            <small>Core 探测通过</small>
          </article>
          <article>
            <span>待处理</span>
            <strong class="warn">{{ pendingCount }}</strong>
            <small>需要连接测试</small>
          </article>
          <article>
            <span>已绑定渠道</span>
            <strong>{{ referenceCount }}</strong>
            <small>由各 MCP 管理绑定</small>
          </article>
          <article>
            <span>最近探测</span>
            <strong class="metric-time">2 分钟前</strong>
            <small>固定探测目标</small>
          </article>
        </section>

        <section class="proxy-panel">
          <div class="proxy-toolbar">
            <label class="proxy-search">
              <span>⌕</span>
              <input v-model="query" placeholder="搜索代理名称或地址" aria-label="搜索代理名称或地址">
            </label>
            <select v-model="protocol">
              <option value="">协议：全部</option>
              <option>HTTP</option>
              <option>HTTPS</option>
              <option>SOCKS5</option>
              <option>SOCKS5H</option>
            </select>
            <select v-model="status">
              <option value="">状态：全部</option>
              <option value="active">正常</option>
              <option value="pending">待测试</option>
              <option value="off">已停用</option>
              <option value="testing">测试中</option>
            </select>
            <select v-model="binding">
              <option value="">绑定：全部</option>
              <option value="bound">已绑定</option>
              <option value="unbound">未绑定</option>
            </select>
            <span class="toolbar-grow"></span>
            <button class="btn" @click="modal = 'import'">⇧ 导入</button>
            <button class="btn" @click="flash('已生成脱敏导出文件（开发预览未下载）')">⇩ 导出</button>
          </div>

          <div v-if="selected.length" class="selection">
            <b>{{ selected.length }}</b> 个代理已选择
            <span class="toolbar-grow"></span>
            <button class="btn" @click="batchTest">▷ 测试连接</button>
            <button class="btn danger" @click="askDelete(rows.filter((row) => selected.includes(row.id)))">删除</button>
          </div>

          <div class="table-wrap">
            <table>
              <thead>
                <tr>
                  <th><input type="checkbox" :checked="allSelected" @change="toggleAll"></th>
                  <th>代理</th>
                  <th>协议</th>
                  <th>地址</th>
                  <th>出口位置</th>
                  <th>延迟</th>
                  <th>引用</th>
                  <th>有效期</th>
                  <th>状态</th>
                  <th></th>
                </tr>
              </thead>
              <tbody>
                <tr v-for="row in filtered" :key="row.id" :class="{chosen: selected.includes(row.id)}">
                  <td>
                    <input type="checkbox"
                           :checked="selected.includes(row.id)"
                           @change="selected.includes(row.id) ? selected = selected.filter((id) => id !== row.id) : selected.push(row.id)">
                  </td>
                  <td>
                    <button class="proxy-name" @click="drawer = row">{{ row.name }}</button>
                    <small>{{ row.id }} · {{ row.tag }}</small>
                  </td>
                  <td><span class="protocol-tag">{{ row.protocol }}</span></td>
                  <td><code>{{ row.host }}:{{ row.port }}</code></td>
                  <td>{{ row.region }}</td>
                  <td :class="row.status === 'active' ? 'ok' : 'warn'">{{ row.latency }}</td>
                  <td>{{ row.refs.length }} 个渠道</td>
                  <td>{{ row.expiry }}</td>
                  <td><span :class="['status', row.status]"><i class="dot"></i>{{ statusText[row.status] }}</span></td>
                  <td class="row-actions">
                    <button class="icon-button" title="查看详情" @click="drawer = row">查看</button>
                    <button class="icon-button" title="编辑" @click="openEdit(row)">编辑</button>
                    <button class="icon-button" @click="row.refs.length ? (drawer = row) : askDelete([row])">{{ row.refs.length ? '引用' : '删除' }}</button>
                  </td>
                </tr>
              </tbody>
            </table>
            <div v-if="!filtered.length" class="empty">
              <b>还没有匹配的代理</b>
              <span>调整筛选条件，或者添加一个新的代理。</span>
              <button class="btn" @click="openAdd">＋ 添加代理</button>
            </div>
            <footer class="table-footer">
              <span>显示 {{ filtered.length }} 个代理</span>
              <span>‹　<strong>1</strong>　›</span>
            </footer>
          </div>
        </section>

        <p class="proxy-footnote">代理出口只由 Station Core 维护；渠道配置、供应商连通性和渠道测试由对应 MCP 负责。</p>
      </div>
    </main>

    <!-- Detail Drawer -->
    <div v-if="drawer" class="drawer-shade" @click="drawer = null"></div>
    <aside v-if="drawer" class="proxy-drawer">
      <header>
        <div>
          <span class="eyebrow">PROXY ENDPOINT</span>
          <h2>{{ drawer.name }}</h2>
          <small>{{ drawer.id }}</small>
        </div>
        <button class="drawer-close" @click="drawer = null">×</button>
      </header>
      <div class="drawer-body">
        <span :class="['status', drawer.status]"><i class="dot"></i>{{ statusText[drawer.status] }}</span>
        <section>
          <h3>Core 代理探测</h3>
          <p class="probe-explain">Station Core 经此代理访问固定探测目标，只证明代理出口可达，不代表供应商渠道已接入。</p>
          <dl>
            <div><dt>协议</dt><dd>{{ drawer.protocol }}</dd></div>
            <div><dt>地址</dt><dd>{{ drawer.host }}:{{ drawer.port }}</dd></div>
            <div><dt>观测出口 IP</dt><dd>{{ drawer.exitIp || '尚未探测' }}</dd></div>
            <div><dt>出口位置</dt><dd>{{ drawer.region }}</dd></div>
            <div><dt>探测目标 / 版本</dt><dd>{{ drawer.probeTarget || '—' }}<small v-if="drawer.probeVersion">{{ drawer.probeVersion }}</small></dd></div>
            <div><dt>最近延迟</dt><dd>{{ drawer.latency }}</dd></div>
            <div><dt>认证</dt><dd class="masked">已配置 · 不回显</dd></div>
            <div><dt>最近探测</dt><dd>{{ drawer.tested }}</dd></div>
          </dl>
          <button class="btn primary" @click="test(drawer)">▷ Core 探测</button>
        </section>
        <section>
          <h3>后端匹配方式</h3>
          <p class="probe-explain">后端按 <code>proxy_id</code> 取 ProxyLease 和凭据版本配置 transport；不会按出口 IP 反查代理。</p>
        </section>
        <section>
          <h3>引用渠道 · {{ drawer.refs.length }}</h3>
          <p v-if="!drawer.refs.length" class="muted">暂无渠道引用，可在对应 MCP 渠道设置中绑定。</p>
          <ul v-else>
            <li v-for="reference in drawer.refs" :key="reference">{{ reference }}</li>
          </ul>
        </section>
      </div>
    </aside>

    <!-- Modals -->
    <div v-if="modal" class="modal-shade" @click.self="modal = null">
      <section class="proxy-modal">
        <header>
          <div>
            <span class="eyebrow">STATION CORE · PROXY</span>
            <h2>{{ modal === 'add' ? '添加代理' : modal === 'edit' ? '编辑代理' : modal === 'import' ? '批量导入代理' : '删除代理' }}</h2>
          </div>
          <button class="drawer-close" @click="modal = null">×</button>
        </header>

        <form v-if="modal === 'add' || modal === 'edit'" @submit.prevent="save">
          <div class="form-grid">
            <label class="full">名称<input v-model="form.name" required placeholder="例如：Google Omni 出口"></label>
            <label>协议
              <select v-model="form.protocol">
                <option>HTTP</option>
                <option>HTTPS</option>
                <option>SOCKS5</option>
                <option>SOCKS5H</option>
              </select>
            </label>
            <label>主机<input v-model="form.host" required placeholder="proxy.example.net"></label>
            <label>端口<input v-model="form.port" required type="number" placeholder="8080"></label>
            <label>出口位置<input v-model="form.region" placeholder="由 Core 探测填写"></label>
            <p class="secret full">凭据由 Station Secret Store 加密保存。密码只写入，不回显已有明文。</p>
            <label>用户名<input v-model="form.username" placeholder="proxy-user"></label>
            <label>密码<input v-model="form.password" type="password" placeholder="输入新密码以轮换"></label>
            <label>有效期
              <select v-model="form.expiry">
                <option>长期有效</option>
                <option>30 天后到期</option>
                <option>90 天后到期</option>
              </select>
            </label>
            <label>标签<input v-model="form.tag" placeholder="google, primary"></label>
            <label class="full">备注<textarea v-model="form.note"></textarea></label>
          </div>
          <footer>
            <button type="button" class="btn" @click="modal = null">取消</button>
            <button class="btn primary">保存代理</button>
          </footer>
        </form>

        <div v-else-if="modal === 'import'" class="import-body">
          <label>粘贴代理 URL（每行一条）<textarea v-model="importText" placeholder="http://user:password@proxy.example.net:8080"></textarea></label>
          <p v-if="importPreview" class="preview">{{ importPreview }}</p>
          <footer>
            <button class="btn" @click="modal = null">取消</button>
            <button class="btn" @click="parseImport">预览解析</button>
            <button class="btn primary" @click="confirmImport">确认导入</button>
          </footer>
        </div>

        <div v-else class="delete-body">
          <p>确定删除 <b>{{ rows.find((row) => row.id === editingId)?.name }}</b> 吗？删除后不能恢复。</p>
          <small>已引用代理不能直接删除。</small>
          <footer>
            <button class="btn" @click="modal = null">取消</button>
            <button class="btn danger" @click="confirmDelete">确认删除</button>
          </footer>
        </div>
      </section>
    </div>

    <transition name="toast">
      <div v-if="notice" class="toast">{{ notice }}</div>
    </transition>
  </div>
</template>

<style scoped>
.proxy-page {
  min-height: 100vh;
  background: var(--bg);
  color: var(--ink);
}

.proxy-workspace {
  width: 100%;
  max-width: 100%;
}

.review-note {
  display: flex;
  align-items: center;
  gap: 7px;
  padding: 9px 14px;
  margin-bottom: 20px;
  border: 1px solid var(--line);
  border-radius: 10px;
  color: var(--muted);
  background: var(--shell-panel);
  font-size: 12px;
}

.review-note b {
  color: var(--ink);
  font-weight: 650;
}

.review-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--accent);
}

.proxy-metrics {
  display: grid;
  grid-template-columns: repeat(5, minmax(0, 1fr));
  gap: 12px;
  margin: 0 0 20px;
}

.proxy-metrics article {
  min-height: 104px;
  padding: 16px;
  border: 1px solid var(--line);
  border-radius: 12px;
  background: var(--shell-panel);
}

.proxy-metrics span,
.proxy-metrics small {
  display: block;
  color: var(--muted);
  font-size: 11px;
}

.proxy-metrics strong {
  display: block;
  margin: 10px 0 3px;
  color: var(--ink);
  font-size: 25px;
  font-weight: 560;
}

.proxy-metrics .metric-time {
  font-size: 18px;
  margin-top: 14px;
}

.ok {
  color: var(--accent) !important;
}

.warn {
  color: var(--amber) !important;
}

.proxy-panel {
  overflow: hidden;
  border: 1px solid var(--line);
  border-radius: 14px;
  background: var(--shell-panel);
}

.proxy-toolbar {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  padding: 14px;
  border-bottom: 1px solid var(--line);
}

.proxy-toolbar .btn,
.proxy-toolbar select,
.selection .btn {
  height: 34px;
  padding: 0 10px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--shell-button);
  color: var(--ink);
  font-size: 11px;
}

.proxy-toolbar select option {
  background: var(--panel);
  color: var(--ink);
}

.proxy-toolbar select {
  min-width: 96px;
}

.proxy-toolbar .btn:hover,
.selection .btn:hover {
  border-color: var(--accent);
}

.proxy-search {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 230px;
  flex: 1;
  height: 34px;
  padding: 0 10px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--shell-panel);
  color: var(--muted);
}

.proxy-search input {
  width: 100%;
  border: 0;
  outline: 0;
  background: transparent;
  color: var(--ink);
  font-size: 11px;
}

.toolbar-grow {
  flex: 1;
}

.selection {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 9px 14px;
  border-bottom: 1px solid var(--line);
  background: var(--shell-button);
  color: var(--muted);
  font-size: 11px;
}

.selection b {
  color: var(--ink);
}

.selection .danger,
.danger {
  color: var(--danger) !important;
}

.table-wrap {
  overflow: auto;
}

.table-wrap table {
  width: 100%;
  min-width: 980px;
  border-collapse: collapse;
}

.table-wrap th {
  height: 42px;
  padding: 0 14px;
  border-bottom: 1px solid var(--line);
  color: var(--muted);
  background: var(--shell-button);
  font-size: 10px;
  font-weight: 600;
  text-align: left;
  white-space: nowrap;
}

.table-wrap td {
  padding: 13px 14px;
  border-bottom: 1px solid var(--line);
  color: var(--ink);
  font-size: 11px;
  white-space: nowrap;
}

.table-wrap tr.chosen {
  background: var(--shell-button);
}

.table-wrap input[type=checkbox] {
  accent-color: var(--accent);
}

.proxy-name {
  display: block;
  padding: 0;
  border: 0;
  background: none;
  color: var(--ink);
  font-size: 12px;
  font-weight: 650;
  text-align: left;
}

.proxy-name:hover {
  color: var(--accent);
}

.table-wrap td small {
  display: block;
  margin-top: 3px;
  color: var(--muted);
  font-size: 9px;
}

.protocol-tag {
  padding: 3px 6px;
  border: 1px solid var(--line);
  border-radius: 5px;
  color: var(--accent);
  font-size: 9px;
  font-weight: 700;
}

.table-wrap code {
  color: var(--ink);
  font-size: 11px;
}

.status {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 11px;
}

.status.active { color: var(--accent); }
.status.pending { color: var(--amber); }
.status.off { color: var(--muted); }
.status.testing { color: var(--blue); }

.status .dot {
  width: 6px;
  height: 6px;
  background: currentColor;
  border-radius: 50%;
}

.dot.on {
  background: var(--accent);
}

.row-actions {
  white-space: nowrap;
  text-align: right;
}

.icon-button {
  padding: 4px 6px;
  border: 0;
  border-radius: 6px;
  background: transparent;
  color: var(--muted);
  font-size: 10px;
}

.icon-button:hover {
  background: var(--shell-button);
  color: var(--ink);
}

.table-footer {
  display: flex;
  justify-content: space-between;
  padding: 12px 14px;
  color: var(--muted);
  font-size: 10px;
}

.table-footer strong {
  color: var(--ink);
}

.empty {
  padding: 48px 20px;
  color: var(--muted);
  text-align: center;
}

.empty b {
  display: block;
  margin-bottom: 5px;
  color: var(--ink);
}

.empty .btn {
  margin-top: 13px;
}

.proxy-footnote {
  margin: 15px 2px 35px;
  color: var(--muted);
  font-size: 10px;
}

.drawer-shade,
.modal-shade {
  position: fixed;
  inset: 0;
  z-index: 20;
  background: rgba(21, 35, 30, .28);
  backdrop-filter: blur(2px);
}

.proxy-drawer {
  position: fixed;
  top: 0;
  right: 0;
  z-index: 21;
  width: min(540px, 100%);
  height: 100vh;
  overflow: auto;
  border-left: 1px solid var(--line);
  background: var(--panel);
  color: var(--ink);
  box-shadow: 0 20px 70px rgba(21, 35, 30, .22);
}

.proxy-drawer header,
.proxy-modal header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  padding: 25px 26px 18px;
  border-bottom: 1px solid var(--line);
}

.proxy-drawer h2,
.proxy-modal h2 {
  margin: 5px 0 2px;
  font-size: 20px;
  font-weight: 580;
}

.proxy-drawer header small {
  color: var(--muted);
  font: 10px ui-monospace, monospace;
}

.drawer-close {
  border: 0;
  background: transparent;
  color: var(--muted);
  font-size: 23px;
}

.drawer-body {
  padding: 20px 26px 35px;
}

.drawer-body section {
  padding: 20px 0;
  border-bottom: 1px solid var(--line);
}

.drawer-body h3 {
  margin: 0 0 12px;
  color: var(--muted);
  font-size: 10px;
  letter-spacing: 1.4px;
}

.drawer-body dl {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 15px;
}

.drawer-body dt {
  color: var(--muted);
  font-size: 10px;
}

.drawer-body dd {
  margin: 3px 0 0;
  color: var(--ink);
  font-size: 12px;
}

.drawer-body dd small {
  display: block;
  color: var(--muted);
  font-size: 10px;
}

.probe-explain {
  margin: 0 0 14px;
  color: var(--muted);
  font-size: 11px;
  line-height: 1.65;
}

.drawer-body code {
  color: var(--accent);
}

.masked {
  color: var(--muted) !important;
}

.drawer-body ul {
  padding: 0;
  margin: 0;
  list-style: none;
}

.drawer-body li {
  padding: 9px 0;
  border-bottom: 1px solid var(--line);
  color: var(--ink);
  font-size: 11px;
}

.drawer-body .btn {
  margin-top: 15px;
}

.modal-shade {
  display: grid;
  place-items: center;
}

.proxy-modal {
  width: min(620px, calc(100% - 28px));
  max-height: calc(100vh - 35px);
  overflow: auto;
  border: 1px solid var(--line);
  border-radius: 14px;
  background: var(--panel);
  color: var(--ink);
  box-shadow: 0 24px 80px rgba(21, 35, 30, .2);
}

.proxy-modal form,
.import-body,
.delete-body {
  padding: 21px 26px;
}

.form-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
}

.form-grid label,
.import-body label {
  display: grid;
  gap: 6px;
  color: var(--muted);
  font-size: 11px;
}

.form-grid .full {
  grid-column: 1 / -1;
}

.form-grid input,
.form-grid select,
.form-grid textarea,
.import-body textarea {
  padding: 9px 10px;
  border: 1px solid var(--line);
  border-radius: 8px;
  outline: 0;
  background: var(--subtle);
  color: var(--ink);
  font: inherit;
}

.form-grid textarea {
  min-height: 65px;
}

.secret {
  padding: 10px;
  border: 1px solid var(--line);
  border-radius: 8px;
  background: var(--shell-button);
  color: var(--amber) !important;
  font-size: 11px !important;
}

.proxy-modal footer {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 14px 26px;
  border-top: 1px solid var(--line);
}

.import-body textarea {
  width: 100%;
  min-height: 145px;
  resize: vertical;
}

.preview {
  padding: 10px;
  border-radius: 8px;
  background: var(--shell-button);
  color: var(--accent);
  font-size: 11px;
}

.delete-body p {
  color: var(--muted);
}

.delete-body small {
  color: var(--amber);
}

.toast {
  position: fixed;
  right: 24px;
  bottom: 42px;
  z-index: 30;
  padding: 11px 15px;
  border-radius: 9px;
  background: var(--paper);
  color: var(--button-ink);
  font-size: 11px;
  box-shadow: 0 15px 45px rgba(0, 0, 0, .16);
}

@media(max-width: 900px) {
  .proxy-metrics {
    grid-template-columns: repeat(3, 1fr);
  }
  .proxy-metrics article:nth-child(4),
  .proxy-metrics article:nth-child(5) {
    display: none;
  }
}

@media(max-width: 680px) {
  .proxy-metrics {
    grid-template-columns: repeat(2, 1fr);
    gap: 8px;
  }
  .proxy-toolbar {
    align-items: stretch;
  }
  .proxy-search {
    min-width: 100%;
  }
  .proxy-toolbar select {
    flex: 1;
  }
  .toolbar-grow {
    display: none;
  }
  .form-grid {
    grid-template-columns: 1fr;
  }
  .form-grid .full {
    grid-column: auto;
  }
  .proxy-drawer {
    width: 100%;
  }
}
</style>
