# Plan：本机 Orca 原生通用 worker 与权限委托

本聊天直接实现本机 Orca 接入调整，历史预留 ID 86 已退出 roadmap，保留编号不复用。依据 [Spec](orca-native-worker.spec.md)，结果归[专属记录](../acceptance/orca-native-worker.md)；当前用户指令是直接实现，不创建 roadmap 执行聊天、不阻塞 #84 或改写其固定文档。

## 实施步骤

1. 更新本机 Orca profile 和 app 目录注册，agent 作为配置值、model/effort 作为 Orca 通用偏好；移除本机 Orca 对 codex.worker-profile、固定 Codex 权限封套与版本探测的依赖。为变更格式设置明确版本，不增加旧格式转换。
2. 先以受控回执和隔离原生场景核实 Terminal worker 的首轮与同会话续轮能力，记录当前版本的原生 mode/资源/handle 含义；本轮不支持 structured 同会话接续，不为其猜测接口。将 Orca Terminal + YOLO 列为启动前环境准备；若组合启动后回执显示不支持类型，在所有清理动作前核对原 runtime/Run/Task/Dispatch/资源所有权；持久 stopped 允许原请求继续 release，不受旧 stop_unknown 回执冻结；保留原请求、已送达输入及首轮效果的已知/未知事实，阻断后续派发与业务推进，按原身份/原生资格停止和释放，不声称首轮零执行，不改全局设置或盲重派。首轮由一个原生请求拥有工作区/agent 启动，记录组合副作用及原请求以供对账；不先启动 agent 再重复创建。首轮传通用 agent/model/effort，effort 必须有 model，核对 requested/effective；续轮只选原 worker，不重传启动偏好。删除 SteerLine 自建 agent 启动、私有会话绑定及 TUI 解析。原生能力不足时记录具体平台前置，不增加专属分支或替代会话。
3. 改为 Orca 原生 worker-start/show/inbox 的输入、等待与结算事实；最终正文使用通用显式输出材料协议，核对原 Dispatch 结算后冻结于应用运行目录，释放终端后仍可查询；停止/释放/保留采用 worker-stop/release，原 Dispatch 结算资格、未知停止及释放回执纳入受控测试。移除本机专属 sandbox readiness 和文件 relay。使用显式 output material 加原 Dispatch 结算，冻结完整材料保障释放后历史；正常观察及重启恢复统一验证输出；冻结摘要和游标覆盖正文与 outcome，JSON预算覆盖最坏转义。坏/未知 Attempt 来源写局部 gap，健康历史继续。运行期从 Orca agentWait 保存宿主等待事实，缺字段保持未知，显式 null 才解除，不创建业务 Decision。不把 terminal tail 当 final。注册目录声明宿主管理权限，正式停止与恢复走原生命周期证据；保证当前持续 worker 的规范 final 采集；接口供后续 #84 消费，不提前实施分步流程，保持 Git/PR 真值核验。
4. 更新受影响配置示例、产品设计、测试 fixture 与隔离原生 driver。补齐通用 worker-start / 输出驱动和当前格式断言，不删减仍有效的身份/未知/恢复要求。
5. 执行下表最小验证及独立复审，登记具体构建、命令、结果、缺口及资源事实；只在 execution-map 记录撤回与不阻塞事实，不将本变更重新加入队列。工作区实现完成不等于 PR 已真实合并。本次不自动覆盖 C:/SteerlineRuntime，不迁移或改写其 102 冻结执行。

本变更拥有 Orca adapter 与其 app 配置/观察接线。本地 Codex 保持自身策略；#84 正在其独立工作区按固定契约执行，本聊天不修改该工作区或其实际状态，不增加等待边。最终集成时核对共同 LifecyclePort 和规范输出接口，不提前重定 #84。

## 测试计划

