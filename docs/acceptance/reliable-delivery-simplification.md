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

## 合并前补充：PR 缓存观察时间

按用户授权补充 PR 原 `checked_at`，经纯投影、独立 DTO 和前端显示“上次核实于”；零值不输出时间。查询失败保留上次已核实时间和错误，缺失显示“未记录查询时间”；始终标注缓存、非实时且本页刷新不查询外部 PR。显示时间不作为控制或交付凭据，正式 TaskDelivery 仍沿原 PR 刷新。

合入最新主干 `534ea902`（含项目维护 #373）；唯一内容冲突在 quick-owner-unified-startup 测试列表，保留主干项目维护用例及本 PR 两个投影用例。没有修改共享主 checkout、安装环境或 guard。该主干已将无后端 owner 的 processwindow 纳入 HTTP 允许闭包；本次 HTTPConsumerBoundary 通过，不再把原实施轮失败当成当前阻塞。

验证：合并后投影、API/HTTP 边界、原子观察、受理绑定、Outbox、quick 登记及项目维护相关定向选择五包通过；时间副本隔离、缺失及失败保留定向复验通过。前端两文件 **18 项通过**，TypeScript/Vite 生产构建通过。原失败和旧构建证据均保留各自范围，不改写为成功。

本次独立[后端复验](evidence/reliable-delivery-20261021-merge/go-tests.log)、[时间投影复验](evidence/reliable-delivery-20261021-merge/timestamp-projection.log)、[前端测试](evidence/reliable-delivery-20261021-merge/web-tests.log)及[构建日志](evidence/reliable-delivery-20261021-merge/web-build.log)归档于本仓；仅本地分级夹具，没有原生模型或安装资格扩展。
