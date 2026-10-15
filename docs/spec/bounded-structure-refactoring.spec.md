# Spec：后端组装与最终回复状态收敛

源码追溯：已迁出的本地源码链接固定到拆分前提交；当前模块归属见 [代码架构](../architecture/current-code-architecture.md)。

本次是用户直接要求的普通开发规格，2026-10-10 用户授权本聊天实施，不分配任务编号、不进入 roadmap 调度。唯一配套文件：[Plan](bounded-structure-refactoring.plan.md) · [专属验收](../acceptance/bounded-structure-refactoring.md)。两个原候选收紧后合并为一个交付范围，预计一个实现 PR；旧编号不作为本次实施身份，实际验证与交付状态归专属验收。

首次提 PR 源码基线：2026-10-10 rebase 后的 `origin/main`，commit `8e292172d84c902e93cedb9870a658e90a9db0c4`。保留原 `21d97350` 中的 Worker 分步流程（PR #262）、Codex CLI 原会话入口（PR #264）和 Orca supervised 双目标/旧本机入口切断（PR #266），并集成 PR #267 的分步反馈修复/测试调整。解决 PR 冲突时再合入 `4786934a2ab6a9be7e38d520c8efb2a7f65fedc0`，包含 #45 的本机 HTTP 客户端成果（PR #249）；范围与同包内部收敛目标保持。以实际主干源码为准，不套用此前三后端登记或旧本机 Orca 权限接线。

## 目标与完成条件

只做两项同包内整理：app 按后端收拢实际组装行为；console 收拢最终回复状态和失效规则。减少共同文件修改和理解范围，保持行为与协议。这是代码归属和状态所有权改善，不宣称编译级隔离或未来 UI 复用能力。

完成时，既有后端局部改动通常落在其所属单元；最终回复状态只有一个修改所有者，目标重置及异步结果受理只有一个实现。旧代码、平铺字段和重复逻辑随提取删除，不能靠增加包装后编译通过结项。

最新主干已将 Orca 两种目标收敛到一个实现，后端整理的收益相应缩小，但配置/工厂/实际 backend 仍混在中央文件；本次按两个实现归属整理即可。主要收益是后端局部行为修改不再触碰中央组合，和 final 状态规则能以窄查询端口单独修改/验证；不为增加并行任务数拆更多单元。

## 后端组装

