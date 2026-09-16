# Provider 开发指南

本文面向需要维护 OryxOS Provider 层、接入新模型厂商或编写新模型适配器的开发者。内容以当前仓库实现为准，重点说明边界、调用链、配置合并、类型转换、调用记录、扩展步骤和回归要求。

Provider 层的目标不是重新实现厂商 HTTP SDK，而是把外部 connector 稳定地适配为 OryxOS 自有的窄模型端口，并把协议差异限制在 `internal/provider` 内。

## 1. 先理解三个概念

### 1.1 OryxOS 模型端口

`internal/llm` 是 Runtime 与 Provider 之间的稳定边界。它由 OryxOS 自己定义，不依赖 Eino：

```go
type ChatModel interface {
	Generate(ctx context.Context, request Request) (Response, error)
}

type Request struct {
	Messages []Message
	Tools    []ToolDefinition
}

type Response struct {
	Message      Message
	Usage        Usage
	FinishReason string
}
```

这个接口刻意保持同步和窄小。Runtime 只需要知道：给定消息与 Tool 定义，模型生成一条响应。它不需要知道厂商 SDK、HTTP endpoint、Eino `schema.Message` 或 connector 配置。

### 1.2 Provider

Provider 是一个明确命名的模型厂商接入策略，例如 `deepseek` 或 `minimax`。一个 Provider 工厂负责封装：

- 选择哪个 Eino-ext connector；
- 使用原生协议还是 OpenAI 兼容协议；
- 官方 endpoint 策略；
- 厂商配置到 connector 配置的映射；
- connector 到 `llm.ChatModel` 的包装。

明确命名的 Provider 不是一个让用户任意填写 endpoint 的通用 HTTP 客户端。用户只提供 API key，模型和 temperature 由 Profile 选择，connector 与 endpoint 由代码中的工厂确定。

### 1.3 Adapter

Adapter 负责翻译两个接口世界。当前的 `einoChatModelAdapter` 把 Eino core 的 `model.ToolCallingChatModel` 适配为 OryxOS 的 `llm.ChatModel`，并完成以下双向转换：

- OryxOS Message ↔ Eino Message；
- OryxOS Tool Definition → Eino ToolInfo；
- Eino Tool Call → OryxOS Tool Call；
- Eino Usage / FinishReason → OryxOS Response。

只要新的 connector 实现 `model.ToolCallingChatModel`，通常就应复用这个通用 adapter，而不是为每个厂商复制一份消息转换代码。

## 2. 架构与依赖边界

当前调用关系如下：

```text
AgentService / ReActLoop（后续课程）
  -> Provider 调用边界 provider.Service
  -> provider.Registry 按 Profile.name 取模型实例
  -> OryxOS llm.ChatModel（实际实现为 einoChatModelAdapter）
  -> Eino model.ToolCallingChatModel
  -> Eino-ext 厂商 connector
  -> Provider API
```

依赖方向必须保持为：

```text
runtime / tool / handler / scheduler
  -> internal/llm

internal/provider
  -> internal/llm
  -> Eino core / Eino-ext

Eino core / Eino-ext
  -X-> runtime / tool / handler / scheduler
```

仓库用架构测试保护这个边界：

- `internal/app/architecture_test.go` 检查 Eino 导入只出现在 `internal/provider`；
- `cmd/oryxos/architecture_test.go` 限制 Eino-ext 依赖只能位于 Provider 边缘，并禁止 Eino ADK。

禁止以下做法：

- 在 Runtime 保存 `*schema.Message`；
- 让 Tool 返回 `*schema.ToolInfo`；
- 在 Handler 中构造 Eino connector；
- 用 Eino ADK 代替 OryxOS 自己的 ReAct Loop；
- 让 Provider adapter 执行 Tool。

## 3. 当前目录与职责

