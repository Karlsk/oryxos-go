# OryxOS Agent 管理后台 UI — 设计文档

> **状态**：草案 v0.1（2026-07-29）
> **覆盖范围**：与 [specs/008-agent-web-service/spec.md](../../specs/008-agent-web-service/spec.md) 配套，承接其在 [plan.md](../../specs/008-agent-web-service/plan.md) 中显式标注为「核心阶段不做」的第一版只读管理平台
> **所属阶段**：**扩展阶段**（与 [CLAUDE.md §3](../../CLAUDE.md) 一致；如要纳入核心阶段需先动宪法 §III）
> **关联基线**：8 个 user story 已交付（001 — 008）—— 5 大核心能力 + Scheduler + Sandbox + 全部审计地基

---

## 0. 你为什么需要读这份文档

后端 10 个 REST 端点已经能跑（`oryxos serve` 启动后访问 `http://localhost:8080/swagger-ui.html`），但业务方真正投产前需要 **一个能点能看的界面** —— 排查 Agent 为什么没响应、看 LLM 成本、看 Tool 审计。本文档回答三件事：

1. **画什么**（信息架构 + 页面清单）
2. **怎么画**（Vue 3 + 同官网视觉栈 + 组件选型）
3. **怎么切**（从 MVP 8 屏到完整控制台，4 个里程碑）

---

## 1. 设计目标

| 目标 | 度量 | 优先级 |
|------|------|--------|
| 让运维 5 分钟内能定位一个 Agent 调用失败根因 | MTTR（首次看到 fail → 找到错误堆栈）≤ 5 min | P0 |
| 让业务方知道 LLM 成本 | 「昨日 token 消耗 + 费用」首屏可见 | P0 |
| 让审计/合规可自助查询 | 全链路（Session → LLM → Tool）可下钻 | P0 |
| 与官网视觉一致 | 复用 VitePress 调色板 + 字体 | P1 |
| 零后端改动接入 | 只消费 [CLAUDE.md §15](../../CLAUDE.md) 10 个 REST 端点 | P0 |

---

## 2. 范围切片

### 2.1 MVP（M0，预计 1 周）— 核心阶段结尾的「只读第一版」

- 8 个页面，全是 **GET**，无任何写操作
- 单租户，无鉴权（业务方前置 API gateway）
- 桌面端 only（≥ 1280px），移动端不做
- 数据全部走轮询（5s / 30s / 5min 三档），不做 SSE / WebSocket

### 2.2 扩展期（M1 — M3）

