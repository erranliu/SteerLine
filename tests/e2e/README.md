# SteerLine E2E

当前测试精简契约见 [Spec](../../docs/spec/runner-test-convergence.spec.md)、[Plan](../../docs/spec/runner-test-convergence.plan.md)和[验收](../../docs/acceptance/runner-test-convergence.md)。执行专项由独立 SteerRunner 项目承担；SteerLine 保留调用方与产品闭环。

## 场景与执行

[suites.json](suites.json)精确登记七项。L2 包含 `smoke.delivery` controlled 和 `smoke.local-codex` live；L3 controlled 包含 `recovery.waiting-merge-restart` 及四项 runtime source-build/path/service-write/dual-home isolation。runtime 保留主干独立断言、独立证据和最多二项并行。

```powershell
go run ./cmd/test-feedback -level=L2 -env=controlled
$env:STEERRUNNER_EXE = 'C:/Tools/SteerRunner/steerrunner.exe'
go run ./cmd/test-feedback -level=L2
# 从有 Job 约束的工具进程发起时，复用仓库独立进程启动上下文
./scripts/test-l2.ps1 -SteerRunnerExe 'C:/Tools/SteerRunner/steerrunner.exe'
go run ./cmd/test-feedback -level=L3 '-suite=recovery,runtime' -env=controlled
```

L2 固定先 gate 后 live，不能按场景绕开 gate。gate 的执行/证据/清理任一失败，不启动真实阶段；gate-only 不等于 full-L2。L3 支持逗号多选 suite/scenario。已退役 ID、Orca chain、native 环境直接拒绝。具体总入口见 [docs/testing.md](../../docs/testing.md)。

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
