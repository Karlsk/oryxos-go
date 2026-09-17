# Provider 手动测试指南

本文用于人工验证第 16 节交付的 DeepSeek 与 MiniMax Provider，覆盖真实凭证注入、网络连通、Provider 选路、同步 Generate、流式 Stream、Tool Calling 响应保留以及 `llm_calls` 审计写入。

## 1. 当前测试边界

第 16 节只交付 Provider、Profile 配置边界和模型调用审计。Provider 尚未接入后续课程的 `AgentService`、ReAct Loop 和 CLI 对话链路，因此当前不要使用 `oryxos serve` 或 `oryxos chat` 判断 Provider 是否连通。

真实 Provider 的测试入口是带 `integration` 构建标签的 `TestProviderSmoke`：

```bash
go test -tags=integration ./internal/provider -run '^TestProviderSmoke$' -count=1 -v
```

该测试不会执行 Tool。它把 `echo` Tool schema 交给模型，对每个 Provider 分别执行一次 Generate 和一次 Stream，检查 Tool Call ID、流式 completed 完整响应以及每次逻辑调用一条审计记录。

## 2. 前置条件

在仓库根目录执行以下检查：

```bash
go version
go list -m \
  github.com/cloudwego/eino \
  github.com/cloudwego/eino-ext/components/model/deepseek \
  github.com/cloudwego/eino-ext/components/model/openai
CGO_ENABLED=0 go test ./internal/provider ./internal/store
```

预期条件：

- Go 版本为 1.26 或更高；
- Eino core 为 `v0.9.19`；
- DeepSeek connector 为 `v0.1.7`；
- OpenAI connector 为 `v0.1.13`；
- 本机能够访问 DeepSeek 和 MiniMax API；
- DeepSeek Key 有权调用冒烟测试模型 `deepseek-flash`；
- MiniMax Key 有权调用 `MiniMax-M3`；
- MiniMax Key 来自中国站开放平台，并可使用固定的 OpenAI 兼容地址 `https://api.minimax.cn/v1`。

如果公司网络需要代理，应在运行测试前按组织安全规范配置代理。不要把代理凭证写进仓库。

## 3. Provider 配置规则

实例级 Provider 声明只允许 `name` 和 `api_key`：

```yaml
providers:
  - name: deepseek
    api_key: ${DEEPSEEK_API_KEY}
  - name: minimax
    api_key: ${MINIMAX_API_KEY}
```

Profile 只选择 Provider、模型和 temperature：

```yaml
name: provider-smoke

provider:
  name: deepseek
  model: deepseek-flash
  temperature: 0
```

必须遵守以下边界：

- 不要在 Profile 中填写真实 Key；
- 不要配置 `base_url`；该字段会被严格加载器拒绝；
- DeepSeek 工厂使用原生 connector 的官方默认地址；
- MiniMax 工厂内部固定 OpenAI 兼容地址；
- 切换 Provider 时修改 `provider.name` 和对应模型，不修改 endpoint。

当前冒烟测试直接从环境变量读取真实 Key，并构造与上述 YAML 等价的 Provider 声明和 Profile。无需创建或提交包含真实凭证的配置文件。

## 4. 注入真实 Key

可以从 `.env` 批量加载，也可以使用 `read` 临时输入。两种方式最终都必须把 Key 导出为当前测试进程可见的环境变量。

### 4.1 从 `.env` 加载

仓库根目录的 `.env` 使用以下变量名，值建议用单引号包裹：

```dotenv
DEEPSEEK_API_KEY='<真实 Key>'
MINIMAX_API_KEY='<真实 Key>'
```

不要提交该文件。加载前确认它已被 Git 忽略，并限制只有当前用户可以读取：

```bash
git check-ignore -q .env && echo '.env 已被 Git 忽略'
chmod 600 .env
```

`go test` 不会自动读取 `.env`。在仓库根目录把其中的变量导出到当前 shell：

```bash
set -a
source .env
set +a
```

`source` 会执行文件中的 shell 语法，因此只允许加载自己创建并确认可信的 `.env`。加载操作不会打印变量值；后续测试必须在同一个 shell 中运行。