| 文件 | 职责 |
|---|---|
| `internal/llm/model.go` | OryxOS 自有模型端口与领域类型 |
| `internal/provider/config.go` | Provider 名称、工厂合并配置、固定 endpoint 常量 |
| `internal/provider/factory.go` | DeepSeek/MiniMax connector 构造与工厂注册表 |
| `internal/provider/eino_adapter.go` | OryxOS 与 Eino 之间的类型转换 |
| `internal/provider/registry.go` | 工厂按 Provider 名路由，模型按 Profile 名隔离 |
| `internal/provider/tool_schema_resolver.go` | 只解析 Tool 元数据，不执行 Tool |
| `internal/provider/service.go` | 单次同步调用、耗时统计、错误脱敏与 `llm_calls` 写入 |
| `internal/store/llm_call*.go` | `llm_calls` 数据模型和短事务写入 |
| `internal/provider/*_test.go` | 工厂、适配、隔离、审计和真实连通性测试 |

## 4. 配置如何进入模型实例

Provider 配置分成两层。

实例级启动配置只声明启用哪些 Provider 及其凭证：

```yaml
providers:
  - name: deepseek
    api_key: ${DEEPSEEK_API_KEY}
  - name: minimax
    api_key: ${MINIMAX_API_KEY}
```

Profile 只选择本 Agent 的模型参数：

```yaml
name: daily-report

provider:
  name: minimax
  model: MiniMax-M3
  temperature: 0.3
```

两层在绑定 Profile 时合并为临时的工厂输入：

```go
type ProviderConfig struct {
	Name        string
	Model       string
	APIKey      string
	Temperature float32
}
```

`ProviderConfig` 不是 YAML 结构，也不会持久化。它只在创建某个 Profile 的模型实例时短暂存在。

关键约束：

- 实例配置中没有 `model`、`temperature` 或 `base_url`；
- Profile 中没有 `api_key` 或 `base_url`；
- 严格 YAML 解析会拒绝未知字段；
- 缺失环境变量会失败，但错误不得包含凭证值；
- 明确 Provider 的 endpoint 不能由部署者覆盖。

配置流如下：

```text
启动 YAML
  -> 展开 ${ENV_VAR}
  -> config.ProviderDefinition{name, api_key}

Profile YAML
  -> profile.ProviderConfig{name, model, temperature}

Registry.BindProfiles
  -> 按 provider.name 匹配两层配置
  -> 调用对应 ModelFactory
  -> 按 profile.name 保存 llm.ChatModel
```

## 5. 当前两个 Provider 的实现

### 5.1 DeepSeek：原生 connector

DeepSeek 使用 Eino-ext DeepSeek connector：

```go
DeepSeek: func(ctx context.Context, cfg ProviderConfig) (llm.ChatModel, error) {
	connector, err := constructors.deepseek(ctx, &deepseekmodel.ChatModelConfig{
		APIKey:      cfg.APIKey,
		Model:       cfg.Model,
		Temperature: cfg.Temperature,
	})
	return wrapEinoChatModel(connector, err)
},
```

这里不传 `BaseURL`，也不定义 DeepSeek URL 常量。原因是原生 connector 已包含官方默认地址，OryxOS 重复设置只会制造两个端点事实来源。

### 5.2 MiniMax：OpenAI 兼容 connector

MiniMax 使用 Eino-ext OpenAI connector，但仍然是明确命名的 `minimax` Provider：

```go
const miniMaxOpenAIBaseURL = "https://api.minimax.cn/v1"

MiniMax: func(ctx context.Context, cfg ProviderConfig) (llm.ChatModel, error) {
	temperature := cfg.Temperature
	connector, err := constructors.openai(ctx, &openaimodel.ChatModelConfig{
		APIKey:      cfg.APIKey,
		Model:       cfg.Model,
		BaseURL:     miniMaxOpenAIBaseURL,
		Temperature: &temperature,
	})
	return wrapEinoChatModel(connector, err)
},
```

OpenAI connector 只是协议实现，不等于 Provider 名必须叫 `openai`。`minimax` 工厂把“使用 OpenAI 兼容协议”和“固定 MiniMax 官方 endpoint”一起封装起来，用户仍然只提供 `MINIMAX_API_KEY`。

### 5.3 为什么工厂构造器可以替换

`connectorConstructors` 把生产构造函数保存为函数字段：

