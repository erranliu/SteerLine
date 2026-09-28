# 模块重构任务创建与 DAG 调整

状态：三项任务及全部新增依赖已在原中心创建并回读通过，服务已恢复。本文记录操作事实，不表示实现验收通过。

规格固定提交：`03fe9b87bc58f36f6017cd9f09679daceebf2a7e`。创建 request_id：`944ba582d7674983a6971678e0e55239`；batch_id：`997966371167b68bce6ad082a481f7a2`。新任务的 Spec/Plan、共同约束与摘要绑定该提交，既有任务契约未重绑定。

| Task | 名称 | 新建时前置 |
| --- | --- | --- |
| 33 | 执行规则包提取 | 无 |
| 34 | 执行上下文封装与服务职责收缩 | 33 |
| 35 | 中心命令查询与SQLite边界封装 | 27 |

已通过原生 add-predecessors 追加 34 → 26、35 → 28、35 → 16。原有所有边与固定契约保留，未请求启动新任务。

## 创建与回读

创建前 21 个节点、30 条边，graph revision 10；原子创建 #33/#34/#35 和 33 → 34、27 → 35 后为 revision 11；三次补边依次为 12、13、14。最终 24 个节点、35 条边，无分页截断。#26 由 open/version 3 变为 version 4，#28 由 version 1 变为 2，#16 由 version 2 变为 3。

逐行摘要核对原 task_contracts、WorkflowRun、Execution、Attempt、Decision、Delivery、Operation、Command、执行绑定和执行操作全部保留；除新契约/命令/操作外没有额外记录，也没有新 WorkflowRun。三个后继只改变版本和前置，原状态及固定契约相同；原 30 条边全部保留。[核对结果](verification.json)与[原表摘要](center-before.json)保存具体数量。

中心通过原 pause_dispatch/stop-service 暂停并退出，停服回执确认仓库锁释放后对同一 SQLite 使用 backup API 取得一致备份，PRAGMA integrity_check 为 ok。备份路径与 SHA-256 在创建意图及原表摘要中；没有用备份作为新业务库。创建及补边均通过安装中的 CLI，二进制与配置未替换。

经用户明确允许，已重开原可见服务窗口并恢复 manual/未暂停，服务实例 `c3ebe12160a50f9238cd8a930c1443e8`、PID 41440，running/ready；重启回读图版本仍为 14，活动 Run 为 0，authority version 为 10。[恢复核对](restored-check.json)保存结果。其他项目服务未作为本次目标。

## 输入与回执

[创建意图](creation-intent.json) · [创建输入](task-batch.json) · [只读校验](validation.json) · [创建回执](receipt.json) · [26 补边](add-26-receipt.json) · [28 补边](add-28-receipt.json) · [16 补边](add-16-receipt.json)

[创建前任务](before.json) · [创建前运行摘要](runtime-before.json) · [创建后任务](after.json) · [中心 DAG](dag.json) · [规划检查](planning-check.json) · [完整核对](verification.json)

[暂停](service-pause.stdout.txt) · [停服](service-stop.stdout.txt) · [窗口启动](service-window-launch.json) · [恢复派发](service-resume.stdout.txt) · [服务连接](service-restored.json)

## 前置状态核对

2026-09-28 中心只读回读显示：数据库 `1ac8d0529fb922c4ccd187a2b8705c27`，graph revision 6，authority version 8；#26 为 in_progress/version 2。其 WorkflowRun `82e5d3178ae7447ee6e2ec12fdf130eb`、Execution `f544e755739f3e5ad9c5665b97029436`、Attempt `b6abb409ddb53e2b4535561fbb3553bd` 在受监管 Orca 的 prepare 步骤，投影为 running/dispatched，未绑定 PR。

现有 `ReopenNativeTask` 只接受精确 blocked/unknown 的本机 Orca implement 运行，还须无 PR、无未决问题、原终端可证实退出及工作区已清理。当前 #26 后端为 orca_supervised，且仍在 prepare，因此不满足支持范围；未声明已停止、未重置、未追加 34 → 26。用户选择保留“重构先于 #26”的意图，不改成相反顺序。

随后用户明确告知四项执行已停止，重新回读确认：#14/#15/#26/#30 均为 open，原 WorkflowRun 已 abandoned、Execution 已 stopped，graph revision 为 10；#26 为 version 3。当前 runtime 没有活动 Run，因此可直接按 open 任务追加前置，本次无需也未执行 reopen。上述运行重置由本次操作之前的处置完成，不能归为本次命令结果。

现有 task create/add-predecessors CLI 必须取得仓库锁，没有在线受理入口。本次按原服务暂停/停服、取得锁后创建与补边、重启并恢复原 manual/未暂停设置的顺序进行；其他项目服务不作为操作目标。新任务创建不会自动执行 start_task。

## 固定规格与后续执行

规划提交尚须按正常代码审阅合入任务配置的 main，再启动新任务；固定提交及其 Git blob 必须在目标端可核验。依赖就绪不等于规格或平台启动资格通过。后续重复查询或响应丢失恢复使用本记录的原 ID/请求，不重建同名任务、不重放工作执行。

初次检查发现的活动远端执行没有被本次操作强制重置；本次是在用户处理后的 open 状态上补边。中心停止监督不等于 worker 已停止，历史处置与本次创建事实分别保留。
