# 模块重构任务创建与 DAG 调整

状态：Spec/Plan 与仓库身份已形成，中心写入尚未执行。本文记录操作事实，不表示实现验收通过。

| Task | 名称 | 新建时前置 |
| --- | --- | --- |
| 33 | 执行规则包提取 | 无 |
| 34 | 执行上下文封装与服务职责收缩 | 33 |
| 35 | 中心命令查询与SQLite边界封装 | 27 |

拟通过原生 add-predecessors 追加 34 → 26、35 → 28、35 → 16。原有所有边与固定契约保留，未请求启动新任务。

## 前置状态核对

2026-09-28 中心只读回读显示：数据库 `1ac8d0529fb922c4ccd187a2b8705c27`，graph revision 6，authority version 8；#26 为 in_progress/version 2。其 WorkflowRun `82e5d3178ae7447ee6e2ec12fdf130eb`、Execution `f544e755739f3e5ad9c5665b97029436`、Attempt `b6abb409ddb53e2b4535561fbb3553bd` 在受监管 Orca 的 prepare 步骤，投影为 running/dispatched，未绑定 PR。

现有 `ReopenNativeTask` 只接受精确 blocked/unknown 的本机 Orca implement 运行，还须无 PR、无未决问题、原终端可证实退出及工作区已清理。当前 #26 后端为 orca_supervised，且仍在 prepare，因此不满足支持范围；未声明已停止、未重置、未追加 34 → 26。用户选择保留“重构先于 #26”的意图，不改成相反顺序。

随后用户明确告知四项执行已停止，重新回读确认：#14/#15/#26/#30 均为 open，原 WorkflowRun 已 abandoned、Execution 已 stopped，graph revision 为 10；#26 为 version 3。当前 runtime 没有活动 Run，因此可直接按 open 任务追加前置，本次无需也未执行 reopen。上述运行重置由本次操作之前的处置完成，不能归为本次命令结果。

现有 task create/add-predecessors CLI 必须取得仓库锁，没有在线受理入口。本次按原服务暂停/停服、取得锁后创建与补边、重启并恢复原 manual/未暂停设置的顺序进行；其他项目服务不作为操作目标。新任务创建不会自动执行 start_task。

## 恢复执行原则

固定规格提交后生成创建输入、原 request_id 与精确补边请求，先执行只读 validate。真正写入前重新回读 Task/图/项目版本，并取得原项目锁与一致备份；未提交的 stale 请求可经明确预检重新制定，已提交或未知请求只按原身份对账。创建后逐一提交可用补边并回读全部节点/边、契约摘要、原运行和交付记录；全部回执确认前不报告 DAG 更新完成。

活动/unknown 现场不通过复制数据库、并行写者、直接 SQL 或修改原生命令绕过。中心停止监督也不等于 worker 已停止。若现有安全重置能力仍不支持 #26，保留该补边的阻塞事实，后续处置须有明确依据。
