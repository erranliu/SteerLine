# Plan：多候选本机与远端真机验收

任务 ID：39；任务名称：多候选本机与远端真机验收。唯一执行依据：[Spec](candidate-native-acceptance.spec.md)。硬前置：#38（传递包含 #37/#36），一个实现 PR 交付。

1. 核对实际交付和受支持目标认证，固定隔离仓库、SQLite、构建与能力快照；所需原生路径未获支持时记录阻塞，不能用mock或本机冒充远端。
2. 复用原生项目、来源与provider环境，分别做本机/受支持远端双候选链；一组混合配置覆盖精确反馈、中心重启和一个候选放弃后的兄弟继续/人工新增。
3. 人合并精确测试PR，核验唯一Delivery与后继包含原merge的base并真实交付；实际提交合并后放弃核验拒绝。
4. 多合并/PR未知使用已有受控故障fixture，真实现场只验证能力和原身份，不为故障矩阵重复发模型请求。停止未支持或待核实如实保留。
5. 汇总CN1–CN8与前置证据，交接 #42；不新增业务、严格名额/自动补位或其他后端。

## 测试计划

实施时在当前外层集成包新增最小opt-in入口TestCandidateNativeLocal、TestCandidateNativeRemote、TestCandidateNativeMixed及受控TestCandidateNativeConflict。复用已有E2E native/driver与独立原生夹具，不建设第二套协议codec。由环境变量STEERLINE_CANDIDATE_ACCEPTANCE_CONFIG指向本次隔离现场manifest，包含构建、项目/目标、冻结profile与证据目录，认证仅环境引用，不写正文。该环境能力随 #39规划实现；未实现/未准备时不宣称以下命令已可运行。

| 级别/组或场景ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向Go：CN5冲突代表 | 真实SQLite、可控provider，无模型 | `go run ./cmd/test-feedback -out docs/validation/candidate-native-acceptance/conflict -- ./internal/steerlineintegration -run '^TestCandidateNativeConflict$' -count=1` | 前置与fixture接通后 | 多合并/已放弃PR合并/未知不覆盖Delivery；原代表断言实际命中 | conflict/原始状态与Go JSON |
| 独立原生：CN1、CN2、CN3、CN6 | 隔离Windows本机Codex及测试仓库；人工合并PR | `go run ./cmd/test-feedback -out docs/validation/candidate-native-acceptance/local -- ./internal/steerlineintegration -run '^TestCandidateNativeLocal$' -count=1` | 设置本次manifest并核对权限后 | 两候选原session/worktree/profile/消息/PR、唯一交付与后继base均有真实证据 | local/构建、身份、回执、PR及资源记录 |
| 独立原生：CN1、CN2、CN4、CN6 | 当前已认证受支持远端与隔离测试仓库 | `go run ./cmd/test-feedback -out docs/validation/candidate-native-acceptance/remote -- ./internal/steerlineintegration -run '^TestCandidateNativeRemote$' -count=1` | 所需原生能力前置实际满足后 | 远端双候选及故障/资源归属正确，不把unsupported取消写作通过停止 | remote/原工具结果、固定目标及PR证据 |
| 独立原生：CN7、CN8 | 同Task本机/受支持远端两套配置，真实原会话 | `go run ./cmd/test-feedback -out docs/validation/candidate-native-acceptance/mixed -- ./internal/steerlineintegration -run '^TestCandidateNativeMixed$' -count=1` | 两条来源与前置能力可复用后 | 混合配置/原反馈/重启隔离；合并前放弃退出实验、合并后拒绝；放弃与停止区别如实记录 | mixed/精确请求、profile、重启与放弃/停止回执 |
| L1固定最小入口 | 隔离STEERLINE_HOME，先清除本次原生opt-in变量 | `go run ./cmd/test-feedback -level=L1 -out docs/validation/candidate-native-acceptance/l1` | 集成修复、自查完成后一次 | 原业务与守卫代表通过，不意外重跑live | l1/summary与原始JSON |

每次使用新的空证据目录，固定精确配置和合并操作者；env缺失、零匹配、skip或原生证据不足不是通过。不增加新的L2/L3注册组：原生检查属于专门现场，复用其前置/环境能力并在实现时登记真实入口。Full、24小时和版本提升组合不在本任务重跑，后两者归 #42；五候选数量与并发偶尔超额的规则复用 #37证据。
