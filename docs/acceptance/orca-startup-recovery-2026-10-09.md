# Orca 启动超时与失败收敛修复

仅归属本聊天对 SteerlineTest 101 现场的直接修复，不进入 roadmap。候选为 `ade7042bef9d858ae7cc4cdcaae957704c407b8f` 加本次工作区修改；原始测试产物留在临时目录，不提交运行数据库或原始业务提示。

## 范围与分级

进程层尊重单次操作预算，普通命令仍使用项目默认预算，父 context 仍可更早取消。Orca 已接受输入但尚未观察到开始时绑定并观察原 worker，不将 accepted 当 started，也不因该状态关闭进程。已证明原资源释放的启动失败收敛到 rejected 和人工处理，保留原意图、效果和精确身份，不重复启动。

超时缺陷在启动超过普通预算时确定触发，不代表每次启动必然失败。L2 保持正常交付 smoke，使用生产默认命令超时，不增加故障场景或模型回合。异常路径纳入 L3 controlled 的四个独立场景，通过外部 CLI fixture 注入故障；产品服务、CLI、Git、SQLite 和重启均真实运行，不向业务数据库写入模拟状态。慢启动使用 2s 普通预算、10s 启动等待和 3s 响应延迟，等价覆盖预算截断，不花 30s 等待真实门槛。

## 测试计划及验收项

| ID | 测试级别 / 场景 | 环境、命令与时点 | 通过条件 | 证据登记 |
| --- | --- | --- | --- | --- |
| OSR-A1 | 定向 Go / 进程与 adapter | Windows，无模型；实现后运行 `go test ./internal/execution/process ./internal/execution/adapters/orca ./internal/execution/adapters/orcaremote ./internal/execution/backend -count=1` | 真实子进程的较长操作预算不被短 host 默认截断，默认/父取消有效；缺回执、错误 request/runtime/owner 不成为确认 | 本文结果及终端输出 |
| OSR-A2 | 定向 Go / 中心重启 | Windows，无模型；实现后运行 `go test ./internal/execution -run '^(TestReleasedWorkerStartConvergesAcrossRestart|TestSimplifiedLifecycle.*|Test.*Lifecycle.*Receipt.*|TestWorkerFlowClaimNeedsNoProjectSteps|TestPS2MandatoryLifecycleCapabilitiesKeepFixedIdentity)$' -count=1` | rejected、pending 输入清除、人工处理无自动继续选项；多次调度与重启零重复启动 | 本文结果及终端输出 |
| OSR-A3 | L3 controlled / `recovery.orca-startup.slow`、`.unobserved`、`.lost`、`.failed` | Windows，无模型；实现后运行 `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled '-scenario=recovery.orca-startup.slow,recovery.orca-startup.unobserved,recovery.orca-startup.lost,recovery.orca-startup.failed'` | 四场景核实唯一原 Dispatch/Execution/Attempt；可继续的输入不误关；已释放失败不再待核对；重启保留身份和数据库；清理通过 | 本文结果、runner summary 与各 scenario 公开快照 |
| OSR-A4 | L2 controlled / `smoke.delivery` | Windows，无模型；测试接线调整后运行 `go run ./cmd/test-feedback -level=L2 -env=controlled` | 原交付门槛与清理通过；不增加 L2 注册场景。完整 L2 live 的真实模型交付不由本项代替 | 本文结果及 gate-only summary |
| OSR-A5 | 测试基础设施与边界 | Windows，无模型；扩展 fixture/场景后运行 `go test ./cmd/test-feedback ./tests/e2e/internal/... ./internal/testkit/boundary/... -count=1`，以及文档链接和 diff 检查 | fixture 仅属于测试工具；生产无 testkit 依赖；注册/选择/编译/证据/清理和本地链接一致 | 本文结果及终端输出 |

未选择 Full 或其他 L3 业务组：本次变化限于启动预算、启动异常与恢复；相关 adapter、中心、场景和边界定向覆盖。真实 Orca TUI 的提示为何未提交不能由 controlled fixture 验收；本次不宣称完整 L2 live、真实模型链路或安装现场已通过。

## 结果

- OSR-A1：相关四包通过；真实进程与 Orca lifecycle 的短预算回归通过。新增 acceptance request/stage/transport 负例通过；最终 Orca adapter 全包通过（4.978s），含连续不可达核对保留原效果且不递归嵌套回执。
- OSR-A2：最终中心失败收敛及原生命周期定向通过（39.217s）；确认无会话的失败启动立即进入 unverified 人工恢复状态，不提供自动继续。
- OSR-A3：四个独立 L3 场景分次通过，逐项 `passed=true`、`cleanup_complete=true`。`slow`（140.39s）、`unobserved`（121.09s）、`lost`（123.03s）证据在 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T071408.720126100Z/scenarios/`；该批整体报告为失败，原因是当时 driver 将注入的预期失败回执当作场景错误，不能将此批 summary 说成通过。修正 driver 后，失败场景进一步揭示正式停服取消被当作服务错误；修复 scheduler 后 `failed`（136.22s）通过，最终报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T072751.682030100Z/summary.json` 的 `passed=true`、coverage=`selected-L3`、清理通过。先前编译失败和 fixture 未发完成导致的超时均保留为失败，不计通过。
- OSR-A4：通过，coverage=`gate-only`，清理通过；准备 11.743s、执行 95.205s。报告：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T070004.800562200Z/summary.json`。
- OSR-A5：runner / E2E 内部包全部通过；driver/testenv 最终定向和测试边界通过。scheduler 停服回归通过（1.651s），取消单独发生时成功退出，取消与持久化错误并存时保留真实错误。含 e2e tag 的 recovery/live 编译通过，只计编译。文档链接检查通过（285 文件、1778 本地链接、0 错误），diff 空白检查通过。

安装目录 `C:/SteerlineRuntime`、101 原任务和 Orca 全局配置未修改；源码修复须通过正常构建/安装后才会作用于安装现场。
