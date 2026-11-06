# 用量事实与计费职责划分验收

归属：[Spec](../../spec/billing-ownership.spec.md) · [Plan](../../plans/billing-ownership.plan.md)。日期：2026-11-05；普通开发实施，不分配Task ID，不启动roadmap。两仓变更已在隔离副本实现；自动验证结果如下，未部署安装版，原 Task 101 未回填或改写。

## 自动验收与结果

执行命令和时点以 Plan §3 为准；执行前固定源码与 Runner 构建、正式 schema、入口及新证据目录。下表的状态只使用本任务证据更新，不继承旧 run-usage-cost-ui 的结果。

| ID / 方法 | 步骤与最小断言 | 环境/安排及阻塞范围 | 当前状态 |
| --- | --- | --- | --- |
| BILL-A01 / 自动 + 契约核对 | 核验既有run-usage/run-usage-events可复用部分，Runner交付必要增量/能力/分页/错误；actual模型/时间/裁定、响应与区间不重叠、去重/修正/未知有来源测试；退出计价职责 | Runner独立副本；固定构建及证据见下文；阻塞真实接线与A07，不阻塞纯计算/配置开发 | 通过本次契约范围；广选中原有提示词冻结失败另列，不声称 Runner 全仓通过 |
| BILL-A02 / 自动 | 真实子进程公开 DTO fixture 验证 Run/state-dir/expected-instance、固定revision全页；错页/重复记录/同游标循环/summary矛盾/过期/超预算/取消均不输出伪总额；查询调用记录无执行/补采/估价 | isolated；P4 后；阻塞客户端接线完成 | 通过：客户端全量；多页同revision/绑定、矛盾、过期及取消；容量上限另作静态预算核对，未做容量压力基准 |
| BILL-A03 / 自动 | 新建、未配置、非法文件、字段/时间/重叠/负价/精度错误、旧版拒绝、版本不可变/active切换、跨进程并发保存冲突、替换失败保原文件、链接拒绝/私有路径、零项目/Runner离线；不写execution.json或项目库 | isolated独立home；P3/P5 后；阻塞配置交付 | 通过：版本/条件写、跨进程一胜一冲突、异常文件/硬链接拒绝和原文保留；零项目浏览器保存 |
| BILL-A04 / 自动 | 用独立计算的固定期望验证下方算术矩阵，不以被测计算器生成预期；相同用量revision/价表/规则版本结果确定 | isolated纯函数；P2 后；阻塞计价功能交付 | 通过：独立固定期望、分类互斥、时间边界/区间、极小额与int64精度 |
| BILL-A05 / 自动 | 候选/Run/目录错绑拒绝；价表保存发生于明细分页中仍用起始版本；费用配置/分页失败保有效Token；旧estimate调用/DTO/字段删除；usage_policy启动正文保留；不改变profile/ConfigDigest或业务授权 | isolated公开Runner/项目fixture及API；P4/P6 后；阻塞产品API交付 | 通过：候选/目录错绑、明细/价表故障保Token、缺元数据与冲突分因、已finalized总额；价格版本在查询开头固定 |
| BILL-A06 / 自动 | 全局设置增版本/切换/冲突/字段错/未保存，零项目/离线；执行页三态、真正零/部分零、金额精度、scope、版本/来源/假设、缺价入口；finalized停轮询后改价仍重算、跨标签页generation变化、取消/迟到错候选错版本隔离、失败保留旧版本标识；宽窄屏/键盘自动验证 | isolated Web/Vitest/浏览器新驱动，P5 补驱动后执行；阻塞Web交付 | 通过：Web全量与最新包浏览器；价表版本/冲突草稿、宽窄屏/键盘及停止轮询后的generation/策略刷新 |
| BILL-A07 / 自动 | 固定匹配Runner真实CLI与Runner自有受控账本fixture，创建可控Run、取公开用量全页、调用方API本地计算和状态对照；至少覆盖可计价、事实缺失和旧能力不支持，不要求实际付费模型；核实stop/服务/资源清理 | 新隔离home/固定EXE，扩展go.runner-contract入口后执行；阻塞跨仓集成完成 | 通过：公开3测试＋受控账本跨CLI计价联验；原生取源另归A10 |
| BILL-A08 / 自动 | L1/L2、go.http、web.unit、受影响owner、构建与文档导航/范围/清单一致性；每个正式选择非零、零意外skip、报告/退出码一致，原失败与重跑分开 | Plan指定原环境，新目录；实现集成后与提交前；阻塞实现完成 | 通过：正式L1/L2、web.unit、go.http、go.environment、公开契约及构建；最终静态检查见下文 |
| BILL-A09 / 自动 | 采集设置明确保存enabled/temporary/grants；host_id和session叶目录来自核实公开事实；错误根/主机拒绝；保存待重启与实际status区分、缺授权在摘要可见、价表无关；新许可/实例变化使停轮询缓存失效，原绑定有界补观察，无新候选或输入 | isolated配置/应用/HTTP/Web fixture；P7后；阻塞远端修复交付 | 通过：策略解码/核实根校验、有效服务原身份只读测试、设置/摘要与跨epoch刷新；真实授权生效归A10 |
| BILL-A10 / 自动 | 新隔离包远端Codex Run先无授权给真实诊断；公开事实核实最小grant，正式停启后同原Run读取可信Token/同版本明细并正确展示，重复读不累加；撤销/回收与资源清理分别核实 | L4 live，已登记usage.remote-codex驱动/清单/命令，固定环境/模型/构建后执行；P8后；阻塞远端真实链交付 | 通过：新隔离Run 1响应/16,166 Token；撤权保历史，停止/结算/临时登记回收均核实 |

