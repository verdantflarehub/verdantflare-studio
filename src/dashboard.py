from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from starlette.requests import Request
from starlette.responses import HTMLResponse, JSONResponse, Response

from .tasks import TaskStore

DASHBOARD_HTML = """<!DOCTYPE html>
<html lang="zh-CN" class="dark">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>VerdantFlare Studio - 多模态项目全景工作台</title>
  <script src="https://cdn.tailwindcss.com"></script>
  <script>
    tailwind.config = {
      darkMode: 'class',
      theme: {
        extend: {
          colors: {
            brand: { 50: '#ecfdf5', 500: '#10b981', 600: '#059669', 900: '#064e3b' }
          }
        }
      }
    }
  </script>
</head>
<body class="bg-slate-950 text-slate-100 min-h-screen font-sans antialiased">
  <div class="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-6">
    <!-- Header -->
    <header class="flex flex-col md:flex-row md:items-center md:justify-between pb-6 border-b border-slate-800 gap-4">
      <div>
        <div class="flex items-center gap-3">
          <span class="inline-flex items-center justify-center p-2.5 bg-emerald-500/10 text-emerald-400 rounded-xl border border-emerald-500/20 shadow-inner">
            <svg class="w-6 h-6" fill="none" stroke="currentColor" viewBox="0 0 24 24">
              <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M19 11H5m14 0a2 2 0 012 2v6a2 2 0 01-2 2H5a2 2 0 01-2-2v-6a2 2 0 012-2m14 0V9a2 2 0 00-2-2M5 11V9a2 2 0 012-2m0 0V5a2 2 0 012-2h6a2 2 0 012 2v2M7 7h10"></path>
            </svg>
          </span>
          <div>
            <h1 class="text-xl font-bold tracking-tight text-white flex items-center gap-2">
              VerdantFlare Studio
              <span class="text-xs px-2 py-0.5 rounded bg-emerald-500/20 text-emerald-300 font-medium border border-emerald-500/30">中枢控制台</span>
            </h1>
            <p class="text-xs text-slate-400 mt-0.5">跨模态生产编排：Music 词曲 · Image 视觉 · Video 运镜全景总览</p>
          </div>
        </div>
      </div>

      <!-- App Downstream Drill-down Links -->
      <div class="flex items-center gap-2 flex-wrap">
        <span class="text-xs text-slate-400 mr-1">底层技术看板:</span>
        <a href="/image/dashboard" target="_blank" class="px-2.5 py-1.5 text-xs bg-slate-900 hover:bg-slate-800 text-slate-200 rounded-lg border border-slate-700 transition flex items-center gap-1">
          <span>🎨</span> Image 看板
        </a>
        <a href="/music/dashboard" target="_blank" class="px-2.5 py-1.5 text-xs bg-slate-900 hover:bg-slate-800 text-slate-200 rounded-lg border border-slate-700 transition flex items-center gap-1">
          <span>🎵</span> Music 探针
        </a>
        <a href="/video/dashboard" target="_blank" class="px-2.5 py-1.5 text-xs bg-slate-900 hover:bg-slate-800 text-slate-200 rounded-lg border border-slate-700 transition flex items-center gap-1">
          <span>🎬</span> Video H3
        </a>
        <button onclick="fetchTasks()" class="px-3 py-1.5 text-xs bg-emerald-600 hover:bg-emerald-500 text-white rounded-lg font-medium transition flex items-center gap-1 ml-2">
          刷新数据
        </button>
      </div>
    </header>

    <!-- Multi-Modal Domain Pipeline Stats Cards -->
    <div class="grid grid-cols-1 md:grid-cols-4 gap-4 my-6">
      <div class="bg-slate-900/60 border border-slate-800 rounded-xl p-4">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-slate-400">🎵 音频制作 (Music)</span>
          <span class="text-xs px-1.5 py-0.5 rounded bg-blue-500/20 text-blue-300 border border-blue-500/30">2x 4090</span>
        </div>
        <div class="text-2xl font-bold mt-2 text-blue-400" id="statMusic">0</div>
        <div class="text-[11px] text-slate-500 mt-1">词曲生成 / 分轨 / 音色 / 对齐 / 混音</div>
      </div>
      <div class="bg-slate-900/60 border border-slate-800 rounded-xl p-4">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-slate-400">🎨 视觉资产 (Image)</span>
          <span class="text-xs px-1.5 py-0.5 rounded bg-purple-500/20 text-purple-300 border border-purple-500/30">双核中继</span>
        </div>
        <div class="text-2xl font-bold mt-2 text-purple-400" id="statImage">0</div>
        <div class="text-[11px] text-slate-500 mt-1">角色四视图 / 人台三视图 / 分镜网格</div>
      </div>
      <div class="bg-slate-900/60 border border-slate-800 rounded-xl p-4">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-slate-400">🎬 运镜视频 (Video)</span>
          <span class="text-xs px-1.5 py-0.5 rounded bg-amber-500/20 text-amber-300 border border-amber-500/30">MiniMax H3</span>
        </div>
        <div class="text-2xl font-bold mt-2 text-amber-400" id="statVideo">0</div>
        <div class="text-[11px] text-slate-500 mt-1">动态长镜头 / 多格运镜连贯渲染</div>
      </div>
      <div class="bg-slate-900/60 border border-slate-800 rounded-xl p-4">
        <div class="flex items-center justify-between">
          <span class="text-xs font-medium text-slate-400">📁 活跃创意工程</span>
          <span class="text-xs px-1.5 py-0.5 rounded bg-emerald-500/20 text-emerald-300 border border-emerald-500/30">/data/projects</span>
        </div>
        <div class="text-2xl font-bold mt-2 text-emerald-400" id="statProjects">0</div>
        <div class="text-[11px] text-slate-500 mt-1">&lt;User&gt;/&lt;Project&gt; 隔离落盘</div>
      </div>
    </div>

    <!-- Filter & Project Scope Bar -->
    <div class="bg-slate-900/40 border border-slate-800/80 rounded-xl p-4 mb-6 flex flex-wrap items-center justify-between gap-3 text-xs">
      <div class="flex flex-wrap items-center gap-3">
        <div class="flex items-center gap-2">
          <span class="text-slate-400 font-medium">当前项目:</span>
          <input type="text" id="filterProject" placeholder="如 mengsk/jyby-mv" class="bg-slate-950 border border-slate-700 text-slate-200 rounded-lg px-2.5 py-1 w-44 focus:outline-none focus:border-emerald-500">
        </div>
        <div class="flex items-center gap-2">
          <span class="text-slate-400 font-medium">模态域:</span>
          <select id="filterDomain" onchange="fetchTasks()" class="bg-slate-950 border border-slate-700 text-slate-200 rounded-lg px-2.5 py-1 focus:outline-none focus:border-emerald-500">
            <option value="">全部模态 (All)</option>
            <option value="music">🎵 音频 (Music)</option>
            <option value="image">🎨 图像 (Image)</option>
            <option value="video">🎬 视频 (Video)</option>
          </select>
        </div>
        <div class="flex items-center gap-2">
          <span class="text-slate-400 font-medium">状态:</span>
          <select id="filterStatus" onchange="fetchTasks()" class="bg-slate-950 border border-slate-700 text-slate-200 rounded-lg px-2.5 py-1 focus:outline-none focus:border-emerald-500">
            <option value="">全部状态</option>
            <option value="queued">排队中 (Queued)</option>
            <option value="running">执行中 (Running)</option>
            <option value="completed">已完成 (Completed)</option>
            <option value="failed">已失败 (Failed)</option>
          </select>
        </div>
        <button onclick="fetchTasks()" class="px-3 py-1 bg-slate-800 hover:bg-slate-700 text-slate-200 rounded-lg border border-slate-700 transition">
          筛选
        </button>
      </div>
      <div class="text-xs text-slate-500" id="totalCountText">共 0 个任务</div>
    </div>

    <!-- Task Table -->
    <div class="bg-slate-900/40 border border-slate-800/80 rounded-xl overflow-hidden shadow-sm">
      <div class="overflow-x-auto">
        <table class="w-full text-left text-xs">
          <thead class="bg-slate-900/80 text-slate-400 uppercase tracking-wider text-[11px] border-b border-slate-800">
            <tr>
              <th class="py-3 px-4">Task ID</th>
              <th class="py-3 px-4">创作者 / 项目</th>
              <th class="py-3 px-4">模态</th>
              <th class="py-3 px-4">动作</th>
              <th class="py-3 px-4">状态</th>
              <th class="py-3 px-4">耗时</th>
              <th class="py-3 px-4">创建时间</th>
            </tr>
          </thead>
          <tbody id="taskTableBody" class="divide-y divide-slate-800/60 text-slate-300">
            <tr>
              <td colspan="7" class="py-8 text-center text-slate-500">加载中...</td>
            </tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>

  <script>
    async function fetchTasks() {
      const proj = document.getElementById('filterProject').value.trim();
      const domain = document.getElementById('filterDomain').value;
      const status = document.getElementById('filterStatus').value;

      let url = '/api/tasks?limit=50';
      if (proj) {
        if (proj.includes('/')) {
          const parts = proj.split('/');
          url += `&user_id=${encodeURIComponent(parts[0])}&project_id=${encodeURIComponent(parts[1])}`;
        } else {
          url += `&project_id=${encodeURIComponent(proj)}`;
        }
      }
      if (domain) url += `&domain=${encodeURIComponent(domain)}`;
      if (status) url += `&status=${encodeURIComponent(status)}`;

      try {
        const res = await fetch(url);
        const data = await res.json();
        renderTable(data.tasks || []);
        document.getElementById('totalCountText').innerText = `共 ${data.total || 0} 个跨模态任务`;
        updateStats(data.tasks || []);
      } catch (err) {
        console.error(err);
      }
    }

    function updateStats(tasks) {
      let mus = 0, img = 0, vid = 0;
      const projs = new Set();
      for (const t of tasks) {
        if (t.domain === 'music') mus++;
        if (t.domain === 'image') img++;
        if (t.domain === 'video') vid++;
        if (t.project_id) projs.add(`${t.user_id}/${t.project_id}`);
      }
      document.getElementById('statMusic').innerText = mus;
      document.getElementById('statImage').innerText = img;
      document.getElementById('statVideo').innerText = vid;
      document.getElementById('statProjects').innerText = projs.size || '-';
    }

    function renderTable(tasks) {
      const tbody = document.getElementById('taskTableBody');
      if (!tasks.length) {
        tbody.innerHTML = '<tr><td colspan="7" class="py-8 text-center text-slate-500">暂无任务记录</td></tr>';
        return;
      }
      tbody.innerHTML = tasks.map(t => {
        const domainColor = t.domain === 'music' ? 'bg-blue-500/10 text-blue-400 border-blue-500/20' :
                            t.domain === 'image' ? 'bg-purple-500/10 text-purple-400 border-purple-500/20' :
                            t.domain === 'video' ? 'bg-amber-500/10 text-amber-400 border-amber-500/20' : 'bg-slate-800 text-slate-300';
        const statusColor = t.status === 'completed' ? 'text-emerald-400' :
                            t.status === 'running' ? 'text-sky-400 animate-pulse' :
                            t.status === 'failed' ? 'text-rose-400' : 'text-amber-400';
        return `
          <tr class="hover:bg-slate-900/50 transition">
            <td class="py-3 px-4 font-mono text-[11px] text-slate-300">${t.task_id}</td>
            <td class="py-3 px-4 font-medium text-slate-200">${t.user_id}/${t.project_id}</td>
            <td class="py-3 px-4"><span class="px-2 py-0.5 rounded border text-[11px] font-medium ${domainColor}">${t.domain}</span></td>
            <td class="py-3 px-4 font-mono text-[11px] text-slate-300">${t.action}</td>
            <td class="py-3 px-4 font-medium ${statusColor}">${t.status}</td>
            <td class="py-3 px-4 text-slate-400">${t.duration_seconds.toFixed(1)}s</td>
            <td class="py-3 px-4 text-slate-500 text-[11px]">${t.created_at ? t.created_at.replace('T', ' ').slice(0, 19) : '-'}</td>
          </tr>
        `;
      }).join('');
    }

    fetchTasks();
    setInterval(fetchTasks, 5000);
  </script>
</body>
</html>
"""


