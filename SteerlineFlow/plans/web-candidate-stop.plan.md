# Plan：Web 候选停止参与修复

2026-10-17 用户直接授权的普通开发；不进入 roadmap，不修改安装环境。依据 [候选停止契约](../spec/candidate-stop.spec.md)，当前 Runner 接线差异及验证归 [专属验收](../testing/acceptance/web-candidate-stop.md)。

## 实施

1. Web 将停止参与放在候选标题操作区，直接展示不可用原因；停止资格与同任务参与计数只读持久参与事实，不按 Runner 执行状态或 Task 状态猜测。确认固定原候选、项目、任务、方案、版本与服务身份，提交时重检当前资格；未确认的本地占位不能提交。
2. SQLite 人工退出事务重检版本、参与资格和剩余数量，持久化原请求、操作者与停止时间，支持精确原请求重放；最后两个候选并发退出最多一个成功。使用既有 Bridge 候选锁串行化人工退出和外发动作。
3. 人工退出仅写 participation=abandoned，不写代表 Runner 停止义务的 stop_requested。Bridge 对人工退出不再发新业务或主动恢复/轮询，包含重启 VerifyBinding 的原执行查询；Task 终态仍可增加 stop_requested，保留原迟到接受与停止结算。输入、PR 交付与投影统一检查参与资格。
4. Web 对账沿用原请求日志；权威快照确认退出后，仍选择原目标才切到编号最大的剩余参与候选，用户已切换则保持选择。已停止历史只读，显示停止事实及保留资源；不把接口接受当作已完成。
5. 更新 [worker 统一规则](../../docs/product-design/worker-lifecycle.md) 的 W12/W17/W18、E04/E05、V03/V06–V08 差异和当前缺口；不改 Runner 内部执行状态机。

## 测试计划

均为 Windows 隔离夹具；证据根为新建临时目录，实际命令、结果和路径登记专属验收。Plan 不登记实时结果。

| 验收项 | 级别 / 场景与命令 | 执行时点、通过条件、证据 |
| --- | --- | --- |
| [WCS-A1 / WCS-A2](../testing/acceptance/web-candidate-stop.md) | L1 Web：`npm --prefix web test -- src/features/runs/state.test.ts src/features/runs/runs.test.tsx src/features/runs/answer.test.tsx src/features/tasks/task-detail.test.tsx src/app/app.test.tsx src/app/project-switch.test.tsx src/features/runs/copy-execution-info.test.tsx src/shared/api/queries.test.ts src/shared/api/client.test.ts` | 接线后；未知参与、最后一个、固定确认、资格变化、原请求对账、权威切换/不抢选和历史只读通过；`web-tests-history.log` |
| [WCS-A3 / WCS-A4](../testing/acceptance/web-candidate-stop.md) | L1 SQLite/HTTP：`go run ./cmd/test-feedback -out <新目录>/go -- ./internal/storage/sqlite ./internal/product/httpapi -run 'TestRunnerCandidateStop\|TestRunnerControl.*Stop\|TestRunnerTaskServiceFreezesOriginalStart\|TestRunnerTaskDeliveryWithoutWorkflowReady\|TestRunnerCandidateReservationAndProjection\|TestCandidateStopHTTPContract' -count=1` | 存储与控制接线后；并发只留一个、回滚、原请求重放/冲突、停止与交付竞争及 Task 原停止义务通过；Go summary/raw logs |
| [WCS-A4](../testing/acceptance/web-candidate-stop.md) | L1 Bridge/投影：`go run ./cmd/test-feedback -out <新目录>/bridge -- ./internal/runnerbridge ./internal/taskrunner ./internal/app -run 'TestSteerRunnerOutboxBoundaries\|TestSteerRunnerRestartOriginalBinding\|TestCandidateWithdrawal\|TestRunnerCandidateStopProjection\|TestCandidateRead' -count=1` | 门禁完成后；人工退出零新 CLI 动作、重启不恢复，Task 终态迟到接受仍结算，停止事实可读；Go summary/raw logs |
| [WCS-A1 / WCS-A2](../testing/acceptance/web-candidate-stop.md) | 构建：`npm --prefix web run build` | 最后；TypeScript 与 Vite 构建通过；`web-build-history.log` |

不选 Full/L2/L3/L4：本次是调用方事务、业务门禁及 Web 交互修复，上述 L1 可覆盖自动项；不验证 Runner 原生停止机制，不启动真实模型或 PR。WCS-M1 人工界面复验可后置；如进行调试须先生成独立 SteerlineTest 包，不能使用安装目录。
