# Spec：App 领域专项与实际执行参数报告

日期：2026-11-03。状态：已实施，自动验收与独立审核通过。对应 [Plan](../plans/app-domain-test-suites.plan.md) 与 [验收规划及断言梳理](../testing/acceptance/app-domain-test-suites.md)。普通开发，不启动 roadmap；用户已授权按本文实施、保留原断言，独立审核通过后提PR。

## 背景与成果

规划源码为 `4920f452d2358b62c3230f7b5f4a04516a4aa862`，包含上一轮测试早期反馈调整。其实际证据见[上轮验收](../testing/acceptance/test-early-feedback.md)：Full 执行1572.484秒，其中 App 包1260.187秒，约80%；环境专项已收敛，部署和 Roadmap 修改仍缺少对应正式 L4 集。上述历史数据只解释动机，不作为本轮通过或提速证明，也不假定上一轮 PR 已合并。

另有实际报告缺口：local、scoped、E2E 的顶层 `collectEnvironment` 接收 nil 参数，可能显示默认并发；真实组命令则显式指定 `-p=1/-parallel=1` 或 `-test.parallel=1`。多组和 test2json 包装命令不能用一份假定参数概括。

本轮成果限定为：新增 `go.deployment`、`go.roadmap` 两个 isolated/L4 专项；如实报告各组实际执行参数；完成四个重型根的断言、层级和准备步骤梳理。遵循[测试架构](../../SteerlineProjectGuideDoc/testing-architecture.md)、[测试指南](../testing.md)与[报告约定](../testing/reports.md)。

## 范围与不变项

允许修改 `cmd/test-feedback` 的固定选择、源漂移检查、参数报告及对应测试，项目测试指南、L4 列表、正式 JSON、报告/路径说明和专属文档。复用既有分组、解析、managed 和进度，不新增调度框架或第二份运行时测试清单。

本轮不修改生产业务、现有业务测试/辅助流程、业务原根或子测试的正文/断言/超时/轮询/次数、提示词正文/组装/黄金、CI、Runner 协议、数据库、安装环境。执行器测试允许追加新选择与参数报告覆盖、调整对应夹具接线，但原有断言和反例不得删除或弱化。重型用例不拆分、不迁移、不去重、不改 fixture、不加分段埋点；只做源码与既有日志的文档分析。发现缺陷单列证据，不在本 Spec 下扩张业务修复范围。

L1 仍为现有固定集，L2 仍为 `smoke.delivery`；仅既有清单检查自然覆盖新增定义，不追加功能根。L3 场景及并发不变，不新增并行组合验证。Full 枚举、分区、实际命令和调度不变，仍执行全部普通 Go 测试；不以两个专项替代 Full，不做 App 并行或 Full 分片。无真实模型、原生 Runner、远端、真实测试 PR 或部署活动。

## S1：部署专项

新增稳定 ID `go.deployment`，入口为：

```powershell
go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.deployment -out <新的外部绝对目录>
```

固定串行分组：`internal/app/host` 全部普通测试；Windows 下 `internal/localservice` 全部普通测试；`internal/app` 的明确根；最后独立执行`internal/runnerclient/upgrade_test.go`的`TestUpgradeDrainedUsesCandidateProofAndBothBusinessReaders`原根及全部故障子例。该原根来自主干直接部署门禁变化，不筛入其他runnerclient根。host 验证共享的私有状态/原子写入、进程与启动器基础边界；localservice 的现有直接测试仅在 Windows 编译，其他平台不建立这个组，不把零测试包算通过。每个实际建立的 owner 组必须非零、零实际 skip。当前不直接纳入独立部署 PowerShell 端到端脚本，它们仍归原 L4 独立入口。

App 选择固定为下表，执行每根的全部子测试：

| 源文件 | 必选根 |
| --- | --- |
| `deployment_prepare_test.go` | `TestDeploymentPreparation`、`TestDeploymentPreparationPackage` |
| `deployment_recovery_test.go` | `TestDeploymentRecoveryReadiness`、`TestDeploymentInterruptedRecovery` |
| `deployment_record_windows_test.go` | `TestDeploymentRecordScriptMeetsRecoveryACL`，仅 Windows 执行 |
| `application_verify_stop_test.go` | `TestDeploymentRechecksFailedReceiptWithoutRestartOrRewrite`、`TestDeploymentRestartCandidateBindingsPreserveOriginalState`、`TestDeploymentCandidateVerificationKeepsOriginalBinding` |
| `runner_preset_test.go` | `TestRunnerPresetFreeze`，整根保留摘要规则、活体替换、显式重启和跨构建拒绝 |

当前 Windows 为9个 App 根，其他平台8个；owner 根数按当次普通源码枚举，不能硬套其他平台数量。该专项证明上述部署准备、回收、冻结与共享基础边界，不宣称完成真实安装升级或覆盖所有服务功能。

## S2：Roadmap 专项

新增稳定 ID `go.roadmap`，入口为：

```powershell
go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.roadmap -out <另一新的外部绝对目录>
```

