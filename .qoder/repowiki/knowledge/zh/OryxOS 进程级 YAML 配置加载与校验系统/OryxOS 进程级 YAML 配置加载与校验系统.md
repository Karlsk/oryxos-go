---
kind: configuration_system
name: OryxOS 进程级 YAML 配置加载与校验系统
category: configuration_system
scope:
    - '**'
source_files:
    - internal/config/config.go
    - internal/config/load.go
    - internal/config/expand.go
    - internal/config/redact.go
    - internal/config/config_test.go
    - internal/config/redact_test.go
    - internal/app/foundation.go
---

## 1. 使用的系统与方案

OryxOS 的运行时配置采用**单一 YAML 文档 + 环境变量插值 + 严格解码 + 结构化默认值**的方案，核心位于 `internal/config` 包：
- 使用 `gopkg.in/yaml.v3` 对 YAML 进行 AST 级别的遍历、展开与严格解码。
- 通过注入的 `lookupEnv func(string) (string, bool)` 支持在 YAML 标量中用 `${ENV_VAR}` 语法引用环境变量。
- 使用 `yaml.Decoder.KnownFields(true)` 实现未知字段拒绝（strict decode），配合自定义错误路径定位，将解析错误精确映射到 YAML 路径。
- 所有超时类字段统一以 Go `time.ParseDuration` 解析并强制为正数；`listen_address` 必须为合法的 `host:port`。
- 提供敏感字段脱敏能力（`redact.go`），用于日志/错误输出时抹除密钥。

该配置系统仅面向进程级 HTTP Server 运行参数（监听地址、日志格式、HTTP 超时、优雅关闭超时），不承载 Agent / Skill / Tool 等业务配置。作者指南明确“配置变更核心阶段重启生效，不实现文件监听或热重载”，因此本仓库未实现配置热更新机制。

## 2. 关键文件与职责

| 文件 | 职责 |
|---|---|
| `internal/config/config.go` | 定义对外类型 `ServerConfig`、`LogFormat` 及内部 raw 结构体（含 yaml tag） |
| `internal/config/load.go` | 入口 `LoadServerYAML`：单文档校验 → 去重 → 变量展开 → 形状校验 → 严格解码 → 默认值填充 → 业务校验 |
| `internal/config/expand.go` | 递归遍历 YAML AST，将 `${NAME}` 占位符替换为 `lookupEnv(NAME)` 返回值，并对变量名做 ASCII+下划线合法性检查 |
| `internal/config/redact.go` | 敏感键匹配（`api_key`、`password`、`secret`、`token`、`authorization`、`mcp_auth`、`webhook_url` 等）、URL 凭据检测、错误字符串脱敏 |
| `internal/app/foundation.go` | 装配点：从 `FoundationOptions.ServerYAML` 读取字节流，调用 `config.LoadServerYAML`，并将结果注入 Web Server 与 Logger |
| `internal/config/config_test.go` | 覆盖默认值、部分覆盖、变量展开、缺失变量、非法时长、零/负超时、未知字段等场景 |
| `internal/config/redact_test.go` | 验证敏感键识别与 URL 凭据脱敏行为 |

## 3. 架构与约定

### 3.1 加载流水线
`LoadServerYAML(data, lookupEnv)` 严格按以下顺序执行，任一阶段失败即返回带路径信息的 `configError`：
1. `decodeSingleDocument`：只允许恰好一个 YAML 文档，多余文档视为错误。
2. `rejectDuplicateKeys`：递归扫描 MappingNode，发现重复 key 即报错。
3. `expandScalars`：在解码前把 `${VAR}` 全部展开，要求变量存在且名称合法（ASCII 字母/数字/下划线，首字符非数字）。
4. `validateYAMLShape`：白名单式校验顶层 key 集合（`listen_address`、`log_format`、`http`、`shutdown_timeout`），并限定 `http.*` 子域。
5. `decodeStrictRaw`：先 encode 再 decode，开启 `KnownFields(true)`，未知字段会触发 `unknown field "..."` 错误，并通过行号回溯到具体 YAML 路径。
6. `validateServerConfig`：应用默认值（`127.0.0.1:8080`、`console`、各超时默认值）并做业务校验（端口范围、正数时长、枚举值）。

### 3.2 环境变量插值约定
- 仅在**标量字符串**中支持 `${NAME}` 形式的环境变量引用。
- 若 `lookupEnv` 为 nil 或未找到对应变量，返回 `environment variable NAME is not set` 错误。
- 变量名必须符合 ASCII 字母/数字/下划线规则，由 `validEnvironmentName` 判定。

### 3.3 默认值策略
所有超时字段都有硬编码默认值（read_header_timeout=5s、read_timeout=30s、write_timeout=5m、idle_timeout=60s、shutdown_timeout=30s），`listen_address` 默认为 `127.0.0.1:8080`，`log_format` 默认为 `console`。空 YAML 或空文档等价于使用全部默认值。

### 3.4 敏感信息保护
`redact.go` 提供三类保护：
- `IsSensitiveKey(path, key)`：当 key 或其任意祖先 path segment 命中敏感词（如 `api_key`、`password`、`secret`、`token`、`authorization`、`mcp_auth`、`webhook_url`、包含 `apikey`/`credential`/`mcp_auth`/`webhook` 的字段）时标记为敏感。
- `RedactValue(key, value)`：将敏感字段的值替换为 `[REDACTED]`。
- `SanitizeErrorString(text)`：在错误消息中自动抹除形如 `key=value`、`user:pass@host`、`Bearer ...` 等凭据片段。

### 3.5 与应用的集成方式
`internal/app/foundation.go` 中的 `NewFoundation` 是唯一的配置消费入口：它从 `FoundationOptions` 获取原始 YAML 字节和 `LookupEnv`，调用 `config.LoadServerYAML` 后，将得到的 `ServerConfig` 同时用于构造 `web.NewServer` 和选择日志格式（JSON vs Console）。测试通过注入 `LookupEnv` 与 `ListenerFactory` 来隔离外部依赖。

## 4. 约定与约束

- **单一 YAML 文档**：配置只能是一个 YAML 文档，多文档会被拒绝（`must contain exactly one YAML document`）。
- **禁止未知字段**：任何未在 `rawServerConfig` 中声明的 key 都会导致启动失败，防止拼写错误或废弃配置静默被忽略。
- **白名单式 schema 校验**：顶层 key 仅限 `listen_address`、`log_format`、`http`、`shutdown_timeout`；`http` 下仅限 `read_header_timeout`、`read_timeout`、`write_timeout`、`idle_timeout`。
- **时长必须为正**：所有 duration 字段经 `time.ParseDuration` 解析后必须 `> 0`，否则报 `must be a Go duration greater than zero`。
- **监听地址格式**：必须为 `host:port`，端口范围 1–65535。
- **日志格式枚举**：`log_format` 仅接受 `console` 或 `json`。
- **环境变量不可为空**：`${VAR}` 引用必须解析为非空字符串；缺失变量直接报错，不支持空值回退。
- **无热重载**：作者指南明确要求“配置变更核心阶段重启生效，不实现文件监听或热重载”，当前实现也仅支持进程启动时一次性加载。
- **敏感信息不出错**：所有可能泄露凭据的错误消息都会经 `SanitizeErrorString` 处理，确保日志/错误输出不含明文密钥。
- **依赖方向**：`cmd` → `app` → `config`，config 包不依赖 web/Cobra/Gin，保持可独立测试与复用。