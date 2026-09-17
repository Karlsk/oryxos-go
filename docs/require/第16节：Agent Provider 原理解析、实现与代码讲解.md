# Agent Provider：原理解析、实现与代码讲解

OryxOS 的第一块核心能力是 Provider，也就是对接大模型（LLM）的那一层。这节讲四件事：Provider 是什么、动手前该想清楚什么、同步与流式调用怎么写、做完怎么验。

技术栈是 Go 1.26 + OryxOS 自有 `llm.ChatModel` 端口 + Eino core/Eino-ext Provider 适配实现。下面的 connector 代码是示意，Eino API 以锁定版本的本地源码为准。

---

## 一、Provider 是什么，干嘛用的

一句话：**Provider 就是 Agent 和大模型之间的一个前台。**

上层要跟大模型说话，但它不想操心"这次到底是调 DeepSeek 还是 MiniMax、每家的接口格式还不一样"。于是这些事全交给 Provider：上层把要说的话递进去，Provider 负责挑对模型、用对方听得懂的格式发出去、再把回话拿回来。

放到 Agent 的整体里看会更清楚。我们说 Agent = LLM + Tools + Memory + Loop + Environment，其中 **LLM 是那个做决策的大脑**。Provider 就是把这个大脑接进系统的工程封装——ReAct 循环每转一圈，都要通过 Provider 调一次大模型。

具体怎么工作：上层传两样东西进来，一个 **Profile**（一份配置，写了这次用哪个 provider、哪个 model），一段由 OryxOS `llm.Message` 组成的 **Prompt**。Provider 按 Profile 挑出对应的模型：同步调用一次返回完整的 OryxOS `llm.Response`，流式调用则通过 OryxOS 自有的 reader 逐块返回，结束前再给出完整响应。好处是：以后想换模型或 connector，ReAct 和 Tool 都不用跟着更换第三方类型。

![Provider 是什么：上层 ReAct 循环通过 ProviderService 调用各家 LLM](../../website/public/images/class-16-1.svg)

图里 OryxOS `llm.ChatModel` 是 Runtime 能看到的唯一模型接口。Eino `model.ToolCallingChatModel` 被包在 Provider 适配器里，不出现在 ReAct、Tool 或 Handler 的签名中。

这里还有一个容易搞混的点：**工具（Tool）这件事，Provider 只做翻译，不做执行。**

大模型能"调用工具"（业界叫 Function Calling），意思是：我们在发请求时，顺带告诉模型"你手上有哪些工具能用、每个工具要传什么参数"——这份工具说明在 OryxOS 中叫 `llm.ToolDefinition`。Provider 适配器才把它转为 Eino `schema.ToolInfo`。模型可能回复"我想调 `http_get`"，Provider 再把 Eino Tool Call 转回 OryxOS `llm.ToolCall`。真正执行工具的仍然是 ToolExecutor。

---

## 二、动手前先想清楚几件事

写这种适配层，别急着敲代码。先把下面几件事定下来，代码基本就顺着出来了。

**第一，把职责划窄。** Provider 要做的事其实很少：挑对模型、发起一次同步或流式调用、把结果拿回来。就这些。循环怎么转、工具怎么执行、上下文怎么拼，都不归它管。这个边界不划清楚，Provider 会越写越胖，最后和 ReActLoop 缠在一起分不开。

![动手前想清楚：Provider 的职责边界](../../website/public/images/class-16-2.svg)

**第二，哪些不自己造。** 各家大模型的 HTTP 协议、鉴权和工具格式不一样，这些由 Eino/Eino-ext connector 处理，OryxOS 不重复造。我们自己定义稳定的 `llm.ChatModel` 和领域类型，再写一层 Eino adapter 做双向转换。

**第三，四个坑——这几个直接决定了架构长什么样。**

*坑一：多个 provider 时，怎么区分谁是谁。* DeepSeek 和 MiniMax 在 OryxOS 中都实现 `llm.ChatModel`，光靠类型分不清。工厂必须维护 **provider 名 → 构造函数** 的显式映射；模型实例则按 `Profile.name` 保存，避免两个都使用 DeepSeek 的 Profile 因模型、温度或凭证不同而串配置。

