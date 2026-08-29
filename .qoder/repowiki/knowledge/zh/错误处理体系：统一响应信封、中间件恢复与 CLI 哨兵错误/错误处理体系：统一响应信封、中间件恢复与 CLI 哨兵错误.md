---
kind: error_handling
name: 错误处理体系：统一响应信封、中间件恢复与 CLI 哨兵错误
category: error_handling
scope:
    - '**'
source_files:
    - internal/web/api/result.go
    - internal/web/middleware/recovery.go
    - internal/web/middleware/body_limit.go
    - internal/web/middleware/access.go
    - cmd/oryxos/main.go
    - cmd/oryxos/root.go
    - internal/observability/logger.go
---

## 1. 整体方案

本仓库采用「分层错误策略」：
- **HTTP 层**：通过 `internal/web/api` 的 `Result[T]` 统一信封（`code/message/data/details/request_id`）对外暴露错误，所有错误码、消息由白名单校验，禁止随意拼接；异常 panic 由 `Recovery` 中间件兜底。
- **CLI 层**：Cobra 命令树关闭 `SilenceErrors`/`SilenceUsage`，顶层 `main` 捕获 `Execute()` 返回 error 并写入 stderr + `os.Exit(1)`；业务用 sentinel error（如 `ErrNotImplemented`）表达可预期失败。
- **日志/可观测性**：错误以结构化 `slog` 事件记录（如 `http.recovered_panic`、`http.request_complete`），并通过 `observability` 包注入 correlation/request_id。
- **配置/初始化阶段**：使用 `panic` 表达不可恢复的配置错误（如 body limit ≤ 0），由进程重启修复。

## 2. 关键文件与职责

| 文件 | 职责 |
|---|---|
| `internal/web/api/result.go` | 定义 `Result[T]` 信封、`Success/Page/Error` 写入器、错误码/消息白名单 `errorDescriptors`、details 字段白名单 `detailRules`、请求 ID 注入 |
| `internal/web/middleware/recovery.go` | Gin 中间件，recover panic → 记录 `http.recovered_panic` → 写 `api.Error(..., 500, "internal", ...)` |
| `internal/web/middleware/body_limit.go` | 请求体大小限制，构造时 panic 校验参数 |
| `internal/web/middleware/access.go` | 记录最终 HTTP 状态码、耗时到 observer 和 slog |
| `cmd/oryxos/main.go` | 唯一入口：`Execute() err != nil` → stderr + exit 1 |
| `cmd/oryxos/root.go` | 声明 `ErrNotImplemented = errors.New("not implemented")` 作为未实现功能的哨兵错误 |
| `internal/observability/logger.go` | 提供 JSON/Console 两种 logger，均经 `RedactingHandler` 脱敏 |

## 3. 架构与约定

### 3.1 HTTP 错误信封
- 成功响应仅允许 `(200, "ok", "ok")` 与 `(201, "created", "created")`，由 `validSuccessDescriptor` 强制。
- 错误响应必须从 `errorDescriptors` 中取 code→message 映射，否则降级为 `writeInternalFallback`（500/internal）。
- `details` 最多两个字段 `field`（匹配 `^[a-z][a-z0-9_]{0,63}$`）+ `rule`（仅限 `required/invalid_format/out_of_range/duplicate/too_large`），非法则同样降级。
- 所有响应自动附加 `request_id`（来自 `observability.CorrelationFromContext` 或随机生成 `req_<hex>`）。

### 3.2 Panic 与 recover 策略
- 仅在「不可恢复的配置/初始化错误」处使用 panic：`body_limit.go` 中 `limit <= 0` 直接 panic；`result.go` 中 PRNG 失败也 panic。
- 所有 HTTP handler 的 panic 被 `middleware.Recovery` 统一捕获，记录 `slog.LevelError` 的 `http.recovered_panic` 事件后返回 500。
- 测试中显式验证 panic 行为（如 `middleware_test.go` 中的 panic fixture）。

### 3.3 CLI 错误传播
- Cobra 根命令设置 `SilenceErrors: true, SilenceUsage: true`，由调用方自行控制输出格式。
- `main` 是唯一的错误出口：`fmt.Fprintln(os.Stderr, err); os.Exit(1)`。
- 业务语义错误使用 sentinel error（`ErrNotImplemented`），便于调用方 `errors.Is` 判断。
- 工作区操作广泛使用 `fmt.Errorf("...: %w", err)` 包装底层错误，并用 `errors.Join` 合并清理阶段的多个错误（如 chmod/publish/temp file cleanup）。

### 3.4 可观测性集成
- 每个 HTTP 请求经 `AccessObservation` 记录 method/route/status/duration 到 `observer.ObserveHTTP` 与 slog。
- 日志通过 `observability.Logger(ctx, baseLogger)` 获取，确保携带 correlation/request_id。
- RedactingHandler 在输出前对敏感属性做脱敏，避免错误信息泄露机密。

## 4. 约束与规则

- **HTTP 错误码与 message 必须来自白名单**：`validErrorDescriptor` 拒绝不在 `errorDescriptors` 中的组合，违规时回退到 500/internal（`result.go:118-122`）。
- **details 字段受限**：仅允许 `field` + `rule` 两项，且 rule 必须在 `detailRules` 集合内（`result.go:124-140`）。
- **panic 仅用于配置/初始化期不可恢复错误**：如 body limit ≤ 0（`body_limit.go:12`）、PRNG 失败（`result.go:184`）。
- **HTTP 层 panic 必须由 Recovery 中间件兜底**：测试覆盖 panic fixture 场景（`middleware_test.go:67`）。
- **CLI 错误一律走 stderr + exit 1**：`main.go` 是唯一出口，禁止在子命令中直接 `os.Exit`。
- **错误链使用 `%w` 包装 + `errors.Join` 聚合清理错误**：`workspace.go` 中多次出现 `errors.Join(err, cleanupErr)` 模式。
- **日志必须经 observability 包**：所有 `slog` 输出通过 `observability.Logger` 获取，保证 correlation 上下文传递。
