<script setup lang="ts">
import { computed, onMounted, onBeforeUnmount, reactive, ref } from 'vue'

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
let previousTheme: string | undefined

const statusText: Record<ProxyStatus, string> = { active: '正常', pending: '待测试', off: '已停用', testing: '测试中' }
const probeFixtures: Record<string, { latency: string; ip: string; region: string }> = { proxy_01HOMNI: { latency: '142 ms', ip: '198.51.100.24', region: '美国 · 洛杉矶' }, proxy_01IMGCDN: { latency: '188 ms', ip: '203.0.113.18', region: '新加坡' }, proxy_01MUSIC: { latency: '241 ms', ip: '192.0.2.44', region: '日本 · 东京' }, proxy_01BACKUP: { latency: '—', ip: '', region: '香港' } }
const filtered = computed(() => rows.value.filter((row) => {
  const needle = query.value.trim().toLowerCase()
  return (!needle || `${row.name} ${row.host} ${row.tag}`.toLowerCase().includes(needle)) && (!protocol.value || row.protocol === protocol.value) && (!status.value || row.status === status.value) && (!binding.value || (binding.value === 'bound' ? row.refs.length > 0 : binding.value === 'unbound' ? row.refs.length === 0 : true))
}))
const activeCount = computed(() => rows.value.filter((row) => row.status === 'active').length)
const pendingCount = computed(() => rows.value.filter((row) => row.status === 'pending').length)
const referenceCount = computed(() => rows.value.reduce((total, row) => total + row.refs.length, 0))
const allSelected = computed(() => filtered.value.length > 0 && filtered.value.every((row) => selected.value.includes(row.id)))

function flash(message: string) { notice.value = message; window.setTimeout(() => { notice.value = '' }, 2600) }
function resetForm(row?: ProxyRow) { Object.assign(form, row ? { ...row, username: '', password: '', note: '' } : { name: '', protocol: 'HTTP', host: '', port: '', region: '', expiry: '长期有效', tag: '', username: '', password: '', note: '' }) }
function openAdd() { editingId.value = null; resetForm(); modal.value = 'add' }
function openEdit(row: ProxyRow) { editingId.value = row.id; resetForm(row); drawer.value = null; modal.value = 'edit' }
function save() {
  if (!form.name || !form.host || !form.port) return
  if (editingId.value) {
    const row = rows.value.find((item) => item.id === editingId.value)
    if (row) Object.assign(row, { ...form, status: 'pending', latency: '—', tested: '尚未测试', exitIp: '', probeTarget: '', probeVersion: '' })
    flash('代理已保存，等待重新测试')
  } else {
    rows.value.unshift({ id: `proxy_${Math.random().toString(36).slice(2, 9).toUpperCase()}`, name: form.name, protocol: form.protocol, host: form.host, port: form.port, region: form.region || '未探测', latency: '—', expiry: form.expiry, status: 'pending', refs: [], tag: form.tag || 'untagged', tested: '尚未测试', exitIp: '', probeTarget: '', probeVersion: '' })
    flash('代理已保存，待连接测试')
  }
  modal.value = null
}
function test(row: ProxyRow) {
  if (row.status === 'testing') return
  row.status = 'testing'; flash(`正在探测 ${row.name}…`)
  window.setTimeout(() => { const fixture = probeFixtures[row.id] ?? { latency: '203 ms', ip: '192.0.2.88', region: row.region }; row.status = 'active'; row.latency = fixture.latency; row.exitIp = fixture.ip; row.region = fixture.region; row.probeTarget = 'egress_probe · ipify'; row.probeVersion = 'probe-2026.09'; row.tested = '刚刚'; flash(`${row.name} Core 探测通过，出口 ${row.exitIp || '未返回'}`) }, 900)
}
function toggle(row: ProxyRow) { row.status = row.status === 'off' ? 'pending' : 'off'; flash(row.status === 'off' ? '代理已停用' : '代理已启用') }
function askDelete(rowsToDelete: ProxyRow[]) { if (rowsToDelete.some((row) => row.refs.length)) { flash('已被渠道引用，不能直接删除；请先解除引用或停用'); return } deleteIds.value = rowsToDelete.map((row) => row.id); editingId.value = deleteIds.value[0] ?? null; modal.value = 'delete' }
function confirmDelete() { rows.value = rows.value.filter((row) => !deleteIds.value.includes(row.id)); selected.value = selected.value.filter((id) => !deleteIds.value.includes(id)); deleteIds.value = []; modal.value = null; drawer.value = null; flash('代理已删除') }
function toggleAll() { selected.value = allSelected.value ? selected.value.filter((id) => !filtered.value.some((row) => row.id === id)) : [...new Set([...selected.value, ...filtered.value.map((row) => row.id)])] }
function batchTest() { selected.value.map((id) => rows.value.find((row) => row.id === id)).filter(Boolean).forEach((row) => test(row as ProxyRow)) }
function parseImport() { const lines = importText.value.split('\n').map((line) => line.trim()).filter(Boolean); const valid = lines.filter((line) => /^(https?|socks5h?):\/\/.*:\d+$/.test(line)).length; importPreview.value = `解析结果：${valid} 条有效，${lines.length - valid} 条需检查，0 条重复` }
function confirmImport() { modal.value = null; importText.value = ''; importPreview.value = ''; flash('已导入代理，待测试') }
function toggleTheme() { document.body.dataset.theme = document.body.dataset.theme === 'light' ? 'dark' : 'light' }
onMounted(() => { previousTheme = document.body.dataset.theme; document.body.dataset.theme = 'light' })
onBeforeUnmount(() => { if (previousTheme) document.body.dataset.theme = previousTheme; else delete document.body.dataset.theme })
</script>

