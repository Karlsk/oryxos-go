# ReAct Runtime 开发指南

本文面向需要理解、维护或扩展 OryxOS ReAct Runtime 的开发者，以当前仓库第 17 节实现为准，重点说明以下四个核心组件及其连接方式：

- `AgentService`：统一 Agent 调用入口；
- `PromptBuilder`：每轮模型调用前的上下文组装器；
- `ReActLoop`：Reason → Act → Observe 循环控制器；
- `ToolExecutor`：Tool 校验、执行、重试与审计边界。

本文同时说明 `BootstrapLoader`、`SkillLoader`、`ProviderService`、`Session` 和调用记录如何接入这四个核心组件，以及启动加载和运行时读取之间的边界。

## 1. 总体架构

完整调用链如下：

```text
CLI / Web / Scheduler
        │
        ▼
AgentService.Invoke
  ├─ 查找 Profile
  ├─ Resolve Session
  │
  ▼
ReActLoop.Run
  ├─ Session.Append(user)
  │
  ├─ PromptBuilder.Build
  │     ├─ Runtime Rules
  │     ├─ Bootstrap Snapshot
  │     ├─ Profile Identity
  │     ├─ Skill Snapshot
  │     ├─ Memory
  │     ├─ Session History
  │     └─ Current Message + Current Time
  │
  ├─ ProviderService.Chat
  │     ├─ 按 Profile.name 找模型
  │     ├─ 绑定 Profile 允许的 Tool schemas
  │     ├─ 调用 llm.ChatModel
  │     └─ 写入 llm_calls
  │
  ├─ 无 Tool Call ────────────────────► 返回最终答案
  │
  └─ 有 Tool Call
        │
        ▼
      ToolExecutor.Execute
        ├─ Profile Tool 白名单
        ├─ Tool 精确查找
        ├─ JSON Schema 参数校验
        ├─ 超时与有限重试
        ├─ InvokableTool.Invoke
        └─ 写入 tool_invocations
        │
        ▼
      Session.Append(tool result)
        │
        └─────────────────────────────► 下一轮 ReAct

AgentService 最后保存 Session
```

核心原则是：入口可以有多个，但业务执行链只有一条。

```text
chat ─────┐
serve ────┼──→ AgentService.Invoke
gateway ──┘
```

禁止为不同入口分别实现 `CLIReActLoop`、`WebReActLoop` 或 `SchedulerReActLoop`。

## 2. 目录与职责

| 文件 | 职责 |
|---|---|
| `internal/runtime/types.go` | `AgentRequest`、`AgentResponse`、`AgentService` 公共契约 |
| `internal/runtime/agent_service.go` | Profile/Session 解析、ReAct 调用和 Session 保存 |
| `internal/runtime/prompt_builder.go` | 分层 Prompt、历史截断、Memory 限制和时间注入 |
| `internal/runtime/react_loop.go` | 自实现 ReAct 循环和 Tool 消息累积 |
| `internal/tool/types.go` | OryxOS 自有 `InvokableTool`、`OryxTool` 契约 |
| `internal/tool/schema.go` | 标准库实现的 JSON Schema 子集校验 |
| `internal/tool/executor.go` | Tool allow-list、执行、超时、重试和最终审计 |
| `internal/bootstrap/loader.go` | Bootstrap 文件的启动时不可变快照 |
| `internal/skill/loader.go` | Profile 显式引用 Skill 的启动时不可变快照 |
| `internal/session/session.go` | 并发安全的内存消息聚合 |
| `internal/provider/service.go` | 单次模型调用、Tool schema 绑定和 `llm_calls` 审计 |
| `internal/store/tool_invocation*.go` | Tool 最终调用结果模型和持久化 |

依赖边界保持为：

```text
AgentService → ReActLoop
ReActLoop → PromptBuilder + Provider 调用窄接口 + ToolExecutor 窄接口
PromptBuilder → OryxOS Profile / Session / Bootstrap / Skill / Memory 窄接口
ToolExecutor → OryxOS InvokableTool + ToolInvocation recorder
ProviderService → OryxOS llm.ChatModel
```

`internal/runtime` 和 `internal/tool` 不导入 Eino。Eino core 与 Eino-ext 只能存在于 `internal/provider`。

## 3. 公共请求与响应

`internal/runtime/types.go` 定义三个入口共享的请求：

```go
type AgentRequest struct {
	ProfileName string
	Channel     string
	UserID      string
	SessionID   string
	Message     string
	Stateless   bool
}

type AgentResponse struct {
	SessionID string
	Content   string
}

type AgentService interface {
	Invoke(ctx context.Context, request AgentRequest) (AgentResponse, error)
}
```

典型调用形态：

```go
// CLI
AgentRequest{
	ProfileName: "default",
	Channel:     "cli",
	UserID:      "gao",
	Message:     "查询上海天气",
}

// Web Session
AgentRequest{
	ProfileName: "default",
	Channel:     "web",
	UserID:      "user-123",
	SessionID:   "session-001",
	Message:     "查询上海天气",
}

// Scheduler
AgentRequest{
	ProfileName: "weather",
	Channel:     "scheduler",
	UserID:      "daily-weather",
	Message:     "生成今天的天气建议",
}
```

`AgentService` 不根据这些字段自行拼接 Session ID。显式 Session、无状态调用、CLI/Web 状态会话和 Scheduler Session 的解析规则属于第 18 节 `SessionService`。

