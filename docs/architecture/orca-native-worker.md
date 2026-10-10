# 本机 Orca 原生通用 worker 调整记录

当前接线已改为 `host=orca_supervised` 的 local/environment 双目标，保持原远端功能。原 `host=orca`、terminal codec/协议和独立正文读取入口已切断，以下原本机实现源码与历史记录保留。当前范围见[Plan](../spec/orca-supervised-targets.plan.md)和[验收](../acceptance/orca-supervised-targets.md)。

2026-10-09 用户明确撤回本变更的 roadmap 身份，改由当前聊天直接实现。历史预留 ID 86 只用于追溯，不复用、不进入执行 DAG、不作为 #84 的前置或等待项。本变更采用 Orca 自己管理的宽松启动权限，本机 Orca 路径中的 agent 只作为配置值；SteerLine 不启动、配置或解释具体 agent。不改变外部 SteerlineTest 的 102 Task、数据库或冻结执行。

用户最新指令为直接实现，不创建新的 roadmap 执行聊天。当前 Orca 原生模式以 Terminal + YOLO 为执行环境前置；structured worker 的完整同会话支持等待 Orca 通用接续接口，不建立特定agent分支。

设计依据：[Spec](../spec/orca-native-worker.spec.md) · [Plan](../spec/orca-native-worker.plan.md) · [验收记录](../acceptance/orca-native-worker.md)。所有者为 Orca adapter 与 app 的 Orca 接线；#84 按原固定 Spec/Plan 继续，不等待本变更。双方在最终集成时核对规范输出和生命周期接口，不静默改写在途契约。

本聊天直接实施并分别登记通过范围和真实启动阻塞；不拆出新的权限框架、Claude adapter 或人工 DAG 任务。未来 agent 的权限与启动差异由 Orca 内部处理。原生 Windows 的无沙箱运行不冒充统一隔离已经实现。

当前安装 Orca 的原生 launcher 已有统一 Agent Permissions / YOLO，未配置的新 agent 默认参数由 Orca 自己选择。CLI worker-start 支持通用 agent/model/effort 与原 terminal 复用，没有 permission-profile 参数。产品采用 Orca 原生设置，不新增虚构参数；验收现场确认实际宽松模式，不改用户全局设置来伪造成功。

自动验证覆盖边界、原身份/原请求、两种配置 agent 的同一路径和原生最小闭环；可后置人工项仅为用户在安装入口确认 Orca 权限显示，不作为自动失败或能力缺失的替代。当前实现与验证结果维护于专属记录，不进入 roadmap 执行队列。
