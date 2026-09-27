# 剩余验收核对（2026-09-27）

基线：`908deac`，Windows/amd64，Go 1.26.4，Codex CLI 0.157.1，本机 Orca 1.4.209。此记录补充 [PR 5 验证](pr-05-intervention.md)和 [Orca 协议剩余门槛](../spec/orca-probe/remaining-gates.md)，不改变原有真机验收结论。

## 已完成的独立核对

- `go test ./... -count=1 -timeout 20m` 通过；核心包 373.670 秒，其余有测试的包均通过。`go vet ./...`、`go build ./...`、`python scripts/roadmap_task_id.py check` 与 `git diff --check` 通过；任务身份检查为 14 个文件，下一个 ID 为 14。
- 在一次性非临时目录 Git 仓库中创建 linked worktree，使用 Codex CLI 0.157.1 的 `workspace-write` 实测提交。仅对 `.git/worktrees/<name>` 使用 `--add-dir` 时，`git add` 因共享 `.git/objects` 无写权限失败；同时对 `.git/objects` 和该 worktree 的管理目录使用 `--add-dir` 时，`git add` 与 `git commit` 成功，HEAD 变为 `0c02be5`，工作区干净。对整个共享 `.git` 与管理目录同时授权也曾成功，但授权范围更大。该早期试验只证明 CLI 在隔离仓库的 Git 写入；后文 95012 才提供了 SteerLine Orca 决策应用与重启对账的限定证据。
- SteerLine 的本机 Codex 与 Orca native `agent_skill` 步骤已接入上述两个精确 `--add-dir` 路径：启动前从固定 worktree 的 Git 元数据解析，只允许与中心仓库身份一致的共享对象目录及该 linked worktree 的管理目录；重启对账重建相同命令。Windows 定向测试覆盖允许的 linked worktree、主仓库拒绝和外来仓库身份拒绝。此代码接线尚待新的真实决策续跑证明，不改变 95008/95009/95010 的失败结论。
- 该代码接线后 `go test ./... -count=1 -timeout 20m`、`go vet ./...` 和 `go build ./...` 通过；完整回归的核心包耗时 407.339 秒。文档及样本通过 JSON 解析、任务 ID 校验和 `git diff --check`。这些结果不代替 H-A14/H-A15 的真实 Codex 应用与恢复证明。
- 在先前隔离的 `SteerLineNativeExitAcceptance` 仓库及独立 `STEERLINE_HOME` 中，使用新构建的二进制创建全新任务 95011（`task create` 回执 `fe23b6e015984bca89792000d8bed7a5`），主工作区干净、手动派发、旧 95008–95010 不重放。`start_task` 回执 `0576d83530b82192c0ca5b56ee2fcf5e` 为 applied，固定 run `c5d1f4c4520f2d5bc592f5e8b3785812` 与原 implement attempt `f64cae799b5fc8f4ea5370034edf9459`。服务第一次因 Orca 原终端存活不可核实而停止；确认该精确终端 `term_4a1f92bf-a4ac-4ea2-b3fd-9157f4501d32` 已以代码 0 退出、原 result.json 包含 12 选项且 worktree 在 base `c3f98ee` 保持干净后，按同一服务配置重启。服务只接回原 attempt 的保存结果，恢复 decision `5f155a7f95f646196cfa762bc47dbb45` 为 `pending`，没有重放 worker。当时 Windows TTY 控制台在独立可见窗口定位至 95011 并等待操作者选择；此时尚无应用证明、提交、推送或 PR。

95011 后续：操作者在 Windows TTY 选择 `marker-01`、备注 `test`，中心命令 `caccce26ae86c61d9ea82330767cad1c` 返回 accepted/answer_saved。续跑 attempt `80588c30979db5bf50bcdf53dcc7128e` 再次在原 Orca 终端存活暂不可核实时进入 unknown；当时旧代码把 Decision 过早标成 failed。确认精确终端 `term_3c0d5b00-a4fa-4941-aa67-2ae4e8a53495` 以代码 0 退出后，保存结果含绑定原 decision/attempt/execution/答案摘要的应用证明和成功的 Git commit `7b50db3965f8d148acc56dd0d130050f1b4c09f6`，worktree 干净，未推送或创建 PR。服务重启只接回该 attempt，`implement` 与 `verify` 通过；但 Decision 因先前的 failed 事实没有转为 applied，不能宣告 H-A14/H-A15 通过。publish fixture 随后以代码 1 返回、结果文件为空，中心按 `outcome_unknown` 保留槽。在线恢复面板确认原终端已退出、worktree 干净且 PR 未绑定，但 `mark_stopped` 被当前 step/Operation 对账门槛禁用；没有提交恢复动作或覆盖状态。隔离服务已通过 `stop_service` 回执 `f7f0385747424bf325049530b25dac89` 正常停止，95011 的未知现场保留。

