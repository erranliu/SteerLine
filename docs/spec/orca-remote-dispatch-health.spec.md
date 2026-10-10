# 远端 Dispatch 失效与固定执行上下文修复

> 历史方案（2026-10-10 退役）：本页仅保留原契约与证据。当前 Orca 路径统一由 [单终端 Spec](orca-terminal-transport.spec.md) / [Plan](orca-terminal-transport.plan.md) 定义，旧 supervised、worker-start、Dispatch 及回传路径均已删除。

本聊天直接授权修复产品问题，不恢复或重发现场 101，不纳入 roadmap。

现场 101 的远端原 Codex 已运行，但提示缺少 assigned branch/base；worker 向 Orca 提问与上报失败，中心 worker-show 返回 identity_changed，协调 inbox 无消息，SteerLine 留在 pending。同一 worktree 另有第二个 Codex 打开相同 conversation 并报占用冲突。第二个会话的创建来源尚未证明，不能归因于 SteerLine 重复启动；现场只记录一次 worker-start。

## 契约与最小验收

| ID | 自动验收 |
| --- | --- |
| DH1 | 初始业务输入及反馈带上已准备工作区的真实 path、assigned branch、冻结 base；不得以 PR target、当前 head 或旧 evidence 替代 |
| DH2 | 远端 coordinator inbox 无消息或超时时，核验原 Run/Task/Dispatch/host/runtime/worktree/terminal/incarnation；失效明确结束观察并交由既有阻塞路径处理，不永久等待 |
| DH3 | 检查失效不产生 started/completed 成功事实，也不启动替代会话或重发业务输入；input_accepted 仍不等于 turn_started |
| DH4 | 独立真实远端双轮交付保持同一原 worker terminal/incarnation/worktree，使用公开 terminal switch 显示原 worker 后仍无第二个 Codex 终端；两轮 PR 更新、输出与结算仍成立 |
| DH5 | Orca 默认打开 worktree 时连接已有 worker，不重新 resume 同一 conversation；必须修复实际恢复路径并复现验证，DH4 的精确 terminal switch 不能替代此项 |

同一 worktree 允许非 agent shell。核验使用 Orca 公开 worker/terminal API，不读取 agent 私有会话文件，不改 Orca 全局启动设置或安装代码。重复打开 conversation 的上游缺陷若在独立现场重现，记录精确终端和回执并明确未修，不通过关闭 101 的终端掩盖。

结果见[专属验收](../acceptance/orca-remote-dispatch-health.md)，安排见[Plan](orca-remote-dispatch-health.plan.md)。
