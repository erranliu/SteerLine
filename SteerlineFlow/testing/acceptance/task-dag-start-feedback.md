# 任务 DAG 启动反馈

2026-11-11，普通开发。受测版本为 `eefb67dc` 加本次工作区改动，尚未部署。

| 验收行为 | 结果 |
| --- | --- |
| 延迟启动回执时，确认后留在 DAG，列表和任务图立即显示「启动中」 | 通过，组件测试和隔离 Edge |
| 已应用回执但任务快照尚未更新时继续显示「启动中」。服务端状态进入 `in_progress` 后切换为「执行中」 | 通过，组件测试和隔离 Edge |
| 从本地请求记录恢复最近一次首次启动，结果不明确时显示「启动结果待确认」，明确拒绝时显示原任务状态和「启动失败」 | 通过，组件测试 |
| 蓝色圆环、紫色呼吸点和静止的待执行徽标可区分。减少动态效果时，图标停止动画且保留颜色和文字 | 通过，隔离 Edge |

- `npm --prefix web test -- src/features/tasks src/app/use-candidate-starts.test.tsx src/features/runs/candidate-starts.test.tsx` 通过 50 项。`npm --prefix web run build` 通过。
- Edge 在本地隔离夹具中渲染实际 DAG 组件，使用延迟回执和受控任务快照。截图位于 `C:\Users\lmas1\AppData\Local\Temp\steerline-dag-status-S3p5uK` 下的 `graph.png` 和 `list-reduced-motion.png`。
- `go run ./cmd/test-feedback -level=L1 -out <独立目录>` 通过，报告位于 `C:\SteerlineDebug\wt-fa1a-dag-start-feedback\test-results\L1\6de9ee28-d109-4f22-8fa3-9ce494947aa2\summary.json`。
- `go run ./cmd/test-feedback -level=L2 -out <独立目录>` 通过，报告位于 `C:\SteerlineDebug\wt-fa1a-dag-start-feedback\test-results\L2\afe28fac-883c-4698-9b6f-d018aaa08225\summary.json`。L1 和 L2 的首次沙箱运行均出现报告写入权限问题，以上报告来自独立目录复跑。
- `npm --prefix web test` 共 471 项，458 项通过、13 项失败，另有 6 个未捕获错误。将项目切换和执行页两组测试在未修改的 `eefb67dc` 和当前工作区分别执行，13 个失败用例逐项一致，两边均有 6 个未捕获错误。对照日志位于 `C:\SteerlineDebug\wt-fa1a-dag-start-feedback\diagnostics\96776577-abe4-4407-9306-505aee8333f3` 下的 `vitest-head-compare.log` 和 `vitest-worktree-compare.log`。
