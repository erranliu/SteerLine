# PR 5 Spec：人工决策与恢复闭环

状态：待实现。依赖：PR 2、4A、4B、4C。对应 [Plan](pr-05-intervention.plan.md)。

## 目标

在控制台完成完整的人工决策与现有恢复操作，让操作者看清问题、提交一次明确答案、确认服务处理结果，并在执行失败或结果未知时按现有证据恢复。

新增控制 capability：answer_decision、retry_run、mark_stopped、accept_result、resolve_command。沿用控制协议已定义的指令与回执语义，后端资格来自 [4A](pr-04a-execution-backends.spec.md)，会话身份来自 [4B](pr-04b-conversation-events.spec.md)。新增决策送达/应用字段必须使用版本化记录或升级 schema；不能在严格 runtime v4 中静默改变字段或含义。中心 SQLite 已落地时复用同一事务与 CommandService。

## H1：决策详情与提交

- 待决策列表同时显示项目、task_id、run_id、decision_id、等待时长和摘要。决策详情可滚动阅读完整 question、context、每个 option.detail 和推荐标记。
- 支持最多 12 个选项，全部可用方向键选择；数字快捷键仅是辅助。推荐项可以预选，但必须有单独的明确提交动作。
- 使用支持 Unicode、多行、粘贴和正确光标/删除行为的文本输入；不能继续按单字节截断中文字符串。
- 选项选择和补充说明分开：有 options 且 allow_free_text=false 时必须选声明的选项，note 可以补充；allow_free_text=true 时允许选择“自定义答复”，这时 option_id 为空且 note 非空；没有 options 时必须填写非空 note。
- 详情只读时不截断证据；超过已存在协议长度上限的输入在提交前明确提示，不能静默把用户确认的答案截短。
- 打开弹窗时保存明确的项目/run/decision 身份，提交时由服务重新核验当前版本。对象已变化时保留草稿并显示冲突，不把旧答案应用到新问题。

## H2：答案身份、冲突与留档

新决策按 attempt 下的 decision_id 保存：

```text
runs/<run_id>/decisions/<decision_id>/{request.json,answer.json,continue.txt}
runs/<run_id>/attempts/<attempt_id>/decisions/<decision_id>/{request.json,answer.json,continue.txt}
```

- 首次 attempt 使用第一种路径，retry 使用第二种；runtime.Decision 的绝对路径继续指向当前问题。
- 新问题必须产生新 decision_id，不能覆盖旧请求/答案/续跑说明。历史页按独立 ID 展示多轮决策及对应 attempt。
- 既有 pending 决策继续使用快照中记录的旧 request_path/answer_path；升级时不迁移其文件，也不生成替代 decision_id。续跑说明的路径兼容旧布局。
- 每轮答案采用 first-wins：相同 decision_id、run_id、option、note、answered_by 的重复写入返回原结果；时间戳不同不构成新答案。不同内容或不同操作者的后续提交返回 answer_conflict，并展示已存在的答复。
- 用该决策的跨进程短锁和原子发布实现先到者生效，UI、在线 CLI 与离线 answer 都使用同一发布函数；禁止存在一条还能覆盖答案的旧路径。
- 上述文件 first-wins 适用于当前文件权威；若 Spec 02 已合入，答案权威转为其事务/唯一约束，在线与离线客户端只追加同一命令入口。历史答案文件经幂等导入并参与相同冲突判断，不能与数据库各自产生一个有效答案；文件留档仍保留原路径，导入未接受时不得显示“服务已接受”。
- 既有文档所述“同 ID 覆盖答案”在本 PR 被上述规则替代。要重新提问，必须由后续执行产生新 decision_id，不通过编辑旧答案纠正。

## H3：答案保存、送达与应用

