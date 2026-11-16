# 项目状态页的操作回执

2026-11-13，基线 `c7b85806` 加当前工作区改动。页底操作回执集中到「项目状态」。原请求的后台核实继续运行，用户可在项目状态页查看历史、查询原请求和复制执行信息。

| 验证 | 结果及证据 |
| --- | --- |
| `npm --prefix web run build` | TypeScript 与生产构建通过。 |
| `npm --prefix web test -- --maxWorkers=2 src/app/app.test.tsx src/app/operations.test.tsx src/app/project-switch.test.tsx src/features/runs/runs.test.tsx` | 四个文件共 51 项通过，覆盖页面入口、原请求核实、项目隔离和候选操作。 |
| `npm --prefix web run test:e2e -- configuration-navigation.spec.ts -g 'runtime environments keep row actions'` | Edge 一项通过。运行环境的回执可在项目状态页展开并复制，切回运行环境或任务执行后无页底回执。截图与 trace 位于 `C:/Users/lmas1/AppData/Local/Temp/steerline-receipts-browser-c38fc74cdd0c4ae7b79ad8814ef097d2/`。 |
| `go run ./cmd/test-feedback -level=L1 -out <报告目录>` | 通过。`C:/Users/lmas1/AppData/Local/Temp/steerline-receipts-l1-da05bf23817046fd929757fa03c4c430/summary.json`。 |
| `go run ./cmd/test-feedback -level=L2 -out <报告目录>` | 通过。`C:/Users/lmas1/AppData/Local/Temp/steerline-receipts-l2-1231b9b5168f460dad86977ae2b850a8/summary.json`。 |

首次在 Windows 沙箱内运行时，Edge 启动与正式报告重命名受阻。上述结果来自独立临时目录的复验。独立运行包的 Web 浏览器冒烟和安装部署未执行。
