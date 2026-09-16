# Spec-Kit 执行指导：从课件到代码（以第16节为例）

`docs/class/` 下 16 节起的课件文档有双重身份：既是讲课的课纲，也是 Spec-Kit 的开发原料。这份指导讲怎么把一节课变成能跑的代码——完整命令序列以第 16 节（Provider）为例给出，后续每一节照同一套流程重复即可。

> **本流程已固化为 skill**：日常直接用 `/oryxos-lesson-dev <节号>`（`.claude/skills/oryxos-lesson-dev/`），它按本指导 + 《Harness 设计》的门禁自动走完全程、在 tasks 后停等确认。本文档保留为原理讲解和手动兜底。

---

## 一、总原则：一次只喂一节课

每节课件的"本节交付物"清单就是一个 feature 的边界。16 节验收通过后再开 17 节的 specify——17 节交付物（`AgentService`/`ContextLoader`/`tool_invocations`）依赖的正是 16 节的产物。授课顺序就是构建顺序，这是当初把交付物按课切开的用意。

两层输入不要混：

- **`/speckit-specify` 喂 WHAT/WHY**——需求陈述（背景、场景、功能需求、边界、验收），不带技术选型；
- **`/speckit-plan` 喂 HOW**——技术栈、类型签名、模块落位这些实现约束。

每节课件的结构天然对应这个分工：第一、二部分（是什么/想清楚）供 specify 取材，第三部分（代码怎么写）供 plan 取材，"本节交付物"就是 tasks 的核对清单。验收分两层：课件带"验收 harness"部分的（16 节起的范式），harness 供 plan 的测试策略取材、由 implement 自动执行，`go test ./...` 全绿是机器判卷；"做完怎么验"清单是人工抽查项。harness 的覆盖现状：16~20、22、24~26、29、30 共十一节代码课都有正式的"验收 harness"部分；27、28 串联课以"把对账固化成 `// +build integration` 端到端测试"的指引替代；21、23 评审课不产码、31 的验收本质是真实运行加发布，均不设 harness。

## 二、前提确认（不用跑命令）

- Constitution 已存在（`.specify/memory/constitution.md` v1.0.0，八原则），不需要重建。
- 旧的 `specs/001-provider-react-loop/` 是早期"三合一"试跑的产物，留着无妨——specify 会新开编号目录，互不覆盖。

## 三、完整命令序列（以第16节为例）

### 第 1 步：/speckit-specify（生成规格）

把下面第四章的需求陈述整段作为参数：

```text
/speckit-specify <粘贴第四章"第16节需求"全文>
```

### 第 2 步：/speckit-clarify（可选，建议跑）

```text
/speckit-clarify
```

它会挑出规格里的歧义点问你（最多 3 个）。课件写得够细时可能没什么可问，跑一下花不了几分钟。

### 第 3 步：/speckit-plan（技术方案）

```text
/speckit-plan 技术栈：Go 1.26+ + Eino core + Eino-ext；DeepSeek 使用原生 connector 并沿用其官方默认地址，MiniMax 使用 OpenAI connector 且在工厂内固定官方兼容地址。动手前用 go.mod、模块缓存和 go doc 核实锁定版本的两个 connector API。实例级启动配置只声明 provider 名和环境变量凭证；Profile 只选择 provider/model/temperature；明确 Provider 的 connector、协议适配和端点策略由各自工厂封装，不接受用户配置 base URL。工厂按 provider.name 注册，模型实例按 Profile.name 隔离。SQLite + GORM 使用 glebarez/sqlite + modernc.org/sqlite，保持纯 Go 和 CGO_ENABLED=0；建表使用手工 SQL migration，不依赖 GORM AutoMigrate。模块落位：Profile 相关归 internal/profile，ProviderService 和 connector 工厂归 internal/provider，LlmCall GORM Model、Repository 和 migration 归 internal/store。测试按课件 harness 执行：profile_loader_test.go/provider_service_test.go/tool_schema_adapter_test.go/llm_call_repository_test.go，覆盖 DeepSeek/MiniMax 工厂路由、同厂商 Profile 隔离、坏 Profile 隔离、失败审计落账和不自动执行 Tool；provider_smoke_test.go 使用 //go:build integration，默认 CI 跳过。完成门禁为 go test ./...、go vet ./...、CGO_ENABLED=0 go build ./cmd/oryxos 全绿。
```

### 第 4 步：/speckit-tasks（拆任务）

```text
/speckit-tasks
```

产出任务清单后**先自己过一眼再往下走**：对照课件"本节交付物"核对没有漏项（16 节是四组交付物），坑一坑二坑三是否体现为任务或任务内约束。

### 第 5 步：/speckit-analyze（可选，一致性检查）

```text
/speckit-analyze
```

交叉核对 spec / plan / tasks 与 constitution 有无冲突（比如任务里是否不小心引入了自动 tool 执行）。

### 第 6 步：/speckit-implement（执行）

```text
/speckit-implement
```