*坑二：Eino 的 ADK 会"自作主张"帮你执行工具。* Eino 的 ADK（Agent Development Kit）自带一套自动执行工具的机制——模型说想调 `http_get`，它会自己跑个小循环，直接把工具执行了，再把结果喂回模型。听起来省事，但我们自己写了 ReActLoop 和 ToolExecutor 来管这件事。两套一起跑，工具会被调两次，而且执行绕过了我们的沙箱检查，出了事都不知道谁干的。所以**必须关掉 ADK 的自动执行**，只留 Eino 的协议转换和 schema 生成，执行权攥在自己手里。

*坑三：教程里的 provider 名字不能替代依赖验证。* 核心阶段固定接 DeepSeek 和 MiniMax：DeepSeek 使用 Eino-ext 原生 connector，MiniMax 使用 Eino-ext OpenAI connector 对接官方 OpenAI 兼容 API。动手前必须用 `go.mod`、模块缓存和 `go doc` 核实两个 connector 在锁定版本中的真实构造函数、配置字段和工具调用行为；示例代码不能作为 API 存在性证据。

*坑四：Provider Stream 不等于 Web SSE。* Eino connector 返回的是只能单消费者读取、必须关闭的 `schema.StreamReader`，这个类型不能越过 Provider 边界。OryxOS 要定义自己的流接口，在 adapter 内转换增量消息、合并被拆开的 Tool Call、处理 EOF 与关闭；本节只交付模型 Provider 的流式能力，不实现 ReAct 流式事件、CLI 增量输出或 HTTP SSE。

这四个坑想清楚，Provider 长什么样就定了：一个薄薄的抽象层，只管一次同步或流式模型调用，靠一张表显式选模型，关掉自动执行，接哪家提前验证过依赖能用。

---

## 三、代码怎么写

核心是 OryxOS `llm.ChatModel` 端口、Eino adapter 和 `ProviderService`。adapter 负责 OryxOS/Eino 类型转换，`ProviderService` 负责按 Profile 取模型、发起一次调用并写审计。对上层只暴露同步 `Chat(...)` 和流式 `ChatStream(...)`，签名中不出现 Eino 类型。

一次调用从头到尾是这样走的：

![Provider 架构：ProviderService 按 Profile 选模型、屏蔽各家差异](../../website/public/images/class-16-3.svg)

对着这个流程，先补一个前置，再分四步写。

**第零步（前置）：Profile 的解析加载，这节一并交付。** Provider 是整个系统里第一个消费 Profile 的模块（要从里面读 provider 名、model、温度），所以 Profile 从 YAML 变成 Go 结构体这件事归这节负责，包含三样：

- **`Profile`**：一个承载全部字段的结构体——`Name`、`Description`、`Identity`、`Provider`（Name/Model/Temperature）、`Tools`、`Skills`、`MCPServers`、`Channels`、`NotifyChannels`、`Schedules`、`Bootstrap`、`Settings`。后面每节用到哪个字段就取哪个字段，结构体本身这节就建全。
- **`ProfileLoader`**：启动时扫 `.oryxos/profiles/` 下所有 YAML，用 `gopkg.in/yaml.v3` 解析成 `Profile`，逐个做合法性校验（本节先校验"provider 名能在全局层找到"这一条，后面各节的字段各自补自己的校验规则）。坏的 Profile 记错误日志、不阻断启动。
- **`ProfileRegistry`**：解析好的 Profile 放进内存索引（`map[string]*Profile`），按 Name 查找。29 节会给它补运行时 `Register()` 方法，现在只有启动扫描这一条注册路径。

**第一步：配置分两层，别搞混。** Provider 相关的配置其实分两层，职责不一样：

- **全局层**（环境变量 + 启动参数）：声明这个实例上到底接了哪些 provider、每家的凭证从哪个环境变量读。解决的是"连不连得上"的问题。
- **Profile 层**（每个 Agent 自己的 YAML）：声明这个 Agent 具体用哪个 provider、哪个 model、什么温度。解决的是"这个 Agent 怎么用"的问题。
```yaml
# global.yaml —— 进程级启动配置，不属于 .oryxos/ 初始化文件
providers:
  - name: deepseek
    api_key: ${DEEPSEEK_API_KEY}
  - name: minimax
    api_key: ${MINIMAX_API_KEY}
```

