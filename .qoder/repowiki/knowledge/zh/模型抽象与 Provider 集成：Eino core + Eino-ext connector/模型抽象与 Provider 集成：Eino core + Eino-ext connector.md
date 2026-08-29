---
kind: external_dependency
name: 模型抽象与 Provider 集成：Eino core + Eino-ext connector
slug: eino-eino-ext
category: external_dependency
category_hints:
    - framework_behavior
    - sdk_real_api
scope:
    - '**'
---

运行时只依赖 Eino core 的 model.ToolCallingChatModel 统一接口；具体厂商接入通过 Eino-ext connector（DeepSeek connector、OpenAI connector 用于 MiniMax OpenAI 兼容路径）完成，工厂按 provider.name 注册并按 Profile.name 保存模型实例。Provider 适配层不得暴露 Eino-ext 具体类型给 Handler/Scheduler/Tool，也不得直接依赖 ADK Agent 封装。首批跑通 DeepSeek 原生 connector 与 MiniMax 的 OpenAI 兼容 connector，需验证 Function Calling、多轮 Tool 消息累积和错误归一化。