```go
type connectorConstructors struct {
	deepseek func(context.Context, *deepseekmodel.ChatModelConfig) (model.ToolCallingChatModel, error)
	openai   func(context.Context, *openaimodel.ChatModelConfig) (model.ToolCallingChatModel, error)
}
```

它不是用户可配置的插件系统，而是一个测试缝。单元测试可以捕获传给 connector 的配置，无需发出真实网络请求，从而精确断言：

- DeepSeek 没有被设置 `BaseURL`；
- MiniMax 使用固定 URL；
- API key、model、temperature 被正确映射；
- 工厂最终返回的是 OryxOS `llm.ChatModel`。

## 6. Eino adapter 的完整转换过程

`einoChatModelAdapter.Generate` 按固定顺序工作：

1. 检查 adapter 和 `context.Context`；
2. 把所有 OryxOS Message 转为 Eino Message；
3. 把所有 OryxOS Tool Definition 转为 Eino ToolInfo；
4. 有 Tool 时调用 `WithTools` 取得绑定 schema 的 connector；
5. 调用一次 `Generate`；
6. 拒绝空响应；
7. 把 Eino 响应转换为 OryxOS Response。

### 6.1 Message 字段必须完整保留

四种角色必须一一映射：

| OryxOS | Eino |
|---|---|
| `llm.RoleSystem` | `schema.System` |
| `llm.RoleUser` | `schema.User` |
| `llm.RoleAssistant` | `schema.Assistant` |
| `llm.RoleTool` | `schema.Tool` |

以下字段不能只保留一部分：

- `Content`；
- `Name`；
- `ReasoningContent`；
- assistant 的全部 `ToolCalls`；
- Tool message 的 `ToolCallID` 和 `ToolName`；
- `Extra`；
- Tool Call 的 `Index`、`ID`、`Type`、function name、原始 JSON arguments 和 `Extra`。

其中 Tool Call ID 是多轮 Function Calling 的关联键。丢失 ID 会导致模型无法把 Tool 返回值与先前调用对应起来。

未知角色必须在调用 connector 前报错，不能猜测或降级成 user message。

### 6.2 Tool Definition 只包含元数据

OryxOS 使用标准 JSON Schema：

```go
type ToolDefinition struct {
	Name        string
	Description string
	InputSchema json.RawMessage
	Extra       map[string]any
}
```

Adapter 将 `InputSchema` 解析成 Eino JSON Schema，再创建 `schema.ToolInfo`。Tool 名为空或 JSON Schema 非法时必须在 connector 调用前失败。

这里不包含执行函数。模型只收到 Tool 的名字、说明和参数 schema；真正的 Tool 查找、参数校验、Sandbox、执行和结果记录由后续的 `ReActLoop + ToolExecutor` 完成。

### 6.3 Response 元数据

如果 connector 返回 `ResponseMeta`，adapter 会保留：

- `FinishReason`；
- prompt token；
- completion token；
- total token。

Provider 未返回 usage 时使用零值。零值表示“不可用或未提供”，不能伪造 token 数量。

### 6.4 当前只暴露同步 Generate

Eino connector 可能实现 Stream，但 OryxOS 当前 `llm.ChatModel` 只暴露同步 `Generate`。不要因为 connector 支持 Stream，就把 Eino Stream 类型向 Runtime 或 Web 层透传。SSE、WebSocket 和流式领域端口都不在当前核心范围内。

## 7. Registry：工厂按厂商，实例按 Profile

Registry 有两张表：

```go
type Registry struct {
	factories map[string]ModelFactory  // key: provider.name
	models    map[string]llm.ChatModel // key: profile.name
}
```

两种 key 不能混用：

- `factories["deepseek"]` 表示如何构造 DeepSeek 模型；
- `models["daily-report"]` 表示 `daily-report` Profile 专属的模型实例。

假设两个 Profile 都选择 DeepSeek：

```yaml
# ops.yaml
name: ops
provider:
  name: deepseek
  model: deepseek-chat
  temperature: 0.2
```