```yaml
# .oryxos/profiles/ops-agent.yaml —— Profile 层：这个 Agent 具体怎么用
provider:
  name: deepseek        # 必须能在全局层的 providers 列表里找到同名项
  model: deepseek-chat  # 用哪个模型，Profile 自己定
  temperature: 0.7
```

两层各管一段：全局层只管"启用与凭证"（provider 存不存在、key 有没有），Profile 层管"调用参数"（用哪个 model、什么温度）。明确命名的 Provider 所用 connector、协议适配和官方 API 地址由对应工厂封装，不要求用户提供 `base_url`，也不允许把 `minimax` 改指向任意 OpenAI 兼容服务。Profile 引用的 `provider.name` 如果在全局层找不到同名项，必须直接报错，不能悄悄用错或者留空跑过去。

`${DEEPSEEK_API_KEY}` 表示运行时从环境变量取，代码和配置文件里都不会出现真实 key。

**第二步：先定义 OryxOS 窄端口，再注册工厂。** `internal/llm` 定义核心阶段真正需要的消息、Tool Call、Tool Definition 和 Usage：

```go
type ChatModel interface {
	Generate(ctx context.Context, request Request) (Response, error)
	Stream(ctx context.Context, request Request) (ResponseStream, error)
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

type StreamEventKind string

const (
	StreamEventDelta     StreamEventKind = "delta"
	StreamEventCompleted StreamEventKind = "completed"
)

type StreamEvent struct {
	Kind     StreamEventKind
	Delta    Message
	Response *Response
}

type ResponseStream interface {
	Recv() (StreamEvent, error)
	Close() error
}
```

`Generate` 阻塞到完整响应返回；`Stream` 返回一个 OryxOS `ResponseStream`。每次 `Recv()` 先返回 `delta` 事件，底层正常结束时再返回且只返回一次 `completed` 事件，其中 `Response` 是已经合并好的完整响应；下一次 `Recv()` 返回 `io.EOF`。`delta` 事件只使用 `Delta`，`Response` 必须为 nil；`completed` 事件只使用非 nil 的 `Response`，`Delta` 为零值。`Delta` 可能包含文本、reasoning 或分片 Tool Call，调用方不能把单个 Tool Call 分片当成完整参数执行。

`ResponseStream` 的所有权规则必须明确：只允许一个 goroutine 调用 `Recv()`，调用方无论读到 EOF、中途报错还是提前退出，都必须且只能关闭底层 reader 一次。OryxOS adapter 应把 `Close()` 包装成幂等操作，不能把 Eino `schema.StreamReader` 暴露给上层，也不需要为了流式接口额外启动 goroutine 或返回 channel。

工厂表按 `provider.name` 选择 DeepSeek 原生 connector 或 OpenAI connector，然后把 connector 包装为 OryxOS `llm.ChatModel`。加载合法 Profile 时，把全局连接信息与 Profile 的 model/temperature 合并，创建独立模型实例并以 `Profile.name` 保存。

```go
type ModelFactory func(context.Context, ProviderConfig) (llm.ChatModel, error)

type ProviderRegistry struct {
	factories map[string]ModelFactory  // key: provider.name
	models    map[string]llm.ChatModel // key: profile.name
}
```

工厂注册示例：

```go
const miniMaxOpenAIBaseURL = "https://api.minimax.cn/v1"

// 工厂注册时，每个 name 对应一个构造函数。
factories["deepseek"] = func(ctx context.Context, cfg ProviderConfig) (llm.ChatModel, error) {
	connector, err := deepseek.NewChatModel(ctx, &deepseek.ChatModelConfig{
		APIKey:      cfg.APIKey,
		Model:       cfg.Model,
		Temperature: cfg.Temperature,
	})
	return newEinoChatModelAdapter(connector, err)
}
factories["minimax"] = func(ctx context.Context, cfg ProviderConfig) (llm.ChatModel, error) {
	temperature := cfg.Temperature
	connector, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:      cfg.APIKey,
		Model:       cfg.Model,
		BaseURL:     miniMaxOpenAIBaseURL,
		Temperature: &temperature,
	})
	return newEinoChatModelAdapter(connector, err)
}
```

