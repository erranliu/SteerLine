# Plan：固定开发流程规则独立

任务 ID：94；任务名称：固定开发流程规则独立。

依据 [Spec](development-flow-rules.spec.md)，实时结果归[专属验收](../acceptance/development-flow-rules.md)。2026-10-14 草稿，调查基线/RoadmapGuide/SpecGuide版本 `5a24abec3c10d184a6e57962e154854bc6011a9f`；实际启动另固定文档与代码版本。

候选核对：2026-10-14继续保留草稿，未纳入execution-map，当前不安排实施。Workflow业务步骤/解析/提示词所有权与正在实施的Worker三层重构直接重叠；拟议development包不能与其workflow包作为两套owner同时实施。三层成果进入主干后重审Spec/Plan与验收，不自动恢复旧方案；原调查和测试安排保留作参考，以下旧并行建议不能作为开工资格。冲突说明见[本轮草稿](../roadmap/drafts/roadmap.md#与worker三层重构的冲突核对)。

## 实施与整合

1. 沿 acceptStageTurn/renderStageInput/startStageFeedback 调查步骤规则、严格解析、报告源/原身份/权限、重试与原子提交，区分纯规则和final提交、Decision/恢复资格、PR I/O。阅读reconcileStageFacts中未知发布真实合并分支，作为#95共同不变量。
2. 新建 `internal/execution/development`，迁入严格结果类型/解析、基础结果指令及步骤决策/下一输入构造。使用已有model StageInput/StageResult和具体规则值；调用方提供固定ID/时间与已核验资格/原因，规则复制自己使用的slice/map，不保留输入可写引用。只返回本次用到的具体声明，不设计通用Transition/Effect框架。
3. actor 将规范turn事实及既有资格映射为规则输入，将返回声明一次应用到当前候选，再沿现有commitWorker屏障提交。保留frame/final归属检查与重复first-wins；actor仍负责已核验PR事实、绑定和未知合并交付，report规则不硬加阶段完成门槛。删除原纯解析/规则重复实现，真正平台I/O/恢复/派发继续留原owner。
4. 新增纯包 `TestDevelopmentRules`，覆盖有效/重复/错源、严格JSON重复键/消费字段大小写别名冲突/错误类型/尾随正文拒绝，以及额外说明字段、完整json/普通围栏、CRLF、既有业务字符串trim和不参与业务的metadata大小写不同键接受。保留TestWorkerStageStrictResult与TestWorkerStagePresentationDoesNotBlockResult的双向样本，不新增DisallowUnknownFields或正文JSON抽取。另覆盖正常步骤、单次重试/补报、源失败、未知、反馈/恢复资格及输入值不变。原TestWorkerStage*保持actor/事务验证，仅按新owner适配调用。
5. 在 testkit/boundary 新增专属 `TestDevelopmentBoundary` 文件（以上两个名称/新包当前不存在），验证all-tag源码与Windows/Linux的项目生产依赖闭包：纯包只消费model值/标准库，不到parent execution、app/product/storage或adapter。检查纯包/项目helper直接导入或调用获得的SQL、文件/进程/网络I/O与输入参数中的业务句柄；不能把fmt等标准库内部的os依赖误判为纯规则取得I/O能力。为直接/经项目helper的违规做最小负例，不扩大既有白名单。新增包普通Go测试可通过定向runner长期执行，不新增E2E场景。
6. 增加actor反例 `TestDevelopmentUnknownPublicationMergedDelivery`（尚不存在），使用既有隔离fixture建立原未知publish、读到真实merged后完整交付，错误PR拒绝。该测试归本项；#95复用自己的交付用例输入不修改此测试，不新增共同helper。
7. 执行测试表，登记真实结果到本项验收；更新架构和并行指南中纯规则owner，结构自查旧规则已删除。正式执行后按真实PR/复审/合入同步execution-map，当前不写入执行表。

## 并行与合入

#92/#93可并行。本项仅拥有staged_flow的纯规则接线；#95保留CompleteNativeTask调用契约，不触碰此文件，二者无硬依赖。默认建议#94先合入，#95后合入核对未知发布/交付组合。#96的运行时接线不改本项纯规则，建议其后集成，不写成任务依赖。publicmodel/持久格式/提交契约冻结，必要变化须重新核定范围与消费者。

最终候选记录实际基线及已包含的兄弟成果；DF94-A3的未知发布真实合并与DF94-A4三个原stage入口始终运行，覆盖已有交付/runtime接线，不要求等待#95/#96。若本项后合入且基线已含#96，另复验其交付后清理失败场景（测试表条件行）；缺该已交付入口/零匹配失败。未含#96则记条件未触发，不当作通过或依赖。

## 测试计划

新增包和测试名称为实施计划入口，步骤2/4/5/6补齐后才可执行，不宣称已有能力。runner每行默认新系统临时 `steerline-test-feedback/<运行时间>/` 保存summary.json、原始Go日志/stderr；将行标识/绝对目录和真实枚举登记本项验收“执行记录”。

| 验收项 | 级别/组/场景、环境 | 命令 | 时点/通过条件 | 证据位置 |
| --- | --- | --- | --- | --- |
| [DF94-A1](../acceptance/development-flow-rules.md#自动验收计划) | 定向纯owner与新增TestDevelopmentBoundary；普通Go/Windows和Linux依赖闭包 | `go run ./cmd/test-feedback -- ./internal/execution/development -run '^TestDevelopmentRules$' -count=1`；`go run ./cmd/test-feedback -- ./internal/testkit/boundary -run '^TestDevelopmentBoundary$' -count=1` | 迁入后、PR前；脱离Service、无I/O、输入不可变、旧重复删除；Linux仅依赖结构声明 | 两个runner目录；DF94-A1 / rules、boundary |
| [DF94-A2](../acceptance/development-flow-rules.md#自动验收计划) | 定向规则/actor；纯值及既有隔离fixture | `go run ./cmd/test-feedback -- ./internal/execution -run '^(TestWorkerStageTransitions|TestWorkerStageResultBinding|TestWorkerStageStrictResult|TestWorkerStagePresentationDoesNotBlockResult|TestWorkerStageReportRound|TestWorkerStagePublishReport|TestWorkerStageReopenedSourceQueuesOnlyReport|TestWorkerStageFeedbackUsesExactFrozenNativePrompt)$' -count=1`，同时消费DF94-A1规则结果 | 接线后、PR前；歧义拒绝与解释字段/围栏/trim接受同时保持，步骤/重试/反馈源/重复及资格保持 | runner目录；DF94-A2 / actor、parser-positive-negative |
| [DF94-A3](../acceptance/development-flow-rules.md#自动验收计划) | 定向actor/事务；既有SQLite+provider fixture，新增未知合并反例 | `go run ./cmd/test-feedback -- ./internal/execution -run '^(TestWorkerStageAtomicCommitFailure|TestDevelopmentUnknownPublicationMergedDelivery|TestWorkerStageReconcileRejectsReplacementPR)$' -count=1` | 接线后及有兄弟交付接线变化的最终候选；结果/转移/下一输入无部分提交，未知合法合并可交付 | runner目录；DF94-A3 / commit、unknown-merge |
| [DF94-A4](../acceptance/development-flow-rules.md#自动验收计划) | 定向controlled app；Codex、Orca local/environment已有入口 | `go run ./cmd/test-feedback -- ./tests/integration -run '^(TestWorkerStageLocalRoundTrip|TestWorkerStageOrcaTerminalRoundTrip|TestWorkerStageOrcaEnvironmentRoundTrip)$' -count=1 -timeout=15m` | PR前；三个实际入口均通过，原会话/Attempt/每轮输入保持，无skip | runner目录；DF94-A4 / assembly |
| [DF94-A3](../acceptance/development-flow-rules.md#自动验收计划) / #96已合入时 | 定向actor/SQLite，本项未知合并与#96交付后清理反例 | `go run ./cmd/test-feedback -- ./internal/execution -run '^(TestDevelopmentUnknownPublicationMergedDelivery|TestWorkerRuntimeDeliveredCleanup)$' -count=1` | 最终候选基线含#96时必跑；未知合并仍交付，清理失败不重开Task或改写唯一交付；同候选已有相同证据可引用 | runner目录、基线commit/触发条件；DF94-A3 / runtime-combination |
| 必要回归 | 固定L1 | `go run ./cmd/test-feedback -level=L1` | PR前一次，固定范围/结构守卫通过 | L1目录；回归记录 |

不选择Full、L2/L3/live：本项不改平台/权限/worker资源和持久协议，纯规则、真实actor事务与双后端controlled接线覆盖范围。无新增人工窗口或原生支持声明。
