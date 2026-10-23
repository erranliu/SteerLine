# Web 候选停止参与修复验收

仅归属 2026-10-17 本次普通开发，依据 [Plan](../../plans/web-candidate-stop.plan.md) 和 [候选停止契约](../../spec/candidate-stop.spec.md)。历史 TUI 验收不作为本次通过证据。

| ID | 自动验收范围 | 状态 / 证据 |
| --- | --- | --- |
| WCS-A1 | 标题区入口与直接可读的禁用原因；未知/准备中仍计参与，最后一个保护、身份确认及确认期间资格重检 | 通过；E1 / E2 |
| WCS-A2 | 原请求对账；结果未核实不切换；权威退出后选择最大剩余编号且不抢用户选择；历史只读、执行事实独立 | 通过；E1 / E2 |
| WCS-A3 | SQLite 原子版本/参与数量保护、并发最多成功一个、回滚、固定停止事实和原请求精确幂等；停止与交付互斥 | 通过；E3 |
| WCS-A4 | 人工退出不触发 Runner stop/新输入/重启恢复；Task 终态仍保持停止义务、迟到接受及原请求核验；HTTP 固定目标 | 通过；E1 / E3 |

## 人工项

WCS-M1：窄/宽窗口从标题停止候选、回看历史、新候选编号递增与不抢选。窗口待定，可后置，不阻塞本次自动交付。未执行；如进行调试先创建独立 SteerlineTest 包。

## 环境与证据

仅使用临时 SQLite、内存公开 CLI 夹具及 jsdom，无真实模型/PR；不读取、修改、停止或重置 `C:/SteerlineInstall`、`C:/SteerlineRuntime`。实际结果如下。

自动项 WCS-A1～A4 已通过。源码基线为 `48c5671926466fe83c23de7fee5377bc61f87188` 加本次未提交工作区；Go summary 保存源码基线和工作区清单。Windows/amd64、Go 1.26.4；Web 依赖按锁文件执行 `npm --prefix web ci --ignore-scripts --no-audit --no-fund`，未修改依赖版本或 lockfile。

证据根：`C:/Users/lmas1/AppData/Local/Temp/steerline-web-candidate-stop-20261017-r5wrziw9`。

| 证据 | 实际执行与结果 |
| --- | --- |
| E1 | `npm --prefix web test -- src/features/runs/state.test.ts src/features/runs/runs.test.tsx src/features/runs/answer.test.tsx src/features/tasks/task-detail.test.tsx src/shared/api/queries.test.ts src/shared/api/client.test.ts src/app/app.test.tsx src/app/project-switch.test.tsx`；8 文件、75 项全部通过，4.50 秒；`web-tests-history.log` |
| E2 | `npm --prefix web run build`；TypeScript 与 Vite 通过，2156 模块，Vite 1.31 秒；`web-build-history.log` |
| E3 | `go run ./cmd/test-feedback -out <证据根>/go-complete -- ./internal/storage/sqlite ./internal/product/httpapi ./internal/runnerbridge ./internal/taskrunner ./internal/app -run 'TestRunnerCandidateStop\|TestRunnerControl.*Stop\|TestRunnerTaskServiceFreezesOriginalStart\|TestRunnerTaskDeliveryWithoutWorkflowReady\|TestRunnerCandidateReservationAndProjection\|TestCandidateStopHTTPContract\|TestSteerRunnerOutboxBoundaries\|TestCandidateWithdrawal\|TestCandidateRead' -count=1`；passed/complete，5 包、29 项（含子例），0 失败/跳过，7.027 秒；`go-complete/summary.json`、`go-complete/go-test.jsonl` |

E3 明确覆盖：准备/未知提交退出、事务回滚、固定停止事实不可篡改、五个名额释放及历史编号递增、最后两个并发退出最多一个成功、停止与真实 SQLite 交付事务竞争（PR 为受控公开事实）、丢失外层回执后的原请求恢复，以及 Task 交付后为已人工退出候选补充独立停止义务。Bridge 夹具分别核对 pending/unknown/accepted 人工退出零 CLI 输入/stop/主动读取，之后 Task 终态仍可处理原 Run 的迟到接受与停止。

