# OryxOS — GO 项目指南

OryxOS 是用 Go 1.26 实现的面向企业场景的 **Agent OS**。它部署在企业自己的 K8s、服务器或虚拟机上，作为统一底座运行多个业务 Agent，共享 Channel、Provider、ReAct、Tool、Memory、Session、Sandbox 和 Web Service 能力。数据留在企业自己的基础设施中，不锁定任何云生态。

核心阶段交付的是 Agent OS 的运行时内核，不是治理能力完备的企业平台。多租户、SSO、RBAC、完整审计、Provider fallback、多 IM Channel 和集群高可用都属于扩展阶段。

> 事实来源：`docs/DemandAnalysis.md` 定义 What 和阶段边界，是唯一需求事实；`docs/TechnicalSolution.md` 定义 How；`docs/IndustryResearch.md` 提供业界背景。发生冲突时按这个优先级处理，不得用本文件反向覆盖需求文档。

---

## 技术栈

| 组件 | 选型 |
|------|------|
| 语言 / 运行时 | Go 1.26+ |
| Agent Runtime | 自实现轻量 ReAct Loop |
| LLM 抽象 | Eino core `model.ToolCallingChatModel` |
| Provider connector | Eino-ext；DeepSeek 原生 connector，MiniMax 使用 OpenAI connector |
| HTTP 服务 | Gin，同步 JSON REST API |
| 命令行 | Cobra |
| YAML 解析 | `gopkg.in/yaml.v3` |
| 持久化 | GORM + `github.com/glebarez/sqlite`，底层 `modernc.org/sqlite` |
| MCP | 官方 Go SDK `github.com/modelcontextprotocol/go-sdk/mcp` |
| 定时调度 | `robfig/cron/v3` |
| 日志 | 标准库 `log/slog` + JSON Handler |
| 项目主页 | VitePress 静态站点 |
| 构建 | Go Modules，`CGO_ENABLED=0` 单二进制 |

核心发布物不得引入 JVM、Python、系统 SQLite 或其他外部运行时依赖。

---

## 模块结构（Go 包）

```text
oryxos-go/
├── cmd/oryxos/
│   └── main.go
├── internal/
│   ├── app/                 # Application 组装与生命周期
│   ├── config/              # ConfigLoader、配置类型和严格校验
│   ├── profile/             # ProfileLoader、ProfileRegistry
│   ├── skill/               # SkillLoader
│   ├── bootstrap/           # BootstrapLoader、Prompt 分段
│   ├── provider/            # Provider 工厂、DeepSeek/MiniMax 适配
│   ├── runtime/             # AgentService、ReActLoop、PromptBuilder
│   ├── memory/              # MarkdownMemoryStore
│   ├── session/             # SessionService、SessionStore
│   ├── tool/
│   │   ├── builtin/         # 9 个内置 Tool
│   │   ├── mcp/             # 官方 MCP SDK client 适配
│   │   ├── registry.go
│   │   └── executor.go
│   ├── sandbox/             # 文件、命令、URL 白名单
│   ├── scheduler/           # Profile schedules → AgentService
│   ├── channel/cli/         # 核心阶段唯一 Channel
│   ├── web/                 # Gin router、Handlers、DTO
│   ├── store/               # GORM、迁移、三张核心表
│   └── observability/       # slog、request/session 关联
├── docs/
├── go.mod
└── go.sum
```

依赖方向：

```text
cmd → app → handler/channel/scheduler → service/runtime
runtime → Eino core interfaces + domain ports
provider/tool-mcp/store → concrete external libraries
```

- `runtime` 不得导入 Gin、Cobra、GORM 或 Eino-ext。
- `web` 不得直接访问 Provider connector 或 Store 实现。
- 使用 Go 语义的 Handler、Store、Registry，避免 Java 风格的 Controller、Repository、Lifecycle 层级。
- 包结构可以随实现演进，但边界变化必须同步更新 `docs/TechnicalSolution.md` 第 11 章和本节，并说明理由；禁止循环依赖。

