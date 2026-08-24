# OryxOS Go 项目初始化 Skill 设计

**日期**：2026-08-16
**状态**：已确认，待实施
**目标 Skill**：`oryxos-init`

## 1. 背景

Java 版本的 `oryxos-init` 负责一次性安装工程地基，包括模块骨架、日志、监控、
HTTP Server、API 规范、开发规范、安全扫描、CI 和 pre-commit。

Go 版本保留这个定位，但所有实现必须遵守以下事实优先级：

1. `docs/DemandAnalysis.md` 定义 What、阶段范围和验收口径。
2. `docs/TechnicalSolution.md` 定义 Go 实现方案。
3. `AGENTS.md` 和 constitution 定义工程治理约束。
4. Java Skill 只提供工作流参考，不作为 Go 架构事实。

## 2. 目标

执行 Skill 后，空仓库或半初始化仓库具备：

- Go 1.26+ 单 module、`CGO_ENABLED=0` 单二进制骨架；
- 技术方案规定的完整包目录和占位包；
- 可运行的配置、日志、内部监控、Application 生命周期和 Gin HTTP Server；
- 统一的 `Result[T]`、`PageResult[T]`、错误码和分页契约；
- 完整的 12 个 Cobra 叶子命令；
- Google Go Style、Effective Go 和 Go Code Review Comments 工程规范；
- 格式、静态检查、测试、源码安全和依赖漏洞检查；
- pre-commit、GitHub Actions 和只读验证脚本；
- 非破坏、可增量执行的初始化流程。

## 3. 非目标

Skill 不实现以下业务能力：

- Provider 和具体模型 connector；
- ReActLoop、PromptBuilder 和 AgentService；
- Memory、Session 和三张业务表；
- 内置 Tool、MCP Client 和 Sandbox；
- Scheduler 业务行为；
- 未定义的前端或项目主页；
- Prometheus endpoint、OpenAPI endpoint、Swagger、SSE 或 WebSocket；
- Provider fallback、circuit breaker 或其他扩展阶段能力。

HTTP 基础设施只实现 Server。出站 HTTP 的重试由后续 `ToolExecutor` 按幂等性决定，
不得在底层 HTTP Client 透明重试；熔断保持扩展阶段边界。

## 4. Skill 包结构

```text
.agents/skills/oryxos-init/
├── SKILL.md
├── agents/
│   └── openai.yaml
├── references/
│   ├── project-layout.md
│   ├── foundation-modules.md
│   ├── configuration.md
│   ├── application-lifecycle.md
│   ├── observability.md
│   ├── http-server.md
│   ├── http-client-boundary.md
│   ├── api-contract.md
│   ├── test-contracts.md
│   ├── go-quality.md
│   └── ci-verification.md
└── scripts/
    └── verify.sh
```

### 4.1 SKILL.md

`SKILL.md` 是执行入口，只保留：

- 触发条件和边界；
- 事实来源和冲突优先级；
- 参数确认；
- 空仓库/增量初始化判断；
- 阶段化工作流；
- reference 路由；
- 每阶段验证门禁；
- 非破坏策略和交付报告格式。

frontmatter 的 `description` 必须以 `Use when` 开头且只写触发条件。工作流细节保留在
正文，避免 agent 把元数据当成执行捷径。

### 4.2 Agent metadata

`agents/openai.yaml` 提供 UI 使用的 `display_name`、`short_description` 和一行
`default_prompt`；其中 default prompt 必须显式调用 `$oryxos-init`。该文件通过
skill-creator 脚本生成，并与 `SKILL.md` 保持同步。

### 4.3 references

`foundation-modules.md` 只做索引，不承载所有实现细节。每个子系统 reference 必须定义：

1. 职责和明确非目标；
2. 包位置和依赖方向；
3. 必须提供的接口；
4. 配置、默认行为和范围校验；
5. 超时、取消和错误传播；
6. 安全约束；
7. 单元与集成测试矩阵；
8. 验收命令。

### 4.4 verify.sh

`scripts/verify.sh` 只读检查目标项目，不创建、修改或删除文件。它负责结构校验、
命令和端点数量检查、依赖边界检查，并调用项目已经配置的质量门禁。

## 5. 执行流程

```text
环境审计
  -> 参数确认与变更清单
  -> Go module 和完整骨架
  -> 基础模块真实实现
  -> 12 个 CLI 命令
  -> 项目开发规范
  -> 质量和安全门禁
  -> CI 与 pre-commit
  -> 全量验证和交付报告
```

创建 Skill 本身时，必须先使用 skill-creator 的 `init_skill.py` 生成标准目录、
frontmatter 和 `agents/openai.yaml`，再替换生成的占位内容；完成后运行
`quick_validate.py`。

### 5.1 环境审计

Skill 必须：

