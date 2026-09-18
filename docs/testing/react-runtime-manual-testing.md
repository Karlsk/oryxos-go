# ReAct Runtime 手动测试指南

本文用于人工验收第 17 节 ReAct Runtime，范围只包括：`AgentService`、`ReActLoop`、
`PromptBuilder`、`ToolExecutor`、Bootstrap/Skill 启动快照，以及 `llm_calls` 和
`tool_invocations` 在这条链路上的语义。

本文不验收第 18 节的具体 SessionStore、第 20 节的 ToolRegistry/内置 Tool/MCP、第 24 节的
具体 Sandbox，也不验收 CLI、Web、Scheduler 的最终应用组装。尚未具备这些依赖时，不能把
真实天气或真实 Tool 流程标记为通过。

## 1. 验收层级

| 层级 | 当前是否可执行 | 验证内容 |
|---|---:|---|
| A：确定性 ReAct harness | 是 | 循环、消息累积、Prompt、超时、重试、日志、审计错误语义 |
| B：真实 Provider + 真实 Tool | 否，依赖第 18/20/24 节及应用组装 | DeepSeek/MiniMax 多轮 Tool Calling、SQLite 最终证据 |

验收记录必须分别填写两层。A 通过不能替代 B，B 尚未执行时应写“BLOCKED/未执行”，不能写
“PASS”。

## 2. 前置检查

在仓库根目录执行：

```bash
go version
git status --short
```

预期：Go 为 1.26 或更高。`git status` 只用于记录测试前状态，不要求工作区干净，也不要删除
不属于本次 ReAct 变更的文件。

## 3. 层级 A：当前即可执行的确定性验收

### 3.1 ReAct 的结束、Tool 循环与串行顺序

```bash
go test -count=1 -v ./internal/runtime \
  -run 'TestReActLoopCompletesFinalAndToolAssistedResponses|TestReActLoopStopsAfterExactMaximumIterations'
```

通过条件：

- 无 Tool Call 的 assistant 响应立即成为最终结果；
- assistant 完整 Tool Call 先进入 Session，Tool 结果随后以匹配的 `tool_call_id` 进入 Session；
- 单轮多个 Tool Call 按模型返回顺序串行执行；
- 持续请求 Tool 的模型在 `max_iterations` 次模型调用后返回 `ErrMaxIterations`。

失败条件：Tool 并行或乱序、Tool 结果丢失 call ID、迭代次数多一次或少一次、失败被转换成成功。

### 3.2 第二轮 Prompt 不重复当前用户消息

```bash
go test -count=1 -v ./internal/runtime \
  -run 'TestReActLoopSecondModelRequestContainsCurrentUserOnce|TestPromptBuilderKeepsCurrentToolTurnOnceAndDropsOnlyOldCompleteTurnsForBudget'
```

通过条件：第二次模型请求中当前用户消息恰好出现一次；同一当前轮的 assistant Tool Call 和
Tool Result 都保留；输入预算不足时只删除最旧的完整历史轮，不拆开 Tool Call/Result，不删除
系统上下文或当前轮。

如果受保护的系统上下文加完整当前轮本身已经超过配置预算，预期行为是明确报错，而不是静默
截断安全规则或当前 Tool 观察。

### 3.3 Prompt 来源、优先级与启动快照

```bash
go test -count=1 -v ./internal/runtime \
  -run 'TestPromptBuilderUsesExactPrecedenceMemoryCapAndClock|TestPromptBuilderKeepsNewestCompleteTurnsAndCurrentRequest'
go test -count=1 -v ./internal/bootstrap ./internal/skill
```

通过条件：

- Prompt 来源标签顺序为 Runtime Rules、项目规则、Identity/Skill、SOUL、USER、Memory、历史、
  当前请求、当前时间；
- Memory 最多注入 4000 个 Unicode 字符；
- `max_history_turns` 按完整用户根轮次裁剪；
- Bootstrap/Skill 只在启动组装时形成不可变 Snapshot；
- 绝对路径、`..` 与软链接逃逸都硬失败，不出现条件性跳过。

### 3.4 Tool 执行超时、重试、结构化日志和最终审计

```bash
go test -count=1 -v ./internal/tool \
  -run 'TestExecutorValidatesLookupAllowListAndSchemaBeforeInvoke|TestExecutorTimeoutRetryAndFinalAudit|TestExecutorLogsEveryPhysicalAttemptWithCorrelationAndRedaction|TestExecutorJoinsToolAndRecorderFailures'
```

通过条件：

- Profile 未允许或未注册的 Tool 在 `Invoke` 前失败；
- 参数 schema 校验失败时不调用 Tool；
- 阻塞 Tool 的 attempt context 实际到期，向调用方传播 `context.DeadlineExceeded`；
- 只有 Retryable、Idempotent 且错误明确可重试时，才在首次调用之外最多重试三次；
- 每次物理 `Invoke` 产生一条 `event=tool_attempt` 日志，含 `session_id`、`tool_name`、
  `attempt`、`final_attempt`、`success`、`duration_ms` 和已有的请求关联字段；
- 日志不记录 Tool arguments/result，错误中的凭证被脱敏；
- 无论物理尝试几次，一次逻辑 Tool Call 只写一条最终 `tool_invocations` 记录。