E1 明确覆盖：标题入口、未知状态、固定项目/任务/候选/版本确认、确认期间剩余资格/版本变化、结果未知只等原请求、快照先到不提前切换、原回执应用后选择最大剩余编号、不抢用户后续选择、历史状态与原执行事实分别展示及已退出问题只读；shared client/queries 与原 Workspace/项目切换测试覆盖共用原请求和缓存机制。

## 失败与修复记录

- 首次 Go 定向运行 `go/summary.json` 失败：新增控制夹具使用了非 32 位候选 ID，被既有协议校验拒绝，未到达预期回执故障点。改为合法候选 ID 后 `go-fixed/summary.json` 通过；新增停止/交付竞争与已退出候选的结项回归后，以 E3 为最终结果。
- 首次 Web 构建 `web-build-initial.log` 缺少本工作区依赖，未找到 tsc；按锁文件安装后 `web-build.log` 又检出旧 Answer 测试夹具缺少新增 submit 接口。补齐该夹具后 E2 通过。
- 最后补强“另一个 pending 请求不能因 Task 已结束而冒充成功”的反例时，`go-receipt-audit/summary.json` 检出 Task discard 夹具请求 ID 格式不合格；修正为合法 ID 后，完整定向组 E3 通过。
- 初始 Web 定向 4 文件、46 项通过（`web-tests-initial.log`）；新增共用提交接口相关的 Answer/TaskDetail/Workspace/项目切换回归后，以 E1 为最终结果。

未运行 Full/L2/L3/L4、真实 Runner/模型/PR 或浏览器人工验收；自动项只证明本次 caller/存储/HTTP/组件边界。没有生成调试环境，没有操作安装目录、安装进程或安装配置。

最后补齐历史问题只读与未确认候选禁用的断言后，以 E1/E2 所列 history 日志为最终 Web 结果；早先 `web-tests-final.log` / `web-build-final.log` 仍保留。新 Plan/验收的本地链接检查和 `git diff --check` 通过。

## PR 前主干集成（2026-10-17）

本次提交后 rebase 到 `11be2af5`（PR #347），保留主干的执行位置复制、冻结预设/RunnerBuild、SubmitObserveOnly、Runner Workflow/audit/materials 投影及原重启核验测试。当前集成源码为 `714ef08f` 加移除重复 render 导入的工作区修正；随后随验收记录一并提交。

主干新加 VerifyBinding 会在启动时读取原 Run/未知提交。本次补上人工退出且无独立停止义务时的跳过门禁，并在 pending/unknown/accepted 三个退出子例里调用 RecoverInputs/VerifyBinding，断言零主动 CLI 查询；Task 终态随后仍可增加停止义务。该补充不改变活动候选原身份核验。

证据根：`C:/Users/lmas1/AppData/Local/Temp/steerline-web-candidate-stop-pr-qigt5dc9`。

- Go：与 E3 相同的五个包和选择式，追加 `TestSteerRunnerRestartOriginalBinding`；`go/summary.json` passed/complete，35 项全部通过，0 跳过，17.281 秒。
- Web：E1 的八个文件加 `src/features/runs/copy-execution-info.test.tsx`；实际 9 文件、82 项全部通过，47.32 秒，`web-tests.log`。初次命令还列了不存在的 execution-location.test.ts 过滤词，它未贡献任何用例，不记为场景通过；复制能力的实际入口为 copy-execution-info.test.tsx。
- 构建：`npm --prefix web run build`，TypeScript/Vite 通过、2158 模块、Vite 1.51 秒，`web-build-fixed.log`。首次集成构建因双方重复导入 render 失败（`web-build.log`），移除重复导入后复测通过。

此前 E1–E3 保留为合入前证据，本节作为 PR 集成后的结果。人工 WCS-M1 仍待验，安装环境始终未操作。
