# Plan：分级测试与共享 E2E 环境的最小实现

目标：`TIERED-E2E-TESTING`。状态：已实现并通过最小验证，待实现 PR 评审；未分配 Task ID、创建中心任务或绑定现有任务。日期：2026-10-01。唯一执行依据：[Spec](tiered-e2e-testing.spec.md)。一个实现 PR 交付，首期仅 Windows、controlled、串行与两个场景。

## 1. 固定现有入口与最小接线

核对 `cmd/test-feedback/main.go`、`scoped.go`、`internal/testfeedback`，以当前 quick 用例/包及完整三进程行为为基线，不采用历史测试提速文档中的旧五步闭环。保留原有用例和入口，记录新增目录对普通包枚举的影响。

核对产品正式入口：`task validate/create/show/list/graph`、`control start_task`、`run -dispatch manual`、停止与公开只读查询。核对首次项目初始化、当前执行/发布配置与权限认证要求；使用已有可配置 command provider 和 worker 可执行路径。复用当前受控 app-server 的最小协议行为，不放松产品权限门槛，也不依赖包内写库 fixture。若现有公开入口无法完成首条场景，先明确缺口并保持本项未完成，不以直接写库替代。

## 2. 建立共享环境管理

建立 Spec §2 的测试包、testdata 和两个场景骨架，场景测试显式使用 e2e tag。`testenv` 用小型 Run/Scenario/Project/Task 结构管理环境，`driver` 封装正式 CLI、现有公开控制客户端与必要公开只读查询。复用产品 DTO/协议，不复刻业务模型。

Run 管构建/产物/选择；Scenario 管独立现场和资源；Project 管真实 Git/remote/配置/SQLite/服务；Task 从实际任务回执取得身份。一个最小项目模板和 A → B 任务模板即足够；提交真实 Spec/Plan 后按当前摘要契约创建任务。先实现创建、启动/就绪、查询等待、停止、保留现场重启、取证和清理，不建设 fixture 继承层次。

资源创建即登记反向清理；子进程环境与绝对路径逐项设置。正常和断言失败时先取证再停止/清理；外层取消/超时复用现有受管进程树机制。产物与 scratch 分离，目录删除校验本次所有权，清理错误汇入结果。资源清单仅作本次清理/诊断依据，不实现后台回收。

先用一个两项目定向检查核对隔离，再用一个可控失败/取消检查核对部分创建、进程树和日志生命周期；覆盖 Spec TT3/TT4 的独有行为，不建立大规模崩溃矩阵。

## 3. 实现最小 controlled 外部组件

在 `internal/controlled` 与 `testdata/controlled-helper` 实现一个辅助二进制的 worker/provider 子命令。参考 `internal/steerline/worker_local_test.go` 的 app-server 协议，以及 `internal/steerline/publication/provider.go` 的 command 契约，保留其既有回归位置，不批量搬迁旧 helper。

helper 完成当前准备/一次业务轮，真实修改/暂存并输出本机发布标记；中心按当前 host_service 契约调用真实 Git commit/push 和受控 GitHub command provider。provider 读取本项目原子状态文件，支持当前场景所需查找/创建/open/merged。受控合并先更新隔离 remote 再提供相应 commit 证据。持久化必要会话/派发计数和身份，服务重启不重置 helper 外部事实。

仅实现当前产品实际调用的必需协议和上述行为。明确拒绝不支持的必需请求；不实现真机登录、远端管理、可插拔后端框架或自由故障脚本。

## 4. 接入统一选择与报告

在 test-feedback 增加 `-level/-suite/-env` 的独立 E2E 分支；L1 别名复用现有 quick，L2/L3 与 full/build-only 等组合按 Spec 明确拒绝。`tests/e2e/suites.json` 固定登记两个场景，用标准 JSON 解析。

一次构建当前产品/helper，向场景包传绝对路径和运行配置，按所选包枚举、核对清单并执行精确测试选择。所有场景串行，显式无缓存；不存在的组、未登记/重复场景与未匹配选择在执行前失败。不增加通用调度器，暂不提供所有 L3 默认执行命令。

复用当前 JSON 汇总，补 E2E level/env/场景与资源证据。核验所选场景恰好一次成功、清理完成，失败/skip/缺失/截断等汇总为非成功；保留准备与执行时间、原始日志及部分失败结果。普通包枚举不带 e2e tag，不改 quick/full 的覆盖声明。

## 5. 完成两个独立业务场景

先实现 `smoke.delivery`：独立创建项目与 A → B，经正式控制客户端领取 A，让真实服务和受控 worker/provider 完成 open → merged → 交付，断言 B 从阻塞转就绪且没有启动。保留任务/工作流/交付身份和必要公开状态快照。

再实现 `recovery.waiting-merge-restart`：复用上述环境操作和模板，重新创建全新现场。在 A 等待合并且 worker 业务轮结束时停止服务，保留全部现场重启；核对原身份、B 阻塞及无新增会话/派发，随后受控合并并交付。再次对账确认同一公开 Delivery 与无新工作流。断言独立写在 L3 测试，不调用 L2 测试函数，不扩大为活动轮或强制退出恢复验收。

