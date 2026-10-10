# Plan：后端组装与最终回复状态收敛

唯一依据：[Spec](bounded-structure-refactoring.spec.md)。无编号普通开发计划，不进入 roadmap、不创建执行聊天、不派发任务。2026-10-10 用户授权本聊天实施及质量审计后提 PR，交付一个实现 PR。首次提 PR 集成基线 `8e292172d84c902e93cedb9870a658e90a9db0c4`，冲突解决合入 `4786934a2ab6a9be7e38d520c8efb2a7f65fedc0`；实时验收归[专属记录](../acceptance/bounded-structure-refactoring.md)。

## 实施步骤

1. **固定行为。** 枚举 app 两项 registration/codec 与 late binding，记录 v4、supervised local/environment 路由、旧 host/codec/ref 拒绝、HostManagedPermissions/LocalWorkspace/ServiceStopBehavior 和 policy resource 接线；核对 console 目标切换、final/body/preview、pin/分页/正文、报告阶段查询过滤、步骤展示与 Ctrl+N。沿原测试判断行为，不复制旧生产实现作 oracle。
2. **整理组装。** 在 app 同包建立 local Codex、Orca supervised 两个后端归属单元与普通私有函数，迁入注册/codec、特有配置/preflight/factory、localExecutionBackend 和各自 policy resource 辅助。中央仅组合/绑定；辅助按真实消费者归属，supervised 双目标仍共用一个实现。无新接口/builder/resolver/utilities；清理原定义，签名、keys/version、命令和生命周期策略不变，不重新接入退役 terminal。
3. **收拢状态。** console 同包的命名组件收拢 final/body/preview 字段及重置/读取/失效/受理，提供少量完整操作，异步只捕获固定值/窄查询端口。删除宿主旧字段和重复规则，不新增字段级转发 API。宿主继续导航及 detailScroll/recovery 清理，组件不修改反馈/恢复状态。
4. **薄接线与缺口测试。** 调整 final 键位/渲染/refresh/update，旧测试只适配状态位置、保留行为断言。新增 TestBuiltinComposition* 验证目录独立、原 keys/能力/停服字段、双目标的配置/工厂绑定及构造零 probe/start；沿用 TestSupervisedTargetsShareRegistrationAndRetiredLocalIsDisconnected 的旧入口拒绝断言。新增 TestFinalReader* 以小查询 stub/独立组件验证重置、乱序/历史范围、gap/预览/正文；用最小 Model 查询 stub 补报告阶段选择原 implement Turn、无匹配 StageResult 时沿 CurrentTurn、完整历史不过滤的接线缺口，归 TestFinalStageQuery*。宿主保留步骤事实派生、workerStagePresentation、nativeSessionOpening/原会话打开接线；不扩大组件端口。已有覆盖不复制，不新建共同模拟框架。
5. **交付收口。** 按下表验证并登记同一真实候选 SHA/dirty、枚举/命令/结果/临时证据。更新 current-code-architecture.md、parallel-development.md 受影响 app/console 行。此 PR 自行收口，清理旧规划导航、不恢复编号执行表。自查与正常独立复审核对旧状态/逻辑删除及无不必要层；实际实现、提交/PR 按后续用户授权处理。

两项可分别实施、在一个 PR 交付，不维护跨任务公共接线所有权。本次重构不改公共 DTO/backend/schema/runner；集成主干的既有契约变化按真实来源保留。消费已合入 #84 的步骤字段与 #45 的客户端接线，不暂停其他执行、不新增依赖或提前建兼容路径，不代改正式清单状态。

## 测试计划

覆盖全部 [BS-A1–BS-A5](../acceptance/bounded-structure-refactoring.md)。Windows/amd64，Go/Git、隔离 home/clone/SQLite、现有 app/integration executable fixture，无模型 opt-in。每次 runner 使用独立临时 evidence；并发启动多个 runner 时，显式加 `-out` 指向各自不同的新空临时目录，避免系统时钟粒度导致默认目录重名。执行前枚举新增入口，必须非零匹配、不能靠 skip。

