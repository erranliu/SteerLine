# Spec：worker 运行时资源所有权收敛

源码追溯：已迁出的本地源码链接固定到拆分前提交；当前模块归属见 [代码架构](../../docs/architecture/current-code-architecture.md)。

任务 ID：96；任务名称：worker 运行时资源所有权收敛。

状态：2026-10-14 草稿规划，未纳入执行、未实施。配套 [Plan](../plans/worker-runtime-ownership.plan.md)、[专属验收](../testing/acceptance/worker-runtime-ownership.md)、[本轮草稿](../Roadmap/drafts/roadmap.md)。调查基线 `5a24abec3c10d184a6e57962e154854bc6011a9f`；实际启动固定已提交文档及最新代码。

候选核对：2026-10-14继续保留草稿，未纳入execution-map，当前不安排实施。生命周期effect/receipt、观察/资源/确认权及actor接线与正在实施的Worker三层重构交叉；三层方案由ReliableExecution承担技术可靠性、actor组装资源，不能并行建设另一套旧边界。三层成果进入主干后重审Spec/Plan与验收，不自动恢复旧方案；原调查和测试安排保留作参考，以下旧并行建议不能作为开工资格。冲突说明见[本轮草稿](../Roadmap/drafts/roadmap.md#与worker三层重构的冲突核对)。

## 问题与限定成果

[Service](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/service_state.go) 同时持有业务候选与 workers、ownedProcesses、观察/在途状态；[lifecycle_effect.go](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/lifecycle_effect.go) 已有无业务写权的异步调用，但 pendingLifecycleEffect 保存接收可写 RunView 的 apply闭包。[worker_types.go](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/worker_types.go) 的桥接与关闭逻辑也由同一个 execution 包持有。

建立 `internal/execution/workerruntime`，收拢原观察器/执行句柄、观察取消/关闭、在途外部调用与完成结果交接的资源所有权。actor 发出已持久化、固定身份的 Operation/request，runtime 使用注入的小执行/对账/观察/停止端口，返回原身份绑定的事实、回执、结果和受控资源引用。runtime 不接收 Service、RunView可写指针或业务状态 apply闭包；actor 收到结果后独自核验、更新和提交。

本项只拆运行时资源，不重写固定流程、业务恢复资格、Decision/人工继续规则、PR交付或adapter证据。复用既有后台调用/Signal和持久屏障，不增加第二执行调度器、全局消息总线、自动重试或第二权威。

## 所有权与生命周期

| actor 保留 | runtime 收拢 |
| --- | --- |
| 领域/committed 快照、流程/Decision/恢复资格、持久 Operation状态、接纳/拒绝迟到结果、业务提交及事件确认门槛 | 原资源按 run/execution/Attempt/generation 的观察/句柄绑定、在途调用互斥、背景调用取消、观察器释放、原结果返回和已获准的资源动作 |

同一物理资源只有一个关闭所有者；actor 可保留身份/能力引用，不保留第二套可写资源 map。runtime 可据固定请求验证资源身份，不自行解释 Task closed/discarded、步骤或恢复授权。原 owned与attached句柄区分、进程树缺证、stop_execution/continue_execution 冻结策略均由现有事实与adapter能力表达。

先持久意图再调用的屏障保持；每个原运行/操作的在途约束保持。未知发送只调原 Reconcile，不再次 Execute。迟到结果必须携带原 request/run/Attempt/generation，不指向当前最新候选；业务终态后的结果由 actor 决定是否可接纳，资源仍按原归属释放。

事件桥接保留 Persisted/Processed 的两道含义，并区分成功确认与失败通知：Persisted的nil仅由actor在该事件事实持久后发出，Processed的nil仅由actor在该事件处理及所需业务提交完成后发出；runtime不能提前报告成功。对应屏障失败须以error结束仍未完成的确认，不能用nil冒充通过，也不能覆盖已完成的确认。

确认权以成功送入原actor队列为交接点：此前runtime负责未交付事件的取消/失败通知；入队成功即由actor拥有，即使同刻取消也不得由runtime再通知。入队后迟到/错绑定、提交失败和停服排空由actor完成仍未完成的确认。每个原事件的每道确认只发一次，Processed的关闭由该道唯一通知owner执行；先Persisted成功、后处理失败时仅Processed报错。runtime可为未交接事件报error，不因此获得业务写权。关闭顺序保持既有取消转发、等待交接收敛、actor排空已接事件与已提交ack、释放资源；等待不能阻塞唯一消费actor。解除观察不冒充外部执行停止，attached资源不当作owned树杀掉。

恢复仍沿既有 worker事实接口：runtime 承载重接/停止/调用资源，原恢复规则留在 actor/model。Codex与Orca local/environment 共用规范端口，各adapter保留原始证据和停服能力，不拆两个Orca实现、不替换会话。

## 修改边界与排期

本项拥有 worker_types、service_workers、生命周期效果的运行时部分及 Service 的相应资源接线；worker_lifecycle/recovery/shutdown 只修改句柄/观察/请求与结果交接。新 runtime仅依赖 backend值/消费端口与标准库，不依赖execution父包、development、delivery、task、product、storage、app或具体adapter。

Service 的 domain/committed、流程和 Decision字段、service_commit、模型/持久格式、平台codec、公开HTTP均冻结。#94拥有 staged_flow规则，本项不改它；#95拥有最终交付，本项只承接其后原资源收尾调用。必要边界guard由本项新增独立规则/测试，不扩大旧白名单。

没有新增任务硬前置。建议在 #94 后集成以降低共享actor修改，但不得把建议写成硬依赖；若启动审查发现实际需要兄弟未交付签名/状态变更，须先重新固定交接，不能猜接口开工。默认后合入者负责最小组合验证。

本项在没有#94/#95成果的基线上也必须完成原阶段推进和交付后的资源收尾验证。相关成果实际已进入合入基线时，再按Plan明确命令复验纯规则→actor→runtime及delivery→资源清理的交接；这是本项最终候选的验证条件，不要求等待兄弟任务开工。交付完成后注入清理失败仍须保持Task closed、原唯一TaskDelivery、图修订和后继资格，不重复交付或重开任务。

## 最小验收

| ID | 自动通过条件 |
| --- | --- |
| WR96-A1 | runtime 能以小端口独立测试，资源只有一个写/关所有者；无业务记录指针/提交能力或上层实现依赖，旧重复资源map/闭包逻辑删除 |
| WR96-A2 | 原身份/在途互斥、一次调用、unknown只对账、迟到结果隔离、owned/attached、观察取消/关闭及无残留资源保持 |
| WR96-A3 | 成功确认遵守原actor两道屏障；交接前取消由runtime报失败、入队后由actor处理迟到/提交失败/停服排空，每道恰好一次；慢/失败观察不阻塞其他原运行，交付后清理失败不回滚真实交付 |
| WR96-A4 | Codex原会话观察重接、Orca本机/environment原终端重接与unknown发送、恢复答案重交通过受控场景，未新派输入/替代会话；原阶段推进及实际已合入兄弟成果的最小组合接线通过 |

无新增人工/native项：本项不修改原生协议/启动命令/权限，既有原生未验保持。必要最小自动验证和能力补齐详见 Plan，结果归专属验收。