```yaml
# reasoning.yaml
name: reasoning
provider:
  name: deepseek
  model: deepseek-reasoner
  temperature: 0.8
```

Registry 必须构造两个实例，不能用 `models["deepseek"]` 让它们共享模型和 temperature。

`BindProfiles` 会先在临时 map 中构造完整的新快照。只要任一 Profile 绑定失败，旧的 `models` 快照保持不变；全部成功后才一次性替换。这保证注册表不会暴露半更新状态。

## 8. Provider Service：调用与审计边界

`provider.Service.Chat` 表示一次同步模型调用，不表示完整 ReAct 轮次。它负责：

1. 校验 Session ID 和 Profile；
2. 按 `Profile.name` 获取模型；
3. 按 Profile 中的 Tool 名解析 OryxOS Tool Definition；
4. 调用一次 `llm.ChatModel.Generate`；
5. 计算耗时；
6. 对上游错误脱敏；
7. 成功或失败都写一条 `llm_calls`；
8. 返回 OryxOS Response 或脱敏后的错误。

只有真正发起的模型调用才产生 `llm_calls`。输入非法、Profile 未绑定或 Tool schema 解析失败发生在调用之前，不应伪造成 Provider 调用记录。

模型调用使用原始 `ctx`，因此取消与超时会传到 connector。调用记录使用 `context.WithoutCancel(ctx)`，这样模型因取消失败后仍会尝试写入失败记录，同时保留原 context 中的值。

如果模型调用成功但审计写入失败，Service 仍然返回错误，因为“调用记录 Day One 写入”是核心正确性要求，不能静默吞掉持久化失败。

错误通过 `config.SanitizeErrorString` 脱敏后才返回和落库。新增 connector 时必须补充包含其真实错误格式的脱敏测试；不要把 Authorization Header、API key 或含凭证的 URL 写入错误。

## 9. 如何新增一个 Provider

### 9.1 先确认范围

当前核心阶段固定只交付 DeepSeek 与 MiniMax。新增第三个 Provider 属于范围变更，编码前应先按事实优先级更新并确认：

1. `docs/DemandAnalysis.md`：为什么需要该 Provider、是否进入当前阶段；
2. `docs/TechnicalSolution.md`：connector、协议、endpoint、模型能力与验证策略；
3. 根目录 `AGENTS.md`：若长期架构边界发生变化，再同步项目指南；
4. 对应课程 spec/plan/tasks：若该变更属于课程需求。

不要只改 `factory.go`，否则代码会与“核心阶段固定两个 Provider”的验收口径冲突。

### 9.2 判断应复用哪个 adapter

按以下顺序决策：

| 条件 | 做法 |
|---|---|
| Eino-ext 有原生 `ToolCallingChatModel` connector | 使用原生 connector，复用 `einoChatModelAdapter` |
| 厂商提供官方 OpenAI 兼容 API | 使用 Eino-ext OpenAI connector，建立明确 Provider 工厂并固定官方 endpoint |
| connector 不支持 Tool Calling | 当前核心链路不能接入；先解决 connector 能力，不得假装支持 |
| 协议字段需要额外归一化 | 优先在 Provider 内增加薄包装，再复用公共转换；避免复制整个 adapter |
| 必须绕过 Eino 或自写 HTTP 协议 | 属于架构变化，先更新需求和技术方案，不应直接实现 |

### 9.3 原生 Eino-ext connector 模板

下面是结构模板，其中 `vendor`、包名和字段需要按锁定版本的本地源码核验，不能直接复制后假定可编译：

```go
const Vendor = "vendor"

func vendorFactory(
	constructor func(context.Context, *vendormodel.ChatModelConfig) (model.ToolCallingChatModel, error),
) ModelFactory {
	return func(ctx context.Context, cfg ProviderConfig) (llm.ChatModel, error) {
		connector, err := constructor(ctx, &vendormodel.ChatModelConfig{
			APIKey:      cfg.APIKey,
			Model:       cfg.Model,
			Temperature: cfg.Temperature,
		})
		return wrapEinoChatModel(connector, err)
	}
}
```

如果原生 connector 已有正确的官方默认地址，不要再增加 `vendorBaseURL` 常量。