| 验收项 / 级别、组或场景 | 命令 | 时点 / 通过条件 | 证据位置 |
| --- | --- | --- | --- |
| BS-A1、BS-A2 / app owner，新 TestBuiltinComposition* 及原冻结/能力回归 | `go run ./cmd/test-feedback -- ./internal/app -run '^(TestBuiltinComposition.*\|TestSupervisedTargetsShareRegistrationAndRetiredLocalIsDisconnected\|TestConfigLoadsLocalAndRemoteOrcaProfilesWithoutProbingTargets\|TestProfileSelectionDigestMatchesServicePreflight\|TestNativeOrcaAgentSelectionUsesFrozenProfile\|TestOrcaNativeProfileKeepsFrozenAuthority\|TestOrcaSelectedTargetMatchesRestoredLifecycle\|TestExecutionBackendCommonCapabilityContract\|TestOrcaPreflightDoesNotProbeAgentExecutable\|TestRemoteOrcaAgentIsOpaqueAndHasNoWorkerCodec\|TestPS2FixedWindowsProfilesFreezeAndRestore\|TestProjectPolicyFrozenSnapshotRejectsDamageAndPreservesAuthority\|TestNativeSessionUsesFrozenProfileAndExactCandidate)$' -count=1` | 后端整理后；原 keys/codec/权限/停服字段保持、supervised 双目标绑定和旧入口拒绝、实例不串线、构造 zero probe/start；原 profile/身份断言实际通过 | summary/JSONL；专属记录 BS-A1/A2 |
| BS-A3、BS-A4 / console owner，新 TestFinalReader*、TestFinalStageQuery* 与原交互 | `go run ./cmd/test-feedback -- ./internal/console -count=1` | 组件/接线完成后；TestFinalReader* 不构造 Model/完整客户端；Model 接线用例验证报告/历史过滤，原隔离/正文/预览/焦点/草稿及 TestWorkerStagePresentation、TestNativeSessionShortcutOpensSelectedCandidateAndReportsFailure 通过 | summary/JSONL；BS-A3/A4 |
| BS-A5 / integration，既有 TestWorkerStageLocalRoundTrip | `go run ./cmd/test-feedback -- ./tests/integration -run '^TestWorkerStageLocalRoundTrip$' -count=1` | owner 通过后；真实 app/SQLite/controlled local app-server implement/report/publish 三轮原意图/同会话/Attempt/来源和重启不重投保持，不冒充 Orca 真机或双目标端到端覆盖 | summary/JSONL；BS-A5 |
| BS-A5 / L1 组合 | `go run ./cmd/test-feedback -level=L1` | 完整候选后；passed/complete，规则/领取/存储/依赖守卫保持。同 SHA 的 L1 若实际执行 console 全包含新增入口，可替代上述 console 命令，核对枚举再登记，不重复跑 | summary/实际枚举；BS-A5，映射 BS-A3/A4 |
| 文档静态 | `python scripts/check_doc_links.py`；`python scripts/roadmap_task_id.py check`；`git diff --check` | 文档完成后；导航/历史身份一致、差异无空白错误；不占号，不计为产品验收 | 临时输出/本轮摘要；验收维护记录 |

步骤 4 负责补齐拟新增 TestBuiltinComposition*、TestFinalReader*、TestFinalStageQuery* 并枚举，不能零匹配通过。已有 app/integration 名称按新基线核对；部分测试仍用 NativeOrca/LocalAndRemote 等历史名称，实际断言已改为 supervised，不能据名称声称旧路径仍支持。BS-A2 新 app 用例必须明确断言两项 ServiceStopBehavior/HostManagedPermissions/LocalWorkspace、supervised 双目标使用同一 factory/lifecycle 和正确冻结 target/config；原 profile 测试不能单独证明组装配置等价。组件与 Model 接线测试分别补本次缺口，保留其他已有入口。

不选 Full/L2/L3/live/native：仅整理现有包内部，没有执行、协议、权限或恢复行为修改；owner、一次当前三轮真实 app controlled 接线和 L1 足够。不再默认叠加旧 TM04 双轮或 Orca 全链路场景；本次未修改 supervised adapter，双目标通过 app 归属测试覆盖组装边界，不宣称重新验过原生双目标交付。实际风险需要额外验证时解释并选最小场景，不为整理重跑停机对账双真实路径。方法见[测试指南](../testing.md)和[E2E 约定](../../tests/e2e/README.md)。

无额外人工项/窗口。Plan 不维护实时结果，具体验收与证据填专属记录；正常 PR 复审另外检查结构收敛。普通开发不修改 execution-map、不新增 Task/DAG。
