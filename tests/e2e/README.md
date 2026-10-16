# SteerLine E2E

执行专项由独立 SteerRunner 项目承担；SteerLine 保留调用方与产品闭环。早期测试精简见 [Spec](../../docs/spec/runner-test-convergence.spec.md)和[验收](../../docs/acceptance/runner-test-convergence.md)；当前 L3/L4 拆分见 [Plan](../../docs/plans/test-level-alignment.plan.md)与[专属验收](../../docs/acceptance/test-level-alignment.md)，历史七项登记不再代表当前清单。

## 分级测试内容清单与维护

L1–L4 的统一定义及设计要求采用 [开发项目测试架构规范](../../SteerlineProjectGuideDoc/testing-architecture.md)，本项目入口见 [测试总入口](../../docs/testing.md)。下面按当前实际登记级别列出测试内容。

[suites.json](suites.json)是机器执行登记的唯一来源，登记九项（L2 两项、L3 三项、L4 四项），保存 ID、level、suite、package、test、env 和适用的 chain；下表维护这些 ID 对应的流程、主要断言和执行约束。不得在其他文档维护相互独立的可执行清单。

### L2：最小核心流程

| 场景 ID | 环境 | 测试内容与主要断言 | 执行约束 |
| --- | --- | --- | --- |
| `smoke.delivery` | controlled | 创建有依赖的任务，领取并执行至 PR 待合并，合并后只交付一次并释放后继；后继未自动启动，重复查询不新增执行或改变交付，交付后静默。 | 固定前置 gate；公开 Runner CLI 与隔离 PR provider，无真实模型。 |
| `smoke.local-codex` | live / local-codex | 真实 Runner/Codex 实现、报告、发布；检查三个公开 final 来源、PR head 中真实产物、精确 merged PR、唯一 TaskDelivery、后继 ready 且未启动及资源清理。 | gate 通过并清理完整后顺序执行；使用本次专属 SteerlineTest base/head。 |

两项组成一条最小核心冒烟链；controlled 仅为 gate-only，完整两阶段才是 full-L2。

### L3：可并行的不同流程

以下均为 controlled，使用真实 SteerLine/Git/SQLite 和公开 Runner/PR fixture；每项拥有独立临时 home、repo、数据库、端口、provider 和证据。三个测试均允许并行。选 recovery/runtime 两包时并行启动两包、各包内部串行；只选 runtime 时包内最多二项并行，总预算二。

| 场景 ID | 测试内容与主要断言 | 相对其他场景的主要覆盖维度 |
| --- | --- | --- |
| `recovery.waiting-merge-restart` | PR 待合并时重启 SteerLine，保留原候选/Run/请求及公开投影，不新增业务输入、不提前释放后继；随后合并并完成唯一交付。 | 在 L2 交付骨架中加入 SteerLine 重启，验证持久化事实和调用方连续性。 |
| `runtime.service-lifecycle` | 中文/空格路径和无关 cwd 下生成启动器；创建任务、核对精确宿主身份、执行正常控制、停止并核对逐项目结算，再启动并确认新实例和原任务事实，最后正常停止。 | 单 home 的正常宿主生命周期，不注入陈旧身份/配置/锁冲突。 |
| `runtime.dual-home-operation` | 两个独立 home 同时在线，使用不同请求分别 discard 不同任务；核对持久化结果、另一项目/provider 不变、无执行候选、原实例不变，最后停止两者。 | 多 home 合法并存及独立操作，不做同请求 ID 重放。 |

本项目按规范维护上表的独有覆盖维度；并行资源声明涵盖 home、repo、数据库、端口、可变构建输出、provider 和清理所有权。

### L4：系统边界

以下均为 controlled/runtime，使用自己的临时 home/repo/provider/端口/证据。独占关系只存在于每项内部人为构造的别名或冲突，不触碰共享安装现场，因此这四项边界可包内二并行；L4 不是全局串行开关。其他包按顺序执行，后续不可并行项需单独登记具体约束并落实调度。

| 场景 ID | 边界条件与主要断言 | 所有者/独有覆盖 |
| --- | --- | --- |
| `runtime.source-build-isolation` | 源码克隆的 bin junction、EXE hardlink 指向本项安装夹具时拒绝 build；合法 build/rebuild 保持源码 clean、来源身份及夹具 EXE/数据库/任务事实不变。 | SteerLine build 输出保护；可写源码克隆仅归本项。 |
| `runtime.path-isolation` | 拒绝 state junction、run_dir 越界、退役 worktree_root 配置和 SQLite DB/WAL/SHM hardlink；受保护数据不变，恢复正常文件后可使用。 | SteerLine 配置解析及 SQLite 打开前保护。 |
| `runtime.service-write-isolation` | 拒绝陈旧实例、宿主冻结目录以外的配置写入、同 repo 跨 home 启动；拒绝无请求/候选副作用，正常控制作为正向对照；重启后仍拒绝原实例写入。 | SteerLine 控制入口与 repository 独占，不测试 Runner 会话恢复。 |
| `runtime.dual-home-isolation` | 两 home 故意使用相同 task/request ID，执行 discard 后以原参数重放；回执一致、任务/graph revision 不变，另一 home/provider 无改写、无候选且实例不变。 | SteerLine Task/请求命名空间与幂等反例，不测试 Runner 原生输入去重。 |

