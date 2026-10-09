# Plan：本机 Orca 原生编排接入单 worker

维护说明：本页保留原交付/试验契约与证据定位。活动任务范围见[一期共同约束](phase1-delivery-constraints.md)。

适用说明（2026-09-30）：重开后观察、结果结算与人工确认外部写入按[停服恢复 Plan](worker-service-recovery.plan.md)实施；原终端真实验收记录保持历史事实。

目标：`WORKER-FLOW-ORCA-NATIVE`。唯一执行依据：[Spec](worker-flow-orca-native.spec.md)。状态：实施中。依赖当前 `worker-flow-v1` 基线，一个实现 PR 交付；不创建或改写生产 Task。

## 1. 固定接口和替换范围

先对照 Spec §2–§8 与当前代码梳理生产入口，并建立 ON1–ON11 证据清单。将已经通过的平台实验作为接口依据，补测真实实现需要但现有记录未证明的最小边界：原请求 ID 恢复语义、长/多行输入、worker-read cursor 与 coordinator 重接。任何未证明的创建回执恢复均按 Spec 保留未知状态，不猜测幂等能力。

| 现有位置 | 实施改动 |
| --- | --- |
| `internal/steerline/config.go`、`execution_profiles.go`、`service_execution_profiles.go` | 解除本机 Orca worker-flow 拒绝和 Codex 强制绑定；明确 agent/启动设置来源及不支持的覆盖配置 |
| `orca_worker_terminal.go`、`worker_flow_local.go` | 用原生 Orca 适配替换 shell 包装 Codex exec/resume、prompt 分块和 events.jsonl；保留 local 后端自己的实现 |
| `worker_start.go`、`worker_state.go`、`model.go` 及对应存储 | 保存 Orca 固定身份、原生轮次和消息映射，按后端核验会话，接入当前单写事件流 |
| `decision.go`、反馈命令及会话读取 | 区分原问题 reply、活跃 send、结算后新 Dispatch；继续复用同一用户反馈入口 |
| `orca_supervised.go` 及现有 Orca DTO/调用封装 | 复用与旧步骤无关的 envelope、Run/Task/Dispatch、mailbox、reader 和 request receipt 解析；不继承远端五步和结果信封门槛 |
| 生命周期、PR 观察及 UI/DTO | 精确 terminal 清理、当前格式恢复、真实传输状态和现有 PR 交付事务接线 |

只提取本次真实共用的接口，不新建通用 agent 框架，不维护 `worker-start` 和低层派发两套可自动切换的实现。

完成条件：代码入口、数据字段所有者和将移除的 Orca Codex 专属逻辑清楚；Spec 与当前支持状态一致。对应 ON1、ON2、ON7。

## 2. 配置与持久化身份

在既有 Execution/Profile/ProcessIdentity/Operation 中增加必要的 Orca 绑定：runtime/worktree/agent terminal/coordinator/Run、每轮 Task/Dispatch 与中心 turn、问题 message/Delivery、外部 request 和 reader source/cursor。字段放在现有权威记录中，不复制主状态或 Decision。

按 agent ID 原样调用 Orca，不为不同 agent 写分支启动命令。将 model/effort/profile 等本路径无法兑现的显式覆盖设为配置错误；local Codex 字段不能被误用于 Orca。冻结 Execution 的配置和资源归属，切换 Profile 仅影响新 Execution。

如持久格式变化，按项目规则固定新格式并在任何外部副作用前拒绝旧记录，不实现旧 shell 执行迁移。当前格式 Operation 恢复必须同时实现。

完成条件：配置和存储回归覆盖非 Codex ID、配置冻结、旧格式拒绝及跨 Execution 身份隔离。对应 ON1、ON7。

## 3. 接通原生启动和观察

重接工作区准备流程：由 Orca `worktree create --agent` 创建首个 agent terminal，核实精确 repo/base/worktree/branch/handle/incarnation，保存资源所有权。为 Execution 创建最小 coordinator terminal、Run、首个 Task 和 `dispatch --inject`，在副作用前保存中心意图。

处理 agent 尚未识别、输入已接受、真实活动和最终结算的不同证据。对明确未产生副作用的 `inject_rejected/no_agent_detected` 作有界原 terminal 观察；对未知回执按原请求对账。不能用原先 Codex JSONL 的 thread.started/turn.completed 规则解释 Orca。

将 worker-show 的身份/资源事实、worker-read 的有界输出和原生 inbox 事件送入既有中心事件处理；保留 reader clipping/source 事实。中文、换行、引号及超限文本必须得到明确且完整的结果。

