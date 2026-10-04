# #38 候选观察界面：实现已复审，CC5 人工确认通过

起点：`28b81d82f4afb3e158cf96bd5cbf257fa48b7adc`，与执行开始时最新 `origin/main` 相同；分支：`codex/candidate-console-38`。实现使用当前 #37 服务/客户端合同，无生产服务、生产任务库或模型请求。

操作入口：Task 主行唯一；详情 `d` 显示候选集合，`{`/`}` 选择精确 WorkflowRun，`b` 选择不同冻结方案新增，`z` 明确不可撤销放弃，`a` 文字/选项/备注反馈。规范会话/正文 `c` / `f`、更多/最终回复 `y` / `n` 保持原查询入口。环境动作 `Ctrl+E` / `Ctrl+O` 绑定原 blocker/turn/Execution/Attempt/version/instance。

| 验证 | 实际结果/证据 |
| --- | --- |
| CC1–CC4 定向 fixture | [view-final-04/summary.json](view-final-04/summary.json)，顶层 TestCC1–TestCC4 共 4 项通过，无 fail/skip；五候选一 Task 主行/计数，精确冻结配置/会话、缺页/重排/迟到/重开、草稿与阅读位置、规范缺口忽略 raw、blocker 不自动补投、原环境目标、原请求/错误回执、放弃与停止分离、merged 关闭入口 |
| 实际应用/客户端/服务/SQLite/Git 接线 | [wiring-final-03/summary.json](wiring-final-03/summary.json)，TestCandidateConsoleWiring 及 merged_PR_service_guard 共 2 项通过，无 fail/skip；原 UI 新增、原请求重读、五候选与第六次开始拒绝、原会话反馈、精确退出、真实 provider 合并后服务拒绝放弃 |
| 受影响 console 包回归 | [console-final-04/summary.json](console-final-04/summary.json)，66 项原布局/编辑/分页/守卫及新增候选回归通过，无 fail/skip |
| Windows/Linux 源码依赖代表 | [build.json](build.json)，两平台编译及 SHA-256；Linux 为交叉编译，无平台运行或 CC5 声明 |
| 最终 L1 | [l1-final/summary.json](l1-final/summary.json)，本次唯一一次固定 L1 通过（7 组，无 fail/skip）；1.2s 准备，229.6s 执行 |
| CC5 | **用户人工确认通过（2026-10-04）**，见[人工确认记录](../candidate-interaction-ux/manual-acceptance-20261004.md)。此前工具拒绝/准备日志保留为历史，未作为原生通过依据。 |

## 证据边界与已修复诊断

`wiring-01` 为编译诊断（未使用 import），`wiring-02` 为接线 fixture 未发送 WindowSizeMsg 的错误，`wiring-03` 为初始测试错误地把服务的通用 `code=failed` 当作具体错误码；现按实际 `status=rejected` 加完整 `message=candidate_limit_reached: ...` 验证原因与候选数不变，没有修改服务或削弱业务断言。`wiring-final-02` 记录了外部控制请求完成后 UI fixture 未再读取最新问题快照导致反馈轮次等待失败；已增加提交前显式刷新和草稿打开/提交诊断，原会话/轮次断言保留，没有放宽。WaitControl 的最后参数是轮询周期，不是超时，已采用 100ms，业务等待仍有独立 deadline。

`view-01/02`、`view-final`、`console-final`、`wiring-04`、`wiring-final` 等是实施中通过快照；最终源码结果以表中选择为准。所有失败与通过的原始 JSONL/stderr 均保留，不覆盖旧产物、不把早期失败记作通过。`windows/prepare` 保持模式没有执行普通接线后半段，不属于完整 wiring 通过记录。

亲自复查实际 diff、当前 Spec/Plan 与生产路径：Task/run/Execution/session 选择、规范 Body/Conversation/Activity、草稿/请求持久化、产品控制/反馈、环境动作和回执验证。修复了迟到读取清除新候选 busy、缺页改选兄弟、merged 状态误当获胜、原冻结配置缺少 run 直读、迟到拒绝/开始回执改变当前选择等问题。worker-only 发布、独立 environment blocker、原业务 Decision 和服务 intent/receipt/ack 屏障继续由原服务实现，本 PR 未新增进程 kill、Task 丢弃、资源删除或自动补投路径。

此前因 CC5 原生证据缺失保持 draft。现由用户明确授权实现先合入并继续 #39 开发；CC5 仍为待人工补验，不把该合入例外记为验收通过，历史失败/诊断及固定任务身份保留。

2026-10-04 追加授权：用户已授权原生终端操作；实际 UI 工具仍返回 `product policy blocks this app: ...powershell.exe`。保存新现场、实际拒绝、公开 Task 查询与进程清理证据；修正 opt-in manifest 未带出受控 Git/history 环境绑定的问题。生产实现未改动，不重跑原生输入或伪造 CC5；本次补充仅选择测试准备能力验证，原最终 L1 不重复。

授权补充验证：[同一接线回归](windows/bindings-regression/summary.json) 2 项通过、无 fail/skip；[正式 manifest 的外部 CLI 检查](windows/authorized-bindings-check/check.json) 使用自动写出的三项环境绑定，读取同一 Task 901 / version 2 / 五候选后通过原 stop_file 清理。该准备能力结果不替代 CC5，生产实现未改动。

## 历史合入授权与人工补验交接

2026-10-04，本协调聊天用户先确认“#38 我来人工补验”，随后明确要求：“合并#38，标记待人工补验即可继续开发#39”。协调按此限定例外记录 accepted_exception 并完成实际平台合并后，才启动 #39；原生条件不删，CC5 仍未通过。该例外不扩大到 #39 CN1–CN8、远端准入或其他尚缺证据。

协调复审修复了放弃/落选/多合并的冲突展示和活动契约入口，67 项 console、最终真实接线（2 项）及 L1 七组已通过；同一 run 中的初始 stale_instance 失败、诊断与最终通过都保留。同步 PR #211 后另按 Plan 核对 PR 文本链接和测试子进程清理的受影响消费者。最终 head/base、实际命令与合并事实在 PR 评论及 run 状态中记录，不用本段文字代替平台证据。

CC5 由用户按 [Windows 人工操作步骤](windows/README.md)完成，取得实际终端/输入法、中文组词、选项加备注、候选导航、窄窗口及滚动证据后，再追加人工通过记录。准备/保持/清理、Go KeyMsg、授权和 PR 合并均不替代这些证据。

## 人工补验收口

2026-10-04，用户明确确认“交互ok”，并要求把 roadmap 人工未验收项记为通过。CC5 现按[用户人工确认记录](../candidate-interaction-ux/manual-acceptance-20261004.md)关闭待验事项；上文此前未通过/延期例外均为历史记录，不代表当前状态。不扩展为 #39 或其他原生验收通过。
