# Plan：正式文档发布与任务创建

> 2026-10-13 归档说明：本文件仅保留原方案/交付历史，不是当前执行或验收要求。旧编号不复用；当前规划仅 #91，见[execution-map](../../../roadmap/execution-map.md)。

> 本轮规划状态：2.2/2.3待重定参考方案，不属于近期准入候选。下文旧模型、依赖、命令及算法不能直接用于新任务固定；以[三节点推进边界](../../roadmap/phase2-delivery.md)为调整入口。流程和原型确认后修订独立Spec/Plan及专属验收，外部已冻结契约须另行核对。

任务 ID：48；任务名称：正式文档发布与任务创建；目标：`PHASE2`。唯一执行依据：[Spec](phase2-document-publish.spec.md)。一个独立实现 PR 交付。

硬前置：#46。全部按原任务交付核验，不能按接口里程碑提前执行。

## 实施顺序

1. 以 #46 的 proposal/guard/文档快照建立 Publication DTO 与人工 publish/reconcile 命令；先固定阶段和恢复日志。
2. 实现隔离 index、确定 Git commit、CAS 分支、逐文件物化和实际 worktree index 的加锁/原子更新；故障注入验证查询版本及 HEAD/index/文件一致。
3. 实现 documents_only 路径，再接目标分支的规划提交祖先及规格 blob 双重检查、固定 task-batch payload 与原请求创建/回读。
4. 用真实临时 Git 仓库验证人工集成、文件冲突、目标漂移及引用内容门槛；不自动操作用户主线或真实任务。
5. 完成 DP1–DP9；覆盖连续发布/同步、同内容 squash 拒绝、needs_review 修订、取消与创建意图竞争、两侧部分完成和在线/离线读取。使用 #46 turn owner 夹具；若 #47 已交付，后合入方补跑消费者回归；真实 Codex 交叉验收归 [二期验收活动](../../../acceptance/phase2.md)，不新增开工前置。

## 自动验收实施与交接

任务专属记录：[验收文档](../../../acceptance/archive/superseded/phase2-document-publish.md)。本 Plan 覆盖 DP1-A, DP2-A, DP3-A, DP4-A, DP5-A, DP6-A, DP7-A, DP8-A, DP9-A；人工部分另列，不以自动通过关闭人工项，实时结果维护在 acceptance。

1. 沿既有 testenv/driver/evidence 扩展公开 publication/reconcile 调用与指定崩溃窗口；临时仓库中的 merge/fast-forward 由驱动模拟产品人工命令，不自动操作用户主线；真实 Codex 交叉验证保留阶段活动归属。
2. 按下表补齐最小场景，在 tests/e2e/suites.json 登记对应 level/suite/package/test/env，live 另登记 chain。受控与真实环境使用不同场景/包，补齐 driver/native/evidence/testenv 的实际缺口及清理，不改生产权限或削弱业务断言。先固定该场景的实际构建入口、测试名称、环境和证据字段；不能仅写“以后补验”。
3. 实现/登记完成后执行各自动入口，逐项核对本任务验收项均有对应断言与结果；零匹配、skip、超时、未解释失败或资源清理失败不能通过。公共边界变化时对真实受影响消费者做定向回归，所需组按最小范围选择。
4. 在任务专属验收文档登记实际命令/构建 commit 与摘要、逐项结果/日期/临时证据位置，按真实准入/执行事实同步 execution-map 汇总及未验链接，再交接接口与结果。Plan 不存实时已验状态，PR 合并不代替 TaskDelivery 或人工通过。

## 测试计划（实施与自动验收）

以下二期场景均为**规划新增、尚未实现/登记**，须先完成上述能力和登记；命令不是当前可执行能力的声明。命令中的 `<本次空临时目录>` 每次替换为新的绝对路径；本任务内新增的能力在实际验收前落实。范围/参数尚未确定时继续完善草稿，准入前固定选择和补齐安排。

| 验收项 / 专属记录 | 级别 / 组 / 场景 | 环境 / 命令 | 执行时点 | 通过条件 | 证据与结果登记 |
| --- | --- | --- | --- | --- | --- |
| [DP1-A、DP2-A、DP3-A、DP4-A、DP5-A、DP6-A、DP7-A、DP8-A、DP9-A](../../../acceptance/archive/superseded/phase2-document-publish.md#自动验收) | L3 / planning / planning.publication（待新增） | Windows 隔离项目/真实服务/当前构建；无真实模型；Git/SQLite/必要原生进程及受控后端；`go run ./cmd/test-feedback -level=L3 -suite=planning -env=controlled '-scenario=planning.publication' '-out=<本次空临时目录>'` | 场景/能力及本任务接线完成后、交付前 | 真实 Git/SQLite、发布/集成/原 ID、部分完成/回执丢失、模式/取消竞争与只读版本一致性；每项真实断言通过、报告完整且资源清理成功 | 临时 summary.json、scenarios/planning.publication/；逐项登记任务验收文档 |
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
| 文档静态 / doc-links、task-identities、whitespace | Windows/Python/Git；python scripts/check_doc_links.py；python scripts/roadmap_task_id.py check；git diff --check | 本轮修订交付前 | 参考状态/导航/身份一致、格式无错 | [规划文档验收](../../../acceptance/roadmap-planning.md)及本次临时输出 |

不选择产品L1/L2/L3/Full、UI或模型场景：没有实现变更，实际流程和原型待定。