### 4.2 使用 `read` 临时输入

不希望把 Key 保存到文件时，可以在当前终端静默输入。Key 不会回显，也不会进入 shell 历史：

```bash
printf '请输入 DEEPSEEK_API_KEY: ' >&2
IFS= read -r -s DEEPSEEK_API_KEY
printf '\n' >&2
export DEEPSEEK_API_KEY

printf '请输入 MINIMAX_API_KEY: ' >&2
IFS= read -r -s MINIMAX_API_KEY
printf '\n' >&2
export MINIMAX_API_KEY
```

后续需要录入其他敏感环境变量时，可以复用下面的函数：

```bash
read_secret() {
  local variable_name="$1"
  printf '请输入 %s: ' "$variable_name" >&2
  IFS= read -r -s "$variable_name"
  printf '\n' >&2
  export "$variable_name"
}

read_secret DEEPSEEK_API_KEY
read_secret MINIMAX_API_KEY
```

这种方式只影响当前 shell：关闭终端或执行 `unset` 后变量即消失，不会修改 `.env`。如果先加载了 `.env`，随后又用 `read` 输入同名变量，当前 shell 将使用后输入的值，但 `.env` 文件不会被改写。

### 4.3 确认变量已设置

只检查变量是否存在，不要打印变量值：

```bash
test -n "${DEEPSEEK_API_KEY:-}" && echo 'DeepSeek Key: 已设置' || echo 'DeepSeek Key: 缺失'
test -n "${MINIMAX_API_KEY:-}" && echo 'MiniMax Key: 已设置' || echo 'MiniMax Key: 缺失'
```

如果只测试一个 Provider，只加载或输入对应变量即可；未设置变量的子测试会显示 `SKIP`，不会发起网络请求。

## 5. 分别验证连通性

### 5.1 DeepSeek

```bash
go test -tags=integration ./internal/provider \
  -run 'TestProviderSmoke/deepseek' \
  -count=1 -v
```

期望结果：

```text
=== RUN   TestProviderSmoke/deepseek
--- PASS: TestProviderSmoke/deepseek
```

该用例验证：

- 使用 `deepseek` 工厂；
- 使用 DeepSeek 原生 connector，OryxOS 不设置 `BaseURL`；
- 调用模型 `deepseek-flash`；
- Generate 和 Stream 都返回非空 Tool Call，并保留 Tool Call ID；
- Stream 按 delta → completed → `io.EOF` 结束，completed 中包含合并后的完整 Tool Call；
- 临时 SQLite 中有两条成功 `llm_calls`，分别对应 Generate 和 Stream，chunk 不重复落账。

### 5.2 MiniMax

```bash
go test -tags=integration ./internal/provider \
  -run 'TestProviderSmoke/minimax' \
  -count=1 -v
```

期望结果：

```text
=== RUN   TestProviderSmoke/minimax
--- PASS: TestProviderSmoke/minimax
```

该用例验证：

- 使用 `minimax` 工厂；
- 实际 connector 是 Eino-ext OpenAI connector；
- 工厂固定使用 `https://api.minimax.cn/v1`；
- 调用模型 `MiniMax-M3`；
- Generate 和 Stream 的 Tool Call ID 都被完整保留；
- Stream 按 delta → completed → `io.EOF` 结束，completed 中包含合并后的完整 Tool Call；
- 临时 SQLite 中有两条成功 `llm_calls`，分别对应 Generate 和 Stream，chunk 不重复落账。

## 6. 一次验证两个 Provider 的选路

两个 Key 都已设置后执行：

```bash
go test -tags=integration ./internal/provider \
  -run '^TestProviderSmoke$' \
  -count=1 -v
```

预期两个子测试都通过：

```text
--- PASS: TestProviderSmoke/deepseek
--- PASS: TestProviderSmoke/minimax
--- PASS: TestProviderSmoke
```

这证明两个明确命名的 Provider 都能通过各自工厂完成真实请求。再运行确定性路由回归，验证同一 Provider 的多个 Profile 不共享模型配置，并验证配置重载会切换绑定：

