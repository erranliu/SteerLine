# 执行配置隔离共同约束

本文固定项目执行抽象、适配器职责及迁移规则，由各轮独立 Spec 和 Plan 引用。本文没有任务身份，不单独启动实现，也不代替各轮范围和验收。适用目标：`EXECUTION-ISOLATION`。

#54 的 01 Spec/Plan 已实施并保持固定。本次新增就绪/宿主交接要求面向后续迁移，不反向修改 #54 的范围或验收；#58 以现成公共契约承接平台事实，必要补充在自身 Spec/实现内交付。

## 业务与实现边界

项目定义执行权限、逻辑仓库与工作区要求、发布责任（固定责任或显式委派给所选执行方案）和必要能力。全局执行方案定义目标、agent、model、reasoning effort 和实现选项；项目授权 execution_profile 发布责任时，方案必须显式提供 worker 或 host_service。每次领取固定两者的有效组合，资源创建后追加实际绑定，不能从后续配置变化补造已有 Execution 的事实。

中心是业务写者，管理 Task、WorkflowRun、Execution、Decision、Operation、消息消费和 TaskDelivery。平台层执行固定意图并返回事实；公共契约不携带可写 Service、Runtime、Run 或业务事务。副作用前意图持久化，成功回执后提交事实；未知结果保留原请求和资源身份，不盲重发。平台确认或消费游标推进在必要内容持久化之后。

输入接受、轮次开始、提问、轮次结束、发布和真实合并分别表达。普通反馈属于原会话；停服恢复遵守[停服恢复 Spec](worker-service-recovery.spec.md)的人工作业门槛，观察重接不自动发送新输入。明确授权的新会话拥有新代次，不能冒充原会话，也不能形成两个未排除活动可能性的 worker。TaskDelivery 继续使用既有唯一事务及真实 provider 合并证据。

主干已有 readiness/business 轮次、WorkerEnvironment 和宿主恢复。准备轮不占业务 CurrentTurn/计数、不关联交付 PR；服务处理可信中心选项，文字反馈仍保存原 worker/question 目标且宿主等待时不注入菜单。接收方及权限来自中心持久记录，不信任 worker 自报的 receiver/action。固定准备 nonce/deadline、原身份重查、准备失败失效与重新检查并发校验在前五轮保持。

## 包与依赖

| 所有者 | 职责与允许依赖 |
| --- | --- |
| `execution` | #54 已固定的底层身份、动作/问题/能力/错误值及小接口；新增纯策略/需求值与校验，不导入 contracts、中心、数据库或平台包 |
| `contracts` | 共享业务持久记录、运行/查询 DTO 与严格解码，引用 execution 的底层值；不依赖 query、sqlite、中心或平台 I/O |
| `query` | 只读规则与 Workflow/Event/Task/ProjectReader 端口，使用 contracts/纯任务规则，不依赖具体存储或写事务 |
| `sqlite` | 端口的 SQL 读取及同事务原始项目记录采样，依赖 contracts/query；不推进业务、不拥有数据库初始化或写事务 |
| `workspace` | Git 与工作区操作、资源解析；使用公共契约，不写业务库 |
| `publication` | 发布与 PR 操作、发布责任规则；使用公共契约，不执行 Task 结项 |
| `workerflow` | 纯业务状态规则，使用固定任务与必要契约；不操作平台、文件、进程或数据库 |
| `codexworker` | Codex 操作、命令与权限编译、原生身份和证据；依赖 `execution` 与 `codexapp` |
| `codexapp` | app-server 客户端、传输与原生参数序列化；不决定项目策略 |
| `orcaworker` | Orca CLI、资源、终端、派发和对账；依赖公共契约，不导入中心或自行解释 Codex 权限 |
| `orca_remote_worker` | 已独立的远程配置/协议、会话/消息/对账事实；依赖 execution 与必要原生 Orca 能力，不导入中心写库 |
| `steerline` | 配置加载、实现组装、业务事务和查询投影接线；组合 ProjectReader，原生配置解释委托对应实现 |

平台专有配置及持久绑定由对应实现定义。中心可保存有版本的原生证据并在组装处选择具体解码器，但共同业务判断只使用类型化事实，不重新解析 JSON、method 名或 terminal tail。不得以一个任意 map、整份 Runtime 或无法定位的 opaque blob 代替契约；原始证据引用和 native ID 命名空间必须可核验。

配置/执行展示复用 `query.Service.ReadProjectView` 与中心 `NewQueryService` 组装入口的同一读取快照。query 注入四类只读能力；sqlite.Reader 返回原始 ProjectRecords，中心 `projectReadRecords` 纯水合 Domain/checkpoint 成 contracts.ProjectView，再通过 ProjectReader 交给 query。不能直接把 sqlite.Reader 当作完整 ProjectReader，也不能把业务投影搬进 SQL 或另查记录拼成混合版本。保留部分只读错误、新鲜度与领取事务核验。

