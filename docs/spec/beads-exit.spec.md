# Spec：解除 Beads 依赖与项目迁移

任务：`BEADS-EXIT`。状态：待实施。配套 [Plan](beads-exit.plan.md)。本文件只定义该任务，由一个独立实现 PR 交付；新项目接入和旧项目迁移是同一任务的两个验收场景，不分设任务或 PR。

## B1：范围与前置

前置：[固定 DAG Spec](task-dag.spec.md) 完整验收并合入，复用其 D2–D5 及 [原生任务模型](task-core.spec.md) C2/C3。引用 [原生任务共同约束](native-task-constraints.md) 全部条款；本任务负责让已实现的原生权威成为可用的项目执行来源。

交付原生配置/check、worker/技能上下文、worktree 核验和结项路径去耦，以及 Beads 导出、预演、一次性导入切换。不再实现发布/ready/claim/close 算法，不实现图形展示、人工拆分/回退流程或长期双向同步。

无 bd/bv、无 Beads 环境的新项目必须完成创建到真实 PR 合并核验及后继就绪。任务源和 backend 分离；不等待整个远程闭环，已启用的受监管远程则必须遵守其已有能力门槛。项目接入、迁移与生产启用都属于本任务的验收范围。

## B2：无 Beads 接入

复用前置配置 `task_source.kind=beads|native`。没有该字段的既有项目按 beads 兼容；显式 native 且没有旧任务/旧运行状态的新项目可初始化原生权威。已有状态不得因改配置初始化第二份数据库，配置与持久权威不符返回 authority_mismatch。

| 修改面 | 原生模式要求 |
| --- | --- |
| 配置/check | 不执行 bd/bv help、ready、context，不要求 Beads 目录或数据库；原生配置若仍设置旧 completion 则拒绝并解释不兼容 |
| service/控制入口 | ready、claim、详情、恢复与关闭使用同一中心任务服务；旧 Beads 查询隔离在兼容实现中 |
| workspace/worker | 核对 task_id、contract digest、run/attempt 和实际 workspace/base；清除注入给 worker 的 BEADS_DIR/BEADS_ACTOR，不从它们推断身份 |
| prompt/Schema | 原生上下文版本 `native-task/v1`；包含项目/任务/契约、claim、运行/执行/尝试、固定仓库/分支/base 和对应步骤输入 |
| 技能入口 | 遵循 [中心执行模型](02-execution-model-and-adapter.md) 的 minimal-v1/project_steps；不再次领取、不写中心表、不调用 bd/bv、拆分或后继解锁脚本，只返回本步骤事实 |
| 结项 | 中心查询平台证据并提交 [固定 DAG Spec D3](task-dag.spec.md) 的事务；不执行 Beads completion、BV 刷新或隐式通用回调 |
| 查看器/快照 | 当前事实来自 QueryService；旧外部身份保留可追踪映射，不能再为原生详情调用 bd show |

新原生项目只运行 minimal-v1，旧整段 start/finish 的 legacy-v1 仅留作 Beads 项目与历史兼容。迁移前必须把项目技能/脚本适配为已提交的 project_steps，明确输入输出版本并在隔离运行中核验。不能只加标记或提示词，宣称已消除脚本的 bd/BV 或额外结项副作用。

上述 native-task/v1 是中心到执行器的任务上下文，不修改外部 Orca 协议。worker 不连接中心 SQLite，不要求在远端安装 SteerLine 服务；使用现有结果通道回传。远端消息限制和版本化引用按 [远程闭环 Spec](03-remote-execution-and-viewer.md)；载荷超限必须领取前尽可能拒绝，不能通过省略关键身份或加大消息上限规避。

## B3：一次性迁移

迁移以项目为单位，中心是唯一导入写者；本版不迁移活跃 claim/WorkflowRun、待合并 PR、未完成 completion、未知 Operation、未解决决策或仍可能持有现场资源的执行。必须暂停新派发并结清这些事实，不能把“进程不在”当作已结清。外部 Beads 写入由用户协调停止，不承诺锁住其他工具。

### 固定输入

导入输入为 `beads-import/v1` JSON 快照，包含 source_project_identity（原 project/database/backend 身份）、目标 project/repository/branch、导出工具版本与时间、未完成任务及其完整前置闭包、原始状态/assignee/类型/元数据/边、Spec/Plan 映射、原 ready 集合和历史完成证据。规范摘要规则沿用 [固定 DAG Spec D2](task-dag.spec.md)。快照由只读导出器经已验证的 Beads CLI JSON 生成，不读取内部数据库文件；具体 CLI 版本/命令与样本必须在 BEADS-EXIT 中固定并测试，未知关系类型保留后拒绝，不静默丢弃。

所有剩余未完成任务须 open 且未分配，特殊 blocked/in_progress 或外部 assignee 需人工整理；保留的已完成前置允许保存历史领取者。epic/aggregate、split 元数据、动态后继规则、跨项目边、循环、缺失闭包或契约引用不全均返回 unsupported_import，报告具体对象。用户人工整理后生成新快照，不由导入器自动扁平化。

每个历史 closed 前置必须有以下之一：可追踪并重新核验的 PR 交付依据，以其 merge commit 作为基线；或用户明确提供的 `accepted_baseline`（task external ID、目标仓库/分支、包含所需成果的完整 baseline_commit、验收引用、确认者、确认时间和说明）。基线在发布的目标主线上核验可达，不伪造历史 WorkflowRun/验证/审查成功；写入 imported_baseline 并区分 evidence 来源为平台核验或人工确认。仅一个 closed 字段不释放依赖。