## 4. AgentService：统一入口和生命周期管理

### 4.1 依赖窄接口

`AgentService` 通过三个窄接口完成编排：

```go
type profileResolver interface {
	Get(name string) (*profile.Profile, bool)
}

type sessionService interface {
	Resolve(
		ctx context.Context,
		request AgentRequest,
		profileName string,
	) (*session.Session, error)

	Save(
		ctx context.Context,
		current *session.Session,
	) error
}

type agentLoop interface {
	Run(
		ctx context.Context,
		current *session.Session,
		userMessage string,
		selected *profile.Profile,
	) (string, error)
}
```

这里没有直接依赖 Gin、Cobra、GORM、Eino 或具体 Provider connector。入口层只需要构造 `AgentRequest`，然后调用 `AgentService.Invoke`。

### 4.2 Invoke 执行顺序

`Invoke` 的固定顺序是：

1. 校验 Context、Profile 名和用户消息；
2. 按 `Profile.name` 获取不可变 Profile；
3. 将完整 `AgentRequest` 交给 Session seam 解析；
4. 调用 `ReActLoop.Run`；
5. 无论循环成功还是失败，都保存当前 Session；
6. 返回 Session ID、最终内容或保留原因链的错误。

核心代码：

```go
selected, ok := service.profiles.Get(profileName)
if !ok {
	return AgentResponse{}, fmt.Errorf(
		"invoke agent: profile %q not found",
		profileName,
	)
}

current, err := service.sessions.Resolve(
	ctx,
	request,
	selected.Name,
)
if err != nil {
	return AgentResponse{}, safeRuntimeWrap(
		"invoke agent: resolve session",
		err,
	)
}

correlation := observability.CorrelationFromContext(ctx)
correlation.SessionID = current.ID
correlation.ProfileName = selected.Name
correlation.Channel = request.Channel
runContext := observability.WithCorrelation(ctx, correlation)

content, runErr := service.loop.Run(
	runContext,
	current,
	request.Message,
	selected,
)

saveErr := service.sessions.Save(
	context.WithoutCancel(runContext),
	current,
)
```

Session 解析后才能得到最终 `session_id`，因此 AgentService 在这里补齐关联字段。入口已经
写入的 `request_id` 或 `schedule_id` 会被保留，Profile、Channel 和 Session 则以本次实际
解析结果为准。ReAct、Tool 尝试日志和 Session 保存共享同一组关联值。

### 4.3 为什么保存使用 WithoutCancel

如果用户在模型或 Tool 执行期间取消请求，原始 Context 已经进入 canceled 状态。直接使用原 Context 保存 Session 会立即失败，从而丢失已经产生的消息和失败现场。

因此保存使用：

```go
context.WithoutCancel(ctx)
```

它保留 Context 中的值，但移除取消信号和 deadline，使失败会话仍有机会被保存。

如果 ReAct 运行和 Session 保存同时失败，使用 `errors.Join` 同时保留两个错误原因：

```go
if safeRunErr != nil && safeSaveErr != nil {
	return response, errors.Join(
		safeRunErr,
		safeSaveErr,
	)
}
```

## 5. PromptBuilder：每轮模型调用前组装上下文

### 5.1 数据分为启动快照和请求状态

`PromptBuilder` 保存：

```go
type PromptBuilder struct {
	runtimeRules       string
	bootstrapByProfile map[string]bootstrap.Snapshot
	skillsByProfile    map[string]skill.Snapshot
	maxInputRunes      map[string]int
	memory             memoryReader
	now                func() time.Time
}
```

启动时固定：

- Runtime Rules；
- Bootstrap Snapshot；
- Skill Snapshot。

请求时动态读取：

- Memory；
- Session History；
- 当前用户消息；
- 当前日期时间。

构造函数会再次复制 Snapshot，避免外部代码修改 PromptBuilder 已持有的启动上下文。

### 5.2 BootstrapLoader 和 SkillLoader 何时调用

`bootstrap.Loader.Load` 和 `skill.Loader.Load` 不应在 `PromptBuilder.Build` 中调用。它们应在应用启动、Profile Runtime 组装时调用一次。

```text
应用启动
  → 加载实例配置
  → 加载所有合法 Profile
  → 针对每个 Profile 加载 Bootstrap Snapshot
  → 针对每个 Profile 加载 Skill Snapshot
  → 构造 PromptBuilder
  → 开始接受 Agent 请求
```

概念代码如下。完整生产组装要等 Session、Memory 和 ToolRegistry 后续模块落地：

```go
bootstrapSnapshots := make(map[string]bootstrap.Snapshot)
skillSnapshots := make(map[string]skill.Snapshot)
inputBudgets := make(map[string]int)

for _, selected := range profileRegistry.List() {
	bootSnapshot, err := (bootstrap.Loader{}).Load(
		workspaceRoot, // .oryxos/
		selected.Bootstrap,
	)
	if err != nil {
		return fmt.Errorf(
			"load bootstrap for profile %q: %w",
			selected.Name,
			err,
		)
	}

	skillSnapshot, err := (skill.Loader{}).Load(
		filepath.Join(workspaceRoot, "skills"),
		selected.Skills,
	)
	if err != nil {
		return fmt.Errorf(
			"load skills for profile %q: %w",
			selected.Name,
			err,
		)
	}

	bootstrapSnapshots[selected.Name] = bootSnapshot
	skillSnapshots[selected.Name] = skillSnapshot
	inputBudgets[selected.Name] = resolveModelInputRuneBudget(selected)
}

promptBuilder, err := runtime.NewPromptBuilder(
	runtimeRules,
	bootstrapSnapshots,
	skillSnapshots,
	memoryService,
	inputBudgets,
	time.Now,
)
```

