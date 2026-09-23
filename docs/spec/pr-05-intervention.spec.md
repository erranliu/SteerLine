# PR 5 Spec：人工决策与恢复闭环

状态：待实现。依赖：PR 2、4。对应 [Plan](pr-05-intervention.plan.md)。

## 目标

在控制台完成完整的人工决策与现有恢复操作，让操作者看清问题、提交一次明确答案、确认服务处理结果，并在执行失败或结果未知时按现有证据恢复。

新增 capability：answer_decision、retry_run、mark_stopped、accept_result、resolve_command。沿用 control 协议 v1 和 runtime v4 已定义的字段。

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
- 既有文档所述“同 ID 覆盖答案”在本 PR 被上述规则替代。要重新提问，必须由后续执行产生新 decision_id，不通过编辑旧答案纠正。

## H3：回答与续跑的两个结果

- 在线 UI/CLI 发送 answer_decision，包含 expected_run_revision、decision_id、option/note。服务核验 pending 与身份后发布答案并保存接受事实；该命令 applied 的含义是“答案已接受”。
- 同一个 decision 的答案已经存在且语义一致时，应优先返回幂等结果，不能仅因其 revision 已推进而把成功重试误报为冲突；不同内容仍拒绝。
- `applyDecisions` 在单写者循环中消费答案，先持久化 resuming，再核验同一 Orca 终端/同一 incarnation，发 continuation 并重新监督。成功后 decision=applied；失败后 run blocked 并显示错误。
- 续跑仍遵循单槽和 unknown 守卫。派发暂停允许这项已授权的原任务继续；停止服务期间不发 continuation。
- 服务崩溃于 resuming 时不自动重发 terminal send，保留 blocked/未知的恢复提示；答案被接受不等于续跑成功。
- UI 区分：未提交、答案已保存/等待服务、服务已接受/等待执行条件、正在续跑、已续跑、续跑失败。状态依据文件回执与 runtime，不能凭提交成功字符串预测后续结果。
- 服务明确离线时，保留原有离线 answer 文件能力；回答前显示“下次服务启动后可应用”。新鲜在线实例的协议冲突、超时或未知存活不能自动降级成离线写入，必须先确定路径。
- 原 CLI `steerline answer` 连接到兼容在线服务时走相同控制客户端；明确离线时走同一不可覆盖的文件发布函数。后续启动消费合法离线答案是已存在的人工授权。
- local 的原执行模型没有同终端续跑能力：新在线/离线回答入口统一返回 host_unsupported，控制台可以查看问题与诊断，但不呈现可用的“回答并续跑”。遗留 local 答案按现有恢复约束报告 blocked/不支持，不能启动新执行来替代续跑。

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
- 宿主资格以 run.Host 的历史事实为准，并核对当前服务是否具备相应适配配置。修改配置里的 host 不能把原 Orca run 变成可 local retry 的运行；不匹配时返回说明，不能借当前默认值改写历史执行身份。
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
