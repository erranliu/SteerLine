# SteerRunner 拆分后的测试精简验收

日期：2026-10-15。归属普通开发任务 [Spec](../spec/runner-test-convergence.spec.md) / [Plan](../spec/runner-test-convergence.plan.md)，无 Task ID，不修改 execution-map。本文件仅覆盖本次测试精简；历史 SteerRunner cutover、Orca 和沙箱验收保持各自事实。

## 验收项

用户已授权实施；已 rebase 到 `20f347cd`，保留主干四项 runtime 拆分及二并行，注册表由 28 项收敛为 7 项。实时状态见下方实施记录；完整 L2 正在实际执行，不以受控通过代替。

| ID | 自动验收及具体检查 | 状态 | 证据登记位置 |
| --- | --- | --- | --- |
| RTC-A1 | manifest 精确七项；L2 controlled→live、gate-only、失败/清理失败阻断；退役参数拒绝、非零匹配 | 待验 | 本项登记 test-feedback 选择测试及 gate-only 的 summary/用例清单 |
| RTC-A2 | caller fixture 与原请求、revision、游标、绑定、事务、交付/停止义务；controlled smoke 无原生依赖 | 待验 | 本项登记 client/bridge/taskrunner/SQLite/必要 HTTP 测试与 smoke 的命令、源码、summary |
| RTC-A3 | 完整 L2 本机 Codex 三阶段执行、真实 Git 产物与 SteerlineTest PR、真实 merge、唯一交付/后继、清理；证据缺失拒绝测试 | 待验 | 本项登记 gate/live 各报告、Runner version/SHA、公开 Run/final、产物 commit、PR URL/repo/base/head/head SHA/merge、TaskDelivery 和 cleanup |
| RTC-A4 | waiting-merge-restart 保持原候选/请求/投影并零重复副作用；installed-debug-isolation 实际通过 | 待验 | 本项登记五个 L3 scenario、实际构建提交、环境和清理结果 |
| RTC-A5 | 28 项及非注册专项去向清单完整；独有 Runner 断言有覆盖；失效脚本/原生基础设施无执行消费者，live 必需能力保留 | 待验 | 下方迁移明细及两仓精确 commit、测试名/命令/结果、依赖/脚本审计 |
| RTC-A6 | L1/Full 选择正确、普通/e2e 编译、当前文档与脚本一致；历史事实未篡改 | 待验 | 本项登记固定 L1、编译、导航/语法/差异及行数清单 |

每次实际执行登记：源码 commit/dirty、环境与 EXE 版本/SHA、精确命令、时间、退出码、实际命中/skip、passed/complete/coverage、原始产物路径及限制。原始日志保存于每次新建临时证据目录，仓库只记录最小摘要；失败和未验不得被后续通过覆盖。

## 跨仓迁移明细

下表为归属摘要；逐场景去向见后附清单。Runner 使用已提交 `af545563c58171c584bd35d098166cf39012e699` 的独立临时副本核验，不改动三个已有 dirty 工作区。步骤 1 必须展开到全部 28 项和非注册专项的具体断言；每行填两仓实际文件/测试名、命令和结果。已有对应测试可引用其精确执行证据，独有缺口按 Plan 步骤 1 补齐并执行。用户取消的 Orca 真机项保持取消，不新增必验。

| 原范围 | 处理要求 | 目标与实际证据 |
| --- | --- | --- |
| smoke.delivery / delivery.basic | 合并 SteerLine Task 断言，删除重复入口 | 待盘点、待实施 |
| smoke.local-codex | 保留完整 L2 live；公开结果＋真实 Git/PR＋Task 证据 | 待实施、待验 |
| waiting-merge-restart / installed-debug-isolation | 保留，受控注入上移到 Runner CLI | 待实施、待验 |
| 其他 recovery / delivery live | 执行专项归 Runner，调用方断言落回原 owner | 待核实 Runner 覆盖及 SteerLine 归属 |
| Orca terminal/environment / nativeworkspace | Runner 专项；不重启已取消真机验收 | 待核实具体测试/证据 |
| sandbox、usage、rollout、原生进程故障 | Runner 专项，清除 SteerLine 专属驱动/解析器 | 待核实具体测试/证据 |
| scripts/protocol/debug 中混合支撑 | 删除失效/专项，保留调试产品能力与最小 live 装配 | 待盘点消费者、语法和闭包 |

## 人工与环境前置

无新增人工验收。完整 L2 需要已有本机 Codex 登录、真实 Runner EXE 和 SteerlineTest GitHub 权限；不可用时 RTC-A3 保持待验/阻断。真实 PR 仅针对本次独立 base/head，保留 PR 历史。临时调试使用生成的新包，不访问安装现场，不复制认证/数据库。

## 本轮文档检查

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
- 新鲜完整 L2、L1、debug 包与其他定向检查正在执行；对应项未完成前不标全部通过。

## 28 项原注册场景去向

| 原 ID | 最终所有者 / 处理 |
| --- | --- |
| `smoke.delivery` | SteerLine 保留；公开 CLI/Task 或完整 L2；四项 runtime 不改业务断言 |
| `smoke.local-codex` | SteerLine 保留；公开 CLI/Task 或完整 L2；四项 runtime 不改业务断言 |
| `delivery.basic` | 并入 smoke.delivery：唯一交付、后继与 closed 静默 |
| `delivery.local-codex` | 正常真实交付由 L2 覆盖；SteerLine 重启由 waiting-merge-restart 覆盖，无独立重复 live |
| `recovery.waiting-merge-restart` | SteerLine 保留；公开 CLI/Task 或完整 L2；四项 runtime 不改业务断言 |
| `recovery.local-codex` | 正常真实交付由 L2 覆盖；SteerLine 重启由 waiting-merge-restart 覆盖，无独立重复 live |
| `runtime.source-build-isolation` | SteerLine 保留；公开 CLI/Task 或完整 L2；四项 runtime 不改业务断言 |
| `runtime.path-isolation` | SteerLine 保留；公开 CLI/Task 或完整 L2；四项 runtime 不改业务断言 |
| `runtime.service-write-isolation` | SteerLine 保留；公开 CLI/Task 或完整 L2；四项 runtime 不改业务断言 |
| `runtime.dual-home-isolation` | SteerLine 保留；公开 CLI/Task 或完整 L2；四项 runtime 不改业务断言 |
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
