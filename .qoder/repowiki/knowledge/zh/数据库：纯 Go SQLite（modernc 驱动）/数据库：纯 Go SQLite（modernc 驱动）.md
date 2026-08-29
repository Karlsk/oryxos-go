---
kind: external_dependency
name: 数据库：纯 Go SQLite（modernc 驱动）
slug: sqlite-modernc
category: external_dependency
category_hints:
    - client_constraint
scope:
    - '**'
---

通过 GORM 搭配 github.com/glebarez/sqlite 使用纯 Go 的 modernc.org/sqlite 驱动，数据文件位于 .oryxos/sessions/oryxos.db。核心阶段仅三张表：sessions、tool_invocations、llm_calls。禁止替换为依赖 CGO 的 mattn/go-sqlite3，发布构建必须通过 CGO_ENABLED=0 go build ./cmd/oryxos 验证单二进制。启用 WAL 与合理 busy_timeout，不在事务内等待外部 LLM/Tool 调用。