---

## 不可违背的原则（Constitution）

以下规则来自已经确认的需求与技术方案，所有实现和评审必须遵守。

### 原则一：自实现 ReAct Loop

`ReActLoop` 必须由 OryxOS 自己实现，不得使用 Eino ADK 等自动 Agent/ReAct 封装替代。OryxOS 必须掌握以下行为：

1. Prompt 组装；
2. LLM 调用；
3. Tool Call 解析；
4. Tool 执行与结果回填；
5. 最大迭代次数；
6. Session、`llm_calls`、`tool_invocations` 持久化；
7. 错误、取消和超时传播。

核心循环保持精简，但不能把控制权交给框架。

### 原则二：Eino core 是运行时边界，Eino-ext 只在工厂层

运行时最终调用 Eino core 的 `model.ToolCallingChatModel`。Eino-ext 只负责构造厂商 connector，不得成为 Handler、Scheduler、Tool 或业务层的接口。

```text
ReActLoop → model.ToolCallingChatModel
ProviderFactory → Eino-ext connector → Provider API
```

- DeepSeek 使用 Eino-ext DeepSeek connector。
- MiniMax 使用 Eino-ext OpenAI connector，并配置 MiniMax 官方 OpenAI 兼容 base URL。
- 不得使用 Eino ADK 自动执行 Tool；Tool 调度只能由 `ReActLoop + ToolExecutor` 完成。
- 上层不得保存或判断 Eino-ext 具体类型。
- 核心 Web API 只做同步 JSON；connector 的 Stream 回归不等于交付 SSE。

### 原则三：Provider 工厂按厂商映射，模型实例按 Profile 隔离

Provider 工厂按 `provider.name` 显式注册，模型实例按 `Profile.name` 保存：

```go
type ModelFactory func(
    ctx context.Context,
    cfg ProviderConfig,
) (model.ToolCallingChatModel, error)

type ProviderRegistry struct {
    factories map[string]ModelFactory                // key: provider.name
    models    map[string]model.ToolCallingChatModel // key: profile.name
}
```

不得只按厂商名复用模型实例。两个 Profile 即使都使用 DeepSeek，也可能使用不同模型、API key、base URL 和 temperature，必须相互隔离。首批只交付 DeepSeek + MiniMax；核心阶段不做 fallback、hedge racing 或动态路由。

### 原则四：Agent = Profile + Skill；Profile YAML 是唯一运行时配置源

- Profile 位于 `.oryxos/profiles/<name>.yaml`，定义“怎么运行”。
- Skill 位于 `.oryxos/skills/**/SKILL.md`，定义“做什么、什么时候做”。
- Profile 通过 `skills` 字段引用 Skill；两者绑定后才是完整业务 Agent。
- 不创建 `.oryxos/agents/`，不使用单数 `AGENT.md`，不从 Markdown frontmatter 派生 Profile。
- `Profile.name` 是唯一运行时标识，供 CLI、API、Session、Scheduler 和 Registry 使用。
- `identity.agent_name` 只用于展示，可以重复。
- `identity.prompt` 负责身份和基础人格，不能替代业务 Skill。

核心阶段只加载当前 Profile 引用的 Skill。完整 agentskills.io 标准、Marketplace 和 Tool LRU 加载属于扩展阶段。Skill 是 Prompt 上下文，不是 Tool，不进入 `ToolRegistry`。

### 原则五：调用记录 Day One 写入

核心阶段每次 LLM 调用都写 `llm_calls`，每次 Tool 调用都写 `tool_invocations`。失败调用也必须记录；错误详情进入结构化日志，不能以“日志已经够了”为由跳过数据库记录。

核心阶段不提供完整审计查询、报表或 SIEM 导出，但三张核心表的数据地基必须从第一天存在。

### 原则六：Sandbox 是统一应用层校验；单二进制不得破坏

核心阶段 Sandbox 不是容器强隔离，必须明确做到：