据此修正代码：仅在原执行**明确失败**且无应用证明时标 Decision=failed；原 Orca 终端暂不可核实时保持 `reply_accepted` 和占槽，待同一 attempt 退出并核验保存结果。新增测试通过故障状态、服务重启与原结果对账，断言原问题应用证明仍可使 Decision=applied，不重发 worker。95011 已保存的 failed 决策不做原地改写，修复后的真机验收必须使用新的隔离任务。

恢复验收核对：旧 95011 的 publish fixture 终端 `term_ec424a49-5207-4ec5-b6f3-119b7024d6a1` 确认以代码 1 退出，结果文件为 0 字节，worktree 干净、HEAD 固定在 `7b50db3`，未绑定 PR。重启后的在线恢复面板显示执行槽仍占用，`mark_stopped` 被“minimal-v1 需要当前 step-attempt/Operation 对账”禁用，`retry_run` 也不可用；没有绕过门槛，原 unknown 状态保留。旧隔离服务经 stop_service 正常停止。

为验证修复后的真机应用，另外从测试仓库创建独立 clone 与独立 `STEERLINE_HOME`，新中心身份与旧 95011 不共享 SQLite 或 Git 管理目录。任务 95012 的创建回执 `5cad6f257851459c8347720e3bbd5bf1` 已接受。第一次 start_task 因新项目未持有 Orca 能力证书而在副作用前拒绝；使用原生验收已捕获且目标/版本一致的三份能力证据经 `certify-backend` 重新核验后，第二次 start_task 回执 `64d6860e4d787b70a633643324c28a50` 为 applied。原 implement terminal `term_dae575d2-43f4-4221-b7cb-3d1655df2593` 以代码 0 退出，保存 12 选项决策结果、base `c3f98ee` 和干净 worktree；服务重启按同一 attempt `f5644888482b64bc9db1326e9feb7d7b` 恢复 decision `703d75d727325eda30fa67bb5be6ea6e` 为 pending，未重放。此 clone 的会话读模型仅有 Orca observation 与 execution.result，没有已确认 turn/final 边界，H-A16 精确折叠仍未通过。当时尚无应用证明、提交、推送或 PR。

95012 的 Windows TTY 后续答案命令 `038fb24851aa7f6c7e6ff032b80a13c1` 被接受，真实选择为 `marker-02`、备注“沿用 marker-01，备注 test”；按服务保存的答案核验，不把备注文字误读成另一次选项选择。续跑 attempt `cd6190a758c900a3f5260aa6417c5ee9` 的 Orca 终端 `term_8663250f-cc81-4e1a-81cf-fc120593406d` 先暂不可核实，Decision 保持 `reply_accepted` 并占槽。确认该精确终端以代码 0 退出后，保存结果包含绑定原 decision、原 attempt、同一 execution 与答案摘要的证明，并把仅有 `evidence/95012.txt` 的提交固定在 `f3f0e42db11330ca7497f65cc9804a45e19c1693`，worktree 干净。服务重启只对账该 attempt，SQLite Decision 变为 `applied`（version 5），`implement` 与 `verify` 均成功；本次证明 H-A14 的成功应用路径及 H-A15 的同 attempt 成功恢复。没有显式注入响应丢失，不能宣称 H-A15 全项通过。[精确身份和结果样本](pr-05-intervention-live-gitfix-2026-09-27.json)留档。

预设 publish fixture 随后以代码 1 退出、结果文件为空；中心将其固定 attempt 保持 `outcome_unknown`、保留槽，未绑定 PR，也未关闭任务。Decision 仍为 `applied`，没有因下游发布故障回退。此 unknown 未按不合格恢复操作强行清除；完整 PR5/H-A1～H-A16 验收仍未完成。

在用户明确授权并保存现有会话后，对本机 Orca 1.4.209 执行一次受控 runtime 重启。重启前在独立测试仓库启动只写一次标记、等待五分钟的测试终端；重启后 runtime ID 改变，原终端 handle/incarnation 不变但标为 `orphaned`，原 Windows 脚本进程仍在运行并自然完成，标记仍只有一行。只关闭这个已完成的测试终端；未向其他终端发送输入。95012 旧 publish unknown 的中心服务也在新 Orca runtime 下以 manual 模式重启：step attempt 数仍为 5，原 publish attempt/占槽/Decision.applied 不变，任务 worktree 无新终端，随后服务正常停止。[精确重启样本](pr-05-orca-runtime-restart-2026-09-27.json)。这说明本次换代和重启没有盲目重派，但没有在**活跃的 SteerLine 固定 worker** 上制造 incarnation 替换，也没有注入 resuming 中死亡或响应丢失，H-A6 仍未通过。

