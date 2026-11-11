# 重构与维护：Runner 只读能力核实

2026-11-10，按用户要求核实维护能力所需的同 worktree 多轮、停止后收尾和费用采集。本次只读源码、构建元数据及公开离线接口，没有实现、构建、启动服务、提交 Run、调用模型或创建 PR。本文记录核实依据与缺口，不代表维护功能验收通过。

产品规则见[重构与维护](../../../docs/product-design/refactoring-maintenance.md)，建议实现与最小补齐方案见[解耦设计](../../../docs/architecture/refactoring-maintenance.md)。

后续状态：Runner #141 已合入受管 worktree 复用与内部 parser 计量，本页仍保留原 `dce4dff8` 构建的核实事实，不作为新版本能力缺失声明。SteerLine 后续接入和参考费用口径以[本期 Spec](../../spec/refactoring-maintenance.spec.md)、[Plan](../../plans/refactoring-maintenance.plan.md)和[专属验收规划](refactoring-maintenance.md)为准，尚未实现或验收。

## 核实基线

| 对象 | 实际身份 |
| --- | --- |
| 本机源码 | `C:\Project\SteerRunner`，干净 HEAD `5ccc3f0aa07764657b3346aa6023b73b878d76e0` |
| 源码目录现有 EXE | `C:\Project\SteerRunner\bin\steerrunner.exe`，并非当前源码 HEAD 的新构建 |
| 安装版 EXE | `C:\SteerlineInstall\bin\steerrunner.exe`，与源码目录现有 EXE 字节一致，匹配安装配置 pin |
| 两份 EXE 的版本和来源 | `0.2.0-dev.4`，`source_commit=dce4dff8e5be8529a2a1a1d6a300f8cfd01087c1`，构建记录 `source_dirty=false` |
| EXE SHA-256 | `8839e70fe60d3852048d12aad92d6bee1427e13db8a3c3b9fd585be42d636596` |
| 资源摘要 | `910c7fa06b933be3632f862062e966200fd581b34873a395268826d44c1bbbe7` |
| 恢复格式 | `steerrunner-state/v8`，`same_build_restart=true`，`resource_rebuild=false` |

实际对两份 EXE 执行 `version --json` 和 `capabilities --offline --json`。源码 HEAD 相对 EXE 来源只有 #136 的待发送阶段重建退役变更；本次核对的 engine 控制入口、runstate 控制资格、usage 采集和模型用量说明没有差异。下列源码链接固定到 EXE 来源提交，不以源码 HEAD 冒充已安装构建。

## 同一 worktree 多轮执行

| 现有路径 | 已确认行为 | 对维护的限制 |
| --- | --- | --- |
| `execute/1` | 可写工作区中的一次调用方输入，不自动进入发布流程；结束并停止本次执行 | 明确拒绝 reply、resume、message、interrupt、close；成功一轮后不能继续原 Run |
| `development/2`、`development-audited/2` | 实施成功后自动进入发布；`pr_ready` 时可在原 worktree、会话、模型和权限下追加 message | 追加消息必须在发布后且原 Run 未关闭、未停止、输入空闲；不能据此实现发布前的维护多轮 |
| `conversation/1` | 只读目录中的多轮 message 和当前轮次 interrupt | 权限和目录访问限定只读，不能作为可写维护会话 |
| 新建 `execute/1` Run | 当前请求仅声明仓库、基线等输入，冻结时按新 Run ID 生成分支与工作区 | 没有引用原受管 worktree 的公开选择器；`repository.local_path` 表示源仓库，不是接管原 worktree |