`resolveModelInputRuneBudget` 表示应用组装层根据所选模型能力换算出的保守输入字符预算，
不是新的 Profile YAML 字段。第 17 节只提供 PromptBuilder 的预算注入 seam，不在 Runtime
硬编码某个厂商的上下文窗口，也不扩展 Provider 端口。

不在每轮重新读取文件的原因是：一次 ReAct 请求可能调用模型多次。如果第二轮重新读取磁盘，同一请求可能在前后两轮看到不同的 Identity、规则或 Skill。

```text
第一轮读取旧 AGENTS.md
→ 模型请求 Tool
→ 用户修改 AGENTS.md
→ 第二轮读取新 AGENTS.md
→ 同一次请求发生规则漂移
```

当前设计保证 Bootstrap 和 Skill 修改重启生效，不做热重载。

### 5.3 默认 Bootstrap 规则

Profile 没有配置 `bootstrap` 时：

```go
(bootstrap.Loader{}).Load(workspaceRoot, nil)
```

按顺序加载：

```text
.oryxos/AGENTS.md
.oryxos/SOUL.md
.oryxos/USER.md
```

默认文件不存在时产生空内容段；显式引用的文件不存在时加载失败。

Bootstrap 和 Skill loader 还会：

- 拒绝绝对路径；
- 拒绝 `..` 路径逃逸；
- 使用真实路径检查软链接逃逸；
- 只接受普通文件；
- Skill 只加载 Profile 明确引用的 `SKILL.md`；
- 拒绝重复 Skill 引用；
- 不扫描或自动发现未引用 Skill。

### 5.4 Build 的固定优先级

`Build` 先按当前 `Profile.name` 选择 Snapshot：

```go
boot, ok := builder.bootstrapByProfile[selected.Name]
skills, ok := builder.skillsByProfile[selected.Name]
```

然后读取 Memory，并按 Unicode 字符截断为最多 4000 字：

```go
memory, err := builder.memory.Read(ctx, selected)
memory = truncateRunes(memory, maxMemoryCharacters)
```

固定标签与顺序：

```go
systemContent := strings.Join([]string{
	"[RUNTIME_RULES]\n" + builder.runtimeRules,
	"[PROJECT_RULES]\n" + projectRules,
	"[AGENT_IDENTITY]\n" + identity,
	"[SKILLS]\n" + renderSkillSections(skills.Sections()),
	"[USER_PREFERENCES]\n" + preferences,
	"[LONG_TERM_MEMORY]\n" + memory,
}, "\n")
```

优先级是：

```text
运行时安全约束
  > AGENTS.md 项目规则
  > Profile Identity / Skill
  > SOUL.md
  > USER.md
  > MEMORY.md
```

Bootstrap 和 Skill 内部还会带文件边界：

```text
--- BOOTSTRAP: AGENTS.md ---
...

--- SKILL: weather/SKILL.md ---
...
```

### 5.5 历史必须保留结构

PromptBuilder 不把历史消息压平成字符串，而是继续使用 OryxOS `llm.Message`：

```go
prefix := []llm.Message{
	{Role: llm.RoleSystem, Content: systemContent},
	{Role: llm.RoleSystem, Content: "[HISTORY]"},
}

history, currentTurn := splitCurrentTurn(
	current.Messages(),
	currentUserMessage,
)
history = limitCompleteTurns(history, historyLimit(selected))

suffix := []llm.Message{
	{Role: llm.RoleSystem, Content: "[USER_MESSAGE]"},
}
suffix = append(suffix, currentTurn...)
suffix = append(suffix,
	llm.Message{
		Role: llm.RoleSystem,
		Content: "[CURRENT_DATETIME]\n" +
			builder.now().Format(time.RFC3339),
	},
)
```

这样能保留：

- assistant 的完整 `ToolCalls`；
- Tool Call ID；
- Tool name；
- 原始 arguments；
- Tool message 的 `ToolCallID`；
- Provider 附加字段。

历史按完整的“用户根轮次”裁剪：

```text
user
  assistant(tool calls)
  tool(result 1)
  tool(result 2)
  assistant(final)
```

不会截出孤立的 Tool 结果，也不会把当前用户消息重复放进模型请求。

这里的“当前轮”是 Session 中最后一个与本次输入匹配的 user 消息，以及其后已经累积的
assistant/Tool 消息。第一次模型调用时它只有 user；第二次调用时它已经是：

```text
user（恰好一次）
assistant(tool calls)
tool(result)
```

先按 `max_history_turns` 保留最近历史轮，再计算整个消息数组的 Unicode rune 数。如果超过
该 Profile 的输入预算，只重复删除最旧的完整历史轮。System 上下文和完整当前轮属于受保护
内容；它们单独已经超限时直接报错，不进行指令截断或 Tool 消息拆分。

### 5.6 Tool schemas 不由 PromptBuilder 解析

PromptBuilder 只生成 `[]llm.Message`。当前 Profile 的 Tool schemas 由 `ProviderService.Chat` 解析：