四项原 runtime ID 保留验证目的，级别统一改为 L4；在 L3 下选择这些 ID 明确失败，无兼容映射。构建输出与配置路径均使用链接反例，但保护的写入口不同；服务测试验证冲突拒绝，双 home 验证命名空间/回执，保留这些独有断言。L2/L3/L4 的实际结果统一见 [本次验收](../../docs/acceptance/test-level-alignment.md)，清单不声明运行稳定性。

### 清单维护规则

通用要求见 [测试架构规范](../../SteerlineProjectGuideDoc/testing-architecture.md#l2l3l4-内容清单)，本项目具体落地如下：

- 新增、删除、改级、改环境或调整主要断言时，在同一变更中同步场景实现、manifest、本节内容和 `cmd/test-feedback` 的相关选择/并行/证据校验；当前不支持的级别须先落实执行能力。
- 退役 ID、跨级选择由 `test-feedback` 明确拒绝；L3/L4 只接受 controlled，live 仅为完整 L2 的一部分。
- 运行器按 manifest 选择、冻结并枚举测试，核对实际执行、非零匹配、终态和清理；[e2e_test.go](../../cmd/test-feedback/e2e_test.go)另核对本文表格中的 ID/级别与 manifest 一致且每项只登记一次。语义正交性及具体断言仍需人工代码审查。
- Plan 引用稳定 ID 与专属验收项，实际通过/失败及日志位置写入 `docs/acceptance/`；内容清单不维护实时测试结果。证据统一为 summary.json 和 `scenarios/<id>/scenario.json`，详情见后文。

## 场景执行入口

```powershell
go run ./cmd/test-feedback -level=L2 -env=controlled
$env:STEERRUNNER_EXE = 'C:/Tools/SteerRunner/steerrunner.exe'
go run ./cmd/test-feedback -level=L2
# 从有 Job 约束的工具进程发起时，复用仓库独立进程启动上下文
./scripts/test-l2.ps1 -SteerRunnerExe 'C:/Tools/SteerRunner/steerrunner.exe'
go run ./cmd/test-feedback -level=L3 '-suite=recovery,runtime' -env=controlled
go run ./cmd/test-feedback -level=L4 -suite=runtime -env=controlled
# 精确选择边界子集；不能跨级或零匹配
go run ./cmd/test-feedback -level=L4 -suite=runtime -env=controlled '-scenario=runtime.path-isolation,runtime.dual-home-isolation'
# Windows 工具 Job 约束下使用独立同用户进程
./scripts/test-runtime-isolation.ps1 -Level L3 -Suite 'recovery,runtime' -Scenario 'runtime.service-lifecycle,recovery.waiting-merge-restart'
./scripts/test-runtime-isolation.ps1 -Level L4
# 默认两级 runtime 顺序执行；-OutDir <新目录> 按 L3/L4 分别保存报告
./scripts/test-runtime-isolation.ps1
```

L2 固定先 gate 后 live，不能按场景绕开 gate。gate 的执行/证据/清理任一失败，不启动真实阶段；gate-only 不等于 full-L2。L3/L4 支持逗号多选 suite/scenario，必须显式选择 suite 和 controlled；子集报告为 selected-L3/selected-L4。已退役 ID、Orca chain、native 环境直接拒绝。具体总入口见 [docs/testing.md](../../docs/testing.md)。

## 真实 L2

真实链为 SteerLine 产品命令 → 固定独立 Runner EXE → 本机 Codex → development 实现、报告、发布 → 真实 SteerlineTest PR → 真实 provider 核实 merge → 唯一 TaskDelivery 和后继释放。

- Windows/amd64、Go、Git、已登录 Codex、gh 及 `erranliu/SteerlineTest` 写权限；选择固定本机 Codex 可执行文件、`gpt-6-luna` / `medium`。Runner 负责原生版本/权限/环境准入，SteerLine 仅装配公开配置。
- 每次独立隔离现场、本次专属随机 base/head；不修改 SteerlineTest 主分支。驱动准备 fixture base，真实 worker 执行业务实现与 commit/push/PR 创建；驱动可精确转 ready、合并、查询及清理。
- 任务产物 `result.txt` 必须在 PR head 提交中为指定内容；通过 Git 对象检查，不读取 Runner 私有工作区、数据库或日志。公开 canonical final/来源必须绑定原候选/Run，三阶段正常结果完整。
- 合并事实必须包含精确 URL/repository/base/head/head SHA/merge SHA。TaskDelivery 只能有一次且绑定所选候选/PR；后继 ready、未自动启动。
- 原生 usage/rollout、沙箱与原生恢复不在 SteerLine 重验。证据缺少公开结果、真实 Git/PR 或 Task 事实仍失败，不能用模型自述、helper 或假 PR 替代。
- 使用既有宿主登录，不复制认证/安装数据库/配置，不写凭据进日志。缺 EXE/登录/权限时明确失败或阻断，不切回模拟。

## 受控夹具与所有者

controlled 场景运行真实 SteerLine、Git、SQLite，但 Runner 由公开 CLI 子进程夹具提供，PR 使用隔离 provider。fixture 保存预设公开 Run/事件与不可变请求记录，不实现 Codex app-server、Orca transport、workflow 或恢复状态机。caller 断言提交/授权/回执/事件/交付的影响，不计数原生线程、终端或业务轮次。

`smoke.delivery` 包含基础交付和交付后静默，不再重复登记 delivery.basic。waiting-merge-restart 只重启 SteerLine，核对原候选/Run/请求/游标，无自动输入，随后唯一交付。更小的未知回执、重复授权、事务回滚、迟到接受/停止等由 client/bridge/taskrunner/SQLite/HTTP owner 承担。

fixture 的 `caller-fixture.json`、provider `requests` 是测试自产公开协议证据，不是 Runner 私有状态。真实 Runner 从不使用这些文件。通用 Git/provider、process 与 UI 夹具继续保留，Orca/Codex 执行专项在 Runner 仓库。

## 环境、预算和清理

临时/AI 调试及真实 PR 调试先由 `scripts/new-debug-release.ps1` 生成 `C:\SteerlineDebug` 新包；分级 E2E 继续用自身隔离夹具。不得使用当前 `C:\SteerlineInstall` 或保留的 `C:\SteerlineRuntime`。调试包默认 live，AI 用 `--simulate` 明确选模拟；方案冻结后不切换，reset 保留定义/契约/依赖/Git/PR，活动资源阻止 reset。

普通 controlled 业务 5 分钟，waiting-merge-restart 和 live 10 分钟；runtime 子场景保留主干 5 分钟业务期限和独立父期限。live Go 父期限 35 分钟包含独立清理，未放宽业务窗口；普通命令、领取/控制及收尾沿现有明确预算。`FinishBody` 冻结业务期限，清理结果不能掩盖业务超时。

清理只处理本次拥有的资源、服务和 base/head；PR 历史保留。先围栏原 Runner，按公开 Run/回执确认停止；未确认不能当作已结算，也不能删除诊断冒充成功。失败保留证据，不自动重试。

## 证据与扩展

summary.json 记录源码/工作树、冻结构建 SHA、实际选择、用例终态、coverage 和清理；每项 `scenarios/<id>/scenario.json` 必须实际通过、清理完整且资源非空。L2 live 另保存 `live_delivery`：Runner 来源、原 Run/候选、三个公开 final 来源、Git 产物、精确 merged PR、唯一 Delivery 与后继事实。缺项由 runner 拒绝。

```powershell
# 测试基础设施与调用方依赖边界
go test ./cmd/test-feedback ./tests/e2e/internal/... ./internal/testkit/boundary/... -count=1
# 编译所有登记场景，不执行业务
go test -tags=e2e ./tests/e2e/... -run '^$'
```

新增场景先判断所有者，优先在原 owner 添加最小用例；不因 Runner 增加后端而扩展 SteerLine 原生矩阵。确有产品闭环差异时在 manifest 登记 ID/level/suite/package/test/env，live 仅本机 Codex 且仍属于完整 L2。一个包不混合环境，runner 冻结/枚举测试并核对 manifest。Plan 按专属验收 ID 覆盖命令、环境、时点、通过条件和证据位置，新增能力必须在实施中落实。

## 执行展示与最近消息

本轮采用定向 Go、Web fake timer 与 Runner 实际 EXE/受控服务验证，不新增 L2/L3 live 场景；范围与命令见 [Plan](../../docs/plans/execution-observability.plan.md)，结果见[专属验收](../../docs/acceptance/execution-observability.md)。不据消息缺失判定卡死，不在安装现场验收。
