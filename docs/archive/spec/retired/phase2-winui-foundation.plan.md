# Plan：WinUI 基座与原生输入

> 2026-10-13 归档说明：本文件仅保留原方案/交付历史，不是当前执行或验收要求。旧编号不复用；当前规划仅 #91，见[execution-map](../../../roadmap/execution-map.md)。#44 的实现删除待 #91 实施；用户撤销后续人工验收要求，原未验保持未验，不记为通过。

任务 ID：44；任务名称：WinUI 基座与原生输入；目标：`PHASE2.1`。范围见 [Spec](phase2-winui-foundation.spec.md)，行为及结果见[专属验收](../../../acceptance/archive/superseded/phase2-winui-foundation.md)。硬前置 #45，一个实现 PR。

## 实施与所有权

1. 建立 ui/SteerLine.Windows/SteerLine.Windows.csproj，固定 .NET/Windows App SDK、构建配置及 Windows 验证版本。此路径在实施时交付，本次不创建空工程。
2. 提供原生窗口和多行编辑组件，显式提交、IME 保护、目标绑定与必要草稿隔离；不做最终规划布局、session 和通用偏好。
3. 实现消费 #45 的窄客户端、身份核验和真实任务查询展示；输入样例明确只产生本地意图，不伪造业务发送成功。
4. 共同 WinUI 工程、连接/输入由本任务拥有；#49/#50 消费已交付组件，页面集成时后合入者复核共享部分。不同时各建客户端/导航。
5. 自动通过后安排 UI2-M/UI4-M，未安排时留待验并交给 2.1 签收；真实失败修复，不能冒充后置。

## 测试计划

以下 UI 工程及测试工程拟在本任务实现，执行前固定实际 SDK/构建参数；不把尚不存在的入口记为已通过。

| 级别 / 场景 ID | 环境与命令 | 时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- |
| 原生构建 / UI1 | Windows 开发环境；dotnet restore ui/SteerLine.Windows/SteerLine.Windows.csproj；dotnet build ui/SteerLine.Windows/SteerLine.Windows.csproj -c Debug | 工程及接线完成后 | 固定依赖构建成功，真实 exe 窗口可启动 | 临时目录 build.log、实际 exe/版本；摘要写专属 acceptance |
| 定向 UI / UI2-A、UI3、UI4-A | Windows；dotnet test ui/SteerLine.Windows.Tests/SteerLine.Windows.Tests.csproj -c Debug | 组件/客户端完成后 | 提交目标、迟到响应、身份拒绝、标签与草稿隔离用例非零通过 | 临时目录 ui-tests.log |
| 真实接线 / UI3、UI5 | #45 真实临时 Windows 服务/SQLite、受控后端；实际服务命令、UI exe 路径在执行前登记 | 自动组件通过后 | 查询与服务一致；UI 关闭无 stop 请求，原进程/worker 身份不变 | 临时目录连接/身份摘要，专属 acceptance 记具体操作 |
| 人工 / UI2-M、UI4-M | 真实中文 IME、粘贴/撤销/换行/显式提交、键盘与常用缩放；参与人/窗口待安排 | 自动通过后集中，最迟 2.1 签收前 | 无误提交、输入和基本查询可达 | 临时截图/操作记录，专属 acceptance 分项记录 |
| 文档静态 | Python/Git；python scripts/check_doc_links.py；python scripts/roadmap_task_id.py check；git diff --check | 本次修订和交付前 | 身份、链接及格式通过 | 本次见 roadmap-planning；实现见专属 acceptance |

不选择 Go L1/L2/L3/Full 或模型场景：仅实现 C# 消费与窗口，真实接线用受控后端。若实际修改 Go 生产接线，补一次 L1 和对应最小定向测试，不能默认跑全量。原始产物放本次临时目录；本次文档修订只运行静态检查。
