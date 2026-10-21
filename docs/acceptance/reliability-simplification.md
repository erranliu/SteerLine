# 可靠性交付简化实施验收

依据[实施 Plan](../plans/reliability-simplification.plan.md)和[经审计方案](../architecture/reliable-delivery-chain.md)。本批完成判定/表达收口（阶段 A）、完整受理前绑定（B）和独立的同快照观察（F）。C/D 的原启动清单、安装租约、owned 启动上下文及离线续装，E 的产品命令承载合并尚未实施；不能据本批结果启用这些路径。

## 本批行为

- `prepare-deployment` 在一次应用启动协调中按 update/replace 目标调用既有停止、只读取证，update 必要时运行一次受控原安装恢复。replace 不进入恢复，reset 不接受为可变目标。原服务退出、兼容、资源结算分别表达，未知不提升成功。
- PowerShell 仅调用上述入口一次，核对退出码、home/目标、本次随机 invocation ID、三个暂存 EXE 的整体摘要及 Runner SHA，并保存完整结果；不再维护关闭/接管/恢复的第二套领域分支。实际安装前后的文件、路径及进程检查保持，报告不表示持续持有安装锁。
- Runner 发布 `steerrunner-service-binding/v1`，在实际 handler、原生/外部查询及持久受理之前校验完整实例、目录、构建、version、resource/协议与 ready。受理读门闩持有到 handler 返回，service stop 用写门闩关闭后续准入，保持 host→engine 的锁序，不增加跨外部 I/O 的 SQLite 事务。
- SteerLine 在绑定运行时取得能力后，只对明确覆盖的命令发送完整绑定并省略 status 预查询；未覆盖或缺能力的入口保持现有检查，完整模式缺字段不降级。每次实际 EXE 校验保留。
- Runner `observe` 在同一只读事务读取原 Run 和有界事件页，游标只越过实际消费事件；没有原生采集或 PR 刷新。SteerLine 普通已绑定观察使用一次调用，控制/停止仍保留必要对账；投影、事件及 cursor 经原 CAS 一起保存，交付前 PR 权威刷新保持。

## 自动验收

日期 2026-10-21，两个源码工作区均有已有未提交变更；本记录仅覆盖列明范围，不表示干净发布或已部署。证据根目录为 `.evidence/reliability-simplification/`。

| 项目 | 结果 | 实际验证与证据 |
| --- | --- | --- |
| RS-A01 / SIM-A01 | 本批通过 | `TestDeploymentPreparation` / `TestDeploymentPreparationPackage`：目标、缺字段、当前失败回执、正常停止、无原宿主不伪造结算和完整包摘要；`phase-a/stop-proof-final.log`，app 包 47.368s（含下项原证据基线） |
| RS-A01 原恢复与原事实基线 | 通过所选子集 | `TestDeploymentInterruptedRecovery/(stale\|missing\|preparation-update\|preparation-failed-cycle\|preparation-replace)`；一次恢复、新停机证明、严格目标零恢复、失败不重试、原未知保留；`phase-a/recovery.log`，76.933s |
| RS-A01 原关闭与绑定 | 通过 | `TestDeploymentRechecksFailedReceiptWithoutRestartOrRewrite`、`TestDeploymentRestartCandidateBindingsPreserveOriginalState`，`phase-a/stop-proof-final.log`；不改原回执或候选事实 |
| RS-A01 CLI/脚本 | 通过 | 参数拒绝见 `phase-a/preparation-final.log`；`scripts/test-deployment-stop.ps1` 见 `phase-a/scripts.log`，错 home/目标/调用/包/Runner、非零退出、坏 JSON 和准备期间改包均阻断；旧在线 takeover 预检及 pin 反例保留 |
| RS-A01 真实两语言接口 | 通过限定范围 | 实际构建 SteerLine CLI 与 Runner，临时空 home 调用生产 PowerShell 函数；`phase-a/public-cli.json`。Web 文件仅作摘要夹具，未执行；只验证包摘要算法和正式 CLI/脚本接线，不是完整安装验收 |
| RS-A02 / SIM-A02 | 本批通过 | Runner `TestSRFullBindingRejectsBeforeEffects` / `TestSRFullBindingStopAdmissionOrder` / `TestSRFullBindingCLI` 及原实例/停机围栏，`phase-b/runner-admission.log`；调用方完整参数、缺字段零调用、缺能力保留预查询见 `phase-b/client.log` |
| RS-A02 / RS-A06 实际公开 CLI | 通过限定范围 | 临时目录真实空闲服务、完整 capabilities、错实例/构建/resource 明确拒绝、observe 原 Run 不存在准确返回 not_found、正式 stop 与结算；`phase-b/public-cli-final.log`，5.563s。无模型/Run 创建/PR，非实际部署或重启资格 |
| RS-A06 / SIM-A09 | 本批通过 | Runner `TestSRObservationSnapshotPaging` / `TestSRObservationConcurrentCommits`，覆盖分页/空页后新增/并发及 row 不一致，见 `phase-f/runner.log`；调用方响应校验、一次观察、坏来源/事件超界/CAS 失败零半页，及既有重启/未知 outbox，见 `phase-f/caller.log` |
| RS-A03 / SIM-A03–A05 | 待实施、未验 | 完整 census 连续性、真实共享/独占安装租约、owned writer 上下文与停止优先；本批没有此能力 |
| RS-A04 / SIM-A07–A08 | 待实施、未验 | prepared/pending、原子整包提交与中断续装；当前安装不宣称可自动续装 |
| RS-A05 / SIM-A06 | 待实施、未验 | 产品控制文件与 DB 承载合并；原请求/效果补查及零重放仍用原实现 |

