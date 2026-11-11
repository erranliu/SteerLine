# Roadmap 规划实施 Plan

2026-11-11 · 实施依据为 [Spec](../spec/roadmap-planning.spec.md) 和[产品草稿](../Roadmap/drafts/roadmap-planning.md)。本计划交付 SteerLine 产品接线。Runner 前置已在 SteerRunner #146（`2de536cb`）合入。实施时固定实际 Runner EXE、来源提交及 SHA-256，并以该构建的公开能力与 schema 为准。

## 1. 整合位置

| 现有位置 | 实施安排 |
| --- | --- |
| `internal/runnercontract`、`internal/runnerclient` | 增补 `conversation/2` 请求、`workspace_write` 投影、acquire/release 与显式 message/close 的公开调用。复用配置校验、实例绑定、inspect、history 和停止证明。 |
| `internal/app/project_runtime.go`、`internal/product/httpapi/roadmaps.go`、`internal/storage/sqlite/` | 新增规划业务记录及原请求回执，沿现有项目权限和命令入口接线。规划存储、查询和恢复沿其业务 owner，既有解析/生成记录继续承载转正。 |
| `internal/app/roadmap_publication.go`、`internal/app/roadmap_admission.go`、`internal/roadmap/files.go` | 复用 provider 身份核验、Git 和路径守卫、内容比较及来源 ID。提取确有两个消费者的发布原语，规划独立构造所选文件提交，转正文档与入图仍由现有入口处理。 |
| `web/src/app/`、`web/src/features/roadmaps/` | 增加项目页签、路由及项目切换映射，复用命令反馈、Markdown 和现有来源导航。规划详情单独管理对话、预览及审阅。 |

## 2. 实施顺序

### P1. 固定 Runner 契约与材料供给

1. 以 #146 公共接口为准核对 `configuration_schemas.managed_conversation_input`、`capabilities.workspace_write` 和服务实例绑定。保存 `workspace_origin_run_id`、`workspace_identity` 及 `execution.workspace`，与原 Run/会话对应。把 `workspace acquire`、`workspace release` 和 `close` 纳入 `runnerclient` 的 CLI 参数构造及实例绑定校验。按公开 schema 校验并使用已配置的本机 `codex-app-server` `full_access` 方案。#146 的配置派生接口不支持 `conversation/2`，启动使用方案的原执行配置。受控调用覆盖完整字段形状、缺能力、版本错误和不合格执行配置。
2. 新增规划短指令、冲突短指令和完整规划规则资产。由 `internal/prompts` 同包组装短指令，只传用户原文、路径及必要事实。因 `conversation/2` 当前材料接口只接受 inline，完整规则以应用管理的固定普通文件提供。将同包规则资产按字节落入应用私有材料目录，记录路径和摘要，用原请求引用文件。本机 Agent 真实读取该文件，项目材料从固定仓库起点读取。缺文件或摘要不符时在提交前失败，保留原材料直到会话和相关请求结算。
3. 核对原生进程能够看到的宿主 Git 登录及 provider 认证。发布用凭据留在 SteerLine provider 调用一侧。固定环境用独立、最小所需权限的账号和测试仓库验证。把有效权限范围和发现的接线限制写入验收。
4. 按[提示词治理](../../docs/prompt-governance.md)提供真实提交及消息正文差异、UTF-8 字节前后值、业务原因、源码和黄金样本验证，并取得独立审核。

### P2. 规划记录、原生会话与生命周期

1. 在项目库内建立规划任务、版本化草稿、原操作和发布/合并记录。用规范路径唯一约束活动占用，以项目 ID 和规划 ID 定位。创建命令记录路径、用户原文、初始 commit、所选 Runner 配置和原请求后派发。沿项目路径策略验证普通文件、大小及 UTF-8。
2. 规划命令经现有 `/commands` 和项目实例/revision 检查。支持创建、消息、原生问答、单任务/全部 Rebase、冲突处理、发布及原请求恢复。项目 API 的 `/roadmap-planning` 和任务子路由提供任务列表、详情、分页历史和审阅快照。查询读取产品记录与已观察事实，异步活动沿现有受理回执反馈。
3. 每个任务只绑定一个 `conversation/2` Run。处理并发时按已核实的当前 revision 受理，重复请求回放结果。按公开 history cursor 和原 command/turn 去重，存储用户输入、最终回复及草稿快照。任务列表默认读取摘要，打开详情再读历史。启动在现有宿主资源约束下运行，空闲会话可继续接收消息。
4. 将规划 Run 纳入项目关闭、服务停机及升级排空核验。Runner 离线时用原 state-dir 的 `inspect --offline` 核对租约和原请求。`collectProjectRunnerStops` 只将当前可停止的规划 Run 纳入启动 `PendingStops`。持有或未核实租约的 Run 保留停止义务，先启动原构建完成在线对账与归还，再以新观察的 revision 提交或核实原关闭。Runner 启动前会执行 `PendingStops`，因此租约尚未归还的 stop 必须延后。确认原会话停止后才完成资源排空。原会话不可恢复时展示原诊断，保留已保存草稿。