详见 [§13 实施阶段](#13-实施阶段)。这些不在本文档详细设计 —— 本文只锚定 **M0** 全貌 + **M1—M3** 路线图。

---

## 3. 用户角色与场景

| 角色 | 占比 | 典型场景 | 关键页面 |
|------|------|---------|---------|
| **平台运维** | 50% | 「Agent X 调了 10 次失败了 8 次，给我看」 | Session 详情 / Tool 审计 / LLM 审计 |
| **Agent 业务方** | 30% | 「今天消耗多少 token？哪个 Agent 最贵？」 | Dashboard / Provider 列表 / 时序图 |
| **审计/合规** | 15% | 「3 月 1 日 user-123 调了什么 Tool？发了什么 Notify？」 | Memory 浏览器 / 审计导出 |
| **演示 / Demo** | 5% | 跑通 3 个 Demo（天气 / 科技日报 / GitHub 日报）展示给老板看 | Dashboard + Agent 详情 |

---

## 4. 信息架构

### 4.1 顶级导航（左侧栏）

```
┌─────────────────────────────────────────────┐
│  OryxOS                       👤 admin      │
├─────────────────────────────────────────────┤
│  📊 总览 Dashboard                          │
│  🤖 Agent 管理                              │
│  💬 Session 会话                            │
│  🧠 Memory 记忆        ◀── 扩展阶段 M1 打开 │
│  🔧 Tool 工具                                │
│  📋 Profile 配置                              │
│  ⚡ Provider 模型                            │
│  ⏰ 定时任务                                 │
│  🩺 系统健康  ◀── MVP 隐藏，运维事件后加     │
│  ─────────────                              │
│  ⚙ 设置    ◀── 扩展阶段 M2 打开             │
└─────────────────────────────────────────────┘
```

### 4.2 页面清单（MVP 8 页）

| # | 路径 | 页面 | 调用的 REST 端点 | 轮询频率 |
|---|------|------|------------------|---------|
| 1 | `/` | Dashboard 总览 | `GET /api/v1/health`、`/info`、聚合查询 | 30s |
| 2 | `/agents` | Agent 列表 | `GET /api/v1/profiles`（按 type=agent 过滤） | 60s |
| 3 | `/agents/:name` | Agent 详情 | `GET /api/v1/profiles/{name}` + 聚合 | 5s |
| 4 | `/sessions` | Session 列表 | `GET /api/v1/sessions`（分页） | 5s |
| 5 | `/sessions/:id` | Session 详情（对话流） | `GET /api/v1/sessions/{id}` | 5s |
| 6 | `/tools` | Tool 浏览器 | `GET /api/v1/tools` | 60s |
| 7 | `/providers` | Provider 列表 | `GET /api/v1/info`（拼装） | 5min |
| 8 | `/schedules` | 定时任务 | 扩展阶段端点（核心阶段 CLI only） | 30s |

---

## 5. 页面设计

### 5.1 Dashboard 总览（首屏）

**目标**：30 秒内告诉使用者「系统活着 + 昨天花了多少钱 + 有没有异常」。

```
┌─────────────────────────────────────────────────────────────┐
│  OryxOS Dashboard                            2026-07-29 14:23│
├─────────────────────────────────────────────────────────────┤
│  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────┐         │
│  │ 24h LLM  │ │ 24h Tool │ │ 活跃     │ │ 异常     │         │
│  │ 调用     │ │ 调用     │ │ Session  │ │ Session  │         │
│  │ 1,234    │ │ 567      │ │ 42       │ │ 3 ⚠      │         │
│  │ +12% ↗   │ │ +5% ↗    │ │ -8% ↘    │ │ 12% ↗    │         │
│  └──────────┘ └──────────┘ └──────────┘ └──────────┘         │
│                                                              │
│  ┌────────────────────────┐  ┌─────────────────────────┐    │
│  │ 24h Token 消耗趋势     │  │ Top 5 异常 Session      │    │
│  │   ▁▂▃▅▆▇█▇▆▅           │  │ 1. agent-3 #s-91  ❌ 429 │    │
│  │ (sparkline)            │  │ 2. agent-1 #s-87  ❌ TO  │    │
│  │ 0───────────24h →      │  │ 3. agent-2 #s-85  ❌ SB  │    │
│  └────────────────────────┘  └─────────────────────────┘    │
│                                                              │
│  ┌──────────────────────────────────────────────────────┐    │
│  │ 最近 5 个 Agent 触发事件                              │    │
│  │ 14:23  chat        → agent-weather  ✓ 1.2s           │    │
│  │ 14:21  scheduler   → agent-github   ✓ 4.8s           │    │
│  │ 14:15  api/invoke  → agent-3        ❌ 30s (timeout) │    │
│  └──────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

**关键组件**：
- 4 个 StatTile（左上，参考 [dataviz skill](https://github.com/anthropics/skills) 的 stat-tile 设计准则）
- 2 个 Chart（时序图 / 排名图）
- 1 个 EventStream（实时事件流，30s 轮询）

**数据源**（**核心阶段没有聚合 API** → MVP 三个选项）：
- 方案 A：后端新增 `GET /api/v1/stats/dashboard`（推荐，1 个端点解决 4 个 tile + 2 个 chart）
- 方案 B：前端轮询 N 次明细聚合（性能差，**否决**）
- 方案 C：直接读 SQLite（绕过 API，**否决**，违反 5 大能力的"接口边界"）

> ⚠ **建议**：将方案 A 作为本规格的 **额外端点**，纳入 008-agent-web-service 的 US-4 范围。

---

### 5.2 Agent 列表（`/agents`）

```
┌─────────────────────────────────────────────────────────────┐
│ Agent 管理                              🔍 搜索  + 刷新     │
├─────────────────────────────────────────────────────────────┤
│ 名称             Provider   模型       状态   24h 调用  状态  │
│ ─────────────── ──────── ────────── ────── ───────── ───── │
│ agent-weather   deepseek  ds-v3      ● 启用   812      0% 错 │
│ agent-github    deepseek  ds-v3      ● 启用   234      2% 错 │
│ agent-news      kimi      kimi-k2    ◐ 禁用   0        -   │
│ agent-stock     qwen      qwen-max   ● 启用   56       12%错│
└─────────────────────────────────────────────────────────────┘
```

**交互**：
- 点击行 → `Agent 详情` 页
- 状态徽章：● 启用 / ◐ 禁用 / ⚠ 异常
- 24h 调用数 + 错误率（红色高亮 > 10%）

---

### 5.3 Agent 详情（`/agents/:name`）

```
┌─────────────────────────────────────────────────────────────┐
│ ← Agent   agent-weather          [▶ 手动触发测试]           │
├─────────────────────────────────────────────────────────────┤
│  Profile YAML 元数据（折叠）                                │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ name: agent-weather                                  │    │
│  │ provider: deepseek / ds-v3                           │    │
│  │ tools: [file_read, http_get, notify]                 │    │
│  │ skills: [weather-cn]                                 │    │
│  │ schedules: [daily-8am]                               │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                              │
│  Tab: [调用历史] [成本] [Tool 分布] [错误]                   │
│                                                              │
│  Tab 1: 调用历史（最近 50 次）                               │
│  时间                触发源      Session       耗时   结果   │
│  2026-07-29 14:23  scheduler  s-91abc        1.2s   ✓     │
│  2026-07-29 08:00  scheduler  s-90xyz        1.4s   ✓     │
│  2026-07-29 06:12  chat       s-89def        0.8s   ✓     │
│  ...                                                        │
└─────────────────────────────────────────────────────────────┘
```

**交互**：
- 「手动触发测试」按钮 → 弹框输入 message → 调 `POST /api/v1/agents/{name}/invoke` → 跳转 Session 详情
- 4 个 Tab 全部走聚合查询，端点同 [§5.1 方案 A](#51-dashboard-总览首屏)

---

### 5.4 Session 列表（`/sessions`）

```
┌─────────────────────────────────────────────────────────────┐
│ Session 会话                          🔍  [+ 新建]          │
├─────────────────────────────────────────────────────────────┤
│ Session ID    Agent           触发  消息  状态  开始时间     │
│ ──────────── ─────────────── ──── ──── ───── ───────────── │
│ s-91abc...    agent-weather   sched  4    ✓    14:23       │
│ s-90xyz...    agent-weather   sched  4    ✓    08:00       │
│ s-89def...    agent-3         api    12   ❌   14:15       │
│ s-88uvw...    agent-github    api    6    ✓    13:50       │
│                                                              │
│                   ◄ 1 2 3 ... 42 ►    [每页 20 ▾]           │
└─────────────────────────────────────────────────────────────┘
```

**关键字段**：
- `s-91abc...` 截断显示，hover 完整 ID + 「复制」按钮
- 状态徽章：✓ 成功 / ❌ 失败 / ⏳ 进行中 / ⊘ 取消
- 触发源：scheduler / chat / api（颜色区分）
- 服务端分页（`?page=1&size=20`）

---

### 5.5 Session 详情（`/sessions/:id`）— 整个产品的核心页

这是管理员 80% 时间停留的页面 —— **完整对话流 + 每一步决策记录**。

```
┌─────────────────────────────────────────────────────────────┐
│ ← Session  s-91abc-def-123                       🔄 自动刷新 │
├─────────────────────────────────────────────────────────────┤
│ 概览：                                                      │
│   Agent: agent-weather    触发: scheduler    耗时: 1.2s     │
│   消息数: 4    Iterations: 3    Tokens: 1,234               │
├─────────────────────────────────────────────────────────────┤
│ ┌─── Step 1: User Input ──────────────────────────────┐    │
│ │  用户原始消息（来自 scheduler 触发）                  │    │
│ │  "查询今天杭州天气"                                  │    │
│ └─────────────────────────────────────────────────────┘    │
│                                                              │
│ ┌─── Step 2: LLM Call ─────────────────────────────────┐    │
│ │  Model: deepseek-v3    Tokens: 234+156 = 390         │    │
│ │  Duration: 0.8s   Cost: $0.0002                      │    │
│ │  ──────────────── Prompt 摘要 ────────────────       │    │
│ │  [人格: 你是一个天气助手]                            │    │
│ │  [Memory: 检索 "杭州天气" 命中 0 条]                │    │
│ │  [Tools: file_read, http_get, notify]                │    │
│ │  ──────────────── Response ────────────────          │    │
│ │  <reasoning>用户要查天气，应该用 http_get</reasoning> │    │
│ │  <tool_call>                                        │    │
│ │    name: http_get                                   │    │
│ │    args: {url: "https://wttr.in/Hangzhou"}          │    │
│ │  </tool_call>                                       │    │
│ └─────────────────────────────────────────────────────┘    │
│                                                              │
│ ┌─── Step 3: Tool Call ─────────────────────────────────┐    │
│ │  Tool: http_get                                      │    │
│ │  Args: {url: "https://wttr.in/Hangzhou"}            │    │
│ │  Sandbox: ✓ 通过                                     │    │
│ │  Duration: 0.4s                                      │    │
│ │  Result: {"temp_C": "32", "weather": "Sunny"}        │    │
│ └─────────────────────────────────────────────────────┘    │
│                                                              │
│ ┌─── Step 4: Final Answer ──────────────────────────────┐    │
│ │  🤖 Agent 回复                                        │    │
│ │  "今天杭州 32°C 晴"                                  │    │
│ └─────────────────────────────────────────────────────┘    │
│                                                              │
│ ┌─── Step 5: Notify ────────────────────────────────────┐    │
│ │  Channel: webhook-default    Status: 200    0.1s    │    │
│ │  Sent: "杭州今日 32°C 晴"                             │    │
│ └─────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

**关键设计**：
- 每个 Step 是一张「时间线卡片」，类似 GitHub PR 的 commit 列表
- Prompt 摘要默认折叠（避免 token 列表过长），点击展开
- Tool 失败的 Step 卡片整体标红
- 不显示完整 Prompt（防 prompt injection 泄露给截图分享的人）
- 自动刷新 5s（仅 session 状态为 ⏳ 进行中 时启用）

---

### 5.6 Tool 浏览器（`/tools`）

```
┌─────────────────────────────────────────────────────────────┐
│ Tool 工具                                  🔍 搜索          │
├─────────────────────────────────────────────────────────────┤
│ 名称         来源      描述            24h  错误率  状态     │
│ ─────────── ──────── ────────────── ──── ──────── ─────── │
│ file_read    builtin  读文件         123  0.0%    ● 启用    │
│ http_get     builtin  HTTP GET       234  1.2%    ● 启用    │
│ notify       builtin  发送通知        56  0.0%    ● 启用    │
│ shell        builtin  执行 shell 命令  12  8.3% ⚠ 危险     │
│ gh           mcp      GitHub CLI     234  0.4%    ● 启用    │
│ kimi-search  java_bean Kimi 搜索      45  2.2%    ● 启用    │
└─────────────────────────────────────────────────────────────┘
```

**交互**：
- 行点击 → 右侧抽屉显示 Tool Schema（参数定义）和最近 20 次调用记录
- 「来源」徽章颜色：builtin 蓝 / mcp 紫 / java_bean 绿（对应 [CLAUDE.md §9.7 tool_invocations.source](../../CLAUDE.md)）

---

### 5.7 Provider 列表（`/providers`）

```
┌─────────────────────────────────────────────────────────────┐
│ Provider 模型                                               │
├─────────────────────────────────────────────────────────────┤
│ ┌──────────────────────────────────────────────────────┐    │
│ │ deepseek                                          ● 健康 │    │
│ │ 模型: deepseek-v3 / deepseek-coder / deepseek-r1     │    │
│ │ 24h 调用: 1,234    24h tokens: 8.2M    错误率: 0.4%  │    │
│ │ P50 延迟: 1.2s   P95 延迟: 3.8s                      │    │
│ │ ▂▃▅▇█▇▆▅▄▃▂▁  ← 24h 调用时序图                       │    │
│ └──────────────────────────────────────────────────────┘    │
│                                                              │
│ ┌──────────────────────────────────────────────────────┐    │
│ │ kimi                                             ● 健康 │    │
│ │ 模型: kimi-k2                                       │    │
│ │ 24h 调用: 234    24h tokens: 1.1M    错误率: 1.2%   │    │
│ │ P50 延迟: 2.1s   P95 延迟: 5.4s                     │    │
│ └──────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────┘
```

**注意**：核心阶段 `GET /api/v1/info` 不返回详细统计 → 需要聚合查询（推荐增 `GET /api/v1/providers` 端点）。

---

### 5.8 定时任务列表（`/schedules`）

> 核心阶段仅 CLI `oryxos schedule list` 暴露，这页要等 `GET /api/v1/schedules` 端点落地（属 008-web-service 扩展）。

```
┌─────────────────────────────────────────────────────────────┐
│ 定时任务                                  🔍 搜索            │
├─────────────────────────────────────────────────────────────┤
│ ID              Agent            Cron          时区    状态  │
│ ────────────── ──────────────── ───────────── ─────── ──────│
│ daily-8am       agent-weather    0 8 * * *     Asia/.. ● 启用 │
│ daily-9am       agent-github     0 9 * * *     Asia/.. ● 启用 │
│ weekly-mon      agent-news       0 9 * * 1     UTC     ◐ 暂停 │
│                                                              │
│ [▶ 立即触发] [⊘ 禁用] [🗑 删除]  ←── 需 P0 写端点         │
└─────────────────────────────────────────────────────────────┘
```

**MVP 状态**：只读列表，写操作按钮显示但 click → 提示「请用 CLI」。M1 接入写端点。

---

## 6. 技术栈

### 6.1 选型决策

| 维度 | 选型 | 理由 |
|------|------|------|
| 框架 | **Vue 3.5** + Composition API + `<script setup>` | 与 [website/](../../website/package.json)（VitePress/Vue 3.5）同栈同视觉 |
| 构建 | **Vite 5** | VitePress 已在用，且 HMR 速度极快 |
| 语言 | **TypeScript 5** | 端点契约 [swagger.json](http://localhost:8080/v3/api-docs) 自动生成 client，弱类型会爆 |
| 路由 | **Vue Router 4** | 官方 |
| 状态 | **Pinia 2** | 官方 |
| HTTP | **Axios** + generated client | 见下 |
| UI 库 | **Naive UI**（推荐） | Vue 3 原生、TypeScript 优先、无障碍良好；**不用 Element Plus**（视觉风格偏 admin，与官网调性不同） |
| 图 | **ECharts 5**（推荐） | 时序图、桑基图、热力图都现成；体积大但按需引入 |
| 表格 | **vxe-table**（推荐） | 大数据量（Session 列表 10w+ 行）虚拟滚动 |
| 表单 | **VeeValidate 4** | 写端点（M1+）需要 |
| 测试 | **Vitest** + **Vue Test Utils** | Vite 生态 |
| E2E | **Playwright** | 录 3 个 Demo 的 happy path |

### 6.2 自动生成 API Client

```bash
# 后端起 oryxos serve 后
npx openapi-typescript-codegen \
  --input http://localhost:8080/v3/api-docs \
  --output ./src/api/generated \
  --client axios
```

→ 生成的 `SessionsService.ts` / `AgentsService.ts` 强类型、自动跟随后端契约。

### 6.3 目录结构

```
oryxos-admin/                          # 独立 repo，monorepo 后续评估
├── package.json
├── vite.config.ts
├── tsconfig.json
├── index.html
├── public/
├── src/
│   ├── main.ts
│   ├── App.vue
│   ├── router/
│   │   └── index.ts
│   ├── stores/                        # Pinia
│   │   ├── sessions.ts
│   │   ├── agents.ts
│   │   └── ...
│   ├── api/
│   │   ├── generated/                 # 自动生成
│   │   └── http.ts                    # Axios 实例 + 拦截器
│   ├── pages/
│   │   ├── Dashboard.vue
│   │   ├── agents/
│   │   │   ├── List.vue
│   │   │   └── Detail.vue
│   │   ├── sessions/
│   │   │   ├── List.vue
│   │   │   └── Detail.vue
│   │   ├── tools/
│   │   ├── providers/
│   │   └── schedules/
│   ├── components/
│   │   ├── StatTile.vue
│   │   ├── EventStream.vue
│   │   ├── TimelineCard.vue           # Session 详情核心
│   │   ├── ToolSchemaDrawer.vue
│   │   └── ...
│   ├── lib/
│   │   ├── format.ts                  # 时间 / 数字 / token 费用
│   │   ├── polling.ts                 # 轮询 hook
│   │   └── theme.ts                   # 调色板（与官网一致）
│   └── styles/
│       ├── tokens.css                 # CSS 变量
│       └── global.css
├── tests/
│   ├── unit/
│   └── e2e/
└── README.md
```

---

## 7. API 集成

### 7.1 10 个 REST 端点 → 页面映射

| 端点 | 调用页面 | 轮询频率 |
|------|---------|---------|
| `GET /api/v1/health` | Dashboard | 30s |
| `GET /api/v1/info` | Dashboard / Providers | 5min |
| `GET /api/v1/profiles` | Agent 列表 | 60s |
| `GET /api/v1/profiles/{name}` | Agent 详情 | 5s |
| `GET /api/v1/sessions` | Session 列表 | 5s |
| `GET /api/v1/sessions/{id}` | Session 详情 | 5s（仅 ⏳ 状态） |
| `POST /api/v1/sessions/{id}/messages` | Session 详情（继续对话） | - |
| `DELETE /api/v1/sessions/{id}` | Session 列表（删除） | - |
| `GET /api/v1/memory` | Memory 浏览器（M1） | 30s |
| `GET /api/v1/tools` | Tool 浏览器 | 60s |
| `POST /api/v1/agents/{name}/invoke` | Agent 详情（手动触发） | - |

### 7.2 错误处理

```typescript
// api/http.ts
axios.interceptors.response.use(
  (resp) => resp,
  (err) => {
    const status = err.response?.status
    if (status === 401) router.push('/login')  // M1 接入鉴权
    if (status === 404) toast.error('资源不存在')
    if (status === 500) toast.error('后端异常，请看日志')
    if (status === 503) toast.error('OryxOS 正在启动')
    return Promise.reject(err)
  }
)
```

### 7.3 实时性

| 数据 | 频率 | 原因 |
|------|------|------|
| Session 详情（⏳ 状态） | 5s | 有正在跑的调用，5s 合理 |
| Session 列表 | 5s | 表格行数少，5s 不卡 |
| Dashboard tiles | 30s | 数字不需要秒级 |
| Provider 列表 | 5min | 健康状态变化慢 |
| Tool 列表 | 60s | 同上 |

> **不引入 SSE / WebSocket**：核心阶段同步阻塞 + 轮询足够；M3 评估 SSE（按 [plan.md §Performance Goals](../../specs/008-agent-web-service/plan.md) virtual threads 100+ 并发）。

---

## 8. 组件库

### 8.1 通用组件（自研，~15 个）

| 组件 | 复用 | 说明 |
|------|------|------|
| `<StatTile>` | Dashboard | 大数字 + 同比箭头 + 颜色 |
| `<EventStream>` | Dashboard | 时间倒序列表 + 状态徽章 |
| `<TimelineCard>` | Session 详情 | 单 Step 卡片（折叠 / 展开） |
| `<ToolSchemaDrawer>` | Tool 浏览器 | 右侧抽屉 + JSON Schema 渲染 |
| `<MarkdownView>` | AGENT.md / Memory | 复用官网 markdown 样式 |
| `<EmptyState>` | 所有列表 | 空数据占位 |
| `<ErrorState>` | 所有列表 | 加载失败占位 |
| `<RelativeTime>` | 所有时间字段 | 「3 分钟前」自动更新 |
| `<TokenCount>` | LLM 成本 | 数字 + 美元 / 人民币换算 |
| `<StatusBadge>` | 多处 | 统一状态颜色规范 |

### 8.2 业务组件

- `<AgentInvocationForm>` —— 手动触发测试的弹框
- `<SessionFilter>` —— Session 列表筛选（按 Agent / 触发源 / 状态 / 时间区间）
- `<CronInput>` —— 定时任务页（M1），复用 `cron-parser` 解析 + 人类可读预览

### 8.3 样式规范

```css
/* styles/tokens.css */
:root {
  --color-bg: #0f1115;             /* 深色为主（与官网一致） */
  --color-surface: #161922;
  --color-border: #232735;
  --color-text-primary: #e6e9ef;
  --color-text-secondary: #9ba3b4;
  --color-accent: #7c5cff;         /* 紫（官网主色） */
  --color-success: #34d399;
  --color-warning: #fbbf24;
  --color-danger: #f87171;
  --radius-sm: 4px;
  --radius-md: 8px;
  --font-mono: "JetBrains Mono", "SF Mono", monospace;
  --font-sans: "Inter", -apple-system, "PingFang SC", sans-serif;
}
```

视觉对标 [website/](../../website/) 的深色科技感。

---

## 9. 状态管理（Pinia）

```typescript
// stores/sessions.ts
export const useSessionsStore = defineStore('sessions', () => {
  const items = ref<SessionDto[]>([])
  const loading = ref(false)
  const error = ref<string | null>(null)
  const filter = ref<{ agentName?: string; status?: string }>({})

  const filtered = computed(() =>
    items.value.filter(s =>
      (!filter.value.agentName || s.agentName === filter.value.agentName) &&
      (!filter.value.status || s.status === filter.value.status)
    )
  )

  async function fetch() { /* ... */ }

  return { items, loading, error, filter, filtered, fetch }
})
```

**核心原则**：
- 每个领域一个 store
- API 调用都走 generated client（保证类型）
- 轮询逻辑在 `usePolling` composable，不耦合到 store

```typescript
// lib/polling.ts
export function usePolling(fn: () => Promise<void>, intervalMs: number) {
  const { pause, resume } = useDocumentVisibility()  // 切到后台自动暂停
  let timer: number | null = null

  function start() {
    timer = window.setInterval(fn, intervalMs)
  }
  function stop() {
    if (timer) clearInterval(timer)
  }

  onMounted(start)
  onUnmounted(stop)
  onDeactivated(pause)
  onActivated(resume)

  return { stop, pause, resume }
}
```

---

## 10. 鉴权与多租户

### 10.1 MVP（核心阶段）

- **无鉴权**：[plan.md §Security](../../specs/008-agent-web-service/plan.md) 明确写鉴权 None
- 部署在企业内部，前置 API gateway（Nginx / Spring Cloud Gateway）做 SSO 透传
- 平台默认监听 `127.0.0.1:8080`，只本机能访问

### 10.2 M1（扩展阶段）

- 接入 OIDC / SSO（Keycloak / Authing）
- token 存 `httpOnly` cookie
- router.beforeEach 拦截无 token 跳转 `/login`
- 每个 API 请求带 `Authorization: Bearer <token>`
- 后端解析 token → 注入 `tenantId` → 多租户隔离

### 10.3 M2（多租户）

- 新增 `tenantId` 列到 5 张表
- 后端强制 `WHERE tenant_id = ?`
- 前端顶部租户切换器

---

## 11. 视觉规范

参考 [website/docs/index.md](../../website/)：
- **主色**：紫 `#7c5cff`（官网 Hero 用色）
- **次色**：青 `#00d4ff`（数据 / 图表）
- **背景**：`#0f1115` 深色
- **字体**：Inter (sans) + JetBrains Mono (code)
- **图标**：Lucide

整体观感：**深色 + 紫青双色 + 大量留白** —— 区别于传统 admin 后台的灰白风格。

---

## 12. 可达性 / 国际化

### 12.1 a11y

- 所有交互元素键盘可达（Tab 顺序）
- 状态徽章配 `aria-label`
- 颜色不是唯一信号（错误还要有图标 + 文字）
- 目标：用 NVDA / VoiceOver 全部页面可读

### 12.2 i18n

- Vue I18n
- 文案 key 命名：`session.detail.step.userInput`
- MVP 只交付中文（与项目其他文档一致）
- M2 接入英文

---

## 13. 实施阶段

### 13.1 路线图（与 [CLAUDE.md §3](../../CLAUDE.md) 核心/扩展划分对齐）

```
M0 ── MVP 只读第一版 ───────────────────────────────────────────
  ✅ 8 个只读页面
  ✅ 8 个核心 REST 端点对接
  ✅ 深色视觉规范建立
  ✅ 1 个 monorepo 工程脚手架
  ⚠ 新增 3 个聚合端点（Dashboard / Agent 详情 / Provider）
  📅 预计 1 周

M1 ── 写能力 + 鉴权 ────────────────────────────────────────────
  - 写端点（Profile / Schedule / Agent 上传）
  - OIDC / SSO 接入
  - Memory 浏览器（独立页）
  - 审计导出（CSV / JSON）
  📅 预计 2 周

M2 ── 多租户 + 高级特性 ──────────────────────────────────────
  - 多租户隔离
  - 实时 SSE（Agent 详情）
  - 全文搜索（Session 内容）
  - 英文 i18n
  📅 预计 3 周

M3 ── 集群 + 高可用 ──────────────────────────────────────────
  - 多节点 Dashboard
  - 集群健康监控
  - 告警接入（飞书 / 企微 / PagerDuty）
  📅 预计 4 周
```

### 13.2 M0 任务切片（≤ 30 个）

| ID | 任务 | 估时 |
|----|------|------|
| M0-01 | 初始化 Vite + Vue 3 + TS 工程 | 0.5d |
| M0-02 | 接入 Naive UI + 主题 token | 0.5d |
| M0-03 | 接入 Axios + 错误拦截器 | 0.5d |
| M0-04 | 生成 API client（openapi-typescript-codegen） | 0.5d |
| M0-05 | 路由 + 顶级导航 | 0.5d |
| M0-06 | StatTile / EventStream 等通用组件 | 1d |
| M0-07 | Dashboard 页 + 聚合端点 stub | 1d |
| M0-08 | Agent 列表 + 详情页 | 1d |
| M0-09 | Session 列表 + TimelineCard | 1.5d |
| M0-10 | Session 详情（5 步时间线） | 1.5d |
| M0-11 | Tool 列表 + SchemaDrawer | 1d |
| M0-12 | Provider 列表 + 时序图 | 1d |
| M0-13 | 定时任务只读页 | 0.5d |
| M0-14 | Pinia stores + usePolling | 1d |
| M0-15 | 视觉规范 token + 全局样式 | 0.5d |
| M0-16 | 主题适配（明亮 / 深色切换） | 0.5d |
| M0-17 | Playwright E2E 3 个 Demo 场景 | 1d |
| M0-18 | README + 开发指南 | 0.5d |
| M0-19 | 部署文档（Docker / Nginx） | 0.5d |
| M0-20 | 演示录屏 + 截图 | 0.5d |

合计 ~15d = 3 周（单人）。

### 13.3 依赖与阻塞

- **阻塞 #1**：[§5.1 方案 A](#51-dashboard-总览首屏) 提议的聚合端点 —— 需在 [specs/008-agent-web-service/plan.md](../../specs/008-agent-web-service/plan.md) US-4 补 3 个聚合端点
- **阻塞 #2**：核心阶段没有「Provider 列表」端点，仅 `/info` 包含基础 —— 需新增 `GET /api/v1/providers` 或扩展 `/info`
- **依赖 #1**：工程搭建依赖 [website/](../../website/) 的 VitePress 视觉规范沉淀

---

## 14. 风险与开放问题

### 14.1 风险

| 风险 | 概率 | 影响 | 缓解 |
|------|------|------|------|
| 后端聚合端点耗时长导致 Dashboard 慢 | 中 | 高 | 后端聚合 + 缓存（5s）；前端骨架屏 |
| 完整 Prompt 渲染泄露 | 中 | 中 | 默认折叠 + 摘要 |
| ECharts 体积大（~1MB） | 低 | 低 | 按需引入 |
| 视觉风格与官网不一致 | 中 | 中 | 复用官网调色板 + 设计评审 |
| 时区漂移（Session 时间） | 中 | 中 | 统一用 UTC 存储，前端按浏览器时区显示 |
| Token 成本估算不准 | 中 | 低 | 用官方公开报价 + 后端可配置 |

### 14.2 开放问题（待你拍板）

- [ ] **Q1**：[§5.1 Dashboard 方案 A](#51-dashboard-总览首屏) 是否在 008-agent-web-service 追加 3 个聚合端点？还是单独立项 008b ？
- [ ] **Q2**：UI 库选 **Naive UI**（推荐）vs **Element Plus**（更主流）vs **自研 + Headless UI**（最自由）？
- [ ] **Q3**：M0 是否纳入「核心阶段」？如要纳入宪法 §III 要改：「... + Web Service + 第一版只读管理平台」
- [ ] **Q4**：是否要支持英文？影响 M0 文案 key 命名（建议 MVP 留 key，M2 翻译）
- [ ] **Q5**：[website/](../../website/) 现在是 VitePress 文档站，不适合改造成 SPA。是新建 `oryxos-admin/` 仓库，还是 `website/` 同 repo 下加 `admin/` 子目录？
- [ ] **Q6**：M0 的 ECharts（~1MB）vs Chart.js（~200KB）vs SVG 自研？数据量小时 Chart.js 够用

---

## 15. 验收标准（M0）

| 维度 | 验收 |
|------|------|
| 功能 | 8 个页面全部能打开，能看到 mock 数据 |
| 数据 | 全部走 10 个 REST + 3 个聚合端点 |
| 视觉 | 与 [website/](../../website/) 调色板和字体一致 |
| 性能 | 首屏 LCP ≤ 2s（4G 网络），Session 详情切换 ≤ 200ms |
| 可达性 | NVDA 可读，关键操作键盘可达 |
| 测试 | 单元覆盖 ≥ 70%，E2E 3 个 Demo 场景全绿 |
| 部署 | `docker run` 一行启动，访问 `http://localhost:5173` 即可 |

---

## 附录 A：Mock 数据预览

`yarn dev` 启动时若后端未起，启用 MSW（Mock Service Worker）拦截 `/api/v1/*`，返回 mock 数据 —— 方便前端独立开发。

```typescript
// mocks/handlers.ts
import { http, HttpResponse } from 'msw'

export const handlers = [
  http.get('/api/v1/sessions', () => HttpResponse.json({
    items: [
      { id: 's-91abc', agentName: 'agent-weather', source: 'scheduler', status: 'success', messageCount: 4, startedAt: '2026-07-29T14:23:00Z' },
      { id: 's-89def', agentName: 'agent-3',       source: 'api',      status: 'failed',  messageCount: 12, startedAt: '2026-07-29T14:15:00Z' },
    ],
    total: 42,
  })),
  // ...
]
```

## 附录 B：相关文档

- [CLAUDE.md](../../CLAUDE.md) — 项目主记忆
- [docs/IndustryResearch.md](../../docs/IndustryResearch.md) — Why
- [docs/DemandAnalysis.md](../../docs/DemandAnalysis.md) — What
- [docs/TechnicalSolution.md](../../docs/TechnicalSolution.md) — How
- [specs/008-agent-web-service/spec.md](../../specs/008-agent-web-service/spec.md) — Web Service 规格
- [specs/008-agent-web-service/plan.md](../../specs/008-agent-web-service/plan.md) — Web Service 实施计划
- [website/](../../website/) — 官网 + 文档站（视觉规范来源）
- [docs/class/第26节：Web Service 与第一版管理平台 实现与代码讲解.md](../../docs/class/第26节：Web%20Service%20与第一版管理平台%20实现与代码讲解.md) — 课程章节

---

> **下一步**：根据 [§14.2 开放问题](#142-开放问题待你拍板) 给方向，落到具体 spec 切片。
