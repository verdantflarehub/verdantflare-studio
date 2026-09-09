# VerdantFlare Studio (`verdantflare-studio`)

VerdantFlare Studio 后端业务中枢与跨模态聚合服务。

作为面向创作者与 AI 智能体（Codex / Antigravity Agent）的统一服务入口，向下连接并聚合调度三大专业生产套件（`verdantflare-app-image`、`verdantflare-app-music`、`verdantflare-app-vedio`），实现多模态项目化生产、跨模态任务中心与统一持久化存储治理。

---

## 核心职责

1. **统一 MCP 服务门面 (`/mcp`)**：对外暴露单一 MCP 端点，向调用方呈现包含 `image.*`、`music.*`、`video.*`、`studio.*` 的全量工具命名空间，内部智能路由代理至下游微服务；
2. **统一 Studio Dashboard (`/dashboard`)**：面向创作者提供 Web 项目全景看板，按 `<User>/<Project>` 聚合呈现音频、图像与运镜视频的全生命周期进度，并提供直达各 App 技术看板的下钻链接；
3. **统一授权中间件**：外部调用统一使用单一 `STUDIO_BEARER_TOKEN`，Studio 验签后通过内网 ClusterIP 向下游 App 通信并透传 `X-User-Id` 与 `X-Project-Id`；
4. **统一任务元数据 `<User>/<Project>/<Taskid>`**：维护全局跨模态任务状态快照，规范落盘至共享存储 `/data/projects/{User}/{Project}/`。

---

## 环境变量配置

| 变量名 | 默认值 | 说明 |
| :--- | :--- | :--- |
| `STUDIO_ARTIFACT_ROOT` | `/data/projects` | 共享持久化存储根路径 |
| `IMAGE_MCP_URL` | `http://image-mcp-server:8000` | Downstream Image MCP 服务地址 |
| `MUSIC_MCP_URL` | `http://music-mcp-server:8000` | Downstream Music MCP 服务地址 |
| `VIDEO_MCP_URL` | `http://vedio-mcp-server:8000` | Downstream Video MCP 服务地址 |
| `STUDIO_BEARER_TOKEN` | *(可选)* | 统一访问口令，配置后启用 Bearer 鉴权保护 |
| `STUDIO_MCP_ALLOWED_HOSTS` | `127.0.0.1:*,localhost:*,[::1]:*` | MCP DNS Rebinding 防护允许的 Host 列表 |
| `STUDIO_MCP_ALLOWED_ORIGINS` | `http://127.0.0.1:*,http://localhost:*,http://[::1]:*` | MCP 允许的 CORS Origin 列表 |

---

## 核心端点

- **`GET /health`**：服务健康检查，返回当前版本及下游三大 App 的网络连接目标；
- **`POST /mcp` 与 `GET /mcp/sse`**：MCP 统一协议端点（Streamable HTTP 与 SSE 模式）；
- **`GET /dashboard`**：Studio 多模态项目全景看板；
- **`GET /api/projects`**：项目列表 REST API；
- **`GET /api/tasks`**：跨模态全局任务列表与统计 REST API。

---

## 本地开发与测试

```bash
# 安装依赖
pip install -r requirements.txt

# 运行单元测试
python3 -m unittest discover -s tests -p 'test_*.py'

# 启动本地服务
uvicorn src.server:app --host 0.0.0.0 --port 8000 --reload
```

---

## Kubernetes 部署

部署清单由 `verdantflare-design/deploys/` 统一管辖：
`deploys/k8s.cn-chengdu.bc-cloud.com/verdantflare-studio/`