def _get_task_store(request: Request) -> TaskStore:
    store = getattr(request.app.state, "task_store", None)
    if store is None:
        store = TaskStore.from_environment()
    return store


STATIC_HTML_PATH = Path(__file__).resolve().parent / "static" / "index.html"


async def dashboard_page(request: Request) -> Response:
    if STATIC_HTML_PATH.exists():
        return HTMLResponse(STATIC_HTML_PATH.read_text(encoding="utf-8"))
    return HTMLResponse(DASHBOARD_HTML)


async def api_list_tasks(request: Request) -> JSONResponse:
    task_store = _get_task_store(request)
    user_id = request.query_params.get("user_id")
    project_id = request.query_params.get("project_id")
    asset_id = request.query_params.get("asset_id")
    job_id = request.query_params.get("job_id")
    domain = request.query_params.get("domain")
    status = request.query_params.get("status")
    limit = int(request.query_params.get("limit", 50))
    offset = int(request.query_params.get("offset", 0))

    tasks, total = task_store.list_tasks(
        user_id=user_id,
        project_id=project_id,
        asset_id=asset_id,
        job_id=job_id,
        domain=domain,
        status=status,
        limit=limit,
        offset=offset,
    )
    return JSONResponse(
        {
            "tasks": [t.to_dict() for t in tasks],
            "total": total,
            "limit": limit,
            "offset": offset,
        }
    )


async def api_list_projects(request: Request) -> JSONResponse:
    task_store = _get_task_store(request)
    user_id = request.query_params.get("user_id")
    projects = task_store.list_projects(user_id=user_id)
    return JSONResponse({"projects": [p.to_dict() for p in projects]})