这里展示的是“厂商名选择构造函数”的结构：DeepSeek 使用原生 connector 自带的官方默认地址，因此不传 `BaseURL`；MiniMax 借用 OpenAI connector，所以由 `minimax` 工厂固定官方兼容地址。`newEinoChatModelAdapter` 是 Eino 类型逃不出 Provider 包的关键。`ProviderConfig` 只合并 `Name/Model/APIKey/Temperature`，不含 `BaseURL`。字段已按锁定的稳定版 Eino-ext DeepSeek `v0.1.7` 和 OpenAI `v0.1.13` 本地源码核验；Eino core 锁定为 `v0.9.19`。

**第三步：写 Chat 方法。** 这是整个 Provider 的核心，骨架长这样：

```go
func (p *ProviderService) Chat(ctx context.Context, sessionID string, profile *Profile, messages []llm.Message) (llm.Response, error) {
	cm, ok := p.registry.models[profile.Name]
	if !ok {
		return llm.Response{}, fmt.Errorf("provider not found: %s", profile.Provider.Name)
	}
	tools, err := p.toolSchemas.Resolve(ctx, profile.Tools) // OryxOS ToolDefinition
	if err != nil {
		return llm.Response{}, err
	}
	startedAt := time.Now()
	resp, err := cm.Generate(ctx, llm.Request{
		Messages: messages,
		Tools:    tools,
	})
	duration := time.Since(startedAt)
	p.audit.Record(context.WithoutCancel(ctx), sessionID, profile.Provider.Name, profile.Provider.Model,
		resp.Usage, err == nil, safeError(err), duration.Milliseconds())
	if err != nil {
		return llm.Response{}, err
	}
	return resp, nil
}
```

流式方法复用完全相同的 Profile 选路与 Tool schema，只把模型调用换成 `Stream`，并返回带审计能力的 OryxOS reader：

```go
func (p *ProviderService) ChatStream(ctx context.Context, sessionID string, profile *Profile, messages []llm.Message) (llm.ResponseStream, error) {
	cm, ok := p.registry.models[profile.Name]
	if !ok {
		return nil, fmt.Errorf("provider not found: %s", profile.Provider.Name)
	}
	tools, err := p.toolSchemas.Resolve(ctx, profile.Tools)
	if err != nil {
		return nil, err
	}

	startedAt := time.Now()
	stream, err := cm.Stream(ctx, llm.Request{Messages: messages, Tools: tools})
	if err != nil {
		p.audit.Record(context.WithoutCancel(ctx), sessionID, profile.Provider.Name, profile.Provider.Model,
			llm.Usage{}, false, safeError(err), time.Since(startedAt).Milliseconds())
		return nil, err
	}
	return newAuditedResponseStream(stream, startedAt, sessionID, profile, p.audit), nil
}
```

`newAuditedResponseStream` 是语义示例：初始化失败当场记一条失败调用；成功返回 reader 后，在 `completed`、中途 `Recv` 错误或提前 `Close` 三条终止路径上落一条且只落一条 `llm_calls`。成功记录使用完整响应中的最终 usage；提前关闭按失败记录，错误信息必须脱敏。模型调用仍使用原始 `ctx` 接收取消，审计写入使用 `context.WithoutCancel(ctx)`，避免调用已取消后连失败记录也无法落库。不能每收到一个 chunk 就写一条 `llm_calls`，因为一次 Stream 仍然只是一轮模型调用。

一行行看它在干嘛：

- `Chat(ctx, sessionID, ...)`——多传一个 sessionID，是因为审计记录要落到 `llm_calls` 表，那张表按 session 关联，方法签名不带这个参数，审计那一步就没法写。
- `p.registry.models[profile.Name]`——按 Profile 唯一运行时标识取启动阶段创建好的独立模型实例。Provider 名只用于选工厂，不作为模型实例缓存键。
- `p.toolSchemas.Resolve(...)`——按 Profile 顺序取出 OryxOS `ToolDefinition`，只取元数据，不执行。
- `cm.Generate(ctx, llm.Request{...})`——发起真正的调用。model 和 temperature 已在为 Profile 创建 connector 时固定；Eino adapter 在内部转换 Message/ToolDefinition，绑定 `schema.ToolInfo`，调用 connector，再把响应转回 `llm.Response`。
- `cm.Stream(ctx, llm.Request{...})`——发起同一模型的流式调用；adapter 返回 OryxOS reader，增量事件可逐块消费，但只有 `completed` 中的完整响应可以用于后续 Tool Call 执行或 Session 持久化。
- `p.audit.Record(..., true, "", ...)`——调用**成功**，把用了哪个 provider、哪个 model、花了多少 token、耗时多久记一笔，`success` 记 `true`。
- `p.audit.Record(..., false, err.Error(), ...)`——调用**失败**（超时、限流、模型报错）同样要记一笔，`success` 记 `false`、把错误信息存进 `error_message`，再把错误继续往上返回。**这一步很容易漏**：只记成功不记失败，一次真实事故在系统里就完全没留下痕迹。
- 最后返回 `llm.Response`。响应里可能带着模型"想调某个工具"的 `ToolCalls`，必须保留 tool call ID、function name 和原始 JSON arguments，但执行不在这儿。

