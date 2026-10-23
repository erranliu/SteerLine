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
