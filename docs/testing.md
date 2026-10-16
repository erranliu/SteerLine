# 测试总入口

执行域遗留清理（2026-10-16）按[Plan](plans/execution-legacy-cleanup.plan.md)核对全平台生产引用、生产入口编译和固定 L1；旧执行实现专属测试随实现删除，已无测试的 `execution/model` 不再独立登记 L1，仍被消费的值编解码和 caller/Task/存储/宿主边界测试保留。实际结果见[专属验收](acceptance/execution-legacy-cleanup.md)。

Runner 执行展示与最近消息的定向验证见 [Plan](plans/execution-observability.plan.md) / [验收](acceptance/execution-observability.md)，覆盖原子状态变化、只读消息契约、当前可见执行轮询与取消；不使用安装环境或真实模型。

三 EXE 安装部署与首次迁移的测试范围、命令和真实结果见 [部署 Plan](plans/install-deploy.plan.md) 与 [安装迁移验收](acceptance/install-cutover-20261015.md)。安装环境仅用于用户授权的实际部署操作，普通回归/模型验收仍使用下述隔离入口；空闲安装停启不代表模型执行或恢复通过。

源码追溯：已迁出的本地源码链接固定到拆分前提交；当前模块归属见 [代码架构](architecture/current-code-architecture.md)。

SteerRunner 拆分后的测试归属及实施安排见 [Spec](spec/runner-test-convergence.spec.md)、[Plan](spec/runner-test-convergence.plan.md)和[专属验收](acceptance/runner-test-convergence.md)。SteerLine 保留调用方、Task/DAG、候选、交付、宿主和 Web 测试；Codex/Orca、原生恢复、沙箱及执行流程专项在 Runner 仓库维护。

完整 L2 保留真实 SteerLine → 固定 Runner EXE → 本机 Codex → 实现/报告/发布 → 真实 SteerlineTest PR → 合并 → TaskDelivery/后继释放。它覆盖 Runner 正常运行能力。执行专项迁出不降低这条链路为模拟测试。

## L1–L4 定义

SteerLine 直接采用 [开发项目测试架构规范](../SteerlineProjectGuideDoc/testing-architecture.md)。L1–L4 定义、L3 正交与并行设计、分级内容清单要求及跨项目测试归属统一在该规范维护；本文记录 SteerLine 的具体入口、环境、清单和实施缺口。

SteerLine 的 L1 是固定的快速集成/单元集合；完整 L2 是 controlled gate 加真实本机 Codex 核心交付链；L3 为等待合并时调用方重启、宿主生命周期和双 home 独立操作；L4 为构建/路径/宿主写入/同 ID 重放的系统边界。本次拆分和执行证据见 [Plan](plans/test-level-alignment.plan.md) / [验收](acceptance/test-level-alignment.md)。

