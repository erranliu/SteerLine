# Plan：多候选本机与远端真机验收

任务 ID：39；任务名称：多候选本机与远端真机验收。唯一执行依据：[Spec](candidate-native-acceptance.spec.md)。硬前置：#38 实现合入（传递包含 #37/#36），一个实现 PR 交付。用户允许 #38 CC5 单独待人工补验后继续本任务开发，不将该例外扩展为本任务的原生验收豁免。

1. 核对实际交付和当前受支持目标/认证，固定隔离仓库、SQLite、构建与能力快照。远端权限尚未准入时标明阻塞；不增加远端权限实现，不用本机Orca、mock或skip代替远端通过。
2. 在 `tests/e2e/suites/candidates/` 编写带e2e tag的场景，经现有driver/native/controlled及证据适配层复用产品入口。场景不直接导入生产facade或平台实现，生产和外层 `internal/steerlineintegration` 不导入 `tests/e2e/internal`；遵守Go internal可见范围与现有测试边界。
3. 复用现有隔离环境与worker侧发布，不用host_service/旧DeferredFeedback或日志产品回退。分别做本机、远端双候选链及本机/远端混合；原profile、session、消息与PR独立。等待反馈重启和一个候选放弃后的兄弟继续/人工新增合入一组混合现场。
4. 人合并精确测试PR，核验唯一Delivery与后继包含原merge的base并真实交付；实际提交合并后放弃核验拒绝，命令回执与真实停止事实分开。
5. CN5及独立blocker/晚到副作用故障复用 #37受控证据，接线变化时只重跑受影响代表；不为故障全排列重复发真实模型请求。停止未支持或待核实如实保留。
6. 汇总CN1–CN8、阻塞项及前置证据交接 #42。只修本链集成缺陷，不新增业务、名额预留、自动补位或后端支持。

本次真机发现 Orca 生命周期未连接既有 codex-session-launch/history binding，修复唯一路径为 `internal/steerline/orcaworker/lifecycle.go`：原 agent 终端使用既有无模型 bootstrap 写原 session binding，再以冻结权限/模型 resume；规范 collector 只读原生 history 并提交数据库。请求在原资源 Operation 后、创建终端前固定，重对账校验同一请求内容；不扫用户日志或制造正文。该生产接线变更需定向 Orca/Codex/候选代表与当前构建 local 重验，旧 local-02 不可复用为通过。

## 并行与构建固定

不增加 #73–#82 硬前置。与解耦线并行前固定本任务 commit/产品与 helper 构建、隔离配置/原生资源及场景清单；现场运行中不替换。接收各迁移任务的路径/身份/接线影响清单，按 Spec 规则复用未变证据，只定向重验受影响 CN 场景。#77/#78/#80 的接线变化不能直接沿用旧构建的通过结论。

TestMC5/6 当前在 `internal/steerline/candidate_workers_test.go`，使用下表主包命令；#76 将跨模块 MC 断言改为真实 app 外层接线，#81 再归 `tests/integration`，届时同次更新本 Plan 到唯一入口。E2E 包/API 路径变化只改 driver/native/evidence 的实际消费者，不从场景直接依赖业务/adapter 实现。

## 测试计划

本任务注册 L3 `candidates` 组三个 live 场景，唯一入口为 `tests/e2e/suites.json` → `tests/e2e/suites/candidates/candidates_test.go` → `tests/e2e/internal/testenv/candidates.go`。driver/native/evidence 分别拥有精确控制、manifest 准入和逐执行原生证据；外层集成包不直接复用 E2E internal。

- `candidates.local` → `./tests/e2e/suites/candidates` 的 `TestCandidateLocal`，env=live。
- `candidates.remote` → 同包的 `TestCandidateRemote`，env=live；所需真实远端准入不满足就明确阻塞，不当作通过。
- `candidates.mixed` → 同包的 `TestCandidateMixed`，env=live；本机/远端精确原身份与配置。

必要环境扩展仅包括：在一个Project登记并冻结多套候选profile；driver按精确WorkflowRun/Execution提交新增、反馈、放弃及查询原请求；native/证据适配层逐候选核验来源、model/effort、权限和usage，不能沿用单一当前profile。沿现有live默认模型/effort与权限，差异可先选择不同执行目标/后端，不扩大模型或权限认证矩阵。

`STEERLINE_CANDIDATE_ACCEPTANCE_CONFIG` 是本次隔离现场 manifest 输入，格式与重跑方法见 [证据入口](../validation/candidate-native-acceptance/README.md)。manifest 固定源码 commit、授权测试仓库、profile ID/chain、精确远端配置与场景组合；runner 固定产品/helper 摘要、out、scratch、认证来源并通过 Run 传入，Project 在该 scratch 下创建独立 config/home/SQLite/资源，规范 snapshot 保存实际身份。认证只引用来源，不写正文。远端先调用当前产品 CheckPolicy；缺目标权限回执在领取、模型和外部资源前写 blocker 并失败，不能以 unsupported/skip 通过。复用 Run/Scenario/Project 管理与原 codec，不新增状态机。