<template>
  <div class="proxy-page">
    <aside class="side" aria-label="Studio 主菜单">
      <a class="brand" href="#/market"><span class="proxy-brand-mark">VF</span><span>青焰 · Verdantflare<small>STUDIO</small></span></a>
      <div class="side-tools"><button class="btn primary new-task" disabled title="创作任务开发中">＋ 新建创作任务</button><button class="side-collapse" aria-label="收起侧栏">‹</button></div>
      <div class="nav-label">创作空间</div>
      <nav class="main-nav">
        <a class="nav-link" href="#/market"><span class="nav-icon">⌂</span><span class="label">工作台</span></a>
        <a class="nav-link" href="#/market"><span class="nav-icon">▣</span><span class="label">应用市场</span><span class="nav-chevron">⌄</span></a>
        <div class="market-subnav"><button class="group-nav">已安装应用<span class="count">12</span></button><button class="group-nav">Image<span class="count">2</span></button><button class="group-nav">Music<span class="count">6</span></button><button class="group-nav">Video<span class="count">5</span></button></div>
        <a class="nav-link" href="#/market"><span class="nav-icon">◇</span><span class="label">模型市场</span></a>
        <a class="nav-link selected" href="#/station/proxies"><span class="nav-icon">⌁</span><span class="label">网络出口</span></a>
        <a class="nav-link" href="#/market"><span class="nav-icon">□</span><span class="label">项目</span></a>
        <a class="nav-link" href="#/market"><span class="nav-icon">◎</span><span class="label">数字世界</span></a>
      </nav>
      <div class="recent-projects"><div class="nav-label">项目</div><p>项目管理开发中</p></div>
      <div class="side-bottom"><a class="nav-link" href="#/market"><span class="nav-icon">?</span><span class="label">帮助与设计说明</span></a><div class="station"><strong><span class="dot"></span><span>成都 Station</span></strong><p>Core 已连接 · 代理预览</p></div></div>
    </aside>
    <main class="main">
      <div class="workspace proxy-workspace">
        <header class="heading">
          <div><span class="eyebrow">STATION · NETWORK EGRESS</span><h1>网络出口</h1><p>维护 Station 的代理出口，供 Video、Image 和 Music MCP 渠道引用。</p></div>
          <div class="heading-actions"><button class="btn ghost" @click="toggleTheme">深色主题</button><button class="btn" @click="flash('代理列表已刷新')">刷新状态</button><button class="btn primary" @click="openAdd">＋ 添加代理</button></div>
        </header>
        <div class="workspace-strip"><div class="workspace-avatar">VF</div><div><strong>成都 Station</strong><p>Station Core · 网络出口</p></div><div class="strip-right"><span class="pill">管理员视角</span><span class="demo">开发预览</span></div></div>
        <div class="review-note"><span class="review-dot"></span><b>开发预览</b> 示例数据 · Core API 接入后替换为真实代理事实</div>
        <section class="proxy-metrics">
          <article><span>代理总数</span><strong>{{ rows.length }}</strong><small>当前 Station</small></article>
          <article><span>可用代理</span><strong class="ok">{{ activeCount }}</strong><small>Core 探测通过</small></article>
          <article><span>待处理</span><strong class="warn">{{ pendingCount }}</strong><small>需要连接测试</small></article>
          <article><span>已绑定渠道</span><strong>{{ referenceCount }}</strong><small>由各 MCP 管理绑定</small></article>
          <article><span>最近探测</span><strong class="metric-time">2 分钟前</strong><small>固定探测目标</small></article>
        </section>
        <section class="proxy-panel">
          <div class="proxy-toolbar"><label class="proxy-search"><span>⌕</span><input v-model="query" placeholder="搜索代理名称或地址" aria-label="搜索代理名称或地址"></label><select v-model="protocol"><option value="">协议：全部</option><option>HTTP</option><option>HTTPS</option><option>SOCKS5</option><option>SOCKS5H</option></select><select v-model="status"><option value="">状态：全部</option><option value="active">正常</option><option value="pending">待测试</option><option value="off">已停用</option><option value="testing">测试中</option></select><select v-model="binding"><option value="">绑定：全部</option><option value="bound">已绑定</option><option value="unbound">未绑定</option></select><span class="toolbar-grow"></span><button class="btn" @click="modal = 'import'">⇧ 导入</button><button class="btn" @click="flash('已生成脱敏导出文件（开发预览未下载）')">⇩ 导出</button></div>
          <div v-if="selected.length" class="selection"><b>{{ selected.length }}</b> 个代理已选择<span class="toolbar-grow"></span><button class="btn" @click="batchTest">▷ 测试连接</button><button class="btn danger" @click="askDelete(rows.filter((row) => selected.includes(row.id)))">删除</button></div>
          <div class="table-wrap"><table><thead><tr><th><input type="checkbox" :checked="allSelected" @change="toggleAll"></th><th>代理</th><th>协议</th><th>地址</th><th>出口位置</th><th>延迟</th><th>引用</th><th>有效期</th><th>状态</th><th></th></tr></thead><tbody><tr v-for="row in filtered" :key="row.id" :class="{chosen: selected.includes(row.id)}"><td><input type="checkbox" :checked="selected.includes(row.id)" @change="selected.includes(row.id) ? selected = selected.filter((id) => id !== row.id) : selected.push(row.id)"></td><td><button class="proxy-name" @click="drawer = row">{{ row.name }}</button><small>{{ row.id }} · {{ row.tag }}</small></td><td><span class="protocol-tag">{{ row.protocol }}</span></td><td><code>{{ row.host }}:{{ row.port }}</code></td><td>{{ row.region }}</td><td :class="row.status === 'active' ? 'ok' : 'warn'">{{ row.latency }}</td><td>{{ row.refs.length }} 个渠道</td><td>{{ row.expiry }}</td><td><span :class="['status', row.status]"><i class="dot"></i>{{ statusText[row.status] }}</span></td><td class="row-actions"><button class="icon-button" title="查看详情" @click="drawer = row">查看</button><button class="icon-button" title="编辑" @click="openEdit(row)">编辑</button><button class="icon-button" @click="row.refs.length ? (drawer = row) : askDelete([row])">{{ row.refs.length ? '引用' : '删除' }}</button></td></tr></tbody></table><div v-if="!filtered.length" class="empty"><b>还没有匹配的代理</b><span>调整筛选条件，或者添加一个新的代理。</span><button class="btn" @click="openAdd">＋ 添加代理</button></div><footer class="table-footer"><span>显示 {{ filtered.length }} 个代理</span><span>‹　<strong>1</strong>　›</span></footer></div>
        </section>
        <p class="proxy-footnote">代理出口只由 Station Core 维护；渠道配置、供应商连通性和渠道测试由对应 MCP 负责。</p>
      </div>
    </main>
    <footer class="statusbar"><span><i class="dot on"></i> 成都 Station · Core 已连接</span><span>代理 {{ rows.length }} · 可用 {{ activeCount }}</span><span class="right">Studio 网络出口 · 开发预览</span></footer>
    <div v-if="drawer" class="drawer-shade" @click="drawer = null"></div><aside v-if="drawer" class="proxy-drawer"><header><div><span class="eyebrow">PROXY ENDPOINT</span><h2>{{ drawer.name }}</h2><small>{{ drawer.id }}</small></div><button class="drawer-close" @click="drawer = null">×</button></header><div class="drawer-body"><span :class="['status', drawer.status]"><i class="dot"></i>{{ statusText[drawer.status] }}</span><section><h3>Core 代理探测</h3><p class="probe-explain">Station Core 经此代理访问固定探测目标，只证明代理出口可达，不代表供应商渠道已接入。</p><dl><div><dt>协议</dt><dd>{{ drawer.protocol }}</dd></div><div><dt>地址</dt><dd>{{ drawer.host }}:{{ drawer.port }}</dd></div><div><dt>观测出口 IP</dt><dd>{{ drawer.exitIp || '尚未探测' }}</dd></div><div><dt>出口位置</dt><dd>{{ drawer.region }}</dd></div><div><dt>探测目标 / 版本</dt><dd>{{ drawer.probeTarget || '—' }}<small v-if="drawer.probeVersion">{{ drawer.probeVersion }}</small></dd></div><div><dt>最近延迟</dt><dd>{{ drawer.latency }}</dd></div><div><dt>认证</dt><dd class="masked">已配置 · 不回显</dd></div><div><dt>最近探测</dt><dd>{{ drawer.tested }}</dd></div></dl><button class="btn primary" @click="test(drawer)">▷ Core 探测</button></section><section><h3>后端匹配方式</h3><p class="probe-explain">后端按 <code>proxy_id</code> 取 ProxyLease 和凭据版本配置 transport；不会按出口 IP 反查代理。</p></section><section><h3>引用渠道 · {{ drawer.refs.length }}</h3><p v-if="!drawer.refs.length" class="muted">暂无渠道引用，可在对应 MCP 渠道设置中绑定。</p><ul v-else><li v-for="reference in drawer.refs" :key="reference">{{ reference }}</li></ul></section></div></aside>
    <div v-if="modal" class="modal-shade" @click.self="modal = null"><section class="proxy-modal"><header><div><span class="eyebrow">STATION CORE · PROXY</span><h2>{{ modal === 'add' ? '添加代理' : modal === 'edit' ? '编辑代理' : modal === 'import' ? '批量导入代理' : '删除代理' }}</h2></div><button class="drawer-close" @click="modal = null">×</button></header><form v-if="modal === 'add' || modal === 'edit'" @submit.prevent="save"><div class="form-grid"><label class="full">名称<input v-model="form.name" required placeholder="例如：Google Omni 出口"></label><label>协议<select v-model="form.protocol"><option>HTTP</option><option>HTTPS</option><option>SOCKS5</option><option>SOCKS5H</option></select></label><label>主机<input v-model="form.host" required placeholder="proxy.example.net"></label><label>端口<input v-model="form.port" required type="number" placeholder="8080"></label><label>出口位置<input v-model="form.region" placeholder="由 Core 探测填写"></label><p class="secret full">凭据由 Station Secret Store 加密保存。密码只写入，不回显已有明文。</p><label>用户名<input v-model="form.username" placeholder="proxy-user"></label><label>密码<input v-model="form.password" type="password" placeholder="输入新密码以轮换"></label><label>有效期<select v-model="form.expiry"><option>长期有效</option><option>30 天后到期</option><option>90 天后到期</option></select></label><label>标签<input v-model="form.tag" placeholder="google, primary"></label><label class="full">备注<textarea v-model="form.note"></textarea></label></div><footer><button type="button" class="btn" @click="modal = null">取消</button><button class="btn primary">保存代理</button></footer></form><div v-else-if="modal === 'import'" class="import-body"><label>粘贴代理 URL（每行一条）<textarea v-model="importText" placeholder="http://user:password@proxy.example.net:8080"></textarea></label><p v-if="importPreview" class="preview">{{ importPreview }}</p><footer><button class="btn" @click="modal = null">取消</button><button class="btn" @click="parseImport">预览解析</button><button class="btn primary" @click="confirmImport">确认导入</button></footer></div><div v-else class="delete-body"><p>确定删除 <b>{{ rows.find((row) => row.id === editingId)?.name }}</b> 吗？删除后不能恢复。</p><small>已引用代理不能直接删除。</small><footer><button class="btn" @click="modal = null">取消</button><button class="btn danger" @click="confirmDelete">确认删除</button></footer></div></section></div>
    <transition name="toast"><div v-if="notice" class="toast">{{ notice }}</div></transition>
  </div>