### BILL-A04 算术与状态矩阵

1. 多模型、多轮、多 Attempt 的独立记录分别匹配再相加；summary-only 数不能覆盖明细；provider 不同而 model 同名不串价。
2. 非缓存输入、cache_read、cache_write、output 分别换算；reasoning 已含在 output，不另加；total/input_total 不参与二次计费。未知分量不补 0。
3. 用量恰在生效边界使用新规则；interval 完全落单一规则才可计价；跨界、时间缺失、模型缺失、价格空档均有未计价原因，不按比例分摊或用当前profile猜测。
4. 完整可计价、采集中但已知部分、缺价/缺字段的部分、全不可计价四种覆盖；部分分类可得 covered 但无 total；记录数分区之和与集合条数一致。
5. 已知0 Token、明确0费率、空且已finalized的真实零集合；未知Token、未配置价表不得显示0；部分金额为0仍非最终总额。
6. 单价12位小数、超精度拒绝、极小金额、最大合法int64 Token、精确求和后显示舍入；负数/非法数字/溢出/上限拒绝，浮点不参与。
7. 未finalized或预期来源未知/不全，即使已知记录均有价格也只得partial；实际消耗记录纠正后按新revision整体重算，不能把前后累计快照再相加。
8. 中途active价表改变不混版本；切换保留版本可复现原版本金额；换币种只改变该次计算币种，不折算、不混币相加；basis和未建模价格条件始终可见。
9. 明细decision非counted不参与金额；counted但metadata_issues含模型/时间冲突时Token保留、费用不计；metadata_basis非observed或time_basis仅completion/unknown时不以配置模型/当前时间补齐。

## 人工验收活动

| ID / 方法 | 操作与判断 | 参与/窗口 | 状态与影响 |
| --- | --- | --- | --- |
| BILL-M01 / 人工 | 在已通过自动验证的新隔离debug包，查看全局设置到候选费用/采集入口、待重启状态、版本切换后历史重算提示、部分金额/未知原因、窄屏信息是否清楚 | 参与人及窗口待安排；使用固定构建和自有home，不操作安装现场 | 待验收；主观体验可后置，不替代A01–A10 |

## 2026-11-05 指定远端 Run 只读诊断

