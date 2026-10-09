# Spec：本机 Orca 原生通用 worker 与权限委托

任务 ID：86；任务名称：本机 Orca 原生通用 worker 与权限委托。配套 [Plan](orca-native-worker.plan.md)、[草稿](../roadmap/drafts/orca-native-worker.md)和[专属验收](../acceptance/orca-native-worker.md)。用户在 2026-10-09 本会话确认实施以下范围。

## 目标与所有权

SteerLine 本机 Orca 路径只依赖 Orca 的原生启动、worker、消息、观察及资源接口。agent 是透传配置值，model/effort 是通用原生启动偏好；具体 agent 的二进制、版本、会话文件、工具格式及权限参数均由 Orca 管理。本任务不创建 Claude 专属 adapter，不增加通用编排框架。

第一阶段采用 Orca 自己的 Agent Permissions / YOLO 宽松运行设置：使用当前用户权限，由 Orca 原生 launcher 处理各 agent 的具体实现。SteerLine 不写 Codex/Claude 设置、不拼接权限绕过参数、不声称已有 OS 隔离。Orca 用户设置中的真实等待/拒绝仍据实显示；没有公开权限设置或证明接口时，不伪造 capability 或把配置标签当生效证据。自动现场必须落实宽松模式并记录 Orca 原生运行事实。

本地 Codex 路径保持自身沙箱、自动审核及原生会话职责。远端存量不进入本轮。默认不兼容以前开发格式；变化的本机 Orca profile / 状态必须明确拒绝旧格式，不静默重启、迁移或丢弃现场。

## 配置与启动

- 本机 Orca profile 只需要 Orca 自身配置，不接受 codex.worker-profile；旧的 agent 可执行文件/权限字段在副作用前拒绝。
- 目录注册支持通用 Orca agent 值，同一实现处理任意已配置 agent；真正可用性由 Orca 原生请求核验。配置示例的 codex/claude 值不构成专属实现分支。
- 工作区按固定仓库/base 创建，原生 Orca launcher 启动配置 agent；不构造 codex-session-launch，不读取或绑定 agent 私有 home/history。
- 派发使用 orchestration worker-start，首次选定 agent，或复用已核实的原 worker terminal；后续轮次只复用原会话。model/effort 通过 Orca 通用参数传入，不解析具体模型命名。
- 领取、启动、投递、停止的外部意图与回执继续持久化；超时或部分效果保留原 Run/Task/Dispatch/请求 ID 对账，不盲重发。

## 就绪、观察与结果

移除 SteerLine 本机 Orca 的 Codex 沙箱探针、特定 TUI 字符匹配和专属工具证据门槛。最小就绪由 Orca 原生 worker/输入回执核验；真实工作区操作在产品自动场景验证，不增加独立模型准备轮次。

统一读取 Orca worker-show / worker-read 与 Run inbox 的事实。原生 agentWait、连接、状态、输入接受和轮次启动分别表达，缺失字段保持未知。final/结构化结果通过 Orca 统一 transcript/结算材料采集，不能直接访问 Codex/Claude 原始日志；terminal tail 不当成完整 final。

worker_done 由 worker 使用 Orca 原生消息接口发送；不再为了 Codex Windows 沙箱 IPC 限制注入 SteerLine 文件 relay。中心继续核验原 Task/Dispatch 和真实结算，再由既有 worker 流程及 Git/PR/provider 核验业务产物。worker_done、轮次结束或自然语言成功均不直接完成 TaskDelivery。

反馈、停服恢复、原身份验证、退出及资源清理保持已有通用业务要求。原目标不可核实时阻断，不能创建替代会话或把静默视为成功。特定 agent 的启动菜单、工具失败和会话解析归 Orca；SteerLine 保存 Orca 给出的诊断。

## 验收

| ID | 自动要求 |
| --- | --- |
| ONW-A1 | 本机 Orca 生产路径没有 Codex/Claude adapter、启动参数、会话/工具/TUI 专属代码；两种 agent 配置走相同通用接口，旧 profile 在副作用前拒绝；本地 Codex 权限回归保持 |
| ONW-A2 | 原生启动及至少两轮 worker-start、原消息反馈与输出收集保持原身份；原请求未知、身份改变、迟到结算不误重发/推进；不调用专属 readiness / relay |
| ONW-A3 | 当前用户权限下的真实 Orca 最小闭环：原生 launcher 启动，worker 实际执行工作区操作，反馈/结算/输出可关联，资源真实停止与清理；记录实际权限来源和构建，不用模拟结果替代 |

ONW-M1 为安装入口和 Orca 权限显示的后置体验确认，详见专属记录；不阻止无关后续任务。所有结果登记 acceptance，Plan 不维护实时状态。