### P3. 由 SteerLine 执行 Rebase 与核验冲突

1. 从项目目标观察固定 commit，核对仓库和目标分支。批次冻结目标及成员，逐任务保存可执行或跳过原因，单任务和批次共用同一 Git 入口。以任务 revision 和写入租约串行化本地操作、Agent 轮次及发布确认。
2. 保存 acquire 意图及完整信封，取到回执后重新 `inspect`，核对原 Run、worktree 身份、owner 和 lease ID。核验或导入固定目标 commit 到受管仓库的对象库，分别核对项目 checkout 和 Runner 工作区。记录原分支、当前基线、目标和检查点，核验当前基线同时为任务 HEAD 和目标的祖先。检查点保全原 HEAD、索引和工作树候选文件。必要时在私有分支创建仅含当前有效改动的本地提交，以当前基线为重放边界，将规划改动 Rebase 到固定目标。Git 使用明确的非交互参数与原受管工作区。保留检查点直至操作结算。
3. 在执行 Git 前记录操作阶段，使用宿主受管进程并记录可核实的写者身份。恢复先按原 acquire/release `request_id` 和当前 Run 投影对账。租约匹配且操作尚未派发时，重新核对任务资格。合并或停止意图已冻结写入时，结算原操作并归还租约。已派发时先核实写者退出，并读取 Git 的真实阶段。结果不明时保留原租约和诊断，避免重跑 Git 或释放未知写者的租约。已完成或停在已知冲突时保存事实，再按当前 Runner revision 和原 lease ID 归还。归还已生效时补记事实，其他租约保持阻断。原命令结果未知时保留原信封。明确未受理且 revision 已变化时，重新核实资格再生成新 Runner 请求。
4. 冲突处理前通过 Git 管理目录核验正在进行的原 Rebase 的 `head-name` 和目标 commit。Runner 归还后投影为 `owner=runner` 且 `controls.can_message=true`，SteerLine 才把固定短指令作为原 Run 的显式 `message` 发出。Agent 解决冲突并继续原 Rebase，需要跳过、中止或改变取舍时交回用户。已暂存但仍在 Rebase 的状态也能继续，用户可通过冲突轮补充意见或回答原生问题。轮次结算后读取原分支、目标祖先、未合并索引及 Rebase 元数据。显式中止由 SteerLine 重新取得租约，执行原 Rebase 的中止并核对检查点恢复结果。普通发布资格只在 Git 状态核实后恢复。

### P4. 文件审阅、PR 发布和交接

