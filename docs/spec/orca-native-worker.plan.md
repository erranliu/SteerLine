# Plan：本机 Orca 原生通用 worker 与权限委托

任务 ID：86；任务名称：本机 Orca 原生通用 worker 与权限委托。实施依据为 [Spec](orca-native-worker.spec.md)，自动项与实时结果归[专属记录](../acceptance/orca-native-worker.md)。开始实现前固定本 Spec/Plan 的实际 Git commit 与路径，执行来源为 execution-map。

## 实施步骤

1. 更新本机 Orca profile 和 app 目录注册，agent 作为配置值、model/effort 作为 Orca 通用偏好；移除本机 Orca 对 codex.worker-profile、固定 Codex 权限封套与版本探测的依赖。为变更格式设置明确版本，不增加旧格式转换。
2. 收口原生 Orca worktree/agent 启动与 worker-start。删除 SteerLine 自建 agent 启动、私有会话绑定及 TUI 解析；原会话继续输入由原 terminal / 原 Dispatch 结算资格控制。保留原外部 Operation 意图、请求 ID、部分效果与未知对账。
3. 改为 Orca 通用 worker-show/read/inbox 的输入、等待、输出、停止与结算事实；移除本机专属 sandbox readiness 和文件 relay。适配现有分步 worker 的 final / 结构化结果采集，保持 Git/PR 真值核验。
4. 更新受影响配置示例、产品设计、测试 fixture 与隔离原生 driver。补齐通用 worker-start / 输出驱动和当前格式断言，不删减仍有效的身份/未知/恢复要求。
5. 执行下表最小验证及独立复审，登记具体构建、命令、结果、缺口及资源事实；同步 execution-map。工作区实现完成不等于 PR 已真实合并。本次不自动覆盖 C:/SteerlineRuntime，不迁移或改写其 102 冻结执行。

本任务拥有 Orca adapter 与配置/观察共同接线；本地 Codex 所有者代码仅作确有必要的目录解耦。#45 消费本任务的实际接口，不并行重复修改同一接线。#84 的实现已在当前主干，采用既有分步流程，不重定它的固定需求。

## 测试计划

| 验收项 | 级别 / 场景 | 环境与命令 | 时点 / 通过条件 | 证据与结果 |
| --- | --- | --- | --- | --- |
| [ONW-A1](../acceptance/orca-native-worker.md) | L1 定向 `TestOrcaNativeAgentBoundary`、profile/config 回归 | Windows；新增上述边界测试后 `go run ./cmd/test-feedback -- ./internal/execution/adapters/orca ./internal/app -run 'TestOrcaNativeAgentBoundary|TestOrcaNativeProfile|TestPS2NativeLaunchAndInputUseAutomaticReview' -count=1`；本地 Codex 定向另在其包运行 | 配置/注册完成后；无生产专属依赖、两个 agent 值同路径、旧格式拒绝；本地策略保持 | runner 独立 -out；专属记录 A1 |
| [ONW-A2](../acceptance/orca-native-worker.md) | L1 受影响 adapter/执行/集成；通用生命周期受控场景 | Windows；补齐 fixture 后 `go run ./cmd/test-feedback -level=L1`，并 `go run ./cmd/test-feedback -- ./internal/execution/adapters/orca -count=1`；原身份恢复定向 `go run ./cmd/test-feedback -- ./tests/integration -run '^TestCD67AdditionalImplementation$/^supported_terminal_recovery_and_original_identity$' -count=1` | 原生接线及输出完成后；每个所选用例实际运行、未知不重复、两轮关联、资源归属成立；受影响旧用例按新契约替换而不虚报零匹配 | runner summary / 日志；专属记录 A2 |
| [ONW-A3](../acceptance/orca-native-worker.md) | L2 live `smoke.orca-codex` 的新版通用 Orca driver | 隔离测试 clone / debug home；保留现有场景身份，新增只运行单个 live smoke 的 L3 注册入口 `orca.native-worker` 后用 `go run ./cmd/test-feedback -level=L3 -suite=orca -env=live '-scenario=orca.native-worker'` | 驱动和注册落实后；真实原生启动/工作区操作/反馈/结算/输出，权限由 Orca 持有，资源停止清理完成；失败/skip 保留 | 新增场景目录含原生回执与精简权限来源；专属记录 A3 |

新增 `orca.native-worker` 是本任务待实现的最小原生场景，不声称入口已存在；它复用 smoke 所需驱动但不默认运行 GitHub 发布或完整 L2。原生账号/Orca宽松设置缺失时记录真实阻塞，不改参数绕过。Full、全量 L3 delivery/recovery 不默认运行；只有受影响断言无法由上述范围覆盖时再补定向入口。

ONW-M1 在新构建部署后由用户集中确认安装入口及权限显示，窗口待安排；不阻塞本任务非人工验证和无关研发。原生自动失败、输出不可核实、清理未知不能改称人工后验收。
