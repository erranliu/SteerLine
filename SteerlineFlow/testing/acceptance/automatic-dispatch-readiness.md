# 自动派发开关与就绪状态

2026-11-12，基于主干 `57d02964` 验证工作区中的本次改动。项目或默认环境未就绪时可保存自动派发模式。设置页和顶栏消费 [派发状态 API](../../../docs/product-design/local-client-protocol.md)，设置页展示后端等待原因及对应入口。状态绑定当前项目实例和派发版本。

候选仍由原派发循环创建。候选预留事务继续复核模式、任务、材料和环境资格。关闭等待中的自动派发后，项目就绪也不会触发自动派发。

## 验证

| 范围 | 结果与证据 |
| --- | --- |
| Go 定向派发、事务和路由 | `go test ./internal/storage/sqlite ./internal/product/applicationhttp -run 'Test(AutomaticDispatchWaitsForReadinessAndRespectsClose\|RunnerDispatchReceiptAndAuthorityCommitTogether\|UnifiedStartupAuthenticationRoutesAndUnavailable)$' -count=1` 通过。覆盖未就绪开启零候选、就绪后原 actor 创建候选、等待期间关闭后零候选及只读路由。 |
| Web 定向组件与查询 | `npm --prefix web test -- --maxWorkers=2 src/features/tasks/dispatch.test.tsx src/app/app.test.tsx src/app/project-switch.test.tsx src/shared/api/queries.test.ts` 通过，58 项。覆盖开关与状态独立、项目和环境入口、版本隔离和跨项目状态。 |
| Web 生产构建 | `npm --prefix web run build` 通过。 |
| L1 `go run ./cmd/test-feedback -level=L1` | 通过。报告为 `%TEMP%/steerline-dispatch-L1-b259c91027284ae9993607e4a2dc4b63/summary.json`。 |
| L2 `go run ./cmd/test-feedback -level=L2` | 通过。报告为 `%TEMP%/steerline-dispatch-L2-c22038aa12b74c7491b4b2f06e41477e/summary.json`。 |
| HTTP 专项 `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.http` | 通过。报告为 `%TEMP%/steerline-dispatch-http-26d9287689c14b8e98ef7311a67962e5/summary.json`。 |
| Web 全集和增量补验 | 首轮 `npm --prefix web test -- --maxWorkers=2` 中 489 项通过，1 项仍使用旧的派发环境夹具。将该用例改为核对派发状态 API 和查询失败后的待核实状态后，定向复跑 `src/features/environment-checks/project-environment.test.tsx` 的 30 项全部通过，生产构建再次通过。 |

未执行真实模型、浏览器现场验收或安装部署。