implement 的完成标准是 harness 与仓库门禁全绿：`go test ./...`、`go vet ./...`、`CGO_ENABLED=0 go build ./cmd/oryxos`。DeepSeek/MiniMax 路由、同厂商 Profile 隔离、失败审计落账、自动执行关闭都有对应回归测试。跑完后人工确认 Eino-ext 依赖、为两个 Provider 配真 key 跑 `provider_smoke_test.go` 冒烟，并检查无明文 key。

## 四、第16节需求（specify 的输入原文）

> 讲课时用它回答"这节课要做什么、为什么做"；跑 Spec-Kit 时从"背景与价值"到"依赖与假设"整段粘贴。

**第16节需求：Provider——对接大模型的统一入口**

**背景与价值。** Agent 的大脑是 LLM，ReAct 循环每转一圈都要调一次模型。企业场景有三个现实约束：多家模型并存（不同 Agent 用不同家）、模型要能随时换（不锁任何一家厂商）、每次调用要可审计（花了多少 token、失败为什么）。没有统一抽象层，上层代码就会和某家 SDK 耦合死，换模型等于改代码——Provider 解决的就是这个。

**用户场景。**

1. 同一个 OryxOS 实例上，运维 Agent 用 DeepSeek、客服 Agent 用 MiniMax（OpenAI 兼容路径），两者并存、路由不串台。
2. 管理员想给某个 Agent 切换到实例已声明的 Provider，只改它 Profile 里的 `provider`/`model` 字段，不碰任何代码或凭证。
3. 审计员事后能查到：某次会话调了哪家模型、输入输出各多少 token、耗时多久；某次调用失败了，失败原因是什么。

**功能需求。**

- FR1：每个 Agent 的模型选择由 Profile（YAML）声明——用哪个 provider、哪个 model、什么温度；系统启动时从 `.oryxos/profiles/` 加载全部 Profile，坏文件记错误但不阻断启动。
- FR2：实例级声明 DeepSeek/MiniMax 清单及凭证来源（环境变量占位）；工厂按 provider 名注册，合并两层配置后为每个 Profile 创建并按 `Profile.name` 保存模型实例。Profile 引用了清单里不存在的 provider 名，必须显式报错，不允许静默跑过。
- FR3：上层传入 sessionID、Profile、OryxOS `[]llm.Message`，系统按 Profile 选中对应模型、发起一次调用、返回 OryxOS `llm.Response`。
- FR4：请求可携带 OryxOS `[]llm.ToolDefinition`；Provider 适配层负责与 Eino schema 双向转换并保留 Tool Call ID/arguments，绝不执行工具，必须关掉 Eino ADK 的自动工具执行。
- FR5：每次调用不论成败都落审计（`llm_calls` 表）：provider、model、token 用量、耗时、success 标识、失败原因，按 session 关联。
- FR6：凭证只从环境变量读取，代码、配置文件、日志里都不出现明文。

**明确不做（边界）。** ReAct 循环本身、工具的真正执行、fallback/熔断/hedge racing、成本聚合看板、流式响应——全部属于后续特性。

**验收标准。** 以 `docs/require/第16节` 的唯一匹配课件为准：可自动化部分由课件 harness 承载。硬门槛包括 DeepSeek/MiniMax 工厂路由不串台、同一 Provider 的两个 Profile 实例隔离、引用错误名有清晰报错、坏 Profile 不阻断合法 Profile、调用失败后 `llm_calls` 有 `success=false` 与脱敏 `error_message`、带工具请求不自动执行。人工项包括两个 Eino-ext connector 的本地 API/依赖确认、真实冒烟和无明文 key。

**依赖与假设。** Eino core、Eino-ext DeepSeek connector、Eino-ext OpenAI connector、GORM 与纯 Go SQLite 依赖必须在本地模块版本中核实；MiniMax 通过 OpenAI connector 接入，但其官方 API 地址由 `minimax` 工厂固定封装，实例配置只提供 API key。本模块的直接下游消费者是第 17 节的 ReAct 循环。

## 五、后续各节照此复制

给任意一节写 specify 输入时，套第四章的骨架即可：**背景与价值 → 用户场景 → 功能需求（从课件一、二部分提炼）→ 明确不做（课件"有几样先别做"）→ 验收标准（自动化部分指向课件的验收 harness，人工部分指向"做完怎么验"）→ 依赖与假设（指向前序课的交付物）**。plan 参数从课件第三部分抄类型名、签名、模块落位，并带上一句测试策略（照 16 节 plan 参数末尾那句的格式：测试文件清单 + 关键回归点 + "实现完成的定义是 go test ./... 全绿"）。

按新大纲的构建顺序：16 Provider → 17 ReAct（含 AgentService/ContextLoader）→ 18 CLI（含 SessionService）→ 19 Notify（注意课件里的"实现顺序说明"，完整接线在 24 节后）→ 20 Tool（含 MCP Client）→ 22 Memory 实现 → 24 Sandbox 实现 → 25 定时 → 26 Web Service → 29 插件化 → 30 动态管理 → 31 两个 Demo。评审课（21、23）和串联课（27、28）不产码：评审课是 22/24 的 specify 素材，串联课的验收清单直接当集成测试任务用。