```go
definitions, err := service.resolver.Resolve(
	ctx,
	selected.Tools,
)

response, callErr := chatModel.Generate(
	ctx,
	llm.Request{
		Messages: messages,
		Tools:    definitions,
	},
)
```

职责划分为：

```text
PromptBuilder
  → 组装模型消息上下文

ProviderService
  → 将 Profile Tool 名解析为 llm.ToolDefinition
  → 调用 Profile 专属模型
```

## 6. ReActLoop：控制 Reason → Act → Observe

### 6.1 三个依赖

`ReActLoop` 依赖：

```go
type chatService interface {
	Chat(...) (llm.Response, error)
}

type toolExecutor interface {
	Execute(...) (string, error)
}

type promptBuilder interface {
	Build(...) ([]llm.Message, error)
}
```

生产对应关系：

```text
chatService   → provider.Service
toolExecutor  → tool.Executor
promptBuilder → runtime.PromptBuilder
```

Runtime 不需要知道模型由 DeepSeek、MiniMax 还是其他 connector 实现。

### 6.2 用户消息只追加一次

```go
current.Append(llm.Message{
	Role:    llm.RoleUser,
	Content: userMessage,
})
```

这行位于迭代循环外。否则每次 Tool 返回后进入下一轮时都会重复添加用户消息。

### 6.3 一次迭代等于一次模型调用

```go
for iteration := 0;
	iteration < selected.Settings.MaxIterations;
	iteration++ {
	// Build prompt
	// Call model once
	// Optionally execute Tool calls
}
```

因此 `max_iterations=10` 表示最多进行十次模型调用，而不是最多执行十个 Tool。

每轮开始都重新基于当前 Session 构建 Prompt：

```go
messages, err := loop.prompts.Build(
	ctx,
	current,
	selected,
	userMessage,
)
```

上一轮写入 Session 的 assistant Tool Call 和 Tool result 会自然成为下一轮的观察输入。

### 6.4 模型调用必须经过 ProviderService

```go
response, err := loop.chat.Chat(
	ctx,
	current.ID,
	selected,
	messages,
)
```

ReActLoop 不直接调用 `llm.ChatModel.Generate`，因为 `ProviderService.Chat` 还负责：

- 按 `Profile.name` 找模型实例；
- 解析 Profile 可用 Tool schemas；
- 调用模型；
- 写入一条 `llm_calls`；
- 记录 token 和耗时；
- 脱敏错误。

#### 6.4.1 模型成功、审计失败时保留响应但停止执行

模型调用和审计写入是两个先后发生的事实。模型可能已经成功返回完整 assistant 消息，
随后 `llm_calls` 插入失败。此时不能把模型响应当成成功结果继续执行，因为其中的 Tool
Call 尚未形成可靠审计；但也不应丢弃已经发生的模型输出。

`ProviderService.Chat` 因此返回两个值：原始 `llm.Response`，以及
`AuditPersistenceError`。该错误实现 `ModelResponseAvailable() bool` 标记：

```go
response, callErr := chatModel.Generate(ctx, request)
outcomeErr := service.recordOutcome(ctx, metadata, startedAt, &response, callErr)
if outcomeErr != nil {
	if callErr == nil {
		return response, &AuditPersistenceError{Cause: outcomeErr}
	}
	return llm.Response{}, outcomeErr
}
```

只有 `callErr == nil`，也就是模型调用确实成功时，才返回这个标记。模型本身失败且失败
记录也写不进去时，错误仍向上传播，但返回值不会被标记为可保留，防止保存不完整响应。

`ReActLoop` 不导入 `internal/provider`，而是识别一个包内窄接口：

```go
type modelResponseAvailableError interface {
	ModelResponseAvailable() bool
}

response, err := loop.chat.Chat(ctx, current.ID, selected, messages)
if err != nil {
	if modelResponseAvailable(err) {
		current.Append(response.Message)
	}
	return "", err
}
```

这里追加 assistant 消息后立即返回，代码不会进入后面的 Tool 遍历。因此最终状态是：

```text
Session: user → assistant（可能含 Tool Call）
返回值: 审计持久化错误
Tool:    0 次执行
```

`AgentService` 随后仍会在脱离取消信号的保存上下文中保存 Session。这样排障人员可以看到
模型实际返回了什么，同时系统维持 fail-closed：没有可靠 `llm_calls` 记录，就不允许产生
后续 Tool 副作用。

### 6.5 先保存 assistant，再判断 Tool Call

```go
current.Append(response.Message)

if len(response.Message.ToolCalls) == 0 {
	return response.Message.Content, nil
}
```

完整 assistant 响应必须在执行 Tool 前保存，因为下一轮模型需要看到自己之前发出的 Tool Call ID、Tool 名称和原始 arguments。

没有 Tool Call 是 ReAct 正常结束条件。

### 6.6 多 Tool 严格串行

```go
for _, call := range response.Message.ToolCalls {
	result, executeErr := loop.executor.Execute(
		ctx,
		current.ID,
		selected.Tools,
		call,
	)

	current.Append(llm.Message{
		Role:       llm.RoleTool,
		Content:    result,
		ToolCallID: call.ID,
		ToolName:   call.Function.Name,
	})

	if executeErr != nil {
		return "", executeErr
	}
}
```

这里没有 goroutine。一个 assistant 响应中的多个 Tool Call 按模型返回顺序逐个执行。

即使 Tool 失败，也会先把它返回的文本结果写入 Session，再把错误向上传播。AgentService 随后会保存失败状态，使调用现场可恢复、可查询。