- 文件：规范化路径，校验真实路径仍位于允许根目录，拒绝软链接逃逸；
- 新文件：校验最近存在父目录的真实路径；
- Shell：使用 executable + argv 执行，不拼接 Shell 字符串；
- HTTP：校验 scheme 和域名；每次重定向后重新校验目标；
- 所有 Tool：设置超时、输入限制和输出大小限制。

完整 Docker、K8s Pod、WASM Sandbox 放扩展阶段。

SQLite 必须通过 `github.com/glebarez/sqlite` 使用纯 Go 的 `modernc.org/sqlite`，不得改用依赖 CGO 的 `mattn/go-sqlite3`。发布构建必须通过：

```bash
CGO_ENABLED=0 go build ./cmd/oryxos
```

### 原则七：核心链路同步、Tool 串行执行

- Gin 核心接口同步返回 JSON，不提供 SSE 或 WebSocket。
- 单轮模型返回多个 Tool Call 时按返回顺序串行执行，不做并行 Tool。
- `context.Context` 必须贯穿 CLI/Web/Scheduler、LLM、MCP 和 Tool。
- 不得为了“高性能”提前引入复杂异步编排。
- CLI、Web、Scheduler 都只能调用同一个 `AgentService.Invoke`，不得另建业务执行链。

### 原则八：Tool 统一进入 Registry/Executor，Skill 和 Bootstrap 不属于 Tool

内置 Tool、MCP Tool、编译进二进制的 Go Tool 全部注册到 `ToolRegistry`，统一由 `ToolExecutor` 完成参数校验、Sandbox、执行、重试判定和调用记录。

- Eino `tool.BaseTool` 只提供元数据。
- 可执行 Tool 必须实现嵌入 `BaseTool` 的 `tool.InvokableTool`，执行方法为 `InvokableRun`。
- OryxOS 用 `OryxTool` 包装运行元数据。
- Skill、Bootstrap、Memory 是 Prompt 上下文来源，不得包装为 Tool。
- 工作区不设置悬空的 `tools/` 配置目录。

### 原则九：核心阶段数字和边界不得漂移

核心阶段固定为：

- 2 个 Provider：DeepSeek + MiniMax；
- 9 个内置 Tool；
- 10 个 REST 端点；
- 12 个 CLI 叶子命令；
- 3 张 SQLite 核心表；
- 2 个验收 Demo；
- 5 个工作区子目录、6 个初始化文件；
- 4 周、每周 3 小时、合计 12 小时。

不得把以下扩展能力提前写成核心承诺：Provider fallback、多 IM Channel、完整 Skill 标准、向量 Memory、任务管理 API、`scheduled_tasks`/`task_executions`、SSE、OpenAPI、Prometheus 端点、认证/RBAC/SSO、多租户、完整审计、集群高可用、客户端 SDK。

---

## 工作区结构（运行时）

`oryxos init` 在当前目录创建 `.oryxos/`：

```text
.oryxos/
├── profiles/
│   └── default.yaml
├── sessions/
│   └── oryxos.db
├── skills/
├── logs/
├── memory/
│   └── MEMORY.md
├── mcp_servers.yaml
├── AGENTS.md
├── SOUL.md
└── USER.md
```

初始化创建五个子目录和六个初始文件：三个 Bootstrap 文件，以及 `profiles/default.yaml`、`memory/MEMORY.md`、`mcp_servers.yaml`。数据库文件在首次需要持久化时创建。

特别注意：

- 仓库根目录的 `AGENTS.md` 是开发代理项目指南。
- 运行时工作区的 `.oryxos/AGENTS.md` 是 Bootstrap 项目行为上下文。
- `SOUL.md` 定义默认人格。
- `USER.md` 由用户维护，OryxOS 只读。
- `MEMORY.md` 由 Agent 通过 `save_memory` 追加，OryxOS 读写。
- 不创建 `agents/` 或 `tools/` 子目录。

---

## 核心数据模型

### Profile YAML（`.oryxos/profiles/<name>.yaml`）