用户给定home为 `C:\SteerlineInstall`、repository为 `C:\Projects\SteerlineTest`、Task为101、Candidate为 `3b21b9e861fc40f79324e9afa1afadc5`。从SteerLine自身任务库以SQLite mode=ro读取目标候选，核实其公开执行投影为远端Orca/Codex、profile为orca-remote-desktop64g，实际Runner Run为 `0b2ac9b03e2d8fdf05ac1d4785bccf03`；查询用后者而非Candidate ID。未打开Runner数据库或私有checkpoint。

安装pin与运行状态一致：

- EXE：`C:\SteerlineInstall\bin\steerrunner.exe`，version=`0.2.0-dev.4`；实际文件SHA256核验为 `cd3d4ace05c00ea81f2888c32833c0b75618a04a73921c4f216f1c31afc2811f`。
- resource_digest=`895a57bc2e6237f86aad26979a2f744139bfc277f756b1d4b95d482cd1bdb9a0`；state-dir=`C:\slr\rehnosf7lwgci2qaqusuz6z56u`。
- 只读status时Runner instance=`0edde5b1327bb021d995dfccedc3b47d`、status=ready。deployment.json记录Runner源码为 `f6a389cfd4ae7827d7c722555a85e94f1e1ba708`，SteerLine源码为 `df18050e87a2ef7852c71224063480407527c6c8`；这些是现场部署记录，不是本工作树的实现版本。
- execution.json未设置usage_policy，也未设置pricing_policy。有效service status明确返回 `{"enabled":true,"remote_log_access":{"mode":"disabled","grants":[]}}`，policy digest=`c011c146545d05b9834e9789142bd1012b03d13bb46c142aa22301ddf49d8ae1`，revision等于上述实例。

实际执行的只读公开命令如下。目标查询均带该实例的 `--expected-instance-id` 与完整 `--expected-binding`（contract/protocol/instance/state-dir/EXE SHA/version/resource digest），无输入信封、无变更动作。静态capability离线读取：

```text
steerrunner capabilities --offline --json --protocol steerrunner-cli/v1
steerrunner service status --state-dir C:\slr\rehnosf7lwgci2qaqusuz6z56u --json --protocol steerrunner-cli/v1
steerrunner run-usage --run-id 0b2ac9b03e2d8fdf05ac1d4785bccf03 <完整实例绑定与同一state-dir/协议参数>
steerrunner run-usage-events --run-id 0b2ac9b03e2d8fdf05ac1d4785bccf03 --snapshot-revision 8 --limit 10 <相同绑定参数>
```

两个用量查询均exit=0/ok=true；run-usage精选事实如下，两个COVERAGE_INCOMPLETE分别为全局不可重试及该source可重试，不能混淆为整个请求失败：

```json
{
  "run_id": "0b2ac9b03e2d8fdf05ac1d4785bccf03",
  "schema": "run-usage/v1",
  "snapshot_revision": 8,
  "status": "unavailable",
  "tokens": null,
  "coverage": {"counted_responses": 0, "expected_sources": 1, "verified_sources": 0, "finalized": false},
  "issue_codes": ["COVERAGE_INCOMPLETE", "REMOTE_LOG_ACCESS_NOT_CONFIGURED"],
  "access_cleanup_status": "not_required",
  "observed_at": "2026-11-05T07:31:34.2357426Z"
}
```

同revision的events=[]、next_cursor为空。capabilities.run_usage.matrix发布orca-terminal/codex/orca_environment的collection/production_run_binding=true，同时remote_log_access_default=disabled；矩阵不是当前Run已授权的证明。离线schema另证实已有run-usage-events字段：event_id/source_ref、provider/model、metadata_basis、call_time/time_basis、decision、metadata_issues、分类Token及next_cursor。此事实已用于纠正首版Spec将明细一概当作新v2接口的假设；不以本地其他Runner checkout推定安装构建行为。

**结论与未验边界**：直接阻断为远端日志读取未授权，Runner尚无可信Token；缺价表是独立设置，补价不能修复采集。SteerLine源码已将该错误码映射为“未配置远端用量读取授权”，但位于折叠明细，且该原因会停止自动轮询；本次范围增加设置入口、摘要主因、生效状态和新许可后重新观察。未经启用和实际取源，不能宣称原日志存在、绑定/格式必定可用或已恢复Token。