按用户要求，后续暂停 home 等远端 Orca 探测，只继续本机验收。另建独立 clone 与中心状态，使用只在 `implement` 首轮生成 12 选项决策、在续跑时留下精确标记并等待的 H-A6 命令 worker；未推送或创建 PR。任务 95013 创建回执 `23cbfb1b059a4804a58c0b651bf63636`、显式启动回执 `bac778e8b82ac47633557ba4d0fe8b40` 均已接受；run `1442f59b23f22da57b43de8fabb3e9ac`、Decision `63737b7b4d0d0c0635ab2cd60bc1d62b` 当时为 `pending`，服务在线，工作区干净且仍在 base；续跑等待标记尚未创建。先保留这一身份和现场，等待操作者对新决策作出测试选择，不预先重发。

用户确认后，95013 答案命令 `d4a6b3f333afb7bb46a78b39bff2f6c6` 被接受，内容为 `marker-01` / `H-A6 local fault test`。续跑 attempt `89bf0e3dc08de6850725af3eb45f6468` 的确定性 worker 先以独占创建的标记记录原 run/execution/attempt/decision，并在精确 Orca terminal `term_e0c54577-ae3f-4a2f-a685-6bd35de24edf` 输出 `H_A6_RESUMING_ENTERED` 后等待。核对终端 handle/incarnation、原工作区和 Decision.reply_accepted 后，仅关闭该测试终端；Orca 返回 `ptyKilled=true`，后续观察为 exited、`stop_unverified`，不伪称正常完成。中心将原 attempt 留在 `outcome_unknown`、Decision 保持 `reply_accepted`、执行槽占用。服务在同一状态目录重启后，step attempts 仍只有 prepare、首轮 implement、原续跑 implement 三条；等待标记仍只有原文件、任务 worktree 无新 terminal、HEAD 仍在 base、无 PR。服务经原命令正常停止。[精确故障样本](pr-05-orca-resuming-death-2026-09-27.json)证明本机 Orca 上这次 resuming 中终端死亡未造成重发；incarnation 替换及显式响应丢失仍未测，H-A6 整体尚未通过。

上述状态修正及 CLI 等待回执边界修正后，`go test ./... -count=1 -timeout 20m` 全部通过（核心包 417.605 秒），`go vet ./...`、Windows 构建、Linux/amd64 交叉构建与 `git diff --check` 通过。此前一次并行完整回归中 CLI 延迟回执测试超时；单测通过而整包失败，随后把测试的正常异步回执等待预算从 1 秒增至 5 秒，并使查询本身撞到期限时仍返回“原 command_id 未确认”而不是裸 context 错误。修正后 CLI 包连续三轮和最终完整回归均通过。
- 读取 Orca 当前环境和资源清单：本机为 1.4.209；已登记的一个远端无法连接，home 远端为 1.4.210 且可连接，并有一个正在运行的 Codex 终端。没有停止、回复或重启任何终端。用户随后明确允许本机 1.4.209/home 1.4.210 的小版本号差异用于远端流程和验收；这改变了后续验收范围，不倒填旧能力报告为混合版本实测。
- 与原始验证基线并行推进的主线现已将 `remoteprotocol` verify 校验器接入受监管中心状态机；P13 正负向 verify 真机回执见[Orca 剩余门槛](../spec/orca-probe/remaining-gates.md)。本轮没有重测该远端路径，也不撤销主线已有证据。

同一 95013 工作树另建裸 Orca 终端 `term_e1741f3d-8d3b-4430-83db-5f6e3addfe7b`，incarnation `7729b38e-d897-4d1d-8faa-da43a27851a0`，确认它存活且与原 worker 身份不同。在它存活时再次重启隔离 SteerLine 服务，SQLite 仍只有原三条 step attempt；续跑 attempt `89bf0e3dc08de6850725af3eb45f6468` 保持 `outcome_unknown`，Decision 保持 `reply_accepted`，执行仍 blocked，且没有新的 worker 终端。服务通过 `stop_service` 回执 `49c8cf2156c4b9e5c47eaa020f68c256` 停止，随后仅关闭该裸测试终端。这证明同工作树的不同终端没有被冒认；未模拟同 handle 的 incarnation 替换，H-A6 仍未全通过。[身份样本](pr-05-orca-resuming-death-2026-09-27.json)。

