# 工作流核验收敛实施 Plan

本次为用户直接授权的普通开发，不纳入 roadmap。核验清单、取舍和验收项见[专属记录](../testing/acceptance/workflow-verification.md)。

## 实施步骤

1. 只读检查安装环境 101、105 的原始事件和正文，按 Task ID 区分服务日志。
2. Orca 完成采集只消费原终端绑定的 native main-agent start/end 与完整 lastAssistantMessage；删除屏幕正文比对、Markdown/换行归一化、可见输入/完成标记、一秒稳定等待和完成时 draft 检查。
3. 就绪检查只查询原 main-agent 状态与 provider/pane 身份。删除 tui-idle、提交前屏幕 draft 检查、准备时屏幕可读性检查、失败时屏幕预览和闲置 screen/cursor API。原始状态暂未具备就绪事实或初始化仍 working 时有界等待；超时、waiting/blocked 不投递业务输入。恢复资格检查也仅使用原始状态，不读屏幕 draft。
4. 保留原请求、原配置、原资源、原轮次和完整正文核验。原始正文按字节保存和摘要，不再因渲染形式变化重写正文；触及公开 hook 的截断上限仍不可冒充完整。
5. 原查询失败或结束后正文晚到时，每 30 秒只读重查原来源；新原生状态提示允许及时查询。取消会导致查询永久停用的单次标志，禁止重发输入。
6. 共用阶段报告接受完整 JSON 代码围栏、无关附加字段和控制值两端空白；继续拒绝重复键（包括已知业务字段的大小写别名）、多个对象、关键字段错误或相互冲突。实施阶段继续接受普通文字。保持已有一次报告重格式化与 publish.report 路径。
7. 核对本地 Codex 直接使用 app-server 原始 thread/turn/final_answer 事件，保持该路径；PR、合并、TaskDelivery 和 DAG 的事实核验保持。
8. Orca 资源 codec 升为 4，并同步 E2E evidence 的 codec/原始 final source；真机恢复驱动离线等待改用绑定的原主代理结束事实。删除旧屏幕候选字段，不新增旧版读取/迁移；不替换安装目录程序、不修写安装数据库。

## 测试计划

所有自动项映射到[验收明细](../testing/acceptance/workflow-verification.md#验收明细)。环境均为现有隔离 Go 夹具，无真实模型、安装服务或外部 PR 操作；在实现后、交付前执行，使用 test-feedback 保存原始日志和 summary.json，在专属验收页登记目录与结果。

| 验收项 | 级别 / 场景 | 命令 | 通过条件 |
| --- | --- | --- | --- |
| WV-A1、A2、A3 | 定向 Go；TestOrcaNativeFinalIgnoresScreen、TestOrcaInputReadiness、TestPreparationAndSubmissionNeverReadScreen、TestOrcaNativeStateGate/Binding、TestOrcaObserverDeliversNativeFinalWithoutScreen、原输入拒绝/未知提交/恢复；完整两个小包 | `go run ./cmd/test-feedback -- ./internal/execution/adapters/orcatransport ./internal/execution/adapters/protocol/orca -count=1` | 101/105 对应正文立即可用；全生命周期不调用 terminal read/wait；仅原始身份、开始/结束和正文允许推进；未知输入不重发 |
| WV-A4 | 定向 Go；TestOrcaNativeFinalRetriesMissingBodyAndQueryFailure、TestTerminalNativeFinalRecoveryNeverReplaysInput | 同上 | 两次原查询失败/缺正文后仍可只读恢复；持久化原回复可恢复，无重复输入 |
| WV-A5 | 定向 Go；TestWorkerStagePresentationDoesNotBlockResult、TestWorkerStageStrictResult；Transitions、PublicationReadRetrySettlesOpenPR、PublicationReportReadRetrySettlesOpenPR、ReconcileRejectsReplacementPR、PublicationMismatchShowsActualBranch | `go run ./cmd/test-feedback -- ./internal/execution -run '^TestWorkerStage(StrictResult|PresentationDoesNotBlockResult|Transitions|PublicationReadRetrySettlesOpenPR|PublicationReportReadRetrySettlesOpenPR|ReconcileRejectsReplacementPR|PublicationMismatchShowsActualBranch)$' -count=1` | 围栏/附加字段被接受；歧义与关键错误拒绝；阶段转移和原 PR 绑定保持 |
| WV-A6 | 定向 Go；本地 Codex 原生结果、PR 身份/合并证据及依赖边界 | `go run ./cmd/test-feedback -- ./internal/execution/publication ./internal/execution/adapters/codex ./internal/testkit/boundary/... -count=1` | 原 thread/turn/final_answer、真实 PR 仓库/base/head 与合并证据核验未退化；无生产到测试的依赖 |
| WV-A8 | 定向 Go / E2E 编译；TestWorkingEvidenceRejectsOtherCommandOrResource、TestOrcaStagedContinuityUsesNativeFinalSource、TestOrcaOriginalEndRequiresBoundNativeFacts | `go run ./cmd/test-feedback -- ./tests/e2e/internal/evidence ./tests/e2e/internal/native -count=1`；`go test ./tests/e2e/internal/testenv -run '^$'` | 新 codec/source 可读、旧 codec/source 拒绝；原生离线结束绑定拒绝旧轮/其他 provider/pane；真机驱动编译通过（不代替执行） |
| WV-A7 | 静态源码/文档检查 | `python scripts/check_doc_links.py`、`git diff --check`；搜索生产 adapter 的屏幕 API/标记 | 无遗留 ReadScreen、tui-idle、终端正文解析核验；文档导航与差异格式通过 |

不跑 Full：修改集中于 Orca 小包和共用报告解码。暂不选 L2/L3 live：本次不变更外部 PR/交付操作，也不把安装环境当测试现场；两目标原生真机复验单列人工项 WV-M1，受控回归不冒充真机通过。后续执行真机复验须先用 `scripts/new-debug-release.ps1` 新建独立环境，禁止复用安装数据库。
