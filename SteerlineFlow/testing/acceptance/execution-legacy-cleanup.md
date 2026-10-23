# 执行域遗留清理验收

2026-10-16，普通开发，专属 [Plan](../../plans/execution-legacy-cleanup.plan.md)。仅覆盖无生产消费者的执行域遗留删除，不代表新增原生执行、恢复或真实 PR 验收。

## ELC-A1 引用与删除范围

通过。基线 `347037958c6fad4e52c62ea4477836261760fae6`；先核对外部生产引用、包内声明依赖闭包和仅测试引用，再删除。AST 审计遍历 512 个 Go 源文件，包括所有平台、build tag 和测试；与基线比较，删除的 193 个顶层声明剩余包限定引用为 0。包内引用和接口方法依赖另由生产/测试编译核对，不以文本零命中代替编译。

引用审计原始证据：`C:\Users\lmas1\AppData\Local\Temp\steerline-execution-legacy-reference-audit.json`。临时审计工具不进入产品代码。

| 删除范围 | 引用核对与保留边界 |
| --- | --- |
| `backend/lifecycle.go`、执行/观察/停止接口、旧 host 动作与环境端口 | 无当前生产消费者；删除对应生命周期测试。`ManagedProcessHost`、`StopHandle`、`ProcessFactsPort` 仍由 SteerLine 宿主/进程实现消费，保留 |
| 原生事件/轮次事实、PR 文本线索、adapter 错误与配置输入信封、旧 Operation/记录校验 | 无当前调用方；删除专属测试及混合 wire 测试中的旧 Operation 断言；保留现有 `ExecutionRef`、问题和选项的 JSON 断言 |
| 旧候选共同 base、候选绑定/停止/丢弃、旧交付门槛和 `State` 转移 | 无当前生产消费者；真实共同 base、PR 绑定和唯一交付继续由 taskrunner/runnerbridge/SQLite 负责，当前 `ValidateDelivery` 保留 |
| 旧问题答案校验、阶段判定、原 worker 活动生成器及专属测试 | 无当前生产消费者；保留产品查询仍解码的 `WorkerStateFields`、`StageResult`、问题/决定展示值及编解码 |

`ServiceStopBehavior` 只保留被冻结 profile 消费的值类型，放入 `backend/service_policy.go`；配置协议常量 `Version` 从已删除的 `model/state.go` 移至 `model/protocol.go`。仍被 reconstruction/checkpoint/产品查询使用的记录形状和数据格式不改动，后续投影收敛另行处理。架构依赖守卫继续保留。

删除全部旧 model 专属测试后，固定 L1 不再独立选择无测试的 `execution/model`，并删除对应失效耗时权重；其生产编译、依赖约束及业务消费者回归仍覆盖。首轮 L1 同时发现既有 `EnvironmentCheckCatalog = json.RawMessage` 违反 DTO 无别名守卫；去掉冗余别名，让原应用端口/客户端/测试直接用 `json.RawMessage`，JSON 和行为不变，没有放宽守卫。

## ELC-A2 编译与定向回归

通过，全部为隔离确定性验证，不运行真实模型。下表的报告根目录为 `C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\`，每次运行独立保存 `summary.json` 和原始日志。

| 命令 | 结果 | 证据子目录 |
| --- | --- | --- |
| `go build ./...` | Windows 生产入口编译通过 | 本次命令退出 0，无编译诊断 |
| 临时 `GOOS=linux CGO_ENABLED=0` 后 `go build ./...` | Linux 生产入口交叉编译通过；环境变量随后恢复 | 本次命令退出 0，无编译诊断；不作为 Linux 运行验收 |
| `go run ./cmd/test-feedback -build-only` | 52 包测试编译通过；未执行 Full | `20261016T085859.079392100Z` |
| `go run ./cmd/test-feedback -level=L1` | 15 组、244 测试通过，0 失败/跳过，complete/passed | `20261016T085858.796090900Z` |
| `go run ./cmd/test-feedback -- ./cmd/test-feedback ./internal/product/httpapi -count=1` | 2 包、161 测试通过，0 失败/跳过，complete/passed | `20261016T085859.251229800Z` |
| `go run ./cmd/test-feedback -- ./internal/runnerbridge ./internal/taskrunner -count=1` | 2 包、21 测试通过，0 失败/跳过，complete/passed | `20261016T090122.884948400Z` |

最后一组覆盖当前 `TestSteerRunnerBoundPRCannotDisappear`、outbox 保存失败零调用、迟到接受保留原 Run 并撤销输入、重启撤回未使用授权、未知旧控制零重发和原 Run/显示版本输入绑定。这些是当前 owner 的回归，不复活已失效的旧状态规则。

首轮 L1 失败证据保留在 `20261016T085524.297378000Z`：无测试的 model 包被记作 skip，且既有 DTO 别名守卫失败。修正后重新执行完整固定 L1；未把初次失败改写为通过。初次默认测试编译报告 `20261016T085728.685968900Z` 仅证明编译，最终编译结果以上表为准。

## ELC-A3 文档与差异

通过：`python scripts/check_doc_links.py` 扫描 453 个 Markdown 文件、2714 个本地导航链接，0 错误；`git diff --check` 无诊断。

同步当前代码架构、生命周期规则归属、当前实现缺口、测试入口和验收索引。4 份历史 Spec/Plan 内共 6 个被删除文件链接固定到本次清理前的基线提交，保留原契约正文；不把旧设计改写为当前实现。

## 范围限制

未运行 Full、L2/L3/L4、真实模型或 PR；未操作 `C:\SteerlineInstall`、`C:\SteerlineRuntime`。本次不改变当前格式、W/E/V 节点异常规则、恢复授权和 TaskDelivery 事务。
