# 配置二级导航与独立计费页

2026-11-13，基线 `ae0ca5ef` 加当前 worktree 修改。侧栏只提供一个「配置」入口。项目设置默认打开项目状态，含运行环境、测试管理和自动派发。应用级的通用设置、执行方案管理和计费由所有项目共享，无项目时也可访问。计费在 `/settings/billing` 展示模型价表及采集设置，执行详情的费用链接、项目就绪入口及自动派发入口直接到达各自页面。

报告根目录为 `C:/SteerlineDebug/wt-1c77-configuration/`。

| 验证 | 结果及报告位置 |
| --- | --- |
| `npm --prefix web run build` | TypeScript 和生产构建通过。 |
| `go run ./cmd/test-feedback -level=L1 -out <报告目录>` | 通过，`test-results/L1/fe11aec13e8a489ca25f8dea71c339ca/summary.json`。 |
| `go run ./cmd/test-feedback -level=L2 -out <报告目录>` | 通过，`test-results/L2/231f820a8582457ea83f2ed7cad97d3b/summary.json`。 |
| `go run ./cmd/test-feedback -out <报告目录> -- ./web/server -run '^(TestWebOriginHostAndStaticBoundaries\|TestWebExecutionConfigRoutes\|TestBillingConfigIsIndependentOfRuntimeAndRequiresCSRF)$' -count=1` | 三项通过，`diagnostics/http-177a4e32c3a84b86bd4198407bbdc8fe/summary.json`。 |
| `npm --prefix web test -- --maxWorkers=2 src/app/app.test.tsx src/app/project-switch.test.tsx src/features/settings/billing-settings.test.tsx src/features/runs/run-usage.test.tsx` | 43 项通过。全量首跑 489 通过、1 项仍期待旧项目设置里的生成并发；改用新页面职责断言并定向复验。 |
| `npm --prefix web run test:e2e -- configuration-navigation.spec.ts ui-simplification.spec.ts` | Edge 九项通过。检查快捷入口、原页面和查询/片段返回、刷新、草稿保护、无项目配置以及 1440、900 和 375 像素视口。截图和 trace 位于 `diagnostics/browser-1a79f09c5e724d3782b7be2723f1dd86/`。 |

首轮 Edge 在 Windows 沙箱内未启动，后续在独立目录运行。首轮可启动的浏览器验证中，任务夹具缺少当前准入字段，导航夹具打开了缺少图数据的 DAG 页。修正夹具后两个场景通过，原失败留在 `diagnostics/browser-3337f2353c51440c840a26bf24547d81/`。

浏览器测试使用受控接口与合成费率。独立运行包的计费浏览器冒烟及安装部署未执行。