### 9.4 OpenAI 兼容 Provider 模板

明确厂商使用 OpenAI 兼容 API 时，为它建立独立 Provider 名和固定 endpoint：

```go
const (
	Vendor                    = "vendor"
	vendorOpenAICompatibleURL = "https://api.vendor.example/v1"
)

factories[Vendor] = func(ctx context.Context, cfg ProviderConfig) (llm.ChatModel, error) {
	temperature := cfg.Temperature
	connector, err := constructors.openai(ctx, &openaimodel.ChatModelConfig{
		APIKey:      cfg.APIKey,
		Model:       cfg.Model,
		BaseURL:     vendorOpenAICompatibleURL,
		Temperature: &temperature,
	})
	return wrapEinoChatModel(connector, err)
}
```

不要为了复用 OpenAI connector 就要求用户设置 `OPENAI_BASE_URL`，也不要给 `ProviderDefinition` 增加 `base_url`。`vendor` 是明确 Provider，它的 endpoint 是工厂策略。

如果未来确实要支持任意自建 OpenAI 兼容服务，应把它设计成另一个明确的通用 Provider 类型，并单独定义其安全配置、endpoint 校验和信任边界；不能借用 `minimax` 或其他厂商名绕过现有规则。

### 9.5 代码修改清单

经过范围确认后，通常需要修改：

1. `internal/provider/config.go`
   - 增加 Provider 名常量；
   - 仅在 OpenAI 兼容路径需要时增加固定 URL 常量。
2. `internal/provider/factory.go`
   - 引入锁定版本的 Eino-ext connector；
   - 增加可替换 constructor；
   - 注册 `ModelFactory` 并包装为 `llm.ChatModel`。
3. `internal/config/load.go`
   - 把新名字加入受支持 Provider 白名单；
   - 仍只接受 `name/api_key`。
4. 测试文件
   - 工厂配置映射；
   - Registry 路由和 Profile 隔离；
   - connector 错误与凭证脱敏；
   - Tool Calling 和 Tool Call ID；
   - 真实 Key 冒烟测试。
5. 文档
   - 更新需求、技术方案、项目指南和手动测试矩阵；
   - 写明测试模型 ID、环境变量名、账号区域和 endpoint 策略。
6. `go.mod/go.sum`
   - 固定经过回归验证的明确版本；
   - 不使用浮动分支或未固定伪版本作为发布基线。

## 10. 如何新增或修改 adapter

### 10.1 大多数 Provider 不需要新 adapter

如果 connector 已实现 Eino `model.ToolCallingChatModel`，新 Provider 的正常工作量应集中在工厂和测试。复制 `eino_adapter.go` 会造成角色映射、Tool schema、usage 和错误处理逐渐漂移。

只有出现可验证的语义差异时才考虑专用 adapter，例如：

- connector 返回的 Tool Call 字段需要额外归一化；
- 厂商把 reasoning content 放在扩展字段中；
- usage 字段需要按厂商响应补全；
- connector 的标准实现无法保留多轮 Tool Call ID。

即使需要专用处理，对外也仍然实现相同的 `llm.ChatModel`。

### 10.2 adapter 的最小契约

任何新 adapter 都必须满足：

```go
var _ llm.ChatModel = (*vendorAdapter)(nil)

func (adapter *vendorAdapter) Generate(
	ctx context.Context,
	request llm.Request,
) (llm.Response, error) {
	// 1. 检查 adapter 与 ctx
	// 2. 完整转换 Messages
	// 3. 完整转换 Tool Definitions
	// 4. 只调用模型，不执行 Tool
	// 5. 完整保留 Tool Calls、Usage、FinishReason
	// 6. 对 nil 或畸形响应显式报错
}
```

adapter 应放在 `internal/provider`，不要放入 `internal/llm`。`internal/llm` 只定义稳定的领域契约，不依赖任何实现库。

### 10.3 转换检查表

实现或评审 adapter 时逐项检查：

