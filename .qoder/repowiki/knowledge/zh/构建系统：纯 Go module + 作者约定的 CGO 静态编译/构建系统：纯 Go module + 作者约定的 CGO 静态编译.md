---
kind: build_system
name: 构建系统：纯 Go module + 作者约定的 CGO 静态编译
category: build_system
scope:
    - '**'
source_files:
    - go.mod
    - go.sum
    - cmd/oryxos/main.go
---

## 1. 使用的系统与工具

本仓库是一个**单一 Go module**（`github.com/Karlsk/oryxos-go`，Go 1.26.0），没有 Makefile、Dockerfile、CI 流水线或 shell 构建脚本。唯一的“构建”入口是 `go build ./cmd/oryxos`，产物为单个可执行文件。

依赖管理完全通过 `go.mod` / `go.sum` 维护，使用 Go Modules 模式；代码质量与漏洞扫描通过 `tool` 段声明的 go:generate 风格工具链完成：
- `github.com/securego/gosec/v2/cmd/gosec` — 安全扫描
- `golang.org/x/tools/cmd/goimports` — 导入排序
- `golang.org/x/vuln/cmd/govulncheck` — 依赖漏洞检查

这些工具以 `go tool` 形式调用，不引入运行时依赖。

## 2. 关键文件

- `go.mod`：声明模块名、Go 版本、直接依赖（gin、cobra、yaml.v3、x/mod）以及 `tool` 段中的开发期工具。
- `go.sum`：锁定所有间接依赖的校验和。
- `cmd/oryxos/main.go`：唯一二进制入口，由 `go build ./cmd/oryxos` 产出。

## 3. 架构与约定

### 构建产物
- 单一可执行文件：`./cmd/oryxos` 即 OryxOS CLI 程序，同时承载 init/status/serve/chat/profile 等子命令。
- 无多目标构建：仓库中不存在针对不同平台/环境的多个构建入口。

### 发布构建约束（作者强制规则）
根据作者指南：**发布构建必须通过 `CGO_ENABLED=0 go build ./cmd/oryxos`**。该规则确保最终二进制不包含 CGO 链接（例如 SQLite 必须使用 pure-Go 实现 `modernc.org/sqlite`，禁止 `mattn/go-sqlite3`），从而生成可在任意 Linux 容器内直接运行的静态二进制。

### 依赖方向约束（影响构建边界）
- 核心模块边界明确：`runtime` 不得导入 Gin、Cobra、GORM 或 Eino-ext；`web` 不得直接访问 Provider connector 或 Store 实现。
- 依赖方向单向：`cmd → app → handler/channel/scheduler → service/runtime → Eino core interfaces + domain ports`。
- 这些约束在编译期由包导入关系强制执行，任何违规都会导致 `go build` 失败。

### 配置与扩展点占位
- `internal/{memory,provider,skill,channel/cli,runtime,sandbox,scheduler,session,profile,store,tool}` 等包目前多为空包（仅含 `doc.go`），作为后续实现的编译期预留位置，保证当前可编译通过。
- Tool 体系按三档划分：零代码（SKILL.md + MCP）、轻代码（自建 MCP server）、重代码（Go Tool 接口实现），对应 `tool/builtin`、`tool/mcp` 等子包骨架。

## 4. 约定与约束总结

| 项目 | 约定/规则 | 来源 | 说明 |
|---|---|---|---|
| 构建命令 | `CGO_ENABLED=0 go build ./cmd/oryxos` | 作者指南 | 发布构建必须启用 CGO 禁用，生成静态二进制 |
| SQLite 驱动 | 必须使用 `modernc.org/sqlite` | 作者指南 | 禁止 `mattn/go-sqlite3`（CGO 依赖） |
| 依赖方向 | cmd→app→…→runtime→Eino 接口 | 作者指南 | 违反会在编译期报错 |
| 模块结构 | 单一 Go module | `go.mod` | 所有包属于 `github.com/Karlsk/oryxos-go` |
| 开发工具 | gosec、goimports、govulncheck | `go.mod` tool 段 | 通过 `go tool` 调用，非运行时依赖 |
| 配置热重载 | 不支持 | 作者指南 | 配置变更需重启进程生效 |

## 5. 缺失项

仓库中未发现以下常见构建工件：Makefile、Dockerfile、docker-compose.yml、`.github/workflows`、`.gitlab-ci.yml`、`build.sh`、`scripts/` 目录、版本注入脚本、交叉编译脚本。因此 CI/CD、容器化、版本打包流程不在本仓库范围内，可能位于外部仓库或由作者在其他地方维护。
