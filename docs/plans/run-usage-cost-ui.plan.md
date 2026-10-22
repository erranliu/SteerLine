# 执行页用量与费用接入

用户于 2026-10-22 确认本聊天方案并授权实施、提交 PR；普通开发，不纳入 roadmap。

## 范围与实施

1. 消费 Runner #47（合并提交 `61d9317524b56f148b2b31e2f23a4acc1de58081`）公开 `run-usage/v1`、`run-cost-estimate/v1`。仅调用绑定原实例的公开 CLI，不访问 Runner 私有日志或数据库。
2. `GET /runs/:candidate/usage?execution_id=:runnerRun` 校验项目候选与原 Run/目录。读取用量后只估算该 snapshot revision；费用失败保留 token。无能力明确不可用。
3. `execution.json` 可选 `pricing_policy` 指定绝对价表文件，`usage_policy` 原样经 service start 交给 Runner 校验。默认不新增远端授权；配置不进入候选执行 profile。Web 保存执行配置保留这两个字段。
4. 当前候选标题下增加一行摘要及原生 details；显示累计主会话 token、计价覆盖、价表、原观察时间和原因。金额十进制字符串只在显示时舍入。未知不补零，partial 不显示总额。
5. 可见且在线的当前候选每五秒查询；完整收敛后停轮询，新 turn/attempt/阶段变化重新查询，命令回执使查询失效。切换或隐藏取消请求；刷新失败保留同查询的最后结果。手动刷新只读。

本次仅增加观察/宿主配置传递，不修改 [worker lifecycle](../product-design/worker-lifecycle.md) W01–W18、异常/重试/恢复/停止矩阵；费用不参与业务 gate。远端授权的校验、采集及回收仍由 Runner 所有，既有服务拒绝冲突策略，不热更新。

## 测试计划

自动验收登记：[专属验收](../acceptance/run-usage-cost-ui.md)。所有测试使用隔离夹具；日志放仓库忽略的 `.evidence/run-usage-cost-ui/`，结果登记在专属验收，不在 Plan 维护。

| 验收项 | 级别 / 组 | 命令与时点 | 通过条件 / 证据 |
| --- | --- | --- | --- |
| U01 | L1 / quick-owner-runner-client | 实现后 `go test ./internal/runnerclient -run 'TestRunUsage\|TestStartupStops\|TestRunnerFullBindingAdmission' -count=1 -timeout=3m -v` | 身份/版本/金额错误被拒绝；原实例及完整绑定、指定 revision、只读命令、启动策略正文正确；runner-client.log |
| U02 | L1 / quick-owner-unified-startup | 实现后 `go test ./internal/app -run '^TestRunUsage\|^TestCM71ProductDependencyBoundary$\|^TestProjectLifecycleOwnershipBoundary$' -count=1 -timeout=3m -v` | 候选/目录错误拒绝；缺价表、估价错误不隐藏用量；caller-final.log |
| U03 | L1 / quick-owner-test-records-http | 实现后 `go test ./internal/product/httpapi -run '^TestRunUsageHTTPReadBoundary$' -count=1 -v` | 认证、精确目标、只读边界及转发路由正确；http-config.log |
| U04 | L1 / quick-owner-web-config | 实现后 `go test ./web/server/executionconfig -run '^TestRunUsageConfigPreservation$' -count=1 -v` | 两项部署配置经读取/保存保留；http-config.log |
| U05 | Web 组件 / 定向 | 实现后在 web 执行 `npm test -- src/features/runs/run-usage.test.tsx src/features/runs/runs.test.tsx src/features/runs/last-message.test.tsx src/shared/api/queries.test.ts` | 完整/部分/零/未知/缺价表、精度、轮询/停止/恢复、迟到取消/身份隔离、原页面回归通过；web.log |
| U06 | 构建 / 定向边界 | PR 前 `npm run build`、`go test ./cmd/test-feedback -run '^TestScoped\|^TestQuickOwnerPartitions\|^TestAssignTests' -count=1`、`git diff --check` | TS/生产资源构建、L1 测试归属与差异检查通过；build.log / scope.log |

不选择完整 L2/L3/L4/full：本次为只读查询和展示，采集/归一化/计价矩阵由 Runner 所有。调用方跨进程测试采用公开 CLI 子进程夹具；真实模型、真实 PR 执行及安装更新不是本次自动验收条件。需要手动查看时须按项目规则新建 debug 包，不使用安装现场。

人工项 M01：开发完成后可在有支持能力且已配置价表的新 debug 包检查窄屏和 details 交互；参与窗口由用户安排，可后置，不阻塞 PR。