最小恢复条件为：先从公开事实核实该environment的稳定host_id和合规session叶目录，显式配置temporary grant，经原正式停服/启动规则生效，再让Runner后台有界观察原绑定并查询新快照；若日志已不可得，保留原因，不补造消耗。当前没有这些已核实授权值，未写安装配置、未登记远端目录、未启动/停止/重启任何服务或Run，也没有模型调用。

初始只读阶段：原沙箱内service status的“Cannot verify service ownership”经沙箱外同一EXE只读查询消除，实际服务为ready；它属于本次工具权限限制，不作为产品根因。该阶段尚未实施；现场诊断不作为修复后通过证据，后续隔离结果单列如下。

## 实施交付与固定契约

- SteerLine 基线 `df18050e87a2ef7852c71224063480407527c6c8`，当前 worktree 未提交改动。纯计算 owner `internal/billing`，home/billing.json 唯一 owner `internal/billingconfig`；事件客户端完整固定快照，应用查询冻结价格后组合。全局设置维护历史版本/active/generation，候选明细含价格来源、匹配规则及精确金额。无数据库变更、任务汇总、自动重启或新执行权限。
- Runner 副本 `C:\SteerlineDebug\wt-1148-billing\runner-src`，branch `codex/billing-ownership`，基线 `f6a389cfd4ae7827d7c722555a85e94f1e1ba708`；未改原 checkout。补丁 `handoff/steerrunner-billing-ownership.patch`（69,584字节，SHA256 `61fd236d5a1710cdfe4effec64d1fc70fb5ef75bad87c3846e86f5b6cd00ae04`），清单 `handoff/runner-manifest.json`，均位于下述外部 scope。
- 固定 EXE `runner-build-v2/steerrunner.exe`，version `0.2.0-dev.billing2`，SHA256 `78bb5f1aa0400a137322ef1501101227c017507fbc35f009db44c3c3330e2236`，source digest `182a8bbe3adfae9992c41688ce11dd9bf429f8e3a7f6ae4613e06aefc1ddfbf4`，resource digest `6e2c12fe80a441209eabb15f57598e53ea1c0ec96a5440f18fd5eff7d39ebee3`。固定构建后只增加 Runner 的 tagged fixture 和文档，未再改运行代码。
- Runner 继续发布 run-usage/v1 与 run-usage-events；补充 same_snapshot/events_schema/max_events/access_requirements，source-linked需求持久化在原快照/checkpoint。删除 estimate-run-cost、价表schema、run_cost和内部估价包。没有为 interval 宣称新原生能力；只有完成时间的响应仍不可按请求时间匹配价格。
- 远端授权仍在 execution.json.usage_policy，通过应用 usage-collection/status 查询当前服务身份/策略，完整查询才读取已有候选的公开需求；保存待正式重启，不热更新或自动登记。浏览器只选择已核实主机/日期叶目录，不能手填宽目录。

## 自动验证证据

全部相对证据路径基于 `C:\SteerlineDebug\wt-1148-billing`，原报告/退出码/所选命令及来源身份保留，不把 diagnostics 当正式 Full。所有已通过选择非零、零skip；Go计数含根与子项，不与独立测试函数数混用。

