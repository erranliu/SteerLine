# 可靠性交付摘要验收

日期：2026-10-21。归属[可靠性交付摘要计划](../plans/reliable-delivery-simplification.plan.md)。基线 `36d4f596`，不使用或修改安装环境。执行来源、Runner 基线与实验日志保存在本次 Runner 工作树 `.evidence/reliable-delivery-20261021/`。

| 验收 | 状态 | 结果与边界 |
| --- | --- | --- |
| RD-D1 | 通过 | 当前说明收敛；历史验收不改写；两仓变更文档链接与 diff 检查通过 |
| RD-D2 | 定向通过 | 原 Run 纯投影及独立展示 DTO，无新查询/写命令；最终前端 2 文件 15 项及生产构建通过，真实组件独立渲染取图检查通过 |
| RD-D3 | 通过，无新增等待层 | 已有 Bridge.Step/deliver 满足异步原命令对账；两包全测通过，新增 pending 后续 stop 反例，零重发 |
| RD-BOUNDARY | 有既有失败，已隔离新增问题 | 初版 API 直接引用 runnercontract 被守卫拒绝，已改独立 DTO；API 边界复验通过。HTTP 闭包仍报告 projectconnection→processwindow，原 HEAD 同样存在此 import；不放宽规则、不改无关实现 |

最终命令：`go test ./internal/app -run 'TestRunnerDelivery|TestRunnerWorkflowReadProjection|TestCM71ProductDependencyBoundary' -count=1 -timeout=2m`、`go test ./internal/runnerbridge ./internal/taskrunner -count=1 -timeout=2m` 均通过。新投影测试登记到既有 quick-owner-unified-startup；`go test ./cmd/test-feedback -run 'TestQuick' -count=1 -timeout=2m` 通过。无新测试分组或执行运行时。

Runner 审计输出试验只在独立目录使用原生 Codex 与 Runner 夹具，不创建真实 PR，不代表本次 UI 已获全产品原生交付资格。原始日志为 Runner 工作树 `.evidence/reliable-delivery-20261021/steerline-projection-pass.log`、`steerline-bridge-pass.log`、`web-tests.log`、`web-build.log`、`test-selection.log`；截图为同目录 `delivery-summary.png`。

Runner 的 V1 小样本对比 4 项通过，额外补报由 2 降为 0；随后旧冻结提示跨构建接管/新默认的两个正式受控分支通过，才更新新 Run 的默认提示文本。调用方没有强制覆盖 audit.code，没有新增原生 schema 或恢复策略；原有个人配置的显式覆盖仍保留。两仓结果不冒充安装环境已更新。

提交 PR 前合入主干 `9cf844639c21f841eb743e7b4f9680b23c7db0af`，保留其同快照观察、完整受理绑定及部署改动；本摘要只消费已有投影。合并后 `go test ./internal/app ./internal/runnerbridge ./internal/taskrunner ./internal/runnerclient ./cmd/test-feedback -run 'TestRunnerDelivery|TestRunnerWorkflowReadProjection|TestCM71ProductDependencyBoundary|TestRunnerObservation|TestSteerRunnerOutboxBoundaries|TestRunnerFullBindingAdmission|TestQuick' -count=1 -timeout=2m` 五包通过。前端源码未因合并改变，复用上列 15 项及构建证据；HTTP 既有失败的 import 在该主干仍存在。合并复验日志登记于 Runner 的 `docs/acceptance/evidence/reliable-delivery-20261021/pr-steerline-rules.log`。
