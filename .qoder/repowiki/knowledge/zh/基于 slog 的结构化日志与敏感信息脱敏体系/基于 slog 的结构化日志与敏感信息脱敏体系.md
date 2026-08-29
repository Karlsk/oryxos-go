---
kind: logging_system
name: 基于 slog 的结构化日志与敏感信息脱敏体系
category: logging_system
scope:
    - '**'
source_files:
    - internal/observability/logger.go
    - internal/observability/console_handler.go
    - internal/observability/redacting_handler.go
    - internal/observability/correlation.go
    - internal/observability/observer.go
    - internal/config/redact.go
    - internal/web/middleware/access.go
    - internal/app/foundation.go
---

## 1. 使用的系统/框架

OryxOS 使用 Go 标准库 `log/slog` 作为唯一日志框架，通过自定义 `slog.Handler` 链实现结构化输出、控制台彩色渲染和敏感字段自动脱敏。没有引入第三方日志库（如 zap、logrus）。

## 2. 核心文件与职责

- `internal/observability/logger.go`：提供 `NewLogger`（JSON 格式）和 `NewConsoleLogger`（人类可读彩色文本）两个工厂函数，统一创建带脱敏能力的 `*slog.Logger`。
- `internal/observability/console_handler.go`：实现 `consoleHandler`，按 `时间 级别 消息 key=value...` 的格式输出，对 ERROR/WARN/INFO/DEBUG 分别着色。
- `internal/observability/redacting_handler.go`：实现 `RedactingHandler`，作为所有 handler 的包装器，递归遍历每条记录的 message 与所有属性，将敏感值替换为 `[REDACTED]`。
- `internal/observability/correlation.go`：定义 `Correlation` 结构体（RequestID、SessionID、ProfileName、Channel、ScheduleID），通过 context 在请求链路中传递关联标识，并提供 `Logger(ctx, base)` 将非空关联字段注入到日志属性中。
- `internal/observability/observer.go`：进程内 HTTP 观测器，聚合 method/route/status 维度的请求计数与耗时，暴露 `Snapshot()` 供健康检查等接口消费。
- `internal/config/redact.go`：集中维护敏感键白名单（`api_key`、`authorization`、`mcp_auth`、`password`、`secret`、`token`、`webhook_url` 及含 `apikey`/`credential`/`webhook` 的变体）和错误字符串脱敏正则（匹配 `key=value`、`user:pass@host` URL、`Bearer/Basic` 头）。
- `internal/web/middleware/access.go`：HTTP 中间件，在每个请求结束时记录 `http.request_complete` 事件，并调用 `observer.ObserveHTTP` 更新指标。
- `internal/app/foundation.go`：应用启动时根据配置选择 JSON 或 Console 日志输出。

## 3. 架构与设计决策

- **Handler 链模式**：每个 logger 由 `RedactingHandler` 包裹下游 handler（JSON 或 console），确保所有输出都经过同一套脱敏逻辑，避免业务代码遗漏。
- **结构化字段约定**：日志属性使用下划线命名（如 `request_id`、`session_id`、`profile_name`、`channel`、`schedule_id`、`duration_ms`、`status`），与 `Correlation` 字段一一对应。
- **关联上下文贯穿全链路**：通过 `context.Context` 携带 `Correlation`，Web 层用 `request_id`，调度器用 `schedule_id`，Agent 会话用 `session_id` + `profile_name` + `channel`，使一次请求的所有日志可被串联检索。
- **进程内指标 + 日志分离**：`Observer` 仅做内存聚合，不写日志；HTTP 访问日志通过 `slog` 输出，两者互补。
- **安全优先**：任何可能包含密钥的字段名或错误消息都会整体替换为 `[REDACTED]`，而非截断，防止凭据泄露。

## 4. 约定与约束

- **日志级别**：统一使用 `slog.LevelDebug/Info/Warn/Error`，默认以 `slog.LevelInfo` 初始化生产 logger。
- **输出目标**：JSON 格式用于机器消费（管道/日志采集），Console 格式用于本地开发调试；二者均受 `slog.Leveler` 控制。
- **敏感字段强制脱敏**：`RedactingHandler` 对所有 `slog.Attr` 递归处理，包括嵌套 group；键命中 `config.IsSensitiveKey`、值为 `error`/`*_error`、或包含凭据 URL/Authorization 头的字符串都会被替换为 `[REDACTED]`。
- **关联字段注入规则**：`observability.Logger(ctx, base)` 仅注入非空的 RequestID、SessionID、ProfileName、Channel、ScheduleID，保持日志整洁。
- **HTTP 日志规范**：`access.go` 固定输出 `http.request_complete` 事件，包含 `method`、`route`、`status`、`duration_ms` 四个属性。
- **构建约束**：发布必须 `CGO_ENABLED=0 go build ./cmd/oryxos`，因此日志实现不能依赖 CGO 依赖（当前仅使用标准库与纯 Go 包，满足该约束）。
- **模块边界**：`internal/observability` 不依赖 Gin/Cobra，仅被 `internal/app`、`internal/web`、`cmd/oryxos` 导入，符合项目单向依赖约定。