| 范围 | 实际入口与结果 | 原始证据 |
| --- | --- | --- |
| Runner受影响owner | 明确选择 usage/access/api/collect/parsers/resources/storage/contracts/cli/host 的非live受影响测试；265项通过、0skip，广选总体因原有提示词校验失败退出1；resources用量/schema两根单独通过 | `diagnostics/20261105T085712Z-runner-a2d658d9d44b454fbbe051c5251d2f00`、`diagnostics/runner-resources-final.jsonl` |
| 纯计算/配置/客户端/策略 | `go run ./cmd/test-feedback -out <新目录> -- ./internal/billing ./internal/billingconfig ./internal/runnerclient ./internal/executionconfig -count=1`，151项通过、0skip；补充多页客户端全量另见下行 | `diagnostics/final-owners-20261105-v2` |
| 完整两页/过期/取消 | 完整runnerclient owner，显式两页固定revision7/实例与after；summary矛盾或unknown计数被补零拒绝，第二页过期拒绝、已取消不启动CLI | `diagnostics/final-pagination-20261105` |
| 应用查询 | `-run '^(TestRunUsage.*\|TestUsageCollectionOnlyReadsOwnedEffectiveService)$'`，20项通过、0skip；含候选/目录错绑、价表/页故障保Token、metadata缺失/冲突、finalized估价、有效策略错实例/目录/构建/离线拒绝、状态查询不创建state-dir | `diagnostics/final-app-20261105-v2` |
| 公开Runner完整登记集 | `-level=L4 -suite=go -scenario go.runner-contract`，三根通过并停止/结算；核对新usage能力、无run_cost、缺Run只读错误及完整实例绑定 | `test-results/L4/final-runner-contract-20261105` |
| 可计价与不可计价公开联验 | Runner的 `-tags=billingacceptance -run '^TestUsageSteerLineFixtureHandoff$'` 自有owner创建全新已停止账本/checkpoint；SteerLine `-tags=runneracceptance -run '^TestBillingUsagePublishedFixture$'` 只经固定CLI读取；100输入×1 + 20输出×2 每百万，精确 USD 0.000140000000000000；completion-only保Token/PRICE_PERIOD_UNKNOWN；cleanup verified=true | `diagnostics/runner-handoff-final.jsonl`、`diagnostics/published-billing-20261105-final`、`published-fixture-20261105-final` |
| L1/L2 | 正式 `-level=L1` / `-level=L2` 通过；L1准备0.7秒/执行16.3秒，L2执行144.2秒。补充测试后的最终L1另存closure报告 | `test-results/L1/serial-final-20261105`、`test-results/L1/closure-20261105`、`test-results/L2/serial-final-20261105` |
| Web全量 | `-level=L4 -suite=web -scenario web.unit`，原生Vitest完整终态、通过/零skip，耗时26.4秒 | `test-results/L4/serial-final-web-20261105` |
| HTTP全量 | `-level=L4 -suite=go -scenario go.http`，七包全量166项通过/零skip；与E2E/Full/压力串行 | `test-results/L4/serial-final-http-20261105` |
| 环境/执行配置 | `-level=L4 -suite=go -scenario go.environment`，owner全量和Windows29个App根（含子项共131项通过）/零skip；不当作真实环境取源证明 | `test-results/L4/final-environment-20261105` |
| 构建/真实浏览器 | 最新独立包 `dev-20261105-173719-df18050e-dirty` 从当前源码实际Go/Web生产构建；零项目home显式维持后端/Web；`npm --prefix web run test:e2e -- smoke.spec.ts -g web.smoke.billing`：保存新价表、版本切换、跨标签页冲突保草稿、重读、宽屏/390px无横向溢出、Tab焦点和无JS错误通过；private读取修正后已重验 | `packages/dev-20261105-173719-df18050e-dirty/evidence/billing-browser`、`billing-browser.log`、`billing-browser-cleanup.json`（同evidence目录；两stop退出0、进程/监听0） |
| 清单/静态 | 正式清单与localSuites、Live显式选择/必需参数校验；Runner受影响包go vet；文档链接与diff空白检查 | `diagnostics/final-registry-20261105`、`diagnostics/runner-vet-final.txt`；最终链接结果见文末 |

### 真实远端取源（BILL-A10）

入口 `go run ./cmd/test-feedback -level=L4 -suite=usage -env=live -scenario usage.remote-codex -out <新目录>`。显式固定上述EXE/version/SHA、外部 `live-input.json` 和不存在的 `STEERLINE_USAGE_LIVE_ROOT`；冻结 execute/1 请求指定已配对环境 `e68f4edf-8a83-42c2-a9da-6d404d3aa3d8`，gpt-6-luna/medium，只回复单行probe，不创建PR。项目本地/远端均为本次独立 clone，未在原业务仓库工作树执行。