</template>

<style scoped>
.proxy-page { min-height:100vh; background:var(--bg); color:var(--ink); }
.proxy-brand-mark { width:30px; height:30px; border-radius:7px; display:grid; place-items:center; background:#dce8e1; color:#355347; font-size:10px; font-weight:700; }
.proxy-page .main { min-height:calc(100vh - 29px); }
.proxy-workspace { max-width:1740px; }
.review-note { display:flex; align-items:center; gap:7px; padding:9px 12px; margin-bottom:18px; border:1px solid rgba(117,199,171,.38); border-radius:10px; color:var(--muted); background:rgba(117,199,171,.09); font-size:11px; }
.review-note b { color:var(--ink); font-weight:650; }
.review-dot { width:6px; height:6px; border-radius:50%; background:var(--accent); }
.proxy-metrics { display:grid; grid-template-columns:repeat(5,minmax(0,1fr)); gap:12px; margin:0 0 20px; }
.proxy-metrics article { min-height:104px; padding:16px; border:1px solid var(--line); border-radius:12px; background:var(--shell-panel); }
.proxy-metrics span,.proxy-metrics small { display:block; color:var(--muted); font-size:11px; }
.proxy-metrics strong { display:block; margin:10px 0 3px; color:var(--ink); font-size:25px; font-weight:560; }
.proxy-metrics .metric-time { font-size:18px; margin-top:14px; }
.ok { color:#398d70 !important; }.warn { color:#a97835 !important; }
.proxy-panel { overflow:hidden; border:1px solid var(--line); border-radius:14px; background:var(--shell-panel); }
.proxy-toolbar { display:flex; align-items:center; gap:8px; flex-wrap:wrap; padding:14px; border-bottom:1px solid var(--line); }
.proxy-toolbar .btn,.proxy-toolbar select,.selection .btn { height:34px; padding:0 10px; border:1px solid var(--line); border-radius:8px; background:var(--shell-button); color:var(--ink); font-size:11px; }
.proxy-toolbar select { min-width:96px; }
.proxy-toolbar .btn:hover,.selection .btn:hover { border-color:var(--accent); }
.proxy-search { display:flex; align-items:center; gap:6px; min-width:230px; flex:1; height:34px; padding:0 10px; border:1px solid var(--line); border-radius:8px; background:var(--shell-panel); color:var(--muted); }
.proxy-search input { width:100%; border:0; outline:0; background:transparent; color:var(--ink); font-size:11px; }
.toolbar-grow { flex:1; }
.selection { display:flex; align-items:center; gap:8px; padding:9px 14px; border-bottom:1px solid var(--line); background:rgba(117,199,171,.08); color:var(--muted); font-size:11px; }
.selection b { color:var(--ink); }
.selection .danger,.danger { color:var(--danger) !important; }
.table-wrap { overflow:auto; }
.table-wrap table { width:100%; min-width:980px; border-collapse:collapse; }
.table-wrap th { height:42px; padding:0 14px; border-bottom:1px solid var(--line); color:var(--muted); background:rgba(0,0,0,.025); font-size:10px; font-weight:600; text-align:left; white-space:nowrap; }
.table-wrap td { padding:13px 14px; border-bottom:1px solid var(--line); color:var(--ink); font-size:11px; white-space:nowrap; }
.table-wrap tr.chosen { background:rgba(117,199,171,.08); }
.table-wrap input[type=checkbox] { accent-color:var(--accent); }
.proxy-name { display:block; padding:0; border:0; background:none; color:var(--ink); font-size:12px; font-weight:650; text-align:left; }
.proxy-name:hover { color:#398d70; }
.table-wrap td small { display:block; margin-top:3px; color:var(--muted); font-size:9px; }
.protocol-tag { padding:3px 6px; border:1px solid rgba(117,199,171,.45); border-radius:5px; color:#398d70; font-size:9px; font-weight:700; }
.table-wrap code { color:var(--ink); font-size:11px; }
.status { display:inline-flex; align-items:center; gap:4px; font-size:11px; }
.status.active { color:#398d70; }.status.pending { color:#a97835; }.status.off { color:var(--muted); }.status.testing { color:#6488b0; }
.status .dot { width:6px; height:6px; background:currentColor; }
.dot.on { background:#398d70; }
.row-actions { display:flex; gap:3px; }
.icon-button { padding:4px 6px; border:0; border-radius:6px; background:transparent; color:var(--muted); font-size:10px; }
.icon-button:hover { background:var(--shell-button); color:var(--ink); }
.table-footer { display:flex; justify-content:space-between; padding:12px 14px; color:var(--muted); font-size:10px; }
.table-footer strong { color:var(--ink); }
.empty { padding:48px 20px; color:var(--muted); text-align:center; }.empty b { display:block; margin-bottom:5px; color:var(--ink); }.empty .btn { margin-top:13px; }
.proxy-footnote { margin:15px 2px 35px; color:var(--muted); font-size:10px; }
.drawer-shade,.modal-shade { position:fixed; inset:0; z-index:20; background:rgba(21,35,30,.28); backdrop-filter:blur(2px); }
.proxy-drawer { position:fixed; top:0; right:0; z-index:21; width:min(540px,100%); height:100vh; overflow:auto; border-left:1px solid var(--line); background:var(--panel); color:var(--ink); box-shadow:0 20px 70px rgba(21,35,30,.22); }
.proxy-drawer header,.proxy-modal header { display:flex; align-items:flex-start; justify-content:space-between; padding:25px 26px 18px; border-bottom:1px solid rgba(21,35,30,.12); }.proxy-drawer h2,.proxy-modal h2 { margin:5px 0 2px; font-size:20px; font-weight:580; }.proxy-drawer header small { color:#73847a; font:10px ui-monospace,monospace; }.drawer-close { border:0; background:transparent; color:var(--muted); font-size:23px; }
.drawer-body { padding:20px 26px 35px; }.drawer-body section { padding:20px 0; border-bottom:1px solid var(--line); }.drawer-body h3 { margin:0 0 12px; color:var(--muted); font-size:10px; letter-spacing:1.4px; }.drawer-body dl { display:grid; grid-template-columns:1fr 1fr; gap:15px; }.drawer-body dt { color:var(--muted); font-size:10px; }.drawer-body dd { margin:3px 0 0; color:var(--ink); font-size:12px; }.drawer-body dd small { display:block; color:var(--muted); font-size:10px; }.probe-explain { margin:0 0 14px; color:var(--muted); font-size:11px; line-height:1.65; }.drawer-body code { color:var(--accent); }.masked { color:var(--muted) !important; }.drawer-body ul { padding:0; margin:0; list-style:none; }.drawer-body li { padding:9px 0; border-bottom:1px solid var(--line); color:var(--ink); font-size:11px; }.drawer-body .btn { margin-top:15px; }.muted { color:var(--muted); font-size:11px; }
.modal-shade { display:grid; place-items:center; }.proxy-modal { width:min(620px,calc(100% - 28px)); max-height:calc(100vh - 35px); overflow:auto; border:1px solid var(--line); border-radius:14px; background:var(--panel); color:var(--ink); box-shadow:0 24px 80px rgba(21,35,30,.2); }.proxy-modal form,.import-body,.delete-body { padding:21px 26px; }.form-grid { display:grid; grid-template-columns:1fr 1fr; gap:14px; }.form-grid label,.import-body label { display:grid; gap:6px; color:var(--muted); font-size:11px; }.form-grid .full { grid-column:1/-1; }.form-grid input,.form-grid select,.form-grid textarea,.import-body textarea { padding:9px 10px; border:1px solid var(--line); border-radius:8px; outline:0; background:var(--subtle); color:var(--ink); font:inherit; }.form-grid textarea { min-height:65px; }.secret { padding:10px; border:1px solid rgba(169,120,53,.25); border-radius:8px; background:rgba(169,120,53,.08); color:#8c672e !important; font-size:11px !important; }.proxy-modal footer { display:flex; justify-content:flex-end; gap:8px; padding:14px 26px; border-top:1px solid var(--line); }.import-body textarea { width:100%; min-height:145px; resize:vertical; }.preview { padding:10px; border-radius:8px; background:rgba(117,199,171,.1); color:#398d70; font-size:11px; }.delete-body p { color:var(--muted); }.delete-body small { color:#a97835; }.toast { position:fixed; right:24px; bottom:42px; z-index:30; padding:11px 15px; border-radius:9px; background:#223d31; color:#eff8f1; font-size:11px; box-shadow:0 15px 45px rgba(0,0,0,.16); }
@media(max-width:900px){.proxy-metrics{grid-template-columns:repeat(3,1fr)}.proxy-metrics article:nth-child(4),.proxy-metrics article:nth-child(5){display:none}}@media(max-width:680px){.proxy-metrics{grid-template-columns:repeat(2,1fr);gap:8px}.proxy-toolbar{align-items:stretch}.proxy-search{min-width:100%;}.proxy-toolbar select{flex:1}.toolbar-grow{display:none}.form-grid{grid-template-columns:1fr}.form-grid .full{grid-column:auto}.proxy-drawer{width:100%}}
</style>
