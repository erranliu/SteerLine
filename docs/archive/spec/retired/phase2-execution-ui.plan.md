# Plan：执行与待处理原生界面

> 2026-10-13 归档说明：本文件仅保留原方案/交付历史，不是当前执行或验收要求。旧编号不复用；当前规划仅 #91，见[execution-map](../../../roadmap/execution-map.md)。

任务 ID：50；任务名称：执行与待处理原生界面；目标：`PHASE2.1`。范围见 [Spec](phase2-execution-ui.spec.md)，结果见[专属验收](../../../acceptance/archive/superseded/phase2-execution-ui.md)。硬前置 #44/#45；正式准入前确认 2.1 最小原型，一个实现 PR。

## 实施与所有权

1. 核对现有控制台 action与 #45 映射，形成首候选/新增/放弃/丢弃、反馈及派发/服务/环境动作的替代清单。
2. 复用 #44 输入/连接及 #49 已交付导航；若本任务先合入，则拥有最小执行导航接线，#49 后合入复核。共享客户端只维护一套。
3. 实现任务/候选详情、有界活动、最终回复、原问题反馈、请求持久化和核验；不建设完整 transcript 或自定义人工事项系统。
4. 注入重复、回执丢失、旧目标与项目切换，用真实服务/可控后端核验原目标和副作用次数；完成自动项后交接 2.1 原生人工验收。

## 测试计划

原型确认后、正式准入前固定页面测试过滤及真实接线驱动命令；不以待定测试入口准入。

| 级别 / 场景 ID | 环境与命令 | 时点 | 通过条件 | 证据 |
| --- | --- | --- | --- | --- |
| 原生构建 | Windows；dotnet build ui/SteerLine.Windows/SteerLine.Windows.csproj -c Debug | 接线完成后 | 构建通过 | 临时 build.log，专属 acceptance 摘要 |
| 定向 UI / EX1–EX6-A | Windows；dotnet test ui/SteerLine.Windows.Tests/SteerLine.Windows.Tests.csproj；精确页面过滤准入前固定 | 输入/反馈完成后 | 状态、目标/版本、请求持久化和迟到响应非零通过 | 临时 UI 测试输出 |
| 真实接线 / EX3–EX5 | 临时 Windows 服务/Git/SQLite、可控双候选/PR后端；驱动命令准入前固定 | 定向通过后 | 一次代表性候选反馈/交付；重复及未知请求至多一个动作，服务是交付权威 | 临时身份/回执/查询摘要，专属 acceptance |
| 人工 / EX6-M | 真实窗口、中文多行、候选操作和窄窗口；参与人/窗口待安排 | 自动通过后，最迟 2.1 签收前 | 当前控制台日常操作可由 UI 完成 | acceptance 操作及未验明细 |
| 文档静态 | python scripts/check_doc_links.py；python scripts/roadmap_task_id.py check；git diff --check | 本次修订/交付前 | 身份/导航/格式通过 | 本次见 roadmap-planning |

纯 C# 消费不跑 Go全量或 live；Go接线确改时补一次 L1与受影响定向测试。既有真实交付证据按构建影响复用，安装版的真实 UI 双后端链在 2.1 集中签收，不重复完整上游矩阵。原始产物在本次临时目录，本次不执行产品验证。