- [ ] system/user/assistant/tool 四种角色都有显式映射；
- [ ] 未知角色返回错误；
- [ ] assistant message 保留全部 Tool Calls，而不是只保留第一条；
- [ ] Tool Call 保留 ID、type、function name 和原始 arguments；
- [ ] Tool result 保留 `tool_call_id` 和 tool name；
- [ ] 多轮历史中的 assistant Tool Call 原消息不被压平成文本；
- [ ] Tool JSON Schema 非法时在网络调用前失败；
- [ ] Tool 顺序与 Profile 请求顺序一致；
- [ ] connector 不会在 adapter 内自动执行 Tool；
- [ ] usage 缺失时返回零值，不伪造数据；
- [ ] finish reason 被保留；
- [ ] nil response 和未初始化 adapter 显式失败；
- [ ] context 原样传到 connector；
- [ ] Eino 或厂商 SDK 类型没有越过 `internal/provider`。

## 11. 测试策略

### 11.1 工厂单元测试

通过替换 constructor 捕获实际 connector 配置，至少断言：

- API key、model、temperature 正确；
- 原生 connector 不被重复配置 endpoint；
- OpenAI 兼容 Provider 使用自己的固定 endpoint；
- connector 构造错误被返回；
- nil connector 不会产生可用 adapter；
- `ProviderConfig` 没有悄悄增加 endpoint 字段。

### 11.2 adapter 单元测试

使用实现 `model.ToolCallingChatModel` 的 fake，至少覆盖：

- 四种 Message 角色；
- assistant Tool Call 和 Tool result 的关联 ID；
- 多个 Tool Call 的顺序；
- JSON Schema 转换；
- response content、reasoning、usage、finish reason；
- 未知角色、空 Tool 名、非法 schema；
- `WithTools` 失败、`Generate` 失败和 nil response；
- 无 Tool 时不调用 `WithTools`。

### 11.3 Registry 单元测试

至少覆盖：

- Provider 名正确选中工厂；
- 同一 Provider 的两个 Profile 得到两个独立实例；
- 不同 Profile 的 model/temperature 不串值；
- 重复 factory 和重复 Profile 绑定被拒绝；
- 未声明 Provider、缺失凭证和未知 factory 被拒绝；
- `BindProfiles` 失败时旧快照不变；
- 成功重建后已移除的 Profile 不残留。

### 11.4 Service 与审计测试

至少覆盖：

- 一次 `Chat` 只调用一次 `Generate`；
- Tool resolver 只提供元数据且保持顺序；
- Provider Service 返回 Tool Calls，但不执行 Tool；
- 成功调用写 token、耗时、Provider、model 和 Session；
- 上游失败写 `success=false` 和脱敏错误；
- usage 缺失写 0；
- 已取消模型调用仍尝试写审计；
- 审计写入失败不会被吞掉；
- 输入校验失败不会误记为模型调用。

### 11.5 真实 Provider 冒烟测试

真实 Key 只通过环境变量注入。当前命令和预期结果见 [`docs/testing/provider-manual-testing.md`](../testing/provider-manual-testing.md)。新增 Provider 后应给 `TestProviderSmoke` 增加独立子测试，并固定一个已确认支持 Tool Calling 的模型 ID。

冒烟测试必须验证：

- 测试实际发起请求，不能因缺少变量而把 `SKIP` 当 `PASS`；
- 路由到预期 Provider 和模型；
- 响应含非空 Tool Call ID；
- `llm_calls` 中有一条对应 Session 的成功记录；
- 输出不包含真实凭证。

## 12. 版本升级流程

当前锁定版本以 `go.mod` 为准：

- Eino core `v0.9.19`；
- Eino-ext DeepSeek `v0.1.7`；
- Eino-ext OpenAI `v0.1.13`；
- Eino JSON Schema bridge `v1.0.3`。

Eino core 仍是 `0.x`，升级可能包含不兼容变化。升级 connector 时不要只看编译是否通过，应执行：

