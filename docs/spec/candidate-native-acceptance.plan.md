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

只保留 L3 `candidates.local`（规划包 `tests/e2e/suites/candidates`、测试 `TestCandidateLocal`、env=live）；执行聊天按实际实现更新包/test 名、`tests/e2e/suites.json` 和 E2E README。删除本任务的 `candidates.remote` 与 `candidates.mixed` 远端场景登记及专用门槛，不把它们列作 skip/blocked，也不强制保留无人使用的远端 fixture。保留已有通用生产/适配层的远端准入规则。

环境扩展只含一个 Project 的两套本机 profile、精确 WorkflowRun/Execution 控制、逐候选来源/权限/usage 证据。`STEERLINE_CANDIDATE_ACCEPTANCE_CONFIG` 仅表达本机隔离构建、项目、profile 和证据目录；不要求远端目标或远端权限回执。具体 schema 与调用入口按实际实现记录，当前格式不做旧远端 manifest 兼容/回退。沿现有 live 模型/effort 与权限，通过本地 Codex/本机 Orca 后端差异比较方案，不另扩模型/权限矩阵。

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向 Go：CN5 / MC5、MC6 | 隔离 SQLite、可控 provider，无模型 | `go run ./cmd/test-feedback -out docs/validation/candidate-native-acceptance/conflict -- ./internal/steerline -run '^TestMC[56]$' -count=1` | 相应交付/冲突接线有变化时；否则复用原证据 | 实际非零命中；多合并/放弃 PR 合并/未知不覆盖 Delivery | 原证据索引或 conflict/报告与原始 JSON |
| 定向 Go：候选适配/manifest/证据 | 本机无模型的现有 E2E 适配层 | 按实际改动选择 `go test ./tests/e2e/internal/driver ./tests/e2e/internal/native ./tests/e2e/internal/evidence -count=1` 的必要包/测试，实施时登记精确命令 | 精确候选接口与证据接通后 | profile/Workflow/Execution/session/请求不串线；本机选择与旧远端输入拒绝符合当前 schema；选择包非零命中 | adapters/报告与原始输出 |
| L3 candidates.local：CN1–CN4、CN6–CN8 | 隔离 Windows，本地 Codex + 本机 Orca、真实模型/Git/PR | `go run ./cmd/test-feedback -level=L3 -suite=candidates -env=live '-scenario=candidates.local' -out docs/validation/candidate-native-acceptance/local` | 场景登记、两种本机能力和固定构建核对后 | 两候选真实重叠、独立反馈/规范正文/正数 usage、唯一交付/后继 base、重启/一例故障与放弃边界有完整原生证据，清理正确 | local/summary、规范状态/回执、原生身份、PR 和资源记录 |
| L1 固定最小入口 | 隔离 STEERLINE_HOME，无 live 环境变量 | `go run ./cmd/test-feedback -level=L1 -out docs/validation/candidate-native-acceptance/l1` | 自查与集成修复完成后一次 | 原业务/守卫通过，不意外重跑 live，E2E 导入边界未放宽 | l1/summary 与原始 JSON |

每次使用新的空证据目录，固定实际合并操作者；零匹配、skip、实际本机环境/原生证据或清理不足不能记为通过。仅尚未执行的人工操作验收按当前规则登记 roadmap 后推进，实际发现的功能/正文失败仍须修复。不默认选择完整 L2、Full、所有模型或 live recovery；24 小时和版本提升归 #42，五候选数量和竞态允许超额复用既有受控证据。本文收缩验收范围，不代表本机场景已通过。