## 6. 文档与 Plan 接入

编写 `tests/e2e/README.md`：级别用途、三条入口、环境组成、目录/所有权、添加场景的步骤、失败产物及首期限制。在项目 `AGENTS.md` 加短约定：新编写或本次修改的实现 Plan 写测试计划，按最小必要范围指定组/场景、环境、命令、执行时点、通过条件和证据。保留现有最小验证与避免全量的规则。

当前 Plan 使用下表作为首份示例；不批量修改历史 Plan，不给其他中心任务新增验证门槛。验收结果记录到 `docs/validation/tiered-e2e-testing/README.md`，引用原始产物，区分实现要求与实际已验证能力。

## 7. 测试计划

以下入口已实现；实际执行与结果见[验证记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/tiered-e2e-testing/README.md)。

| 级别/性质 | 选择与环境 | 命令 | 执行时点与通过条件 |
| --- | --- | --- | --- |
| L1 定向 | test-feedback 选择/报告；既有 testfeedback 汇总 | `go test ./cmd/test-feedback ./internal/testfeedback -count=1` | 工具改动完成后一次；TT1/TT4 的选择和失败传递通过 |
| 基础设施定向 | testenv/driver/controlled；controlled | `go test -tags=e2e ./tests/e2e/internal/... -count=1` | 环境/helper 完成后一次；两项目隔离、部分失败和取消/清理通过 |
| L1 | 当前固定 quick | `go run ./cmd/test-feedback -level=L1` | 最终候选一次；当前 quick 覆盖与原入口等价 |
| L2 | `smoke.delivery`；controlled | `go run ./cmd/test-feedback -level=L2 -env=controlled` | 最终候选一次；真实内部闭环及 A/B 断言通过 |
| L3 | `recovery.waiting-merge-restart`；controlled | `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled` | 最终候选一次；原身份保持、无重复派发、同一交付及后继就绪 |
| 文档/依赖 | 包边界、索引、引用和 tag | 定向依赖/源码检查及 `git diff --check` | 交付前；生产不导入测试，默认入口不启动 E2E，文档与实现一致 |

对取消/超时涉及的真实受管后代进程做一次有界基础设施检查；用小型受控输出覆盖空匹配、skip、失败、截断、证据和报告/清理错误，不重复启动产品为每个汇总错误造现场。L1 兼容性结合参数定向测试与一次 quick 证明，不额外重复同一 quick。

最终稳定候选的两个业务场景各执行一次；后续只有变更、失败或明确未覆盖风险才重跑受影响项。默认不运行完整三进程套件、真实平台或 Windows/Linux 全矩阵；既有入口枚举/tag 通过定向检查确认，不为此执行全部业务测试。

## 8. 完成条件

用户于 2026-10-01 明确要求补充完整基线，并先 rebase 最新主干。实际范围与版本见[补充基线记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/tiered-e2e-testing/baseline-2026-10-01/README.md)：完整 Go 回归及崩溃后缺失用例补跑失败，分级场景与环境检查在夹具换行修正后通过。该补充不改变其他任务默认按最小必要范围选测的约定。

TT1–TT5 的代码、场景、证据及文档可审阅，新增两个级别能够独立选择且共用环境管理。PR 描述列出 package/入口边界、两个场景断言和实测结果，明确 controlled 不构成真机认证。

缺少真实内部接线、资源清理或必要报告时保持未完成，不将演示输出写成通过。本次实现范围为分级入口和受控 E2E 环境；未创建中心任务、执行真实平台验收、部署生产或迁移数据库。

## 9. 测试文档总入口

新增 `docs/testing.md`，从根 README 导航，E2E README 反向链接。按当前 runner 参数和 suites.json 说明 L1、Full/串行、定向、L2/L3、基础设施、编译、独立原生验收及证据；保留 E2E 具体约定的单一维护位置。当前基于已合并 #180 的 main，只改文档，不新增测试环境或业务场景。

### 测试计划

| 级别 | 组/场景 ID | 环境 | 命令 | 时点/通过条件 | 证据 |
| --- | --- | --- | --- | --- | --- |
| 文档静态检查 | docs.navigation / docs.commands / docs.scenarios | 本机，无模型 | `python "$env:TEMP/sl-testing-docs-check.py"`；`git diff --check` | 编辑后；相对文件链接有效，命令参数/注册的九项场景一致，三个导航入口双向连通 | PR 验证说明、检查输出 |
| L1 定向 | runner.selection | 本机，无模型 | `go test ./cmd/test-feedback -run '^TestLevelSelectionsRejectAmbiguity$' -count=1` | 提交前；文档使用的分级选择约束与现有 parser 测试一致 | PR 验证说明、命令输出 |

不运行完整 L1、Full 或 L2/L3：只增加文档导航和现有命令说明，没有改执行、断言、配置或环境能力；无需再次消耗 token 或创建测试 PR。
