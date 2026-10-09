# Plan：本机 Orca 原生通用 worker 与权限委托

任务 ID：86；任务名称：本机 Orca 原生通用 worker 与权限委托。实施依据为 [Spec](orca-native-worker.spec.md)，自动项与实时结果归[专属记录](../acceptance/orca-native-worker.md)。开始实现前固定本 Spec/Plan 的实际 Git commit 与路径，执行来源为 execution-map。

## 实施步骤

1. 更新本机 Orca profile 和 app 目录注册，agent 作为配置值、model/effort 作为 Orca 通用偏好；移除本机 Orca 对 codex.worker-profile、固定 Codex 权限封套与版本探测的依赖。为变更格式设置明确版本，不增加旧格式转换。
2. 先以受控回执和隔离原生场景核实 Terminal worker 的首轮与同会话续轮能力，记录当前版本的原生 mode/资源/handle 含义；本轮不支持 structured 同会话接续，不为其猜测接口。将 Orca Terminal + YOLO 列为启动前环境准备；若组合启动后回执显示不支持类型，保留原请求、已送达输入及首轮效果的已知/未知事实，阻断后续派发与业务推进，按原身份/原生资格停止和释放，不声称首轮零执行，不改全局设置或盲重派。首轮由一个原生请求拥有工作区/agent 启动，记录组合副作用及原请求以供对账；不先启动 agent 再重复创建。首轮传通用 agent/model/effort，effort 必须有 model，核对 requested/effective；续轮只选原 worker，不重传启动偏好。删除 SteerLine 自建 agent 启动、私有会话绑定及 TUI 解析。原生能力不足时记录具体平台前置，不增加专属分支或替代会话。
3. 改为 Orca 通用 worker-show/read/inbox 的输入、等待、输出与结算事实；停止/释放/保留采用 worker-stop/release/retain，原 Dispatch 结算资格、未知停止及释放回执纳入受控测试。移除本机专属 sandbox readiness 和文件 relay。保证当前持续 worker 的规范 final 采集；接口供后续 #84 消费，不提前实施分步流程，保持 Git/PR 真值核验。
4. 更新受影响配置示例、产品设计、测试 fixture 与隔离原生 driver。补齐通用 worker-start / 输出驱动和当前格式断言，不删减仍有效的身份/未知/恢复要求。
5. 执行下表最小验证及独立复审，登记具体构建、命令、结果、缺口及资源事实；同步 execution-map。工作区实现完成不等于 PR 已真实合并。本次不自动覆盖 C:/SteerlineRuntime，不迁移或改写其 102 冻结执行。

本任务拥有 Orca adapter 与配置/观察共同接线；本地 Codex 所有者代码仅作确有必要的目录解耦。#84/#45 消费本任务的实际接口，不并行重复修改同一接线。#84 尚未实现；PR #253 合入的是规划，不证明交付。按 #86→#84→#45→#44 串行集成，#84 实施前核对其 adapter 计划与本任务的实际通用接口，不在本任务抢先实施其流程。

## 测试计划

| 验收项 | 级别 / 场景 | 环境与命令 | 时点 / 通过条件 | 证据与结果 |
| --- | --- | --- | --- | --- |
| [ONW-A1](../acceptance/orca-native-worker.md) | L1 定向 `TestOrcaNativeAgentBoundary`、profile/config 回归 | Windows；新增上述边界测试后 `go run ./cmd/test-feedback -- ./internal/execution/adapters/orca ./internal/app -run 'TestOrcaNativeAgentBoundary|TestOrcaNativeProfile' -count=1`；本地策略另用 `go run ./cmd/test-feedback -- ./internal/execution/adapters/codex -run '^TestPS2NativeLaunchAndInputUseAutomaticReview$' -count=1` | 配置/注册完成后；无生产专属依赖、两个 agent 值同路径、旧格式拒绝；每个所选包/顶层测试实际命中，本地策略保持 | runner 独立 -out；专属记录 A1 |
| [ONW-A2](../acceptance/orca-native-worker.md) | L1 受影响 adapter/执行/集成；通用生命周期受控场景 | Windows；补齐 fixture 后 `go run ./cmd/test-feedback -level=L1`，并 `go run ./cmd/test-feedback -- ./internal/execution/adapters/orca -count=1`；原身份恢复定向 `go run ./cmd/test-feedback -- ./tests/integration -run '^TestCD67AdditionalImplementation$/^supported_terminal_recovery_and_original_identity$' -count=1` | 原生接线及输出完成后；每个所选用例实际运行、未知不重复、首轮单次创建、续轮原会话关联；不支持 structured 类型的拒绝/部分资源；worker-stop/release 的资格、拒绝/未知回执与资源归属成立；受影响旧用例按新契约替换而不虚报零匹配 | runner summary / 日志；专属记录 A2 |
| [ONW-A3](../acceptance/orca-native-worker.md) | L3 live，复用 `smoke.orca-codex` 的新版通用 Orca driver | 隔离测试 clone / debug home，Orca Terminal + YOLO；保留现有场景身份，新增只运行单个原生闭环的 L3 注册入口 `orca.native-worker` 后用 `go run ./cmd/test-feedback -level=L3 -suite=orca -env=live '-scenario=orca.native-worker'` | 驱动和注册落实后；真实原生启动/工作区操作/反馈/结算/输出，至少两轮保持原会话，权限由 Orca 持有，资源停止清理完成；失败/skip 保留 | 新增场景目录含原生回执与精简权限来源；专属记录 A3 |

新增 `orca.native-worker` 是本任务待实现的最小原生场景，不声称入口已存在；它复用 smoke 所需驱动但不默认运行 GitHub 发布或完整 L2。原生账号/Orca宽松设置缺失时记录真实阻塞，不改参数绕过。Full、全量 L3 delivery/recovery 不默认运行；只有受影响断言无法由上述范围覆盖时再补定向入口。

ONW-M1 在新构建部署后由用户集中确认安装入口及权限显示，窗口待安排；不阻塞本任务非人工验证和无关研发。原生自动失败、输出不可核实、清理未知不能改称人工后验收。