Profile 是运行时配置唯一来源。`oryxos init` 后，用户直接编辑 `profiles/default.yaml` 的 `provider.api_key` 和 `provider.model`。

```yaml
name: default
description: Default OryxOS profile

identity:
  agent_name: Oryx
  prompt: You are a helpful enterprise assistant.

provider:
  name: deepseek
  model: deepseek-chat
  api_key: ${LLM_API_KEY}
  base_url: ""
  temperature: 0.7

tools:
  - read_file
  - write_file
  - list_dir
  - shell
  - http_get
  - http_post
  - save_memory
  - recall_memory
  - notify

skills: []
mcp_servers: []
notify_channels: []
schedules: []

channels:
  - name: cli
    config: {}

bootstrap:
  - AGENTS.md
  - SOUL.md
  - USER.md

settings:
  max_iterations: 10
  max_history_turns: 20
```

字段规则：

- `name` 在工作区内唯一；
- `identity.agent_name` 仅展示；
- Provider 核心字段固定为 `name/model/api_key/base_url/temperature`；
- `tools/skills/mcp_servers` 的引用必须存在且不得重名；
- `notify_channels` 条目包含 `name/type/url`，其中 `name` 在 Profile 内唯一，`url` 推荐使用环境变量；
- `schedules` 条目包含 `id/cron/timezone/message/enabled`，其中 `id` 在 Profile 内唯一；
- `max_iterations` 默认 10，`max_history_turns` 默认 20；
- 配置变更核心阶段重启生效，不实现文件监听或热重载。

### Memory

核心长期记忆只有 `.oryxos/memory/MEMORY.md`：

- `save_memory(content)` 追加内容；
- `recall_memory(query)` 做大小写不敏感的关键词检索；
- Prompt 注入最多 4000 字，超出时简单截断；
- 核心阶段没有 `memory.backend`、scope、热/冷分区、归档 Memory 文件、Memory 表、Mem0 或向量数据库。

`USER.md` 是用户手写偏好，OryxOS 只读；`MEMORY.md` 是 Agent 成长记录，OryxOS 读写。Memory 不能覆盖指令。

### SQLite 核心表

数据库固定在 `.oryxos/sessions/oryxos.db`，核心阶段只有三张业务表。

**sessions**

| 字段 | 类型 / 约束 |
|------|-------------|
| `session_id` | TEXT PRIMARY KEY |
| `profile_name` | TEXT NOT NULL |
| `channel` | TEXT NOT NULL |
| `user_id` | TEXT NOT NULL |
| `messages_json` | TEXT NOT NULL |
| `status` | TEXT NOT NULL（`active` / `archived`） |
| `created_at` | DATETIME NOT NULL |
| `last_active_at` | DATETIME NOT NULL |
| `archived_at` | DATETIME NULL |

建立普通复合索引 `(channel, user_id, profile_name, status)`，供 SessionService 按 `status=active` 查询，不设唯一约束。记录唯一性由 `session_id` 主键保证。

Session 身份：

```text
stateful CLI/Web: channel + user_id + profile.name
stateless invoke: channel="http_invoke" + user_id=request_id + profile.name
scheduler:        channel="scheduler" + user_id=schedule.id + profile.name
```

无状态 invoke 每次生成唯一 `request_id`；同一 `schedule.id` 历次触发复用 Session，不同 schedule 相互隔离。

**tool_invocations**（核心阶段每次 Tool 调用写入）

| 字段 | 类型 / 约束 |
|------|-------------|
| `id` | INTEGER PRIMARY KEY AUTOINCREMENT |
| `session_id` | TEXT NOT NULL, INDEX |
| `tool_name` | TEXT NOT NULL |
| `input_json` | TEXT NOT NULL |
| `result_json` | TEXT NOT NULL |
| `success` | BOOLEAN NOT NULL |
| `error_message` | TEXT NULL |
| `duration_ms` | INTEGER NOT NULL |
| `created_at` | DATETIME NOT NULL |