入口：[directory.go](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/app/directory.go)；所属实现：[directory_local.go](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/app/directory_local.go)、[directory_supervised.go（历史）](https://github.com/erranliu/SteerLine/blob/872dba4462a4f659d7685eb2a397211a1653c632/internal/app/directory_supervised.go)。保持一个 app Go 包，采用普通私有函数和文件归属：

- local Codex、Orca supervised 两个实现单元分别收拢 registration/codec、特有配置/target、preflight/claim 和 factory；localExecutionBackend 随 local 单元迁入，不只拆登记表。supervised 的 `target: local/environment` 是同一实现的配置分支，不拆成两个 registration、组装单元或生命周期。
- NewBuiltinDirectory 只保留版本、显式组合及当前实例的一次性目录绑定。新后端仍需显式接线；既有后端局部行为变化通常不用改中央函数。
- 真正多方共享的 policy/workspace 辅助可集中；单一消费者的辅助留在所属单元，不建立通用 helpers 层、不为文件对称复制逻辑。
- 复用现有 ExecutionRegistration、ImplementationProfileCodec、ManagedProcessHost 及晚绑定闭包。构造完成前不调用 factory；拆函数所需引用采用实例局部闭包。不新增 builder、工厂/resolver 接口、registry 对象、全局注册、反射或插件发现。

保持当前登记顺序、`builtin-execution-v4`、key/profile/codec/归一化/摘要、target/ref、readiness/claim、命令/权限写根、能力与清理语义。特别保持以下新基线事实：

1. 当前只有 local Codex 和 `host: orca_supervised` 两项登记、Codex/Orca supervised 两项 profile codec。保持 `orca/orca_supervised`、通用 agent、`orca.supervised-profile` 格式2及现有双目标归一化/冻结语义；未填 target 的远端配置仍按 environment 解释。本机不带远端路由字段，不将本机 workspace 交给 local Codex 的准备路径。
2. 每个 registration 的 ServiceStopBehavior、HostManagedPermissions（当前两项均为 false）、LocalWorkspace 与 policy resource 封套接线不变。Orca 原生管理启动权限不等于将旧 terminal 的 HostManagedPermissions=true/无 policy resource 路径迁给 supervised；supervised 两种目标均保留取消不支持、交付后保留资源、中心停服只解除观察的现有能力。
3. 原启动/执行身份、请求、冻结输入与回执保持，不重定向原工作区/会话。
4. 目录实例及 ProcessHost 不串线；构造不 probe/start 未选后端，supervised 支持和授权范围不扩大。旧 `host: orca`、`orca.worker-profile`、`orca_terminal_v2` 继续在产品目录及 ref 校验中拒绝，不重新登记、读取或转换；保留的 adapter 源码/独立测试不在本次清理范围。

## 最终回复状态

入口：[model.go](https://github.com/erranliu/SteerLine/blob/47fafc12721ceb8fa39ba1ab227872fd14513bcb/internal/console/model.go)、[final_replies.go](https://github.com/erranliu/SteerLine/blob/47fafc12721ceb8fa39ba1ab227872fd14513bcb/internal/console/final_replies.go)、[final_preview.go](https://github.com/erranliu/SteerLine/blob/47fafc12721ceb8fa39ba1ab227872fd14513bcb/internal/console/final_preview.go)。保持一个 console Go 包，引入一个私有状态组件（可名 finalReplyReader）：

- 收拢 final 索引/选定 item/pin、bodyRef/正文、preview 及各自 generation/busy/error/readAt；projectState 用命名字段持有。无匿名嵌入、旧平铺字段或双缓存；不把独立失效语义的 generation 强行合并。
- 组件只接收 ConversationQueries、context/时钟值、产品 DTO 和当前目标事实。宿主仍唯一决定项目/候选选择；组件只保存读取的固定绑定，不维护另一套导航或业务快照。
- 绑定、final/body/preview 重置/失效、事件版本合并、分页/正文完整性和结果受理归组件。提供少量完整操作，不建立字段级 getter/setter、通用事件接口或第二层 view model。
- 异步命令捕获固定目标/请求值及窄查询端口，不捕获可变组件或宿主。Model 只定位原项目、提供目标/读取模式、分发结果；渲染消费现有事件/字符串，不复制状态。
- 组件不接收完整 ConsoleClient、控制/决策写端口、反馈草稿、Model 或 projectState。焦点、布局、详情开关/滚动和 recovery 仍归宿主；原目标变化时的宿主清理保持，通过组件返回必要的目标变化事实衔接，不引入新的面板调度协议。

完整历史与当前轮预览分别处理，不收紧历史查询的 Attempt/Turn。保持适用的 project/candidate/Attempt/Turn/generation/bodyRef 迟到结果隔离、后台项目归属、规范 final/item/SourceRevision 合并、gap/失败及预览失效、游标前进、正文身份/字节数/摘要核验、预览 4096-byte 读取和 800-rune/6-line 展示限制，以及 pin、f/返回、焦点/滚动、未发送反馈。

主干已有的 `implement.report` 非完整索引查询会从 StageResults 反向选择最近 `implement.execute` 的 TurnID；完整历史不采用该过滤。宿主从当前候选派生读取模式、查询 Attempt/Turn 和受理所需目标事实后交给组件，保留现有查询与预览受理各自的判断，不擅自把查询 Turn 和 CurrentTurnID 合并为一个值。步骤/status、核验后的原实现正文与来源未核验提示仍由 `worker_stage.go` 展示；组件不拥有 StageResults 业务推进或第二份步骤正文缓存。Ctrl+N 原会话入口及 nativeSessionOpening 仍归宿主，final 查询不取得 nativeSessionOpener 能力。发现既有行为问题另行说明，不借提取修复。

## 改动边界与集成

允许 app/directory*.go 及私有组装文件、console final 实现与必要宿主薄接线、所属测试及相关文档。禁止修改 execution 业务/生命周期/退出、adapter 实现、backend/目录公开类型、product/api/client/query、存储/schema/格式版本、权限、公共测试驱动和依赖白名单。

不包含原副作用执行器提取；不新增 Go 包/框架、重写全 TUI、兼容读取/转发别名/双路径。若必须越出范围，先说明真实需要并重新定范围，不混入行为修复。

#84 与 #45 的实现均已进入本次冲突解决集成基线；本次消费实际字段与接线，不代改它们的固定规格、交付/验收状态或增加前置。实施及合入时核对最新主干，共同文件按实际差异集成并保留步骤展示、本机客户端传输、原会话入口及原身份，必要时加最小定向回归；不消费猜测的未合入接口。一个 PR 自行完成受影响架构文档，不再由另一任务代管汇总。

## 验收要求

| ID | 要求 |
| --- | --- |
| BS-A1 | 后端具体组装归属与薄组合成立，登记/codec 保持，构造无探测/启动，实例独立 |
| BS-A2 | 当前 profile/摘要、supervised 双目标/旧入口拒绝、权限/readiness/停服能力、原身份及冻结输入保持 |
| BS-A3 | 回复状态单一所有者、窄端口可独立测试，宿主不重复实现重置/失效/受理 |
| BS-A4 | 结果隔离、版本/gap、历史/正文/预览及真实 Model 导航/编辑、步骤查询/展示、Ctrl+N 行为保持 |
| BS-A5 | 真实 app 当前三轮 controlled 分步接线及 L1 组合边界通过，公开契约无扩大 |

具体步骤与结果归专属验收，没有新增人工/live/native 项。结构条件通过候选差异和正常独立 PR 复审核对，不另立产品人工验收活动。
