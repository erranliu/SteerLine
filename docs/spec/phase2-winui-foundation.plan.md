# Plan：WinUI 基座与原生输入

任务 ID：44；任务名称：WinUI 基座与原生输入；目标：`PHASE2`。唯一执行依据：[Spec](phase2-winui-foundation.spec.md)。一个独立实现 PR 交付。

硬前置：无。

## 实施顺序

1. 建立 WinUI 工程、锁定依赖和原生构建入口，定义页面导航参数及本地视图存储。
2. 实现可折叠布局、项目/session 切换与原生输入组件；用明确标识的固定样例接导航。
3. 完成按 session 的草稿持久化、IME 发送保护和快捷键设置；接入公共连接状态容器。
4. 在真实 Windows 完成 UI1–UI5，记录构建、缩放、输入法和独立服务身份证据。

## 自动验收实施与交接

任务专属记录：[验收文档](../acceptance/phase2-winui-foundation.md)。本 Plan 覆盖 UI1-A, UI2-A, UI3-A, UI4-A, UI5-A；人工部分另列，不以自动通过关闭人工项，实时结果维护在 acceptance。

1. 步骤 1 固定实际 WinUI 工程路径、依赖锁定和构建命令；在既有 driver/native/testenv 扩展最小 Windows 窗口启动/读取/输入与退出观察能力，场景使用真实原生客户端及隔离数据，不新增生产测试入口。
2. 按下表补齐最小场景，在 tests/e2e/suites.json 登记对应 level/suite/package/test/env，live 另登记 chain。受控与真实环境使用不同场景/包，补齐 driver/native/evidence/testenv 的实际缺口及清理，不改生产权限或削弱业务断言。先固定该场景的实际构建入口、测试名称、环境和证据字段；不能仅写“以后补验”。
3. 实现/登记完成后执行各自动入口，逐项核对本任务验收项均有对应断言与结果；零匹配、skip、超时、未解释失败或资源清理失败不能通过。公共边界变化时对真实受影响消费者做定向回归，所需组按最小范围选择。
4. 在任务专属验收文档登记实际命令/构建 commit 与摘要、逐项结果/日期/临时证据位置，按真实准入/执行事实同步 execution-map 汇总及未验链接，再交接接口与结果。Plan 不存实时已验状态，PR 合并不代替 TaskDelivery 或人工通过。

## 测试计划（实施与自动验收）

以下二期场景均为**规划新增、尚未实现/登记**，须先完成上述能力和登记；命令不是当前可执行能力的声明。命令中的 `<本次空临时目录>` 每次替换为新的绝对路径；本任务内新增的能力在实际验收前落实。范围/参数尚未确定时继续完善草稿，准入前固定选择和补齐安排。

| 验收项 / 专属记录 | 级别 / 组 / 场景 | 环境 / 命令 | 执行时点 | 通过条件 | 证据与结果登记 |
| --- | --- | --- | --- | --- | --- |
| [UI1-A、UI2-A、UI3-A、UI4-A、UI5-A](../acceptance/phase2-winui-foundation.md#自动验收) | L3 / ui / ui.foundation（待新增） | Windows 隔离原生 UI/公开 DTO fixture、当前构建；无真实模型；真实服务/owned worker 只用于 UI5 生命周期核验，不要求未交付的 #45 API；`go run ./cmd/test-feedback -level=L3 -suite=ui -env=controlled '-scenario=ui.foundation' '-out=<本次空临时目录>'` | 场景/能力及本任务接线完成后、交付前 | 原生构建/启动、输入逻辑、草稿/焦点隔离、可访问性标签与 UI 关闭后的原服务/worker 身份；每项真实断言通过、报告完整且资源清理成功 | 临时 summary.json、scenarios/ui.foundation/；逐项登记任务验收文档 |
| 必要回归（非全部产品验收） | L1 + 实际受影响 owner 定向 | Go 接线改动时 `go run ./cmd/test-feedback -level=L1`；执行前在本 Plan 固定受影响包/用例的精确定向命令；仅 UI 改动用已有原生构建及组件回归 | 自动场景接通后一次；新失败/接口变化才补跑 | 非零命中、原消费者不回退；L1 不代替上列原生/业务场景 | 各自新临时目录与本任务验收文档的回归备注 |
| 文档静态（非产品验收） | doc-links / task-identities / whitespace | Windows、Python/Git；`python scripts/check_doc_links.py`；`python scripts/roadmap_task_id.py check`；`git diff --check` | 草稿修订交付前；实现时导航改变后 | 导航、身份与格式一致 | 本次静态输出；实现时在本任务验收文档登记，不冒充自动产品通过 |

构建/依赖：按本任务实际入口固定 Windows 构建命令，Go 公共平台依赖变化时补 Linux 编译并恢复 GOOS/GOARCH；本任务交付并固定原生 UI 工程及构建回执，C# 构建不能证明 Linux UI。Go 接线修改后 vet 仅选实际受影响包，不默认全项目扫描。

不默认 Full、完整 L2、全部 live 或阶段整链：本任务只负责上述边界，controlled 与 live 各按所需证据选择；未来真实失败或未覆盖影响才扩范围并说明原因。仅改指南/Plan 时不执行本表产品或模型入口。

## 人工安排

[UI2-M、UI4-M](../acceptance/phase2-winui-foundation.md#人工验收) 由人工执行，参与人/窗口待安排，实际命令/构建与步骤在专属文档执行前固定。尽量在必需自动检查通过后集中后置；后续不依赖该结果且无明确先验门槛时不阻塞无关执行。影响核心语义/后继资格或出现真实失败时仍阻塞相应范围，不能静默豁免 Spec。交付保留“待人工验收”及 execution-map 链接，不记全部已验。
