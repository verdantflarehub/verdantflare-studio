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
```

Windows 下可通过 Git Bash 执行脚本，程序带 `.exe` 后缀。脚本自动完成依赖安装、前端构建和 Go 编译。

网站默认访问 `http://127.0.0.1:8000/`（自动重定向兼容旧 `/studio/`）。通过 `STATION_CORE_URL` 指定 Station，`STUDIO_PUBLIC_ORIGIN`、`STUDIO_LISTEN` 配置网站访问来源与监听地址；`STUDIO_IMAGE` 指定镜像名称。

`image` 目标通过默认 Dockerfile 构建 Gin/Vue 网站镜像。

Project 业务服务可单独构建：`go build -o <output-path> ./cmd/project`，提供
`migrate` / `serve`。配置、接口及权限事实源在中央设计仓库的
`docs/design/studio/details/07-studio-project-design.revisions.md`；该内部入口
尚未接到公网 Gateway 或 UI。

本地检查：`go test ./...`、`go vet ./internal/project ./internal/artifactclient ./migrations ./cmd/project`。
数据库测试需要 `STUDIO_TEST_DATABASE_URL` 指向可创建数据库的本机临时 PostgreSQL；
跨服务测试还需 `ARTIFACT_TEST_BINARY` 指向构建后的 Artifact 可执行文件。
缺少这些环境变量时相应集成测试明确跳过。测试使用独立临时数据库及合成内容，
不得连接生产；`STUDIO_PROJECT_CONTRACT_EVIDENCE_DIR` 可选保存合成 HTTP 契约证据。