| 验收项 | 级别 / 场景 | 环境与命令 | 时点 / 通过条件 | 证据与结果 |
| --- | --- | --- | --- | --- |
| [ONW-A1](../acceptance/orca-native-worker.md) | L1 定向 `TestOrcaNativeAgentBoundary`、profile/config 回归 | Windows；新增上述边界测试后 `go run ./cmd/test-feedback -- ./internal/execution/adapters/orca ./internal/app -run 'TestOrcaNativeAgentBoundary|TestOrcaNativeProfile' -count=1`；本地策略另用 `go run ./cmd/test-feedback -- ./internal/execution/adapters/codex -run '^TestPS2NativeLaunchAndInputUseAutomaticReview$' -count=1` | 配置/注册完成后；无生产专属依赖、两个 agent 值同路径、旧格式拒绝；每个所选包/顶层测试实际命中，本地策略保持 | runner 独立 -out；专属记录 A1 |
| [ONW-A2](../acceptance/orca-native-worker.md) | L1 受影响 adapter/执行/集成；通用生命周期受控场景 | Windows；补齐 fixture 后 `go run ./cmd/test-feedback -level=L1`，并 `go run ./cmd/test-feedback -- ./internal/execution/adapters/orca -count=1`；停止恢复另用 `go test ./internal/execution -run 'Test(HostManagedStopRecovery|ExecutionStopIntent|PS3StopReceipt|RecoveryCannotSelectReplacementSession)' -count=1`，tagged local driver 编译 `go test -tags ps2native ./internal/app -run '^$'`；审计回归包括 adapter 的 `TestPartialWorkerRecoveryChecksOwnershipAndPersistentStop`、`TestOutputEvidenceCoversOutcomeAndJSONExpansion`、`TestOutputDiscoveryIsolatesDamagedSource`、`TestRecoveredDoneRequiresCompleteFrozenOutput`、`TestNativeStartTransportBudgetCoversReadiness`、`TestSettledWorkerStopReleasesOriginalResource`、`TestNativeHostWaitIsObservedWithoutBusinessQuestion`；核心另 `go test ./internal/execution -run 'Test(HostManagedStopRecovery|LifecycleHostWait|HostInputWait)' -count=1` 覆盖首丢弃/重启和等待桥接。原身份恢复定向 `go run ./cmd/test-feedback -- ./tests/integration -run '^TestCD67AdditionalImplementation$/^supported_terminal_recovery_and_original_identity$' -count=1` | 原生接线及输出完成后；每个所选用例实际运行、未知不重复、首轮单次创建、续轮原会话关联；不支持 structured 类型的拒绝/部分资源；worker-stop/release 的资格、拒绝/未知回执与资源归属成立；受影响旧用例按新契约替换而不虚报零匹配 | runner summary / 日志；专属记录 A2 |
| [ONW-A3](../acceptance/orca-native-worker.md) | 定向 live `TestOrcaNativeWorkerSmoke` | 独立无远端 Git 仓库、Orca Terminal + YOLO；设置 `STEERLINE_ORCA_NATIVE_REPOSITORY`、`STEERLINE_ORCA_NATIVE_EXECUTABLE`、`STEERLINE_ORCA_NATIVE_EVIDENCE` 后 `go test ./internal/execution/adapters/orca -run '^TestOrcaNativeWorkerSmoke$' -count=1 -v` | 实际接线完成后；真实工作区操作、至少两轮原会话、完整输出及原生结算、精确资源清理；环境缺失的 skip 不算通过 | 原始 receipts / 输出 / 测试日志；专属记录 A3 |

Live 驱动直接采用上述 opt-in Go 入口，不虚构 L3 注册场景；默认单元入口跳过该 live 测试。未设置隔离仓库时不得对产品仓库运行。原生账号/Orca宽松设置或 launcher readiness 不可用时记录真实阻塞，不改专属参数绕过。Full、全量 L3 delivery/recovery 不默认运行；只有受影响断言无法由上述范围覆盖时再补定向入口。

ONW-M1 在新构建部署后由用户集中确认安装入口及权限显示，窗口待安排；不阻塞本任务非人工验证和无关研发。原生自动失败、输出不可核实、清理未知不能改称人工后验收。