**llm_calls**（核心阶段每次模型调用写入）

| 字段 | 类型 / 约束 |
|------|-------------|
| `id` | INTEGER PRIMARY KEY AUTOINCREMENT |
| `session_id` | TEXT NOT NULL, INDEX |
| `provider` | TEXT NOT NULL |
| `model` | TEXT NOT NULL |
| `prompt_tokens` | INTEGER NOT NULL DEFAULT 0 |
| `completion_tokens` | INTEGER NOT NULL DEFAULT 0 |
| `total_tokens` | INTEGER NOT NULL DEFAULT 0 |
| `duration_ms` | INTEGER NOT NULL |
| `created_at` | DATETIME NOT NULL |

不得为 Scheduler、Memory、Profile 或配置新增核心表。SQLite 启用 WAL 和合理的 `busy_timeout`，不在数据库事务内等待 LLM 或 Tool。

---

## ReAct Loop 工作机制

```text
CLI / HTTP / Scheduler
  → 统一 AgentRequest
  → AgentService 按 Profile.name 找 ProfileRuntime
  → SessionService 获取或创建 Session
  → PromptBuilder 组装 Prompt：
      [1] RUNTIME_RULES
      [2] AGENTS.md 项目规则
      [3] Profile.identity.prompt + SOUL.md
      [4] Profile 引用的 SKILL.md
      [5] USER.md
      [6] MEMORY.md（最多 4000 字）
      [7] 最近 max_history_turns 轮会话
      [8] 当前用户消息
      [9] 当前 Profile 可用 Tool schemas
  → ToolCallingChatModel 调 LLM，写 llm_calls
  → [无 Tool Call] 保存最终响应并返回
  → [有 Tool Call] ToolExecutor 按返回顺序逐个执行
      → 参数 schema 校验
      → Profile Tool 列表与 Sandbox 校验
      → InvokableRun 或 MCP call
      → 写 tool_invocations
      → 追加完整 assistant response + Tool messages
  → 继续循环，最多 max_iterations 次
```

Prompt 冲突优先级：

```text
运行时安全约束 > AGENTS.md > Profile identity / Skill > SOUL.md > USER.md > MEMORY.md
```

必须保留 assistant 的完整 Tool Call 响应和对应 Tool message，尤其要回归 MiniMax/OpenAI 兼容路径的 tool call ID 和多轮消息累积。核心阶段只截断早期历史，不做 LLM 总结压缩。

---

## Tool 体系

### OryxTool（所有可执行 Tool 的统一包装）

```go
type OryxTool struct {
    Tool       tool.InvokableTool
    Retryable  bool
    Idempotent bool
    Timeout    time.Duration
}
```

- `tool.BaseTool`：只提供元数据；
- `tool.InvokableTool`：嵌入 `BaseTool` 并提供 `InvokableRun`；
- `ToolRegistry`：注册和按 Profile 过滤 Tool；
- `ToolExecutor`：参数校验、Sandbox、执行、有限重试、结果回填和调用记录。

### 内置 Tool（核心阶段 9 个）

| Tool | 说明 |
|------|------|
| `read_file` | 读取文件，路径白名单 |
| `write_file` | 写文件，路径白名单、大小限制，默认非幂等 |
| `list_dir` | 列目录，路径白名单、条目数限制 |
| `shell` | 执行命令，精确白名单、argv、超时和输出限制 |
| `http_get` | GET，scheme/域名白名单、重定向重检 |
| `http_post` | POST，同上，默认非幂等 |
| `save_memory` | 追加 `MEMORY.md`，默认非幂等 |
| `recall_memory` | 关键词检索 `MEMORY.md` |
| `notify` | 发送到 Profile 通知渠道，核心阶段通用 Webhook，默认非幂等 |

`notify(content, channel?)` 规则：

- 无通知渠道：返回配置错误；
- 一个渠道：`channel` 可省略；
- 多个渠道：必须传 `channel`，按唯一的 `notify_channels[].name` 精确匹配；
- 核心阶段不广播；
- 发送目标仍通过 HTTP Sandbox；
- 成功或失败都写 `tool_invocations`。

