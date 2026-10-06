# VerdantFlare Studio

基于 Wails v3 的桌面应用，采用 Vue 3 + TypeScript + Go；早期通过 Gin 网站入口验证功能。前端依赖统一由 pnpm 管理。

## 构建与运行

需安装 Go、Node.js 和 Corepack（或项目指定版本的 pnpm）；桌面构建另需 Wails v3 平台依赖，镜像构建需 Docker。

```bash
# 网站（默认）：构建后启动
bash scripts/build.sh
./build/studio-web

# 桌面：构建后启动
bash scripts/build.sh desktop
./build/studio-desktop

# 网站镜像
bash scripts/build.sh image

# Project/World 内部服务（不依赖前端构建）
bash scripts/build.sh project
./build/studio-project migrate
./build/studio-project serve

# Project 工作副本客户端（无需前端、etcd 或内部服务凭据）
bash scripts/build.sh workspace
./build/studio-workspace --help
```

Windows 下可通过 Git Bash 执行脚本，程序带 `.exe` 后缀。脚本自动完成依赖安装、前端构建和 Go 编译。

网站默认访问 `http://127.0.0.1:8000/`（自动重定向兼容旧 `/studio/`）。通过 `STATION_CORE_URL` 指定 Station，`STUDIO_PUBLIC_ORIGIN`、`STUDIO_LISTEN` 配置网站访问来源与监听地址；`STUDIO_IMAGE` 指定镜像名称。

`image` 目标通过默认 Dockerfile 构建 Studio 镜像，包含 `/studio-web`、
`/studio-project` 和 `/studio-workspace`。默认入口仍为 `/studio-web`；Project/World 作为独立进程部署，
使用 `--entrypoint /studio-project <image> migrate` 或 `serve`，不会随 Web 自动启动或迁移。

Project 业务服务提供 `migrate` / `serve`。配置、接口及权限事实源在中央设计仓库的
`docs/design/studio/details/07-studio-project-design.revisions.md`；该内部入口
通过现有 Gateway 服务发现接入 Project/World UI；实际可用性取决于数据库迁移、
Artifact、内部 TLS 和受控凭据的部署，构建镜像不代表服务已上线。

本地检查：`go test ./...`、`go vet ./internal/project ./internal/artifactclient ./migrations ./cmd/project`。
数据库测试需要 `STUDIO_TEST_DATABASE_URL` 指向可创建数据库的本机临时 PostgreSQL；
跨服务测试还需 `ARTIFACT_TEST_BINARY` 指向构建后的 Artifact 可执行文件。
缺少这些环境变量时相应集成测试明确跳过。测试使用独立临时数据库及合成内容，
不得连接生产；`STUDIO_PROJECT_CONTRACT_EVIDENCE_DIR` 可选保存合成 HTTP 契约证据。

release CI 同时交付 Linux/Windows amd64 `studio-workspace` 下载产物。客户端读取
`STUDIO_MCP_URL` 和 `STUDIO_MCP_BEARER_TOKEN`（已绑定真实 Core 用户的会话）；
不接受旧共享 token 作为管理权限。`call --input request.json` 接收
`{"name":"project.list","arguments":{}}` 等中央 MCP 工具请求；工作副本命令及
JSON 输入形状以 `--help` 和中央 `07-studio-project-design.local-workspace.md` 为准。
同一变更请求保留 commit_id/write_id；工作副本保存由已有 journal 负责恢复。