SteerLine 的 Full 只运行现有确定性 Go 测试，不包含 L2/L3/L4/live。L2/L3/L4 的机器登记为 `tests/e2e/suites.json`，可读内容清单及本项目维护约定见 [E2E 指南](../tests/e2e/README.md#分级测试内容清单与维护)；实际结果在任务专属 acceptance 文档维护。

## 日常选择

按变更运行最小必要范围，L1/Full 不自动运行真实模型。所有验收行为及实时结果在 [acceptance](acceptance/README.md)记录，原日志使用新建临时目录。

| 入口 | 范围 | 外部效果 |
| --- | --- | --- |
| L1 | 固定 caller/Task/存储/HTTP/边界，默认三个进程分组 | 无真实模型 |
| 定向 Go | 受影响的 owner 包或测试 | 普通用例为隔离 fixture |
| Full | 全部保留的确定性 Go 测试，现有三 worker 分区/覆盖核对 | 不自动运行 L2/L3/L4/live |
| L2 controlled | Runner 公开 CLI/PR 夹具驱动产品交付 | 无模型，仅 gate-only |
| 完整 L2 | controlled 通过后本机 Codex 真实三阶段及 GitHub PR/交付 | 消耗模型、创建并合并本次真实 PR |
| L3 controlled | 调用方重启、宿主生命周期、双 home 独立操作 | 无模型，独立临时 home/Git/SQLite，最多二场景并行 |
| L4 controlled | 构建/路径/SQLite 拒绝、宿主写入拒绝、同 ID 碰撞与重放 | 无模型，全部为隔离边界现场，runtime 最多二项并行 |
| Web | 前端单元、构建及按需浏览器场景 | 浏览器使用新建 simulate 包 |

```powershell
# 固定最小集；诊断串行可加 -workers=1
go run ./cmd/test-feedback -level=L1

# 定向，Go 参数放在 -- 后；runner 的 -out 等参数放在 -- 前
go run ./cmd/test-feedback -- ./internal/runnerclient ./internal/runnerbridge ./internal/taskrunner -count=1

# 只有确需全量时执行
go run ./cmd/test-feedback -workers=3

# 编译不等于场景通过
go run ./cmd/test-feedback -build-only
go test -tags=e2e ./tests/e2e/... -run '^$'
```

L1 固定范围以 [scoped.go](../cmd/test-feedback/scoped.go)及 [task_scope.go](../cmd/test-feedback/task_scope.go)为准；Full 保留原分区、包枚举与不遗漏/不重复核对。原生专项开关及其执行器不再属于 SteerLine 测试准入。

## L2 / L3 / L4 清单现状与入口

[注册表](../tests/e2e/suites.json)是当前可执行场景的来源，具体测试内容见 [维护清单](../tests/e2e/README.md#分级测试内容清单与维护)。当前核对结果：

| 级别 | 已登记内容 | 清单与实现现状 |
| --- | --- | --- |
| L2 | 2 项：controlled gate、live 本机 Codex | 有固定 ID、实际测试映射和选择校验，内容清单可追踪。 |
| L3 | 3 项：等待合并重启、正常宿主生命周期、双 home 独立操作 | 支持多 suite/scenario；recovery/runtime 可跨包并行，总预算二；只选 runtime 时包内二并行。 |
| L4 | 4 项：源码构建保护、路径保护、宿主写入拒绝、同 ID 碰撞与重放 | 支持 `-level=L4 -suite=runtime -env=controlled`；每项独立资源，runtime 包内最多二项并行。 |

当前九项场景有固定 ID、实际测试映射和分级清单。四项原 runtime ID 改为 L4，按旧 L3 选择会明确失败，不提供别名；拆出的正常流程使用新 ID。“固定清单”不等于已证明运行稳定，本次结果见 [分级验收](acceptance/test-level-alignment.md)，早期冻结版本见 [历史精简验收](acceptance/runner-test-convergence.md)。

```powershell
# 无 token 门槛，不能当作完整 L2
go run ./cmd/test-feedback -level=L2 -env=controlled

# 完整 L2，先固定独立 Runner EXE；不接受跳过 gate 直跑 live
$env:STEERRUNNER_EXE = 'C:/Tools/SteerRunner/steerrunner.exe'
go run ./cmd/test-feedback -level=L2

# Codex 工具进程内可使用既有独立进程上下文，保持同一 L2 与证据规则
./scripts/test-l2.ps1 -SteerRunnerExe 'C:/Tools/SteerRunner/steerrunner.exe'

# 三项正常流程；跨包/包内调度共用二并行预算
go run ./cmd/test-feedback -level=L3 '-suite=recovery,runtime' -env=controlled

# 四项系统边界；可用 -scenario=<逗号分隔ID> 精确选择
go run ./cmd/test-feedback -level=L4 -suite=runtime -env=controlled

# 工具进程有 Windows Job 约束时，使用独立同用户进程
./scripts/test-runtime-isolation.ps1 -Level L3 -Suite 'recovery,runtime'
./scripts/test-runtime-isolation.ps1 -Level L4
# 默认依次运行两级 runtime；分别生成报告
./scripts/test-runtime-isolation.ps1
```

L2 的 controlled 运行、证据或清理失败时，阻止真实阶段。仅 gate 报告 `coverage=gate-only`，完整两阶段才是 `full-L2`。退役的 delivery/recovery live、Orca chain、native 环境和旧 runtime 聚合 ID 明确拒绝，不映射到其他场景。

真实链仅用已登录本机 Codex、固定模型 `gpt-6-luna` / `medium`、固定公开 Runner profile 和已有 GitHub 权限。L2 使用其分级隔离夹具与 `erranliu/SteerlineTest` 本次专属 base/head；任务实现/业务 commit、push、PR 创建必须由真实 Runner/Codex 执行。驱动可以准备 base、查询、转 ready、精确合并与清理；不能代做任务伪造通过。

L2 通过证据包括公开阶段/final 来源、原 Run/候选、Git 提交中的真实产物、精确已合并 PR、唯一 TaskDelivery、后继 ready 且未启动、资源清理。SteerLine 不解析原生 rollout、统计 token 或重验沙箱。没有原生 usage 不是补造数值的理由；公开结果、真实 Git/PR 与 Task 证据缺失仍必须失败。详见 [E2E 指南](../tests/e2e/README.md)。

## 夹具、Web 与调试包

确定性测试使用公开 Runner CLI 子进程夹具，不要求安装真实 Runner、Codex、Orca 或远端主机。夹具仅提供脚本化公开回执/Run/事件，不执行或模拟原生 workflow。原请求、未知回执、授权 revision、投影/游标、绑定、事务回滚与停止义务由对应 caller/SQLite/HTTP owner 验证。

前端与 HTTP owner 使用 `scripts/test-web.ps1`，浏览器入口及新包前置见 [Web 工程](../web/README.md)。浏览器 fixture 消费公开 Run/请求证据，浏览器关闭不停止原业务、同请求不重复提交、多项目/多标签隔离等要求继续保留。

临时/AI 调试及真实 PR 调试先运行 `scripts/new-debug-release.ps1`，创建 `C:\SteerlineDebug` 下独立环境；不使用安装现场。生成默认 live，AI 明确 `--simulate`；模拟方案使用包内公开 Runner CLI fixture，真实方案使用固定真实 EXE。`scripts/test-debug-release.ps1 -SteerRunnerExe <absolute>` 验证包生成、冻结、停止与 reset；其 `-Live` 复用完整 L2，不再运行第二条沙箱验收链。详见 [调试环境验收](acceptance/debug-environment.md)。

认证引用既有宿主登录，不复制安装数据库、认证或用户配置。方案启动后冻结，换方案新建包；重置不备份且活动服务/资源阻止重置。

## 报告与文档检查

runner 默认写系统临时目录 `steerline-test-feedback/<本次时间>`；显式 `-out` 须为新空目录。核对实际源码/dirty、选择范围、用例终态、`passed`/`complete`、场景/清理和覆盖类型；skip、零匹配、超时、缺证据都不是通过。失败保留原诊断，不自动重跑；原生未确认停止时保留现场，不用父进程退出代替停止事实。

纯文档检查：

```powershell
python scripts/check_doc_links.py
git diff --check
```

新 Plan 的测试计划按任务专属验收 ID 映射场景/命令、环境、时点、通过条件与证据位置；缺能力须列入实施或明确前置。结果只登记 acceptance，Plan 不维护实时结果。历史原生结果仅证明各自冻结版本与原范围，参见 [历史验收](acceptance/archive/README.md)。
