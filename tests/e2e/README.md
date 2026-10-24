# SteerLine E2E

SteerLine 验证调用方与产品闭环，Runner 的原生执行、恢复和协议故障矩阵由 SteerRunner 维护。统一分层见 [测试架构](../../SteerlineProjectGuideDoc/testing-architecture.md)，日常入口见 [测试指南](../../SteerlineFlow/testing.md)。

## 登记与清单

[suites.json](suites.json)登记 E2E ID、级别、suite、package、test、env 及适用 chain；当前 L2 为单一受控冒烟，L3 仅资源隔离的受控功能，真实链及共享构建目录场景归 L4。完整内容只在 [L2](../../SteerlineFlow/testing/l2.md)、[L3](../../SteerlineFlow/testing/l3.md)、[L4](../../SteerlineFlow/testing/l4.md)维护，并与 [JSON](../../SteerlineFlow/testing/test-list.json)同步。

## 执行

```powershell
go run ./cmd/test-feedback -level=L2
go run ./cmd/test-feedback -level=L3 -suite=recovery,runtime -env=controlled
go run ./cmd/test-feedback -level=L4 -suite=runtime -env=controlled -scenario=runtime.path-isolation
$env:STEERRUNNER_EXE = 'C:/Tools/SteerRunner/steerrunner.exe'
go run ./cmd/test-feedback -level=L4 -suite=smoke -env=live -scenario=smoke.local-codex
```

L2 的完整受控冒烟报告为 `full-L2`，不会构造真实模型现场。L3/L4 子集为 selected-L3/selected-L4，跨级、退役 ID、空选择和混合环境拒绝；live 仅支持单独精确选择。L3 总并发二；L4 包间串行，runtime 四项独立边界可包内二并行，加入共享构建目录场景则串行。

Windows 工具进程有 Job 约束时，使用 `scripts/test-l2.ps1`、`scripts/test-live-delivery.ps1`、`scripts/test-audited-workflow.ps1` 或 `scripts/test-runtime-isolation.ps1` 的独立同用户进程上下文，不修改产品权限或安装现场。

## 真实链与环境

Windows/amd64、Go/Git、已登录本机 Codex、固定独立 Runner EXE。交付和审计需要 gh 及 `erranliu/SteerlineTest` 写权限；每次只使用本次随机 base/head，不修改主分支。worker 实际执行任务实现、commit、push 和 PR 创建，驱动仅准备 fixture base、查询、转 ready、精确合并与清理。任务定义和输入公开，认证、原生转录及私有状态不写入共享证据。

真实交付固定 `gpt-6-luna` / `medium`；审计通过 `STEERLINE_AUDIT_EXECUTION_MODEL` / `STEERLINE_AUDIT_MODEL` 显式指定两个不同模型，执行 medium、审计 high。验证公开 final、原候选/Run、真实 Git 产物、精确已合并 PR、唯一 TaskDelivery、后继 ready 未启动、资源结算；审计另核对正式产品详情中的模型、材料及审计来源。测试管理真实链按自己的 proof 核对安排和报告消费，不发布 PR。

调试包及浏览器前置遵循 [独立调试流程](../../docs/runtime-install-debug.md)。依赖/构建使用源码可写目录的场景归 L4，不能与其他 Web 安装/构建并发。

## 证据与扩展

输出到全新 `-out` 目录：summary、原 JSONL、冻结构建摘要及 `scenarios/<id>/scenario.json`。执行器核对实际非零命中、用例终态、选择、必要 proof 和清理；缺失、跳过、超时或自然语言成功均不能代替通过。失败保留现场与诊断，不自动重试。受管报告与 Git 历史导出见 [报告约定](../../SteerlineFlow/testing/reports.md)。

新增、改级、改环境或改变主要断言时，同步实现、manifest、分级列表、JSON 和选择/并行/证据校验。各场景说明独有覆盖和清理所有权；L3 不共享可变产物，无顺序依赖。测试只消费依赖公开边界，不在调用方重建 Runner 内部流程。历史 Spec/Plan/验收按冻结定义读取，当前命令以本指南和分级清单为准。

测试管理真实专项使用 PowerShell 7 的正式进度写入器，核对 Runner 单文件快照及 API/浏览器的通过、重跑未完成状态；原 managed 报告仅作为独立原始记录，不要求候选收集。真实模型专项仍需显式选择，不以受控联验替代。

SteerlineTest 的原项目脚本可通过独立 owner 入口验证真实本地进度链：

```powershell
./scripts/test-project-progress-live.ps1 -ProjectRepository C:/Projects/SteerlineTest -ProjectCommit <完整提交SHA> -SteerRunnerExe <固定Runner绝对路径> -OutRoot C:/SteerlineDebug/<本次全新scope>
```

需要 Windows、Go、Git、Python、gh 和已登录的本机 Codex；原生认证必须可访问 `erranliu/SteerlineTest`。入口建立独立 clone/home/任务及 loopback Git，固定原项目的三个 Python 程序，真实生成测试安排后让 worker 执行一次原程序；不启动样本 101–105，不发布 PR。默认每阶段六秒，核对六个 API 状态与同版本 Runner 原始快照、实际失败子进程和最终原报告，再停止原 Run 并验证资源清理。测试使用 `gpt-6-luna` / `medium`，会消耗模型。测试由 `e2e` build tag 隔离，普通 Go Full 不包含此项；显式编译后若未注入项目参数会跳过，不能计为本专项通过。

入口输出 `result.json`、`test.log` 和 `reports/scenarios/testing.project-progress-local-codex/`，其中保留 `source.json`、`initial-binding.json`、`runner-*.json`、`api-observations.json`、`original-run.json`、`worker-program-sha256.json`、`original-reports/`、原 stop 请求/回执、`stopped-run.json`、`proof.json` 及 `scenario.json`。只有实际执行通过且清理完整才写总结果通过；它尚未登记为统一受管测试，不导出 `go.full` 或其他 L4 项通过。

远端及连续浏览器联验在同一入口追加 `-RemoteProfile <公开执行配置JSON绝对路径>`。文件是一项完整 execution profile，必须明确使用 `orca-terminal` / `orca_environment`、现有 Orca CLI 和远端 Codex；不复制认证或安装状态。需要 Node/npm、Edge、远端注册的 SteerlineTest 仓库及 Python。自动材料生成仍使用本机默认模型，任务执行显式选择远端 profile。入口在 GitHub 推送本次随机 base ref，仅在远端根仓 fetch 此 ref，核对根工作区未变；原 Runner 负责创建、执行和停止实际 worker。

远端模式每阶段十五秒，使用同一 Edge 页面连续观察 0→1→2→1→2→3，核对 DOM 完成项、计数、版本与相同版本的原 Runner 快照；观察期间不得刷新或导航。远端原报告和程序由独占诊断终端只读取得，终端必须关闭；驱动不修改进度、不向原 worker 输入。证据在 `reports/scenarios/testing.project-progress-orca-browser/`，另存 `remote-target.json`、`remote-fetch.json`、`remote-source.json`、Orca 公开回执，以及 `browser/observations.json`、六张截图和 Playwright trace。原 Run 停止后先核对调用方已观察同一原候选的 stopped 投影，再关闭调用方并结算自有 base 删除。此项会构建 Web，不与其他 Web 安装/构建并发；不覆盖安装版或 Task 18。