正式报告 `test-results/L4/20261105T085347119Z-16915efad49f4c919b1a52758bb9f195-remote/summary.json`：实际非零、0skip、通过，56.55秒。证据 `live-runs/20261105T085347119Z-16915efad49f4c919b1a52758bb9f195` 保存原请求/Run、缺授权快照、有效策略、授权后快照、完整明细、SteerLine视图、撤权快照和cleanup。

- 同一新Run `d5abd273c190810af91f6f4b643403ac`、state-dir `C:\slr\cc32lp3k4c5rs5a5nclyci7pza`：默认未授权为unavailable/tokens=null；需求来自原结构化session与核实元数据，不读取日志正文或先登记目录。
- 核实 host `sha256:1ae52a9bdd4e24da7de31bbdc2fd40af37710c2de0a47df878e97e9a40606799`、原 session 日期叶目录后显式temporary grant；正式停启同构建，新实例拒绝旧绑定，原Run/session/turn/input digest不变、没有重发模型输入。
- revision7 available/finalized，来源1/1，1响应、**16,166 Token**：input16155 = uncached4379 + cache_read11776 + cache_write0；output11（reasoning0），总量16166。固定revision全页与汇总一致，重复查询不重复计数。
- 此原生样本 time_basis=completion；SteerLine查询视图保留Token，费用为unavailable/PRICE_PERIOD_UNKNOWN。没有拿合成价格、完成时间或profile猜测真实账单。
- 撤权后已核实历史保留，access_cleanup_status=revoked；原Run stop、服务stop与VerifyServiceSettlement成功，cleanup verified=true。随后通过Orca CLI关闭/移除仅本次创建的probe workspace和bootstrap workspace，回执 `orca-workspace-cleanup.json` 两项closed/removed=true。独立clone与诊断证据保留；没有遗留本次活动会话，未改安装Task 101。

### 原失败、重跑与验证边界

1. 初期沙箱内命名互斥体/Windows ACL检查受工具权限限制、npm ci失败；经隔离范围的允许权限执行后通过，非产品根因。
2. Runner广选的 `resources.TestSRWorkflowSelection` 在原仓库也失败，三个相关文件与f6a基线及隔离副本内容相同，见 `diagnostics/runner-prompt-baseline.json`。未改提示词正文/接线/黄金样本来掩盖失败；本次不宣称Runner整仓L1/Controlled通过。
3. 第一轮真实链仅前置阻断：误读metadata文件名，尚未发模型请求；正式通过使用新的输入/root/报告，不覆盖失败 `test-results/L4/20261105T085311865Z-a8fffb7cb11748b2899b7ca313f7b973-remote`。
4. 第一轮受控账本夹具缺已停止/checkpoint事实，最新投影partial且清理证明不完整；未创建原生资源。原失败 `diagnostics/published-billing-20261105` 保留；修正Runner自有夹具，在新目录v2及final联验通过，未修改生产采集门槛。
5. 首个浏览器包通过短启动器启动后发现文件消失，未进入计费页；改用零项目home显式维持serve/Web，原失败 `packages/dev-20261105-165450-df18050e-dirty/evidence/billing-browser` 与清理保留。最新包完整通过。
6. `test-results/L1/final-20261105` 的 quick-web 因我安排的 npm ci/测试重叠而暂缺std-env，缺原生报告且整体失败；待构建完成后完全串行，serial-final的L1/L2/Web/HTTP全部通过，不把首次失败改记为通过。
7. 更早验证、取消的重复HTTP调用及后续补验均各自保留原目录；最终通过只引用上表。未执行Go Full、真实PR、其他agent/平台/取源恢复全矩阵，未跑16MiB/50k容量压力基准；已实现固定预算和失败闭环。人工BILL-M01主观体验仍可后置。
8. 原安装候选缺授权的根因已明确，但其日志当前是否仍可得、能否按新构建复观察未验证。以后安装升级需原资源停止/结算和用户明确安排；本次没有部署、改安装策略或重启原Run。新隔离Run的通过不代表原候选已恢复。