```bash
go test ./internal/provider \
  -run 'TestRegistryRoutesAndIsolatesProfiles|TestRegistryBindProfilesAtomicallyRebuildsCurrentSnapshot' \
  -count=1 -v
```

选路矩阵如下：

| Profile 选择 | connector | endpoint 策略 | 测试模型 |
|---|---|---|---|
| `deepseek` | DeepSeek 原生 connector | connector 官方默认地址 | `deepseek-flash` |
| `minimax` | OpenAI connector | 工厂固定 `https://api.minimax.cn/v1` | `MiniMax-M3` |

测试中不存在 fallback。任一 Provider 调用失败时，不会自动改用另一个 Provider。

## 7. 验证严格配置与凭证边界

运行配置和 Profile 回归：

```bash
go test ./internal/config ./internal/profile \
  -run 'TestLoadServerYAMLProviders|TestLoadServerYAMLRejectsInvalidProviders|TestLoaderRejectsInvalidProviderChoicesPerFile' \
  -count=1 -v
```

这些用例应证明：

- `${DEEPSEEK_API_KEY}` 和 `${MINIMAX_API_KEY}` 可从环境展开；
- 重复或不支持的 Provider 会失败；
- 实例配置中的 `base_url` 会失败；
- Profile 中的 `api_key` 或 `base_url` 会失败；
- 错误信息不会打印真实凭证。

## 8. 结果判定

只有满足下列条件，才能认为人工验证通过：

- DeepSeek 子测试为 `PASS`，而不是 `SKIP`；
- MiniMax 子测试为 `PASS`，而不是 `SKIP`；
- 两个子测试分别使用预期模型；
- 两个 Provider 的 Generate 与 Stream 响应都包含非空 Tool Call ID；
- Stream 都产生 completed 完整响应并随后返回 `io.EOF`；
- 每次逻辑模型调用都写入一条成功的 `llm_calls` 记录，Stream chunk 不重复写入；
- 路由与配置回归测试全部通过；
- 输出中没有真实 API Key。

建议保存以下非敏感证据：测试日期、Git commit、Go 版本、Provider 名、模型名和测试结果。不要保存 Key、Authorization Header 或带凭证的 URL。

## 9. 常见失败与处理

| 现象 | 常见原因 | 处理 |
|---|---|---|
| 子测试显示 `SKIP` | 对应环境变量未设置或未导出 | 重新静默输入并 `export`，再检查变量是否非空 |
| `401` / `403` | Key 无效、权限不足或账号区域不匹配 | 在厂商控制台确认 Key、模型权限和账号区域；不要通过新增 `base_url` 绕过 |
| 模型不存在或 `404` | 账号未开通文档中的模型名 | 核对账号可用模型；若需修改课程固定模型，先走需求确认 |
| 连接超时、DNS 或 TLS 错误 | 网络、代理、防火墙或证书问题 | 验证到厂商官方域名的网络路径，按组织规范配置代理和证书 |
| 模型直接回答，没有 Tool Call | 模型或账号未按预期支持当前 Tool Calling 请求 | 保留完整错误输出，核对模型能力；不得放宽 Tool Call 断言制造通过 |
| 调用失败且审计检查也失败 | SQLite 临时库写入或迁移异常 | 先运行 `CGO_ENABLED=0 go test ./internal/store -count=1 -v` |
| 输出疑似包含凭证 | 上游错误格式未被脱敏规则覆盖 | 立即停止共享日志，撤销并轮换 Key，再补充脱敏回归测试 |

## 10. 测试后清理

测试结束后从当前 shell 删除凭证：

```bash
unset DEEPSEEK_API_KEY MINIMAX_API_KEY
```

再次确认变量已清理：

```bash
test -z "${DEEPSEEK_API_KEY:-}" && echo 'DeepSeek Key: 已清理'
test -z "${MINIMAX_API_KEY:-}" && echo 'MiniMax Key: 已清理'
```

`unset` 只清理当前 shell，不会修改 `.env`。继续保留 `.env` 时，应保持 `0600` 权限和 Git 忽略状态；不再需要时可由用户自行安全删除。

冒烟测试数据库位于 Go 测试创建的临时目录，测试结束后自动清理，不会写入 `.oryxos/sessions/oryxos.db`。
