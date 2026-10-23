# Plan：规划提案与受控草稿

> 2026-10-13 归档说明：本文件仅保留原方案/交付历史，不是当前执行或验收要求。旧编号不复用；当前规划仅 #91，见[execution-map](../../../Roadmap/execution-map.md)。

> 本轮规划状态：2.2/2.3待重定参考方案，不属于近期准入候选。下文旧模型、依赖、命令及算法不能直接用于新任务固定；以[三节点推进边界](../../roadmap/phase2-delivery.md)为调整入口。流程和原型确认后修订独立Spec/Plan及专属验收，外部已冻结契约须另行核对。

任务 ID：46；任务名称：规划提案与受控草稿；目标：`PHASE2`。唯一执行依据：[Spec](phase2-planning-drafts.spec.md)。一个独立实现 PR 交付。

硬前置：#45。全部按原任务交付核验，不能按接口里程碑提前执行。

## 实施顺序

1. 以 #45 当前接口建立规划领域表和 DTO；实现 RegisterPlanningWorkspace、人工 fast-forward 同步、固定 Git 文档基线及离线读模型。
2. 先实现共用 guard 与持久 owner/generation，使用模拟 turn/publish 消费者验证争抢、旧 token 与恢复阻塞，后继直接调用同一入口。
3. 扩展共享 Git ref 分配器为按 request 摘要的原子批次预留，接 SQLite Operation；验证与现有 CLI 同时占号及各个响应丢失窗口。
4. 实现提案生命周期、受管正文版本与草稿投影，接路径/配额和既有 DAG/任务结构校验。
5. 实现正式/草稿/GoalRef/差异读模型、needs_review 重审和失效诊断，按 PD1–PD9 在临时真实 Git worktree 与 SQLite 验证后交接给会话/发布消费者。

## 自动验收实施与交接

任务专属记录：[验收文档](../../../testing/acceptance/archive/superseded/phase2-planning-drafts.md)。本 Plan 覆盖 PD1-A, PD2-A, PD3-A, PD4-A, PD5-A, PD6-A, PD7-A, PD8-A, PD9-A；人工部分另列，不以自动通过关闭人工项，实时结果维护在 acceptance。

1. 在现有 driver/evidence 扩展公开规划命令/读模型适配，使用模拟 turn/publish 消费者与真实 Git/SQLite；明确共享 ref 分配与资源所有权，不能写 SQL 伪造产品完成。
2. 按下表补齐最小场景，在 tests/e2e/suites.json 登记对应 level/suite/package/test/env，live 另登记 chain。受控与真实环境使用不同场景/包，补齐 driver/native/evidence/testenv 的实际缺口及清理，不改生产权限或削弱业务断言。先固定该场景的实际构建入口、测试名称、环境和证据字段；不能仅写“以后补验”。
3. 实现/登记完成后执行各自动入口，逐项核对本任务验收项均有对应断言与结果；零匹配、skip、超时、未解释失败或资源清理失败不能通过。公共边界变化时对真实受影响消费者做定向回归，所需组按最小范围选择。
4. 在任务专属验收文档登记实际命令/构建 commit 与摘要、逐项结果/日期/临时证据位置，按真实准入/执行事实同步 execution-map 汇总及未验链接，再交接接口与结果。Plan 不存实时已验状态，PR 合并不代替 TaskDelivery 或人工通过。

## 测试计划（实施与自动验收）

以下二期场景均为**规划新增、尚未实现/登记**，须先完成上述能力和登记；命令不是当前可执行能力的声明。命令中的 `<本次空临时目录>` 每次替换为新的绝对路径；本任务内新增的能力在实际验收前落实。范围/参数尚未确定时继续完善草稿，准入前固定选择和补齐安排。

| 验收项 / 专属记录 | 级别 / 组 / 场景 | 环境 / 命令 | 执行时点 | 通过条件 | 证据与结果登记 |
| --- | --- | --- | --- | --- | --- |
| [PD1-A、PD2-A、PD3-A、PD4-A、PD5-A、PD6-A、PD7-A、PD8-A、PD9-A](../../../testing/acceptance/archive/superseded/phase2-planning-drafts.md#自动验收) | L3 / planning / planning.drafts（待新增） | Windows 隔离项目/真实服务/当前构建；无真实模型；Git/SQLite/必要原生进程及受控后端；`go run ./cmd/test-feedback -level=L3 -suite=planning -env=controlled '-scenario=planning.drafts' '-out=<本次空临时目录>'` | 场景/能力及本任务接线完成后、交付前 | 真实临时 worktree/SQLite、原占号/请求、guard/CAS/落盘恢复、路径/授权拒绝及只读映射；每项真实断言通过、报告完整且资源清理成功 | 临时 summary.json、scenarios/planning.drafts/；逐项登记任务验收文档 |
| 必要回归（非全部产品验收） | L1 + 实际受影响 owner 定向 | Go 接线改动时 `go run ./cmd/test-feedback -level=L1`；执行前在本 Plan 固定受影响包/用例的精确定向命令；仅 UI 改动用已有原生构建及组件回归 | 自动场景接通后一次；新失败/接口变化才补跑 | 非零命中、原消费者不回退；L1 不代替上列原生/业务场景 | 各自新临时目录与本任务验收文档的回归备注 |
| 文档静态（非产品验收） | doc-links / task-identities / whitespace | Windows、Python/Git；`python scripts/check_doc_links.py`；`python scripts/roadmap_task_id.py check`；`git diff --check` | 草稿修订交付前；实现时导航改变后 | 导航、身份与格式一致 | 本次静态输出；实现时在本任务验收文档登记，不冒充自动产品通过 |

构建/依赖：按本任务实际入口固定 Windows 构建命令，Go 公共平台依赖变化时补 Linux 编译并恢复 GOOS/GOARCH；原生 UI 复用 #44 固定工程及构建回执，C# 构建不能证明 Linux UI。Go 接线修改后 vet 仅选实际受影响包，不默认全项目扫描。

不默认 Full、完整 L2、全部 live 或阶段整链：本任务只负责上述边界，controlled 与 live 各按所需证据选择；未来真实失败或未覆盖影响才扩范围并说明原因。仅改指南/Plan 时不执行本表产品或模型入口。

## 人工安排

现有范围未单列人工验收；产品中的人工命令/授权由自动场景通过公开接口模拟，不因此变为人工验收任务。需人准备权限/环境时提前约窗口；缺能力和自动失败不归类为允许后置人工补验。若范围细化产生人工项，补入任务专属文档并说明实际阻塞范围。

## 本轮规划测试计划

本次仅标明旧方案待重定，不实施旧步骤。正式准入前按成熟Spec重写最小实现测试计划；旧的完整回归及docs/validation输出要求不沿用。

| 级别 / 场景 | 环境与命令 | 时点 | 通过条件 | 证据 |
| --- | --- | --- | --- | --- |
| 文档静态 / doc-links、task-identities、whitespace | Windows/Python/Git；python scripts/check_doc_links.py；python scripts/roadmap_task_id.py check；git diff --check | 本轮修订交付前 | 参考状态/导航/身份一致、格式无错 | [规划文档验收](../../../testing/acceptance/roadmap-planning.md)及本次临时输出 |

不选择产品L1/L2/L3/Full、UI或模型场景：没有实现变更，实际流程和原型待定。
