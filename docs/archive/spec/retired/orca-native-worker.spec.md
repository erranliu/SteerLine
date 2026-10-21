# Spec：本机 Orca 原生通用 worker 与权限委托

> 归档于 2026-10-21：退役方案，原失败和未验项保留，不作为当前补验或实施队列。当前接入见[入口](../../../integration/steerrunner.md)。固定 commit/原路径及证据内容继续按原记录取证。

> 历史方案（2026-10-10 退役）：本页仅保留原契约与证据。当前 Orca 路径统一由 [单终端 Spec](../../../spec/orca-terminal-transport.spec.md) / [Plan](../../../spec/orca-terminal-transport.plan.md) 定义，旧 supervised、worker-start、Dispatch 及回传路径均已删除。

变更：本机 Orca 原生通用 worker 与权限委托。历史预留 ID 86 仅追溯，已按用户最新指令退出 roadmap；由当前聊天直接实现，不作为 #84 的前置，不创建新的执行聊天。配套 [Plan](orca-native-worker.plan.md)、[调整记录](../../design/orca-native-worker.md)和[专属验收](../../../acceptance/archive/superseded/orca-native-worker.md)。#84 在途执行使用其原固定版本，本变更不改写其 Spec/Plan 或阻塞交付。

## 目标与所有权

SteerLine 本机 Orca 路径只依赖 Orca 的原生启动、worker、消息、观察及资源接口。agent 是透传配置值，model/effort 是通用原生启动偏好；具体 agent 的二进制、版本、会话文件、工具格式及权限参数均由 Orca 管理。本任务不创建 Claude 专属 adapter，不增加通用编排框架。

第一阶段采用 Orca 自己的 Agent Permissions / YOLO 宽松运行设置：使用当前用户权限，由 Orca 原生 launcher 处理各 agent 的具体实现。SteerLine 不写 Codex/Claude 设置、不拼接权限绕过参数、不声称已有 OS 隔离。Orca 用户设置中的真实等待/拒绝仍据实显示；没有公开权限设置或证明接口时，不伪造 capability 或把配置标签当生效证据。自动现场必须落实宽松模式并记录 Orca 原生运行事实。

本地 Codex 路径保持自身沙箱、自动审核及原生会话职责。远端存量不进入本轮。默认不兼容以前开发格式；变化的本机 Orca profile / 状态必须明确拒绝旧格式，不静默重启、迁移或丢弃现场。

## 当前 Orca 原生前置

本轮支持用户在 Orca 中选择的 Terminal worker 模式；权限由 Orca 的统一 Agent Permissions / YOLO 设置提供。两者是执行环境前置，在原生验收前准备，不能用后置人工 ONW-M1 豁免。SteerLine 不修改用户全局设置，也不读取具体 agent 的权限配置。

已读取安装版 Orca 的原生实现：首次可返回 structured worker，但当前 worker-start 的 --terminal 分支只复用真实 terminal，未提供已核实的原 structured session 续轮能力。完整 structured worker 支持须先由 Orca 提供通用同会话接续接口，作为后续平台能力，不为它新增特定 agent 代码。启动前核实 Terminal 前置；组合 worker-start 同时投递输入，若回执到达后才发现不支持类型，首轮可能已经执行，须保留输入送达和首轮效果的已知/未知事实，阻断后续轮次与业务推进并按原身份及原生资格停止/释放，不声称首轮零执行。自动测试覆盖这些部分效果及未知记录；不得擅改模式、另起会话或将首次成功冒充多轮闭环。即使无 terminal 的 worker-show/read 可用，也不据此允许后续派发。

## 配置与启动

