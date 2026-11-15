# 活动中心放弃活动验收

通过条件见 [Spec](../../spec/activity-abandonment.spec.md#4-通过条件)，验证安排见 [Plan](../../plans/activity-abandonment.plan.md)。受测工作区以 `07a5c769` 为基线，包含本次未提交修改。未部署。

## 2026-11-13 验证

| 范围 | 结果与证据 |
| --- | --- |
| 固定基线 | `go run ./cmd/test-feedback -level=L1` 及 `-level=L2` 均退出 0。报告目录 `C:\SteerlineDebug\wt-915c-activity-abandonment\test-results\L1\20261113T103827-af92ce47`、`C:\SteerlineDebug\wt-915c-activity-abandonment\test-results\L2\20261113T103827-af92ce47`。 |
| 归档事务、回执及原停止 | 定向 `go test` 退出 0。覆盖停止意图与回执一并提交、回执冲突回滚、重开数据库后归档时间不变、原停止重核实及来源隔离。原报告 `C:\SteerlineDebug\wt-915c-activity-abandonment\diagnostics\20261113T105040-5bfab773-focused.log`。 |
| HTTP | `go.http` 退出 0，报告 `C:\SteerlineDebug\wt-915c-activity-abandonment\test-results\L4\20261113T104345-0e39c7db-go.http\summary.json`。 |
| 环境 | `go.environment` 退出 0，报告 `C:\SteerlineDebug\wt-915c-activity-abandonment\test-results\L4\20261113T104345-0e39c7db-go.environment\summary.json`。 |
| Roadmap | `go.roadmap` 的 owner 和 host 组通过。应用组的发布夹具创建 Git worktree 时触发 Windows Git 路径限制，整个专项退出 1。短路径补验越过该限制，仍在丢失 PR 回执后应保留原观察时间的断言失败。报告 `C:\SteerlineDebug\wt-915c-activity-abandonment\test-results\L4\20261113T104345-0e39c7db-go.roadmap\summary.json`，补验 `C:\SteerlineDebug\wt-915c-activity-abandonment\diagnostics\20261113T111658-b300c551-short-path-roadmap.log`。 |
| Web | 生产构建通过。活动、独立页、回执、环境定向 65 项通过。受管 `web.unit` 一次运行中有两项未通过，分别是项目切换的活动深链接等待和抽屉分页请求数。保留 `C:\SteerlineDebug\wt-915c-activity-abandonment\test-results\L4\20261113T104918-ae2c7f9e-web.unit\summary.json`。`npm --prefix web test -- --maxWorkers=2` 补验 54 个文件、490 项通过，退出 0。 |
| 受控活动链 | 两次运行的抽屉和独立页浏览器子用例均通过。第二次通过真实应用 HTTP 放弃正式候选，验证原回执、历史中的原状态和随后正常交付及资源结算。证据 `C:\SteerlineDebug\wt-915c-activity-abandonment\test-results\L3\20261113T104754-7719f15a\scenarios\runtime.async-activities\activity-proof.json`。两个完整场景均因另一浏览器子用例的第二次候选提交断言超时而退出 1。原报告目录 `C:\SteerlineDebug\wt-915c-activity-abandonment\test-results\L3\20261113T104052-91e5dc8c` 和 `C:\SteerlineDebug\wt-915c-activity-abandonment\test-results\L3\20261113T104754-7719f15a`。 |
| 存储专项 | 首轮暴露 Roadmap 停止锁重入，已修复并通过原竞态定向测试。补验中的候选活动筛选矩阵在并行负载下触发一秒读预算，报告 `C:\SteerlineDebug\wt-915c-activity-abandonment\test-results\L4\20261113T104803-7b053be5-go.storage\summary.json`。单独重跑筛选矩阵及分页/关联用例均退出 0，证据 `C:\SteerlineDebug\wt-915c-activity-abandonment\diagnostics\20261113T105439-eec26c4e-sql-budget.log`。 |

受控证据覆盖归档与业务状态分离、候选交付以及部分停止竞态。六个来源分别使用真实运行的人工验收未执行。

## 2026-11-15 失败复现与复验

干净基线 `07a5c769` 和当前工作区均使用独立短路径夹具。两项重点失败都已在活动放弃改动之前的基线复现。

| 范围 | 基线结果与修复 |
| --- | --- |
| L3 第二次候选提交 | 浏览器在首个 `start_task` 仍为 `submitted` 时确认新增，后者进入「等待首次启动确认」。约 41 秒后首个请求落库，浏览器约 49 秒后收到 `applied`，随后发出的任务详情请求在原断言截止前未完成，实际只看到一次候选提交。用例现在先读到包含首个候选及 `start_candidate` 资格的任务快照，再确认新增。原 60 秒提交断言及每种命令一次、请求 ID 各异的断言保留。首次确认前的排队由 `use-candidate-starts.test.tsx` 覆盖。基线报告 `C:\SteerlineDebug\wt-915c-activity-abandonment\test-results\L3\20261115-ba669108-baseline\summary.json`，该次清理未能核实，原夹具按报告保留。 |
| Roadmap 发布观察时间 | 旧发布路径（`3a2b10877`）会把本地推进到 `pr_unknown` 记成一次成功观察。丢失 PR 创建回执时，基线的 `ObservedAt` 从 `04:39:00.1988734Z` 推进到 `04:39:12.6223059Z`。修复后只有核实路径记录观察结果。基线证据 `C:\SteerlineDebug\wt-915c-activity-abandonment\diagnostics\20261115-ba669108-baseline-roadmap.log`。 |

| 复验范围 | 结果与证据 |
| --- | --- |
| L3 `runtime.async-activities` | 修后完整场景两次通过，清理标志均为 `true`。最终源码复验核对两次独立候选提交、放弃后原候选交付和资源结算。报告 `C:\SteerlineDebug\wt-915c-activity-abandonment\test-results\L3\20261115-t86a6cfcb-candidate-fix\summary.json`。 |
| L4 `go.roadmap` | 使用 Windows 原生短路径后，owner、App、host 三组完整通过。原失败的 `single-task-planning` 保留原观察时间、核实原 PR 并走完后续入图断言。报告 `C:\SteerlineDebug\wt-915c-activity-abandonment\test-results\L4\20261115-t5a3c7e24-roadmap-native-temp\summary.json`。第一轮 App 和 host 通过，两条 owner 测试的 `mklink /J` 将正斜杠临时路径误识别为开关，原报告保留在 `C:\SteerlineDebug\wt-915c-activity-abandonment\test-results\L4\20261115-tcf9d59a2-roadmap\summary.json`。 |
| L4 `go.storage` | 完整专项通过，含此前触发读预算的候选活动筛选矩阵，原预算和断言保留。报告 `C:\SteerlineDebug\wt-915c-activity-abandonment\test-results\L4\20261115-tb84f6cab-go.storage\summary.json`。 |
| L4 `web.unit` | 受管全集通过，54 个测试文件、490 项，包含上次失败的活动深链接和分页请求数用例。报告 `C:\SteerlineDebug\wt-915c-activity-abandonment\test-results\L4\20261115-web-unit-rerun\summary.json`。 |
| Web 定向 | 候选启动 hook、活动组件共 29 项通过。浏览器驱动独立 TypeScript 严格检查通过。Vitest 证据 `C:\SteerlineDebug\wt-915c-activity-abandonment\diagnostics\20261115-candidate-start-hook-web.log`。 |

## PR 变基后复验

PR 基线为 `68829335`。上表保留 `07a5c769` 上的失败和修复证据。合入基线采用 #556 明确的发布观察语义。本地已持久化的发布进度可推进观察时间，缺失远端 PR 回执时原状态仍为 `pr_unknown`，核实沿原 PR 继续。该行为及用例修正见 [Roadmap 规划验收](roadmap-planning-workspace.md)。

维护放弃沿用 #515 的原停止转换、队列和名额释放规则。放弃后退出维护当前列表，历史及占用名额提示仍可打开原行动。维护、活动和 Roadmap 定向 Web 69 项及生产构建通过，覆盖当前选择退出和原名额核实。

`main` 已使用 SQLite 版本 50，新增归档表递增至 51。当前结构初始化、旧结构拒绝及归档事务定向测试通过，证据 `C:\SteerlineDebug\wt-915c-activity-abandonment\diagnostics\20261115-pr-rebase-schema.log`。

最终功能提交 `e5d7b5de` 的 Roadmap 丢失回执发布及入图、活动读取和归档应用测试通过，证据 `C:\SteerlineDebug\wt-915c-activity-abandonment\diagnostics\20261115-pr-final-app.log`。同一提交的 L3 `runtime.async-activities` 整体通过，第二个候选独立提交，已放弃候选继续交付，资源结算和清理均完成。报告 `C:\SteerlineDebug\wt-915c-activity-abandonment\test-results\L3\20261115-pr-final\summary.json`。