### 6.7 迭代耗尽

模型在每一轮都继续请求 Tool 时，循环在达到 Profile 上限后返回稳定 sentinel：

```go
var ErrMaxIterations = errors.New(
	"react loop reached maximum iterations",
)
```

调用方可以使用：

```go
errors.Is(err, runtime.ErrMaxIterations)
```

## 7. ToolExecutor：受控 Tool 执行边界

### 7.1 OryxOS 自有 Tool 接口

```go
type InvokableTool interface {
	Info(ctx context.Context) (llm.ToolDefinition, error)
	Invoke(ctx context.Context, arguments string) (string, error)
}

type OryxTool struct {
	Tool       InvokableTool
	Retryable  bool
	Idempotent bool
	Timeout    time.Duration
}
```

`Info` 只提供 OryxOS Tool 元数据，不执行 Tool。`Invoke` 接收模型返回的原始 JSON arguments。

`OryxTool` 把执行逻辑与运行策略分离：

- `Retryable`：Tool 是否允许考虑重试；
- `Idempotent`：重复执行是否默认安全；
- `Timeout`：每次物理尝试的超时时间。

### 7.2 一次 Execute 是一次逻辑 Tool 调用

```go
func (executor *Executor) Execute(
	ctx context.Context,
	sessionID string,
	allowedTools []string,
	call llm.ToolCall,
) (string, error)
```

外层 `Execute` 负责：

1. 记录开始时间；
2. 调用内部 `execute`；
3. 计算整个逻辑调用耗时；
4. 脱敏最终错误；
5. 构造一条 `ToolInvocation`；
6. 使用 `context.WithoutCancel` 写入最终调用记录；
7. 合并 Tool 错误和审计错误。

重试发生在内部 `execute`，不单独写数据库。因此一次模型 Tool Call 无论物理执行多少次，都只产生一条 `tool_invocations`。

### 7.3 校验顺序

内部执行顺序固定：

```text
Tool name 非空
→ 当前 Profile allow-list
→ 精确查找 Tool
→ Tool 实现非空
→ Timeout 为正数
→ 读取 ToolDefinition
→ ToolDefinition.Name 与调用名一致
→ JSON Schema 参数校验
→ Invoke
```

Profile allow-list 校验：

```go
if !containsExact(allowedTools, toolName) {
	return "", fmt.Errorf(
		"execute tool: tool %q is not allowed by profile",
		toolName,
	)
}
```

即使全局 Registry 中注册了 `shell`，只要当前 Profile 没启用它，模型就不能调用。

### 7.4 JSON Schema 子集

参数验证支持：

- `object`；
- `array`；
- `string`；
- `number`；
- `integer`；
- `boolean`；
- `null`；
- `properties`；
- `required`；
- `additionalProperties: false`；
- `items`；
- `enum`。

例如：

```json
{
  "type": "object",
  "properties": {
    "city": {
      "type": "string"
    }
  },
  "required": ["city"],
  "additionalProperties": false
}
```

以下参数会在 `Invoke` 前失败：

```json
{}
```

```json
{"city": 123}
```

```json
{"city": "Shanghai", "unexpected": true}
```

不支持的 schema keyword 会 fail closed，而不是静默忽略。

### 7.5 超时和重试

每次物理尝试都有新的 timeout Context：

```go
attemptContext, cancel := context.WithTimeout(
	ctx,
	configured.Timeout,
)

result, err = configured.Tool.Invoke(
	attemptContext,
	arguments,
)

cancel()
```

只有同时满足以下条件才重试：

```text
OryxTool.Retryable == true
AND
OryxTool.Idempotent == true
AND
错误实现 Retryable() bool 并返回 true
```

```go
if attempt >= maxToolRetries ||
	!configured.Retryable ||
	!configured.Idempotent ||
	!isRetryable(err) {
	return result, fmt.Errorf(
		"execute tool %q: %w",
		toolName,
		err,
	)
}
```

`maxToolRetries=3` 表示首次尝试之外最多重试三次，即最多四次 `Invoke`。指数退避为 10ms、20ms、40ms，等待期间继续响应父 Context 的取消。

每次真正调用 `InvokableTool.Invoke` 都写一条结构化日志：

```text
event=tool_attempt
session_id=...
profile_name=...
channel=...
tool_name=...
attempt=1
final_attempt=false
success=false
duration_ms=...
error_message=已脱敏错误
```

重试后的最后一次成功或失败使用 `final_attempt=true`。日志只描述物理尝试，不记录 Tool
arguments 或 result；数据库仍然按一次逻辑 Tool Call 写一条最终 `tool_invocations`，两者不能
混为多条审计记录。

副作用 Tool 默认不得重试，例如：

- `write_file`；
- `shell`；
- `http_post`；
- `notify`；
- `save_memory`。

### 7.6 Sandbox 边界

第 17 节没有提前实现第 24 节的具体 Sandbox。后续具体 Tool 必须在外部副作用发生前调用相应的操作级 Sandbox 方法：

```text
文件 Tool → ValidatePath
Shell Tool → ValidateCommand
HTTP Tool → ValidateURL，并在重定向后再次验证
```

ToolExecutor 负责统一调度，但不能用一个含义模糊的通用 Sandbox 方法替代这些操作级校验。

### 7.7 Tool 调用记录

最终记录：