完成条件：可控 Orca 回执测试证明无重复资源创建/注入，正式中心能启动一次真实 Codex Dispatch 并收到真实结算；不把 input_accepted 当 started。对应 ON2、ON7、ON8。

## 4. 打通问答、活动补充和结算后续聊

将 Delivery 中的 question 按 Run/Task/Dispatch/message 去重落入现有 Decision，持久化整批消息后 ack。用户纯文字或选项＋备注通过原 message ID reply；已回答与仍等待回复的状态均从原问题事实对账，不额外创建工作轮次。

活跃 Dispatch 的补充用原生 send；结算后的反馈创建一个新 Orca Task/Dispatch，复用经过核验的原 terminal。把每条中心反馈操作绑定到明确目标，保存旧轮历史。对 send 与 worker_done 竞争、问题过期、重复/迟到结算执行 Spec §6 的规则。

保留五主阶段和 PR 观察：worker_done 结束本轮，PR provider 才能证明中心 Task 最终完成。不增加答案已应用证明、项目五步或工作区 reset。

完成条件：ON3、ON5 自动化通过；正式中心完成 ON4 的双轮真实 Codex 闭环，保存同会话上下文证据、两轮 Task/Dispatch 和原 handle/incarnation。

## 5. 完成当前格式恢复与资源清理

沿现有 Service 启动恢复重建 Execution、coordinator mailbox、当前 Dispatch、待问题、reader 及 Operation。对支持原请求恢复的命令使用 request-show/原 retry-request；创建类命令无已证明重试能力时按精确资源证据接回，无法唯一核实则保持未核实，不补建替代资源。

为工作区/terminal/Run/Task 创建、Dispatch 接受、reply/send、Delivery ack、worker_done 后新轮绑定以及 close 响应丢失设置针对性故障用例。重点验证同一用户消息不重发、旧结算不覆盖新轮、数据库记录先于 ack、中心重启不重复领取。

实现 SteerLine 所有的精确 terminal close：正常结算保留会话，最终交付/明确丢弃记录清理意图并核实归属；中心停止观察不关闭 agent；活动/未知或已被用户接管的 terminal 保留真实资源状态。Orca runtime 变化且原身份不可证明时阻止发送和关闭。

完成条件：ON6、ON7、ON9 自动化通过；真实中心在待回答问题期间重启，恢复后完成 reply 和新 Dispatch 续聊。Orca 自身重启只验证安全拒绝，不宣称原会话自动恢复。

## 6. 配置/UI 文档与交付回归

更新 Profile 可用性、启动设置来源、待决策和会话 DTO：呈现已保存、平台接受、回合活动、输出不完整及宿主审批阻断的真实区别；用户仍使用既有统一反馈入口。更新 `docs/product-design/data-model.md`、`docs/product-design/decisions.md`、相关运行说明和 worker-flow 的实际支持状态。

用隔离 SQLite 和 provider 测试 PR 错配、关闭未合并后续聊、真实 merge 唯一交付及后继解锁。维护 local Codex 的现有回归，确认新增 Orca 分支没有改变其会话验证。用两种 agent ID 的契约测试验证参数转发，第二种真实 agent 的缺失单独记录。

开发和最终候选均采用最小必要验证，复用现有覆盖，只补测新增 Orca 协议和恢复差异。使用快速入口，并追加本次受影响的包/用例：

```text
go run ./cmd/test-feedback -quick
```

按本机真机实验需要构建 Windows 候选；仅在修改触及平台共用代码且存在具体风险时增加对应平台构建或 vet。确需全量时使用 `go run ./cmd/test-feedback -workers=3`，记录扩大验证的原因。对纯文档编写只查链接、身份、实验结论和 Spec/Plan 一致性，不运行实现验收。

在隔离测试项目记录构建版本、冻结 Profile、Execution、Orca Run/Task/Dispatch、question/answer/Delivery、terminal/incarnation 和 PR 身份，逐项交付 ON1–ON11 清单。真实测试 PR 的合并由用户完成；合并前只记录中心链路已到待合并。第二 agent 未可用时报告跨 agent 真机未验证。

完成条件：Spec §9 规定的实现 PR 门槛通过，人工合并链与第二 agent 能力认证的实际状态明确，无未说明的自动回退或历史状态迁移。提交实现 PR 前核对固定任务契约；本 Plan 不授权发布生产二进制、修改生产数据库或代用户合并测试 PR。

## 测试计划（历史导航调整，非产品验收）

本次只更新设计文档位置，不改变 ON 场景的实现、限制或待补验收状态。

