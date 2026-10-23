# 候选确认、连续创建与失败留档实现 Plan

本次为用户直接授权的控制台开发，不纳入 roadmap。验收明细见[专属验收](../testing/acceptance/candidate-start-retention.md)。

## 实施

1. 每次明确确认使用独立 request ID；控制台立即选择该请求对应的新候选，显示提交状态，清理上一候选的回复、活动、问题和滚动位置。原请求的重试仍使用同一身份。
2. 服务先在事务内保存候选、执行、Attempt、冻结方案和不可复用的编号，然后进行外部检查、基线获取及 worker 启动。首次任务只领取一次；后续候选沿用共同基线。共同基线尚未建立时，后续候选先保存，再等待这个必要前置。
3. 启动准备异步执行；控制台不再把同任务的其他新增请求视为重复提交。请求与回执按候选绑定，晚到回执不改变用户当前选择。
4. 事务内限制最多 5 个参与候选，准备中的候选计入。明确的准备失败、worker 预检失败或原生回执证明启动失败且 worker/writers 已退出时，保留候选并结束参与。未知效果、环境阻塞及仍可能运行的候选不释放名额。
5. 启动前的持久化记录允许缺少尚未取得的基线和环境证据；准备完成后一次性固定这些证据。重启恢复原记录，不创建替代执行。保留既有已启动执行的恢复与发布边界。

## 测试计划

全部自动项归属本任务，映射至[CSR-A1～CSR-A4](../testing/acceptance/candidate-start-retention.md)。使用现有隔离 Go/Git/SQLite 和受控 worker 夹具；不使用安装目录，不调用真实模型或真实 PR。

| 验收项 / 级别 | 场景 / 命令 | 时点 | 通过条件 / 证据位置 |
| --- | --- | --- | --- |
| CSR-A1、CSR-A3、CSR-A4 / 定向 Go | `go run ./cmd/test-feedback -- ./internal/console -count=1`；`TestCandidateConfirmationSelectsIndependentPendingHistory`、`TestCandidatePendingStartSurvivesJournalReopen`，保留 CC3、操作回执及候选切换回归 | 控制台实现后 | 每次确认立即选中新候选，旧正文和活动清空，晚到回执不改选、重放不重复创建；重开继续查询原请求；报告路径登记专属验收 |
| CSR-A2、CSR-A3、CSR-A4 / 定向 Go | `go run ./cmd/test-feedback -- ./internal/execution -run '^Test(TaskStartControl|ReleasedWorkerStart|CandidateStartReservations|CandidateReservationConcurrentLimit|CandidateResumeError|NativeClaimCompetition)' -count=1 -timeout=8m` | 后端实现后 | 准备阻塞时仍可落库 5 个；第 6 个拒绝；并发事务也不超额；失败历史保留且下一候选新编号；原 ID 重启恢复；DAG、权威版本及未知效果保护保留；报告路径登记专属验收 |
| CSR-A4 / 定向 Go | `go run ./cmd/test-feedback -- ./internal/execution -run '^TestMC[129]$' -count=1` | 创建事务及恢复改动后 | 原候选独立身份、共同基线、请求幂等及数量上限回归通过；报告路径登记专属验收 |
| 必要边界回归 / L1 | `go run ./cmd/test-feedback -level=L1` | 定向验证通过后 | 固定领取事务、SQLite 接线及架构边界通过；报告路径登记专属验收 |
| 必要数据投影回归 / 定向 Go | `go run ./cmd/test-feedback -- ./internal/storage/sqlite ./internal/execution/model ./internal/execution/reconstruction ./internal/product/query -count=1` | 完成后 | 编号投影、固定身份和存储约束通过；报告路径登记专属验收 |

不选择 Full、L2/L3：本次不修改模型提示、原生传输或真实 PR 发布；必要启动和恢复行为可由已有隔离夹具验证。CSR-M1 为可后置的人工控制台体验复验，窗口及未验事实登记专属验收，不阻塞代码交付。
