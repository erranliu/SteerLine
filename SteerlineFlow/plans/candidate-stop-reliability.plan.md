# Plan：候选停止参与的可靠受理与核实

2026-10-25，已自审收敛并按用户授权实现；实际测试/未验项见专属记录。依据 [Spec](../spec/candidate-stop-reliability.spec.md)，实际验收归 [专属记录](../testing/acceptance/candidate-stop-reliability.md)。本聊天普通开发，不纳入 roadmap，不申请任务编号，不操作安装环境。

## 已查代码与整合决定

| 位置 | 当前行为 / 本次整合 |
| --- | --- |
| `internal/storage/sqlite/runner_stop.go`、`runner_candidates.go` | 用户显示版本和内部 CAS 共用 revision；停止改用当前行做 CAS，显示版本仅校验合法和不超前。复用原参与字段、事务与 commands 表 |
| `internal/taskrunner/control.go`、`control_validation.go` | 停止在入库前和事务内重复相等检查，效果与 finish 分开保存；复用 withdrawal 原请求恢复，原结果优先重放，停止独立于派发 revision |
| `internal/taskrunner/actor.go`、`service.go`、`internal/execution/scheduler/loop.go` | 长 tick 遍历所有候选并等待外部 CLI，500ms control tick 无法抢占；在原单 actor 加通知/子 context 让路，取消普通批次后优先处理停止 |
| `internal/app/client.go`、`host/control_client.go`、`runner_service_wiring.go`、`project_runtime.go` | inbox 写入与 actor 组装接点；持久化后发通知，复用冻结 runtime host，停止受理/原回执读取不重新做 Git/Runner 外部准入 |
| `internal/runnerbridge/bridge.go`、`internal/runnerclient/client.go` | 原候选锁/unknown outbox；保持串行边界，保证取消传播与锁释放，不能因让路重发已不明的输入 |
| `internal/product/httpapi/command_registry.go`、`command_authority.go`、`command_intents.go` | 唯一 transport registry 与原请求映射；原业务结果优先，停止放宽仅作用于该 action，身份/正文仍严格 |
| `web/src/shared/api/client.ts`、`journal.ts`、`web/src/app/use-original-requests.ts` | fetch/锁无独立期限、请求逐条等待；共用有界核实调度、请求级诊断、终态单调与停止只读核实 |
| `web/src/features/runs/stop-candidate.tsx`、`runs.tsx`、`web/src/app/operations.tsx` | 确认因普通 revision 变化失效，局部 pending 与 journal 分别持有；统一请求状态判定，区分结果核实与快照刷新 |

复用一个 HTTP 命令入口、一个 control inbox、一个 actor 和同一 SQLite 业务权威。新增的停止通知只缩短等待，不能执行业务；核实调度只管理现有 GET，不构造新命令。不新增参与版本字段或新通用框架。

## 实施顺序与完成门槛

1. **停止事务与协议。** 保留既有 SQLite 退出事务和 Control.Handle 的原请求结果恢复，withdrawal 继续作为原子效果证据。移除 stop 对候选/派发 revision 的相等要求，保留观察版本合法/不超前检查。更新 HTTP 解析、映射、服务验证、确认 UI 和相关夹具；其他 action 不改变语义。完成门槛为 CSR-A01/A02，尤其保持内部 CAS、最后一个和交付竞争。
2. **权威回执与故障恢复。** 保留 Control.Handle 已有的 Command 重放及 withdrawal 精确恢复。调用方原请求读取在外层 receipt 缺失时核对原 Command/withdrawal，身份损坏不回退；不重做业务，不新增停止事务 API，不在 GET 写入推断拒绝。验证响应丢失、文件写失败、事务回滚、重启和旧实例查询。完成门槛为 CSR-A03/A05。
3. **调度及时让路。** 在现有 scheduler/actor 注册唯一停止唤醒信号，持久化成功后发出，首轮控制先于外部工作。为普通 tick 引入可取消子 context，逐一审计候选观察、生成观察、Task 派发/准备、PR 核对、actor.Call 内可能的外部工作及锁等待；通知只打断当前等待，随后由原 actor 执行停止。通知跨 goroutine 只操作信号/context，不读写 actor.State。批次内及时退出，控制按停止优先处理；让路不使服务退出，也不丢实际错误。关闭时取消并等待所有在途调用后再关闭 store。完成门槛为 CSR-A04，不能仅加优先队列或调整 select 顺序交差。
4. **跨层等待预算。** 用同一剩余 context 覆盖服务端本地处理；移除停止链/回执链中无必要的外部重解析，不让 SQLite 单连接被未关闭 rows 或长事务占住。registry 全局短锁只管理请求门闩；同 ID 的门闩覆盖完整原提交，无关请求不共享长调用锁，竞争等待可取消。前端预算覆盖 navigator.locks 排队和网络，POST 10 秒、GET 5 秒；停止处理 5 秒。取消后的实际结果按持久证据核对。完成门槛为 CSR-A04/A05/A06。
5. **Web 收敛。** 调整 Api、journal 和原请求 hook，在原 owner 内做最多 3 个并发、单请求 single flight、2/5/10/30 秒退避和生命周期取消；补锁等待期限，不用一个 Promise.all 批次等待所有成员才安排后续。停止查询 404 不自动 POST；其他 action 补交契约保持。界面显示请求级核实诊断与时间，30 秒后结束忙碌动画；rejected 解锁，applied 与快照滞后分别呈现，保留选择保护与历史只读。完成门槛为 CSR-A06/A07。
6. **隔离集成和文档。** 复用原 app/SQLite/公开 Runner fixture 与 Web 测试接线补充阻塞/回执故障用例，选择现有 recovery.waiting-merge-restart L3 核对重启边界；不新增独立场景登记或新测试框架。更新原停止文档的当前规则链接和长期 worker 文档，旧验收保留原版本。完成门槛为 CSR-A08、固定 L1/L2 和文档一致性。