> Eino `Generate`、`Stream`、`WithTools` 和 `schema.StreamReader` 的确切写法只出现在 Provider adapter 内，动手前要核对锁定版本。Eino reader 要求单消费者且必须关闭，OryxOS wrapper 负责把这条资源规则收口在 Provider 层。

**第四步：Eino 适配和审计。** `EinoChatModelAdapter` 负责把 OryxOS `Message/ToolDefinition` 转成 Eino `schema.Message/schema.ToolInfo`，并把 Eino 同步响应或流式 chunk 转回 OryxOS 类型。同步转换回归必须覆盖 system/user/assistant/tool 四种 role、完整 assistant Tool Calls、Tool message 的 `tool_call_id`、JSON Schema、usage 和 finish reason。

流式转换要额外维护一个小状态机：每次底层 `Recv()` 成功时先把 chunk 保存并返回 `delta`；底层返回 `io.EOF` 时使用 Eino `schema.ConcatMessages` 合并同一轮的全部 chunk，再复用同步响应转换函数生成完整 `Response`，返回一次 `completed`；下一次读取才返回 `io.EOF`。这样被分段返回的 Tool Call ID、name 和 JSON arguments 会在 Provider 边界内恢复完整，后续 ReAct 不需要理解 Eino chunk 格式。

审计要记两种情况：调用**成功**，把 provider、model、token 用量写进 `llm_calls`；调用**失败**（超时、限流、模型报错或 Stream 未完成就关闭），也要写一条，`success` 记 `false`、`error_message` 记原因。所以 `llm_calls` 表本身得有 `success`/`error_message` 这两列——不然模型调用失败时，这次事故在数据库里完全没留下痕迹，跟工具那边的 `tool_invocations` 表比就不对称，"可审计"这个卖点就打了折扣。流式调用无论多少 chunk 也只写一条调用记录，并用 `sync.Once` 或等价状态保证完成、错误和关闭路径不会重复落库。这张表的 GORM Model、Repository 和建表脚本也归这节交付；注意 SQLite 的 `ALTER TABLE` 很弱，建表用手工维护的脚本，别指望 GORM `AutoMigrate` 做迁移。

**有几样先别做。** fallback（一家挂了换另一家）、hedge racing（同时发几家抢最快的）、熔断，这些都放到扩展阶段；现在故障就直接把错误返回给上层。成本看板也放后面，眼下只落 `llm_calls` 这张表，够审计用就够了。本节的 Stream 到 ProviderService 为止，不改 ReActLoop，不做 Agent 级事件流、CLI 增量输出、SSE 或 WebSocket。核心阶段的目标是“Provider 同步和流式两条模型调用路径都可靠”，不是提前实现整条流式产品链路。

**本节交付物**（Spec-Kit 拆解锚点）：

- 代码：OryxOS `llm.ChatModel` 及 Message/Tool Definition/Tool Call/Usage/Stream 类型、`Profile`、`ProfileLoader`、`ProfileRegistry`、`ProviderService`（含同步 `Chat(...)` 与流式 `ChatStream(...)`）、Eino 同步/流式双向适配器、`LlmCall` GORM Model + `LlmCallRepository`
- 测试：`ProfileLoaderTest`、`ProviderServiceTest`、`EinoAdapterTest`、`LlmCallRepositoryTest`、`ProviderSmokeIT`（见第四部分的验收 harness）
- 配置：全局层的 providers 声明；Profile YAML 的 `provider` 段
- 表：`llm_calls`（含 `success`/`error_message` 列，手工建表脚本）