- 在线 UI/CLI 发送 answer_decision，包含 expected_run_revision、decision_id、option/note。服务核验 pending 与身份后发布答案并保存接受事实；该命令 applied 的含义是“答案已接受”。
- 同一个 decision 的答案已经存在且语义一致时，应优先返回幂等结果，不能仅因其 revision 已推进而把成功重试误报为冲突；不同内容仍拒绝。
- `applyDecisions` 在单写者循环中消费答案，先持久化 resuming 与 Reply/Continue Operation，再通过 4A 契约核验原绑定并调用支持的续跑操作；Orca terminal v2 须为同一 terminal/incarnation，supervised 须为同一权威 Dispatch。仅接受回执记录 reply_accepted，不把 terminal send 成功写成 decision=applied。
- applied 需要已验证平台应用事件或严格绑定 decision_id、attempt、原执行身份及固定答案摘要的 worker 确认（包括受核验结果中的证明）。仅有新文本、turn.started、进程活着或 final 回复均不够。轮次启动、宿主活跃和答案应用分别记录；旧 attempt/旧问题的证明只留作历史，不解锁新问题。
- 缺少应用证明能力时，在发送前说明“可确认送达，应用状态不可核实”；发送后保持 reply_accepted/应用待核实，保留原问题与槽约束，禁止盲目重发。只有明确应用证明才可标 applied。若原执行已确认终止但仍无证明，则将决策记为 failed（应用未核实）、run 记为 blocked，保留原答案和回执后按 H4 恢复，不能补写 applied。结构化结果缺必要应用关联时不能借 result 成功绕过尚未解决的决策。
- 续跑仍遵循单槽和 unknown 守卫。派发暂停允许这项已授权的原任务继续；停止服务期间不发 continuation。
- 服务崩溃于 resuming 或平台响应丢失时不自动重发；通过 ReconcileOperation 查询原动作，只有平台已验证的同请求去重/恢复协议允许恢复发送。已接受仍缺证明保持待核实，明确拒绝/失效进入 failed/blocked，未知保留阻塞；不能从超时判断未发送。
- UI 区分：未提交、答案已保存/等待服务、服务已接受/等待执行条件、后端已接受、轮次已开始（若有证据）、答案已应用、明确失败、应用待核实。状态依据持久事实，command.applied 只表示答案被服务接受，与 Decision.applied 分开。
- 服务明确离线时，保留原有离线 answer 文件能力；回答前显示“下次服务启动后可应用”。Windows 的下一次启动须由用户重新打开固定服务窗口入口，不能由回答客户端代启；答案保存不证明已经送达或应用。新鲜在线实例的协议冲突、超时或未知存活不能自动降级成离线写入，必须先确定路径。
- 原 CLI `steerline answer` 连接到兼容在线服务时走相同控制客户端；明确离线时走同一不可覆盖的文件发布函数。后续启动消费合法离线答案是已存在的人工授权。
- 当前 local continuation 能力为 unsupported：在线/离线回答入口共用服务资格与能力判断，保留兼容的 host_unsupported 错误码及具体原因。控制台可查看诊断，不呈现可用的“回答并续跑”。未来补齐能力必须有独立实现和原生恢复验收；遗留答案不能通过新启动执行来替代原会话续跑。
- 待回答/应用待核实卡片位于 4C 折叠组之外，可导航到相关 session/turn/attempt；折叠和轮次结束均不消费问题或修改 Decision。宿主权限请求不使用 answer_decision 自动回答。

## H4：恢复动作

恢复前控制台展示只读核验报告：目标身份、当前执行/交付状态、宿主进程/终端核验、worktree 与 base、claim、PR 绑定和适用条件。报告带 run revision 和采样时间，最终服务执行时重新核验；报告不是绕过检查的通行证。

| 动作 | 前提与效果 | 成功回执的含义 |
| --- | --- | --- |
| retry_run | manual、未暂停、无执行/决策/unknown/控制阻塞；仅 local 的 blocked failed/succeeded，无 PR，工作目录干净且 HEAD=base，claim/Beads 身份不变；保留 task/run/worktree，新建 attempt | 新 attempt 的 worker 已启动并保存身份 |
| mark_stopped | 对 unknown run，操作者明确确认旧执行已停止；已知仍活着的原进程/终端一律拒绝；设置 failed/blocked，保留 claim/工作目录 | 不确定执行被明确标记为停止，未启动 worker |
| accept_result | 对待恢复的 blocked/unknown run，无当前 worker/准备动作，显式确认原执行已停止；已有完整受核验 workspace 证据；校验指定结果和真实 PR | 原任务接回 waiting_pr_merge；完成仍待 PR/结项核实 |
| resolve_command | 对 control.blockers 中的 indeterminate 指令逐项核验，按下面规则解除这一条阻塞 | 该指令的不确定副作用已被记录为已核验/停止；没有重放原指令 |

补充规则：

- retry 不重新 claim、不另选任务、不清理旧日志。在线 retry 使用当前服务启动时的配置；修改配置需先停服再启动。Orca retry 始终返回明确不支持原因，不能暗中回退 local。
- 宿主资格以 4A 固定 ExecutionRef 和能力证据为准；旧记录保留 run.Host 的原有含义并核对证据完整性。修改默认 host 不能把原 Orca run 变成可 local retry 的运行；缺少匹配适配配置/原目标无法验证时明确拒绝，不能借当前默认值改写历史身份。业务层通过公共 Observe/Inspect/Reconcile 操作核验，不直接拼接 Orca CLI 或 local OS 调用。
- 在线 retry 后服务保持运行，并继续跟踪本项目其他 PR；现有离线 CLI RetryOne 仍仅处理所选 run、结束后退出。两种宿主方式共用校验与 startRetry 原语，不共用外围退出循环。
- mark_stopped/accept_result 不能把活跃服务标成 stopped，不能无条件清空其他 run 的 current_task/slot。动作完成后重新计算全部执行阻塞。
- 存在其他 worker/准备动作时拒绝恢复变更；其他 unknown 可以保留，但解除一个 unknown 不能释放另一个 unknown 的槽保留。
- waiting_decision 正在等待/续跑的记录先走决策流程；不能用结果接回按钮悄悄覆盖尚未处理的决策。续跑失败已变 blocked 时才进入恢复入口。
- 已知原进程存活时，确认框不能覆盖拒绝。不可核验的旧执行沿用现有人工确认责任，界面展示原因并记录操作者及确认内容。
- completed run 不可通过任何这些动作回退；缺失准备证据的旧 run/孤儿记录不得补造 workspace/base 以通过 accept_result。