Tool 只有在“错误明确可重试”且“调用幂等或带可靠幂等键”时才自动重试，指数退避最多三次。`write_file`、`shell`、`http_post`、`notify`、`save_memory` 默认不重试。

### Plugin Tool 三档

| 方式 | 门槛 | 推荐 | 实现 |
|------|------|------|------|
| 零代码 | 最低 | ⭐⭐⭐ | 写 SKILL.md + 复用社区 MCP server + Profile 引用 |
| 轻代码 | 中 | ⭐⭐ | 任意语言写 MCP server，连接定义在 `mcp_servers.yaml` |
| 重代码 | 高 | ⭐ | Go 实现 Eino `tool.InvokableTool`，编译进二进制 |

> 选择原则：能用方式一就不用方式二，能用方式二就不用方式三。

`.oryxos/mcp_servers.yaml` 是 MCP 连接唯一来源：

- stdio：`name/transport/command`，可选 `args/env`；
- remote：`name/transport/url`，认证通过环境变量注入；
- Profile 的 `mcp_servers` 只按名称引用；
- 使用官方 MCP Go SDK；
- MCP Tool 也必须进入同一 Registry、Sandbox 和调用记录链路。

---

## Web Service API

核心阶段严格只有 10 个端点，统一前缀 `/api/v1`：

| 方法 | 路径 | 说明 |
|------|------|------|
| `POST` | `/sessions` | 创建会话 |
| `POST` | `/sessions/{id}/messages` | 发消息并进入 AgentService |
| `GET` | `/sessions/{id}` | 查询历史 |
| `DELETE` | `/sessions/{id}` | 逻辑归档 |
| `POST` | `/agents/{name}/invoke` | 按 Profile.name 无状态调用 |
| `GET` | `/profiles` | 列 Profile |
| `GET` | `/memory` | 查询 `MEMORY.md` |
| `GET` | `/tools` | 列可用 Tool |
| `GET` | `/health` | 健康检查 |
| `GET` | `/info` | 版本、运行模式和 Provider 状态 |

Handler 只做请求解析、校验、调用 Service 和响应映射，不直接访问 GORM、Provider connector 或 Tool。

核心阶段不做：认证、SSE、WebSocket、限流、RBAC、Profile CRUD API、Memory 写 API、任务管理 API、Prometheus `/metrics`、OpenAPI endpoint。

---

## 命令行工具（12 个）

```bash
# 启动和状态
oryxos init
oryxos status
oryxos chat [--profile <name>]
oryxos serve
oryxos gateway

# Profile 管理
oryxos profile list
oryxos profile create <name>
oryxos profile show <name>
oryxos profile delete <name>

# 查询
oryxos provider list
oryxos tool list
oryxos session list
```

- `serve` 启动 Gin REST API 和 Scheduler。
- `gateway` 在核心阶段承载常驻 Runtime 和 Scheduler；多 Channel 宿主属于扩展阶段。
- 三种模式共享同一 ProfileRegistry、AgentService 和 SessionStore。
- 不添加需求未定义的 Agent、Memory、Schedule 管理命令。

---

## 配置加载规则

1. Profile YAML 是 Agent 运行配置唯一来源。
2. `ConfigLoader` 先读取 YAML，再展开 `${ENV_VAR}`，然后严格反序列化。
3. 必填项、未知关键字段、Provider、cron、通知渠道、设置范围和引用在启动阶段校验。
4. 缺失环境变量时报告变量名和 Profile 名，不能打印凭证。
5. `api_key`、Webhook URL、MCP auth 在日志和错误中脱敏。
6. `Profile.name`、MCP server name、通知渠道 name 和 schedule id 必须在各自作用域唯一。
7. Profile 修改后核心阶段重启生效，不做文件监听或热重载。
8. 加密存储、密钥轮转、Vault/KMS 属于扩展阶段。

