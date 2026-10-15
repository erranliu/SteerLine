# SteerRunner 拆分后的测试精简验收

日期：2026-10-15。归属普通开发任务 [Spec](../spec/runner-test-convergence.spec.md) / [Plan](../spec/runner-test-convergence.plan.md)，无 Task ID，不修改 execution-map。本文件仅覆盖本次测试精简；历史 SteerRunner cutover、Orca 和沙箱验收保持各自事实。

## 验收项

用户已授权实施；已 rebase 到 `20f347cd`，保留主干四项 runtime 拆分及二并行，注册表由 28 项收敛为 7 项。本任务最小自动验收已完成，包含完整真实 L2 和单/双项目浏览器；三包批量生成脚本的独立限制见下方，未声明其整体通过。

| ID | 自动验收及具体检查 | 状态 | 证据登记位置 |
| --- | --- | --- | --- |
| RTC-A1 | manifest 精确七项；L2 controlled→live、gate-only、失败/清理失败阻断；退役参数拒绝、非零匹配 | 通过 | 本项登记 test-feedback 选择测试及 gate-only 的 summary/用例清单 |
| RTC-A2 | caller fixture 与原请求、revision、游标、绑定、事务、交付/停止义务；controlled smoke 无原生依赖 | 通过 | 本项登记 client/bridge/taskrunner/SQLite/必要 HTTP 测试与 smoke 的命令、源码、summary |
| RTC-A3 | 完整 L2 本机 Codex 三阶段执行、真实 Git 产物与 SteerlineTest PR、真实 merge、唯一交付/后继、清理；证据缺失拒绝测试 | 通过 | 本项登记 gate/live 各报告、Runner version/SHA、公开 Run/final、产物 commit、PR URL/repo/base/head/head SHA/merge、TaskDelivery 和 cleanup |
| RTC-A4 | waiting-merge-restart 保持原候选/请求/投影并零重复副作用；四项 runtime isolation 实际通过 | 通过 | 本项登记五个 L3 scenario、实际构建提交、环境和清理结果 |
| RTC-A5 | 28 项及非注册专项去向清单完整；独有 Runner 断言有覆盖；失效脚本/原生基础设施无执行消费者，live 必需能力保留 | 通过 | 下方迁移明细及两仓精确 commit、测试名/命令/结果、依赖/脚本审计 |
| RTC-A6 | L1/Full 选择正确、普通/e2e 编译、当前文档与脚本一致；历史事实未篡改 | 通过 | 本项登记固定 L1、编译、导航/语法/差异及行数清单 |

每次实际执行登记：源码 commit/dirty、环境与 EXE 版本/SHA、精确命令、时间、退出码、实际命中/skip、passed/complete/coverage、原始产物路径及限制。原始日志保存于每次新建临时证据目录，仓库只记录最小摘要；失败和未验不得被后续通过覆盖。

## 跨仓迁移明细

下表为归属摘要；逐场景去向见后附清单。Runner 使用已提交 `af545563c58171c584bd35d098166cf39012e699` 的独立临时副本核验，不改动三个已有 dirty 工作区。步骤 1 必须展开到全部 28 项和非注册专项的具体断言；每行填两仓实际文件/测试名、命令和结果。已有对应测试可引用其精确执行证据，独有缺口按 Plan 步骤 1 补齐并执行。用户取消的 Orca 真机项保持取消，不新增必验。

| 原范围 | 处理要求 | 目标与实际证据 |
| --- | --- | --- |
| smoke.delivery / delivery.basic | 合并 SteerLine Task 断言，删除重复入口 | 已合并；`rtc-gate-3` 及完整 L2 gate 已通过 |
| smoke.local-codex | 保留完整 L2 live；公开结果＋真实 Git/PR＋Task 证据 | 已实施；实际证据见下方 A3/A4 |
| waiting-merge-restart / installed-debug-isolation | 保留，受控注入上移到 Runner CLI | 已实施；实际证据见下方 A3/A4 |
| 其他 recovery / delivery live | 执行专项归 Runner，调用方断言落回原 owner | 已核实；28 项明细及 Runner owner 结果见下方 |
| Orca terminal/environment / nativeworkspace | Runner 专项；不重启已取消真机验收 | 已核实；公开契约保留，原生专项 owner 见下方 |
| sandbox、usage、rollout、原生进程故障 | Runner 专项，清除 SteerLine 专属驱动/解析器 | 已核实；公开契约保留，原生专项 owner 见下方 |
| scripts/protocol/debug 中混合支撑 | 删除失效/专项，保留调试产品能力与最小 live 装配 | 已移除失效消费者；边界/编译/脚本语法通过，共享浏览器已通过 |