自动化故障注入 `TestNativeOrcaProjectStepsUseFixedTaskContextAndCenterDelivery` 另覆盖 Orca 终端已创建且命令已完成、但 `terminal create` 回执丢失的情况：中心持久化原 step attempt 为 `outcome_unknown`，服务重启后保持占槽、没有创建第二个终端。此为协议级 fake Orca 测试，不作为 H-A15 真机响应丢失的通过证据。

## 2026-09-28 本机 Orca 显式响应丢失

另建独立 clone、SQLite 与 H-A15 单次故障代理。代理未布置故障时通过真实 Orca 1.4.209 的 worktree/agent 启动、输出、精确观察、一次暂断后同 terminal/incarnation 重接、工作树检查与终端关闭取证，为其独立目标认证五项本机必需能力。隔离 Task 95013 的 12 选项 Decision 接受 `marker-01` 后，代理让原 `terminal create` 在 Orca 已返回 `ok=true` 后丢弃回执。原续跑 attempt `535ec59416ca1713b72fff6465ba4516` 的唯一终端实际运行并写一次绑定标记；中心保存 Operation=`outcome_unknown`、Decision=`resuming` 与执行槽。重开同一 SQLite 后 attempts 仍只有三条、同一工作树只有原 terminal/incarnation，没有第二次创建或 PR。由于 Orca 该回执没有 request ID，未把 unknown 自动提升为 applied。最后仅关闭精确测试终端，回读 exited/`stop_unverified`，未伪称正常完成。[原回执、标记与状态样本](pr-05-h15-response-loss-2026-09-28.json)。

清理后再次启动最初暴露原格式 `resuming`/`answer_saved` 的答案记录不完整：运行投影保留答案，但 SQLite Decision row 丢失答案字节，导致身份校验拒绝。仅在原接受命令、原 Decision/Attempt、下一轮 unknown Attempt 和丢失回执 Operation 全部匹配时，中心现从同一 SQLite 的接受命令恢复缺失答案字节；不重发或标记应用。修复后的服务能启动并生成[只读恢复预览](pr-05-h15-recovery-preview-2026-09-28.json)：槽仍占用、工作树干净、旧终端身份不可用，`retry_run` 与 minimal-v1 `mark_stopped` 仍不合格。因此 H-A16 的**可执行**恢复确认尚未通过。

## 2026-09-28 活跃 worker incarnation 故障注入

复用测试源 clone，另建独立 worktree、SQLite 与 Orca 代理目标，并在该目标上重新认证五项本机必需能力。隔离 Task 95013 的原 implement 问题接受 `marker-01` 后，续跑 worker 在精确终端写一次身份标记并保持存活。代理仅把一次真实 `terminal show` 回执中的 `incarnationId` 改为新值，此后对同句柄观察保持该值；[原回执与改后回执](pr-05-h6-incarnation-2026-09-28.json)除这一个字段完全相同。中心以 `identity_mismatch` 退出当前 worker，将原续跑 Attempt 留为 `outcome_unknown`、Decision 保持 `reply_accepted`、执行槽保留。绕开代理直接查 Orca，原句柄/原 incarnation 的终端当时仍存活。同 SQLite 重启后服务 `running/ready`，仍只有原三条 Attempt、同一原终端，未重派或创建 PR；随后只关闭测试终端并正常停服务。这个结果验证活跃 worker 对观察到的同句柄新 incarnation 会拒绝冒认并保留现场；它是**代理注入**，不宣称 Orca 平台实际创建了同句柄的新 incarnation。

## 尚未取得的门槛

| 门槛 | 下一步与判定边界 |
| --- | --- |
| #3 H-A6、H-A14、H-A15 | 95012 已取得绑定证明、Decision.applied 与同 attempt 重启对账；旧隔离 95013 验证 resuming 中终端死亡不重派，H-A15 验证 Orca 实际创建原续跑终端后回执丢失、同 SQLite 重启保留原 Attempt/Operation/占槽且不重派。新的[活跃 worker 同句柄 incarnation 注入](pr-05-h6-incarnation-2026-09-28.json)验证身份拒绝、unknown/占槽保留与同库重启不重派；Orca 平台真实产生这种同句柄换代尚未观察。H-A15 回执没有平台 request ID，所以不支持自动确认应用。95008/95009/95010 的失败 attempts 不重放；95011 的 failed 决策与 publish unknown 现场不回写成成功。 |
| #3 H-A16 与恢复确认 | 在有真实可读 turn/final 事件的 TTY 会话里核对精确轮次折叠与导航；准备符合当前 step/Operation 资格的隔离恢复对象，检查完整预览并执行确认。原 95010 无可读 session 记录，不能作为精确折叠通过证据。 |
| #4 P10 | 本机中心 1.4.209/home 远端 1.4.210 已验证命令权限提示的观察和人工处置；活跃 home 问题上的 8553 字节序列化回复已在实际发送入口拒绝，并保留原问题供有界回复。剩余早期管理员/UAC 提示本身及处置的完整可见捕获。 |
| #4 P13 | 主线已接入中心校验器，正负向受监管 verify 回执和有界修复循环已有真机证据。本轮未重测远端；其余无效结果按[现行门槛](../spec/orca-probe/remaining-gates.md)分别核对，不把纯函数测试单独当作端到端证据。 |