- 读取需求、技术方案、`AGENTS.md` 和 constitution；
- 确认 Go 版本不低于 1.26；
- 已有 `go.mod` 时沿用 module path；
- 没有 `go.mod` 时询问 module path；
- 确认目标目录、HTTP 端口和服务名；
- 扫描目标文件并生成 `create / merge / skip / conflict` 清单；
- 在出现冲突时暂停，不覆盖；
- 不执行 `git init` 或 `git commit`。

### 5.2 非破坏分类

| 状态 | 行为 |
|---|---|
| `absent` | 创建文件 |
| `identical` | 跳过并报告 |
| `compatible` | 生成精确补丁，修改后验证 |
| `conflict` | 停止并请求用户确认 |

Skill 不得整文件覆盖已有 `go.mod`、CI、配置或业务文件，不得删除既有代码。
依赖下载或验证失败时可以保留可审阅变更，但必须报告未完成的验证，不得声称成功。

## 6. 目标项目骨架

```text
cmd/oryxos/
internal/
├── app/
├── config/
├── profile/
├── skill/
├── bootstrap/
├── provider/
├── runtime/
├── memory/
├── session/
├── tool/
│   ├── builtin/
│   └── mcp/
├── sandbox/
├── scheduler/
├── channel/cli/
├── web/
│   ├── api/
│   └── middleware/
├── store/
└── observability/
```

尚未实施的业务包只创建带职责和依赖说明的 `doc.go`。占位包不得预定义可能漂移的
业务接口，不得返回假业务结果。基础包必须提供真实、可测试和可运行的实现。

## 7. 基础模块

### 7.1 Configuration

`internal/config` 负责：

- 服务监听地址和 HTTP 超时；
- 环境变量读取、默认值和严格范围校验；
- 明确的字段级错误；
- 日志和错误中的敏感信息脱敏；
- 不保存 Profile 业务配置或真实凭证。

所有超时必须显式、可配置且有安全默认值。HTTP Server 的写超时必须兼容同步 Agent
调用，不能使用会提前中断正常 LLM 请求的固定短值。

### 7.2 Observability

`internal/observability` 使用标准库 `log/slog` JSON Handler。日志字段至少支持：

- `request_id`；
- `session_id`；
- `profile_name`；
- `channel`；
- `schedule_id`。

基础实现提供内部 Metrics 接口和 HTTP 请求观测点，但不注册 Prometheus endpoint。
日志和指标不得包含 API key、Webhook URL、MCP token 或展开后的敏感配置。

### 7.3 Application Lifecycle

`internal/app` 负责依赖组装、启动顺序、信号处理、`context.Context` 传播和反向关闭。
正常错误必须返回，不能使用 panic 作为流程控制。关闭必须有截止时间并汇总错误。

### 7.4 HTTP Server

`internal/web` 和 `internal/web/middleware` 提供：

- Gin router；
- 非零的 Header、Read、Write、Idle timeout；
- 有截止时间的 graceful shutdown；
- request ID 复用或生成；
- panic recovery；
- JSON access log；
- 请求耗时和 HTTP 状态观测；
- 请求体和响应体边界预留。

初始化阶段只注册：

- `GET /api/v1/health`；
- `GET /api/v1/info`。

不得注册 `/metrics`、OpenAPI、Swagger、SSE 或 WebSocket。

## 8. 统一 API 契约

所有 Handler 必须通过 `internal/web/api` 响应，不得直接调用
`gin.Context.JSON`。

```go
type Result[T any] struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Data      *T             `json:"data,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
	RequestID string         `json:"request_id"`
}

