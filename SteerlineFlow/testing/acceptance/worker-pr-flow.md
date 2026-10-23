# 统一 Worker 的 PR 成果流程验收

仅归属本次普通开发设计。[Spec](../../spec/worker-pr-flow.spec.md) · [Plan](../../plans/worker-pr-flow.plan.md)。2026-10-12 已实施并完成下述最小受控验证；真实模型/真实 PR 和人工界面验收不在本次通过声明中。

| ID | 自动验收项 | 入口与证据 | 状态 |
| --- | --- | --- | --- |
| WPF-A1 | Codex、本机 Orca、environment 共用流程；旧/不可用阶段成果观测不阻断报告与发布，原输入/来源保留 | 既有 WorkerStage unit 与三路 RoundTrip；unit.log、unit-boundaries.log、flow-local.log、flow-environment.log、flow.log（仅指定通过场景） | 通过（受控） |
| WPF-A2 | 反馈更新原分支及同一 PR；head 变化/无 HeadRevision 不阻断，另一 URL 不替换绑定 | 既有 WorkerStage/WorkerPRIdentity unit、local RoundTrip；unit.log、unit-boundaries.log、flow-local.log、flow-environment.log、flow.log（仅指定通过场景） | 通过（受控） |
| WPF-A3 | 正确 PR 实际合并后唯一交付并释放依赖；动态观测不阻断，真实身份变化/错 PR/未合并仍拒绝 | 既有 WorkerFlowDelivery/DeliveryGate unit、TestTaskDeliveryCommitPRFlow、local RoundTrip；unit.log、unit-boundaries.log、storage.log、flow-local.log | 通过（受控） |
| WPF-A4 | 源失败不被补报覆盖；发布结果查询失败后只对账；新流程等待合并重启不重发/重发布 | 既有 WorkerStage unit、RoundTrip 与 LocalPublishReport；unit.log、unit-boundaries.log、flow-local.log、flow-environment.log、flow.log（仅指定通过场景） | 通过（受控） |
| WPF-A5 | API/console、helper/test-feedback/debug-template 使用新格式；旧字段/比较/专属恢复/死代码清除，必要资源 inspection 保留 | 既有 API/console 投影、全仓编译及删除审计；projection.log（仅 API 场景）、projection-consumers.log、template-consumers.log、compile-final.log、cleanup-audit.md | 通过（受控） |
| WPF-A6 | development/v3 使用现有入口拒绝旧执行，无兼容路径或额外版本层；当前文档/正常材料没有活跃旧规则 | 既有 WorkerStageUnsupportedBeforeNativeProbe、删除审计；unit-boundaries.log、cleanup-audit.md | 通过（受控） |

既有多候选、放弃后晚合并、未知原生发送和并发恢复规则保持；仅在修改其实现时增加对应定向回归，不把整套历史验收扩为本次门槛。

实施后在本文登记实际源码与 Spec/Plan commit/path、验证环境/时间/命令、非零场景数、各项结果及证据目录。受控 environment 只代表适配接线，不记作远端真机或真实模型通过。历史证据保留版本，不用于本次通过记录。安装环境和旧 102 不纳入本次验证。

| ID | 人工内容 | 参与窗口与阻塞范围 | 状态 |
| --- | --- | --- | --- |
| WPF-M1 | 用户在新环境查看阶段显示、同一 PR 反馈更新及交付结果 | 用户参与，环境/时间待定；可后置，不阻塞实现或自动验收 | 待安排 |

## 本次执行记录

环境 Windows/amd64、Go 1.26.4；代码基线 8df615db，本 PR 工作树变更。Spec/Plan 路径为 SteerlineFlow/spec/worker-pr-flow.spec.md 和 SteerlineFlow/plans/worker-pr-flow.plan.md；提交后实际代码及文档 commit 登记在证据 source-identity.json。证据目录：C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/worker-pr-flow-20261012/。