## 人工与环境前置

无新增人工验收。完整 L2 需要已有本机 Codex 登录、真实 Runner EXE 和 SteerlineTest GitHub 权限；不可用时 RTC-A3 保持待验/阻断。真实 PR 仅针对本次独立 base/head，保留 PR 历史。临时调试使用生成的新包，不访问安装现场，不复制认证/数据库。

## 最初规划轮文档检查

仅检查规划文档，不计入 RTC-A1～RTC-A6 的产品验收。

- 文档导航：`python scripts/check_doc_links.py` 通过，434 个 Markdown 文件、2,613 个本地链接、0 错误。
- 差异格式：`git diff --check` 通过；新增三个未跟踪文档另用 Python 检查行尾空白，全部通过。
- Spec/Plan/验收映射：RTC-A1～A6 在三份文档均有对应要求与安排；目标表精确七项，包含 L2 controlled 与本机 Codex live，自动核对通过。
- 本轮只新增三份规划文档及两个导航入口，没有修改测试代码/注册表，没有运行 L1/L2/L3、模型或真实 PR 验收。

## Rebase 后实施与验证记录

- 代码基线：`20f347cdec99dc0f0701b41311112f7e330389b6`，分支 `codex/runner-test-convergence`；Spec/Plan 已同步 28→7 场景及 Web/debug 共享夹具消费者。
- Runner 来源：`af545563c58171c584bd35d098166cf39012e699`、clean；版本 `0.1.0-dev.1`，Windows/amd64，构建 SHA-256 `7ff7941cb625cec63bf32180058d5113c6f54799c567289135b6420014da312e`。独立副本位于系统临时目录 `steerline-rtc-runner-af54556`；不修改其他 Runner 工作区。
- RTC-A1/A2 定向 owner 已通过：`.evidence/rtc-owners-3/summary.json`；client/bridge/taskrunner/SQLite 与 test-feedback 选择/证据拒绝，实际命中且 complete/passed。
- 受控 L2 已通过：`.evidence/rtc-gate-3/summary.json`，smoke 66.64 秒、清理通过、coverage=gate-only；包含唯一交付、后继和交付后静默，不能当完整 L2。
- 早期两次 gate 调试在修正夹具本地仓库/PR 刷新字段后主动终止，原报告保留于 `.evidence/rtc-gate-1`、`rtc-gate-2`，均不算通过。
- 第一轮完整 L2：`.evidence/rtc-l2-live-1` 的 gate 通过；live 在本次新 clone 的 CRLF 配置变更导致 `git rm` 拒绝时失败，未执行模型或创建 PR。已将 clone 前的 Git 配置固定为 LF，并修正初始化前 cleanup 不应要求已存在 Task DB；保留失败，不以失败现场重试。
- 后续完整 L2、L1、L3 和 debug 结果见下方；失败现场不复用、不改写为通过。

## 28 项原注册场景去向

