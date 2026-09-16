<script setup lang="ts">
import { onMounted, onBeforeUnmount } from "vue"
import { mountMarket } from "./market"
let dispose: (() => void) | undefined
onMounted(() => { dispose = mountMarket() })
onBeforeUnmount(() => dispose?.())
</script>
<template>

  <aside class="side"
         aria-label="Studio 主菜单">
    <a class="brand"
       href="#" data-placeholder="true"><img id="vfHeaderLogo"
           width="30"
           height="30"
           alt=""><span>青焰 · Verdantflare<small>STUDIO</small></span></a>
    <div class="side-tools"><button class="btn primary new-task"
              id="newTask">＋ 新建创作任务</button><button class="side-collapse"
              id="sideCollapse"
              aria-label="收起侧栏"
              aria-expanded="true">‹</button></div>
    <div class="nav-label">创作空间</div>
    <nav class="main-nav">
      <a class="nav-link"
         href="#" data-placeholder="true"
         title="工作台"><span class="nav-icon"><svg viewBox="0 0 24 24"
               fill="none"
               stroke="currentColor"
               stroke-width="1.5"
               stroke-linecap="round"
               stroke-linejoin="round"
               aria-hidden="true">
            <path d="M4 11.5 12 5l8 6.5V20H4z" />
            <path d="M9 20v-6h6v6" />
          </svg></span><span class="label">工作台</span></a>
      <button class="nav-link selected"
              data-nav="market"
              title="应用市场"><span class="nav-icon"><svg viewBox="0 0 24 24"
               fill="none"
               stroke="currentColor"
               stroke-width="1.5"
               stroke-linecap="round"
               stroke-linejoin="round"
               aria-hidden="true">
            <rect x="4"
                  y="5"
                  width="16"
                  height="15"
                  rx="3" />
            <path d="M8 5V3m8 2V3M4 10h16" />
          </svg></span><span class="label">应用市场</span><span class="nav-chevron">⌄</span></button>
      <div class="market-subnav"><button class="group-nav"
                data-nav="installed">已安装应用<span class="count"
                id="navInstalledCount">0</span></button>
        <div id="groupNav"></div>
      </div>
      <button class="nav-link"
              data-nav="models"
              title="模型市场"><span class="nav-icon"><svg viewBox="0 0 24 24"
               fill="none"
               stroke="currentColor"
               stroke-width="1.5"
               stroke-linecap="round"
               stroke-linejoin="round"
               aria-hidden="true">
            <path d="M12 3 4.5 7v10L12 21l7.5-4V7z" />
            <path d="m4.5 7 7.5 4 7.5-4M12 11v10" />
          </svg></span><span class="label">模型市场</span></button>
      <a class="nav-link"
         href="#" data-placeholder="true"
         title="项目"><span class="nav-icon"><svg viewBox="0 0 24 24"
               fill="none"
               stroke="currentColor"
               stroke-width="1.5"
               stroke-linecap="round"
               stroke-linejoin="round"
               aria-hidden="true">
            <path d="M3 7h7l2 2h9v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z" />
            <path d="M3 7V5a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v2" />
          </svg></span><span class="label">项目</span></a>
      <a class="nav-link"
         href="#" data-placeholder="true"
         title="数字世界"><span class="nav-icon"><svg viewBox="0 0 24 24"
               fill="none"
               stroke="currentColor"
               stroke-width="1.5"
               stroke-linecap="round"
               stroke-linejoin="round"
               aria-hidden="true">
            <circle cx="12"
                    cy="12"
                    r="8" />
            <path d="M4.5 10h15M7 17h10M12 4c2.4 2.3 3.5 5 3.5 8S14.4 17.7 12 20c-2.4-2.3-3.5-5-3.5-8S9.6 6.3 12 4z" />
          </svg></span><span class="label">数字世界</span></a>
    </nav>
    <div class="recent-projects"><div class="nav-label">项目</div><p>项目管理开发中</p></div>
    <div class="side-bottom"><a class="nav-link"
         href="#" data-placeholder="true"
         title="帮助与设计说明"><span class="nav-icon"><svg viewBox="0 0 24 24"
               fill="none"
               stroke="currentColor"
               stroke-width="1.5"
               stroke-linecap="round"
               stroke-linejoin="round"
               aria-hidden="true">
            <circle cx="12"
                    cy="12"
                    r="9" />
            <path d="M9.8 9a2.4 2.4 0 1 1 3.2 2.3c-.7.3-1 1-1 1.7v.3M12 17h.01" />
          </svg></span><span class="label">帮助与设计说明</span></a>
      <div class="station"><strong><span class="dot"></span><span class="station-name">成都 Station</span></strong>
        <p>等待连接 Station</p>
      </div>
    </div>
  </aside>
  <main class="main">
    <div class="workspace">
      <header class="heading">
        <div><span class="eyebrow">FILM AI OS · APP MARKET</span>
          <h1 id="pageTitle">应用市场</h1>
          <p id="pageSubtitle">为 Station 安装创作应用，管理模型与运行状态。</p>
        </div>
        <div class="heading-actions"><button class="btn ghost"
                  id="theme"
                  aria-label="切换浅色或深色主题">深色主题</button><button class="btn"
                  id="reset">刷新状态</button><button class="btn primary"
                  data-tab="installed">查看已安装</button></div>
      </header>
      <div class="workspace-strip">
        <div class="workspace-avatar">VF</div>
        <div><strong>青焰创作工作区</strong>
          <p>Image · Music · Video</p>
        </div>
        <div class="strip-right"><span class="pill">管理员视角</span><span class="demo">正在连接 Station</span></div>
      </div>
      <div class="market-columns">
        <div class="market-primary">
          <section class="hero">
            <div class="hero-copy"><span class="eyebrow">VIDEO COLLECTION</span>
              <h2>从应用入口，到本地视频创作。</h2>
              <p>查看真实应用与运行状态，安装管理即将接入。</p>
            </div>
            <div class="hero-actions"><button class="btn primary"
                      data-detail="video-mcp-server">Video MCP ↗</button><button class="btn"
                      data-detail="video-minimax-h3-vdn">MiniMax H3 VDN</button></div>
          </section>
          <section class="catalog-section">
            <div class="tabs-row">
              <div class="tabs"><button class="tab active"
                        data-tab="all">全部应用 <span id="allCount"></span></button><button class="tab"
                        data-tab="installed">已安装 <span id="installedCount">0</span></button><button class="tab"
                        data-tab="active">进行中 <span id="activeCount">0</span></button></div><label class="search"><span
                      aria-hidden="true">⌕</span><input id="search"
                       placeholder="搜索应用、模型或能力"
                       aria-label="搜索应用、模型或能力"
                       type="search"></label>
            </div>
            <div class="filters"><button class="chip active"
                      data-group="all">全部分组</button><button class="chip"
                      data-group="image">Image</button><button class="chip"
                      data-group="music">Music</button><button class="chip"
                      data-group="video">Video</button><span class="result-count"
                    id="resultCount"></span></div>
            <div id="catalog"></div>
          </section>
        </div>
        <aside class="market-rail"
               aria-label="Station 与操作概览">
          <section class="rail-panel"><span class="eyebrow">OPERATIONS</span>
            <div class="rail-heading">
              <h2>本地 Station</h2><span class="pill">待连接</span>
            </div>
            <p>应用安装与运行概览</p>
            <div class="rail-metrics">
              <div><strong id="railInstalled">0</strong><small>已安装应用</small></div>
              <div><strong id="railRunning">0</strong><small>运行中</small></div>
            </div><button class="btn rail-button"
                    data-tab="installed">管理已安装应用 →</button>
          </section>
          <section class="rail-panel"><span class="eyebrow">STATION MCP</span>
            <h2>创作能力入口</h2>
            <p>应用启动后，对应入口点亮。</p>
            <div id="mcpNav"></div>
          </section>
          <section class="rail-panel">
            <div class="rail-heading"><span class="eyebrow">INSTALL ACTIVITY</span><button class="text-button"
                      data-tab="active">查看全部</button></div>
            <h2>安装与操作</h2>
            <div id="railActivity"></div>
          </section>
          <section class="rail-panel resource-panel"><span class="eyebrow">STATION RESOURCES</span>
            <h2>设备与存储</h2>
            <div class="resource-row"><span>GPU</span><span>未连接</span></div>
            <div class="resource-row"><span>可用空间</span><span>—</span></div>
            <div class="resource-row"><span>模型缓存</span><span id="railModels">状态未知</span></div>
            <p>资源统计尚未接入。</p>
          </section>
        </aside>
      </div>
      <footer class="footnote"><span>应用与版本来自当前部署清单 · 实时运行状态来自 Station</span><a
           href="#" data-placeholder="true">Market 设计 ↗</a></footer>
    </div>
  </main>
  <footer class="statusbar"><span><i class="dot"></i> 目标 Station · 未连接</span><span id="statusSummary">已安装 0 · 运行中
      0</span><span class="right">Studio 应用市场 · 在线预览</span></footer>
  <dialog id="launchDialog">
    <div class="launcher-head">
      <div><span class="eyebrow">CREATE WITH APPS</span>
        <h2>选择创作应用</h2>
      </div><button class="btn ghost"
              id="closeLaunch"
              aria-label="关闭创作应用选择">✕</button>
    </div>
    <p>选择应用打开现有工作台原型；未安装的应用可先查看并安装。</p>
    <div id="launchApps"></div>
  </dialog>
  <div class="overlay"
       id="overlay">
    <section class="drawer"
             role="dialog"
             aria-modal="true"
             aria-labelledby="detailTitle"
             tabindex="-1">
      <div class="drawer-head"><span class="eyebrow">APPLICATION</span><button class="btn ghost"
                id="closeDrawer"
                aria-label="关闭应用详情">✕</button></div>
      <div id="detailHead"></div>
      <div id="actions"
           class="action-row"></div>
      <div id="progress"></div>
      <div class="detail-tabs"
           role="tablist"><button role="tab"
                data-detail-tab="overview"
                class="active">概览</button><button role="tab"
                data-detail-tab="models">模型</button><button role="tab"
                data-detail-tab="capabilities">能力与渠道</button><button role="tab"
                data-detail-tab="logs">操作记录</button></div>
      <div id="detailContent"
           class="detail-content"></div>
    </section>
  </div>
  <dialog id="confirm">
    <h2 id="confirmTitle"></h2>
    <p id="confirmText"></p>
    <div class="buttons"><button class="btn"
              id="confirmCancel">取消</button><button class="btn primary"
              id="confirmOK">确定</button></div>
  </dialog>
  <div id="toast"
       class="toast"
       role="status"
       aria-live="polite"></div>

<dialog id="loginDialog" aria-labelledby="loginTitle"><form id="loginForm"><h2 id="loginTitle">登录 Station</h2><p>连接你的青焰创作工作区</p><label>用户名<input name="username" autocomplete="username" required value="admin"></label><label>密码<input name="password" type="password" autocomplete="current-password" required></label><p id="loginError" role="alert"></p><button class="btn primary" type="submit">登录</button></form></dialog>



</template>