- 本机 Orca profile 只需要 Orca 自身配置，不接受 codex.worker-profile；旧的 agent 可执行文件/权限字段在副作用前拒绝。
- 目录注册支持通用 Orca agent 值，同一实现处理任意已配置 agent；真正可用性由 Orca 原生请求核验。配置示例的 codex/claude 值不构成专属实现分支。
- 工作区按固定仓库/base 创建，原生 Orca launcher 启动配置 agent；不构造 codex-session-launch，不读取或绑定 agent 私有 home/history。首轮启动只由一个 worker-start 请求拥有，不先用 worktree-create 的 agent 参数启动再调用 worker-start 创建第二个 agent。允许同一个原生请求创建工作区并启动 agent，或在固定的现有工作区中启动；需要的 Run/Task/coordinator 先取得原生回执。
- 派发使用 orchestration worker-start。首轮传 agent，非空 model/effort 通过 Orca 通用参数传入，effort 要求同时提供 model；核对 requested/effective 启动偏好与实际工作区/worker 身份。续轮绑定已核实的原 worker，不传 agent/model/effort，不创建新会话；原生平台无法继续原会话时明确不可用。
- worker 类型和资源以 Orca 原生回执为准，核验本轮 Terminal 前置。不为了适配具体 agent 建立 TUI 字符识别或私有会话恢复；structured 模式按前述平台能力边界拒绝，不从句柄前缀猜测能力。
- 领取、启动、投递、停止的外部意图与回执继续持久化；超时或部分效果保留原 Run/Task/Dispatch/请求 ID 对账，不盲重发。

## 就绪、观察与结果

移除 SteerLine 本机 Orca 的 Codex 沙箱探针、特定 TUI 字符匹配、文件IPC relay和专属工具证据门槛。最小就绪由 Orca 原生 worker/输入回执核验；真实工作区操作在产品自动场景验证，不增加独立模型准备轮次。

统一读取 Orca worker-show / worker-read 与 Run inbox 的事实。原生 agentWait、连接、状态、输入接受和轮次启动分别表达，缺失字段保持未知。当前 Orca 通用 transcript 缺少严格 final 标记。采用显式通用输出材料：worker 在工作区 .steerline-output/<workflow>/<turn>.json 写入原 workflow/turn/Orca Task/Dispatch 和完整 body，之后通过 Orca 原生 worker_done 结算。中心以原 Dispatch 身份、实际结算和完整材料建立应用定义的规范结果来源，证据标记 worker_declared，不声称 Orca 原生 phase 已保留，也不据此认定工具成功或 TaskDelivery。材料限长、严格格式、原身份及未修改正文；原结算时将正文及绑定摘要冻结在应用运行目录，资源释放后仍查询该不可变材料，缺失材料明确诊断而不宣告规范结果完整；不直接访问具体 agent 的原始日志，terminal tail 不当成完整 final。

worker_done 由 worker 使用 Orca 原生消息接口发送；不再为了 Codex Windows 沙箱 IPC 限制注入 SteerLine 文件 relay。中心继续核验原 Task/Dispatch 和真实结算，再由既有 worker 流程及 Git/PR/provider 核验业务产物。worker_done、轮次结束或自然语言成功均不直接完成 TaskDelivery。当前持续 worker 的规范 final 采集须可用，未来 #84 的结构化报告消费该通用来源；本任务不提前实现其分步流程。

反馈、停服恢复、原身份验证、退出及资源清理保持已有通用业务要求。监督 worker 的停止与结算后释放分别使用 Orca worker-stop / worker-release 的原生能力；绑定原 Dispatch、请求和资源身份，未知回执沿原请求核实。不能把既有 terminal-close 当作所有 worker 的停止/释放证明。原目标不可核实时阻断，不能创建替代会话或把静默视为成功。特定 agent 的启动菜单、工具失败和会话解析归 Orca；SteerLine 保存 Orca 给出的诊断。

## 验收

| ID | 自动要求 |
| --- | --- |
| ONW-A1 | 本机 Orca 生产路径没有 Codex/Claude adapter、启动参数、会话/工具/TUI 专属代码；两种 agent 配置走相同通用接口，旧 profile 在副作用前拒绝；本地 Codex 权限回归保持 |
| ONW-A2 | 原生启动及至少两轮 worker-start、原消息反馈与输出收集保持原身份；原请求未知、身份改变、迟到结算不误重发/推进；不调用专属 readiness / relay；不支持的 worker 类型阻断后续派发，首轮部分效果如实保留，资源按原身份处理 |
| ONW-A3 | 当前用户权限下的真实 Orca 最小闭环：原生 launcher 启动，worker 实际执行工作区操作，反馈/结算/输出可关联，资源真实停止与清理；记录实际权限来源和构建，不用模拟结果替代 |

ONW-M1 为安装入口和 Orca 权限显示的后置体验确认，详见专属记录；不阻止无关后续任务。所有结果登记 acceptance，Plan 不维护实时状态。