| 级别 / 组 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 文档静态 / doc-links | 当前仓库、Python 3 | python scripts/check_doc_links.py | 设计迁移与引用修正后 | 本地导航无缺失目标 | docs/validation/product-design-layout-2026-10-07/README.md |
| 文档静态 / task-identities、whitespace | 当前 Git 仓库 | python scripts/roadmap_task_id.py check；git diff --check | 文档交付前 | 当前身份、名称、入口一致，无差异格式错误 | 同上 |

不选择 L1/L2/L3、Full 或原生 ON 实验：本次没有改变产品逻辑、协议、测试登记或后端接线，导航检查不能补齐原生验收缺口。无需新增产品测试场景或环境能力。

## 测试计划（当前自动验收覆盖）

任务专属记录：[Orca 编排验收](../acceptance/worker-flow-orca-native.md)。原 ON1–ON11 的实际要求/证据仍按 Spec 核对；下表覆盖当前待补自动项并明确其余项的复用/不适用边界，不能仅凭候选链通过关闭 ON 全部场景。

| 验收项 | 级别 / 组 / 场景 | 环境 / 命令 | 执行时点 | 通过条件 | 证据与登记 |
| --- | --- | --- | --- | --- | --- |
| ON3-A | L3 / orca-native / orca-native.input（待新增） | Windows、真实本机 Orca/原问题和 Dispatch；`go run ./cmd/test-feedback -level=L3 -suite=orca-native -env=live '-scenario=orca-native.input' '-out=<本次空临时目录>'` | 原活动问题驱动/证据能力与登记齐备后 | 纯文字/选项＋备注原 message reply、重复提交/原 Dispatch 均符合 Spec；新反馈不能代替活动轮次回复 | 临时报告/原 ask/reply/tool 回执；专属记录 ON3-A |
| ON5-A / ON7-A | L3 / orca-native / orca-native.faults（待新增，controlled） | 隔离状态/受控 Orca 协议及 owned 资源；`go run ./cmd/test-feedback -level=L3 -suite=orca-native -env=controlled '-scenario=orca-native.faults' '-out=<本次空临时目录>'` | 精确竞争/断联/迟到/身份/审批注入完成后 | 原目标与回执不串线、不重复注入、不误关闭资源；只证明已注入语义 | 临时场景断言/回执，专属记录 ON5-A/ON7-A |
| ON5-A / ON7-A / ON8-A / ON9-A | L3 / orca-native / orca-native.observation（待新增，live） | Windows、真实原 terminal/Dispatch 与本次创建的资源；`go run ./cmd/test-feedback -level=L3 -suite=orca-native -env=live '-scenario=orca-native.observation' '-out=<本次空临时目录>'` | 原生观察/停观察/输出/故障最小驱动及登记后 | 实际可控故障、来源/gap/游标及输出边界可追溯；中心停观察不假定终端停止，本次 owned 资源处理真实核实；controlled 不能替代真机 | 临时原身份/原生日志/退出与清理事实；专属文档分项结果 |
| ON1-A / ON2-A / ON4-A / ON6-A / ON10-A | 现有 owner/候选证据复用 | 核对 Spec 与固定历史原证据；实际变更后固定受影响 Orca/执行/SQLite/接线精确用例。candidates.local 仅复用已证实的对应部分，不自动重跑真实 PR 链 | 待补验前核对；有新影响时定向回归 | 每项范围与原源码/目标一致，未证明部分明确留待验，不拼成一次全通过 | 本任务记录中的原来源、复用范围与缺口 |
| ON11-A | 跨 agent 契约 / 当前真机边界 | 现有契约用例核对不同 agent ID 原样传递；第二真实 agent 未可用时仍保留不在当前一期必需范围的未验证说明 | 实际 agent 支持范围改变才重新规划真机 | 不从 Codex 推定全 agent；不默认新增安装/环境能力 | 专属文档适用范围与原证据说明 |

能力补齐写入实施：沿既有 driver/native/evidence/testenv 新增最小活动问题回复、精确竞争/故障、输出边界与原终端观察，分别登记 controlled/live 场景/包、live 的 orca-codex chain 和本次 owned 清理。尚未实现/登记的场景不能直接执行，真实能力不支持或结果未知保持原阻塞；不另建模型调度器或生产测试入口。

执行并向专属文档登记实际源码/工具/身份、命令、逐项结果与临时证据，同步 execution-map 属于交付安排。人工测试 PR 合并若在受影响验证中确需安排，单列参与人/窗口和对应阻塞范围，不把它串入所有待补项；第二 agent 仍按当前一期范围另行规划。默认不选 Full、完整 L2/全部 live 或矩阵重跑；只修 Plan 不执行本表入口。
