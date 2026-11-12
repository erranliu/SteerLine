# 活动中心视觉调整验收

日期 2026-11-12。改版前使用 `0b0e1bf5` 加任务执行页改动的生产构建，改版后在同一工作树加入活动中心改动。使用隔离 Edge 与模拟 API，工作树未部署。

- 活动列表、摘要和对话沿用任务执行页的标题、按钮和正文字号。选中项、当前阶段和待处理操作各有明确层级。
- 摘要与展开对话两组图使用相同的模拟数据、时间和 1440×1050 视口。验证摘要进入时不读取对话，展开后读取原会话，在 800、375、320 像素宽度无页面横向溢出。窄屏返回恢复列表焦点，需处理与历史筛选可用。
- `npm --prefix web run build` 通过。`npm --prefix web test -- src/features/activities` 通过 6 个文件、45 项。
- Edge 基线对照 1 项通过。新版对照与既有 B4 路由及活动入口回归共 2 项通过。原证据在 `C:/SteerlineDebug/wt-82d4-activity-visual/diagnostics/` 的 `browser-before-3aa87f8721894113bb863dc33e8a492c` 和 `browser-after-95e470689b7d4de7bea040a7115d13e3`。
- 复用[任务执行页验收](task-execution-visual-refresh.md)的 L1、L2 通过记录。本次改动涉及独立活动中心页面、对话的展示和 CSS，L1 的固定前端集及 L2 的受控交付冒烟涉及的实现、依赖与定义未变化。
