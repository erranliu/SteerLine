# Orca supervised 分步结果接线验收

> 归档于 2026-10-21：退役方案，原失败和未验项保留，不作为当前补验或实施队列。当前接入见[入口](../../../../../docs/integration/steerrunner.md)。固定 commit/原路径及证据内容继续按原记录取证。

2026-10-10 后续用户直接授权恢复 #84 的统一提示词设计，当前改动归[恢复验收](../../worker-prompt-convergence.md)。本页 payload 协议、探针和安装记录保留为历史构建事实，不能推定当前原生回复采集或真实交付已经通过。

归属：用户直接授权的 Orca supervised 结果修复；[Spec](../../../../archive/spec/retired/orca-supervised-results.spec.md)、[Plan](../../../../archive/spec/retired/orca-supervised-results.plan.md)。前置已验证范围见[原生消息探针](orca-supervised-result-probe-2026-10-10.md)。

| ID | 自动验收 / 条件 | 状态 / 证据 |
| --- | --- | --- |
| OSR-A1 | 完整结果、严格字段/身份/版本/限长和冲突；摘要不能代替正文 | 通过，TestSupervisedResultContract / RejectsUnboundOrIncomplete；重复/未知字段、错 Command/版本/来源、超限及冲突拒绝 |
| OSR-A2 | 本机/远端 observer、提交后 ack、重启只读恢复、原结算/进程与冻结结果 | 通过，TestSupervisedResultObserverPersistenceAndRestore；完成 envelope 持久化后、processed/ack 前取消观察，以序列化状态恢复；替换进程/未结算不返回 final，冻结后不查询已释放 worker。既有问答/消费回归通过 |
| OSR-A3 | 中心分步规则及原结果正文入库、幂等和冲突拒绝 | 通过，TestDeclaredFinalStoredExactlyAndReplaySafe / MissingProvenanceCannotAdvanceFailedImplement；既有 TestWorkerStage 组及最新 transitions/result binding 通过；公开 Body 查询保持 Unicode 正文，重放不重复、冲突不可覆盖 |
| OSR-A4 | 产品 supervised 能力、正式领取与固定目标接线 | 通过，TestSupervisedDevelopmentFlowClaimsWithDeclaredFinalCapability 正式创建/领取一条流程；原生不可达仍无副作用拒绝。integration TestSupervised 组及 app Supervised/CD67/TM06 定向通过 |
| OSR-A5 | 构建、架构边界和文档导航 | 通过，产品构建、boundary/e2e、文档导航及 diff 检查；安装后 check 核验配置/原生 authority/Git/PR，不领取任务 |
| OSR-A6 | 最新主干独立调试包初始化/启动及共同交付链 | 通过，TestAcceptanceHeadingLineEndings、TestAPIPrivateDirectoryOwnedWithModifyAccess 与 httpapi/model 包；TestDebugReleaseRuntime 在独立 simulate 包完成 execute/report/publish、合并、唯一 TaskDelivery、后继释放及重置；不代表真实 Orca 三轮交付 |

OSR-M1：真实原会话 execute/report/publish、反馈与精确 PR 合并交付，待安排专用产品驱动和窗口；不计本次自动回归已通过。人工参与仅为原生启动/安装现场确认。安装窗口以当前服务无活跃模型轮次为前置；运行版替换不擅自中断现有任务。

## 2026-10-10 实施与安装记录

源码 HEAD 40446a671badc901bcf868872a383b421d646803，modified=true；普通直接开发，未提交/推送 PR。完整原生前置探针与本次 Go 回归范围分开记录，没有把历史远端交付算作当前 L3。

结果直接冻结在现有 execution-state envelope（codec 12，旧 codec 11 明确拒绝）；核验结算后由相同 TurnEvidence 写入现有规范正文数据库，不增加第二种 reader、结果文件或私有日志访问。首次核验从同一 worker-show 快照取得身份和结算；已冻结结果可脱离外部资源恢复。缺声明结果的失败 implement 也保持 reconciling，不用失败摘要推进 report。

实际命令：

```powershell
go test ./internal/execution/adapters/orcaremote ./internal/execution/adapters/orca -count=1
go test ./internal/execution -run 'Test(WorkerStage|DeclaredFinal|Staged)' -count=1
# 新增正文回归和最终分步绑定验证
go test ./internal/execution -run 'TestDeclaredFinal|TestWorkerStageTransitions|TestWorkerStageResultBinding' -count=1 -timeout=120s -v
go test ./tests/integration -run 'TestSupervised' -count=1 -timeout=120s
go test ./internal/app -run 'Test(Supervised|CD67|TM06)' -count=1
go test ./internal/testkit/boundary/... -count=1
go build -o <证据目录>/steerline.exe ./cmd/steerline
python scripts/check_doc_links.py
git diff --check
```

产物目录：`C:/Users/lmas1/AppData/Local/Temp/steerline-orca-results-20261010-100143/`。adapter-tests.log、integration-tests.log、doc-links.log、installed-check.log、install.log、runtime-idle-verification.json、runtime-preservation.json 与安装前后 task 快照保留。其余 Go 输出见本聊天对应命令结果，未伪造不存在的完整 JSON 报告。

原生安装文件：`C:/Steerlineruntime/bin/steerline.exe`，SHA-256 `A160E70AC414C9BAD55D2D5FC08E0821B704B6BDC8CADCE506D653E7E2AC634D`。通过产品 install 原子安装，旧可执行文件和 README 保存于 `C:/Steerlineruntime/backup/orca-supervised-results-20261010-100143/`；execution.json 和业务数据库没有重建/直接修改。

