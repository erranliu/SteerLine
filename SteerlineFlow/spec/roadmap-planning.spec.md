# Roadmap 规划 Spec

2026-11-11 · 依据[产品草稿](../Roadmap/drafts/roadmap-planning.md)。交付一个项目内的规划分页，从用户与 SteerAgent 讨论目标开始，到用户审阅的 Markdown PR 合并、草稿进入现有「Roadmap 草稿」页为止。[实施 Plan](../plans/roadmap-planning.plan.md) · [专属验收](../testing/acceptance/roadmap-planning-workspace.md)

## 范围与职责

| 所有者 | 职责 |
| --- | --- |
| SteerLine | 规划任务、页面与稳定链接、对话与决定的产品记录、Rebase 批次和本地 Git 操作、冲突提示词与结果核验、审阅快照、PR 发布和合入交接。 |
| Runner | `conversation/2` 的受管可写 worktree、原生会话、逐轮消息与恢复、worktree 写入租约。 |
| SteerAgent | 在原会话调查仓库、调用工具、编辑草稿及相关文件。收到冲突处理指令时解决当前冲突并执行 `git rebase --continue`，需要用户取舍时保留现场并说明。 |
| 用户 | 目标、取舍、Rebase 与冲突处理的触发、逐文件发布确认及 PR 合并。 |

首版以本机 Codex 和新建草稿为范围。继续编辑既有草稿、其他执行后端和跨 Runner 构建接管列为后续范围。规划任务 ID 是项目内产品记录身份。解析、生成 Spec/Plan、正式 Task 编号和入图继续由[现有 Roadmap 草稿流程](../../docs/product-design/roadmap-promotion.md)处理。

## 创建、对话和预览

- 用户在「Roadmap 规划」创建任务，填写名称、目标和草稿文件名。SteerLine 绑定项目、目标分支、固定相对路径和所选本机 Codex 配置。路径位于 `SteerlineFlow/Roadmap/drafts/` 下，按 Windows 大小写及规范路径判重。创建时拒绝已有草稿或活动路径占用，绑定后路径固定。不同任务使用独立 worktree，可在宿主额度内并行。
- 创建动作核实当前目标分支 commit，保存原创建意图并向 Runner 提交一次 `conversation/2`。每项任务绑定一个 Run、原生会话和受管 worktree。用户后续消息通过原 Run 的 `message` 发送，一次只有一个活动轮次。页面显示历史、当前轮次和错误。Runner 提问按原问题资格回答。用户明确确认的取舍随产品记录保存，原生上下文由 Runner 和 Codex 延续。
- 稳定链接按项目与规划任务 ID 打开任务和最新已核验的草稿预览。还没有文件时展示当前状态。SteerLine 在写入结算后读取绑定路径并保存版本、摘要与正文，刷新时可继续展示最近有效版本。无效、缺失或仍含未合并条目的草稿禁止发布，页面显示原因。渲染采用现有安全 Markdown 组件。
- 启动资格以固定 Runner 构建的公开能力和 schema 为准。`conversation/2` 使用本机 `codex-app-server`、显式 `full_access`，提交 `repository`（`url`、`remote`、`base_branch`、固定 `base_revision`，可选 `local_path`）、`conversation.instruction` 与 `conversation.materials: []`。省略 `conversation.directory`、`documents`、`publication` 和 worktree 复用字段。项目规则和仓库材料可在原 worktree 读取。平台完整规则通过固定文件提供，短指令及冲突指令由 `internal/prompts` 同包组装。缺材料时阻断对应请求。

## Rebase 和一键解决冲突

1. 用户点击「Rebase」或「全部 Rebase」。SteerLine 获取并固定本次目标 commit。批次保存成员与各自操作结果。活动轮次、未结算操作、已有冲突及已交接任务不参加本批可写操作，页面显示对应原因。其他任务独立继续。
2. SteerLine 读取 `workspace_write.can_acquire` 和同一 Run revision，以持久原请求调用 `workspace acquire`。成功回执的 `command_id` 为租约 ID。实际写入前，核验原 Run、`workspace_origin_run_id`、`workspace_identity`、`execution.workspace`、`workspace_write.owner=steerline` 和当前租约。Runner 的租约仲裁同一 worktree 的写入。SteerLine 在任务层串行化消息、Rebase 和发布操作。
3. 在原 worktree 建立可恢复的本地检查点，保全已提交、暂存、未暂存及纳入审阅范围的未跟踪内容。将改动同步到固定目标 commit。已在目标且无需同步时报告当前。远端历史改写、无关 Git 操作或身份不符时保留现场并报告原因。
4. Git 停在冲突时，SteerLine 核验 Rebase 仍属于原分支和目标，保存检查点和冲突文件，并在本地写入结算后以原租约 ID 和当前 revision 调用 `workspace release`。Runner 恢复 `owner=runner` 且 `controls.can_message=true` 后，用户可一键向原会话发送固定短指令。SteerAgent 在原 worktree 修复并继续原 Rebase。需要取舍时，用户可回答原生问题或在下一轮冲突处理中补充决定。普通规划消息、新 Rebase 与发布在此期间保持禁用。
5. SteerLine 区分仍有冲突、冲突已暂存但 Rebase 仍在进行、Rebase 完成及异常中止。前两种都可在原会话继续。以原分支恢复、目标 commit 的祖先关系、Rebase 元数据消失和无未合并索引共同核验完成。未知结果保留现场和最近有效预览。用户也可显式中止已核实的原 Rebase，由 SteerLine 在租约下中止并核对检查点恢复结果。开始 Rebase 即撤销未发布的旧审阅确认，完成后按新基线刷新预览和 diff。

