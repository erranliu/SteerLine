# Plan：多候选本机验收

任务 ID：39；任务名称：多候选本机验收。唯一执行依据：[Spec](candidate-native-acceptance.spec.md)。硬前置：#38 实现合入（传递包含 #37/#36），一个实现 PR 交付。#38 CC5 已由用户人工确认通过，不豁免本任务的本机 CN1–CN8 条件。

1. 核对本地 Codex 与本机 Orca 的实际能力/认证，固定隔离仓库、SQLite、产品/helper 构建、两套本机 profile 及来源版本。远端已从本任务范围移除，不增加远端准入、权限实现或远端补验待办。
2. 在 `tests/e2e/suites/candidates/` 接一个 L3 live 本机场景，复用现有 driver/native/controlled/evidence；两种本机后端在同 Task 内真实重叠工作，逐候选冻结配置/权限、独立会话/worktree/分支/PR。场景不得直接依赖业务 facade/平台实现，也不从生产或外层集成包导入 E2E internal。
3. 将两份原会话反馈、等待反馈时中心重启、一例本机候选连接故障、合并前放弃及兄弟继续/人工新增合入同一现场。不同配置/后端不扩成全排列，不建第二套状态机或 codec。
4. 操作者合并精确测试 PR，核验唯一 Delivery、合并前后后继资格、后继包含原 merge 的 base 与实际 PR 交付；合并后实际提交放弃核验拒绝，原命令/Operation 回执与真实停止分开。
5. 正文、会话、raw 与活动均从规范数据库取证，逐候选核对真实 session/turn、model/effort、权限和正数 usage。已有真实正文/采集/接线失败必须诊断修复，不能用原始日志回填产品结果或改写为待人工补验。
6. CN5 和独立 blocker/晚到副作用的完整故障边界复用既有受控证据；仅接线改变时定向重验。汇总本机 CN1–CN8 与原资源清理证据交接 #42，远端不再计入本任务覆盖或阻塞。

## 并行与构建固定

不增加 #73–#82 硬前置。与解耦线并行前固定 commit、产品/helper 构建、隔离配置/home/原生资源及本机场景；运行中不替换。路径、身份或接线迁移按 Spec 的实际影响定向重验，未变证据可复用；#77/#78/#80 的变化不能直接沿用旧构建通过结论。

TestMC5/6 当前在 `internal/steerline/candidate_workers_test.go`；迁移任务交付后同次更新本 Plan 到唯一入口。E2E 接口/路径变化只改 driver/native/evidence 的实际消费者，不从场景直接依赖业务/adapter 实现。

## 测试计划

本次实际发现已有 open PR 的 pending feedback 被轮询提前恢复等待合并的接线问题；`internal/steerline/worker_pr.go` 在原 pending command/turn 存在时保留其等待反馈门槛。定向入口 `TestCandidateOpenPRPollPreservesOriginalPendingInput` 调用真实隔离 provider poll，核对原 Decision、pending command 与 PR 归属，不仅测试局部状态函数；并复核既有 `TestWorkerFeedbackPhaseCommitIsDurableAndRejectsStaleActor`。命令：`go run ./cmd/test-feedback -out docs/validation/candidate-native-acceptance/pending-pr-02 -- ./internal/steerline '-run=^TestCandidateOpenPRPollPreservesOriginalPendingInput$|^TestWorkerFeedbackPhaseCommitIsDurableAndRejectsStaleActor$' -count=1`。最终现场重验此增量，并经 driver 的原 public reader Query 采集原 Operations/receipts（常规产品 Workflow 视图只提供筛选业务记录）。

已实现的唯一入口：`tests/e2e/suites.json` → `tests/e2e/suites/candidates/candidates_test.go:TestCandidateLocal` → `tests/e2e/internal/testenv/candidates.go:CandidateAcceptance`。driver 的 `MainCandidate` 提交当前 Task/graph/run 版本、精确 Execution/Decision 和原 command ID，reader 读取规范 Conversation/完整 Body；native 负责本机 manifest/模型/usage，evidence 按精确 WorkflowRun/Execution 解码原 profile/resource/session。无生产或外层 E2E internal 依赖。

manifest schema 固定 version=1、source_commit（实际40位 HEAD）、repository=erranliu/SteerlineTest、两份本机 profiles（不同 ID、local-codex 与 orca-codex）、local profile 顺序；runner 的 Run 固定产品/helper SHA-256、out/scratch、AuthHome。本次 Project 在 scratch 创建独立 config/home/SQLite，实际配置、目标和原会话/回执见规范 snapshot。只有本机字段，不兼容旧远端输入。live 固定 gpt-6-luna/medium 与 danger-full-access/never，不扩认证矩阵。

本机真机已定位 Orca 生命周期缺少既有 codex-session-launch/history binding 的接线。Orca 正文接线修复位于 `internal/steerline/orcaworker/lifecycle.go`：在原资源 Operation 后、外部创建终端前固定 launch 请求，原终端无模型 bootstrap 写原生 session binding，再按冻结权限 resume，规范 collector 提交原 history 正文。重复对账只接受原意图，不从 raw 回填。`internal/steerline/codexworker/history.go` 的 resume 无新提示时不注入空提示。此接线改变须当前构建 local 重验，local-02 失败不能当通过。

native 的 app-server usage 适配要求精确新 turn/started；仅排除其前同一 thread 的 resume 旧 usage 快照，之后外会话/晚到外轮次仍拒绝。未知 usage 错误立即记录失败；清理读使用独立 deadline。候选多轮现场独立采用20分钟上限（其他 live 10分钟），后继使用冻结 local-codex profile。local-04 已实际用12分钟完成双候选/重启/反馈/晚到/后继，新增同场故障、兄弟额外反馈和替代候选后采用这个明确上限，不靠缩减断言压缩时限。

