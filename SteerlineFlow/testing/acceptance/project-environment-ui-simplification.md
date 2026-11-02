# 项目运行环境页面精简

2026-11-02。用户确认按界面建议直接实施并提 PR；验证基于 `75c85a82005f326a1bed10e1839eaa9a5fe6f80f` 的 dirty 工作树，未部署。证据根为 `C:/SteerlineDebug/wt-621e-environment-ui/`。

## 最终行为

页面集中展示运行机/链路选择、一次当前状态及原因、检测时间、检测 ID 和开始/重新/停止检测按钮。单方案自动采用，多方案保留检测方案选择；同机同链路的多个环境选项附上方案可读名称以便区分。准备要求、更多设置和检测历史默认折叠；材料与历史仅展开后查询。准备文档展示不含机器调用用的 manifest 块。

移除环境键、方案 ID、Run ID、请求 ID、实例 ID、提交 SHA、轮次、原始结果文件、Agent 回复和协议说明的专用展示；检测 ID 可完整复制。项目维护、派发栏及跳回本页的全局环境摘要在此页隐藏，其他分页保持原入口。

本页操作回执只显示最近操作及尚未确认操作的简短结果；未知结果保留“重新查询”，沿同一原请求核实。后台记录、浏览器请求日志、自动核实、检测准入、原活动停止和结果结算继续沿原逻辑。历史列表只增加既存记录的 created_at 投影，不改持久数据。

## 验证与证据

所有正式报告保留原命令、dirty 源码状态、实际选择及终态。下表路径相对证据根。

| 检查 | 结果 | 证据 |
| --- | --- | --- |
| TypeScript / Vite | 构建通过 | 本次 `npm --prefix web run build` 输出 |
| web.unit | 35 文件、258 项全部通过，零失败/跳过 | `test-results/L4/20261102T102800Z-web-final/summary.json` |
| 固定 L1 | complete=true / passed=true，执行 18.4 秒 | `test-results/L1/20261102T102800Z-final/summary.json` |
| 固定 L2 | passed=true / cleanup_complete=true，执行 182.4 秒 | `test-results/L2/20261102T102800Z-final/summary.json` |
| go.http | complete=true / passed=true，执行 31.0 秒 | `test-results/L4/20261102T102100Z-http/summary.json` |
| go.environment | 未完成：internal/app 触及固定 25 分钟上限；314 项通过、零断言失败/跳过 | `test-results/L4/20261102T101946593Z-environment/summary.json` |
| 配置导航浏览器 | 3 场景通过，12.4 秒；环境六状态、任务往返、观察故障恢复、简短回执、ID 不可见及 1440/375 宽度检查通过 | `diagnostics/browser-20261102T102800Z-final/` |

已查看成功轮的 runtime-environments-1440.png 和 runtime-environments-375.png。浏览器第一轮 `diagnostics/browser-20261102T102100Z/` 为 2 通过 / 1 失败：测试以“运行环境”模糊匹配多个标签，选项计数误含检测方案选择器；改为精确标签后独立重跑通过，原失败截图和 trace 保留。此前的完整 web.unit（257 项）、L1/L2 及浏览器成功轮分别保留在 `test-results/L4/20261102T102300Z-web/`、`test-results/L1/20261102T101915745Z-L1/`、`test-results/L2/20261102T101931706Z-L2/` 与 `diagnostics/browser-20261102T102500Z/`；最终增加同机同链路可读区分及回归断言后，前端、L1/L2、浏览器使用独立目录补验。定向组件测试初轮修正了新增夹具的异步观察等待与 mock 委托，最终上述 web.unit 完整通过，不改业务校验条件。

完整 go.environment 的 `complete=false / passed=false`，原始报告与超时栈保留；未放宽时限或重写结果。超时时正在执行 `TestCallerPromptConvergencePinnedInputsAndWriter/crlf`，这是包级累计时限，不能据此判断该用例自身超时或把未运行部分算作通过。三个环境模型/fixture 包已整体通过，`TestEnvironmentHistoryFiltersBeforeTwentyAndKeepsRemovedTarget`（含本次 created_at 断言）已通过；完整 internal/app 仍未取得通过结论。

未运行真实模型、远端 Orca 或真实项目检测，不将受控 UI 验证计为环境就绪。没有固定提示词及其调用接线、规则资产、CI、持久 schema 或检测协议变更；安装配置及服务未修改。

文档链接检查：654 个 Markdown、4069 个本地导航链接、零错误；`git diff --check` 通过。

提 PR 前无冲突 rebase 到 `cf934675`（新增开发者配置边界检查，业务代码未变），补跑固定 L1：complete=true / passed=true，执行 41.0 秒，证据 `test-results/L1/20261102T104808133Z-pr/summary.json`。此前各结果仍沿原受测基线与证据，不将其改写为 rebase 后重验。
