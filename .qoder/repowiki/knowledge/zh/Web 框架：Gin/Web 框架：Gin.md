---
kind: external_dependency
name: Web 框架：Gin
slug: gin
category: external_dependency
category_hints:
    - vendor_identity
scope:
    - '**'
---

OryxOS 的 Web Service 基于 Gin（github.com/gin-gonic/gin）实现，提供核心阶段 10 个 REST 端点（会话管理、Agent 调用、Profile/Memory/Tool 列表、health/info）。Handler 层按 Session/Agent/Profile/Memory/Tool/System 拆分，仅做协议解析与校验，不直接访问 GORM、Provider 或 Tool。构建时通过 CGO_ENABLED=0 输出单二进制，Gin 作为唯一 HTTP 框架依赖。