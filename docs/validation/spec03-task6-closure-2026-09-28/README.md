# 原中心 #6：真实 TaskDelivery

[中心回读](receipt.json)记录原 Task #6 在 2026-09-28 为 `closed` version 5，graph revision 18；Delivery 固定 [PR #110](https://github.com/erranliu/SteerLine/pull/110) 的 head `9d0e080eef4ffb48f86f0b213e40d019a43fd4b2` 和真实 merge commit `b738a800cabd5f67040a338c29de22e8bf32d8ab`。同一 WorkflowRun `ad77df16f40aa035c333227d1a5c05b4` 的 prepare、implement、verify、review、publish_pr 五个 Attempt 均 succeeded，WorkflowRun/Execution 最终 completed/succeeded。verify 保存 `go_test`、`go_vet`、`go_build` 三项 passed；candidate、verified、reviewed revision 完全相同。后继 #29 的 #3/#4/#6 前置均已交付，当前只余 #28 未交付。

此前远端 Run `a4711862227df69c2be90c6ea6357b27` 的 worker 曾提交候选代码，但结构化 status 抄错 `workflow_run_id`。中心拒绝该结果，原 implement Attempt 保持 unknown；没有越过 verify 或创建 PR。该提交经单独审查和测试合入 [PR #107](https://github.com/erranliu/SteerLine/pull/107)，原 Dispatch 完成、原终端精确关闭、原远端 worktree 删除且提交分支保留后，[受约束重开 PR #108](https://github.com/erranliu/SteerLine/pull/108)在原库把该 Attempt 记 stopped、WorkflowRun 记 abandoned，Task 回到 open。独立 SQLite 副本先演练同一恢复命令，#14/#15/#30 的 Run/Dispatch 与项目占槽均未改变。随后才启动上述新候选；失败历史没有被写成成功。

[最终 R01–R19 范围核对](../spec03-final-scope-audit-2026-09-28.md)汇合早期版本族回执、[台式机64g 真机闭环](../spec03-desktop64g-followup-2026-09-28/README.md)、[SteerlineTest Task 105 的 4 秒 viewer 样本](../spec03-viewer-task105-2026-09-28/README.md)与[当前目标受影响故障回执](../spec03-current-host-affected-2026-09-28/README.md)。原中心 #3/#4 的 Delivery 见[结项记录](../phase1-task-closure-2026-09-28/README.md)。

中心服务结项后继续 `running/ready`，#14/#15/#30 的原远端 Dispatch/占槽不变。PR #110 合并源码已构建成版本化二进制，SHA-256 `EF0B7D428CD92F81F00FDF78A7ABBD9B1E1EC2ACE401B8E471718ABC5B92902A`，并以只读 Task 查询核对当前数据库；运行中的服务仍固定于此前版本 `bc760742c887e0d6ed05fd5e8bbdadbcdeaf617e`，待 #32 stable/dev 受控提升，不在这些活跃 worker 上热替换。长期无人值守、固定 stable 24 小时与资源趋势继续由 #32 验收；远程取消仍 unsupported。
