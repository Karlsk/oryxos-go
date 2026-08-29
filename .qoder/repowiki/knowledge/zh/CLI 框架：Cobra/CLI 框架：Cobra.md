---
kind: external_dependency
name: CLI 框架：Cobra
slug: cobra
category: external_dependency
category_hints:
    - vendor_identity
scope:
    - '**'
---

OryxOS 的命令行入口使用 Cobra（github.com/spf13/cobra），定义 12 个叶子命令：init、status、chat、serve、gateway、profile list/create/show/delete、provider list、tool list、session list。三种运行模式（交互对话、HTTP API、常驻 gateway）共用同一 Application 组装根，避免三套行为。