最终UI补充：重算失败保留同一完整候选身份下的旧版本估算并显式标注；价格/采集状态读取失败提示尚未核实；跨候选不复用。组件8项与生产Web构建已通过，最新L1为 `test-results/L1/ui-closure-20261105`（通过）；最新Web为 `test-results/L4/final-ui-web-20261105`（274项通过、0skip/失败）。L2复验另存 `test-results/L2/final-isolated-20261105`，不使用未执行的Web目录。该补充不改变Go/HTTP/Runner代码，前述同源码HTTP/环境/公开Runner证据保留；浏览器专项针对未再修改的全局计费设置。

最后一轮L2的原失败单列：`test-results/L2/ui-closure-20261105` 在 start_task 前由 environment_not_ready 拒绝，controlled-helper 元数据探测 context deadline exceeded；cleanup_complete=true。后端源码未改变，先前serial-final的L2仍通过；不降低探测超时或断言，按原入口在final-isolated新目录独立复验，最终结果登记如下。

L2最终复验：`test-results/L2/final-isolated-20261105/summary.json` passed=true、cleanup_complete=true，TestDeliverySmoke原断言通过（193.77秒）；原超时参数未放宽。最新UI的正式L1、L2、274项Web全量均通过。

末次静态检查：Markdown: 686 files, 4246 local navigation links, 0 errors；`git diff --check` 通过。Runner受影响包go vet退出0；没有提示词正文、接线或黄金样本变更。精确改动文件摘要和最终报告索引另存外部 `handoff/steerline-manifest.json`。


## 提交 PR 前质量审计（2026-11-05）

本轮按用户指令审计和修复，再提两仓 PR，不合并、不部署。两仓工作分支均为 codex/billing-ownership；SteerLine 已同步 main 的 9b2e5d88，Runner 已同步 main 的 a66038c8。此前 f6a/df180 的报告保留原资格，本轮集成结果另登记，不继承为最新构建通过。

| 确认问题 | 修复与回归 |
| --- | --- |
| Runner 的授权需求在重新观察失败后仍留在 checkpoint，可能在缓存发布时复活 | 每次实际重新观察先清空旧需求，仅本次核实的缺授权结果能重新建立；新测试修复前失败、修复后通过 |
| 价表体积限制漏算末尾换行，临界更新可生成超过读取上限的文件 | 对最终落盘字节检查；用精确1MiB有效原文件和generation 9→10回归，拒绝更新并保留原文 |
| 两个设置页的旧GET可在保存成功后晚到，覆盖新版本或误报已生效 | 保存成功后取消同键在途读取，再发布保存快照；延迟GET的价表/采集两组竞态测试通过 |
| 授权需求查询超过64个Run即整体失败，丢掉已核实条目 | 保留当前服务/已核实条目，返回明确截断诊断，仍不查询第65项或授权未知根；真实子进程边界回归 |
| 非Windows配置锁遇竞争立即失败，无法返回串行比较后的412 | 使用阻塞锁串行读取/比较/发布；现有Docker WSL执行静态Linux测试，无需安装工具 |
| Unix home只大小写不同的链接路径被错误当作同一路径 | 仅Windows忽略大小写；Linux仅大小写链接home回归通过 |
| 授权需求查询取消后仍继续消耗后续查询额度 | 每次公开查询前检查上下文，立即停止并保留已核实需求；与第64/65项边界一同回归 |
| 保存未变化或改回已生效策略时仍提示必须重启 | 保存通知引用实际生效状态，不把保存动作等同待重启；改回生效策略的组件断言通过 |

隔离审计证据根：C:/SteerlineDebug/wt-1148-billing/audit/20261105T102227Z。owners、app的原始诊断均通过；Linux六根全部通过，见 linux/result.json 与 test.txt；Runner修复前失败 diagnostics/audit-stale-requirement-before.jsonl、修复后 diagnostics/audit-runner-after.jsonl 位于共同scope根。目标旧GET竞态测试曾暴露busy禁用查询后的重读时序问题，已去掉不必要禁用，13项定向Web通过；原失败不改记通过。

