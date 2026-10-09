# 远端 Orca 正式路径与 L3 实现 Plan

绑定[Spec](orca-remote-l3.spec.md)，直接在当前聊天实现；此前保留的 #85 不构成 roadmap 调度。结果记录于[专属验收](../acceptance/orca-remote-l3.md)。

## 实施

2026-10-09 本轮按用户新边界修订：远端使用单一 Orca codec v2；移除 Codex codec/版本/CLI/私有 JSONL 依赖；注册只对远端 host 提供 opaque agent 路由，精确本机注册保持；公开 worker-read 取代私有 usage 采集。Orca 当前无 token usage 字段，按 Spec 明示缺口，历史私有文件验证不作为新边界通过证据。

1. AGENTS 明确普通开发直接实现、roadmap-runner 仅在明确调度请求下使用。
2. orcaremote owner 去除无条件封锁，保留原生配置和真实目标预检；修复 coordinator、中心/平台 Run 命名空间、受控超时及执行回执接线；远端 codec 在既有资源封套保存原生启动权威，观察独立于单次 mutation context，先保存 worker_done 正文/PR 线索再核对平台结算。同步 app 与原门槛回归，不能简单关闭所有校验。
3. tests/e2e/internal/native 提供无凭据 remote manifest、CLI 预检、测试 owned helper terminal 和 Orca 公开 worker-read 采集；testenv 准备独立真实 GitHub base 合同、公开创建/领取/反馈、双轮断言与精确清理；evidence 核对 Orca 自有冻结启动选择。补齐 runner chain、环境输入透传、场景登记和公开输出/usage 不可用的报告门槛。
4. 用已配对台式机64g及其 SteerlineTest 实际执行新场景，修复范围内真实失败；失败和未知动作先读取原现场对账，不重派。
5. 记录本次 source/dirty、命令、构建与产物、实际结果和剩余能力边界；普通开发不更新 execution-map 的其他任务。

## 测试计划

| 验收项 | 级别/场景 | 环境/命令 | 时点与通过条件 | 证据登记 |
| --- | --- | --- | --- | --- |
| [RO1-A](../acceptance/orca-remote-l3.md) | owner/定向集成 | `go test ./internal/execution/adapters/orcaremote -count=1`；`go test ./internal/app ./tests/integration -run 'Test(Remote|SupervisedOrca|PS2FixedConfiguration|ConfigLoadsLocalAndRemote|WorkerFlowMode)' -count=1`；`go test ./internal/execution -run 'Test.*(Directory|Registry|Profile|FixedExecutionIdentity)' -count=1` | 接线后；opaque agent 与单一 Orca codec 可选、通用默认/约束及无效目标拒绝、原协议身份/资源断言通过 | acceptance 定向回归；原日志临时目录 |
| RO2-A–RO6-A | L3 delivery.orca-remote-codex | Windows 中心及已配对 Windows 远端；`STEERLINE_REMOTE_ACCEPTANCE_CONFIG` 绝对路径；`go run ./cmd/test-feedback -level=L3 -suite=delivery -env=live '-scenario=delivery.orca-remote-codex'` | 修改接线并补齐公开输出取证后；按 Spec 完成两轮、Orca 精确公开输出/真实合并/唯一交付/清理，1 test/0 skip；缺认证/取证/环境或清理失败均失败；usage 不可用真实登记 | acceptance 按项登记；summary.json、scenarios 下状态/生命周期与 worker-read 回执/public-output/资源与清理 |
| 必要回归 | runner/native/evidence/testenv/boundary | `go test ./cmd/test-feedback ./tests/e2e/internal/... ./internal/testkit/boundary/... -count=1`，按需要 e2e tag 编译登记包 | 共用支持修改后；本机入口、登记、隔离与生产依赖边界保持 | acceptance 回归摘要 |

新增 remote helper、Orca 公开输出读取及报告能力在本任务内实现，执行 live 前落实。token usage 未暴露，按真实不可用状态登记，不补 agent 私有协议。沿用普通真实场景 10 分钟业务期限与独立清理期限，不自动重试。无本任务人工验收项；远端登录/连接需准备，但不是允许后置的验收失败。未选 L2、恢复/candidates/full，因为未修改这些业务范围，按实际共同影响选最小回归。