---

## 四、验收 harness：把验收标准变成可执行的测试

第 6 节讲过：SDD 给目标，**Harness 给边界**。这一节的 Harness 就是一套测试——AI 写的实现对不对，不靠人盯着代码看，靠这套测试跑绿。所以测试清单要在实现之前（或同时）定下来，`/speckit-implement` 产出的代码必须让它们全绿，这就是"验收"的工程化形态。

**先定分层：什么用单测，什么用集成冒烟。** 判断标准就一条——要不要碰真实网络：

- **单测（默认全跑）**：Runtime/Service 侧 mock OryxOS `llm.ChatModel` 的 `Generate/Stream`；Provider adapter 测试再 mock Eino `model.ToolCallingChatModel`。路由、校验、同步/流式审计和双向转换都不依赖真实网络。
- **集成冒烟（加 `// +build integration` 标签，本地手动跑）**：真调一次模型，验证"key 对、依赖对、真的通"。CI 里默认跳过——不能让外部 API 的可用性变成自己流水线的可用性。

**四个单测文件，逐条对应验收标准：**

| 测试文件 | 覆盖的验收点 |
|---|---|
| `profile_loader_test.go` | 合法 YAML 全字段解析；引用不存在的 provider 报错清晰；坏文件不阻断其余加载；全局 Provider 凭证的 `${ENV}` 占位正确解析 |
| `provider_service_test.go` | DeepSeek/MiniMax 工厂路由不串台；同一 Provider 的两个 Profile 实例隔离；Service 只依赖 OryxOS `llm.ChatModel`；同步和流式成功/失败都各落且只落一条审计，提前关闭按失败记录 |
| `eino_adapter_test.go` | 四种 role、Tool Call ID/arguments、Tool Result ID、JSON Schema、usage/finish reason 双向转换；Stream delta 顺序、分片 Tool Call 合并、completed/EOF/Close 语义；只绑定 schema，不执行 Tool |
| `llm_call_repository_test.go` | 手工建表脚本建出的 `llm_calls` 能存能读，`success`/`error_message` 两列真实存在 |

**最值钱的四个测试方法，写出来看。** Service 测试 mock OryxOS `llm.ChatModel`，adapter 测试单独 mock Eino connector：

```go
func TestRoutesDeepSeekAndMiniMaxByProviderName(t *testing.T) {
    deepseek := &mockChatModel{}
    minimax := &mockChatModel{}
    service := NewProviderService(map[string]llm.ChatModel{
        "ops-agent": deepseek, "support-agent": minimax,
    }, adapter, audit)

    service.Chat(context.Background(), "s-1", profileNamed("support-agent", "minimax"), prompt)

    if minimax.callCount != 1 {
        t.Errorf("expected minimax to be called once, got %d", minimax.callCount)
    }
    if deepseek.callCount != 0 {
        t.Errorf("expected deepseek to never be called, got %d", deepseek.callCount)
    }
}

func TestRecordsFailedLLMCallBeforeReturningError(t *testing.T) {
    chatModel := &mockChatModel{err: errors.New("connect timeout")}
    service := NewProviderService(map[string]llm.ChatModel{
        "ops-agent": chatModel,
    }, adapter, audit)

    _, err := service.Chat(context.Background(), "s-1", profileNamed("ops-agent", "deepseek"), prompt)
    if err == nil {
        t.Fatal("expected error, got nil")
    }

    if audit.lastRecord == nil {
        t.Fatal("expected audit record, got nil")
    }
    if audit.lastRecord.Success {
        t.Error("expected success=false in audit record")
    }
    if !strings.Contains(audit.lastRecord.ErrorMessage, "timeout") {
        t.Errorf("expected error message to contain 'timeout', got %s", audit.lastRecord.ErrorMessage)
    }
}

func TestEinoAdapterPreservesToolCallsAndToolResults(t *testing.T) {
	connector := &fakeEinoModel{response: assistantToolCall("call-1")}
	adapter := newEinoChatModelAdapter(connector)
	resp, err := adapter.Generate(context.Background(), llm.Request{
		Messages: []llm.Message{toolResult("call-0", "ok")},
		Tools:    []llm.ToolDefinition{httpGetDefinition()},
	})
	if err != nil || resp.Message.ToolCalls[0].ID != "call-1" {
		t.Fatalf("tool call conversion lost identity: %#v, %v", resp, err)
	}
	// fake connector 只记录 WithTools/Generate，没有 Tool 执行入口。
}

func TestEinoAdapterStreamReassemblesFragmentedToolCall(t *testing.T) {
	connector := fakeEinoStream(
		assistantToolCallChunk(0, "call-1", "http_get", `{"url":`),
		assistantToolCallChunk(0, "", "", `"https://example.com"}`),
		assistantFinalChunk("tool_calls", usage(11, 7, 18)),
	)
	adapter := newEinoChatModelAdapter(connector)
	stream, err := adapter.Stream(context.Background(), llm.Request{Tools: toolDefinitions()})
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()

	completed := receiveCompleted(t, stream)
	call := completed.Response.Message.ToolCalls[0]
	if call.ID != "call-1" || call.Function.Arguments != `{"url":"https://example.com"}` {
		t.Fatalf("fragmented Tool Call was not reassembled: %#v", call)
	}
	if _, err := stream.Recv(); !errors.Is(err, io.EOF) {
		t.Fatalf("Recv after completed = %v, want io.EOF", err)
	}
}
```