用户首次使用的固定流程：

```text
oryxos init
  → 创建五个目录和六个初始文件
  → 用户编辑 .oryxos/profiles/default.yaml
  → 配置 provider.api_key 和 provider.model
  → 启动时 ConfigLoader 展开环境变量并校验
```

---

## 五大核心能力与验收 Demo

| 能力 | 核心组件 | 验收覆盖 |
|------|---------|---------|
| **一：对接 LLM** | `ProviderFactory`、`ProviderRegistry`、`ToolCallingChatModel` | Demo 一和二 |
| **二：ReAct 循环** | `AgentService`、`ReActLoop`、`PromptBuilder`、`ToolExecutor` | Demo 一和二 |
| **三：Memory** | `MarkdownMemoryStore`、`MEMORY.md`、Session | Demo 二体现用户关注偏好 |
| **四：Plugin Tool** | `ToolRegistry`、Sandbox、官方 MCP Client | Demo 一用内置 Tool；Demo 二验收 SKILL.md + MCP |
| **五：Web Service** | Gin、6 个 Handler、10 个 REST 端点 | 两个 Demo 的 Session 可查询并支持人工补跑 |

### Demo 一：每日天气

- Profile 引用只承载天气任务指令的最小 Skill；
- 该 Skill 是 Java “光杆 `AGENT.md` 正文”的 Go 映射，不包含 MCP 或子指令；
- Scheduler 到点调用 `AgentService`；
- Agent 调 `http_get` 查询天气，生成穿搭建议，再调 `notify`；
- 两次出站都通过域名白名单并写 `tool_invocations`；
- `GET /api/v1/sessions/{id}` 可查完整记录；
- 此 Demo 不承担 Demo 二的零代码 Plugin Tool 验收。

### Demo 二：每日科技日报

- 业务方不写 Go 代码；
- 只写 SKILL.md、`mcp_servers.yaml` 和 Profile 的 `schedules`；
- Agent 自主调用新闻 MCP Tool、组织日报、调用推送类 MCP Tool；
- 日报体现 `MEMORY.md` 中“更关注 AI 和芯片”等偏好；
- Scheduler、人工 CLI 和无状态 invoke 都复用 `AgentService`。

核心阶段只有这两个 Demo，不增加 GitHub Daily 第三个 Demo。两个 Demo 都必须支持钟推自动运行和人推手动补跑。

---

## 四周实施节奏

| 周次 | 核心任务 | 关键交付 | 验收 |
|------|---------|---------|------|
| 第一周 | LLM + ReAct | init、Profile、DeepSeek/MiniMax 工厂、自研 ReAct、HTTP Tool、CLI、内存 Session | 两个 Provider 各完成多轮 Tool Calling |
| 第二周 | Memory + Tool | MEMORY.md、9 个内置 Tool、Sandbox、官方 MCP Client、调用记录 | Memory 追加/检索、MCP 调用、副作用 Tool 不误重试 |
| 第三周 | Web Service | Gin、10 个 REST 端点、环境变量展开和配置校验 | 10 个端点逐个回归 |
| 第四周 | 多 Agent + 持久化 + 调度 | 12 个 CLI、纯 Go SQLite、三表、Bootstrap、Scheduler、两个 Demo、日志、主页 | `CGO_ENABLED=0` 构建、重启恢复、schedule 隔离和防重入 |

核心阶段每周 3 小时，合计 12 小时。时间不足时优先保证最短完整闭环和两个 Demo，不得通过扩大核心范围“补齐”文档。

---

## 常见陷阱

