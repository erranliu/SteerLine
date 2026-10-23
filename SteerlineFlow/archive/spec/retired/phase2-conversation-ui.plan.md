# Plan：规划对话原生界面

> 2026-10-13 归档说明：本文件仅保留原方案/交付历史，不是当前执行或验收要求。旧编号不复用；当前规划仅 #91，见[execution-map](../../../Roadmap/execution-map.md)。

> 本轮规划状态：2.2/2.3待重定参考方案，不属于近期准入候选。下文旧模型、依赖、命令及算法不能直接用于新任务固定；以[三节点推进边界](../../roadmap/phase2-delivery.md)为调整入口。流程和原型确认后修订独立Spec/Plan及专属验收，外部已冻结契约须另行核对。

任务 ID：51；任务名称：规划对话原生界面；目标：`PHASE2`。唯一执行依据：[Spec](phase2-conversation-ui.spec.md)。一个独立实现 PR 交付。

硬前置：#44、#47。全部按原任务真实交付，样例页面或提前完成接口不释放依赖。

## 实施顺序

1. 接 #44 原生输入和导航、#47 session/turn/approval DTO，完成真实新建/恢复/发送。
2. 实现流式正文、工具折叠、完整最终回复和缺口展示，路径引用接统一深链接。
3. 接共享写锁状态、按原 turn 追加/中断和允许的审批选项，保存完整原信封与未发送输入；区分 command_not_found 补交和未知外部 turn 只对账。
4. 在真实 Windows/Codex 完成 PC1–PC5，验证 UI 退出仍保留原 session、未知输入不重发。

## 自动验收实施与交接

任务专属记录：[验收文档](../../../testing/acceptance/archive/superseded/phase2-conversation-ui.md)。本 Plan 覆盖 PC1-A, PC2-A, PC3-A, PC4-A, PC5-A；人工部分另列，不以自动通过关闭人工项，实时结果维护在 acceptance。

1. 复用 #44 原生构建/窗口驱动与 #47 的规划适配，增加本页面公开输入/读取操作；真实场景登记 local-codex 链标识，steer/审批不可用时按 Spec 明示，不由自动测试伪造能力。
2. 按下表补齐最小场景，在 tests/e2e/suites.json 登记对应 level/suite/package/test/env，live 另登记 chain。受控与真实环境使用不同场景/包，补齐 driver/native/evidence/testenv 的实际缺口及清理，不改生产权限或削弱业务断言。先固定该场景的实际构建入口、测试名称、环境和证据字段；不能仅写“以后补验”。
3. 实现/登记完成后执行各自动入口，逐项核对本任务验收项均有对应断言与结果；零匹配、skip、超时、未解释失败或资源清理失败不能通过。公共边界变化时对真实受影响消费者做定向回归，所需组按最小范围选择。
4. 在任务专属验收文档登记实际命令/构建 commit 与摘要、逐项结果/日期/临时证据位置，按真实准入/执行事实同步 execution-map 汇总及未验链接，再交接接口与结果。Plan 不存实时已验状态，PR 合并不代替 TaskDelivery 或人工通过。

## 测试计划（实施与自动验收）

以下二期场景均为**规划新增、尚未实现/登记**，须先完成上述能力和登记；命令不是当前可执行能力的声明。命令中的 `<本次空临时目录>` 每次替换为新的绝对路径；本任务内新增的能力在实际验收前落实。范围/参数尚未确定时继续完善草稿，准入前固定选择和补齐安排。