| 原 ID | 最终所有者 / 处理 |
| --- | --- |
| `smoke.delivery` | 保留 controlled；公开 CLI＋唯一 TaskDelivery、后继 ready/未启动与交付后静默 |
| `smoke.local-codex` | 保留完整 L2 live；真实本机 Codex 三阶段、Git/PR/merge、唯一交付与停止 |
| `delivery.basic` | 并入 smoke.delivery：唯一交付、后继与 closed 静默 |
| `delivery.local-codex` | 正常真实交付由 L2 覆盖；SteerLine 重启由 waiting-merge-restart 覆盖，无独立重复 live |
| `recovery.waiting-merge-restart` | 保留 caller 重启；原候选/Run/请求/游标继续，零重复业务副作用 |
| `recovery.local-codex` | 正常真实交付由 L2 覆盖；SteerLine 重启由 waiting-merge-restart 覆盖，无独立重复 live |
| `runtime.source-build-isolation` | 保留主干独立隔离范围与二并行；受控执行改用公开 CLI fixture |
| `runtime.path-isolation` | 保留主干独立隔离范围与二并行；受控执行改用公开 CLI fixture |
| `runtime.service-write-isolation` | 保留主干独立隔离范围与二并行；受控执行改用公开 CLI fixture |
| `runtime.dual-home-isolation` | 保留主干独立隔离范围与二并行；受控执行改用公开 CLI fixture |
| `runtime.sandbox-git.local-codex` | Runner tests/integration TestSRPermissionQualification 与 adapters/codex TestLaunchServerFixesWindowsSandboxBeforeStarting；原生取证解析器退役；SteerLine 调试发布模板复用测试保留 |
| `recovery.observation-reconnect` | Runner ClassifyRecovery、ReliableRecoveryAndAuthorization、Codex recovery_history 等原资源资格；SteerLine 原请求/显式授权由现有 caller owner 保留 |
| `recovery.codex-resume` | Runner ClassifyRecovery、ReliableRecoveryAndAuthorization、Codex recovery_history 等原资源资格；SteerLine 原请求/显式授权由现有 caller owner 保留 |
| `recovery.resume-gates` | Runner ClassifyRecovery、ReliableRecoveryAndAuthorization、Codex recovery_history 等原资源资格；SteerLine 原请求/显式授权由现有 caller owner 保留 |
| `recovery.codex-resume-live` | Runner ClassifyRecovery、ReliableRecoveryAndAuthorization、Codex recovery_history 等原资源资格；SteerLine 原请求/显式授权由现有 caller owner 保留 |
| `recovery.failure-results` | Runner WorkflowSourceAndFormatRules、SRReportFreezesOriginatingNativePolicy、Codex HistoryMixedTurnRetainsOnlyCompleteFinal；SteerLine 公开 final 投影由 runner_reads 保留 |
| `recovery.missing-successful-result` | Runner WorkflowSourceAndFormatRules、SRReportFreezesOriginatingNativePolicy、Codex HistoryMixedTurnRetainsOnlyCompleteFinal；SteerLine 公开 final 投影由 runner_reads 保留 |
| `recovery.terminal-local-reconnect` | Runner adapters/orcatransport lifecycle/native_state/recovery_state 与 protocol/orca；原生执行专项，不在 SteerLine 保留 |
| `recovery.terminal-environment-reconnect` | Runner adapters/orcatransport lifecycle/native_state/recovery_state 与 protocol/orca；原生执行专项，不在 SteerLine 保留 |
| `recovery.codex-report-resume` | Runner WorkflowSourceAndFormatRules、SRReportFreezesOriginatingNativePolicy、Codex HistoryMixedTurnRetainsOnlyCompleteFinal；SteerLine 公开 final 投影由 runner_reads 保留 |
| `recovery.terminal-local-unknown-send` | Runner adapters/orcatransport lifecycle/native_state/recovery_state 与 protocol/orca；原生执行专项，不在 SteerLine 保留 |
| `recovery.terminal-environment-unknown-send` | Runner adapters/orcatransport lifecycle/native_state/recovery_state 与 protocol/orca；原生执行专项，不在 SteerLine 保留 |
| `recovery.terminal-resource-gates` | Runner adapters/orcatransport lifecycle/native_state/recovery_state 与 protocol/orca；原生执行专项，不在 SteerLine 保留 |
| `recovery.answer-replay` | 调用方部分由 taskrunner input、runnerbridge outbox/SQLite control；原生回答重放由 Runner adapters/codex TestAsyncQuestionNativeIdentityOptionsReplayAndFinal |
| `recovery.terminal-target-gate` | Runner adapters/orcatransport lifecycle/native_state/recovery_state 与 protocol/orca；原生执行专项，不在 SteerLine 保留 |
| `recovery.terminal-incarnation-gate` | Runner adapters/orcatransport lifecycle/native_state/recovery_state 与 protocol/orca；原生执行专项，不在 SteerLine 保留 |
| `recovery.terminal-gap-gate` | Runner adapters/orcatransport lifecycle/native_state/recovery_state 与 protocol/orca；原生执行专项，不在 SteerLine 保留 |
| `recovery.report-source-changed` | Runner WorkflowSourceAndFormatRules、SRReportFreezesOriginatingNativePolicy、Codex HistoryMixedTurnRetainsOnlyCompleteFinal；SteerLine 公开 final 投影由 runner_reads 保留 |

