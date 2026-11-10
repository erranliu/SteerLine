# 异步操作执行信息与页面关联

2026-11-09。按用户确认的精简方案实施；先从 `4e106c6c` rebase 到 `origin/main` 的 `5fdf1bd3`，无冲突。分支 `codex/async-execution-info`。本次未部署，安装现场未修改。

## 实现范围

- 共用一个“复制执行信息”按钮及格式：操作名称、原请求、关联记录、Task、Runner Run/Attempt ID，附简短说明；相同 ID 合并。已知未创建与状态待核实分别表达，剪贴板失败就地提供原正文。
- 任务页合并原有两个复制入口；候选未确认、Roadmap 首次提交和环境提交待核实时也可复制原请求。
- 活动中心接入只读 `execution_ref`，由各来源的原持久绑定投影。Roadmap 后续发布/入图/恢复使用对应原命令 ID，不借用生成 Run。
- Roadmap 当前/历史结果独立标注，直接展示具体澄清正文与诊断；复制使用选中记录。环境总览布局保留，检测详情和历史复制原检测绑定，历史增加精确详情入口。
- 活动与操作回执跳回对应业务页面；发布等操作携带 `request_id`，按原回执校验项目和业务关联，展示指定操作结果。后台查询和复制无业务副作用。
- 未增加通用执行状态表、日志阅读器、历史搜索、配置/输入正文复制。提示词及其接线、资产、黄金样本均未修改。

## 验证记录

证据根：`C:/SteerlineDebug/wt-195b-execution-info/`。各次目录独立保留原始结果；基线提交加本次工作树修改为受测来源。

| 检查 | 结果 | 相对证据路径 |
| --- | --- | --- |
| TypeScript / Vite 生产构建 | 通过 | `npm --prefix web run build` 终端输出 |
| L1 最小集 | 通过，complete=true | `test-results/L1/last/summary.json`（初轮 `run-01` 亦通过） |
| L2 核心交付冒烟 | 通过，cleanup_complete=true | `test-results/L2/run-01/summary.json` |
| Web 全集 | 391 项通过，零失败/跳过，complete=true | `test-results/L4/web-last/summary.json`、`web-unit/vitest.json` |
| Go 定向 | 通过；含公开绑定投影、三来源原 Run/Attempt 及私有绑定不泄露；初始 Roadmap 原操作关联亦通过 | `diagnostics/references-final/summary.json`、`diagnostics/initial/summary.json` |
| 生产前端 Edge fixture | 通过；当前/历史复制、原发布请求精确跳转、1440/390/320px、零业务写入及页面错误 | `diagnostics/browser-complete/` 截图及 trace（包含环境总览回归，2 项通过）；入口 `web/tests/execution-info.spec.ts` |
| 存储专项 | 通过，complete=true | `test-results/L4/go.storage-01/summary.json` |
| 环境专项 | 通过，complete=true | `test-results/L4/go.environment-01/summary.json` |
| Roadmap 专项 | 通过，complete=true | `test-results/L4/go.roadmap-01/summary.json` |
| HTTP 专项 | 通过，complete=true | `test-results/L4/go.http-01/summary.json` |

已查看 1440px 和 320px 截图，复制按钮和具体问题可见。浏览器使用隔离静态服务及 API fixture，不能代表真实模型、Runner 或安装现场已验。新场景是专项诊断，不冒充已登记业务 E2E。

首轮 Web 全集 `web-01` 为 384 通过、1 项项目切换等待活动入口超时；未修改该测试或放宽业务断言，最终 Web 全集通过。首轮浏览器 `browser-01` 因测试 fixture 缺少 `/maintenance` 响应触发页面错误；补齐 fixture 后通过，原失败证据保留。

补测 `web-final` 为 389 通过/1 失败：检测复制测试在环境概览已到达、原检测详情尚未加载时立即点击，此时只能复制原检测 ID 并标记 Runner 待核实。测试改为等待页面的“最近检测”详情出现，再验证原 Run/Attempt；保留业务语义与原断言，最终 391 项全部通过。

收尾检查：Markdown 本地导航 727 文件/4487 条、零错误，`git diff --check` 通过。L1/L2、Web、四个 owner 专项及新增定向断言均通过；未执行真实模型/远端 PR/安装部署。