现场变化：开发过程中 105 通过本机 Codex 形成了原 WorkflowRun `053716949e454db1a6861f98c5d65f6c`，当前 publish/blocked；101 原 Run `26e142c46231ef2dceb1133e5cb39bf8` 仍 await_merge/waiting_external。安装前只读核对两者 current_turn_id==last_stopped_turn_id、turn_ended_at 存在且无 pending input。实际 stop-service 检查时服务已离线，进程枚举也没有服务/控制台；因此没有中断在途轮次，安装后保持已关闭状态。安装前后两份公开 workflow 快照相等，不重派 101/105，不改变其阻塞。

已知独立限制仍为原生前置探针记录的本机长草稿提交、账号模型支持和 structured worker 续轮边界；本变更不宣称完整真实三轮业务交付已经通过。

## 最新主干与独立调试验证（2026-10-10）

该验证窗口已 rebase 到 `fbde4163a42639d5570dbe718b32cd42e95dc676`，modified=true；保留原修改并解决 e2e README 冲突，当时未提交/推送。此前 40446a 构建的记录仅为历史；后续提交前审计见下节。

调试包由 `scripts/new-debug-release.ps1` 从源模板创建，没有复制安装版数据库、配置或认证。AI 使用 simulate。首包初始化失败暴露 CRLF 验收标题识别问题，第二包启动失败暴露 Windows 重复要求修改已有 owner 的 WRITE_OWNER 问题；均记录失败后做最小修复，未弱化私有 ACL。

通过的调试包：`C:/SteerlineDebug/dev-fbde4163-dirty-20261010-105835/`。`evidence/rebase-owner-tests.log` 为 Orca adapter/httpapi/model 完整包回归；`evidence/rebase-debug-test.log` 为独立三阶段模拟交付及重置（129.95s）。该场景使用模拟 Codex app-server/PR 和真实 Git/SQLite，不是原生 Orca 模型业务验收。中心及 integration 最新回归日志在 `C:/SteerlineDebug/dev-fbde4163-dirty-20261010-104717/evidence/rebase-execution-tests.log` 和 `rebase-integration-tests.log`。

最新 app Supervised/CD67/TM06、boundary/e2e、文档导航和 diff 检查通过，命令输出见本聊天。产品 SHA-256 为 `644B4D3DEA1977110FE1BA3815C4763DFCA65578CFF9DF4A16812C77AEDF7AB6`，由新包 `C:/SteerlineDebug/dev-fbde4163-dirty-20261010-110711/bin/steerline.exe` 安装。安装日志在该包 `evidence/rebased-install.log`；被替换的可执行文件与 README 在 `C:/SteerlineRuntime/backup/orca-supervised-results-rebased-20261010-111700/`。

安装版任务重置为用户另外明确授权的实际管理操作，记录在[专属重置验收](../../steerlinetest-runtime-reset-2026-10-10.md)；安装目录没有作为测试环境。

## 提交前质量审计（2026-10-10）

用户授权审计、修复并提 PR。本聊天直接复核结果解析、冻结/恢复/ack、中心正文存储与分步规则、Windows ACL、CRLF 识别和验收记录；未另行调度 agent。分支 `codex/orca-supervised-results` 已再 rebase 到最新 `origin/main` 的 `41ae5515`（调试目录改为时间优先命名）。

确认并修复两项问题：首次结算改用同一 Show 快照时，遗漏 worker.agentTerminalHandle 与 terminal.handle 的交叉核对；外层 payload 默认解析会接受重复字段并取最后值，也会将错大小写的结果字段当作标准字段。现已补齐终端交叉核对及外层无重复/结果字段精确匹配，保持原生附加 payload 字段和字符串 payload 支持。恢复测试新增跨终端负例，并显式等待截止/关闭 observer，避免失败时测试挂起。既有健康轮询 fixture 补齐真实协议必需的终端引用。

最终定向验证：

- `go test ./internal/execution/adapters/orcaremote -count=1 -timeout=90s`：通过，含新增重复外层/错名/字符串 payload 和本机/远端错终端恢复回归；`.../evidence/audit-adapter.log`。
- `go test ./internal/execution/adapters/orca ./internal/product/httpapi ./internal/task/model -count=1 -timeout=120s`：对应包通过，命令输出见本聊天。
- `go test ./internal/execution -run '^(TestDeclaredFinal.*|TestWorkerStageTransitions|TestWorkerStageResultBinding)$' -count=1 -timeout=3m -v`：通过（101.058s），`audit-execution.log`。首次较宽 `Test(DeclaredFinal|WorkerStage|Staged)` 选择触发 120s 总时限，未将其记为通过；定向场景已完成。未跑 full。
- integration `TestSupervised` 组（52.440s）和 app `Supervised/CD67/TM06`：通过，`audit-integration.log`及本聊天输出。
- boundary/e2e、文档链接与 diff 检查：通过。
- 新独立包 `C:/SteerlineDebug/dev-20261010-112626-41ae5515-dirty/` 的 TestDebugReleaseRuntime：通过（154.11s），`evidence/audit-debug-runtime.log`。共同链路三轮、合并、唯一交付/后继释放及包内无备份重置通过；此处使用模拟 Codex，不替代 OSR-M1 原生 Orca 三轮真实交付。

本轮仅修改仓库并验证独立环境，安装版已安装的 fbde4163 构建与先前重置记录仍是其实际快照。没有把本轮待提交审计修复记为已安装或真实业务已验收。