固定串行三组：`internal/roadmap` 全部普通测试；App 明确根；host 明确根。`internal/roadmap/model` 当前无普通测试，不作为已执行 owner 组，仍随 Full 编译枚举。

| 包 / 源文件 | 必选根 |
| --- | --- |
| App `roadmap_test.go` | `TestRoadmapAdmissionAtomicNumbersSourceAndReplay`、`TestRoadmapSourceAndMissingDocumentCapability`、`TestRoadmapUnknownReceiptKeepsBusinessEffectPending`、`TestRoadmapPublicationRejectsAnotherInFlightGeneration`、`TestRoadmapGeneratedIndexCannotOverwriteAnotherSource` |
| App `roadmap_preparation_test.go` | `TestRoadmapStaticInitializationAndPublication`、`TestRoadmapStaticInitializationRejectsGitOperation`、`TestRoadmapInitializationPendingReceiptReconcilesWithoutWriting`、`TestRoadmapPublishedGuideMustBeValidUTF8AndBounded` |
| App `roadmap_publication_test.go` | `TestRoadmapPublicationAndAdmissionWithRealGit`、`TestRoadmapPublicationAndAdmissionWithCRLFCheckout` |
| host `native_state_preflight_test.go` | `TestRoadmapPublicationStateCanReopenWithoutRelaxingDirectoryGuard` |

当前 App 为11根、host为1根；roadmap owner 当前20根，以全量普通枚举执行，不写成固定20根筛选。保留普通链中的本地真实 Git、受控 provider、正式文档与 Task 写入、LF/CRLF 差异、原请求重放和越界拒绝。App 的 `roadmap_generation_test.go` 目前只有辅助代码，仍纳入源码漂移审视。

`roadmap_live_test.go` 的 `TestRoadmapLocalCodexPromotion`、`TestRoadmapPreparedSteerlineTestLocalCodexPromotion` 带 e2e 标签，明确排除；不通过开标签、自动模型调用或移除受控链断言补范围。真实模型/浏览器/远端发布仍属于原 live 或独立入口，不把本专项通过登记成它们通过。

## S3：选择与证据完整性

两个专项复用原串行 `localGroup`，owner 组不带 App 的 `-run`，定向组使用明确根名及精确匹配，不能运行宽泛的名称正则后凭数量判通过。真实命令保持既有 `-json -count=1 -timeout=25m -p=1 -parallel=1`，不继承 GOFLAGS 缩小选择。

固定选择有源文件到根的单一 Go 定义。上述 App 文件及 host 的定向文件核对全部声明根；无条件检查 Windows 源码和已明确排除的 live 文件/标签，运行时按当前 GOOS 选取活动组。新增 deployment/roadmap 前缀测试文件、根增删/改名、已登记文件平台/标签变化、正式定义漏根/多根均阻断并要求维护；非前缀跨文件根也按上表检查。辅助文件新增根不能静默漏选。额外发现的领域相关文件先审视归属，不按测试名自动扩张执行集合。

将相同漂移检查接入既有 L1 清单根与两个正式入口；不额外扩张 L1 固定根。普通 owner 仍运行包全量。按 `(package, root)` 及原始包/测试开始、终态双向唯一校验，子测试全保留；缺失、额外、重复、孤立、skip、零匹配、任一失败或缺证据使退出码、summary、managed、进度一致不通过。跨专项复用同一基础用例允许，每次专项内部不得重复执行同一分配。

新增专项各自输出 managed，不冒充 go.full；不在 `tests/e2e/suites.json` 伪造普通 Go 条目。同步项目指南、L4 列表、test-list.json、清单校验与路径/报告说明；JSON 只使用现有支持字段。规划轮不提前登记未实现命令。

## S4：实际参数报告

在现有原始命令旁，记录每个实际执行组的 `execution_parameters`。直接诊断报告采用同义信息，Web 组明确非 Go 测试；不改变实际参数或调度来迎合报告。字段至少表达命令类型、测试次数、测试期限、测试并行、Go 包并行、标签/筛选和来源，适用时记录既有控制器/包级 worker 设置。具体字段类型在实施的同包结构中统一，不能维护第二份期望参数表来替代实际命令。

- 从最终执行命令及实际构建/调度事实生成，区分 `go test`、冻结测试二进制、`go tool test2json` 和 Vitest；显式值、未显式的 Go 默认、未知、不适用分开表达，不把未知写成0或通过。
- 识别 `-p=1` / `-p 1`、`-parallel=1` / `-parallel 1` 及 `-test.parallel` 等真实形式；test2json 自身 `-p <package>` 是包名，不能误当构建并行。冻结二进制的标签来自其实际构建记录，测试并行/次数/期限来自二进制实际参数；不伪造该阶段发生 `go test -p` 编译。
- 原始 argv 和原始 stdout/stderr 继续保留。GOMAXPROCS 环境值、报告控制器的 `runtime.GOMAXPROCS`、子测试显式并行及外层组/包 worker 是不同事实；不能把控制器值宣称为未观测的子进程值。
- 顶层并发信息仅在同一适用维度可一致汇总时写共同值；不同组写 mixed 并定位各组，非 Go 组不强行合并。设置尚未形成或执行前失败时写未执行/未知，不填默认成功值。
- 覆盖现有 L1、Full、本地专项、诊断和 E2E 报告路径；Web 的不适用也明确。仅报告结构及其生成/核验改变，现有失败判定、managed 内容摘要和进度规则保留。新字段进入原 summary 的证据摘要；不新增旧报告兼容读取、迁移或回退路径。