| 验收项 / 专属记录 | 级别 / 组 / 场景 | 环境 / 命令 | 执行时点 | 通过条件 | 证据与结果登记 |
| --- | --- | --- | --- | --- | --- |
| [PC2-A、PC3-A、PC4-A、PC5-A](../../../testing/acceptance/archive/superseded/phase2-conversation-ui.md#自动验收) | L3 / ui / ui.planning-contract（待新增） | Windows 隔离项目/真实服务/当前构建；无真实模型；Git/SQLite/必要原生进程及受控后端；`go run ./cmd/test-feedback -level=L3 -suite=ui -env=controlled '-scenario=ui.planning-contract' '-out=<本次空临时目录>'` | 场景/能力及本任务接线完成后、交付前 | session/草稿隔离、原信封/迟到回执、权限/gap/链接及控制逻辑；每项真实断言通过、报告完整且资源清理成功 | 临时 summary.json、scenarios/ui.planning-contract/；逐项登记任务验收文档 |
| [PC1-A、PC2-A、PC3-A、PC4-A、PC5-A](../../../testing/acceptance/archive/superseded/phase2-conversation-ui.md#自动验收) | L3 / ui / ui.planning-local（待新增） | Windows 隔离项目/真实服务/当前构建；真实 Codex、固定版本/权限与专用登录环境，消耗模型 token；`go run ./cmd/test-feedback -level=L3 -suite=ui -env=live '-scenario=ui.planning-local' '-out=<本次空临时目录>'` | 场景/能力及本任务接线完成后、交付前 | 真实 #47 session/工具/轮次、stream/final、原请求恢复及服务回执；每项真实断言通过、报告完整且资源清理成功 | 临时 summary.json、scenarios/ui.planning-local/；逐项登记任务验收文档 |
| 必要回归（非全部产品验收） | L1 + 实际受影响 owner 定向 | Go 接线改动时 `go run ./cmd/test-feedback -level=L1`；执行前在本 Plan 固定受影响包/用例的精确定向命令；仅 UI 改动用已有原生构建及组件回归 | 自动场景接通后一次；新失败/接口变化才补跑 | 非零命中、原消费者不回退；L1 不代替上列原生/业务场景 | 各自新临时目录与本任务验收文档的回归备注 |
| 文档静态（非产品验收） | doc-links / task-identities / whitespace | Windows、Python/Git；`python scripts/check_doc_links.py`；`python scripts/roadmap_task_id.py check`；`git diff --check` | 草稿修订交付前；实现时导航改变后 | 导航、身份与格式一致 | 本次静态输出；实现时在本任务验收文档登记，不冒充自动产品通过 |

构建/依赖：按本任务实际入口固定 Windows 构建命令，Go 公共平台依赖变化时补 Linux 编译并恢复 GOOS/GOARCH；原生 UI 复用 #44 固定工程及构建回执，C# 构建不能证明 Linux UI。Go 接线修改后 vet 仅选实际受影响包，不默认全项目扫描。

不默认 Full、完整 L2、全部 live 或阶段整链：本任务只负责上述边界，controlled 与 live 各按所需证据选择；未来真实失败或未覆盖影响才扩范围并说明原因。仅改指南/Plan 时不执行本表产品或模型入口。

## 人工安排

[PC3-M](../../../testing/acceptance/archive/superseded/phase2-conversation-ui.md#人工验收) 由人工执行，参与人/窗口待安排，实际命令/构建与步骤在专属文档执行前固定。尽量在必需自动检查通过后集中后置；后续不依赖该结果且无明确先验门槛时不阻塞无关执行。影响核心语义/后继资格或出现真实失败时仍阻塞相应范围，不能静默豁免 Spec。交付保留“待人工验收”及 execution-map 链接，不记全部已验。

## 本轮规划测试计划

本次仅标明旧方案待重定，不实施旧步骤。正式准入前按成熟Spec重写最小实现测试计划；旧的完整回归及docs/validation输出要求不沿用。

| 级别 / 场景 | 环境与命令 | 时点 | 通过条件 | 证据 |
| --- | --- | --- | --- | --- |
| 文档静态 / doc-links、task-identities、whitespace | Windows/Python/Git；python scripts/check_doc_links.py；python scripts/roadmap_task_id.py check；git diff --check | 本轮修订交付前 | 参考状态/导航/身份一致、格式无错 | [规划文档验收](../../../testing/acceptance/roadmap-planning.md)及本次临时输出 |

不选择产品L1/L2/L3/Full、UI或模型场景：没有实现变更，实际流程和原型待定。
