# 执行页用量与费用接入验收

归属：[实施 Plan](../plans/run-usage-cost-ui.plan.md)。用户确认方案日期：2026-10-22。测试范围为 SteerLine 消费公开 Runner 查询及 UI，不替代 Runner 原生采集资格。

| ID | 自动验收项 | 状态与证据 |
| --- | --- | --- |
| U01 | CLI 固定身份/快照/完整实例绑定、只读查询、金额状态及启动策略传递 | 通过；4 个顶层定向测试，`.evidence/run-usage-cost-ui/runner-client.log` |
| U02 | 调用方候选/目录绑定及费用故障隔离 | 通过；两个新增顶层测试及依赖/生命周期边界，`caller-final.log`；另 `caller.log` 保留共享服务 cleanup 回归通过 |
| U03 | HTTP 认证/目标/参数/只读及转发边界 | 通过；`TestRunUsageHTTPReadBoundary`，`http-config.log` |
| U04 | Web 保存配置保留价表和采集策略 | 通过；`TestRunUsageConfigPreservation`，`http-config.log` |
| U05 | 摘要/折叠、完整/部分/未知/零/缺价表、精度、刷新/取消/隔离 | 通过；4 文件 / 32 项组件和查询测试，`web.log` |
| U06 | 前端生产构建及定向 L1 测试归属检查 | 通过；`npm run build`，scope/owner/assignment 定向测试及 `git diff --cached --check`；`build.log` / `scope.log` |

M01 人工：支持的真实 Run + 部署价表下检查视觉、窄屏和折叠交互。待用户窗口，可后置，不阻塞 PR。

限制：仅 Runner 当前公开支持的主会话范围；codex-app-server、远端 Pi/DSH 不由本次 UI 接入扩展支持。无内置现价，不以合成价表作为供应商报价；远端读取依赖部署显式 usage_policy，不由页面查询授权。

运行日期：2026-10-22；源码为本 PR 提交前工作树，起点 `9385d3df`。Go 1.26.4 / Windows，Node 与 npm 按锁文件安装。上述短日志均在本工作树 `.evidence/run-usage-cost-ui/`，原始日志不纳入 Git。新增用例登记既有 `quick-owner-runner-client`、`quick-owner-unified-startup`、`quick-owner-test-records-http` 和 `quick-owner-web-config`。

首次验证修正了三处测试准备问题：组件夹具直接修改缓存对象、JSON 空白格式的字节比较，以及候选夹具缺少子进程模式/仓库材料准备。修正后按原断言重跑通过，没有将这些失败当作通过。候选测试通过真实隔离 SQLite/项目生命周期读取，CLI 为公开 DTO 子进程夹具；未调用真实模型、未创建测试 PR、未调试或更新安装现场。新服务采集策略通过 CLI 正文断言，远端目录授权/撤销机制仍由 Runner 专项验证。
