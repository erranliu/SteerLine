# #38 候选观察界面：实现待复审，CC5 未通过

起点：`28b81d82f4afb3e158cf96bd5cbf257fa48b7adc`，与执行开始时最新 `origin/main` 相同；分支：`codex/candidate-console-38`。实现使用当前 #37 服务/客户端合同，无生产服务、生产任务库或模型请求。

操作入口：Task 主行唯一；详情 `d` 显示候选集合，`{`/`}` 选择精确 WorkflowRun，`b` 选择不同冻结方案新增，`z` 明确不可撤销放弃，`a` 文字/选项/备注反馈。规范会话/正文 `c` / `f`、更多/最终回复 `y` / `n` 保持原查询入口。环境动作 `Ctrl+E` / `Ctrl+O` 绑定原 blocker/turn/Execution/Attempt/version/instance。

| 验证 | 实际结果/证据 |
| --- | --- |
| CC1–CC4 定向 fixture | [view-final-04/summary.json](view-final-04/summary.json)，顶层 TestCC1–TestCC4 共 4 项通过，无 fail/skip；五候选一 Task 主行/计数，精确冻结配置/会话、缺页/重排/迟到/重开、草稿与阅读位置、规范缺口忽略 raw、blocker 不自动补投、原环境目标、原请求/错误回执、放弃与停止分离、merged 关闭入口 |
| 实际应用/客户端/服务/SQLite/Git 接线 | [wiring-final-03/summary.json](wiring-final-03/summary.json)，TestCandidateConsoleWiring 及 merged_PR_service_guard 共 2 项通过，无 fail/skip；原 UI 新增、原请求重读、五候选与第六次开始拒绝、原会话反馈、精确退出、真实 provider 合并后服务拒绝放弃 |
| 受影响 console 包回归 | [console-final-04/summary.json](console-final-04/summary.json)，66 项原布局/编辑/分页/守卫及新增候选回归通过，无 fail/skip |
| Windows/Linux 源码依赖代表 | [build.json](build.json)，两平台编译及 SHA-256；Linux 为交叉编译，无平台运行或 CC5 声明 |
| 最终 L1 | [l1-final/summary.json](l1-final/summary.json)，本次唯一一次固定 L1 通过（7 组，无 fail/skip）；1.2s 准备，229.6s 执行 |
| CC5 | **未通过**。[用户授权后的实际工具拒绝](windows/authorized-20261004/diagnostic.json) 和[实际准备/清理记录](windows/authorized-20261004/prepare/summary.json) 仅证明隔离准备能力；没有原生 IME/输入/导航/窄窗/滚动操作证据。[blocker 与人工操作方法](windows/README.md) |

## 证据边界与已修复诊断

`wiring-01` 为编译诊断（未使用 import），`wiring-02` 为接线 fixture 未发送 WindowSizeMsg 的错误，`wiring-03` 为初始测试错误地把服务的通用 `code=failed` 当作具体错误码；现按实际 `status=rejected` 加完整 `message=candidate_limit_reached: ...` 验证原因与候选数不变，没有修改服务或削弱业务断言。`wiring-final-02` 记录了外部控制请求完成后 UI fixture 未再读取最新问题快照导致反馈轮次等待失败；已增加提交前显式刷新和草稿打开/提交诊断，原会话/轮次断言保留，没有放宽。WaitControl 的最后参数是轮询周期，不是超时，已采用 100ms，业务等待仍有独立 deadline。

`view-01/02`、`view-final`、`console-final`、`wiring-04`、`wiring-final` 等是实施中通过快照；最终源码结果以表中选择为准。所有失败与通过的原始 JSONL/stderr 均保留，不覆盖旧产物、不把早期失败记作通过。`windows/prepare` 保持模式没有执行普通接线后半段，不属于完整 wiring 通过记录。

亲自复查实际 diff、当前 Spec/Plan 与生产路径：Task/run/Execution/session 选择、规范 Body/Conversation/Activity、草稿/请求持久化、产品控制/反馈、环境动作和回执验证。修复了迟到读取清除新候选 busy、缺页改选兄弟、merged 状态误当获胜、原冻结配置缺少 run 直读、迟到拒绝/开始回执改变当前选择等问题。worker-only 发布、独立 environment blocker、原业务 Decision 和服务 intent/receipt/ack 屏障继续由原服务实现，本 PR 未新增进程 kill、Task 丢弃、资源删除或自动补投路径。

#38 活动 roadmap 收缩为 CC5 原生输入验收残留，已实现 Spec 不作为后续强制必读；尚未完成验收，不解除 #39 前置，也不启动 #39。本 PR 保持 draft，供协调聊天独立复审，不合并。

2026-10-04 追加授权：用户已授权原生终端操作；实际 UI 工具仍返回 `product policy blocks this app: ...powershell.exe`。保存新现场、实际拒绝、公开 Task 查询与进程清理证据；修正 opt-in manifest 未带出受控 Git/history 环境绑定的问题。生产实现未改动，不重跑原生输入或伪造 CC5；本次补充仅选择测试准备能力验证，原最终 L1 不重复。

授权补充验证：[同一接线回归](windows/bindings-regression/summary.json) 2 项通过、无 fail/skip；[正式 manifest 的外部 CLI 检查](windows/authorized-bindings-check/check.json) 使用自动写出的三项环境绑定，读取同一 Task 901 / version 2 / 五候选后通过原 stop_file 清理。该准备能力结果不替代 CC5，生产实现未改动。
