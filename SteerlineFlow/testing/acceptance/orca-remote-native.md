# Orca 远端原生链路接入验收

日期：2026-10-19。归属用户直接授权的 SteerRunner 远端 helper 删除任务，不属于 roadmap 调度。Runner Spec/Plan 位于相邻工作区 `C:/Users/lmas1/.codex/worktrees/ee21/SteerRunner/SteerlineFlow/spec/orca-remote-native.spec.md` 与 `.plan.md`，最终现场记录在该仓 `SteerlineFlow/testing/acceptance/orca-remote-native.md`。

远端 preset/example/debug 生成器不再输出 helper 路径和 SHA；公开 configuration schema 来自本次 Runner capabilities。逐 Run 重验增加只读 stop_evidence_scope 解码，经 ApplicationStopVerification 原有输出展示。部署成功仍取 resources_settled，未知及旧记录不因 scope 改变授权。

定向验证命令：`go test ./internal/app ./internal/runnerclient ./cmd/steerline -run 'TestDeployment|TestServiceSettlement|TestStoppedVerification|TestOrcaNative|TestEnvironmentCheckPublishedContract|TestSteerRunnerUsesPublishedSchema' -count=1 -timeout=180s -v`。7 项顶层测试通过；cmd/steerline 在此筛选无匹配，不计通过，另运行 `TestCLIExecutionProfileFlagRejectsUnrelatedActions`，1 项、3 子例通过。部署失败回执重验、原始实例围栏、旧未知阻断与 scope 解码覆盖在现有测试内。

跨仓公开 EXE 验证非 skip：STEERRUNNER_EXE 固定本次构建，version=0.1.0-dev.1，SHA256=0f0efbb45a2ad880db181ce956c9f57dff1c39df45bd0e0058e80f15fbdb1d7c，resource digest=7d52c66f03d0a6ea4ef968e107091553f2cc5329be162034ba5f963e1162b4f9。`TestEnvironmentCheckPublishedContract` 启动独立空闲服务，读取公开 capabilities/schema，确认 local Check 可用及远端 Check unsupported，测试服务已退出。`TestOrcaNativePublishedConfiguration` 验证正常远端配置合法，helper 键（含空值）拒绝。没有读取 Runner 私有数据库、启动模型或部署产品环境。

原始日志和两仓源码逐文件 SHA 清单：Runner `.evidence/orca-native-implementation-20261019T042843Z/consumer-tests.log`、`consumer-source.json`。当前修改未提交，实际基线 commit 见清单；不以 Runner 的模型测试代替 SteerLine 产品端 live 验收，本次调用方接入采用既有隔离单测和公开 CLI 契约验证。


## Rebase 后最终接入

Runner 已整合到 origin/main 95ae8a72792d2b11cc59b938257165b472b5b4de。保留上游 workflow v2 / 独立配置；更新公开 fixture、示例、debug-template 与 Orca 调试生成器到 workflow v2，unknown-version 反例使用 3，不新增业务状态机。消费端仍只消费公开 CLI。

`go test ./internal/runnerclient -count=1 -v -timeout=180s`：14 项顶层测试通过，包括有实际 EXE 的公开 Check/schema、workflow、远端无 helper 配置与 stop scope。`go test ./internal/app ./cmd/steerline -run 'TestDeployment|TestServiceSettlement|TestStoppedVerification|TestCLIExecutionProfileFlagRejectsUnrelatedActions' -count=1 -v -timeout=180s`：2 项部署与 1 项 CLI 测试通过。

最终 EXE 0.2.0-dev.2、SHA A1BF1A2C46E887532BB11FB627452C99DFEF44C0A58A0AC02CF24F0641063EA4、resource c7b197e382299922599ae31faf1f69f54db447dee949035a8181332780ac411f。以此重新跑公开契约/配置/停止解码 3 项通过；原始日志在 Runner `.evidence/orca-native-rebase/consumer-final-exe.log`，前次完整组分别为 consumer-runnerclient-final.log / consumer-app-final.log。未部署安装或产品 debug 环境，产品端模型闭环不作为本次消费者单测通过。


最终原生开始回执修正后，Runner SHA EB8AF1DEB2811BB951DE35231FC2B06E21C61C1AB3AE1EB74E339479EC9B96BF（版本/resource 不变）。三项公开配置/停止解码/实际 EXE 契约复验通过，原始 consumer-start-wait.log。Runner rebase 后真实活动停止已通过，但完整 workflow 受最新主线 aaf5480 的 unknown-outcome 门阻塞；本消费者接通测试不代表该业务闭环通过。尚未修改结果契约或部署产品环境。

Runner 末轮仅修旧远端 v5 stop codec，构建 SHA 87DBF4FC2622171C9E7C2F4E670665A7E331EF6D8F33938090CE65985CCBDAF2；三项公开契约复验通过，consumer-legacy-state.log。原生 workflow outcome 冲突仍待决定，未记闭环通过。


用户现已确认区分 unreported 与显式 unknown：CLI v1 字符串扩展，消费端不修改调度/部署，原值往返与持久化不可篡改由 TestRunnerNativeOutcomeContract / TestRunnerPublicReadsKeepOriginalFinalAndCallerBinding 验证。最终 Runner SHA 29D44DF0315190FDD533E7AB091C9450ABF868E766F0A46985DE2045DEBFEF3A，15 项 runnerclient（含真实 EXE）全部通过，storage 公共读取用例通过。原始日志 consumer.log / consumer-public.log 位于 Runner .evidence/orca-native-unreported-20261019。历史记录不重新分类，业务闭环以 Runner 本轮最终验收为准。


Runner 最终闭环已通过：#601 的精确 head 合入本轮专属测试 base，160.00s；active stop 123.06s，通过且收尾。NativeOutcome=unreported 与业务 PRready 分别核实。workflow v2 的 pr_ready feedback 仍 invalid_state，native input 数3→3；不恢复旧反馈路径。本消费者没有部署产品环境，历史 unknown 未被重分类或自动放行。完整事实及当前源码/EXE/driver 身份见 Runner 最终 acceptance。

## 提交前最终审计

Runner 审计修复持久 StageResult 的原 outcome 经 parser/报告回放丢失的问题：即使历史 Succeeded=true，unknown/failed/cancelled/未知值也不能晋升为可发布来源。消费端未新增成功推断、历史迁移或调度/部署例外；仅补齐两个既有文件的 gofmt。

最终 Runner EXE SHA256 819D2079231B8FDD33F96F1264B3279E40896B1FACD3EEE8B761417F85D0D639，版本/resource 不变。固定 STEERRUNNER_EXE 后执行 runnerclient 的 `TestRunner|TestEnvironmentCheck|TestOrcaNative|TestStoppedVerification` 与 sqlite 的 `TestRunner`，24 顶层测试、零 skip，8.352s/6.475s，通过。原始 consumer.log、两仓源码哈希及 Runner 最终补验在 Runner `SteerlineFlow/testing/acceptance/evidence/orca-remote-native-final-audit-20261019/`。

Runner 此构建真实 full-access/default 296.63s，通过实施→原报告→发布→核验→专属测试 base 合入→停止，测试 PR #610；反馈拒绝且输入数4→4、零远端 helper、offline scope/settled 均成立。本消费者未部署产品环境。提交分支 codex/orca-native-contract 从 origin/main d12779733a21cfe4397baf1d51e73881a1ba15ee 建立，只携带本任务改动；原本机 main 的额外合并历史未进入 PR。