1. 查看目标版本的配置结构、默认 endpoint 和 Tool Calling 变更；
2. 更新固定版本；
3. 运行 factory 与 adapter 单元测试；
4. 运行两个真实 Provider 的 Tool Calling 冒烟测试；
5. 回归 assistant Tool Call + Tool result 的多轮消息；
6. 检查 token usage 和 finish reason；
7. 确认错误脱敏覆盖新错误格式；
8. 再运行全仓质量门禁。

不要使用“latest”作为长期依赖声明。所谓稳定版本，是项目已固定并通过上述回归的明确版本，不是包管理器当时解析到的任意最新版本。

## 13. 质量门禁

修改 Go 代码后，在仓库根目录至少执行：

```bash
gofmt -w <本次修改的 Go 文件>
go test ./...
go vet ./...
CGO_ENABLED=0 go build ./cmd/oryxos
```

Provider 相关的聚焦回归可以先执行：

```bash
go test ./internal/config ./internal/profile ./internal/provider ./internal/store -count=1
```

检查依赖边界：

```bash
go test ./internal/app ./cmd/oryxos -run Architecture -count=1
```

检查是否有 Eino 类型扩散：

```bash
rg 'github.com/cloudwego/eino' internal cmd \
  --glob '*.go' \
  --glob '!internal/provider/**'
```

最后一个命令允许命中架构测试中的规则字符串，但不应命中其他生产包的 import。

真实连通测试需要人工准备 Key，不属于默认 `go test ./...`：

```bash
go test -tags=integration ./internal/provider \
  -run '^TestProviderSmoke$' \
  -count=1 -v
```

## 14. 常见错误

| 错误做法 | 后果 | 正确做法 |
|---|---|---|
| 在配置中加入 `base_url` | 用户可绕开明确 Provider 的 endpoint 策略，配置源漂移 | 固定在对应工厂；原生 connector 有默认值时不重复配置 |
| 把 OpenAI connector 当成 Provider 名 | 协议实现与厂商身份混淆 | 为 MiniMax 等厂商建立明确工厂 |
| 按 Provider 名缓存模型 | 同厂商不同 Profile 串模型参数 | 工厂按 Provider，模型按 Profile |
| 每个厂商复制一份 Eino adapter | 转换逻辑漂移，修复无法共享 | connector 满足接口时复用通用 adapter |
| 只转换文本内容 | Tool Call ID、Tool result 关联或 reasoning 丢失 | 完整映射所有消息和 Tool Call 字段 |
| Provider Service 执行 Tool | ReAct 控制权分裂，可能重复执行 | 只返回 Tool Call，由 ToolExecutor 串行执行 |
| 错误直接 `%w` 返回 | 可能泄漏 API key 或带凭证 URL | 在用户可见和持久化边界先脱敏 |
| Provider 失败后自动切换 | 超出核心范围，调用行为不可预测 | 当前直接返回错误；fallback 属于扩展阶段 |
| 因 connector 支持 Stream 就提供 SSE | connector 能力与产品 API 承诺混淆 | 当前保持同步 JSON 与同步模型端口 |
| 只跑 fake 测试 | 无法发现账号区域、模型 ID、协议兼容问题 | 单元测试之外再跑真实 Tool Calling 冒烟测试 |

## 15. 提交前检查清单

- [ ] 需求范围允许新增或修改该 Provider；
- [ ] `internal/llm` 没有引入 Eino 或厂商 SDK 类型；
- [ ] 新 connector 与 endpoint 策略只存在于 `internal/provider`；
- [ ] 用户配置仍只提供 API key，Profile 仍只选 name/model/temperature；
- [ ] 工厂按 Provider 名注册，实例按 Profile 名保存；
- [ ] adapter 完整保留 Message、Tool Call、Usage 和 FinishReason；
- [ ] Provider 层不执行 Tool；
- [ ] 每次实际模型调用都有成功或失败的 `llm_calls` 写入；
- [ ] 错误和测试输出不泄漏凭证；
- [ ] connector 版本已固定；
- [ ] 单元测试、架构测试、vet 和无 CGO 构建通过；
- [ ] 真实 Provider 冒烟测试使用明确模型 ID 并验证 Tool Call ID；
- [ ] 需求文档、技术方案、项目指南和测试文档与代码一致。
