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

## 项目顶栏自动派发状态

2026-11-12，在 `af6a484f` 加本次工作区改动上验证。项目内的顶栏显示关闭、默认执行方案名称或待核实状态，点击进入当前项目配置并定位到任务自动派发设置。开关、派发阻断原因和处理入口在同一个设置区。

报告目录为 `C:/Users/lmas1/.codex/visualizations/2026/11/12/01a1ce05-299f-75b3-8832-0cb110dbcfa3/`。

- `go run ./cmd/test-feedback -level=L1` 和 `go run ./cmd/test-feedback -level=L2` 在本机隔离目录通过，见 `dispatch-l1-local/summary.json` 和 `dispatch-l2-local/summary.json`。沙箱首轮的临时文件权限和 Windows 边界失败保留在 `dispatch-l1-b16459f0/summary.json` 和 `dispatch-l2-b025ceb7/summary.json`。
- 最终前端构建和 5 项定向 Vitest 通过，覆盖导航、离线、项目切换及开关原有的命令门槛，见 `dispatch-build-final.log` 和 `dispatch-targeted-final.json`。较宽的首轮 Vitest 还复现了上述历史记录中的环境页错误提示和 StrictMode 活动查询失败，见 `dispatch-vitest.json`。
- Edge 使用模拟 API 在 1440、780 和 390 像素宽度下验证顶栏状态、设置区定位、项目切换以及导航期间零写请求。390 像素场景另外验证环境查询失败时仍显示已读到的默认执行方案，见 `dispatch-browser-final.log`。更新的 6 个 Playwright 用例通过收集，见 `dispatch-playwright-list.log`。完整 Web E2E 未运行。