| 陷阱 | 症状 | 修复 |
|------|------|------|
| 使用 Eino ADK 自动 Agent/Tool 执行 | ReAct 控制权丢失或 Tool 重复执行 | 只保留 `ToolCallingChatModel`，由 `ReActLoop + ToolExecutor` 执行 |
| 业务层直接依赖 Eino-ext | connector 类型散落，升级困难 | Eino-ext 只出现在 `internal/provider` 工厂 |
| 模型实例只按 provider.name 缓存 | 不同 Profile 的 key/model/base URL 相互覆盖 | 工厂按 provider name，实例按 Profile.name |
| 只实现 `tool.BaseTool` | 有 schema 但无法执行 | 实现 `tool.InvokableTool.InvokableRun` |
| 把 Skill 当成 Tool | Skill 被注册或模型看不到业务指令 | Skill 由 `SkillLoader/PromptBuilder` 加载，不进 Registry |
| 恢复 `agents/<name>/AGENT.md` | 出现第二配置源，与 Profile 冲突 | 只保留 `profiles/*.yaml + skills/**/SKILL.md` |
| 用 `identity.prompt` 承载完整业务任务 | 身份和业务定义混在一起 | 身份放 identity，任务放 Skill |
| 引入多个 Memory backend 或 Memory 表 | 核心范围膨胀 | 核心只实现一个 `MEMORY.md` |
| 改用 `mattn/go-sqlite3` | `CGO_ENABLED=0` 构建失败 | 使用 glebarez dialector + modernc 驱动 |
| 为 Scheduler 建任务表或管理 API | 核心表和端点数量漂移 | Profile schedules 是定义源，只复用三张调用表 |
| 为 `(channel,user_id,profile_name)` 建全量唯一索引 | 归档或无状态 invoke 后发生约束冲突 | 使用普通 `(channel,user_id,profile_name,status)` 查询索引；invoke 用唯一 request_id |
| 副作用 Tool 无条件重试 | 重复写文件、执行命令或推送 | 只有可重试且幂等/有幂等键时重试 |
| 多通知渠道省略 channel 或默认广播 | 推错渠道或重复推送 | 多渠道必须按 name 精确指定，核心不广播 |
| 并行执行多个 Tool Call | Tool 结果顺序和上下文不稳定 | 核心阶段按模型返回顺序串行执行 |
| 把扩展端点加入核心 API | 10 端点验收口径失真 | 对照需求文档，扩展能力只预留接口 |
| 只写日志不落调用表 | 后续无法可靠关联 Session、模型和 Tool | 每次调用写 `llm_calls/tool_invocations` |
| 在数据库事务内等待 LLM/Tool | SQLite 锁时间过长 | 外部调用在事务外，最终使用短事务提交 |

---

## 设计原则

- **需求文档是唯一需求事实**：实现范围、数字和验收口径先查 `docs/DemandAnalysis.md`。
- **底座优先于业务 Agent**：核心交付是让多个 Agent 可靠运行的环境，不是单个 Demo。
- **Agent = Profile + Skill**：Profile 决定怎么跑，Skill 决定做什么。
- **自实现核心，复用管道**：ReAct 自己实现；模型与 Tool 抽象复用 Eino，MCP 协议复用官方 Go SDK。
- **依赖倒置**：Runtime 依赖 Eino core 和内部端口，外部 connector/数据库/HTTP 框架在边缘。
- **开放标准**：Tool 对接 MCP，Skill 兼容 agentskills.io 的完整支持放扩展阶段。
- **状态外置**：Profile/Skill/Bootstrap/Memory 在文件系统，Session 和调用记录在 SQLite。
- **安全是地基**：白名单、超时、最小权限、凭证环境变量和调用记录从第一天存在。
- **单二进制承诺**：纯 Go SQLite，`CGO_ENABLED=0` 必须持续可构建。
- **分阶段克制**：核心阶段只做最短闭环，治理、分布式和完整生态延后。
- **变更同步**：需求变化先改需求文档，技术决策再改技术方案，最后同步本指南。

代码实现后，每次交付前至少执行：

```bash
# 对本次修改的每个 Go 文件执行 gofmt -w
go test ./...
go vet ./...
CGO_ENABLED=0 go build ./cmd/oryxos
```

若仓库尚未进入代码阶段、命令因 `go.mod` 不存在而无法执行，必须明确说明，只能做文档一致性验证，不能声称代码构建或测试通过。
