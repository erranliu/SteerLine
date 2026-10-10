# Orca 单终端 worker

2026-10-10 的当前接线使用 host=orca 与 orca_terminal_v3。本机与 environment 的能力相同；Orca 只承载 worktree / terminal 原语，上层共用 development/v2 的短提示词、报告、发布及交付检查。

工作区创建时不投递提示词；每回合先持久保存原终端输出游标，再一次 terminal send --enter。CLI 按原游标读取带 Command 标识的紧凑结果，原终端空闲且完整结果匹配后交由上层。身份变化、输出缺口、截断或冲突保持未核实，不补 Enter、不重发或寻找替代终端。

没有辅助终端、文件回传、HTTP 回调、Orca Run / Task / Dispatch / coordinator / worker_done，也不读取代理私有日志或构造代理模型命令。完整全文与原生强制只读不在该路径能力内；报告只比较两个主机均能由 CLI 获得的 revision 事实。旧 supervised 实现和专属夹具直接删除。

[Spec](../spec/orca-terminal-transport.spec.md) · [Plan](../spec/orca-terminal-transport.plan.md) · [验收与待验项](../acceptance/orca-terminal-transport.md)。普通开发由当前聊天实施，不进入 roadmap，也不改安装环境。
