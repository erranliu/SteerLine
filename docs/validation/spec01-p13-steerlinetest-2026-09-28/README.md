# P13：SteerlineTest 远端 verify 的两项负向门槛

2026-09-28，Windows 本机 Orca 1.4.209 / `台式机64g` Orca 1.4.215，远端 Codex CLI 0.156.1。两个试次各用独立 `SteerlineTest` clone、SQLite、WorkflowRun 和原生 Task 91024；远端仓库 `id:903fb315-7d5e-4246-8913-816e1d3bdf03`、固定测试 base `592a745f3ed8dc26ad74c4f01066d632e77bee30`。已启动的正式 SteerlineTest 服务与 Task 104 没有作为故障注入目标，远端取消仍不支持。

[受控入口代理源码](../spec03-p13-verify-ingress-proxy.go.txt) SHA-256 `5172a7cd80d5aff742f85621f5d68a5a008042ca4b44ad4c8e28d21a0be45c70`，测试二进制 SHA-256 `0ca381761d4ea4e5525ed3edef45bed589ec31190502b5f0c945c7d22cb19819`。代理仅包装**隔离中心**的 `orca orchestration check`：远端 worker 正常执行检查、发送 status/worker_done，Orca 的 Run/Task/Dispatch/Delivery 不变；代理在读回的原 verify status payload 中只改下面指定字段。第一次变异的原/改回执按字节留档，`verify_evidence.py` 对消息身份、单字段差异、中心 SQLite 状态做可执行核对，两种运行均退出 0。

| 试次 | 原回执与单字段变异 | 中心及平台结论 |
| --- | --- | --- |
| 缺必需检查 | 原 [check](missing-original-check.json) 有一条 exit 0、`passed=true`、实际提交等于候选的回执；[变异](missing-mutated-check.json)只把 `step_result.result.checks` 改为 `[]`。消息 `relay_ffb35df9b3a5`、原 Dispatch 不变；[自动核对](missing-center.json)通过 | WorkflowRun `e38b5b6bbc5e29ca9cd08eb866a79260` 在 verify Attempt `7db6722df3a84b7bc42dba2dba03f7bd` 后 `blocked`；`verified_revision=null`，无 TaskDelivery 或 PR。原 verify Dispatch `ctx_02ec0a318249` 为 `succeeded/completed/released/exited`，无越步 review/publish |
| 检查提交不符 | 原 [check](commit-original-check.json) 的 `actual_commit=65b9ab570e0cf232d3656a19be667b0f0a339fa2` 与候选一致；[变异](commit-mutated-check.json)只将该字段置为 40 个 `0`。原 Delivery 同时含 worker_done，均未改；消息 `relay_61306f80b923` 不变；[自动核对](commit-center.json)通过 | WorkflowRun `0d01ad21e588f878cce92638ee6187e2` 在 verify Attempt `051d84f8a590e7a341c727bdb33cbba6` 后 `blocked`；`verified_revision=null`，无 TaskDelivery 或 PR。三个原 Dispatch 均 `succeeded/completed/released/exited`，GitHub 对原工作分支的 PR 查询为 `[]` |

此前 [P13 非零退出负向真机回执](../spec/orca-probe/samples/spec03-supervised-negative-verify-retry-2026-09-25.json)与[正向回执](../spec/orca-probe/samples/spec03-supervised-staged-implement-verify-2026-09-25.json)保留原目标。至此 P13 的非零退出、必需回执缺失、检查提交不符三个拒绝分支，均有真实受监管 worker/中心状态边界证据；后两项是明确标注的**中心入口单字段故障注入**，并非声称远端 agent 自发发送了坏数据。

不计入 P13 的先前试次：第一次固定 base 在新主机未获取，Orca 工作树创建失败，未进入 verify；精确 fetch 后远端 `git cat-file` 确认 commit。另一次本机测试进程在 prepare 启动回执入库前退出，原 Dispatch 后来 `succeeded/completed` 并按精确身份 release；原 SQLite 的 `in_flight` Operation 保留，不重派、不把它算作 verify。两个有效负向试次在中心已 blocked、原 worker 释放后停止测试监督进程；正向闭环测试本身预期 Task 关闭，故没有把人为中断伪称为该测试 PASS。未创建或合并任何 P13 测试 PR。