依据：[工作流契约](https://github.com/erranliu/SteerRunner/blob/dce4dff8e5be8529a2a1a1d6a300f8cfd01087c1/docs/workflows/README.md#通用单次执行-execute1)、[控制入口](https://github.com/erranliu/SteerRunner/blob/dce4dff8e5be8529a2a1a1d6a300f8cfd01087c1/internal/application/engine.go)、[资格投影](https://github.com/erranliu/SteerRunner/blob/dce4dff8e5be8529a2a1a1d6a300f8cfd01087c1/internal/runstate/record.go)、[请求与冻结](https://github.com/erranliu/SteerRunner/blob/dce4dff8e5be8529a2a1a1d6a300f8cfd01087c1/internal/application/freeze.go)。

结论：同一存活开发 Run 在 PR 后的多轮已经存在；维护所需的“发布前多轮、停止后仍沿原 worktree 推进”没有完整公开路径。不能笼统表述为 Runner 完全没有多轮能力，也不能说已有 message 已满足维护。

## 停止后收尾

`execute/1` 的 stop 关闭本 Run，控制资格明确不再接受 resume 或新输入。对本机结构化 Codex 开发 Run，确认关闭原会话后 `ResumeRequired=false`；源码也拒绝已停止结构化开发会话的 resume。`resume` 是原资源资格核对，不负责重建工作区或创建替代会话。

现有 `interrupt` 仅限 conversation 的当前轮次，通过已有原生 turn interrupt 实现；它与整 Run stop 是不同效果。当前不能将其用于可写的 execute 或开发 Run。

因此，“停止当前执行后结束并交付”若只使用现有 stop，再给同 Run 发收尾提示词，会被拒绝。另开普通 Run 又会创建新 worktree。最小缺口是：**已核实停止后，允许一个明确的新 Run 排他使用原受管 worktree，保留分支和未提交成果。**旧 Run 保持终态，新会话作为新的执行事实登记，不称为旧会话恢复。

依据：[stop 结算](https://github.com/erranliu/SteerRunner/blob/dce4dff8e5be8529a2a1a1d6a300f8cfd01087c1/internal/application/control.go)、[内容中断](https://github.com/erranliu/SteerRunner/blob/dce4dff8e5be8529a2a1a1d6a300f8cfd01087c1/internal/application/content.go)、前述 engine 与 runstate。

## 费用采集

安装版公开 `run-usage/v1` 和 `run-usage-events/v1`，`scope=main_session`、固定快照分页及 `configured_model_attribution=frozen_stage_input` 均存在。事件已含 `run_id`、`attempt_id`、`command_id`、原生会话/回合、Token、`step_id`、配置模型和推理强度。

本地 Codex 采集从 Runner 自有 app-server 日志读取 `thread/tokenUsage/updated`，按原 Run、命令、会话与回合关联，按稳定事件身份去重。采集入口没有仅限 development 的本地工作流筛选；内容流程也冻结 ExecutionModel、StageInput 并保存 StageResult。因此，generate、execute 的本地主会话有可复用的采集与模型归属链路，不需要为 maintenance 新建 Token 解析器。但本次没有用真实模型证明三类维护调用的实际费用采集已经通过。

| 范围 | 核实结论 | 最小后续工作 |
| --- | --- | --- |
| 本地 Codex 原主会话 | Token、命令身份、配置模型、覆盖状态和固定快照已实现 | 对维护的决策、单轮执行、独立审计各自登记 Run，补受控和真实链验证 |
| 同一行动多个 Run | Runner 提供逐 Run 事实；SteerLine 现有费用页面绑定普通候选 | 在维护业务聚合各 Run，按事件去重并复用已有价格计算；不让 Runner 计算业务预算 |
| 同一 Run 的轮次归属 | Runner 事件已有 command_id/attempt_id；SteerLine 当前 UsageEvent 投影未保留这些字段 | 需要按输入区分时补解码；采用一轮一个 Run 时可以先按 Run 归属，不能重复累加累计快照 |
| 独立 LLM parser、子 Agent、工具内部模型 | 明确不在 main_session 覆盖范围内 | 作为完整费用的真实缺口保留，不能以主会话总量冒充全部成本 |
| Orca Codex | 能力目录声明 local 与远端 collection/production binding 支持 | 不据此宣称维护完整链路已支持远端 |
| Orca Pi | 能力目录声明 `collection=false`、`PRODUCTION_BINDING_UNSUPPORTED` | 当前不能依赖 Pi 支撑美元预算；无需为本地 Codex 首版扩大范围 |

最小费用方案是让维护自身发起的决策、执行、审计和交付模型调用各有公开 Run/输入归属，优先使用已有主会话计量，跨 Run 汇总。采用一次结果和显式报告文件，避免本方案再引入未计量的内部 LLM parser。任意嵌套模型调用的完整采集不因此得到解决；在不能限制或核实这些调用时，完整美元预算资格仍有缺口，只能如实展示已覆盖估算，不能宣称总费用完整。

对应的最小 Runner 补齐建议是：把自身内部 parser、可追踪子调用的父 Run/命令、独立来源和冻结模型登记到原用量账本，新增明确区别于 main_session 的全部可归属调用范围，继续使用固定快照和覆盖事实；不可追踪的工具内部调用公开为缺口。SteerLine 负责跨 Run 汇总并对父子来源去重。该建议尚未实现，不能把新增范围名称或未来来源登记当作已经采集到 Token。

依据：[模型用量契约](https://github.com/erranliu/SteerRunner/blob/dce4dff8e5be8529a2a1a1d6a300f8cfd01087c1/docs/model-usage.md)、[事件结构](https://github.com/erranliu/SteerRunner/blob/dce4dff8e5be8529a2a1a1d6a300f8cfd01087c1/internal/usage/api/api.go)、[本地采集](https://github.com/erranliu/SteerRunner/blob/dce4dff8e5be8529a2a1a1d6a300f8cfd01087c1/internal/usage/collect/appserver.go)、[配置归属](https://github.com/erranliu/SteerRunner/blob/dce4dff8e5be8529a2a1a1d6a300f8cfd01087c1/internal/usage/collect/collector.go)、SteerLine [费用读取](../../../internal/app/runner_usage.go)与[事件投影](../../../internal/runnercontract/usage.go)。

## 安装版离线反例

通过 `steerrunner.exe validate --kind run --input - --json` 执行四个纯格式探针。输入使用固定虚拟仓库、完整占位 commit、普通 Spec/Plan 路径，不读取这些材料，也不创建任何执行资源。

| 探针 | 实际退出码与结果 |
| --- | --- |
| 合法的本地 Codex execute/1 单次请求 | 0，`ok=true`，作为正对照 |
| 在同一请求 repository 中增加 `existing_run_id` | 2，`invalid_input`，嵌入输入契约不接受；此为说明缺少选择器的反例，不是建议字段名 |
| conversation 使用 full_access 执行和目录访问 | 2，`invalid_input`，明确要求只读目录访问 |
| 使用尚不存在的 workspace-session/1 | 3，`unsupported_schema`；此为不存在工作流的反例，不是实施要求 |

以上证明公开 gate 的接受与拒绝边界，不代替真实同 worktree 续作、停止后收尾、发布或计费验收。本次未运行 Runner 单测、L1/L2 或真实链；现有源码测试仅作为已读实现依据，没有记作本次复跑通过。