CN4/CN8 的故障路径由 `native.DisconnectCandidate` 对隔离 local app-server 的同一 Windows handle 校验 PID/creation/executable 和原 session envelope 后终止；真实连接丢失与业务 stop Operation 区分。通过产品提交原 Decision 的 fault-probe，确认原 Execution/session 不替换、turn 不增加；原 Orca 兄弟继续反馈/PR，再按精确 Task/run 当前版本提交放弃。收到实验退出回执立即人工开始新的 local 候选，不等待停止事实，核对原 base/历史/PR保留和新候选正数 usage。晚到验证改用这个新增候选，原故障样本仍保留逐执行证据。

本次实际定向命令：`go run ./cmd/test-feedback -out docs/validation/candidate-native-acceptance/native-wiring-01 -- ./internal/steerline/orcaworker ./internal/steerline/codexworker ./internal/steerline '-run=TestCandidateSessionLaunch|TestMC[56]|TestNativeLifecycle|TestExecution|TestConversation|TestCodex|TestManaged|TestResume' -count=1`，27测试事件通过、无 fail/skip；覆盖原 session launch 意图/权限、CN5/MC5–MC6 和规范采集。适配器最终命令采用下表包、runner/testboundary 与必要场景；每次新空目录，最终精确命令/构建及结果见 [证据入口](../validation/candidate-native-acceptance/README.md)。

只保留 L3 `candidates.local`（规划包 `tests/e2e/suites/candidates`、测试 `TestCandidateLocal`、env=live）；执行聊天按实际实现更新包/test 名、`tests/e2e/suites.json` 和 E2E README。删除本任务的 `candidates.remote` 与 `candidates.mixed` 远端场景登记及专用门槛，不把它们列作 skip/blocked，也不强制保留无人使用的远端 fixture。保留已有通用生产/适配层的远端准入规则。

环境扩展只含一个 Project 的两套本机 profile、精确 WorkflowRun/Execution 控制、逐候选来源/权限/usage 证据。`STEERLINE_CANDIDATE_ACCEPTANCE_CONFIG` 仅表达本机隔离构建、项目、profile 和证据目录；不要求远端目标或远端权限回执。具体 schema 与调用入口按实际实现记录，当前格式不做旧远端 manifest 兼容/回退。沿现有 live 模型/effort 与权限，通过本地 Codex/本机 Orca 后端差异比较方案，不另扩模型/权限矩阵。

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向 Go：CN5 / MC5、MC6 | 隔离 SQLite、可控 provider，无模型 | `go run ./cmd/test-feedback -out docs/validation/candidate-native-acceptance/conflict -- ./internal/steerline -run '^TestMC[56]$' -count=1` | 相应交付/冲突接线有变化时；否则复用原证据 | 实际非零命中；多合并/放弃 PR 合并/未知不覆盖 Delivery | 原证据索引或 conflict/报告与原始 JSON |
| 定向 Go：候选适配/manifest/证据 | 本机无模型的现有 E2E 适配层 | 按实际改动选择 `go test ./tests/e2e/internal/driver ./tests/e2e/internal/native ./tests/e2e/internal/evidence -count=1` 的必要包/测试，实施时登记精确命令 | 精确候选接口与证据接通后 | profile/Workflow/Execution/session/请求不串线；本机选择与旧远端输入拒绝符合当前 schema；选择包非零命中 | adapters/报告与原始输出 |
| L3 candidates.local：CN1–CN4、CN6–CN8 | 隔离 Windows，本地 Codex + 本机 Orca、真实模型/Git/PR | `go run ./cmd/test-feedback -level=L3 -suite=candidates -env=live '-scenario=candidates.local' -out docs/validation/candidate-native-acceptance/local` | 场景登记、两种本机能力和固定构建核对后 | 两候选真实重叠、独立反馈/规范正文/正数 usage、唯一交付/后继 base、重启/一例故障与放弃边界有完整原生证据，清理正确 | local/summary、规范状态/回执、原生身份、PR 和资源记录 |
| L1 固定最小入口 | 隔离 STEERLINE_HOME，无 live 环境变量 | `go run ./cmd/test-feedback -level=L1 -out docs/validation/candidate-native-acceptance/l1` | 自查与集成修复完成后一次 | 原业务/守卫通过，不意外重跑 live，E2E 导入边界未放宽 | l1/summary 与原始 JSON |

每次使用新的空证据目录，固定实际合并操作者；零匹配、skip、实际本机环境/原生证据或清理不足不能记为通过。仅尚未执行的人工操作验收按当前规则登记 roadmap 后推进，实际发现的功能/正文失败仍须修复。不默认选择完整 L2、Full、所有模型或 live recovery；24 小时和版本提升归 #42，五候选数量和竞态允许超额复用既有受控证据。本机验收及协调复审的实际构建、命令和结果见[证据入口](../validation/candidate-native-acceptance/README.md)。

## 协调最终验证

集成 #74/#75 后固定 `919a4ce755eb6d56759efa479608b75bef34749c`：运行同一 `candidates.local` 场景、上表 L1，以及受影响 worker/history/候选适配/边界的定向测试；通过条件为原 CN1–CN8、正数 usage、规范正文校验和、真实 PR/后继交付、无 fail/skip 且清理完成。实际命令及证据位置见 [coordinator-final.json](../validation/candidate-native-acceptance/coordinator-final.json) 和证据入口；不跑 Full 或无关平台矩阵。仅整理文档后的最终 head 与该构建的 Go/场景源码相同。
