# 执行配置隔离共同约束

本文固定项目执行抽象、适配器职责及迁移规则，由各轮独立 Spec 和 Plan 引用。本文没有任务身份，不单独启动实现，也不代替各轮范围和验收。适用目标：`EXECUTION-ISOLATION`。

## 业务与实现边界

项目定义执行权限、逻辑仓库与工作区要求、发布责任和必要能力。全局执行方案定义目标、agent、model、reasoning effort 和实现选项。每次领取固定两者的有效组合，资源创建后追加实际绑定，不能从后续配置变化补造已有 Execution 的事实。

中心是业务写者，管理 Task、WorkflowRun、Execution、Decision、Operation、消息消费和 TaskDelivery。平台层执行固定意图并返回事实；公共契约不携带可写 Service、Runtime、Run 或业务事务。副作用前意图持久化，成功回执后提交事实；未知结果保留原请求和资源身份，不盲重发。平台确认或消费游标推进在必要内容持久化之后。

输入接受、轮次开始、提问、轮次结束、发布和真实合并分别表达。普通反馈属于原会话；停服恢复遵守[停服恢复 Spec](worker-service-recovery.spec.md)的人工作业门槛，观察重接不自动发送新输入。明确授权的新会话拥有新代次，不能冒充原会话，也不能形成两个未排除活动可能性的 worker。TaskDelivery 继续使用既有唯一事务及真实 provider 合并证据。

## 包与依赖

| 所有者 | 职责与允许依赖 |
| --- | --- |
| `execution` | 公共类型、小接口、策略与纯校验；不导入中心、数据库、具体后端或执行 I/O |
| `workspace` | Git 与工作区操作、资源解析；使用公共契约，不写业务库 |
| `publication` | 发布与 PR 操作、发布责任规则；使用公共契约，不执行 Task 结项 |
| `workerflow` | 纯业务状态规则，使用固定任务与必要契约；不操作平台、文件、进程或数据库 |
| `codexworker` | Codex 操作、命令与权限编译、原生身份和证据；依赖 `execution` 与 `codexapp` |
| `codexapp` | app-server 客户端、传输与原生参数序列化；不决定项目策略 |
| `orcaworker` | Orca CLI、资源、终端、派发和对账；依赖公共契约，不导入中心或自行解释 Codex 权限 |
| `steerline` | 配置加载、实现组装、调度、事务、查询及 UI 接线；原生配置解释委托对应实现 |

平台专有配置及持久绑定由对应实现定义。中心可保存有版本的原生证据并在组装处选择具体解码器，但共同业务判断只使用类型化事实，不重新解析 JSON、method 名或 terminal tail。不得以一个任意 map、整份 Runtime 或无法定位的 opaque blob 代替契约；原始证据引用和 native ID 命名空间必须可核验。

配置/执行展示复用 `QueryService.ReadProjectView` 的同一读取快照，冻结值从该次 Domain/Runtime 投影取得，不另查询记录拼成混合版本。保留部分只读结果和错误标识，快照不完整或连接过期不放行写动作；Ready 与展示可用性不代替领取事务校验，进程存活观察独立表达。

发布责任的值类型 `execution.PublicationPolicy` 在 #60 的 `execution/publication.go` 中定义，`publication/policy.go` 负责发布能力匹配和具体校验。workerflow 只导入公共值类型，不导入含 Git/HTTP 操作的 publication 包。通用 PR URL 线索提取归 `execution/event.go`，供两类适配器复用；PR 归属及合并校验归 publication 与中心。

只为实际消费者建立接口。共同意图、身份、证据引用和能力结构由 #54 固定；Git/PR 实现各用必要的输入。Codex 启动命令以 executable、独立 argv、工作目录和非秘密环境引用表达，Orca 负责 shell 包装，不把已经拼接的任意命令当成可追加配置的通用入口。

## 迁移与版本

每轮建包即迁入完整职责，更新生产调用与相关测试并删除原实现。不单独交付空包，不长期保留类型别名、转发或双实现。前五轮保持已有行为，尚未迁移的当前调用只能继续使用唯一原实现，不能复制到另一条路径；每轮明确剩余消费者及后续所有者。

类型移动尽量保持当前 JSON 与数据库表示。确需改变格式时按[项目规则](../../AGENTS.md)更新版本并在外部副作用前拒绝旧格式、保留现场，不迁移、双写、自动降级或跨后端回退。当前格式内的恢复、幂等与并发校验不能减免。不启用既有未认证的 supervised 路径，也不重新引入五步编排或中心项目测试门槛。

## 前置与共享修改

各轮 Spec 自己固定 task ID、硬前置、接口交接和共享冲突。准备性阅读或低层实验不能绕过领取硬前置。并行包内工作不等于共享中心文件可同时合入；公共契约集中修改，后合入方更新主干并运行受影响接线回归。

主干已完成[协议事实](architecture-01-worker-protocol.spec.md)、[显式原子提交](architecture-02-worker-commit.spec.md)和[Runtime 边界](architecture-03-runtime-boundary.spec.md)的相关实现，worker 业务接线已拆为 start/events/completion/feedback/pr/recovery 等文件。复用 `service_commit.go`、`worker_commit.go`、`worker_attachment_store.go` 的事务和绑定/回执附件，不重新引入 Runtime 写回、文件绑定写入或第二个业务提交入口。本系列只迁移这些边界的消费者与实现所有者。

所有静态配置/model/参数校验须在任何外部副作用前完成。领取前的能力查询可以运行固定、短命的只读版本/help 查询，使用隔离临时文件并精确清理；不得创建 worktree、worker 会话、terminal/Dispatch 或初始化 sandbox。区分这种宿主能力查询与在实际 worker 权限内执行的就绪证明，不把其中一个推定为另一个。

## 最小验证与交付

每轮复用受影响 fixture 和确定性用例，只补缺少的接口、身份、策略或事务接线断言。用 `go list` 检查新包依赖，运行受影响包与少量中心接线测试；不默认运行全量或平台故障矩阵。实际启动、权限变化另补必要的隔离原生证据，不把 mock 写成真机认证。确需全量时使用既有 `go run ./cmd/test-feedback -workers=3` 入口。

每轮 Plan 给出命令与验收映射；测试因搬包改名时明确旧用例到新用例的对应，不能因正则未命中而宣告通过。文档编写只检查身份、链接、前置、格式及语义，不执行代码测试、生产部署、中心建任务、数据库更改或 PR 合并。

纯规则与公共包的依赖检查覆盖传递依赖，不能因为当前文件没有 I/O 就忽略其导入了含 I/O 的包。Plan 中的中心回归保留一份代表性事务接线；迁入新包的同名纯测试改用实际新包路径并从父包筛选删除，不同时执行重复矩阵。
