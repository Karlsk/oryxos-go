---
kind: external_dependency
name: MCP 客户端：官方 Go SDK
slug: modelcontextprotocol-go-sdk
category: external_dependency
category_hints:
    - vendor_identity
    - sdk_real_api
scope:
    - '**'
---

通过官方 MCP Go SDK github.com/modelcontextprotocol/go-sdk/mcp 连接外部 MCP server，支持 stdio（command+args+env）与 remote（url+bearer_token）两种 transport。MCP Server 配置集中在 .oryxos/mcp_servers.yaml，Profile 的 mcp_servers 字段仅引用 name；启动时按 Profile 引用建立或复用 client，进程关闭时统一释放。MCP 提供的 Tools 必须经过名称冲突检查、Profile 可用 Tool 列表过滤，并走统一 ToolRegistry 与调用记录入口。