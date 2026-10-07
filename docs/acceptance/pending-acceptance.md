# 原生验收边界与待补项

以下继承既有记录，本次目录清理没有执行模型、原生补验或改变完成状态。任务要求继续以[活动 Spec](../spec/README.md)为准。

## 当前待验清单

| 验收项 | 方法 | 状态 / 所需安排 | 阻塞范围 |
| --- | --- | --- | --- |
| app-server 定向 race | 自动 | 待验收；待 CGO/C 工具链，沿既有并行/恢复/回复场景固定命令 | race 结论未取得；不改称人工补验 |
| ON3 原活动轮次回复 | 自动，真实平台 | 待验收；原 question/message、纯文字及选项＋备注、重复提交需定向场景 | 相应原生能力签收，不能由候选反馈通过替代 |
| ON5 / ON7 竞争与故障 | 自动，定向注入及真实平台核验 | 待验收；补齐发送/完成竞争、迟到事件、断联/审批/身份变化 | 相关故障能力；已知问题须修复，不能因人工后置放行 |
| ON8 / ON9 输出与生命周期 | 自动，真实平台 | 待验收；核验 transcript 边界及中心停观察后的原终端/工作区 | 相应输出和 owned 资源能力 |
| Orca 原 agent 退出后恢复 | 自动，真实平台 | 待验收；同工作区排除旧写者及安全新建 agent 的真机证据缺失 | 未取得事实时保持未知，禁用替代 worker |

上述是自动验收能力/环境的未完成项，不能因为需要人准备环境就改称允许后置的人工验收。实际命令、构建和结果在准备时补入本页；原已验范围见下文，仍绑定原时点。需人参与准备时单独安排参与人/窗口，不改变自动项通过条件。

## local Codex app-server

2026-09-29 的 A/B/C 已通过：同服务双任务、原 thread/turn/call 恢复、原会话第二轮；自由文字、选项加备注、开放 PR 反馈；真实宿主退出后的原身份重接；自动回归、vet 与 Windows/Linux 构建。原 Codex CLI 0.157.1、app-server userAgent Codex Desktop/0.158.0-alpha.2.1。同步主线后用 58.7 秒定向回归补验，没有重复全量。**定向 race detector 仍缺工具链条件及实际结果**。

独立 app-server 存活属于旧宿主语义，不能替代当前受限 Job 下的人工继续恢复。精确版本、时长与原证据见[原状态](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/local-codex-app-server/status.md)；后续按[Spec](../spec/local-codex-app-server.spec.md)补验。

## Orca native

活动轮次输入、消息竞争/故障、完整输出和独立生命周期仍按[ON 场景 Spec](../spec/worker-flow-orca-native.spec.md)核验。原 `implementation-audit.md` 是 2026-09-29、`2ce020b` 的实施前审计，不能当当前实现清单或 WF1–WF12 验收通过；[历史审计](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/worker-flow/implementation-audit.md)保留原探针及当时缺口。#39 本机候选通过也不自动补齐 ON 全部场景。

## worker 停服恢复

2026-09-30 本机验证已通过：Windows 受管 Job 随 owner 关闭；待答时宿主退出后，重开先进入人工恢复门槛，不自动投递保存答案；明确选择后在新 Attempt/进程恢复原 Codex thread，并开启新的 turn。隔离 SQLite/fake 回归覆盖来源 Attempt/恢复版本、重复核验、迟到事件、未知操作、原 Orca terminal/Dispatch 重接与原回执结算。

**Orca 旧 agent 退出后，在同工作区排除旧写者并安全新建 agent 的真机证据仍缺失**；身份无法核实时保留未知并禁用替代 worker。fake 重接通过不是新 agent 真机恢复证明。原独立服务的 Job 外启动检查也没有因此验收通过。来源及精确进程/会话事实见[原记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/worker-service-recovery/README.md)；后续按[Spec](../spec/worker-service-recovery.spec.md)核验。