主干已固定 contracts → execution，execution 不依赖任何项目包。#59 ExecutionPolicy、#60 PublicationPolicy、#61 ExecutionRequirements 的新增纯值及相应纯校验在 execution/policy.go、publication.go、requirements.go 定义；contracts 的冻结记录直接引用它们，不能让 execution 再导入完整记录形成环。完整快照组合/摘要与记录校验由中心对必要小输入处理，底层校验不接整份 Domain。发布能力匹配归 publication；workerflow 只用纯 execution.PublicationPolicy，不导入发布 I/O。

只为实际消费者建立接口。共同意图、身份、证据引用和能力结构由 #54 固定；Git/PR 实现各用必要的输入。Codex 启动命令以 executable、独立 argv、工作目录和非秘密环境引用表达，Orca 负责 shell 包装，不把已经拼接的任意命令当成可追加配置的通用入口。

已有原生工具事件、收据、安全文件读取和 HostFact 解释归平台适配器；准备/阻断/DeferredDecision 与业务派发条件归中心。WorkerEnvironment 仍是中心持久事实，不把其整个状态机移入后端，也不建立第二个 readiness 引擎。

#54 已唯一拥有 ProcessIdentity、ExecutionTarget、WorkspaceEvidence、Question、DecisionOption、启动命令等底层执行值。HostFact、ProbeIdentity/Receipt/ToolEvidence、WorkerEnvironment、固定 Profile 与业务记录归 contracts，并引用执行值。后续复用字段与严格 JSON，平台解释仍归实现层；不重新定义或迁移已固定的 01 类型。

## 迁移与版本

每轮建包即迁入完整职责，更新调用/测试并删除原实现，不交付空包或双份定义。现有父包 QueryService/记录及平台 DTO 的 contracts 类型别名只做当前契约接线，没有第二份实现，不按旧版兼容强制全局删除；禁止为旧版本或重复迁移路径增加别名/转发。前五轮保持行为，未迁移调用使用唯一原实现，各轮明确剩余所有者。

类型移动尽量保持当前 JSON 与数据库表示。确需改变格式时按[项目规则](../../AGENTS.md)更新版本并在副作用前拒绝旧格式、保留现场，不迁移、双写、降级或跨后端回退。保留当前本地 Codex、本机 Orca、远程 Orca 三条已接通链路及各自能力拒绝；远程取消仍 unsupported。协议/Service 回归不冒充缺失的双机真机认证，不恢复五步编排或中心项目测试门槛。

## 前置与共享修改

各轮 Spec 自己固定 task ID、硬前置、接口交接和共享冲突。准备性阅读或低层实验不能绕过领取硬前置。并行包内工作不等于共享中心文件可同时合入；公共契约集中修改，后合入方更新主干并运行受影响接线回归。

主干已完成[协议事实](architecture-01-worker-protocol.spec.md)、[显式原子提交](architecture-02-worker-commit.spec.md)和[Runtime 边界](architecture-03-runtime-boundary.spec.md)的相关实现，worker 业务接线已拆为 start/events/completion/feedback/pr/recovery 等文件。复用 `service_commit.go`、`worker_commit.go`、`worker_attachment_store.go` 的事务和绑定/回执附件，不重新引入 Runtime 写回、文件绑定写入或第二个业务提交入口。本系列只迁移这些边界的消费者与实现所有者。

[环境就绪](worker-environment-readiness.spec.md)原范围已交付，有同原 terminal/incarnation 的工具、原子收据、准备结算和业务派发证据。#57 接收已有受管启动和安全参数检查，#58 迁移平台准备/宿主事实，#59 将其改为项目策略来源；已交付能力不反向依赖尚未完成的 #59。

所有静态配置/model/参数校验须在任何外部副作用前完成。领取前的能力查询可以运行固定、短命的只读版本/help 查询，使用隔离临时文件并精确清理；不得创建 worktree、worker 会话、terminal/Dispatch 或初始化 sandbox。区分这种宿主能力查询与在实际 worker 权限内执行的就绪证明，不把其中一个推定为另一个。

## 最小验证与交付

每轮复用受影响 fixture 和确定性用例，只补缺少的接口、身份、策略或事务接线断言。用 `go list` 检查新包依赖，运行受影响包与少量中心接线测试；不默认运行全量或平台故障矩阵。实际启动、权限变化另补必要的隔离原生证据，不把 mock 写成真机认证。确需全量时使用既有 `go run ./cmd/test-feedback -workers=3` 入口。

每轮 Plan 给出命令与验收映射；测试因搬包改名时明确旧用例到新用例的对应，不能因正则未命中而宣告通过。文档编写只检查身份、链接、前置、格式及语义，不执行代码测试、生产部署、中心建任务、数据库更改或 PR 合并。

纯规则与公共包的依赖检查覆盖传递依赖，不能因为当前文件没有 I/O 就忽略其导入了含 I/O 的包。Plan 中的中心回归保留一份代表性事务接线；迁入新包的同名纯测试改用实际新包路径并从父包筛选删除，不同时执行重复矩阵。