主干冲突保留原native-catalog及文档；自动冲突处理曾意外截尾，已从三个原Git对象重做合并并逐字校验尾部，最终差异保留完整主干。未改提示词或其黄金样本。最新固定Runner与正式回归见后续结果，配套Runner PR为 [#120](https://github.com/erranliu/SteerRunner/pull/120)。

### 审计最终版本验证

本节路径相对上述 audit 根。受测 SteerLine 运行代码提交 `f75e67f41ab1eed89835f11c0f0796b4f77bacb4`；后续仅更新验收文档。Runner 受测提交 `3bac937bfb62c3a4a74889c3b7b46627f6a36dcf`，独立构建 `0.2.0-dev.billing-audit`，SHA256 `125124b9f8f835fc886c7bd1cab025da12b67d9a2242ea0d8bbf20e017a4c55b`，source digest `f378f808e8f8ff807c2f9445de661f58db95cbcaf354452134a3637b9a5f7252`，source_dirty=false。不是前文历史 billing2 构建。

- `owners`、`app`、`app-boundary`：相关 owner 与应用事实组合通过；边界测试实际执行第64次子进程查询，第65次与取消后不再执行，保留已核实需求。
- `runner-selection.json` 明确逐包选择96个根；`runner-test-summary.json` 为245项（含子项）通过、0skip/0fail，114.99秒；`runner-vet.txt` 退出0。未将有同名测试的其他包误选入，不宣称整仓通过。
- `runner-contract` 正式公开契约三根通过；`published` 经固定CLI消费新的Runner自有已停止夹具，通过精确USD 0.00014及completion-only不可计价断言；`runner-fixture.jsonl`、`published-fixture` 保留创建和结算证据。
- `final-L1`、`final-L2`、`final-web` 正式全部通过，Web 290项/零skip；L1执行31.27秒。`http` 七owner全量通过（96.6秒），与E2E串行。
- `linux/result.json` 与 `test.txt`：已有Docker WSL中真实执行静态Linux配置测试，六根通过、零skip，含跨进程锁和大小写链接；不只做交叉编译。
- 独立包 `packages/dev-20261105-190641-f75e67f4` 完整生产构建与真实浏览器 `web.smoke.billing` 通过，见包内 `evidence/billing-browser`；`browser-result.json` 确认清理完成，无本轮包服务进程遗留。
- `remote/summary.json` 为正式live通过（45.51秒）。新Run `56da21db97db98a07d6485c0e4816343` 在同一原绑定内完成默认拒绝→核实叶目录授权→正式停启→读取→去重→撤权保历史→原Run停止/服务结算；revision7、source1/1、1响应、14,862 Token（输入14,851，其中uncached3,075/cache_read11,776；输出11）。原生时间仍为completion，费用保持PRICE_PERIOD_UNKNOWN。`live/cleanup.json` verified=true，`orca-workspace-cleanup.json` 确认仅本轮workspace关闭/移除。未重新发送原输入，未操作安装Task101。

环境全量的第一次 `environment` 因 `TestEnvironmentDetectionNeedsNoProjectCatalogueOrPlatformCopies` 中夹具Git初始化超时失败；保留原报告。原断言单独在 `environment-isolated` 通过（29.10秒），没有放宽超时。最后 `environment-final/summary.json` 正式全量 passed=true/complete=true，App组739.77秒；原参数和全部登记根均保留，未跳过失败用例。

最终静态检查：`registry-final` 三根正式清单/显式live选择回归通过；Markdown 694文件、4277本地导航链接、0错误；两仓 `git diff --check` 通过。进程核实仅筛选本轮独立scope，无遗留Runner/后端/Web服务；未停止或改动其他聊天的安装/测试进程。

提交PR前刷新：SteerLine目标main仍为 `9b2e5d886b8f56eaf07bc697ca78c578c91eec00`。Runner main前进到 `a78d87da05120b7e6b99981b396d6cecd4d60d92`（#118能力传输复用）；与受测分支的 `git merge-tree --write-tree` 返回0、无内容冲突，不为Rebase提示重写受测历史。本节结果仅属于上述受测head，不冒充最新main合并后复验。