Runner 租约跨重启保留且无自动过期，持有期间包括 `message`、`close`、`stop` 在内的新 Run 控制会被拒绝。SteerLine 持久保存原 acquire/release 信封和本地操作阶段。恢复时通过原 `request_id` 对账回执，再以当前 Run 投影确定租约。匹配的租约只有在本地写入已结算后才可归还。已归还的租约只补记归还事实，其他租约或未知本地写者保留阻断。Runner 离线时只读核对，启动同构建服务时延后提交受租约阻挡的停止命令。归还核实后再执行原会话关闭并核实停止。适用边界见[宿主和 Runner 协作](../../docs/integration/steerrunner.md)及[生命周期约定](../../docs/product-design/worker-lifecycle.md)。

## 审阅、发布和交接

1. SteerLine 从当前规划基线与已结算工作区生成累计 diff，覆盖已提交改动以及当前实际文件内容的新建、修改和删除。页面默认选中草稿，其他文件由用户逐项查看后勾选。审阅中标明不可安全发布的文件及原因。草稿须为路径安全、有效 UTF-8 的普通 Markdown 文件。发布的候选文件须通过现有路径守卫，拒绝重定向、子模块、Git 元数据、越界和平台路径别名。
2. 「确认并创建 PR」绑定项目、任务 revision、基线、目标、草稿及所选文件的内容、模式和删除标记。SteerLine 在排他租约下复验当前工作区并持久保存被确认的字节，再归还租约。文件或基线变化需要重新审阅。后台发布从独立干净工作区只应用确认的文件，核对完整提交差异后推送。更新原 PR 前核验其仍开放，并以已知远端 head 保护更新。观察到 PR 已关闭时保留原记录和状态。原 PR 的各个已发布确认版本保留可核验的 head 与快照，供合并和更新竞态时对账。
3. 平台发布请求、凭据和合并观察由 SteerLine 管理。首版本机执行采用 Runner `full_access`，应用流程的排他范围是遵守租约的写入者。Runner 和 Agent 的主机权限仍由执行环境决定。实施须核验发布凭据供给与原生宿主登录边界，并在固定环境验收记录实际权限范围。
4. SteerLine 通过 provider 核验原 PR、仓库、目标分支、实际合并的已发布确认 head 和 merge commit。以该次确认快照逐项核验合入提交的文件内容、模式和删除，再确认合并提交进入目标分支。若旧的已确认 head 先合并，以实际合并版本完成核验，保留较新的未合入改动供追溯。合并事实落库时停止受理规划写操作，并持久记录关闭义务。已有租约先结算归还，之后关闭原会话。
5. 合并后，项目 checkout 须位于目标分支，本地 HEAD 须已发布到该分支且包含合并提交。现有来源读取入口须返回该路径的有效 Markdown，且内容与本地 HEAD 的已提交版本一致。后续已合入更新可作为最新来源。工作树滞后时显示「已合并，待同步」。错分支、本地独有提交、草稿本地改动和文件已移除分别说明原因，并保留已合并版本预览。来源可读后持久记录交接，任务退出规划列表，原稳定链接按来源 ID 打开现有草稿。关闭义务独立跟踪，资源停止核实后释放活动路径占用。来源解析仍由用户发起。

## 产品状态与原请求

- 项目 SQLite 保存任务 revision、草稿路径和当前规划基线、Runner 绑定、草稿快照、原操作和发布回执。Runner 公开接口提供会话事实和租约。Git 与 provider 提供工作区及合并事实。
- 读取只投影已记录或已核验事实。写操作使用现有项目命令信封，绑定项目、实例、目标与版本，原请求在外部效果前保存。重复同请求返回原回执，未知沿原身份核实。Rebase 批次、PR 待同步和停止义务均可跨 SteerLine 重启恢复。
- 通过条件覆盖原生多轮读写、并行与恢复、单任务与全部 Rebase、一键冲突处理、文件级审阅、真实 PR 合并及稳定链接交接。具体场景归[专属验收](../testing/acceptance/roadmap-planning-workspace.md)。
