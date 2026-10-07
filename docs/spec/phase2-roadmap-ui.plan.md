# Plan：DAG 与任务原生浏览

任务 ID：49；任务名称：DAG 与任务原生浏览；目标：`PHASE2.1`。范围见 [Spec](phase2-roadmap-ui.spec.md)，结果见[专属验收](../acceptance/phase2-roadmap-ui.md)。硬前置 #44/#45；准入前确认 2.1 最小原型，一个 PR。

## 实施与所有权

1. 在已交付 WinUI 工程接 #45 的任务/基本图查询；只固定本页面的选择和详情关系。
2. 展示直接前置/后继、真实阻塞和精确交付链接；明确截断，不恢复旧高级图接口。
3. 与 #50 共用 #44 客户端/连接；首个页面负责最小执行导航接线，后合入者复核，单一所有者合入，不先建规划导航。
4. 核验 RV1–RV5；人工窗口集中交给 2.1，未知关联不伪造。

## 测试计划

最终页面/测试过滤名在原型确认后、正式准入前固定；以下确定测试范围，不以待定命令获得执行资格。

| 级别 / 场景 ID | 环境与命令 | 时点 | 通过条件 | 证据 |
| --- | --- | --- | --- | --- |
| 原生构建 | Windows；dotnet build ui/SteerLine.Windows/SteerLine.Windows.csproj -c Debug | 页面接线后 | 原生构建通过 | 临时 build.log，专属 acceptance 摘要 |
| 定向 UI / RV1–RV5-A | Windows；dotnet test ui/SteerLine.Windows.Tests/SteerLine.Windows.Tests.csproj；页面精确过滤命令准入前固定 | 查询/导航完成后 | 只读、分页/阻塞、迟到响应和身份隔离非零通过 | 临时 UI 测试输出 |
| 真实接线 / RV1–RV4 | 临时 Windows 服务/Git/SQLite、受控后端；原服务与 UI 命令执行前登记 | 定向通过后 | 页面与真实查询一致，无业务变更；复用当前执行现场 | 临时身份/页面证据，专属 acceptance |
| 人工 / RV5-M | 键盘、宽窄窗口；参与人/窗口待安排 | 自动通过后，最迟 2.1 签收前 | 任务与阻塞可读取、链接可达 | acceptance 逐项记录 |
| 文档静态 | python scripts/check_doc_links.py；python scripts/roadmap_task_id.py check；git diff --check | 本次修订/实现交付前 | 导航/身份/格式通过 | 当前见 roadmap-planning，后续见专属 acceptance |

仅 C# 消费不跑 Go L1/Full/L2/L3/live；若实际改 Go，补一次 L1及受影响定向测试。所有原始输出用本次临时目录，不重跑发布故障矩阵。本次仅规划，不执行上述产品验证。
