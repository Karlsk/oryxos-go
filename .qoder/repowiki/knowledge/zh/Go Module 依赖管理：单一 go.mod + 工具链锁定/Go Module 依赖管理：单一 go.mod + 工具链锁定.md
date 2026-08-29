---
kind: dependency_management
name: Go Module 依赖管理：单一 go.mod + 工具链锁定
category: dependency_management
scope:
    - '**'
source_files:
    - go.mod
    - go.sum
---

## 1. 使用的系统/方法

仓库采用 Go 官方模块系统（`go mod`）进行依赖管理，根目录仅存在 `go.mod` 与 `go.sum`，未使用 vendor 目录、`Gopkg.toml`、`dep`、`glide` 等替代方案。模块名为 `github.com/Karlsk/oryxos-go`，声明的 Go 版本为 `1.26.0`。

## 2. 关键文件

- `go.mod`：声明直接依赖与间接依赖，并集中定义开发期工具链。
- `go.sum`：锁定所有依赖的精确校验和（由 `go mod tidy` 生成）。

## 3. 架构与约定

### 3.1 依赖分层
`go.mod` 将依赖分为两类：
- **直接依赖**（`require` 块）：仅 4 个——`gin-gonic/gin`（Web 框架）、`spf13/cobra`（CLI）、`golang.org/x/mod`（模块解析）、`gopkg.in/yaml.v3`（配置加载）。这些是 OryxOS 运行时实际 import 的库。
- **间接依赖**（`// indirect` 标记）：由 Gin、Cobra、yaml 等引入的大量第三方库，包括 OpenAI/Anthropic SDK、Google Cloud SDK、MongoDB driver、OpenTelemetry 等，表明项目当前处于“骨架阶段”，大量 SDK 已作为间接依赖被拉入但尚未直接使用。

### 3.2 工具链锁定
通过 `tool` 指令在 `go.mod` 中锁定三个开发期工具及其版本：
- `github.com/securego/gosec/v2/cmd/gosec`：静态安全扫描
- `golang.org/x/tools/cmd/goimports`：import 自动格式化
- `golang.org/x/vuln/cmd/govulncheck`：漏洞检查

这确保团队 CI 与本地构建使用一致版本的 lint/security 工具，避免“在我机器上能过”的问题。

### 3.3 无私有仓库 / 无 vendor
仓库未包含 `vendor/` 目录，也未见 `.env`、`GOPRIVATE`、`GOPROXY` 或 `go.work` 等私有代理/工作区配置。依赖全部从公共 Go Proxy（默认 `proxy.golang.org`）拉取。

### 3.4 发布约束影响依赖选择
作者指南明确要求：
- SQLite 必须通过 `github.com/glebarez/sqlite` 使用纯 Go 的 `modernc.org/sqlite`，禁止 CGO 依赖 `mattn/go-sqlite3`。
- 发布构建必须 `CGO_ENABLED=0 go build ./cmd/oryxos`。

这两条规则对依赖选型产生强约束：任何后续引入的数据库/存储依赖都必须满足纯 Go 实现，以支持 CGO 禁用的静态二进制发布。

## 4. 约定与约束

| 约定 | 说明 | 来源 |
|---|---|---|
| 单一模块 | 整个仓库只有一个 `go.mod`，按 `cmd → internal/*` 单向依赖组织 | `go.mod` 模块名 + 目录结构 |
| 依赖最小化 | 仅显式声明 4 个直接依赖，其余均为间接依赖 | `go.mod` require 块 |
| 工具链锁定 | 通过 `tool` 指令固定 gosec、goimports、govulncheck 版本 | `go.mod` tool 块 |
| 禁用 CGO 发布 | 构建时必须 `CGO_ENABLED=0` | 作者指南 |
| SQLite 实现限定 | 必须用 `modernc.org/sqlite`，禁止 `mattn/go-sqlite3` | 作者指南 |
| 无 vendor 策略 | 不提交 vendor 目录，依赖由 `go.sum` 锁定 | 仓库现状 |
| 无私有代理配置 | 未发现 GOPROXY/GOPRIVATE 设置 | 仓库现状 |

## 5. 观察到的状态

当前仓库处于 Agent OS 骨架阶段：核心业务代码较少，但 `go.sum` 中已出现大量 AI SDK（OpenAI、Anthropic、Google GenAI）与基础设施依赖，说明依赖清单可能超前于实际代码引用。建议后续通过 `go mod tidy` 定期清理未被直接引用的间接依赖，保持依赖树精简。