```go
invocation := &store.ToolInvocation{
	SessionID:  sessionID,
	ToolName:   toolName,
	InputJSON:  normalizedJSON(call.Function.Arguments),
	ResultJSON: marshalText(result),
	Success:    safeInvokeErr == nil,
	DurationMS: duration,
	CreatedAt:  finishedAt.UTC(),
}
```

记录使用取消脱离的 Context：

```go
recordErr := executor.recorder.Create(
	context.WithoutCancel(ctx),
	invocation,
)
```

错误优先级：

```text
Tool 成功 + 记录成功 → 返回 Tool result
Tool 失败 + 记录成功 → 返回 Tool result 和 Tool error
Tool 成功 + 记录失败 → 返回记录错误
Tool 失败 + 记录失败 → errors.Join 保留两条原因
```

## 8. Session 如何保持完整消息链

第 17 节提供最小内存 Session aggregate：

```go
type Session struct {
	ID string

	mu       sync.RWMutex
	messages []llm.Message
}
```

所有读取和追加都经过每个 Session 自己的锁：

```go
func (session *Session) Append(message llm.Message) {
	session.mu.Lock()
	defer session.mu.Unlock()
	session.messages = append(
		session.messages,
		cloneMessage(message),
	)
}
```

`Messages` 和 `RecentTurns` 返回深层防御性副本，避免调用方修改底层消息、ToolCalls、Index 指针或 `Extra` map。

一次 Tool 辅助请求最终应形成：

```text
user
assistant(tool call id=call-1)
tool(tool_call_id=call-1)
assistant(final answer)
```

完整 assistant Tool Call 和匹配 Tool message 必须同时保留。

## 9. 天气请求的完整执行示例

用户输入：

```text
上海今天天气怎么样，我该穿什么？
```

Profile：

```yaml
name: weather

identity:
  agent_name: Oryx Weather
  prompt: You are a weather assistant.

tools:
  - http_get

skills:
  - weather/SKILL.md

settings:
  max_iterations: 10
  max_history_turns: 20
```

### 9.1 入口

```go
AgentRequest{
	ProfileName: "weather",
	Channel:     "cli",
	UserID:      "gao",
	Message:     "上海今天天气怎么样，我该穿什么？",
}
```

AgentService 查找 `weather` Profile、解析 Session，然后调用 ReActLoop。

### 9.2 第一轮 Reason

ReActLoop 先追加用户消息：

```text
[0] user: 上海今天天气怎么样，我该穿什么？
```

PromptBuilder 组装 Runtime Rules、Bootstrap、Identity、Skill、Memory、历史、当前请求和时间。ProviderService 同时绑定当前 Profile 的 `http_get` ToolDefinition。

模型返回：

```text
assistant:
  content: 我先查询上海天气
  tool_calls:
    - id: call-weather-1
      name: http_get
      arguments: {"url":"https://weather.example/..."}
```

ReActLoop 先把完整 assistant 消息加入 Session。

### 9.3 Act

ToolExecutor：

```text
检查 http_get 在 Profile.Tools 中
→ 从 Tool lookup 精确查找
→ 校验 ToolDefinition.Name
→ 校验 URL arguments 的 JSON Schema
→ 创建超时 Context
→ 调用 http_get.Invoke
→ 写入一条 tool_invocations
```

返回：

```json
{"temperature": 18, "weather": "rain"}
```

### 9.4 Observe

ReActLoop 追加：

```text
tool:
  tool_call_id: call-weather-1
  tool_name: http_get
  content: {"temperature":18,"weather":"rain"}
```

此时 Session：

```text
[0] user
[1] assistant(tool_call=call-weather-1)
[2] tool(tool_call_id=call-weather-1)
```

### 9.5 第二轮 Reason

PromptBuilder 再次读取 Session，模型看到：

```text
用户问题
→ 自己发出的 Tool Call
→ 匹配的 Tool 结果
```

模型返回无 Tool Call 的最终回答：

```text
上海今天约 18℃，有雨。建议穿薄外套并携带雨伞；
你比较怕冷，可以再加一件轻薄针织衫。
```

ReActLoop 保存 assistant 最终消息并结束。AgentService 使用取消脱离的 Context 保存完整 Session。

调用记录应包含：

```text
llm_calls
  → 第一轮模型调用
  → 第二轮模型调用

tool_invocations
  → 一次 http_get 逻辑调用
```

## 10. 启动模式与加载范围

不是所有 CLI 命令都加载完整 Runtime。只有需要调用 `AgentService.Invoke` 的运行模式共享完整 Runtime Core。

| 模式/命令 | 加载范围 |
|---|---|
| `oryxos chat` | Runtime Core + CLI 交互入口 |
| `oryxos serve` | Runtime Core + Gin HTTP + Scheduler |
| `oryxos gateway` | Runtime Core + 常驻生命周期 + Scheduler |
| `oryxos init` | 只创建工作区，不加载 Runtime |
| `oryxos profile list/show` | 只加载 Profile |
| `oryxos provider list` | 实例配置和 Provider 状态 |
| `oryxos tool list` | Profile 和 ToolRegistry |
| `oryxos session list` | SessionStore |

三种 Agent 执行模式应共享一个公共 Runtime 构造过程：

```text
BuildRuntime
  ├─ Config / Profile
  ├─ Bootstrap / Skill Snapshot
  ├─ Provider Registry
  ├─ Tool Registry / Executor
  ├─ Memory / Session
  ├─ PromptBuilder
  ├─ ReActLoop
  └─ AgentService
```

