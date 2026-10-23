# 项目维护一键生成与全局 generate 队列

用户已确认：保留自动生成；完成提示五秒；home 级并发默认五，执行配置重启生效。普通开发，不进入 roadmap。

## 实施

1. 复用 Generation 持久记录，明确区分未提交排队与已调用未知。应用宿主持有共享队列，启动前恢复全部项目的活动/未知占位。按入队时间调度，名额覆盖整个执行，核实停止后释放。
2. 自动维护、单项重试与批量生成共用生成服务。排队取消零 Runner 调用，已发送未知只对账原请求。复用维护命令登记，持久化批次引用及完成时间。
3. 维护标题始终显示动画/进度；首次观察全成功后显示五秒（轮询延迟不扣减，同一浏览器刷新不重置显示窗口；旧完成时间不重新提示），失败/未知保留提示。分母固定，原候选材料不变。
4. 唯一 execution.json 增加正整数并发配置，缺省五，重启后应用。
5. 同步 [Worker 生命周期](../../docs/product-design/worker-lifecycle.md) W01/W08–W14/W18、E04/E07、V03/V06–V08 的调用方增量，不建立另一套 Runner 执行状态机。

## 测试计划

验收项见 [专属验收](../testing/acceptance/generate-queue.md)。全部为 L1，使用隔离临时目录、公开 Runner fixture 和 Vitest/jsdom，不操作安装目录。

| 验收项 | 组/入口 | 时点与通过条件 | 证据 |
| --- | --- | --- | --- |
| GQ-A01–A03 | `go test ./internal/testmanagement ./internal/storage/sqlite ./internal/app -run 'Test(TMGeneration|GenerationQueue|GQ|PMSGeneration)' -count=1` | 后端完成后；跨项目上限/FIFO、未知零重发、排队取消/恢复、批量去重与持久进度通过 | 专属验收执行记录 |
| GQ-A04 | `go test ./internal/executionconfig ./web/server/executionconfig -count=1` | 配置完成后；默认/正整数/保存重读通过，运行时沿启动冻结值 | 同上 |
| GQ-A02、A05 | web 下 `npm test -- src/features/project-maintenance/project-maintenance.test.tsx src/features/execution-configs/execution-configs.test.tsx` 及 `npm run build` | 完成后；批量命令、折叠进度、五秒隐藏、失败保留、配置保存及类型构建通过 | 同上 |

不选 L2/L3/L4/full：本轮风险是调用方调度、持久化窗口和 UI，由定向集成覆盖；不改 Runner 原生执行或 PR 流程。若发现跨进程接线缺口，再补最小场景。人工视觉体验可后置，不以组件测试冒充真实模型验收。