## 测试安排

实现阶段在所属包新增下表指定前缀的故障回归，不能以零匹配、skip 或旧回归通过代表完成。以通道屏障/注入时钟和可控 CLI 夹具制造竞争，不靠实际 sleep 猜测时序。

所有输出用 [路径清单](../paths.md) 的独立根 `C:/SteerlineDebug/wt-83d4-candidate-stop/test-results/` 或 `diagnostics/`，每次执行创建新叶目录并显式传入 `-out` / `-EvidenceDir`。下表 `<out>` 必须替换为该次唯一绝对目录。

| 验收项 | 入口 / 命令 | 环境、时点与通过条件 |
| --- | --- | --- |
| CSR-A01/A02/A03 | `go run ./cmd/test-feedback -out <out> -- ./internal/storage/sqlite ./internal/taskrunner ./internal/product/httpapi -run 'TestCandidateStopReliability' -count=1` | Windows 隔离 SQLite/HTTP；步骤 1–2 后。版本增长可退出，其他 action 严格，最后一个/交付竞争、退出事务原子性及原请求恢复成立 |
| CSR-A04/A05 | `go run ./cmd/test-feedback -out <out> -- ./internal/execution/scheduler ./internal/taskrunner ./internal/runnerbridge ./internal/runnerclient ./internal/app ./internal/app/host -run 'TestCandidateStopReliability' -count=1` | 可控 CLI/时钟/锁；步骤 3–4 后。阻塞外部调用时 5 秒内停止，服务仍活动，无重复输入/泄漏，真实错误保留，过期与未知区分 |
| CSR-A06/A07 | `npm --prefix web test -- src/shared/api/client.test.ts src/shared/api/journal.test.ts src/app/use-original-requests.test.tsx src/features/runs/runs.test.tsx src/app/project-switch.test.tsx` | jsdom/fake timers/fake-indexeddb/锁夹具；步骤 5 后。新增 journal/hook 测试文件，明确每文件实际执行数量；超时不阻塞队列，不重发，状态/选择/身份正确 |
| CSR-A08 | `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled -scenario=recovery.waiting-merge-restart -out <out>` | 现有隔离场景加本次 owner 故障回归；步骤 6 后。真实 Go/Git/SQLite/应用 HTTP、公开 Runner fixture，无模型/真实 PR；原请求+重启闭环和清理证据完整 |
| CSR-A08 Web | 沿现有 Web 测试接线增加网络故障/锁集成用例，实际命令写验收记录 | 不新增包生成/浏览器框架；组合生产 Api、journal、hook 和 Runs 验证请求计数、失败释放和快照延迟；真实浏览器体验另列人工项 |
| 全部实现项 | `go run ./cmd/test-feedback -level=L1 -out <out>`；`go run ./cmd/test-feedback -level=L2 -out <out>` | 代码完成后固定集；另运行 `npm --prefix web run build` 和本表定向项。不默认扩展 Full 或 live |
| 文档/登记 | `python scripts/check_doc_links.py`；`go test ./cmd/test-feedback -run 'Test(TMRegisteredCatalogueMatchesExecutableManifest\|ScenarioContentListMatchesRegisteredIDsAndLevels)$' -count=1`；`git diff --check` | 规划期只检查当前文档链接/空白；实现阶段新增场景后再检查登记一致性 |

Web 单测/build 日志使用外部 diagnostics 唯一目录保存；浏览器原配置、源码 SHA、应用包 manifest、Runner fixture 身份、命令和证据路径登记专属验收。用实测值报告 5 秒预算和清理，不仅写“很快”。

## 本次规划与交付

2026-10-25 用户要求先审计再实施，按本次收敛 Spec 执行；检查与运行结果单独登记验收记录。原只读调查不计实现通过。

实施交付物为代码、所属模块故障回归、现有受控场景及实际验收记录。安装部署与 Task 18 的再次停止属于之后显式维护动作；不改写原 rejected 请求，不通过直接改库消除现场。