type PageResult[T any] struct {
	Items      []T   `json:"items"`
	Page       int   `json:"page"`
	PageSize   int   `json:"page_size"`
	Total      int64 `json:"total"`
	TotalPages int64 `json:"total_pages"`
}
```

普通成功使用 `Result[T]`，分页成功使用 `Result[PageResult[T]]`，错误使用同一外层
`code / message / details / request_id` 字段。HTTP 状态码必须保持真实语义，不能全部
返回 200。

API 规则：

- JSON 字段使用 `snake_case`；
- `code` 是稳定字符串，不直接暴露 Go error；
- request ID 同时进入响应头、响应体和日志；
- 空分页的 `items` 必须为 `[]`，不能为 `null`；
- `page` 从 1 开始，`page_size` 有默认值和最大值；
- 时间使用 RFC 3339 UTC；
- 精确金额等值不得使用浮点；
- 内部路径、堆栈和敏感值不得进入响应。

## 9. CLI 命令

完整注册 12 个 Cobra 叶子命令：

1. `oryxos init`；
2. `oryxos status`；
3. `oryxos chat`；
4. `oryxos serve`；
5. `oryxos gateway`；
6. `oryxos profile list`；
7. `oryxos profile create`；
8. `oryxos profile show`；
9. `oryxos profile delete`；
10. `oryxos provider list`；
11. `oryxos tool list`；
12. `oryxos session list`。

`init`、`status` 和 `serve` 提供真实基础实现。其余命令返回稳定的
`not implemented` 错误和非零退出码，不伪造业务结果。

`oryxos init` 必须幂等且非破坏：创建缺失工作区内容，已有目标只报告 `skipped`。

## 10. Go 开发规范

目标项目生成 `docs/DevelopmentGuide.md`，规范依据为：

- Effective Go；
- Google Go Style Guide；
- Go Code Review Comments。

强制规则包括：

- `gofmt` 和 `goimports`；
- 简短小写包名，禁止泛化的 `util/common/base` 包；
- 接口由消费方定义；
- 错误带上下文并使用 `%w` 保留错误链；
- 正常错误不用 panic；
- `context.Context` 作为首参数传递且不存入结构体；
- goroutine 有明确所有权、取消和退出路径；
- 禁止无边界并发和 goroutine 泄漏；
- 导出符号有 Go doc；
- 注释解释原因，不复述代码；
- table-driven tests 为默认测试组织形式。

目标项目还生成：

- `docs/ApiConvention.md`；
- `.github/pull_request_template.md`；
- `.editorconfig`。

## 11. 质量和安全门禁

生成以下文件：

```text
go.mod
go.sum
Makefile
.editorconfig
.golangci.yml
.pre-commit-config.yaml
.github/workflows/ci.yml
```

`.golangci.yml` 使用 v2 schema。golangci-lint 固定具体版本，GitHub CI 使用官方
Action；不得使用浮动版本。适合主 module 的 Go 工具使用 Go 1.26 `tool` directive
固定版本。golangci-lint 按官方建议使用独立的固定二进制/Action，不强塞入主 module
的工具依赖图。

`Makefile` 提供：

- `fmt`；
- `lint`；
- `test`；
- `test-race`；
- `security`；
- `build`；
- `verify`。

完整门禁：

```bash
gofmt / goimports
go mod tidy -diff
go vet ./...
go test ./...
go test -race ./...
golangci-lint run
gosec ./...
govulncheck ./...
CGO_ENABLED=0 go build ./cmd/oryxos
```

pre-commit 执行快速、无网络或低网络成本检查；GitHub Actions 执行完整测试、
race test、lint、安全扫描和纯 Go 构建。race test 可以使用 CI 的 CGO 环境，但发布
二进制必须由 `CGO_ENABLED=0` 构建。

## 12. 测试契约

| 子系统 | 强制覆盖 |
|---|---|
| Config | 默认值、非法值、缺失变量、未知字段、脱敏 |
| Observability | JSON、关联字段、错误字段、无敏感信息 |
| Result | 成功、空 data、错误 details、request ID |
| PageResult | 空数组、页码、页大小、总数和总页数 |
| Middleware | request ID、panic recovery、状态码和耗时 |
| HTTP Server | health/info、超时配置、优雅关闭和 404 |
| Endpoint boundary | metrics、OpenAPI、Swagger 均未注册 |
| CLI | 12 个叶子命令、退出码和占位错误 |
| init | 首次创建、重复执行、created/skipped 和不覆盖 |
| Architecture | runtime 不导入 Gin、Cobra、GORM、Eino-ext |
| Build | 常规构建、race test 和 `CGO_ENABLED=0` 构建 |

每个真实基础包按“契约测试 -> 实现 -> 子系统验证”的顺序完成。一个阶段失败时不得
进入下一阶段。

## 13. Definition of Done

- Skill 包结构完整，所有 reference 都能从 `SKILL.md` 按需路由；
- `agents/openai.yaml` 与 `SKILL.md` 一致并通过 skill-creator 校验；
- 空仓库可以生成完整骨架；
- 半初始化仓库不会被覆盖；
- 所有占位包可编译且只包含职责文档；
- `init`、`status`、`serve` 可运行；
- CLI 恰好有 12 个叶子命令；
- HTTP 只注册 health/info；
- 所有响应使用统一 Result 契约；
- 空分页输出 `items: []`；
- 日志是 slog JSON 且无明文凭证；
- Prometheus/OpenAPI/circuit breaker 未进入核心实现；
- pre-commit 和 CI 配置完整；
- verify 脚本只读；
- 所有实际执行过的验证命令有明确结果；
- 未执行的验证不得标记为通过。

## 14. 官方参考

- Go tool dependencies：https://go.dev/doc/modules/managing-dependencies
- Effective Go：https://go.dev/doc/effective_go
- Go Code Review Comments：https://go.dev/wiki/CodeReviewComments
- Google Go Style：https://google.github.io/styleguide/go/
- golangci-lint configuration：https://golangci-lint.run/docs/configuration/file/
- golangci-lint CI installation：https://golangci-lint.run/docs/welcome/install/ci/
- govulncheck：https://go.dev/doc/security/vuln/
- gosec：https://github.com/securego/gosec