本轮不重构准备/执行/清理计时，不新增机器负载调度、跨聊天窗口协调或自动重试。修正参数报告不等于取得新性能或并行能力证明。

## S5：重型根只梳理

梳理对象固定为 `TestRunnerPresetFreeze`、`TestDeploymentInterruptedRecovery`、两项 `TestRoadmapPublicationAndAdmissionWith*`。按根/实际子例/循环分支记录：前置事实、动作、正反断言、失败条件、直接验证层级、依赖与副作用、准备/启停/查询步骤、清理责任和现有覆盖来源。动态 fault/state 循环完整展开清单，不只记顶层根或最长子例。

区分“当前直接验证”“仅作为前置”“其他层已有证据”“待确认重复”。例如 Runner 冻结夹具中的存储 ready 不能冒称真实环境准备已执行；CRLF 用例的原字节/实质修改拒绝不能当成 LF 链重复删除。纯摘要规则是 owner 规则候选，真实绑定/重启/发布/持久化/清理属于原集成链；层级分析不改变当前 L4 归属，不把纯规则塞入 L1。

本轮交付的是可复核的断言矩阵与后续候选建议，不实现任何移层或去重。每个建议要给出等价覆盖尚缺的证据；“可能重复”不等于允许删除。历史耗时只作定位；无需为这份梳理重复长集或承诺 Full 提速。

## 验收要求

| ID | 成果与通过条件 |
| --- | --- |
| ADT-A1 | 正式 go.deployment 按平台精确选择、owner 非零全量、原根/子项完整且0skip；普通真实夹具通过，不触及安装现场。 |
| ADT-A2 | 正式 go.roadmap 的完整 owner、App11根及host1根通过，原LF/CRLF和状态重开拒绝保留，live根不执行。 |
| ADT-A3 | 源/标签/平台/清单漂移与缺失/额外/重复/skip等负例，经过原最终汇总、managed及进度均不能通过；原集合不被改写。 |
| ADT-A4 | 各类真实命令的参数证据准确，默认/mixed/未知/不适用不混淆；L1、Full、本地专项、诊断、Web、代表性E2E报告能与原始命令逐项对上。 |
| ADT-A5 | 四个重型根的全部断言/动态分支、前置与清理完成矩阵；原函数、子例、正文与期限未变，移层/去重只保留为后续候选。 |
| ADT-A6 | 最终L1/L2及受影响执行器回归通过；最终Full原普通根零缺失、完整通过、0实际skip；编译项单列，不继承上轮通过。 |
| ADT-A7 | 正式文档/清单/实现一致，提示词相关正文差异与字节审计、实际报告及断言保留经独立审核；无新增人工项。 |

本轮不设专项必须少于几分钟或减少某比例的门槛，只登记实际耗时、范围及条件。功能、完整性和报告真实性不能由速度替代。实施与最终真实结果统一登记专属验收；实际状态以专属验收为准，最终Full与证据终审均已通过。


### 实施基线集成

实际基线为`cca5114297564df176b27145e66fda71d2582682`，上一轮PR #438已合并。本轮继承主干Roadmap静态初始化四根：`TestRoadmapStaticInitializationAndPublication`、`TestRoadmapStaticInitializationRejectsGitOperation`、`TestRoadmapInitializationPendingReceiptReconcilesWithoutWriting`、`TestRoadmapPublishedGuideMustBeValidUTF8AndBounded`，位于`internal/app/roadmap_preparation_test.go`，全部纳入明确映射；App原7根全保留，当前11根。重型源码按此集成基线冻结，主干已有更新不记作本轮修改；本轮仍不改业务/测试辅助正文。


主干直接部署门禁补充：`internal/runnerclient/upgrade_test.go`的原升级排空根精确纳入go.deployment，原候选证明、两业务读取器与故障子例保留。Roadmap live文件的两个e2e根均显式排除且核对实际标签，非当前平台源码也检查。


## 实施终态

本轮两个L4领域专项、实际参数报告及仅文档的重型断言/层级梳理已完成。最终Full为72包632根1652终态、0fail/skip，原626根零缺失且恰新增6个执行器根；L1/L2/代表L3/Web和完整执行器owner均通过。976个生产/业务测试/资产文件及四个重型根/辅助保持原字节，1760运行文件冻结一致；原断言、期限和调度未放宽。实际命令、旧失败、参数/managed证据和独立终审结论统一见[验收记录](../testing/acceptance/app-domain-test-suites.md)，不新增本轮性能提速结论。