1. 用当前规划基线与工作树最终内容构建审阅差异。包含本地提交、实际工作树的修改/删除及可发布未跟踪文件，标明不支持的类型。按路径安全校验完整候选集，确认时取得写入租约，在同一原操作内复验所选文件及草稿的字节、模式和删除标记。冻结审阅摘要和已选字节并归还租约后，发布流程只消费已冻结材料。
2. 从干净基线在独立发布工作区应用确认文件，核对实际提交的完整差异和所选集合。保存已确认快照与候选 head 后推送并提 PR。更新前核对原 PR 仍开放且远端 head 符合预期，用精确的远端旧 head 保护推送。未知推送或 PR 创建结果按原分支、提交与 provider 回执核实，保留可供合并核验的历次确认快照。原 PR 已关闭或发生更新竞态时沿原 PR 对账。
3. 先核验 provider 的仓库、目标分支、原 PR 和实际合并 head 对应的确认快照。以 Git tree 核验 merge commit 的选中文件内容、模式和删除，并确认合并进入目标分支。合并与更新竞态按实际合入的已确认 head 结算，head 无法可靠归属时保留待核实。合并落库时原子冻结后续写操作并保存原会话关闭义务。关闭沿 P2/P3 先核实本地写入和租约，再核实停止。工作树待同步期间保留已合入版本预览及较新的本地改动记录。
4. 交接检查使用现有来源读取入口。在同一个可核实的本地 HEAD 上检查当前分支为目标分支、HEAD 包含 merge commit 且已在当前观察的目标分支历史中、来源 Markdown 与 HEAD 对应文件一致。沿用 `SameMarkdown` 的 LF/CRLF 比较，保留原始摘要。同步入口用项目维护的 `maintenance_update`。后续已合入更新且来源仍有效时打开最新文件。本地独有提交、脏稿、错分支或文件不存在时保留原预览和诊断。通过后持久保存来源 ID 与交接状态，稳定链接定向到 `/roadmaps?source=<source_id>`。交接和会话停止分别保存回执，两项均核实后释放活动路径占用。页面读取保持现有查询与操作边界。
5. Web 完成并行任务列表、对话、预览、稳定链接、批量结果、冲突按钮和逐文件审阅。默认选草稿，显示额外文件 diff 后可勾选。刷新与项目切换按路由、任务 revision 和当前操作恢复，复用现有项目导航及异步反馈。

## 3. 测试计划

正式选择见[同名测试安排](roadmap-planning.tests.json)和[测试清单](../testing/test-list.json)。证据根用 `C:\SteerlineDebug\<本次专用 scope>`，按[路径清单](../paths.md)分别传入独立 `-out` 或 `-OutRoot`，真实命令、构建和结果登记至[专属验收](../testing/acceptance/roadmap-planning-workspace.md)。共享 Web 构建、重型 owner 与真实链串行，按[测试指南](../testing.md)复用有效证据。

| 时点与性质 | 验收映射 | 执行安排、环境和通过条件 |
| --- | --- | --- |
| P1–P2 后，定向自动 | RPW-A01、A02、A07 | `go test ./internal/runnerclient ./internal/prompts -count=1`，Windows、公开协议夹具与固定资产。公开契约、能力拒绝、同身份消息、原请求和材料守卫均须命中。应用与存储相关断言随受控链执行。 |
| P4 后，专属受控自动 | RPW-A02–A06 | `scripts/test-roadmap-planning.ps1 -Mode Controlled -OutRoot <新绝对目录>`。Windows、Go/Git、Node 24+，使用实际 Git/SQLite、Runner 回执和 provider 夹具，运行规划业务、Web 代理与组件测试并构建 Web。核对租约恢复、原 Rebase 续作和中止、发布更新与合并竞态、工作树交接。真实 Edge 行为由下方 Live 场景覆盖。 |
| P4 后，固定与 owner 回归 | RPW-A07 | 按正式入口运行 `l1.minimum`、`smoke.delivery`、`go.roadmap`、`go.storage`、`go.http`、`go.deployment`、`web.unit`。分别覆盖固定规则/核心交付、转正业务、记录与原请求、命令路由、停机/升级及前端消费者。各集合非零、完整、通过，原 summary 与 managed 报告可定位。 |
| 候选稳定后，专属真实自动 | RPW-A01、A04–A06 | 实施中为同一入口补 `-Mode Live -RunnerExe <固定EXE> -OutRoot <新绝对目录>`。固定 Windows/Codex/Edge，使用[独立调试包与真实 PR 约定](../../tests/e2e/README.md)中的隔离 clone、授权测试远端及独占 base/head。真实两轮写稿与同会话解决 Git 冲突，通过浏览器选文件并提交、由测试端合并、核验发布及稳定链接。退出前核验停止、监听和远端测试资源清理。 |
| 自动验收通过后，人工 | RPW-M01 | 同构建走查规划分页和批量/文件审阅体验。参与人与窗口待安排，人工体验允许后置。 |

新专属入口未登记为正式测试 ID。受控和真实两种模式分别保留命令、选中场景、版本及退出事实，跳过、零匹配、缺凭据或缺清理证据均按实际结果登记。

## 4. 交付收口

完成代码后核对 Spec 各项要求，保存提示词独立审核和全部自动验收结果。更新实际受影响的产品契约及导航，将人工体验和执行环境限制记入专属验收。实施授权、正式任务身份和执行清单由后续决定。
