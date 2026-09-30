# Spec Codex 执行适配器迁移

任务 ID：57；任务名称：Codex 执行适配器迁移；目标：`EXECUTION-ISOLATION`。状态：已实施，待合入。配套 [Plan](execution-isolation-04-codex.plan.md)。硬前置：[#55](execution-isolation-02-workspace.spec.md)、[#56](execution-isolation-03-publication.spec.md) 合入，一个实现 PR 交付。

## 目标与范围

将中心的 Codex 平台操作、原生证据解释与恢复读取移入 `codexworker`，遵守[共同约束](execution-isolation-constraints.md)。本轮保持已有权限、model/effort 默认语义、发布责任和人工恢复门槛，不新增 CLI 能力或实际权限规则。

## 迁移落点

新增 `backend.go`、`config.go`、`permissions.go`、`command.go`，复用已有 launch/connection/observe/ask/journal/capability/outcome/event_fact 文件。主要消费者来自 `codex_worker.go`、`codex_worker_reply.go`、`codex_worker_recovery.go`、`codex_worker_evidence.go`、`worker_mode.go`、`worker_evidence.go`、`worker_resume.go` 与 `codex_conversation.go` 中属于执行现场的部分。

| 层 | 本轮所有权 |
| --- | --- |
| `codexapp` | WebSocket/JSON-RPC、原生请求与参数序列化 |
| `codexworker` | 启动/连接/停止、thread/turn 操作、问题回答、日志读取、generation/进程/原生 ID 核验与事实归一 |
| 中心 | 领取、准备/输入意图、用户恢复授权、消费/游标提交、业务转换、发布与交付事务 |

固定操作输入来自 execution 契约，workspace 资源来自 #55，发布请求来自 #56。适配器返回部分绑定、请求身份、观察句柄及类型化回执；中心在派发下一动作前持久化它们。不得用回调传入整个中心来隐藏业务写入。需要取消未登记 observer 时返回可取消的现场句柄，不要求公共接口认识中心 Worker。

Command、CodexAskCall 与冻结 Profile 归 contracts，Question/ProcessIdentity/动作与错误归 execution。本轮按真实所有者复用，只迁解释与校验，不复制持久记录或重写 01。execution 不导入 contracts，适配器配置/行为不被持久层反向导入；必要记录到动作输入的转换归中心。

主干已有 `worker_start.go` 的启动协调、`workerflow/recovery.go` 的恢复规则及 `worker_attachment_store.go` 的原子绑定。继续由中心按保存/调用/回执顺序驱动，不在新 backend 内复制一个自动推进的生命周期循环。既有 OS 进程承载通过最小受管进程端口注入，不让 codexworker 反向导入主包平台 helper。

## 原生证据与恢复

实时处理、历史补读、恢复对账使用同一原生解析及身份核验。thread、turn、call、generation 与 endpoint/process 绑定校验归 Codex 实现，中心核对 Task/Execution/代次和当前请求。回答正文证据仍匹配已保存答案，迟到回执不能写回终态或另一轮。

恢复读取和恢复输入分开：服务重开可读取及接回观察，不能自行 resume/start turn 或补发答案。用户明确选择后，中心才提交固定恢复意图；新进程或新会话须保存真实代次及证据，不冒充旧会话。

## 启动命令交接

Codex CLI argv、model/effort、现有 sandbox 参数、Git 附加目录与 profile 处理由 Codex 层生成。Orca shell 包装暂留原包。本任务迁入已有 `orcaworker.FixedAgentCommand` 的 Codex 参数决定及 `ValidateManagedArgs`，由同一个 Codex 生产者提供 #54 的固定命令契约，#58 只接 shell/terminal。命令、参数顺序及权限契约摘要与当前保持一致，不在本轮补上新的项目策略。

Windows Orca 当前有/无 model 均受管，model/effort 可分别省略；保留 approval=never、strict-config、固定 Git 根、排除隐式临时根及原 args 安全集合，不能恢复 opaque agent 分支或 model/effort 成对必填。原生启动设置在环境绑定中仍可与命令/根目录逐次对照；搬包不改变冻结摘要、probe 脚本或收据关联。

上述命令生产者用于本机路径。远程 orca_remote_worker 的原生 worker-start/--agent/--model/--effort 与原 terminal 续聊入口继续归独立协议，不替换成中心 Codex argv 或本机 shell，也不把本机可省略 model/effort 的规则扩到要求显式二者的远程预检。

特别保留主干本地 Windows 设置：受限 local 缺省 elevated 在摘要前规范化并冻结，workspace-write + unelevated 领取前拒绝，read-only 的显式 unelevated 保持可用。`codexapp.LocalSandboxArgs` 的实现解释移入 codexworker 权限层并更新调用/测试，codexapp 只序列化已编译原生参数；不复制另一份 Windows 校验，前五轮配置字段仍保持当前表示。

实现配置解释归 Codex Config，现有 contracts 固定记录的 JSON/字段保持当前行为；#59 引入项目策略来源，#61 完成注册。不能为把具体 Config 塞进共享快照让 contracts 导入 codexworker。中心负责转换/构造，不把业务路径的原生 method 判断留给 #61。

## 最小验收

| 编号 | 通过条件 |
| --- | --- |
| CX1 | codexworker 不依赖父包/SQL，中心的 Codex 生命周期和恢复业务消费者不解析原生协议或日志格式 |
| CX2 | 两轮反馈保留同一逻辑会话与工作区，原生回执可恢复，回答证据不匹配及旧轮次回执拒绝 |
| CX3 | 停服观察不自动发送已保存输入，意图写入失败不启动/发送；新会话选择仍需原人工授权 |
| CX4 | 固定 CLI/app-server、local Windows 规则及 Orca 无 model 受管/独立 effort/安全 args 语义保持，命令和冻结证据不漂移 |

使用既有 fake app-server 与命令 fixture。无行为变化不要求新真机认证；若实施中必须改变启动行为，须单独说明原因并补必要隔离证据，不能当作机械迁移隐藏。

## 实施记录

最终所有者、迁移测试路径、实际最小验证及基线剩余项见[验收记录](../validation/execution-isolation-04-codex/README.md)。
