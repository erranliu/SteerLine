# 项目维护一键生成与全局队列验收

归属：[实施 Plan](../../plans/generate-queue.plan.md)。

| ID | 自动验收项 | 结果 |
| --- | --- | --- |
| GQ-A01 | 同 home 跨项目统一上限、FIFO、名额覆盖执行与未知直至核实停止 | L1 定向通过 |
| GQ-A02 | 自动/单项/批量统一入口，已就绪跳过、活动复用、明确失败安全重试，固定批次进度 | L1 定向通过 |
| GQ-A03 | 排队持久化、取消零调用、启动恢复占位、未知零重发、停止/换代识别未提交记录 | L1 定向通过 |
| GQ-A04 | 默认五、正整数校验、全局配置保存重读，重启生效 | L1 定向通过 |
| GQ-A05 | 折叠后标题仍显示进度；成功五秒隐藏；失败/未知保留；页面恢复沿原显示窗口，后端完成时间排除历史提示 | L1 定向通过 |

人工 GQ-M01：真实浏览器动画与多页切换体验，窗口待定，可后置，不阻塞自动验证交付。真实模型吞吐不在本轮最小验证范围。

## 2026-10-22 实施与验证结果

功能已实现，未操作安装或保留运行环境。数据库当前格式为 29，新增批次字段；不提供旧库迁移、兼容读取或回退。

- Generation 持久 outbox 区分 pending 与 started；共享队列覆盖整个 Run，未知保留名额。启动 census 恢复所有项目的活动与等待条目；项目退出仅移除未提交准入。换构建的未提交记录明确拒绝，显式重试才使用新构建。
- 一键生成跳过就绪内容、合并活动批次、复用同输入生成对象，并只重试已核实停止的失败。批次成员、完成时间和显式命令本地处理回执持久保存；原请求对账不会重新生成，丢失外层回执不重置完成显示窗口。
- 折叠标题显示后端汇总。首次观察完成后显示完整五秒，浏览器 sessionStorage 仅保存展示窗口；刷新不重新计时，历史完成不会反复提示。远端检查失败也能记录既有生成的完成时间。
- 全局 `max_generate_concurrency` 默认五，显式值为正整数；保存重读、待重启提示及现有启动冻结路径通过。

| 实际命令/范围 | 结果与证据 |
| --- | --- |
| `go test ./internal/testmanagement ./internal/storage/sqlite ./internal/app -run 'Test(TMGeneration\|GenerationQueue\|GQ\|PMSGeneration)' -count=1 -timeout=5m -v` | 三包通过；[生成与存储回归](evidence/generate-queue-20261022/generation.txt) |
| 最终补齐维护回执后，`go test ./internal/app -run '^TestGQ' -count=1 -timeout=2m -v` | 四项通过；[最终批次验证](evidence/generate-queue-20261022/batch-final.txt) |
| 配置包定向测试及 `TestQuickOwnerPartitionsRejectMissingAndReplayedGroups` | 三包通过；[配置及 L1 登记](evidence/generate-queue-20261022/config-and-registration.txt)。另完整运行过 `go test ./internal/executionconfig ./web/server/executionconfig -count=1`，两包通过 |
| web 下 `npm test -- src/features/project-maintenance/project-maintenance.test.tsx src/features/execution-configs/execution-configs.test.tsx` | 20/20 通过；[组件结果](evidence/generate-queue-20261022/web-tests.txt) |
| web 下 `npm run build` | TypeScript 与 Vite 通过；[构建结果](evidence/generate-queue-20261022/web-build.txt) |
| 架构边界：`TestWebExecutionConfigOwnership`、`TestTM07StorageBoundary`、`TestTM08AppHostBoundary`、`TestCallerOwnerBoundary`、`TestTakeoverContractOwnership` | `internal/testkit/boundary` 定向通过；未运行全量测试 |
| `git diff --check` | 通过 |

初轮问题保留：存储夹具缺少请求正文，补上有效 JSON 后同组通过；[首轮批次日志](evidence/generate-queue-20261022/generation-initial.txt)中的“已完成”夹具遗漏 stopped，被正常清理为取消，补齐原资源终态后通过。新增 L1 owner 后原分区测试硬编码数量同步为四组，登记检查通过。这些夹具/登记失败未记为产品通过；最终结果以上表为准。

本轮仅 L1 受控调用方验证与前端构建，不宣称真实模型并行吞吐、真实 Runner 多项目执行或人工浏览器体验已验收。GQ-M01 保持待验，可后置。
