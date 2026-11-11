# 项目运行环境表内检测

2026-11-11，基于 `eefb67dc0445b608024a845c102fdfddb430304c` 及本次工作树差异。

运行环境表格每行提供检测方案和开始、重新检测或停止按钮。单方案直接显示，多方案按环境记忆选择；从任务进入会定位到对应行并带入任务方案。原请求待核实时显示该行提交反馈并禁止重复提交，停止操作使用该行的当前检测 ID 和版本。

准备要求直接显示一次，隐藏机器协议段。检测历史默认折叠，打开后先展示项目范围，可按环境筛选并查看指定检测结果。移除更多设置中的超时输入和手动标记失效入口，提交仍带原默认值 3600 秒。变更涉及 Web 界面及测试夹具。

## 验证

报告根目录为 `C:/SteerlineDebug/wt-8512-environment-table`。

| 执行 | 结果与证据 |
| --- | --- |
| `npm --prefix web run build` | 通过，TypeScript 检查和 Vite 生产构建完成。 |
| `npm --prefix web test -- src/features/environment-checks/project-environment.test.tsx` | 29 项通过，覆盖方案选择、检测命令和停止目标、任务定位、状态更新、原请求反馈、准备要求和历史范围。 |
| `go run ./cmd/test-feedback -level=L1 -out <目录>` | 通过，`test-results/L1/20261111T100842Z-3b5f44b6-4937-4267-a792-80bd6d459991/summary.json`。随后调整的窄屏样式、运行中操作状态和浏览器夹具由定向检查覆盖。 |
| `go run ./cmd/test-feedback -level=L2 -out <目录>` | 受控交付冒烟通过，清理完成。报告 `test-results/L2/20261111T100842Z-3b5f44b6-4937-4267-a792-80bd6d459991/summary.json`。 |
| `npm --prefix web run test:e2e -- environment-overview.spec.ts configuration-navigation.spec.ts --global-timeout=125000` | Edge 5 项通过。表格覆盖 320、375、390、800、960、1440 像素视口，核对任务往返、状态刷新和错误恢复。截图与追踪位于 `diagnostics/browser-72a3a22b-66ea-46ba-81e5-9a66742f0fcc`。 |
| `go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled -scenario runtime.async-activities -out <目录>` | 场景未通过，清理完成。活动观察阶段通过，显式反馈阶段在进入环境页前等待第二次候选提交超时。报告 `test-results/L3/20261111T100842Z-3b5f44b6-4937-4267-a792-80bd6d459991/summary.json`。 |
| `npm --prefix web run test:e2e -- environment-overview.spec.ts --global-timeout=95000` | Edge 定向补验通过。在 320 像素视口点击检测，核对目标、方案、超时与版本、等待回执期间的本行反馈及收到拒绝后恢复操作。追踪位于 `diagnostics/browser-b4d47669-c8fc-4a79-b441-056c44a7fe27`。 |

首次在沙箱中运行 L1、L2 和 Edge 时遇到报告写入失败、清理未核实或浏览器启动失败。沙箱外首次 Edge 运行确认窄屏行标题需要调整，另发现配置导航用例缺少环境查询夹具。活动反馈浏览器用例由 `runtime.async-activities` 提供隔离项目和服务，直接调用会因缺少这些输入停止。

定向补验的首次回执夹具缺少 `original` 信封，界面保留待核实状态。补齐夹具后通过，原追踪位于 `diagnostics/browser-9e547f9e-9fb6-4a6f-afb6-767126607ef0`。真实 Runner 和部署未执行。
