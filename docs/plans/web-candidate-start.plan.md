# Web 候选即时反馈与连续创建

用户直接授权开发，不纳入 roadmap。参考历史 [TUI 方案](../spec/candidate-start-retention.plan.md)，本次验收独立登记在[专属验收](../acceptance/web-candidate-start.md)。

## 实施范围

1. 确认方案后立即生成请求 ID、显示并选择提交卡片，关闭选择面板。正式编号和执行信息由服务器提供；原请求对应的正式候选原位接替卡片，迟到回执不改变选择。
2. 用一个 Web hook 合并本页确认与既有请求日志；首次领取之前的等待选择仅存当前 Workspace 内存。已发送请求继续使用原 IndexedDB 日志和对账，页面重开不重建或自动发送未发出的选择。
3. 仅新增并行候选允许同目标独立请求并发；原请求锁、完整信封先保存、其他动作的目标锁与未知保护保持。准备及未知候选占用参与名额，前端计数去重并限制五个，后端事务上限不变。
4. 首次领取未核实时继续接收选择、显示等待卡片。原首次请求 applied 后读取实际任务；只接受原候选存在、任务为 in_progress、版本恰为领取前 +1、图版本不变的结果，再构造后续信封。拒绝或已证实未发送结束等待项，未知保持等待。项目或实例切换丢弃未发送等待项，不把请求转到新实例。

本次只修改客户端提交反馈，不修改 Runner 或后台准备/恢复流程。[worker 统一规则](../product-design/worker-lifecycle.md)的 W01/W08、E04/E07、V 输入与身份边界保持；无新重试、替代执行或跨服务换代发送授权，不改节点/异常/验证矩阵。

## 测试计划

全部自动验收项归属本次 [WCS-A1～WCS-A4](../acceptance/web-candidate-start.md)。使用现有 Vitest/jsdom、fake-indexeddb、受控 HTTP/Promise 与 TanStack Query 夹具，不启动安装环境或调试服务。

| 验收项 / 级别 | 场景与命令 | 时点 / 通过条件 / 证据 |
| --- | --- | --- |
| WCS-A1～A4 / L1 定向 Web | `npm --prefix web test -- src/app/use-candidate-starts.test.tsx src/features/runs/candidate-starts.test.tsx src/shared/api/client.test.ts src/features/tasks/task-detail.test.tsx src/features/runs/runs.test.tsx src/app/app.test.tsx src/app/project-switch.test.tsx --maxWorkers=2` | 实现后；全部非零通过。延迟响应时五张卡片即时可选、首次版本衔接正确、独立发送且原请求去重、未知/拒绝分开、切换与迟到回执隔离。JSON 报告路径登记专属验收。 |
| WCS-A1～A4 / 构建 | `npm --prefix web run build`；`git diff --check` | 完成后；TypeScript、生产构建及差异检查通过，日志路径登记专属验收。 |

不选择 Full、L2/L3 或真实 PR：本次不改变后台启动、模型、原生传输或交付，延迟/乱序/失败和路由选择由定向客户端集成夹具验证。WCS-M1 为可后置的真实浏览器人工体验复验；如安排服务调试，须按项目规则生成新 SteerlineTest 调试包。
