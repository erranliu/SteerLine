# SteerLine 任务 PR 合入核验实施 Plan

对应[Spec](../spec/task-pr-merge-ownership.spec.md)和[专项验收](../testing/acceptance/task-pr-merge-ownership.md)。实施仅改 SteerLine；以实施时主干为起点复核当前链路。

## 1. 验证 provider 接线

- 先验证候选冻结字段能驱动 SteerLine 主机查询。已完成的受控探针从现有 `runnerPublication` 冻结配置，经真实子进程和本机 HTTP 夹具验证 command/Gitea 与原认证引用。实际探针、命令和未验范围见专项验收。
- 在 SteerLine 的 `internal/execution/publication` 放一条只读平台查询入口，复用 `PRProviderConfig.PRNumber` 与现有受管命令/HTTP 能力。用同一字段规则解码 command 和 Gitea 的原响应契约；保留原认证引用、超时、结果大小和脱敏规则。对照 Runner 目前两个 provider 的正反例，覆盖配置变化、缺失认证、非法 URL、错误 repo/base/head、关闭未合入、合入证据缺失及重定向。
- 通过应用宿主的固定构建验证当前支持的 executable 路径、PATH、工作目录与认证环境。受控探针用绝对测试程序路径，生产宿主与 Runner 的可见环境仍须验明。以冻结候选设置做接线，凭证值从进程环境按原引用读取。

## 2. 切换交付、轮询和废弃

- 把现有 `runnerPRBinding` 的纯绑定规则收口为查询前与事务内复用的规则。调用方以原候选快照固定 PR 对象。保持正常 Runner 投影同步，在已绑定的查询路径移除 `Bridge.RefreshPR` / `ObservePR` 的依赖；确认无其余调用后清理 SteerLine 的专用刷新包装。
- 让 `Service.Deliver`、`Actor.TickPR` 和 `Service.Discard` 共用查询能力，保持原触发时点、候选参与规则与重放顺序。废弃记录本次候选集合和每个原 PR 的观察；任一候选未知或已合入时沿原分支退出。
- 调整 `RunnerResult` / `RunnerDiscard` 的内部提交入参携带本次平台证据。SQLite 事务在写入前重读并比对候选、原 PR 绑定和版本；保持原有 TaskDelivery/TaskDiscard、图推进、停止义务、PR 唯一性和幂等事务。继续使用现有任务交付记录作为成功后的产品读取依据，不增加平台查询历史表。查询失败沿现有待核实与服务诊断入口返回。
- 复核成功交付后的读取，确保已交付任务从 TaskDelivery 显示合入事实，Runner 投影仍显示其自身历史观察。切换通过后同步[可靠交付链](../../docs/architecture/reliable-delivery-chain.md)和[Runner 接入说明](../../docs/integration/steerrunner.md)的当前职责说明。既有运行/停止异常规则沿[统一 worker 生命周期](../../docs/product-design/worker-lifecycle.md)，本项差异限于业务提交前的取证来源与提交输入。

## 3. 验证与验收

按[测试指南](../testing.md)执行。定向结果用 `go run ./cmd/test-feedback -out <绝对目录> -- <测试参数>`，固定测试集将 `-out <绝对目录>` 传给入口。每次执行使用[路径清单](../paths.md)下独占输出，实际版本、命令和证据登记在专项验收。

| 验收项 | 时点 / 环境 | 入口与通过条件 |
| --- | --- | --- |
| PMO-A01 | 接线后 / isolated、controlled | 定向 `./internal/execution/publication ./internal/app`，用受管真实子进程与本机 Gitea HTTP 夹具；另从固定应用宿主验证 executable、工作目录和环境。正反例与原认证、冻结身份断言均命中。 |
| PMO-A02、A03 | 事务切换后 / isolated | 定向 `./internal/taskrunner ./internal/runnerbridge ./internal/storage/sqlite ./internal/app`，并运行 `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.storage`。用无法响应在线刷新的 Runner 夹具证明无 `--refresh-pr` 调用，覆盖原投影、交付/废弃竞争、全量候选版本、失败原子性、显示及重放。 |
| PMO-A02、A03、A04 | 交付前 / controlled | 固定 `l1.minimum` 和 `smoke.delivery`，入口为 `go run ./cmd/test-feedback -level=L1`、`go run ./cmd/test-feedback -level=L2`。另跑 `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled -scenario recovery.waiting-merge-restart`。L2 与重启夹具提供真实命令或 HTTP 受控平台证据，验证原候选、唯一交付、后继、停止、清理。 |
| PMO-A04 | 交付前 / controlled | 固定 Runner 构建核对公开投影与独立 `inspect --refresh-pr`，并复跑 Runner 的 `TestSRPublicationIdentity` / `TestSRPublicationBeforeWorkflowReady`。验证自主发布、适用的 head 核验、早期原 PR 绑定及独立刷新仍成立。Runner 变更范围为空。 |

受控夹具覆盖公开路径和失败注入。实际配置依赖专用命令或私有 Gitea 认证时，按隔离安装验证流程补该组合的只读查询，并记录实际资格。
