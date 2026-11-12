# 任务派发设定与任务执行导航

2026-11-12，在 `805cfcae` 的工作区改动上验证。任务派发设定只在任务 DAG 和任务执行显示，任务执行名称同时用于侧栏、面包屑和页标题。

原始报告位于 `C:/Users/lmas1/.codex/visualizations/2026/11/12/01a1cde2-4e66-7033-b311-432c1a1151bc/`。

- `go run ./cmd/test-feedback -level=L1` 和 `go run ./cmd/test-feedback -level=L2` 均通过，报告分别在 `dispatch-l1-240138c7/summary.json` 和 `dispatch-l2-240138c7/summary.json`。
- `npm --prefix web run build` 通过，见 `dispatch-build-final.log`。
- 定向 Vitest 覆盖应用路由、派发控件、DAG、任务执行和环境页，共 57 项通过。环境页错误提示和 StrictMode 活动查询各有一项失败，使用改动前的应用代码和测试复现了相同错误，见 `dispatch-vitest.log`。
- Edge 使用模拟 API 在 1440 和 780 像素宽度下检查了 DAG、任务执行、配置和返回 DAG，验证入口显隐、自动派发开关和新页名，见 `dispatch-browser.json`。涉及的 Playwright 场景已通过收集检查，完整运行现场未复跑。

## 任务执行筛选

等待前置交付的未启动任务从任务执行的列表和计数中排除，依赖解除后重新显示。启动中和排队中的候选计入「运行中」，详情继续显示具体状态。回执结果待确认的候选保留在「全部」的计数中，执行失败或阻塞的候选仍可在「需处理」定位。

- 执行页、任务行、候选状态和启动反馈的定向 Vitest 共 41 项通过，见 `execution-filter-vitest-final.log`。
- `npm --prefix web run build` 通过，见 `execution-filter-build-final.log`。`go run ./cmd/test-feedback -level=L1` 和 `go run ./cmd/test-feedback -level=L2` 均通过，报告分别在 `execution-filter-l1-0915a203/summary.json` 和 `execution-filter-l2-0915a203/summary.json`。
- Edge 使用模拟 API 在 1440 和 780 像素宽度下验证阻塞任务、列表计数、排队候选及筛选命中，见 `execution-filter-browser.json`。`web.query-cache` 已通过场景收集检查，未运行完整双项目现场。