然后按模式挂接外围入口：

```text
BuildRuntime → CLI Channel
BuildRuntime → Gin Handlers + Scheduler
BuildRuntime → Gateway + Scheduler
```

不要在 `chat`、`serve` 和 `gateway` 中复制 Profile、Bootstrap、Skill、Provider 或 ReAct 的加载组装逻辑。

## 11. 启动配置与运行状态的边界

启动时加载并形成快照：

```text
实例配置
Profile
Bootstrap
Skill
MCP Server 定义
Provider 模型绑定
Tool 注册信息
```

请求时读取或写入：

```text
Session
MEMORY.md
LLM 调用
Tool 调用
llm_calls
tool_invocations
```

因此最终边界是：

```text
Profile / Bootstrap / Skill
  → 启动时加载，重启生效

Session / Memory / 调用记录
  → 请求时读取或写入

CLI / HTTP / Gateway / Scheduler
  → 按模式启动，但共享 AgentService
```

## 12. 当前实现边界与后续接入

第 17 节已经完成四个组件的控制链、契约和确定性测试，但完整生产启动组装仍依赖后续模块：

```text
第 18 节
  → SessionService / SessionStore / Session 身份规则

第 20 节
  → ToolRegistry / 9 个内置 Tool / MCP Tool

第 22 节
  → MarkdownMemoryStore

第 24 节
  → 具体 Sandbox 校验
```

后续完整组装关系：

```text
ProfileRegistry
SessionService ───────────────┐
                              │
BootstrapLoader ─┐            │
SkillLoader ─────┼→ PromptBuilder
MemoryService ───┘       │
                         │
ProviderService ─────────┼→ ReActLoop
ToolRegistry ─→ ToolExecutor
                         │
                         ▼
                    AgentService
```

第 17 节可以理解为完成了 Agent 的大脑与神经中枢；后续章节把持久化 Session、具体 Tool、长期 Memory 和 Sandbox 接入同一条执行链。

## 13. 回归测试

相关测试：

| 测试文件 | 覆盖内容 |
|---|---|
| `internal/runtime/agent_service_test.go` | 三种调用形态、Profile 隔离、关联字段、成功/失败保存、错误合并 |
| `internal/runtime/prompt_builder_test.go` | 标签顺序、Memory 上限、当前轮去重、完整轮次与输入预算截断、时间注入 |
| `internal/runtime/react_loop_test.go` | 最终回答、单/多 Tool、第二轮消息、串行顺序、取消、迭代上限、审计失败 fail-closed |
| `internal/tool/executor_test.go` | 白名单、schema、真实超时、重试、逐尝试日志、最终审计、错误合并 |
| `internal/bootstrap/loader_test.go` | 默认顺序、显式引用、路径逃逸、不可变快照 |
| `internal/skill/loader_test.go` | Profile-only Skill、重复引用、路径逃逸、无自动发现 |
| `internal/session/session_test.go` | 并发安全、防御性副本、完整 Tool 消息组 |
| `internal/store/tool_invocation_repository_test.go` | 手写迁移、成功/失败记录、字段校验 |

修改本模块后至少运行：

```bash
gofmt -w <changed-go-files>
go test ./internal/runtime ./internal/tool ./internal/bootstrap ./internal/skill
go test -race ./internal/session
go test ./internal/provider ./internal/store
go test ./...
go vet ./...
CGO_ENABLED=0 go build ./cmd/oryxos
git diff --check
```

默认测试使用模型、Tool、Memory、Session 和时钟 double，不依赖真实 Provider 凭证或网络。
完整的人工执行步骤和依赖门禁见
[`docs/testing/react-runtime-manual-testing.md`](../testing/react-runtime-manual-testing.md)。

## 14. 优化二：审计存储健康熔断的具体实现方案（待实现）

### 14.1 目标与边界

目标是在 SQLite 已经明确不可写时，于付费模型调用之前快速失败，避免产生费用后才发现
`llm_calls` 无法落库。该方案不新增数据库表、REST 端点、CLI 命令或第三方依赖，也不把
完整审计治理提前到核心阶段。

它不能替代本节已经实现的调用后审计。健康状态只能减少已知故障期间的无效调用，无法
消除“健康检查刚通过、下一次写入立即失败”的竞争窗口。因此调用后的同步写入与
`AuditPersistenceError` 仍然必须保留。

### 14.2 状态与公开语义

在 `internal/store/audit_health.go` 增加一个进程内、并发安全的状态机：

```go
type AuditStoreState uint8

const (
	AuditStoreHealthy AuditStoreState = iota
	AuditStoreDegraded
	AuditStoreUnavailable
)

var ErrAuditUnavailable = errors.New("audit store is unavailable")

type AuditHealth interface {
	Allow() error
	ObserveWrite(error)
	Snapshot() AuditHealthSnapshot
}
```

状态含义：

| 状态 | `Allow()` | 含义 |
|---|---:|---|
| `healthy` | 通过 | 最近实际审计写入成功 |
| `degraded` | 通过 | 出现短暂锁竞争等瞬时错误，但尚不能断定存储不可用 |
| `unavailable` | 拒绝 | 已出现确定性不可写错误，或连续瞬时失败达到阈值 |

建议初始状态为 `healthy`，因为应用启动已经成功打开数据库并执行迁移。快照至少包含
`state`、`consecutive_failures`、`last_error_kind`、`changed_at`，不保存原始错误文本和
凭证。

