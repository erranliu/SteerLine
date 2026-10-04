# Plan：多候选本机与远端真机验收

任务 ID：39；任务名称：多候选本机与远端真机验收。唯一执行依据：[Spec](candidate-native-acceptance.spec.md)。硬前置：#38（传递包含 #37/#36），一个实现 PR 交付。

1. 核对实际交付和当前受支持目标/认证，固定隔离仓库、SQLite、构建与能力快照。远端权限尚未准入时标明阻塞；不增加远端权限实现，不用本机Orca、mock或skip代替远端通过。
2. 在 `tests/e2e/suites/candidates/` 编写带e2e tag的场景，经现有driver/native/controlled及证据适配层复用产品入口。场景不直接导入生产facade或平台实现，生产和外层 `internal/steerlineintegration` 不导入 `tests/e2e/internal`；遵守Go internal可见范围与现有测试边界。
3. 复用现有隔离环境与worker侧发布，不用host_service/旧DeferredFeedback或日志产品回退。分别做本机、远端双候选链及本机/远端混合；原profile、session、消息与PR独立。等待反馈重启和一个候选放弃后的兄弟继续/人工新增合入一组混合现场。
4. 人合并精确测试PR，核验唯一Delivery与后继包含原merge的base并真实交付；实际提交合并后放弃核验拒绝，命令回执与真实停止事实分开。
5. CN5及独立blocker/晚到副作用故障复用 #37受控证据，接线变化时只重跑受影响代表；不为故障全排列重复发真实模型请求。停止未支持或待核实如实保留。
6. 汇总CN1–CN8、阻塞项及前置证据交接 #42。只修本链集成缺陷，不新增业务、名额预留、自动补位或后端支持。

## 测试计划

本任务规划新增L3 `candidates` 组，三个live场景如下；包/测试/环境在实施时登记到 `tests/e2e/suites.json`，并更新E2E入口说明。当前未注册，不宣称下面的新场景命令已经可运行。不再从外层集成包创建直接复用E2E内部库的opt-in runner。

- `candidates.local` → `./tests/e2e/suites/candidates` 的 `TestCandidateLocal`，env=live。
- `candidates.remote` → 同包的 `TestCandidateRemote`，env=live；所需真实远端准入不满足就明确阻塞，不当作通过。
- `candidates.mixed` → 同包的 `TestCandidateMixed`，env=live；本机/远端精确原身份与配置。

必要环境扩展仅包括：在一个Project登记并冻结多套候选profile；driver按精确WorkflowRun/Execution提交新增、反馈、放弃及查询原请求；native/证据适配层逐候选核验来源、model/effort、权限和usage，不能沿用单一当前profile。沿现有live默认模型/effort与权限，差异可先选择不同执行目标/后端，不扩大模型或权限认证矩阵。

`STEERLINE_CANDIDATE_ACCEPTANCE_CONFIG` 规划为本次隔离现场manifest的输入，包含构建/项目、明确profile、远端目标及证据目录，认证只引用既有授权来源，不写正文；能力随 #39实现。远端缺权限/连接等前提在发模型和创建外部资源前报告，不通过补权限或偷偷换本机继续。各场景复用已有Run/Scenario/Project资源管理，保留原模型/工具回执和清理证据，不新增第二套状态机/codec。

| 级别/组或场景ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向Go：复核CN5与 #37 MC5/MC6 | 隔离SQLite、可控provider，无模型 | `go run ./cmd/test-feedback -out docs/validation/candidate-native-acceptance/conflict -- ./internal/steerlineintegration -run '^TestMC[56]$' -count=1` | 只有集成接线改变相应边界时；否则复用前置证据 | 前置 #37已实现入口实际命中；多合并/放弃PR合并/未知不覆盖Delivery | 原证据索引或conflict/原始状态与Go JSON |
| L3 candidates.local：CN1–CN3、CN6 | 隔离Windows本地Codex、真实Git/PR链，人工合并 | `go run ./cmd/test-feedback -level=L3 -suite=candidates -env=live '-scenario=candidates.local' -out docs/validation/candidate-native-acceptance/local` | 场景注册且原生环境验证后 | 双候选、原会话、worker实际发布、唯一交付与后继base有真实证据 | local/runner summary、身份、回执、PR及资源记录 |
| L3 candidates.remote：CN1、CN2、CN4、CN6 | 已认证受支持真实远端与隔离测试仓库 | `go run ./cmd/test-feedback -level=L3 -suite=candidates -env=live '-scenario=candidates.remote' -out docs/validation/candidate-native-acceptance/remote` | 远端实际准入及能力证据满足后 | 真远端双候选/故障/资源归属正确，停止unsupported如实记录 | remote/原工具结果、固定目标/配置及PR证据 |
| L3 candidates.mixed：CN7、CN8 | 同Task本机/远端两套profile，真实原会话 | `go run ./cmd/test-feedback -level=L3 -suite=candidates -env=live '-scenario=candidates.mixed' -out docs/validation/candidate-native-acceptance/mixed` | 双路径证据可复用且远端前提满足后 | 精确反馈/重启隔离，合并前放弃退出实验、不等停止便可新增，合并后拒绝；规范数据库查询与原身份保持 | mixed/精确请求、profile、重启/放弃/停止回执与runner证据 |
| L1固定最小入口 | 隔离STEERLINE_HOME，无本次live环境变量 | `go run ./cmd/test-feedback -level=L1 -out docs/validation/candidate-native-acceptance/l1` | 集成修复、自查完成后一次 | 原业务/守卫通过，不意外重跑live，e2e导入边界没有放宽 | l1/summary与原始JSON |

前置或包路径变化时更新实际命令到唯一所有者；新增/迁入测试必须非零命中。每次使用新的空证据目录，固定精确配置与合并操作者；环境缺失、零匹配、skip、原生证据/清理不足不能通过。不默认选择完整L2、Full、全部模型或live recovery；24小时和版本提升归 #42，五候选数量与允许偶尔超额复用 #37受控证据。本次只有Plan更新，未注册场景、改测试环境或执行原生测试。
