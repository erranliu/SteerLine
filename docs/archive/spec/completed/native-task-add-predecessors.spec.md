# Spec：已建任务追加前置与安全重置

任务 ID：25；任务名称：已建任务追加前置与安全重置；目标：`TASK-DAG`。配套 [Plan](native-task-add-predecessors.plan.md)。本任务由一个独立实现 PR 交付。

## 目标与边界

在已创建的原生 DAG 中，允许对未交付任务**只追加**同项目 AND 硬前置，不删除或改写既有边、Task 身份及固定执行契约。复用 [固定 DAG](task-dag.spec.md) 的 SQLite 权威、环检测、就绪计算、真实 TaskDelivery 和任务领取 base 校验；不建立第二套图或通用编辑框架。

已 `closed` 或已有 TaskDelivery 的后继拒绝追加。已 `discarded` 的后继及前置拒绝。PR 已合并但中心尚未完成核验的执行不能通过“重置”避开交付对账。追加前置不改变已有 Spec/Plan 的交付范围；若任务内容需要修改，规划者另行修订相应执行契约。

## 命令与状态

`task add-predecessors` 接收后继 task_id、一项或多项前置 task_id、request_id、expected task version、expected graph revision、原因及操作者。已领取后继还须给出原 workflow_run_id 和确认旧进程停止。服务核验项目/仓库身份、节点、重复、自环、环和未交付状态。前置集合规范排序，原请求幂等返回同一回执；不同内容复用 request_id 拒绝。所有边、版本、事件、Operation 与回执在一个 SQLite 事务提交。提交后恢复从权威图重算就绪，不依赖内存缓存。

- `open` 后继：原子追加边，同时提升任务版本和 graph revision。并发领取若持有旧任务或图版本必须失败并重新预检；新领取必须从包含全部已交付前置 merge commit 的新 base 启动。
- `in_progress` 后继：只在原运行已阻塞、未发布 PR、没有未决问题、原 Orca 终端精确核验为已退出或不存在、原 worktree 已清理时，利用既有受控 reopen 路径，**同一事务**封存旧 WorkflowRun/Execution/Attempt、重置为 `open` 并追加边。旧历史保留，不能作为新执行的交付证据。正在运行、终端不可核、PR 发布结果不明或已有 PR 的任务保持原状态，返回需先结算的错误；操作者确认不能代替平台身份核验。
- `closed`、已合并但未记账、`discarded` 后继：拒绝。对未记账合并先按原固定 PR 身份结算；不能回退或重开已交付任务。

重置不自动启动新 worker，也不自动关闭外部 PR 或取消远端执行。本任务保留远程取消不支持的现状；未知资源继续占用并要求人工按精确身份处置。

## 验收

| 编号 | 通过条件 |
| --- | --- |
| AP1 | 已建 `open` 任务追加一个或多个前置后立即变为依赖阻塞；task version 与 graph revision 增长一次，重启后查询相同 |
| AP2 | 同 request_id/同内容幂等；异内容、重复边、自环、环、缺失或跨项目节点、丢弃节点、陈旧版本均无部分写入 |
| AP3 | 领取与追加竞争时，旧图预检不能越过新边；领取 base 必须包含新前置的已核验 merge commit |
| AP4 | 原 `in_progress` 任务的已停止且未发布运行在一个事务中转为 abandoned，任务重开并取得新边；重放不重复重置，旧执行与尝试仍可查 |
| AP5 | 原终端仍活跃、不可核、工作区未清理、存在 PR/发布未知、仍有未决问题时，不追加边、不重置任务、不释放执行占用 |
| AP6 | 已交付或已丢弃任务无法取得新边；SQLite 写入入口也拒绝在非 `open` 后继上插边 |