### 3.5 模型成功但 `llm_calls` 写入失败时 fail-closed

```bash
go test -count=1 -v ./internal/provider ./internal/runtime \
  -run 'TestProviderServiceAuditsSuccessFailureAndPersistenceErrors|TestReActLoopPreservesSuccessfulResponseButSkipsToolsWhenAuditFails|TestReActLoopDoesNotPreserveResponseForOrdinaryModelError'
```

通过条件：

- 模型已成功、仅审计写入失败时，Provider 返回原始响应和 `AuditPersistenceError`；
- ReActLoop 把已经发生的 assistant 响应追加到 Session，向上返回错误，不执行其中任何 Tool；
- 普通模型错误不追加不存在的模型响应；
- 模型失败与审计失败同时发生时，不把响应标记为可用。

### 3.6 AgentService 统一入口与关联字段

```bash
go test -count=1 -v ./internal/runtime \
  -run 'TestAgentServiceUsesOnePathForCallerShapesAndProfileIsolation|TestAgentServiceSavesAfterFailureWithDetachedContextAndJoinsErrors|TestAgentServiceSavesAfterSuccessAndReturnsSaveFailure'
```

通过条件：CLI/Web/Scheduler 形状的请求都走同一 `Invoke`；Session 由 SessionService 解析，
不是“每个 Profile 固定一个 Session”；传入 ReActLoop 的 Context 包含最终 `profile_name`、
`channel`、`session_id`，并保留入口已有的 request/schedule 关联信息；成功和失败都尝试保存
Session。

## 4. 层级 A：完整交付门禁

```bash
go test -count=1 ./internal/profile ./internal/bootstrap ./internal/skill ./internal/tool ./internal/store ./internal/provider ./internal/runtime
go test -race -count=1 ./internal/session
go test -race -count=1 ./internal/runtime ./internal/tool
go test -count=1 ./...
go vet ./...
CGO_ENABLED=0 go build ./cmd/oryxos
git diff --check
```

边界扫描：

```bash
rg 'github.com/cloudwego/eino' internal --glob '*.go' --glob '!**/*_test.go'
rg 'AutoMigrate|mattn/go-sqlite3' internal --glob '*.go' --glob '!**/*_test.go'
rg 't\.Skip' internal/bootstrap internal/skill internal/runtime internal/tool --glob '*_test.go'
```

预期：Eino 生产依赖只在 `internal/provider`；没有 `AutoMigrate` 或 CGO SQLite 驱动；最后一条
命令没有输出。真实 Provider smoke test 因缺少凭证而 `SKIP` 不属于这个确定性扫描。

## 5. 层级 B：依赖满足后执行的端到端人工验收

只有同时满足以下条件后才执行：

- 第 18 节已提供可恢复的 SessionService/SessionStore；
- 第 20 节已提供 ToolRegistry、`http_get` 等具体 Tool；
- 第 24 节 Sandbox 已接入 Tool 的外部副作用路径；
- 应用启动代码已将 Profile、Provider、PromptBuilder、ToolExecutor 和 AgentService 组装起来；
- `chat` 或等价调试入口确实调用统一 `AgentService.Invoke`。

准备一个仅允许 `http_get` 的测试 Profile 和最小天气 Skill。先用 DeepSeek，再把 Profile 的
Provider/模型切到 MiniMax，分别发起“查询天气后给出穿衣建议”。每次测试记录：

1. 第一轮 assistant 含非空 Tool Call ID 和 `http_get`；
2. Session 随后含同 ID 的 Tool message；
3. 第二轮模型能读取 Tool 结果并返回无 Tool Call 的最终建议；
4. 同一个 Profile 可通过不同显式 Session ID 形成多个 Session；resume 只恢复指定 Session；
5. `llm_calls` 有两条逻辑模型调用记录，`tool_invocations` 有一条逻辑 Tool 调用记录；
6. 如果人为令 `llm_calls` 不可写，assistant Tool Call 可以留在 Session 供排查，但 Tool 不执行；
7. MiniMax/OpenAI 兼容链路中的 Tool Call ID 在 assistant 与 Tool message 间完全一致。

任何网络、凭证、后续模块缺失导致的中断都记为 BLOCKED，不记为 ReAct PASS 或 FAIL。

## 6. 证据记录模板

```text
日期/时间：
提交或工作树标识：
Go 版本：

层级 A：PASS / FAIL
- focused harness：
- race：
- full test：
- vet：
- CGO_ENABLED=0 build：
- diff/invariant scan：

层级 B：PASS / FAIL / BLOCKED / 未执行
- DeepSeek：
- MiniMax：
- Session 消息序列证据：
- llm_calls 行数/状态：
- tool_invocations 行数/状态：
- 阻塞依赖或失败原因：

敏感信息检查：日志和截图中无 API Key、Webhook URL、认证头或完整 Tool 输入/输出。
```

## 7. 清理

测试完成后：

- 删除仅为人工验收创建的临时工作区和临时 SQLite；
- `unset DEEPSEEK_API_KEY MINIMAX_API_KEY`（如果本次 shell 注入过）；
- 不提交真实凭证、`.env`、数据库、日志或包含业务数据的 Session 导出；
- 再次执行 `git status --short`，确认没有误生成 ReAct 范围外的文件。