非注册专项同步清理：Orca probes/state-transitions、sandbox-setup 及脚本、原生 usage/rollout/process fault、protocol appserver/terminal/staged、controlled recovery/history。其失效测试工具及旧取证格式随门槛退役，不将工具删除声明为原生能力已验；Runner 原生行为由其对应 owner 承担。debug-template 发布脚本是 SteerLine 自有产物，保留本机模板的实际 Git/PR 复用测试；Web/debug 共享消费者改查公开请求和 Run。

## 实际验证汇总（2026-10-15）

均为 Windows/amd64、Go 1.26.4；除说明的真实 L2 外不调用模型。原始目录均为本次独立目录，`.evidence/` 为本地忽略的日志区；JSON 中保存完整命令、源码 commit/dirty、时间、实际命中、退出码和 EXE SHA。此处的相对证据路径是登记标识，不是长期 CI 工件地址。

| 验收项 | 命令/结果与证据 |
| --- | --- |
| A1/A2/A3/A5/A6 | `go run ./cmd/test-feedback -out .evidence/rtc-final-owners -- ./cmd/test-feedback ./internal/runnerclient ./internal/runnerbridge ./internal/taskrunner ./internal/storage/sqlite ./tests/e2e/internal/driver -run '^Test(L2|Runner|SteerRunner|E2E|Retired|Manifest|Level|DebugPublication)' -count=1`：6 包、67 个含子项测试、0 skip，passed/complete，14.91 秒，源码 `039225ec`＋证据元数据修改。`rtc-owners-3` 另含较早完整 caller owner 定向结果。 |
| A2/A5/A6 | `go run ./cmd/test-feedback -out .evidence/rtc-boundaries-final -- ./internal/testkit/boundary/... ./tests/integration ./tests/e2e/internal/... -count=1 -timeout=3m`：7 包、96 pass、2 skip，16.82 秒，passed/complete；skip 为需显式独立 debug 包的场景，已另验。最终辅助清理后 `go test ./tests/e2e/internal/... ./cmd/test-feedback -count=1 -timeout=3m` 再通过。 |
| A1/A2 | `go run ./cmd/test-feedback -out .evidence/rtc-gate-3 -level=L2 -env=controlled`：`smoke.delivery` 66.64 秒、cleanup=true、coverage=gate-only；未算真实 L2。 |
| A3 | `scripts/test-l2.ps1 -SteerRunnerExe <本次固定EXE> -OutDir .evidence/rtc-l2-live-3`：源码 `039225ec` clean，09:47:06–09:53:37 UTC，gate 83.17 秒＋live 285.54 秒，两个场景均实际 pass，cleanup=true、coverage=full-L2；详见 `summary.json`、raw log、scenarios 和下方精确 PR。 |
| A4 | `go run ./cmd/test-feedback -out .evidence/rtc-recovery-3 -level=L3 -suite=recovery -env=controlled -scenario=recovery.waiting-merge-restart`：`039225ec`＋证据/模板修改，87.18 秒，原候选/请求保持，pass、cleanup=true。 |
| A4 | `scripts/test-runtime-isolation.ps1`：四项 runtime 在源码 `88fc3120` clean 实际通过，二并行、cleanup=true；完整命令与结果在 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261015T094017.475341800Z/summary.json`。后续没有改变这四项实现。 |
| A4/A6 | 新包 `C:/SteerlineDebug/中文 test-357ec5ad50e2/dev-20261015-174018-88fc3120` 的 `TestDebugReleaseRuntime` 78.12 秒通过，证据 `.evidence/rtc-existing-debug.log`；由生成器成功建立且未使用过的独立包，覆盖 freeze/stop/reset。 |
| A6 | `go run ./cmd/test-feedback -out .evidence/rtc-l1 -level=L1`：passed/complete、132.7 秒；`-build-only` 的 `.evidence/rtc-build/summary.json` 通过、34.62 秒；`go test -tags=e2e ./tests/e2e/... -run '^$'` 通过。编译不算场景通过，没有运行 Full。 |
| A2/A6 | Web 定向 `npm --prefix web test -- src/features/runs/state.test.ts src/features/runs/answer.test.tsx`：9 项通过；Web build 通过。修复真实 `pr_ready` 分类，closed/abandoned 仍只读；浏览器实测单独登记。 |
| A5/A6 | 全部保留 PowerShell 脚本 AST 解析通过；`python -X utf8 scripts/check_doc_links.py`：434 文件、2,606 本地链接、0 错误；`git diff --check` 通过。生产边界及全部 tag 依赖检查由 boundary/integration 实际通过，没有保留测试原生协议消费者。 |

### A3：真实 PR 与公开证据

- Runner 固定版本/SHA 见上方；模型固定本机 `gpt-6-luna` / `medium`，引用宿主已有登录，没有复制认证。
- Run `c2975b0f729923fda18c1a1f35e63fb2`，candidate `d93df4633f4a4c2096f6caae1714a763`；公开事件三个唯一 canonical final，分别来自真实实现、报告、发布。业务文件为已提交的 `result.txt`，内容 `real model delivery`。
- [PR #258](https://github.com/erranliu/SteerlineTest/pull/258) 已真实合并；repo `erranliu/SteerlineTest`，base `codex/e2e/74b0ed28a9aa411881bd3873ea81a96c/smoke.local-codex/base`，head `codex/runner-c2975b0f729923fda18c1a1f35e63fb2`。
- head/产物 commit `9df76c129e1a26e32abe7da7365343c2b543a450`，merge `87ffc0bbeb0ddbe4582de11a2d9c2ec572f1ea6b`；唯一 TaskDelivery=1、绑定相同 candidate/PR、后继 ready 且未启动，最终公开 stopped 和 cleanup 均通过。
- live 通过后将 final 来源元数据强化为显式 Run/candidate/key 三元组；用本次真实公开事件重新构建并通过当前严格校验，保存 `rtc-l2-live-3/revalidated-public-proof.json`，同时错误 Run/candidate 的拒绝测试通过。没有为元数据格式变化额外重复模型任务，也未将此重放声明为第二次 live。

### 失败与限制（保留事实）

- `rtc-l2-live-2` 真实模型/PR [#256](https://github.com/erranliu/SteerlineTest/pull/256) 已合并且 Task 已交付，但 test2json 管道未关闭导致报告超时，**不计通过**。改为直接拥有测试 EXE、写 raw 文件后转换有限日志。该次专属 base/head 经 PR 精确身份及 SHA 核对后用 lease 清除，保留 PR 历史；不操作其他分支。
- `rtc-l3` 直接在工具 Job 中测试遇到 SourceBuild install 拒绝/调用方停机早于 stop 对账；`rtc-recovery-2` 同样过早停机。已分别用既有独立 Windows 进程入口、公开 stopped 等待解决；后续成功证据独立保存。
- 三包连续生成的第三包因同名进程不可查询而被 installer 保守拒绝；另一次并行 npm ci 导致构建失败。保留失败，不改 installer 权限规则、不碰安装现场，后续包生成串行。已成功生成的新鲜独立 debug 包按原入口实际通过；不能将失败的三包命令整体记为通过。
- `rtc-infra-final` 曾扫描到 `.evidence/` 中一次性校验 Go 程序触发依赖守卫；删除已用完的一次性源文件后原守卫通过，未放宽边界。
- 浏览器旧现场分别暴露原生文本断言重复、`pr_ready` 未纳入 Web、提交时 caller revision 已过期；失败日志在各新包 evidence 保留，最终场景结果见后续登记。保留服务未结算时的失败停机诊断，不把进程已退出等同业务清理通过。

### A5：Runner 专项实际所有者

固定源码 `af54556` clean，未修改 Runner。最终在既有独立 Windows 同用户进程入口重跑已选 owner 共 7 包、57 个顶层测试，全部 pass，0 fail；精确 `go test` 参数与完整测试名保存于 `.evidence/rtc-runner-final-command.json`，逐测试 JSONL 在 `.evidence/rtc-runner-final.jsonl`。这次核对重复执行的原因是补齐精确命令证据与权限专项。早期直接在工具 Job 中 `TestSRPermissionQualification` 的 service start exit=4 失败保留于 `rtc-runner-permissions.jsonl`，不计通过；独立进程未改变 Runner 权限策略即通过。

| Runner owner（路径） | 实际通过的顶层测试 |
| --- | --- |
| `internal/application` | `TestSRNativeAnswerRejectsAmbiguityBeforeIntent`, `TestSRReportFreezesOriginatingNativePolicy` |
| `internal/execution/adapters/codex` | `TestAsyncQuestionNativeIdentityOptionsReplayAndFinal`, `TestAsyncQuestionObserveSteerAndReconcile`, `TestAsyncQuestionObserverUsesNewInputCommand`, `TestGitPermissionLinkedWorktreeWriteArgsCompilesOnlyResolvedDirectories`, `TestHistoryAdapterRejectsOversizedNativeCursor`, `TestHistoryDiscoveryRejectsWorkspaceReplacementAndKeepsSourceAttempts`, `TestHistoryMixedTurnRetainsOnlyCompleteFinal`, `TestHistoryPageCandidateIsRecreatedAfterDiscardAndLateFinalKeepsTurnIdentity`, `TestHistoryPositionCannotCrossItsOriginalBinding`, `TestHistoryReadRejectsReplacedProjectBeforeOpeningClient`, `TestLaunchServerFixesWindowsSandboxBeforeStarting`, `TestManagedCommandIndependentEffortAndOrderedPermissionArgs`, `TestManagedOrcaLaunchFreezesPermissionsAndRejectsArgs`, `TestPreparationApplicabilitySkipsTerminalLaunch`, `TestRecoveryHistoryIdleRequiresOriginalStoppedTurn`, `TestRecoveryHistoryRejectsChangedFrozenBindingBeforeNativeRead`, `TestRecoveryProcessIdentityRequiresPositiveNativeProof`, `TestWorkspaceLifecyclePrecedesSessionAndRejectsRetargeting` |
| `internal/execution/adapters/orcatransport` | `TestPreparationApplicabilityOriginalTerminal`, `TestRecoveryRejectsProviderReplacementInsideOriginalTerminal`, `TestRecoveryRejectsStaleScreenIdleAgainstNativeMainAgent`, `TestTerminalChangedIncarnationCannotComplete`, `TestTerminalClippedScreenDoesNotBlockNativeFinal`, `TestTerminalLifecycleReplyReuseAndExactStop`, `TestTerminalLocalAndRemoteUseIdenticalReturnAndNativePorts`, `TestTerminalNativeFinalRecoveryNeverReplaysInput`, `TestTerminalPreviousReplyCannotCompleteNewTurn`, `TestTerminalRecoveryQualificationIsIndependentOfManagedProcess`, `TestTerminalReplyRequiresOriginalStartedAndEnded`, `TestTerminalUnknownSubmissionOnlyObservesOriginal`, `TestWorkspaceCreationReceiptRecovery`, `TestWorkspaceCreationRecoveryRejectsChangedIdentity`, `TestWorkspaceCreationUsesConfiguredTimeout`, `TestWorkspaceRepositoryFollowsProject`, `TestWorkspaceTerminalSurfaceReadiness` |
| `internal/execution/adapters/protocol/orca` | `TestBoundTerminalRejectsRuntimeAndIncarnationReplacement`, `TestTerminalCloseRequiresExactRuntimeHandleAndPtyProof` |
| `internal/execution/reliable` | `TestClassifyRecovery`, `TestReliableExpiredIntentCannotExecute`, `TestReliableFreshActionKeepsOriginalIntent`, `TestReliableInputOutcomeBoundaries`, `TestReliableLateFacts`, `TestReliableOutcomeCannotTreatMissingReceiptAsSuccess`, `TestReliablePreparationApplicability`, `TestReliablePreparationFacts`, `TestReliableReconciliationRejectsForeignIntent`, `TestReliableRecoveredNativeStartRequiresOriginalIntent`, `TestReliableRecoveryAndAuthorization`, `TestReliableTurnIdentityAndNotifications`, `TestReliableUnknownEffectAndGateCannotResubmit`, `TestReliableUnverifiedReceiptRequiresOriginalReconciliation` |
| `internal/execution/workflow` | `TestSRWorkflowDevelopment`, `TestWorkflowExecutionFacts`, `TestWorkflowSourceAndFormatRules` |
| `tests/integration` | `TestSRPermissionQualification` |

以上 owner 对应本文件 28 项去向；调用方答案/重放、原请求、游标、唯一交付和停止义务由 A2 的原包承担，未随执行专项删除。沙箱资格和实际启动权限参数由 integration/codex owner 承担；旧 SteerLine 专属 usage/rollout 解析器作为工具退役，不将其格式统计声明为新的原生功能验收。Orca 专项使用 Runner transport/protocol fixture，不声称已重新执行被用户取消的远端真机验收。

### 共享消费者与精简量

- 单项目 `scripts/test-web-smoke.ps1 -SteerRunnerExe <本次固定EXE> -WebAddress 127.0.0.1:19765`：新包 `C:/SteerlineDebug/dev-20261015-182151-039225ec-dirty`，浏览器 2.1 分钟通过、cleanup 通过（processes/listeners=0、stop_errors 为空）。证据为包内 `evidence/smoke.log`、`smoke-cleanup.json`、browser trace/截图/原 Run 身份/回执。第一次已过期回答被拒绝后，显式刷新保留草稿并重新确认，最终公开 reply 仅一次；未重发 unknown 请求。
- 模拟模板 `pr_poll` 最终为 `15s`，解决原 `100ms` 与公开 CLI 多次往返成本不匹配；产品默认配置、授权规则未变。`5s` 的中间现场仍在宿主消费控制前发生过期拒绝，保留其失败，不作为完成证据。
- 相对 rebase 基线 `20f347cd`：测试代码与测试基础设施（`tests/`、`internal/testkit/`、`cmd/test-feedback/`，含普通 `.go`）79 文件、增加 866 行、删除 5,959 行，**净减少 5,093 行**；脚本/模板另净减少 398 行。Web 保留其原有独有场景并重接公开 fixture。完整分类结果 `.evidence/rtc-diff-stats.json`，不把将测试 helper 移入普通 Go 计作删测。
- 四项 runtime 和实际 debug Runtime/freeze/stop/reset 已通过。原三包批量生成脚本整体未通过的限制仍保留；本任务最小调试验证按 Plan 收口为生成成功的新鲜独立单项目 Runtime 与单/双项目浏览器，没有把未执行的批量门槛虚报通过。

- 双项目第一现场 `C:/SteerlineDebug/dev-20261015-182548-48c1a009`：两个项目实际 online，但已有 `outsideJob` 在回执文件出现、写句柄未关闭时读取 JSON 触发 EBUSY，浏览器失败；包内日志/trace 和 cleanup 保留。修复测试启动器原子发布结果文件后另建新包验证，不将本次失败记为通过。

- 双项目最终现场 `C:/SteerlineDebug/dev-20261015-182924-48c1a009-dirty`：`scripts/test-unified-startup.ps1 -Package <该包> -Simulate` 的既有 `project-switch.spec.ts` 实际通过，3.4 分钟；两个项目在线、原请求丢回执后查询、Web 重启保持原公开 Run、实际受控发布/合并交付、CLI 任务/回执、活动 Run 的宿主停止诊断、重启后原绑定及零重复 submit 均通过。包内 `evidence/unified-startup.log`、`web-project-switch.json`、browser trace 保存明细。
- 最终 `evidence/cleanup.json`：processes=0、listeners=0、stop_errors=[]、shutdown.clean=true；两个项目均 local_closed/locks_released/resources_settled=true。回执原子发布改动已由浏览器独立命令和外层独立进程实际消费，不另外建立测试矩阵。
- RTC-A1～A6 按本任务最终最小验证范围通过；批量三包命令、早期失败与未重跑的 Orca 真机仍保留各自事实。没有运行 Full，没有改动安装现场或其他 Runner 工作区，也未创建本实现的远端 PR。

## 提 PR 前再次整合主干

- 2026-10-15 rebase 到 `7cd2ebe1`，包含 #328 安装部署与 #329 执行可观测性；仅手工解决 `docs/acceptance/README.md`、`docs/testing.md`、`tests/e2e/README.md` 三处文档冲突，保留部署/消息指南以及双安装目录保护。`git range-diff` 核对原五提交，执行代码无额外改写；原证据源码 SHA 如实保留，后续没有重复模型、真实 PR 或 runtime 矩阵。
- RTC-A1/A2/A5/A6：Plan 中追加的七包定向 owner 在 `.evidence/rtc-pr-rebase/summary.json` 保存精确命令/源码/环境，64 项实际通过、0 skip、passed/complete，9.505 秒；两个 HTTP 当前读取/路由 owner 单独实际通过。
- RTC-A2/A6：当前 Web state/answer/last-message/runs 四文件共 16 项实际通过；Web TypeScript/Vite build 通过。e2e tag 全包编译通过，仅编译不计业务场景重跑。Markdown 导航 439 文件、2,622 本地链接、0 错误；差异格式检查通过。
- 原提交 `36b00790 / 88fc3120 / 039225ec / 48c1a009 / 77e9bcc1` 分别重放为 `4ee90244 / 78447064 / 0c911c38 / da4222d5 / b2273538`；既有真实/受控验收记录中的旧 SHA 代表实际采集时版本，不是 rebase 后重跑声明。
