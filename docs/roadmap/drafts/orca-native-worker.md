# 本机 Orca 原生通用 worker

本轮选定草稿。2026-10-09 用户确认按本会话建议调整：先使用 Orca 自己管理的宽松启动权限跑通，本机 Orca 路径中的 agent 只作为配置值；SteerLine 不启动、配置或解释具体 agent。任务身份沿用下表，不改变外部 SteerlineTest 的 102 Task、数据库或冻结执行。

用户明确本轮只调整规划文档，未授权创建执行聊天或启动实现。当前 Orca 原生模式以 Terminal + YOLO 为执行环境前置；structured worker 的完整同会话支持等待 Orca 通用接续接口，不建立特定agent分支。

| 任务 ID | 任务名称 | 成果 / 所有者 | 前置与集成 | Spec / Plan | 专属验收 |
| --- | --- | --- | --- | --- | --- |
| 86 | 本机 Orca 原生通用 worker 与权限委托 | execution/adapters/orca 与 app 接线；原生 agent 启动、worker-start、通用观察和反馈、Orca 权限所有权 | 无未完成实现硬前置；复用已合入 #83 和当前持续 worker；与 #84/#45 共享接线，应先集成本任务 | [Spec](../../spec/orca-native-worker.spec.md) · [Plan](../../spec/orca-native-worker.plan.md) | [记录](../../acceptance/orca-native-worker.md) |

一个实现任务、一个实现 PR；不拆出新的权限框架、Claude adapter 或人工 DAG 任务。未来 agent 的权限与启动差异由 Orca 内部处理。原生 Windows 的无沙箱运行不冒充统一隔离已经实现。

当前安装 Orca 的原生 launcher 已有统一 Agent Permissions / YOLO，未配置的新 agent 默认参数由 Orca 自己选择。CLI worker-start 支持通用 agent/model/effort 与原 terminal 复用，没有 permission-profile 参数。产品采用 Orca 原生设置，不新增虚构参数；验收现场确认实际宽松模式，不改用户全局设置来伪造成功。

自动验证覆盖边界、原身份/原请求、两种配置 agent 的同一路径和原生最小闭环；可后置人工项仅为用户在安装入口确认 Orca 权限显示，不作为自动失败或能力缺失的替代。任务完成前保留本草稿；执行与验收状态以 execution-map 和专属记录为准。
