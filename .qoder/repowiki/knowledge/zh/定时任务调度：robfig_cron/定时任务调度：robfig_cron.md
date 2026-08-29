---
kind: external_dependency
name: 定时任务调度：robfig/cron
slug: robfig-cron
category: external_dependency
category_hints:
    - vendor_identity
scope:
    - '**'
---

AgentScheduler 使用 robfig/cron/v3 扫描 Profile YAML 中的 schedules，按 cron 表达式与时区注册启用的任务。到点后以 channel=scheduler、user_id=schedule.id 构造 AgentRequest 调用统一的 AgentService，复用 ReAct、Tool、Memory、Session 与审计链路。同一 schedule key 使用 try-lock 防止重入，单次失败只记录日志并释放锁，不影响后续触发。