### 预演与切换

1. dry-run 核对源/目标身份、闭包、原生约束、规格与历史证据，以及项目接入能力；只生成包含 snapshot_digest、authority_version、预期图/ready、原 ready 差异及阻塞项的报告，不改任务与权威。
2. 正式导入通过 `-report <report.json>` 携带预演报告和摘要；存在 ready 差异时还必须指定 `-accept-differences`，确认绑定到该报告摘要。中心重新读取并比较源的任务/依赖/状态摘要、目标权威版本及执行静默条件；不同则 snapshot_changed/stale_version，要求重新预演。前后两次一致检查不宣称能阻止外部并发写，操作期间仍由用户保证源冻结。客户端声明的报告不能替代中心重算，含任何阻塞项的报告即使确认也拒绝。
3. 规格/平台/基线等外部读取完成后，在单个事务再次核验中心状态，导入任务、契约、全部边、交付基线、来源映射、迁移摘要与事件，并把权威切为 native、递增版本；暂停状态保留。命令参数中的 expected-authority-version 与该事务 CAS 一致。
4. 已由中心执行模型建立的同来源 Task 必须复用其内部 ID，既有运行/PR/消息引用不重建。未建立者分配新 ID；冲突、历史 PR 重复绑定或身份不符使整个导入失败。历史非闭包任务保留 legacy_history 只读来源标记，不成为原生候选，不为它们虚构交付。
5. 事务失败保持旧权威；提交后响应丢失按原 request_id/snapshot_digest 查回原映射，不再导入。投影/配置文件更新失败不回滚数据库，查询显示实际 native 权威；旧配置不能启动 Beads 路径。
6. 切换后把配置声明更新为 native，再通过既有命令显式恢复派发；该配置更新是接入步骤，不是第二权威。此后正常读写不得访问 Beads；旧快照供追踪，不双写、不自动切回。

无需新建原生项目执行此迁移。迁移只能完成已符合约束的项目；特殊业务回调或额外交付副作用仍未适配时，拒绝切换，不通过丢弃回调使迁移“成功”。切换后的人工拆分/回退依共同约束，由用户承担，不增加产品恢复工作流。

### 导出与导入命令

新增只读导出器 `task export-beads -output <snapshot.json>`，使用已验证的 Beads CLI 生成上述快照；普通原生项目不需要该工具或 Beads 安装。输入快照可在预演前由用户补充规格映射及 accepted_baseline，不允许导出器伪造交付证明。

| CLI 入口 | 中心语义 | 副作用 |
| --- | --- | --- |
| `task import-beads -file <snapshot> -dry-run` | 导入预演与报告 | 只读校验，不切换 |
| `task import-beads -file <snapshot> -report <report.json> -request-id <id> -expected-authority-version <n>` | 一次性导入切换 | 中心事务，不恢复派发；ready 存在差异时另需 -accept-differences |

上述命令接受 -config；命令携带实际固定内容，不让服务重新读取可变的客户端文件。幂等与 payload 规范化复用固定 DAG Spec D2；源摘要重核、阻塞项不可绕过等要求以本节为准。新增错误至少包含 unsupported_import、import_not_quiescent、snapshot_changed，并沿用契约/权威/版本/存储错误。

## B4：历史、远端与交接

Spec 02 的旧 runtime 导入负责既有运行历史，本任务只接替任务源，复用内部 ID、PR 和日志/会话关联。旧 completion 和 Beads 查询仅留在未切换项目中；切换后不再调用。

与 [远程闭环 Spec](03-remote-execution-and-viewer.md) 共享已存在的 AttemptSpec/结果通道和中心状态机，先落地的一方提供权威接口，后者扩展；不得因 native 上下文新增远程取消、提升载荷限制或改变目标版本。交给 [关系视图 Spec](dag-view.spec.md) 的是稳定的原生任务态、来源映射和交付来源。

## 验收

| 编号 | 场景与通过条件 |
| --- | --- |
| NT12 | 无 bd/bv、无 Beads 环境的新项目通过 check、创建、启动、验证/发布、真实合并核验和后继就绪 |
| NT13 | 旧 start/finish、completion 或隐含 bd/BV 脚本未适配则预检失败；原生 worker 不再收到 Beads 环境/路径 |
| NT14 | 迁移正常闭包/来源/历史基线，复用已存在 Task ID；ready 差异明确；不伪造历史步骤成功 |
| NT15 | 活跃/未知执行、待合并/回调、外部 assignee、聚合/动态关系、未知 closed、源变更、PR 冲突拒绝切换 |
| NT16 | 导入事务失败无切换；提交后丢回执/配置写失败/重启恢复同一映射；切换后 normal path 无 Beads 调用 |
| NT17 | local 与已支持本机 Orca 的原生路径均通过；已启用 supervised 时追加真实远端回归，不扩大版本/取消能力 |
| NT21-EXIT | 切换前 Beads 项目、旧历史只读、watch/answer/retry/accept-result、会话及控制生命周期回归通过；切换后正常路径无 Beads 调用。 |

本任务的全部验收必须在一个独立实现 PR 内完成；配套 Plan 只负责本任务。证据不足不得标为完成。

最终演示使用隔离项目 A → B：无 Beads 安装，B 最初阻塞；A 按固定契约执行、真实 PR 经用户合并后中心核验交付，B 就绪；中途重启仍保留身份和正确资格。不得自动合并 PR，真实证据不足不得完成本任务。
