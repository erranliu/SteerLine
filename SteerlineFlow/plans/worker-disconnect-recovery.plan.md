# Plan：核心断连恢复及最小 L3 补齐

日期：2026-10-12。状态：工作树实施已完成；执行结果统一见专属验收，不部署 Runtime。普通开发，无 Task ID 或 roadmap 调度。

[Spec](../spec/worker-disconnect-recovery.spec.md) · [验收登记](../testing/acceptance/worker-disconnect-recovery.md) · [L3 场景及命令](../../tests/e2e/suites/recovery/README.md)

## 实施顺序

本轮可一次实施，按下面顺序形成一个交付；不是将原全面计划分批搬进同一范围。保留尚未提交的 `u` 修复。开始实现时固定实际 commit/dirty/tree 与 CLI 版本；规划基线不当执行基线。

### P0：按现有 worker 归口恢复，固定最小契约

1. 依据 [Spec 代码归口](../spec/worker-disconnect-recovery.spec.md#6-代码归口与解耦约束)列出正在使用的入口/调用关系：worker_recovery 的 owner gate、worker_lifecycle_recovery 的异步原请求核验、worker_lifecycle 的观察重接、worker_qualification 的资格重验、worker_feedback 的正式继续、worker_completion/staged_flow 的结算。每个入口明确“原生读取 / 事实接受 / 业务授权 / 副作用”职责；替代的分支删除，不新增平行流程。
2. 共同身份与未知操作守卫、事实分类、恢复版本和 Decision 更新在 worker_recovery/worker_recovery_facts 等现有 worker 文件复用；异步返回统一进入 service writer，保留取消/迟到版本拒绝和不阻塞兄弟执行。staged_flow 只处理受管阶段路由，不探测原生会话。
3. 在现有 LifecycleRecoveryEvidence/PolicyCapabilities 最小表达原资源可继续与同会话重建资格，修正 worker_feedback 对 ManagedProcess 的误用。Codex/Orca 分别提供证据，未知资格默认拒绝；不新增恢复端口层级/注册机制，不让 adapter 得到中心写权限。
4. recovery.go 的只读 lifecycle 装配复用冻结 snapshot/setup，显式限制依赖；不以完整运行态 Service 的 ownedProcesses/resources 必然存在为前提。和正式恢复共享事实语义，保持无资源能力时未核实；此局部整理纳入 u 修复，不扩成查询层重构。
5. 复用既有架构边界检查与非平台 fixture：无 ManagedProcess 但原资源可继续时走复用，不误入新 Attempt；原资源不可恢复则拒绝；同会话重建资格通过时走既有事务。只补必要子用例，不新增业务能力或 L3 场景。

### P1：修正失败分类与恢复入口

1. 定向检查 worker_lifecycle、staged_flow、worker_recovery_facts。优先沿用 RecoveryFacts、LastTurnOutcome、RecoveryVersion 与现有 Operation，不建立通用恢复事件框架。
2. 已停止轮次先补读原结果和检查原操作，再处理恢复资格；移除造成已知失败永久等待的提前返回。非 implement 的明确失败不以 final 缺失阻止记录失败，但不能因此把成功缺 final 或未知外部操作改成失败可重跑。
3. implement/report 的已知失败进入 blocked/waiting_input，复用已有 Decision，缺 Decision 时创建。相同事实保持版本，变化使旧授权失效。Query/console 分清“原轮次结束”和“任务成功”，显示继续资格与缺口；`u` 只读且错误可返回。

### P2：接回原观察与 Codex 原会话

1. Codex 复用 HistoryClient 读取冻结原 thread/cwd 和精确 turn；补充死亡进程路径的证据读回，SessionAvailable 不再只检查 ID 格式。调用放在 adapter，服务不解析 endpoint/私有会话文件。
2. 原进程活着只重接观察。已停止原受管树、无未知操作且原会话可核实时，复用 beginWorkerRecoveryAttempt/BindRecovery 创建新 Attempt/generation 并 resume 同 thread；实际恢复失败保留原身份/原因，不能 thread/start。
3. Orca 复用现行单终端 Observe/Reconcile，补齐中心离线后完整原结果结算及明确错误返回。已保存结果优先重放，不为重放访问已经丢失的 terminal；尚无可核实结果的资源丢失保持阻断。不增加 Orca 会话重建探针或能力平台。

### P3：复用 continue 与实施核验路由

1. 正式 Decision/answer 沿用 `continue`，不新增 verify-existing-result 协议或按钮。提交时重新检查 service/run/attempt/recovery/Decision 版本、原 session 空闲/恢复资格和未知操作；人工新轮次尚活动或空闲不可核实时不发新输入，只消费一次授权。
2. implement 失败及 已发送失败报告或 source 失败/缺失的 report 恢复，复用现有 implement 反馈入口创建新 StageInput；保留旧失败，原工作区在任务契约下重新核验，成功后生成新的 report source。
3. 主干 development/v3 已删除成果快照；首次 report 尚未发送且 source 成功时沿用原 report，其余 report 恢复回到 implement，不新增快照或内容摘要。最小定向测试覆盖路由，不增加独立候选确认/内容摘要系统。
4. 复用资源/输入意图事务与原请求对账。重复答案不增加发送；恢复期间服务再次退出后未知原发送保持未核实，不自动发新的请求。首次资源创建、原生问答和发布链的其他边界沿用现有逻辑，不额外改造。

### P4：补最小 L3 与真实链

新增三组 controlled 的独立入口、两条 live，保持 recovery suite 与现有选择规则。未实现/未枚举到顶层测试时不注册 suites.json。

驱动仅新增本轮使用的能力：精确 owned service/app-server kill、当前 Codex 协议 helper 的线程/轮次与调用计数、Orca 当前单终端 fake（local/environment）、一次读取断开/发送已接受回执丢失、显式 answer、Windows ConPTY 的 `u`/退出脚本。复用已有隔离 Git/SQLite、证据和 owned 清理。只在 helper/传输代理注入，不加产品后门或写业务 SQL。

不实现通用 commit barrier 框架、全窗口计数矩阵、远端 live fault wrapper。重复恢复重点验证：一条受控链在新输入接受而中心回执未知时退出/重开，原请求零重发；另一条在新结果已提交后重开，结果不重复结算。其余事务细节由相关定向 Go 测试覆盖。

Codex live 只验证一次原进程死亡、公开同 thread 恢复及实施重核；人工同 thread 完成成果的组合在 controlled 覆盖，不向真实模型链叠加手工续跑、二次故障等窗口。Orca live 只验证一次中心离线完成后原终端重接，本机链贯穿正常 PR/合并；原终端丢失分支在 controlled 验证。

真实 PR 仅使用 `erranliu/SteerlineTest` 本次专属 base/head，精确核对 SHA 与归属。既有 L3 使用隔离夹具；临时/AI 自动调试先运行 `scripts/new-debug-release.ps1`，不得使用安装目录或任务 102。

## 测试计划

全部自动项见[专属验收](../testing/acceptance/worker-disconnect-recovery.md#自动验收项)。以下场景的命令代号在 [L3 命令表](../../tests/e2e/suites/recovery/README.md#规划命令)展开；实现状态和实时结果见验收记录。

| 自动项 | 级别 / 场景与命令 | 环境 / 执行时点 | 通过条件与登记 |
| --- | --- | --- | --- |
| [CR-A01](../testing/acceptance/worker-disconnect-recovery.md#自动验收项)、CR-A02、CR-A05、CR-A06 | L3 C1 / recovery.observation-reconnect | Windows controlled；P1/P2/driver 完成后 | 同绑定接观察、离线原结果补记一次、失败有 Decision、u 正常/错误无 panic 且无写入；逐项登记 acceptance 与 C1 子用例证据 |
| CR-A03、CR-A04 | L3 C2 / recovery.codex-resume | Windows controlled；P2/P3/driver 后 | 原 app-server 已退出，同 thread 新 Attempt；102 型人工成果通过新 implement/report source；首次未发送报告沿用原 source，已发送失败报告重新实施的路由也通过 |
| CR-A03、CR-A05 | L3 C3 / recovery.resume-gates | Windows controlled；P2/P3/driver 后 | 会话缺失/旧写者/未知操作/身份变化阻断；重复确认/未知发送/再次重启零重复；Orca lost terminal 明确拒绝 |
| CR-A01、CR-A03、CR-A04 | L3 V1 / recovery.codex-resume-live | Windows live；对应 controlled 入口通过后 | 真实同 thread 恢复、受管新结果/final/usage、正常 PR/合并/唯一交付与清理 |
| CR-A01、CR-A05 | L3 V2 / recovery.orca-local-reconnect-live | Windows live、本机当前 Orca 单终端；controlled 与本机驱动通过后 | 原 terminal/Command/原生状态基线，离线完成、零重发、正常 PR/合并/唯一交付与清理 |
| [CR-A07](../testing/acceptance/worker-disconnect-recovery.md#自动验收项) | 定向 Go B1 / 现有边界与 worker lifecycle fixture；C1–C3 补两路径断言 | isolated；P0 后，最终候选复验 | 共用恢复规则、两 adapter 无中心写/交叉恢复依赖；原资源复用资格独立于 ManagedProcess；只读装配不访问缺失运行态能力；记录测试反馈 summary/源码审查与 L3 身份计数 |
| 必要既有回归 | L3 R1 / recovery.waiting-merge-restart | Windows controlled；最终候选 | 原待合并重启/交付门槛保持；登记原场景证据 |

各 L3 证据固定为 runner 的 summary.json、scenarios/<id>/scenario.json、必要子分支 result.json、before/after 身份与正式 Command、原输入/恢复发送计数、final/source/snapshot 和 owned cleanup。acceptance 登记实际路径与覆盖分支，Plan 不维护实时结果。

定向源码测试：P1 后覆盖失败与缺 final 分类/同事实版本；P2 后覆盖精确历史身份和 writer gate；P3 后覆盖实施/report 路由、过期/重复答案与新旧 source 隔离。实施时固定实际非零匹配测试名，入口 `go run ./cmd/test-feedback -- ./internal/execution -run '<精确集合>' -count=1`；adapter/model/console 使用对应包同类入口。它们是 CR-A02–CR-A06 的补充，不代替上述 L3。

B1 复用已有入口 `go run ./cmd/test-feedback -- ./tests/integration -run '^(TestCD67CoreBoundary|TestCD67AdditionalImplementation)$' -count=1`；worker 定向至少包含已有 `TestHostManagedStopRecoveryUsesOriginalLifecycleProof` 和 `TestRecoveryInspectionReadsBoundRunWithoutServiceResources`，资格子用例实施时固定名称，使用相同 runner 定向入口。既有测试通过不足以证明新增分支，须补“非进程原资源复用/拒绝”和两观察入口共用分类的断言；检查 imports、共享 worker 平台分支、唯一 Decision/结果提交点及被替代路径删除。证据写入 CR-A07，不用测试通过掩盖源码结构未整合。

最终执行固定 `go run ./cmd/test-feedback -level=L1` 与表中必要 L3。未选完整 L2/Full/无关组：本轮直接测试恢复路径，既有 smoke 不证明新恢复能力，全量测试不能替代精确场景。远端 live、首次准备/原生答案/publish 全窗口、实际硬件重启不在本轮验收项内；缺少这些验证时如实限制支持声明。

## 规划检查

仅修改设计/实施/验收/L3 文档与导航，运行文档链接、映射检查和 git diff --check。新增场景的 ID 必须与已注册条目无冲突，六项功能验收与一项结构验收都有入口映射。实现与执行结果统一登记 acceptance；Runtime 部署不在本次授权范围。

## 受控入口拆分

2026-10-12 实测：单个观察重接现场 215.39 秒通过，多现场串联超出原 5 分钟场景上限。按 P4/场景时限约定拆分入口，不提高时限，不增加故障范围。C1 对应 observation-reconnect、failure-results、missing-successful-result、terminal-local-reconnect、terminal-environment-reconnect；C2 对应 codex-resume、codex-report-resume；C3 对应 resume-gates、answer-replay、terminal-local-unknown-send、terminal-environment-unknown-send、terminal-resource-gates。C2 的重复答案检查拆到 answer-replay，保留 C2 完整交付。命令以 L3 README 为准。

C3 的 writer/会话空闲/冻结配置资格同时由 B1 worker/adapter 定向用例验证；实际对应原生 host 能力的 refusal 不通过构造新中心状态来伪造。不得将缺少的必需分支登记为通过。

四种 terminal 拒绝使用独立入口 terminal-resource-gates（丢失）、terminal-target-gate、terminal-incarnation-gate、terminal-gap-gate；共用 fixture/断言，每个现场只有一次原故障与一次中心重开。它们替代同一现场四次改故障重启，范围不增加，仍沿原 5 分钟期限。

C2 的 report-source-changed 使用仍活着的原资源，只改变原生 Git 工作区后正式继续，验证 report 失败且 source 变化时的新实施、新 report source 与交付；不叠加进程死亡或第二次故障。

## 主干整合后的审计验证补充

以 `efd492ca`（PR #295/#296）为基线，保持阶段快照删除及 Orca 原生状态判定。补 current/pending 迟到资格拒绝、BindRecovery 拒绝不产生半个 Attempt、Codex 身份不明拒绝、Orca 屏幕 idle 与主 agent working/waiting/身份变化/旧 done 不一致的最小定向回归。它们映射 CR-A03/A05/A07，命令为 `go run ./cmd/test-feedback -level=L1 -- ./internal/execution ./internal/execution/adapters/codex ./internal/execution/adapters/orcatransport -run '^(TestWorkerRecovery|TestWorkerStageReopened|TestCandidate(Open|Closed)PR|TestRecovery|TestTerminalRecoveryQualification|TestOrcaNativeState)' -count=1`；隔离环境，审计修复后执行，零失败/skip，证据登记在专属验收。Orca controlled 通过公开加密状态协议提供 native 基线/working/done，禁止产品注入钩子；未知发送没有接受证据时验收为拒绝接纳和零重发。受影响 L3 选择 local/environment reconnect、unknown-send、Codex 两种失败恢复与原 PR 等待合并回归；其余未改变场景引用原证据，不作为当前版本全矩阵重跑。live 仅按可用原生前置执行；任何失败如实登记。

PR #297 整合：B1 增加 `TestWorkerRecoveryDistinguishesPreparedRejectedAndUnknownInput` 与 `TestWorkerInputRejectedReceipt`（CR-A03/A05/A07），验证 prepared/明确 rejected 等待正式确认，unknown_session/unknown_input 无继续资格、不补造结果。以 `go run ./cmd/test-feedback -level=L1 -- ./internal/execution ./internal/execution/adapters/orcatransport -run '^(TestWorkerRecovery.*|TestWorkerInputRejectedReceipt|TestOrcaInput.*|TestRecovery.*|TestTerminalRecoveryQualification.*)$' -count=1` 在隔离环境执行，零失败/skip，证据见专属验收。重验 L3 本机重接/保存结果失去 terminal、environment unknown-send、native state gap，以覆盖配对 Bootstrap 和共同 worker 输入判定；不扩大全窗口矩阵。