本机现场选择本地 Codex 与本机 Orca 两份 profile，同 Task 两个原生业务轮次须真实重叠。初轮各自结束并等待原 Decision；重启后核对原 session/turn/usage，再提交包含选项与区分标记的文字，丢失客户端响应时重读同一反馈 command，worker 自己提交/推送/创建真实 GitHub PR。操作者使用精确 head 合并，核验唯一 Delivery、真实合并后放弃拒绝及后继包含 merge 的 base 与真实 PR 交付。CN5 和故障/晚到边界未改变生产接线，复用 #37 receipt-final-02、flow-abandon-01 的受控证据；不称其为本次真远端通过。

远端产品准入尚未支持，因此远端/混合原生链不能执行；本次注册可复现 blocker 与共用精确控制/证据能力。真远端断连、混合合并前放弃/无需停止新增等现场增量仍待外部准入满足后实现/实测，不以本机结果豁免 CN4/CN7/CN8。任务保持活动 Spec/Plan，PR 保持 draft。

| 级别/组或场景ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向 Go：原生 session 接线/当前候选边界 | 隔离 native fixture/SQLite/provider，无模型 | `go run ./cmd/test-feedback -out docs/validation/candidate-native-acceptance/native-wiring -- ./internal/steerline/orcaworker ./internal/steerline/codexworker ./internal/steerline '-run=TestCandidateSessionLaunch|TestMC[56]|TestNativeLifecycle|TestExecution|TestConversation|TestCodex|TestManaged|TestResume' -count=1` | 修复 history 接线之后、local 重验之前 | 各包非零命中；原 launch intent/权限及会话绑定保持，CN5/放弃守卫无回退 | native-wiring/summary、原始 Go JSON |
| 定向 Go：manifest/身份/runner/边界 | 无模型、本机隔离临时目录 | `go run ./cmd/test-feedback -out docs/validation/candidate-native-acceptance/adapters-final -- ./cmd/test-feedback ./tests/e2e/internal/driver ./tests/e2e/internal/native ./tests/e2e/internal/evidence ./internal/testboundary -count=1` | 场景登记和适配完成后 | 非零命中；远端先拒绝、混合不换本机、乱序/重复/跨候选证据拒绝、生产导入边界保持 | adapters-final/summary、Go JSON |
| 定向Go：复核CN5与 #37 MC5/MC6 | 隔离SQLite、可控provider，无模型 | `go run ./cmd/test-feedback -out docs/validation/candidate-native-acceptance/conflict -- ./internal/steerline -run '^TestMC[56]$' -count=1` | 只有集成接线改变相应边界时；否则复用前置证据 | 前置 #37已实现入口实际命中；多合并/放弃PR合并/未知不覆盖Delivery | 原证据索引或conflict/原始状态与Go JSON |
| L3 candidates.local：CN1–CN3、CN6 | 隔离Windows本地Codex、真实Git/PR链，人工合并 | `go run ./cmd/test-feedback -level=L3 -suite=candidates -env=live '-scenario=candidates.local' -out docs/validation/candidate-native-acceptance/local` | 场景注册且原生环境验证后 | 双候选、原会话、worker实际发布、唯一交付与后继base有真实证据 | local/runner summary、身份、回执、PR及资源记录 |
| L3 candidates.remote：CN1、CN2、CN4、CN6 | 已认证受支持真实远端与隔离测试仓库 | `go run ./cmd/test-feedback -level=L3 -suite=candidates -env=live '-scenario=candidates.remote' -out docs/validation/candidate-native-acceptance/remote` | 远端实际准入及能力证据满足后 | 真远端双候选/故障/资源归属正确，停止unsupported如实记录 | remote/原工具结果、固定目标/配置及PR证据 |
| L3 candidates.mixed：CN7、CN8 | 同Task本机/远端两套profile，真实原会话 | `go run ./cmd/test-feedback -level=L3 -suite=candidates -env=live '-scenario=candidates.mixed' -out docs/validation/candidate-native-acceptance/mixed` | 双路径证据可复用且远端前提满足后 | 精确反馈/重启隔离，合并前放弃退出实验、不等停止便可新增，合并后拒绝；规范数据库查询与原身份保持 | mixed/精确请求、profile、重启/放弃/停止回执与runner证据 |
| L1固定最小入口 | 隔离STEERLINE_HOME，无本次live环境变量 | `go run ./cmd/test-feedback -level=L1 -out docs/validation/candidate-native-acceptance/l1` | 集成修复、自查完成后一次 | 原业务/守卫通过，不意外重跑live，e2e导入边界没有放宽 | l1/summary与原始JSON |

前置或包路径变化时更新实际命令到唯一所有者；新增/迁入测试必须非零命中。每次使用新的空证据目录，固定精确配置与合并操作者；环境缺失、零匹配、skip、原生证据/清理不足不能通过。不默认选择完整L2、Full、全部模型或live recovery；24小时和版本提升归 #42，五候选数量与允许偶尔超额复用 #37受控证据。失败重验采用 local-02 等全新目录，最终实际命令与逐项结果在证据入口登记；文档/场景登记不代表整体通过。
