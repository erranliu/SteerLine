# 重构任务调试信息

2026-11-17，受测版本为 `001ecb7623e1` 加本次前端与测试差异。

重构与维护的列表、新建表单和详情统一使用“重构任务”。每个列表项和详情顶部均提供“复制调试信息”。在线时，列表按钮在点击时读取该任务的详情，复制完整 ID、状态、运行环境、起始提交、已有诊断以及各轮操作与 Runner Run ID，页面保留原来的任务选择。离线时使用已缓存的详情或列表快照。剪贴板不可用时，页面显示可手动复制的文本。

| 执行 | 结果与证据 |
| --- | --- |
| `npm --prefix web test -- src/features/refactoring-maintenance/refactoring-maintenance.test.tsx src/features/activities/execution-info.test.tsx src/features/activities/activities.test.tsx src/features/runs/copy-execution-info.test.tsx src/features/activities/operation-result.test.tsx` | 五个文件共 40 项通过，覆盖详情复制、从列表按需读取历史任务、复制时保留当前选择、读取失败后的重试和剪贴板失败后的手动复制。 |
| `npm --prefix web run build` | 类型检查和生产构建通过。 |
| `go run ./cmd/test-feedback -level=L1 -out <目录>` | 复测通过，报告位于下述目录的 `l1-refactoring-list-copy-recheck-997be16b342b44b2ba72b830f36c94b8/summary.json`。 |
| `go run ./cmd/test-feedback -level=L2 -out <目录>` | 复用 `001ecb7623e1` 加详情复制差异时通过的受控核心交付冒烟，新增列表入口由后续前端与 L3 验证覆盖。原报告位于 `C:/Users/lmas1/AppData/Local/Temp/steerline-refactor-debug-L2-345a05f5e6cf47508379f55993e4aca8/summary.json`。 |
| `go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled -scenario runtime.refactoring-maintenance -out <目录>` | 受控场景通过。Edge 在 375 像素视口实际从列表复制任务 ID 和 Runner Run ID，详情中的按钮仍可见。报告位于下述目录的 `l3-refactoring-list-copy-95c01a37388c45d786dcc689c975e384/summary.json`。 |

本轮报告根目录为 `C:/Users/lmas1/.codex/visualizations/2026/11/17/01a1e7a8-1187-7b60-b123-7b23bf6a7b1a`。首次 L1 的 Windows 依赖扫描在 60 秒上限处退出，原报告在 `l1-refactoring-list-copy-9255651e18c74cbdaa1b5c7e9eb668af/summary.json`。复测使用本机 Go 缓存和新的独立报告目录。