第一个测的是坑一（DeepSeek/MiniMax 工厂映射）；同文件还必须增加“两个 Profile 都用 DeepSeek 但配置不串台”的实例隔离测试。第三个测试证明同步路径只做 schema/消息转换且 Tool Call 关联不丢失。第四个测试证明 Stream 不把 Eino reader 泄漏出去，并能把多个 chunk 恢复为完整 Tool Call。第二个测的是最容易漏的失败审计路径：断言“返回了错误**并且**审计先落了账”；流式 Service 还要增加初始化失败、中途失败、提前关闭和完成后关闭四条审计回归，全部断言只写一条记录。

**`llm_call_repository_test.go` 的一个讲究**：建表要走那份手工脚本（测试里执行 `schema.sql`），不要让 GORM `AutoMigrate` 自动建——不然测试绿了、生产上跑真脚本时列名对不上，白测。

**集成冒烟 `provider_smoke_test.go`**：分别覆盖 DeepSeek 原生 connector 和 MiniMax OpenAI 兼容 connector；读取环境变量里的真 key，对每个 Provider 分别回归同步 Generate 和流式 Stream。Stream 必须至少收到一个 delta 和一个 completed，completed 中包含完整 Tool Call ID/arguments，随后返回 `io.EOF`；每次同步或流式调用各对应一条 `success=true` 的 `llm_calls`，不能按 chunk 重复记录。跑法：

```bash
go test ./...                                     # 日常：只跑单测，全绿才算实现完成
DEEPSEEK_API_KEY=xxx MINIMAX_API_KEY=xxx go test -tags=integration ./internal/provider/...  # 手动：两条路径冒烟验真
```

---

## 五、做完怎么验

harness 全绿之后，剩下这几条需要人工确认（自动化覆盖不到或不值得自动化的部分）：

- 用到的 provider，对应的 Eino-ext connector 模块已确认在 `go.mod` 里能下载、能解析——不是照着教程的名字就假设一定能用（`go mod tidy` 看一眼）。
- 集成冒烟真跑过一次：配真 key，`provider_smoke_test.go` 的 Generate 与 Stream 子测试都通过；Stream 收到过增量事件、完整 Tool Call 和最终 usage。
- key 走环境变量：`grep -r "sk-"`（或你的 key 前缀）在代码和配置里搜不到明文。
- 其余验收点——双 provider 路由、错误名报错、同步/流式成功失败审计、Stream 资源关闭、分片 Tool Call 合并、自动执行关闭、只翻译不执行——已由第四部分的单测覆盖，`go test ./...` 绿就等于打勾。

Provider 自己没有独立用户入口，它要和下一节的 ReAct 一起，才能撑起 Demo 一（每日天气）的对话版（问天气、给穿搭建议）。本节只证明同一 Provider 能以 Generate 返回完整响应，也能以 Stream 返回增量并恢复完整响应；下一节仍可先使用同步调用，CLI 与 Web 是否向用户实时输出不属于本节验收。