### indeterminate 指令的解除

1. 读取原 request、receipt、checkpoint 留存事实，关联保留的 run_id 与宿主执行身份；查询当前 task claim 和已知工作目录。
2. 若持久化顺序及外部核验能确定原动作在 claim/执行副作用之前中断，记录该结论，解除此 blocker；不会自动再次发出原 start。
3. 已有 run 且存在不确定执行时，必须先核验/人工确认旧执行停止，将目标保留为 failed/blocked。resolve_command 可调用同一个停止确认原语完成这个转换，并清除与它精确关联的 blocker。
4. 存在 claim 但没有完整 run 时，按原孤儿恢复规则建立真实的恢复记录，保留任务和原预分配身份；未知字段保持缺失。不能因此让该任务获得 retry/accept-result 资格。
5. 任一已知执行仍活着、身份冲突或必须的外部事实读取失败时，拒绝解除。一次操作只解除指定 request_id，其他 blocker 保持。
6. 原 indeterminate 回执作为历史证据保留，新增恢复回执引用原 request_id；runtime 清除对应 blocker。结果记录明确 resolved_by_request_id/操作者，不把原始未确定结果伪写成“从未出错”。

## H5：结果接回的内容固定

- 操作者选择已有 result 文件，UI 一次读入最多 2 MiB，显示 task/run、PR、worktree、base 和摘要供核对。
- accept_result 请求携带原文件字节的 base64 和 SHA-256，整体受 PR 2 的 3 MiB 上限约束。服务解码后校验大小和摘要，再按现有 Result Schema 解析；不在执行时重新读取一个可能被替换的任意路径。
- 服务把被接受的原始内容保存到 `runs/<run_id>/recovery/<request_id>/result.json`，并引用该受管路径。UI 预览与服务验证的内容必须一致。
- PR 必须经真实提供方查询核对仓库、URL、目标分支和 open/merged 状态；closed_unmerged、unknown/查询失败、PR 重复绑定或 workspace 身份错误都拒绝。
- 接回成功后继续沿用既有 PR 查询/结项幂等流程；控制台不能直接把 run 设置 completed。

## H6：在线与离线 CLI 的统一

- `retry`、`mark-stopped`、`accept-result` CLI 在有兼容在线服务时提交同一动作，并保留现有显式确认参数；明确离线时取得既有两把锁，调用相同业务原语作为维护入口。
- 在线处理不另建 NewService，不与现有服务争锁。离线入口中 service.mode/status 的维护副作用由外围 wrapper 负责。
- 保留 stop、进程身份核验和领取规则；不能为了让 UI 按钮可用而扩大 retry/结果接回的业务资格。
- 审计记录操作者、对象、确认、前后状态版本、结果摘要和拒绝原因。UI 关闭后仍可从请求/回执及独立决策文件查回。

## 验收标准

| ID | 场景与结果 |
| --- | --- |
| H-A1 | 12 个选项、长 detail/context、中文多行/粘贴/退格均可完整查看与正确提交 |
| H-A2 | options+allow_free_text、纯自由文本、必选选项的校验与 UI 行为一致 |
| H-A3 | 两客户端回答同一问题，同内容幂等、不同内容一方冲突，无覆盖 |
| H-A4 | 弹窗打开后 decision 已换轮/已应用，旧草稿不能影响新问题；多轮文件保留 |
| H-A5 | 在线、离线答复与服务启动后消费，准确显示答案接受和续跑结果 |
| H-A6 | Orca incarnation 变化、死亡终端、resuming 崩溃均不重发；local 不显示假续跑能力 |
| H-A7 | local retry 保持 task/run/worktree、生成新 attempt；dirty/advanced/PR/unknown/Orca 均按条件拒绝 |
| H-A8 | 在线 mark/accept 不把服务停掉，不释放别的 unknown/worker 槽；completed 不回退 |
| H-A9 | result 预览后源文件变化不改变提交内容；错误摘要/PR/分支/身份拒绝，成功继续合并核验 |
| H-A10 | 无完整 run 的 indeterminate 指令可按证据恢复；已知存活或事实冲突时不能解除 blocker |
| H-A11 | 旧 pending 决策路径仍可回答一次；CLI/控制台/离线维护共享业务规则 |
| H-A12 | 决策续跑和恢复动作各持久化边界崩溃后，无重复 terminal send、worker start 或 PR 伪完成 |
| H-A13 | 两后端共用能力/资格入口；unsupported/not_tested 拒绝，历史 ref 固定，未验证路径不隐式 fallback |
| H-A14 | backend accepted、turn started 均不标 applied；只有原问题/attempt/答案绑定证明可应用，迟到证明不解锁新问题 |
| H-A15 | 响应丢失、已接受但无证明、服务重启分别保留原 Operation 与待核实状态，不重复答复；平台明确支持时才按原请求恢复 |
| H-A16 | 自动/手动折叠与 turn 结束不隐藏未解决问题、不改变 Decision；业务答复与宿主权限处理分开 |
