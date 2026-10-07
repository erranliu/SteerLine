# Plan：Codex 规划会话服务

任务 ID：47；任务名称：Codex 规划会话服务；目标：`PHASE2`。唯一执行依据：[Spec](phase2-planning-sessions.spec.md)。一个独立实现 PR 交付。

硬前置：#43、#46。全部按原任务交付核验，不能按接口里程碑提前执行。

## 实施顺序

1. 在 #43 的固定版本证据和 #46 guard 上实现 app-server 专用适配；服务配置显式核验权限与工具集合。
2. 接会话/轮次命令及规范事件、正文存储，建立请求到外部 thread/turn 的持久身份。
3. 把受控草稿工具接到 #46，验证 session、turn、generation 和 file_id；人工产品命令不进入 agent 工具集。
4. 实现服务停止与未知轮次对账，分别注入发送丢回执、工具落盘后、终态前故障；检查不重发不确定动作。
5. 在真实 Windows 完成 PS1–PS6，与 #48 并行开发但共用接线串行集成；未有 #48 时使用 #46 publish owner 夹具，后合入方回归已交付消费者，真实跨组件验收由 [二期验收活动](../acceptance/phase2.md) 承担。

## 自动验收实施与交接

任务专属记录：[验收文档](../acceptance/phase2-planning-sessions.md)。本 Plan 覆盖 PS1-A, PS2-A, PS3-A, PS4-A, PS5-A, PS6-A；人工部分另列，不以自动通过关闭人工项，实时结果维护在 acceptance。

1. 消费 #43 已固定的实际能力和 #46 guard，扩展规划 session 的公开 driver 与精确故障点；真实场景登记 local-codex 链标识，未交付 #48 时用已约定 publish owner 夹具，不添加新的开工前置。
2. 按下表补齐最小场景，在 tests/e2e/suites.json 登记对应 level/suite/package/test/env，live 另登记 chain。受控与真实环境使用不同场景/包，补齐 driver/native/evidence/testenv 的实际缺口及清理，不改生产权限或削弱业务断言。先固定该场景的实际构建入口、测试名称、环境和证据字段；不能仅写“以后补验”。
3. 实现/登记完成后执行各自动入口，逐项核对本任务验收项均有对应断言与结果；零匹配、skip、超时、未解释失败或资源清理失败不能通过。公共边界变化时对真实受影响消费者做定向回归，所需组按最小范围选择。
4. 在任务专属验收文档登记实际命令/构建 commit 与摘要、逐项结果/日期/临时证据位置，按真实准入/执行事实同步 execution-map 汇总及未验链接，再交接接口与结果。Plan 不存实时已验状态，PR 合并不代替 TaskDelivery 或人工通过。

## 测试计划（实施与自动验收）

以下二期场景均为**规划新增、尚未实现/登记**，须先完成上述能力和登记；命令不是当前可执行能力的声明。命令中的 `<本次空临时目录>` 每次替换为新的绝对路径；本任务内新增的能力在实际验收前落实。范围/参数尚未确定时继续完善草稿，准入前固定选择和补齐安排。

| 验收项 / 专属记录 | 级别 / 组 / 场景 | 环境 / 命令 | 执行时点 | 通过条件 | 证据与结果登记 |
| --- | --- | --- | --- | --- | --- |
| [PS2-A、PS4-A、PS5-A、PS6-A](../acceptance/phase2-planning-sessions.md#自动验收) | L3 / planning / planning.sessions-contract（待新增） | Windows 隔离项目/真实服务/当前构建；无真实模型；Git/SQLite/必要原生进程及受控后端；`go run ./cmd/test-feedback -level=L3 -suite=planning -env=controlled '-scenario=planning.sessions-contract' '-out=<本次空临时目录>'` | 场景/能力及本任务接线完成后、交付前 | guard/工具授权/原信封故障与规范事件恢复；可控协议不证明真实模型行为；每项真实断言通过、报告完整且资源清理成功 | 临时 summary.json、scenarios/planning.sessions-contract/；逐项登记任务验收文档 |
| [PS1-A、PS2-A、PS3-A、PS4-A、PS5-A、PS6-A](../acceptance/phase2-planning-sessions.md#自动验收) | L3 / planning / planning.sessions-local（待新增） | Windows 隔离项目/真实服务/当前构建；真实 Codex、固定版本/权限与专用登录环境，消耗模型 token；`go run ./cmd/test-feedback -level=L3 -suite=planning -env=live '-scenario=planning.sessions-local' '-out=<本次空临时目录>'` | 场景/能力及本任务接线完成后、交付前 | 两个真实 session/原 thread、实际流式/审批/steer/中断、故障恢复与 UI 关闭后的原会话；每项真实断言通过、报告完整且资源清理成功 | 临时 summary.json、scenarios/planning.sessions-local/；逐项登记任务验收文档 |
| 必要回归（非全部产品验收） | L1 + 实际受影响 owner 定向 | Go 接线改动时 `go run ./cmd/test-feedback -level=L1`；执行前在本 Plan 固定受影响包/用例的精确定向命令；仅 UI 改动用已有原生构建及组件回归 | 自动场景接通后一次；新失败/接口变化才补跑 | 非零命中、原消费者不回退；L1 不代替上列原生/业务场景 | 各自新临时目录与本任务验收文档的回归备注 |
| 文档静态（非产品验收） | doc-links / task-identities / whitespace | Windows、Python/Git；`python scripts/check_doc_links.py`；`python scripts/roadmap_task_id.py check`；`git diff --check` | 草稿修订交付前；实现时导航改变后 | 导航、身份与格式一致 | 本次静态输出；实现时在本任务验收文档登记，不冒充自动产品通过 |

构建/依赖：按本任务实际入口固定 Windows 构建命令，Go 公共平台依赖变化时补 Linux 编译并恢复 GOOS/GOARCH；原生 UI 复用 #44 固定工程及构建回执，C# 构建不能证明 Linux UI。Go 接线修改后 vet 仅选实际受影响包，不默认全项目扫描。

不默认 Full、完整 L2、全部 live 或阶段整链：本任务只负责上述边界，controlled 与 live 各按所需证据选择；未来真实失败或未覆盖影响才扩范围并说明原因。仅改指南/Plan 时不执行本表产品或模型入口。

## 人工安排

现有范围未单列人工验收；产品中的人工命令/授权由自动场景通过公开接口模拟，不因此变为人工验收任务。需人准备权限/环境时提前约窗口；缺能力和自动失败不归类为允许后置人工补验。若范围细化产生人工项，补入任务专属文档并说明实际阻塞范围。