## home 远端 P10 探测进展

本机 Orca 1.4.209 与 home Orca 1.4.210 均报告 `ready/connected`。home 上的 `SteerlineTest` 仓库可见；探测前 `worker-list` 为空，另有一条不属于本探测的 Codex 终端，未操作它。新建隔离 Run `run_a72cbb88f68e`、Task `task_53a46753e151`、Dispatch `ctx_af2f5c09ad0b`，使用新 worktree `p10-permission-20260927`；没有创建 PR、推送或改动旧终端。

`worker-start` 的原请求 `21166b69-90e2-4396-b882-00d36d5c47f1` 已由 `request-show` 证实完成，但回执阶段只有 `input_accepted`。远端精确 terminal `term_c2b7028e-575a-4dfd-9527-44e123b0f801` 当时存活且显示空的 Codex 输入区；`worker-show` 报 `agentWait: null`，没有 turn-started、权限提示或 worker_done 证据。没有重发已接受的提示。终端启动横幅显示 `permissions: YOLO mode`；这种启动方式无法用来验收“宿主权限提示交人工”。当时保留原 Run/Dispatch/worktree 供精确对账，P10 未通过。

当时的下一步是取得用户对原测试终端的停止授权，并让后续权限探测使用会请求人工批准的 Codex 启动方式；不能把 YOLO 的自动放行或 `agentWait: null` 记作人工处置通过。

后续得到用户授权后，对原 Dispatch `ctx_af2f5c09ad0b` 执行精确 `worker-stop`。Orca 返回 `stopped/closed_agent_terminal/ptyKilled=true`；执行端再读到 `exited`、`exitCause=operator_close`，原 Task 失败并保留可追溯身份。随后在同一隔离 worktree 以 `codex -s workspace-write -a on-request` 启动新终端，通过 `retry-of` 将原 Task 的新 Dispatch `ctx_6cc7d85f505d` 绑定到它。Codex 终端显示任务开始，短暂请求超时和自动重连后出现只读 `whoami` 的批准提示。Orca `worker-show` 对该精确 Dispatch 返回 `observation.status=live`、`exactWorker=true` 与 `agentWait={source:prompt-text, reason:agent-interactive-prompt}`。提示在记录时尚未答复；这只验证宿主提示可观察，不验证人工处置结果，也不满足 P10 的 8 KiB 发送前拒绝门槛。脱敏摘要见[真机样本](../spec/orca-probe/samples/p10-host-permission-2026-09-27.json)。

操作者随后在该终端作一次性批准。终端显示 `You approved codex to run whoami this time`，只读命令返回 home 用户身份；同一 Dispatch 的 `agentWait` 变为 `null`。这验证了宿主提示的观察与人工处置，没有把它冒充业务 ask。Codex worker 在最后两次尝试 `orca orchestration check` 和发送 `worker_done` 时失败：其 `workspace-write` 沙箱不能读取 home 的 `%APPDATA%\\orca\\orca-runtime.json`，报 `Orca is not running`；中心从主机侧查询仍确认 Orca 1.4.210 `ready/connected`。因此没有 worker_done 回执，不能把 Orca Task 标成成功。Codex 轮次结束后，对精确 Dispatch `ctx_6cc7d85f505d` 执行 `worker-stop`；返回 `stopped/closed_agent_terminal/ptyKilled=true`，执行端 fleet 读到 `exited`，两次探测尝试均为已停止的失败 Dispatch，没有继续运行的测试 worker。P10 仍缺 8 KiB 发送前拒绝；远端 Codex 在 `workspace-write` 下无法调用 Orca CLI 也须在后续适配器设计中处理。

Windows 系统剪贴板粘贴已由用户在 2026-09-26 报告通过；本次未重复操作。上述实验没有创建 PR、推送代码或操作遗留 worker。