新增 L1 已纳入既有 quick-owner-deployment-recovery、quick-owner-runner-client、quick-owner-runner-bridge；固定分区/选择的定向测试通过。真实 Runner CLI 联验要求显式构建参数，单独运行，不把缺参数 skip 记成通过。

补充检查：Windows PowerShell 5.1 运行生产脚本的专项夹具通过（`phase-a/windows-powershell.log`）；原 caller/takeover/执行依赖闭包守卫通过，16.100s（`boundary.log`）。Runner storage 包全部规则通过，2.525s。文档检查覆盖 512 份 Markdown、3200 个本地导航链接，零错误；两仓 `git diff --check` 通过。上述包级/静态检查不代表完整固定 L1 或完整 E2E。

## 失败与修复

首轮合并运行旧 app 夹具使用 120s 总超时，发生总超时且一个新断言预期了过后的创建身份门槛。修正夹具为精确原 EXE/launch 后验证“匹配当前失败回执”阻断，拆为小范围验证；原失败不计通过。记录 `phase-a/first-attempt.txt`。

观察接口初次添加误用了既有观察文件名，编译暴露缺少原符号；原文件已恢复到原内容，新接口改为 `run_observation.go`，受影响包复验通过。真实 CLI 首次观察能力检查失败，发现能力放进了 version 输出而非 capabilities，已修正，失败日志 `phase-b/public-cli-observe.log` 保留，最终用新构建重新核验；没有把旧二进制测试结果继承给新接口。

## 本批自查与边界

实现自查补上了报告的 invocation/整包摘要绑定、准备前后重新散列；不能仅凭同 home/Runner 的旧 success 报告继续替换。原资源未知仍保留，空 home 仅报告 no_original_host，不宣称原生已结算；当前实例失败回执不能绕过。观察采用持久同快照，并保持历史 read_only 投影及原 CAS/PR 规则。

本批没有改用安装租约、没有启用直接离线恢复、没有移除普通命令文件通道、没有实现续装或长期散列缓存。现有受控原安装恢复继续使用。未运行全量、真实业务模型、安装现场或真实 OS 重启；本批不会覆盖历史无清单中断缺失的事实。

## 提交前主干同步

创建 PR 前 rebase 到 SteerLine `070bb7d7`（含 #370/#371）；唯一冲突位于 worker lifecycle 顶部新增段落，保留主干工作区命名前缀说明和本批部署恢复说明。同步后定向复验 app、steerline CLI、runnerclient、runnerbridge、test-feedback 五包，所选部署目标/包、CLI 参数、完整绑定、观察/原 outbox 和固定分区测试均通过；证据 `.evidence/reliability-simplification/pr/after-rebase.log`。文档检查为 514 份、3207 个本地链接，零错误；工作区与 diff 检查通过。此前的真实 CLI 证据仍仅对应其记录的构建 SHA，不将本次定向复验冒充完整部署或新原生资格。
