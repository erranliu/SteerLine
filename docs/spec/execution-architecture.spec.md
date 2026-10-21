# 执行架构共同约束

本文维护当前任务可复用的执行边界，不单独启动实现任务。2026-10-21 按独立 Runner、Web 唯一前端及现行候选/恢复契约校准；固定任务仍读取其原 commit/路径，历史要求与原验收不回写为当前通过。

## 平台与职责

当前通过固定 EXE 的 `steerrunner-cli/v1` 接入本地 Codex 与远端 Orca environment，本机 Orca 路径已放弃。未发布版本默认不兼容旧配置、数据库或协议，不新增迁移、双写和回退路径。

SteerLine 管 Task/DAG、候选参与关系、原 CLI outbox、公开事实投影与唯一 TaskDelivery；独立 SteerRunner 管全部 LLM 操作、原会话、工作区、workflow、原生资源及执行结算。Web/CLI 经产品查询与命令入口操作，不成为第二业务写者。SQLite 校验身份、前值、版本和唯一约束，事务不跨越外部平台调用。职责以[整体架构](../architecture.md)为准，格式来源见[数据模型](../product-design/data-model.md#当前格式来源)。

一个 home 共享一个 Runner 服务，各项目保持自己的 Task actor、数据库和锁；应用宿主拥有共享服务，项目借用固定客户端，整体关闭统一结算。具体边界见[共享服务 Spec](shared-runner-service.spec.md)。开发 workflow 的 ID/version 取所选 Runner 构建公开目录，与 Task 的 `worker-flow-v2` 协议分别冻结；不从迁出前 development/v3 推定当前步骤或能力。

## 身份、候选与交付

Task、Candidate、Runner Run、Attempt、原生会话/轮次、命令与 PR 分别绑定。候选在领取事务冻结原任务材料、仓库/base、执行预设和完整提交；后续事实不能覆盖原身份或兄弟候选。首次冻结及必要材料检查见[项目维护](../product-design/project-maintenance.md)，候选规则见[数据模型](../product-design/data-model.md#人工候选实验)。

人工停止参与只改变参与关系并保护最后一个参与候选，不授权 Runner 停止或宣告资源结算；Task 丢弃和终态停止义务分别记录。具体要求见[候选停止参与验收](../acceptance/web-candidate-stop.md)。

任一已绑定候选 PR 经核验确已合并，即可在同一事务记录唯一 TaskDelivery、关闭 Task、释放后继并保存全部候选停止义务。其他候选已合并、状态未知或尚未到 pr_ready 不阻塞正确候选交付；closed 后停止该任务全部候选 PR 查询，重启保持。自然语言成功、轮次结束、命令 completed 或资源已停均不替代交付。具体要求与证据见[首个合并交付](../acceptance/candidate-first-merged-close.md)。

## 外部动作、反馈与恢复

副作用前持久化固定意图，原请求未知时只按原 ID 对账，不能换 ID 重发或改投后端。问题、答案保存、发送意图、平台回执、原生轮次结果与业务阶段分别表达；迟到事实不取得新输入、重建资源或推进兄弟候选的授权。详细规则归[决策与反馈](../product-design/decisions.md)及[Worker 生命周期](../product-design/worker-lifecycle.md)，不在任务中另建平行状态机。

显式重启 SteerLine 后核验新 Runner 实例；运行中的旧 ProjectRuntime 不自动重绑。兼容构建接管按原 Run/输入/请求及公开逐项证明核验，不要求原资源全部先停；销毁/reset 仍须取得准确原范围的停止与结算证明。SteerLine 零自动 submit/reply/resume；Runner 只沿原持久授权继续已证明未发送的后继。未知副作用、资源丢失、不兼容和明确停止均保留原门槛，详见[服务换代恢复契约](service-generation-recovery.spec.md)。

## 验证与历史边界

测试只选择受影响的最小必要范围，Plan 映射任务专属验收项、场景/命令、环境、时点、通过条件与证据位置，入口见[测试指南](../testing.md)。本地、远端、受控和真实模型证据分别判断，不能用编译、skip 或原版本通过替代当前业务验收。

一期组合活动和旧 TUI/HTTP 交付保留原[一期约束](phase1-delivery-constraints.md)、[二期约束](phase2-constraints.md)及专属验收，仅按仍适用的范围引用；退役路线的未验不自动变成当前待办。当前模块归属见[代码架构](../architecture/current-code-architecture.md)，旧资料见[归档](../archive/README.md)。