| 实际命令/场景 | 结果与证据 |
| --- | --- |
| `go test ./internal/execution ./internal/execution/model ./internal/execution/publication -run '^(TestWorkerStagePublishReport\|TestWorkerFlowDelivery\|TestWorkerFlowProviderDelivery\|TestDeliveryGate\|TestWorkerPRIdentity\|TestWorkerPRBinding\|TestWorkerPrompt)(.*)/(head_advanced\|no_head\|.*)$' -count=1 -timeout=5m -v` | 12 个顶层测试、19 个子场景通过，execution 161.205s；unit.log |
| `go test ./internal/execution -run '^(TestWorkerStageReportRound\|TestWorkerStageUnsupportedBeforeNativeProbe\|TestDeliveryGateRequiresSelectedWorker\|TestMC5\|TestMC6\|TestMC7)$/(no_snapshot\|unavailable_inspection\|head_advanced\|source_failed\|unknown\|needs_input\|abandoned_merged\|late_after_delivery\|discard_full_history_and_exited_set)$' -count=1 -timeout=8m -v` | 6 个顶层测试、9 个子场景通过，254.663s；unit-boundaries.log |
| `go test ./internal/storage/sqlite -run '^TestTaskDeliveryCommitPRFlow$' -count=1 -v` | 6 个子场景通过；采样变化可交付、真实身份变化拒绝、换 PR 拒绝与重复交付验证；storage.log |
| `go test ./tests/integration -run '^TestWorkerStageLocalRoundTrip$' -count=1 -timeout=6m -v` | 162.89s 通过；六轮原会话/分支/PR、真实 Git 新提交与合并、缺 HeadRevision、重启及唯一交付/后继解锁；flow-local.log |
| `go test ./tests/integration -run '^TestWorkerStageOrcaEnvironmentRoundTrip$' -count=1 -timeout=4m -v` | 102.52s 通过；environment 为受控接线；flow-environment.log |
| 原三路组合命令中的 LocalPublishReport / OrcaTerminalRoundTrip | 分别 91.83s / 94.60s 通过；flow.log，仅引用这两项，整条命令未通过 |
| API/console 定向投影 | TestAPIWorkerStageQueryProjection 96.63s 通过（projection.log）；修正文案断言后 TestWorkerStagePresentation 通过（projection-consumers.log） |
| `go test ./tests/e2e/internal/driver ./cmd/test-feedback ./internal/execution/adapters/codex -run '^(TestDebugPublicationReusesPR\|TestSandboxScenarioRequiresBothOriginalMergedTasks\|TestWorkerRuntime)' -count=1 -v` | 4 个顶层测试、11 个子场景通过；两份 PS 模板真实更新隔离 bare Git，GitHub 事实 stub，无外部写入；template-consumers.log |
| `go test ./... -run '^$'`、删除审计、`git diff --check` | 全仓编译与检查通过；编译不记作业务测试；compile-final.log、cleanup-audit.md |

初次阶段单元有两项旧“head 变化应阻断”的断言失败，改为新规则后通过。初次集成将反馈放在重开服务后，正确触发既有恢复确认；原组合命令随后因总 8 分钟期限中断 environment，不计整组通过。夹具调整为反馈→重启→合并，local 与 environment 分开重验通过。初次 console 旧事实比对文案断言失败，修正后通过；原失败日志保留，test-results.json 汇总实际场景，没有 skip 或零匹配充当通过。

## PR #295 修改质量审计（2026-10-12）

审计基线 67ab146e，直接修复四项：

1. 首次发布查询到 closed_unmerged 时仍留在 publish，PR 后续不被轮询；改为已绑定 PR 在 await_merge 下人工等待。
2. PR 关闭后业务状态为 waiting_input，后续原 PR 实际合并不能通过交付 gate；空闲且无 pending 输入/真实问题/未知原生操作时恢复外部结算状态并唯一交付。已经合并不再新建可选修改问题；真实 pending 输入与原问题保留。
3. 两份调试模板只查询 open，关闭/已合并的原分支可能另建 PR；改查 all，明确拒绝把已有关闭/合并 PR 当作新发布。对账路径也统一把另一 URL 作为本候选阻断，保留原结果，不把换绑业务冲突向外抛出。
4. test-feedback 的合并时间只要求非空；改为 RFC3339Nano 可解析且非零，拒绝伪造时间证据。

证据目录 C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/worker-pr-flow-audit-20261012/。

| 入口与自动项 | 实际结果 |
| --- | --- |
| Plan 审计测试命令 worker 定向组，WPF-A2/A3/A4 | 6 个顶层场景通过，77.679s；worker-final.log。随后增加已合并状态下 pending 输入仍保留的断言，两个相关场景重验通过（34.215s，merge-pending-final.log）。涵盖关闭 PR 位置、后续实际合并唯一交付、不新增输入、原 pending 保留、换 PR 拒绝、正常发布/补报查询恢复 |
| Plan 模板/证据命令，WPF-A2/A5 | 两个顶层测试、6 个模板子场景 + 11 个证据子场景通过；driver 23.901s / feedback 0.830s，template-evidence.log。真实隔离 Git，GitHub stub，无外部写入 |
| 全仓仅编译、差异检查，WPF-A5/A6 | 编译和 git diff --check 通过；compile.log。未运行 full、真实模型或安装环境 |

初次新增夹具尚未停止 publish 原回合，且 neutral worker 的人工问题/pending report 未清理为正常 PR 等待现场，正确触发保护并失败；夹具改为已核实停止、当前服务下完成原轮次并使用正常 PR 反馈后通过。失败诊断保留，不靠生产代码取消原输入/原问题门槛来通过测试。审计结果没有恢复 SHA 成果锁、兼容路径、新状态或新版本层。
