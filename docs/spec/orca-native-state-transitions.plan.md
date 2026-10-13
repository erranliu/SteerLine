# Plan：Orca 单终端原生状态切换

日期：2026-10-13。[Spec](orca-native-state-transitions.spec.md) · [专属验收](../acceptance/orca-native-state-transitions.md)。普通开发，不占 Task ID；Plan 描述交付安排，结果统一登记专属验收。

## 实施步骤

1. 在 `internal/execution/adapters/protocol/orca` 内实现现有公开 RPC 的最小连接、鉴权、订阅/查询和取消接线，保留现有 CLI 的资源与输入操作。协议接线后先执行第 5 步的最小原生样本，确认实际响应中的原终端身份、主 agent 来源、状态顺序及开始/结束绑定，再做步骤 2–4；必要证据不可用则明确未核实，不改 Orca，不建立通用 SDK。
2. 在现有 orcatransport lifecycle/observer 内保存发送前基线，只由可信状态信号接受本轮开始/结束。过滤旧记录、重复/合成状态及子 agent；结束之后才采集完整普通回复，复用既有解析、持久化与阶段推进。只补既有契约确实缺少的事实字段，不增加独立状态机。
3. 增加一个 30 秒无信号补查询定时器，限制单在途和同窗口一次；订阅与查询按各自事实类型校验。查询失败/旧状态/超时只保留原因，不能自动完成或重发。原 pane 的新 done 提示触发一次确认查询；重复提示不重复查询，补查仍 working 后仍接受后续新提示。补定向场景覆盖这个窗口，不增加周期轮询回退。测试使用可控时钟。
4. 在现有隔离夹具补最小状态响应能力和一项阶段用例，复现 publish 仍 working、屏幕保留旧 report；验证不会接受旧结果或发送 publish.report，真正结束并取得完整结果后才按现行规则推进。
5. 新增小型原生探针入口 `scripts/test-orca-state-transitions.ps1 -PackageRoot <新包> -OrcaExecutable <固定CLI>`，复用生产协议解码，不复制状态判定。先运行 `scripts/new-debug-release.ps1 -Orca -OrcaExecutable <固定CLI>` 创建 C:/SteerlineDebug 下的新 live 包；探针在唯一原终端运行两次最小只读回合，采集 working/结束与补查询样本并清理自己的资源，不启动发布任务、不创建 PR。记录实际版本和字段映射；能力不足则记录未通过，不扩大方案。
6. 实现后更新相关传输说明中被替代的完成判定，登记最小验证结果。安装环境不属本次操作范围。

## 测试计划

覆盖全部自动项 [OST-A01–OST-A05](../acceptance/orca-native-state-transitions.md#自动验收项)。下面的新脚本/测试先在实施步骤中落实，再执行；零匹配、skip或缺证据不计通过。

| 级别 / 场景 | 自动项 | 环境、命令、时点 | 通过条件 / 证据位置 |
| --- | --- | --- | --- |
| 本机原生 / OST-NATIVE（新增探针） | OST-A01 | Windows、独立新 live 包、实际 Orca；协议接线后运行步骤 5 的命令 | 普通终端订阅/查询实际可用；两轮状态及原身份、来源/顺序可核实，输入各一次；保存响应、版本和清理结果至包 evidence，登记 A01 |
| 定向单元 / OST-STATE（新增测试） | OST-A02 | 隔离响应/可控时钟夹具；状态判定完成后：`go run ./cmd/test-feedback -- ./internal/execution/adapters/orcatransport ./internal/execution/adapters/protocol/orca -run '^TestOrcaNativeState(Stream|Identity|Gate|Lifecycle|Binding)$' -count=1 -timeout=2m` | 明确 started/working→done 才结束；waiting、旧/合成/重复/子状态不结束；30秒补查、单在途、同窗口一次、新旧记录/查询失败与订阅竞态正确；补查working后新done再次核实；长输入/相同回复、原生正文不变及 inline Markdown 绑定正确；元数据省略与明确替换原实例正确区分；runner summary/输出登记 A02 |
| 受控阶段 / OST-FLOW（新增用例） | OST-A03 | 既有隔离进程/Git/SQLite夹具；单元后：`go run ./cmd/test-feedback -- ./tests/integration -run '^TestWorkerStageOrcaNativeStateGate$' -count=1 -timeout=5m` | local/environment复用同一规则；publish working+旧report无提前推进，结束先到但新回复未到仍无下一输入，完整结果后按现行阶段推进；提示词不变、输入/结果各一次；runner summary与阶段/计数登记 A03 |
| 真实全流程 / OST-LIVE-LOCAL | OST-A04 | 先运行 `scripts/new-debug-release.ps1 -Orca -OrcaExecutable <固定CLI>`，注册本包仓库；通过包 `debug-helper.exe debug run` 启动 live 服务，control start_task 启动首轮；到 await_merge 后核验专属 PR，`gh pr ready`、`gh pr merge --merge --match-head-commit <候选SHA>` | 首轮 implement/report/publish 均只发送一次，原生普通终端采集完整结果；首轮 PR 的 base/head/SHA 正确且合并，任务交付并释放后继；包 evidence 保存 snapshot、PR JSON、版本及命令，登记 A04 |
| 真实全流程 / OST-LIVE-REMOTE | OST-A05 | 生成另一个新包，指定 `-OrcaEnvironment <原生保存environment> -OrcaRepositorySelector <远端专属checkout>`；本包唯一 base 推送后，远端原生 shell clone 该 base 并注册；同上启动两个任务及核验/合并 | 与 A04 相同，执行事实必须来自远程 runtime、远端原终端和 checkout；本机/远程不共用 base/head，记录实际远端版本、清理资源和首轮交付 |

OST-STATE 只覆盖本次判定的必要正反例，不扩展故障窗口矩阵。OST-FLOW 复用现有阶段夹具和 loopback provider，不另建 E2E suite或测试平台。本次新增独立保存原生 outcome 及已结束但回复未核实事实的共享接线，追加 `go run ./cmd/test-feedback -- ./tests/integration -run '^TestWorkerStageLocalRoundTrip$' -count=1 -timeout=5m`，作为 A03 的共享接线证据；并覆盖共用阶段推进。

不跑 full 或全套 L1/L2/L3；真实本机/远端流程为用户追加的验收。实施时修正 Orca 调试模板：任务只定义结果文件及当前分支，使后继有独立变更，Spec/Plan 不指挥阶段推进。现有包控制、snapshot、原生 CLI 和 gh 已能驱动验收，不另建测试平台。原始证据使用 runner默认临时目录或新包 evidence，验收文档登记最小摘要、准确命令、版本与路径，不复制安装版数据库/配置/认证。

只保留生产状态规则和一个生产 lifecycle 原生探针；删除已被替代的实验性解析器、本机管道代码与原门槛探针。之前的实际字段样本保留在验收历史中，不作为当前测试入口。

生产验收完成后登记 A02/A03。步骤 5 的脚本直接运行 `TestOrcaNativeStateLiveLifecycle`，以新隔离包的两轮普通只读任务检验生产订阅、查询、正文核对和清理，不创建 PR。随后使用最新主干的新 live 包执行 A04/A05。服务快照就绪后 resume_dispatch，再启动任务 1；PR 核验合并后确认交付与后继释放。用户已取消第二轮并要求整理提 PR；不再启动后继，失败及未完成的首轮如实登记，不计通过。无需单独人工验收。
