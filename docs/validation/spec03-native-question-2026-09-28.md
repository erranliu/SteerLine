# Spec 03 原生业务问答与真实交付（2026-09-28）

本机 Orca 1.4.209、home Orca 1.4.210 和 Codex CLI 0.157.1，在隔离的 `erranliu/SteerlineTest` 项目运行。中心使用独立 clone、SQLite 和原生 Task 91024；测试基线为 `codex/spec03-native-question-base` 的固定提交 `e33e519`。没有操作旧 Beads 状态或其他任务的远端终端。

implement 的原 Dispatch `ctx_4d67df073f7f` 在写文件前提出一次 `yes/no` 问题 `relay_8efa2526f66a`。中心在同一 WorkflowRun `46eb499dbc36e059da6aadf900900110`、Execution `39f13cc89474e5ee9494bf262164df83` 和 Attempt `53f1750ae1cec89ac75486fad421d8b0` 上保存 Decision。正式答案命令 `8026a8a7191bec997e0c1581fcaee60b` 以 `yes` 获得 `accepted`；`reply_orca_question` Operation 确认后，远端另发含原问题、Attempt、Execution、答案正文及实际 Codex 版本的应用证明，Decision 才变为 `applied`。仅有答复回执时，中心没有提前放行 implement。

原 worker 随后只提交 `steerline-spec03-native-stage.txt`，候选 `64dfc4e7bd333d7321f9fff36dbbafe4f0f55517`。独立 verify 的实际内容、祖先及固定 HEAD 检查通过；独立 review 返回 passed，VerifiedRevision 和 ReviewedRevision 均为该候选。五个 Dispatch 先后运行于同一 worktree，没有重叠修改。

publish_pr 的原提示一度停在输入框，Orca 报 `session_not_reported`；中心把原 Attempt 留为 `outcome_unknown`。核对原 terminal/incarnation 后只提交一次 Enter，原 worker 创建唯一 [测试 PR #17](https://github.com/erranliu/SteerlineTest/pull/17)。中心在同一 SQLite 重启后重读原 status/worker_done，按固定 base/head 验证；PR 真实合并为 `a0172dbebd29621b737265810fd21e47759d2214`，中心写入 Delivery 并关闭 Task 91024。全部五个 Dispatch 均为 `succeeded/completed/released/exited`。精确身份、命令及资源回执见[样本](../spec/orca-probe/samples/spec03-native-question-2026-09-28.json)。

先前三个[隔离失败探测](../spec/orca-probe/samples/spec03-native-question-retries-2026-09-28.json)分别暴露原生问答 Run 等待状态在快照刷新中反复退回 pending、重启后等待中的原 Dispatch 未重接、Windows 工作树路径表示造成答案资格误拒，以及 worker 的应用证明模板缺少 `remote_codex_version`。修复后才运行本次全新任务；未把失败 Attempt 回写成成功。最终验收测试在**已完成的原 SQLite**上重新读取证据并通过；完整执行是同一中心状态经历受控停止、重开与原 publish 对账，不是一次未中断测试进程的结果。

当前目标的原生业务问答、应用证明和真实交付路径由此通过。生产目标仍须按自己的配置认证；可见 viewer 的 2 秒展示、管理员/UAC 设置提示及未覆盖的故障矩阵继续开放。远程取消保持 unsupported。
