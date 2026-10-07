# worker 停服恢复验收记录

唯一归属：[worker 停服恢复与手动继续 Spec](../spec/worker-service-recovery.spec.md)，配套 [Plan](../spec/worker-service-recovery.plan.md)。这是原任务的专属记录，不新建 Task ID。本次只拆分既有记录，没有执行补验或改变结果。

## 已验范围

2026-09-30 本机验证已通过：Windows 受管 Job 随 owner 关闭；待答时宿主退出后，重开先进入人工恢复门槛，不自动投递保存答案；明确选择后在新 Attempt/进程恢复原 Codex thread，并开启新的 turn。隔离 SQLite/fake 回归覆盖来源 Attempt/恢复版本、重复核验、迟到事件、未知操作、原 Orca terminal/Dispatch 重接与原回执结算。

原独立服务的 Job 外启动检查没有因此验收通过。来源及精确方法、进程/会话事实见[原记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/worker-service-recovery/README.md)，不扩大历史通过范围。

## 待验项

| 验收项 | 方法 | 状态 / 所需安排 | 通过条件与阻塞范围 |
| --- | --- | --- | --- |
| SR2-ORCA-A：原 agent 退出后恢复 | 自动，真实平台 | 待验收；Plan 已规划最小场景，驱动/实际安全能力与登记仍待落实 | 同工作区排除旧写者并安全新建 agent 的真机证据完整；无法核实时保持未知，禁用替代 worker |

SR1-A/SR2-A/SR3-A 分别关联 Spec 的状态/门槛、继续/回执与本机真机恢复自动要求；通过范围只限上文原已验事实，不能仅新增 ID 就宣称源场景全部已验。后续按 Plan 核对复用范围或运行受影响最小入口，自动结果逐项登记。

fake 重接通过不是新 agent 真机恢复证明。实际命令、构建和结果在准备时补入本页，原始输出留临时目录；需人准备环境时另约窗口，不把缺能力或自动失败改称人工补验。验收流程见[入口](README.md)，execution-map 同步未验链接和结果汇总。