### 14.3 状态转换

`LlmCallRepository.Create` 的真实写入结果驱动状态，不在每次模型调用前额外访问数据库：

```text
任意状态 --实际写成功----------------------------> healthy
healthy   --一次 SQLITE_BUSY/LOCKED--------------> degraded
degraded  --实际写成功----------------------------> healthy
degraded  --30 秒内连续 3 次瞬时写失败-----------> unavailable
任意状态 --READONLY/FULL/IOERR/CORRUPT/CANTOPEN--> unavailable
unavailable --周期性可写探测成功------------------> healthy
```

错误分类留在 `internal/store`。Repository 把 SQLite 错误码归一化成 `transient`、
`unavailable` 或 `request`；字段校验等请求错误不能污染存储健康状态。不要在 Provider、
Runtime 中匹配 SQLite 错误字符串。

### 14.4 接入 Provider 调用链

为 `ProviderService` 注入窄门禁接口，而不是让它读取全局变量：

```go
type auditGate interface {
	Allow() error
}
```

调用顺序调整为：

```text
校验请求/Profile
→ auditGate.Allow()
→ 解析模型与 Tool schema
→ 调用模型
→ 写 llm_calls
→ 把真实写入结果反馈给 AuditHealth
```

当状态为 `unavailable` 时，`Chat` 返回 `ErrAuditUnavailable` 和空响应；不会调用模型，
也不会进入 ReAct Tool 分支。`degraded` 仍允许调用，因为它表示短暂锁竞争而非已经确认
不可写。若希望更保守，可以把阈值设为 1，但不要改变三个状态的语义。

建议用 Recorder 装饰器反馈结果：

```go
type HealthAwareLlmCallRecorder struct {
	next   LlmCallRecorder
	health AuditHealth
}

func (r *HealthAwareLlmCallRecorder) Create(ctx context.Context, call *store.LlmCall) error {
	err := r.next.Create(ctx, call)
	r.health.ObserveWrite(err)
	return err
}
```

后续 `tool_invocations` 也应使用同一个健康实例和同类装饰器，使 `audit_store` 表示整个审计
存储，而不是只表示 `llm_calls`。不过模型调用前门禁仍放在 ProviderService，因为费用在
这里产生。

### 14.5 恢复探测

`unavailable` 状态由应用生命周期托管的单个恢复组件探测，不允许每个请求创建 goroutine。
建议新增 `internal/app/audit_recovery.go`，实现已有 `app.Component`：

- 只在 `unavailable` 时运行探测；`healthy/degraded` 时休眠；
- 退避间隔使用 1s、2s、5s、10s、30s 上限，并加入少量 jitter；
- 每次探测使用独立的 1 秒 Context；
- 应用关闭时通过根 Context 停止，并由 `Close` 等待 worker 退出；
- 探测成功一次即恢复 `healthy`，失败只更新脱敏后的错误类别和下次探测时间。

探测必须验证“主数据库的目标表可写”，单纯 `SELECT 1` 不够。可在短事务中向
`llm_calls` 插入一条合法探测记录，然后显式回滚；事务成功写入且回滚成功才算恢复。探测
记录不会留下，三张核心表和业务审计口径均不改变。不能用临时表，因为临时数据库可写
并不代表主数据库可写。

### 14.6 可观测性和入口行为

将状态快照接入现有 `observability.Snapshot`。第 26 节实现既有 `/api/v1/health` 时，在同一
端点返回 `audit_store`，不增加新端点：

```json
{
  "status": "degraded",
  "audit_store": "unavailable"
}
```

`serve/gateway` 在审计不可用时保持进程存活以便自动恢复，但 readiness 应为 false；CLI、
Web 和 Scheduler 通过共享 `AgentService` 收到同一个 `ErrAuditUnavailable`。Scheduler
应记录本次触发失败，不在健康门禁之外自行重试模型调用。

### 14.7 建议文件与测试

| 文件 | 改动 |
|---|---|
| `internal/store/audit_health.go` | 状态机、错误分类、快照 |
| `internal/store/audit_health_test.go` | 并发、阈值、硬错误、恢复状态转换 |
| `internal/store/llm_call_repository.go` | 反馈真实写入结果或由装饰器包装 |
| `internal/store/sqlite_audit_probe.go` | 短事务写入并回滚的可写探测 |
| `internal/provider/service.go` | 模型调用前执行健康门禁 |
| `internal/provider/provider_service_test.go` | unavailable 时模型调用次数为 0 |
| `internal/app/audit_recovery.go` | 单 worker、退避、关闭等待 |
| `internal/app/audit_recovery_test.go` | 假时钟下恢复、退避、无 goroutine 泄漏 |
| `internal/observability/observer.go` | 暴露脱敏后的审计健康快照 |

验收重点：

1. `unavailable` 时连续请求都不调用模型，也不逐请求探测数据库；
2. `degraded` 状态仍允许调用，一次真实写成功立即回到 `healthy`；
3. 硬错误立即熔断，瞬时错误只在阈值达到后熔断；
4. 只有一个恢复 worker，退避和 shutdown 可由假时钟确定性测试；
5. 探测事务不留下 `llm_calls` 行；
6. 恢复后下一次请求重新允许模型调用；
7. 三张表、10 个端点、12 个 CLI 命令和纯 Go SQLite 约束保持不变。
