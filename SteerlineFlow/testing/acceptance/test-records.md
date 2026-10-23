# 测试集执行记录与只读测试管理验收

唯一归属：[测试记录 Spec](../../spec/test-records.spec.md) · [Plan](../../plans/test-records.plan.md)。2026-10-21 用户已确认范围并授权按修订后的 Spec/Plan 实施；本轮实现及自动验证结果在下方登记。无 roadmap Task ID。

## 自动验收

各项的级别/组/环境/命令/执行时点和证据安排见 Plan“测试计划”。以下要求不表示已通过；无报告、零匹配、编译成功、跳过或原型模拟不能替代产品行为。

### TR-A01

状态：自动验证通过（2026-10-21；命令、范围和证据见“实施结果登记”）。脚本输出统一真实报告，按入口报告唯一 ID、执行开始前冻结定义/选择、时间、固定源码/dirty/unknown、实际结果和证据正确；失败、未完成、子集、零匹配不能冒充整体通过；登记结果独立且原测试退出码保持。

### TR-A02

状态：自动验证通过（2026-10-21；命令、范围和证据见“实施结果登记”）。独立脚本不绑定 Task/Candidate 也能通过正式 CLI 登记/补登记；认证、项目/repository、目录/链接/证据摘要/预算严格；每份报告原子保存、同内容幂等/内容冲突拒绝，1 MiB 编码边界、缺失/过大 summary 仍留可查询记录，同摘要补齐与错误摘要拒绝正确；离线不丢本地报告、不重跑测试。

### TR-A03

状态：自动验证通过（2026-10-21；命令、范围和证据见“实施结果登记”）。显式父子链中失败→失败→通过、跨修正 SHA、部分/后继再失败/未完成、定义/环境/范围变化及脏/未知源码正确；只有单次完整匹配且必要证据齐全的后继报告解除失败，脏/未知源码的复验通过明确标注非固定版本证明，不强制先提交，多份子集/不同 SHA 不能拼成通过；无关联不解除，分叉分别展示，迟到/筛选不改顺序和原始失败，不以跨机器时钟拒绝真实复验。

### TR-A04

状态：自动验证通过（2026-10-21；命令、范围和证据见“实施结果登记”）。Runner 原公开 artifact 保留原 Evidence 后登记项目记录，脚本和 Runner 同份报告去重；索引暂时写失败不吞原证据/不推进投影 artifact 数量，候选随后放弃/交付或 Runner 离线仍由本服务已存证据补登，成功后不全库重放；PlanTests 仍读 EvidenceView，固定 Plan/定义/候选/Run/Git/必要证据不放宽，独立记录不冒充候选证明；无新增模型/执行路径。

### TR-A05

状态：自动验证通过（2026-10-21；命令、范围和证据见“实施结果登记”）。目录来源、无效/缺失/历史定义、组合筛选、明确汇总口径、游标入库上界/并发新增、完整链和有界 summary/不可用诊断查询正确；项目/实例/筛选游标隔离、查询无执行/写结果副作用。

### TR-A06

状态：自动验证通过（2026-10-21；命令、范围和证据见“实施结果登记”）。正式 Web 导航、记录/定义、筛选/URL/空与错误、展开/证据/来源候选、原 PlanTests 跳转正确；离线最后观察、隐藏停轮询、项目切换/迟到响应不串记录；页面无执行测试或改结果入口。

### TR-A07

状态：自动验证通过（2026-10-21；命令、范围和证据见“实施结果登记”）。唯一生成器创建新独立 debug 包、SteerlineTest，真实脚本断言经 CLI/SQLite/API/浏览器闭环，只覆盖实际小型脚本失败→提交修正→通过→原报告幂等重放；部分/离线/第二项目等矩阵由相关 L1 和组件测试覆盖，不在此重复全矩阵；无真实模型、无安装目录写入，退出和资源清理证据完整。

## 人工验收

### TR-M01

状态：待人工，窗口在自动验证之后由用户安排。体验原始结果与复验状态、筛选和汇总口径、详情密度、证据可读性及窄屏。可后置，仅阻塞体验签收；真实失败和必要自动验收不豁免。

记录参与人、产品版本/环境、实际步骤、实际/预期、证据和结果；失败与后续复验分别保留。当前无人工产品验收结果。

## 原型与文档检查

本节只记录本轮设计交付，不能据此标记 TR-A01–A07 或 TR-M01 通过。

2026-10-21：已更新 [HTML](../../../docs/product-design/prototypes/planning-ui/index.html) 与 [README](../../../docs/product-design/prototypes/planning-ui/README.md)。在 Edge headless / Playwright 中打开本地静态 HTML，验证原始失败保留、显式完整链、部分/定义变化、环境/分支/日期筛选、无记录/无搜索命中、最新失败、只读入口、候选跳转及离线；桌面 1450×1100、窄屏 430×950 截图检查。浏览器零 JS 异常，不启动 SteerLine/Runner 或创建产品调试环境。

设计检查的文件摘要、检查范围和本地截图位置见[最小证据](evidence/test-records-prototype-20261021.json)。文档链接和空白检查结果在该证据中记录；该次设计检查时，产品场景 testing.records 尚未实现/运行；本轮产品结果见下文。原型未实现真实导入、分页、项目切换、分叉或真实证据读取。

## Spec / Plan 质量审计（2026-10-21）

本轮为主聊天对照已确认需求和现有代码的审计，已直接修订文档；不等于产品验收。确认问题与修复如下：

| 问题 | 修复及取舍 |
| --- | --- |
| 跨提交/多次子集通过可被合成“复验通过”，没有单次完整报告支撑 | 只接受最新一份可比完整后继；多个部分报告不拼通过，较新失败/未完成生效；原型加两个反例 |
| 为新分页要求候选 PlanTests 改读新存储，扩大现有交付链重构范围；只靠候选继续被观察也会漏掉已放弃记录 | 原 EvidenceView 保持；项目记录是同报告的查询索引，原 Evidence 的已投影 artifact 数量 支持 tick 从已存证据补登，不依赖候选仍活跃/Runner 在线 |
| 强制多入口统一大报告/预期范围协议及全目录事务，和现有按 ID/环境输出不符 | 扩展原 Report，保留 managed-<id>.json/每报告原子登记；范围只适配已有可报告能力，不建新注册表 |
| 1 MiB 现有 HTTP 预算未落实，且缺证据直接拒绝会让失败历史消失 | 明确报告/summary/编码预算，缺/大 summary 可留带诊断记录，不能解除失败；无新分片/附件平台 |
| 假定 CLI 有 home 参数，复验按跨机时间准入，且未安排执行前冻结清单 | 复用 STEERLINE_HOME 和独立 register 参数；先冻结定义/选择，顺序用显式引用，时间仅校验单报告 |
| 只读脚本记录错误套用候选发布的 clean/固定 SHA 准入，日常改代码后的实际通过不能显示复验通过 | 记录保留并按实际匹配报告展示复验通过，同时显著标注脏/未知源码，不能作为固定 SHA 证明；原候选规则保持 |
| 多组反复跑所有 TestTR，受控 L3 重复完整故障矩阵且缺小型真实生产者安排 | 测试按 Producer/Register/Retest/Runner/Query 职责前缀归属；唯一 L3 用真实小型项目脚本，不跑全 L1/递归本场景 |

审计后的静态检查、验收映射、原型反例检查和当次文件摘要追加在[设计证据](evidence/test-records-prototype-20261021.json)的 audit 条目。该文件原始记录/摘要保留，代表上轮设计快照，不冒充修订后的摘要。该次设计审计时产品 TR-A01–A07 全部待实施；本轮实施结果见下文。

## 实施结果登记

2026-10-21：按已确认 Spec/Plan 完成实现。源码基线 `b9e5002d38da4722dc8457338e063ce56817fb48`，本次为 dirty 工作树；未提交、发布或操作安装环境。实现文件摘要、所有实际证据路径及报告/清理事实见[本轮最小证据](evidence/test-records-implementation-20261021.json)。

| 验收项 | 实际覆盖与结果 |
| --- | --- |
| TR-A01 | `TestTRProducer*`、`TestTMManaged*`、`TestTMRegistered*` 和实际 marker 脚本验证执行前冻结、不同记录 ID、真实失败/未完成/零匹配及选集限制。Git 不可用保留空元数据，不把错误文本当 SHA。 |
| TR-A02 | `TestTRRegister*` 验证离线原报告不变、认证/项目/repository、普通文件与链接/预算、同报告去重/冲突、同摘要补齐/不降级、Runner 来源补齐/绑定冲突及正式信封 1 MiB 临界值。真实 CLI 登记/补登记另由 TR-A07 闭环证明。 |
| TR-A03 | `TestTRRetest*` 验证单份完整后继、部分/多份/跨 SHA 不拼接、较新失败/未完成/缺证据、定义/环境变化、无关联/分叉/时钟偏差，dirty/unknown 允许复验但不称固定源码证明。 |
| TR-A04 | `TestTRRunner*`、`TestTMEvidence*`、`TestTMPlan*` 验证索引失败不推进/不损原证据，已放弃候选的已存证据补登、Git-only 更新不重放、追加 artifact 再投影；原 PlanTests 的身份、Git 新鲜度和证据门槛保留，新增摘要声明核对。 |
| TR-A05 | `TestTRQuery*` 验证观察 commit 目录与历史定义、查询/过滤/时区范围、分页入库上界/筛选和项目隔离、链跨页、历史定义最新入库项与毫秒排序、认证转发和查询零控制副作用。 |
| TR-A06 | Vitest 覆盖只读记录、筛选/定义、证据缺失/刷新、原失败与非固定源码复验标记、候选来源定位、PlanTests 跳转、离线/隐藏、项目切换及迟到响应隔离；57 项通过，TypeScript/Vite 生产构建通过。实际桌面页面/证据截图已自查；用户体验签收仍为 TR-M01。 |
| TR-A07 | 最终 `testing.records` 实际通过：新包 `C:\SteerlineDebug\dev-20261021-160902-b9e5002d-dirty`，SteerlineTest，冻结 simulate；真实脚本退出 1→提交修正→退出 0，CLI 登记/原报告重放后仅 2 条，API 和 Edge 查看原始失败、完整目录、显式复验链及摘要。外层 scenario `passed=true/cleanup_complete=true`，包内 proof `passed=true/stopped=true/resources_settled=true`。 |

实际执行入口（每次均使用新 out，完整命令/日志位置在证据 JSON）：

- 定向 Go：`go run ./cmd/test-feedback -out <新目录> -- ./internal/testmanagement ./internal/storage/sqlite ./internal/app ./internal/product/httpapi ./internal/product/applicationclient ./cmd/steerline ./cmd/test-feedback ./tests/e2e/internal/driver ./web/server -run '^(TestTR|TestTM)' -count=1`；62 项通过、零失败/跳过，完整原始 JSONL 保留在 `C:\Users\lmas1\AppData\Local\Temp\steerline-tr-l1-final-20261021-1613`。
- 前端：`npm --prefix web test -- src/features/tests src/features/runs/plan-tests.test.tsx src/app/project-switch.test.tsx src/shared/api`；57 项通过，`npm --prefix web run build` 通过，日志在 `C:\Users\lmas1\AppData\Local\Temp\steerline-tr-front-final-20261021-1614`。
- 闭环：`go run ./cmd/test-feedback -level L3 -suite testing -scenario 'testing.records' -env controlled -out <新目录>`；最终结果在 `C:\Users\lmas1\AppData\Local\Temp\steerline-tr-flow-final-20261021-1609`。这是唯一选定受控小脚本流程，不代表完整 L1/L2/L3 或真实模型/PR 通过。
- 最小入口/清单选择回归 `go test ./cmd/test-feedback -run 'Test.*(Scope|Quick|Owner|L3|L4|Scenario|Manifest)' -count=1` 通过。新用例按 owner 纳入原 L1 最小入口；不递归运行 testing.records。
- 依赖边界 `go test ./internal/testkit/boundary/... -count=1`、文档本地链接检查及 `git diff --check` 通过；未运行全仓全量测试。

最终实际记录：原失败 `752718daf9a74cb6b34f15376b3773b0`，修正后通过 `b70da27d844f4ce791003a1705dfaae0`；前 SHA `2cee92b92c88616ed2d59f4729862e528e6ca73e`，修正 SHA `17b14b2c611c4e8ff3ee5f519585f86ceb722629`。原始 failed 保留，显式后继对应复验通过，均为独立脚本来源，无候选绑定。

自查修正：页面路由同时补齐 React 项目路径识别与 Go SPA 直达入口；大 producer sequence 在查询 DTO 中转字符串；登记回执按规范化报告摘要核对；源记录不可变/证据补齐不降级；历史目录明确取最近入库定义，时间索引用固定 UTC 精度；拒绝报告/summary 链接越界及硬链接；刷新同步当前证据并清除分页上界；候选与独立历史共享当前报告解析，保留原候选证据 owner。未新增上传会话、修复工作流、兼容迁移、测试执行器或后台 worker。

保留先前真实失败：`steerline-tr-flow-20261021-1551` 的驱动源码路径被准备阶段清理；`1555` 的浏览器路由识别遗漏；`1559` 的测试代码 TypeScript 参数不合法导致包构建失败；`1601` 的 Go SPA 直达 URL 未放行。上述原 summary/现场保留，前两类准备/构建失败未进入产品运行，两次浏览器失败现场均已停服并核实资源。修正后 `1605` 首次全链通过，随后最终 `1613` Go 定向回归及 `flow-final-20261021-1609` 当前闭环通过；旧失败不改写为通过。

TR-M01 仍待用户安排人工体验验收，只阻塞体验签收。


## 实现质量复审与修复（2026-10-21）

本轮由当前聊天复审并修复，未声称独立代理复审。已集成主干 `6cf83c63662f8f586e0922e11cdbbc4f2098bdef`，保留自动材料生成及过期候选测试限制；本功能 schema 使用 28，不对旧版迁移。逐项证据、命令、文件摘要及旧失败见[本轮审计证据](evidence/test-records-audit-20261021.json)，前轮证据保持原样。

| 审计问题 | 修复与验证 |
| --- | --- |
| 登记对账借用 UI 详情，耦合整套历史投影，且不能完整重验附带证据 | owner 提供单记录回执读取，完整验证原报告及 summary；HTTP 不依赖 UI DTO。TR-A02 新反例验证已提交但回执未知、无重复提交、错误证据/未知字段拒绝。 |
| 暂时存储错误错误归为确定拒绝，CLI 与 test-feedback 的登记循环和落盘重复 | 明确有效输入错误/身份冲突与未知结果，共用发送前落盘、按尝试追加的本地日志；原报告及原未知回执保留。TR-A01/A02 定向验证。 |
| 首批坏候选可长期阻塞后续记录，同批父子报告依赖 artifact 枚举顺序 | 有界批次逐项处理、进程内轮转和已有父项先登记；不增加持久队列/新 worker，原证据与完成数量不误推进。TR-A04 及 SQLite 轮转/重试验证。 |
| 环境变化被定义摘要变化覆盖，且生产者直到执行后才暴露环境不匹配 | 复验优先判环境差异，执行前冻结时拒绝清单/实际环境不符。TR-A01/A03 验证。 |
| 冗余解析包装/未用参数，build-only 和已有日志汇总不必要地依赖报告清单 | 共用一个公开报告解析入口，移除未用参数；只在实际登记级别执行时冻结报告。入口/清单选择回归通过。 |
| 最新主干集成后的测试夹具及调试证据目录不完整 | 补齐项目切换维护接口夹具；共享自动材料驱动先创建自己拥有的证据目录。保留自动生成语义，不恢复人工 apply。新包重新验证通过。 |

本轮实际验证：

- TR-A01–A05 及候选材料/读取回归：9 个包共 **77 项通过，零失败/跳过**。证据 `C:\Users\lmas1\AppData\Local\Temp\steerline-tr-audit-final-l1-20261021-1715`；目录创建修复后的 driver 定向测试另通过。
- TR-A06：6 个前端文件 **59 项通过**，无未处理异常；TypeScript/Vite 生产构建通过。测试入口/owner/manifest 和边界检查通过；本地文档链接及空白检查通过。
- TR-A07：`testing.records` 在新包 `C:\SteerlineDebug\dev-20261021-171607-3101a288-dirty` 实际通过。正式脚本失败→提交修正→通过→CLI 登记/原报告重放→API/Edge 查询；仍仅两条业务记录，失败报告登记日志保留两次尝试。外层 `passed/cleanup_complete` 与包内 `passed/stopped/resources_settled` 均为 true；页面截图已自查。
- 首次主干集成闭环 `steerline-tr-audit-flow-20261021-1712` 在准备阶段因证据目录缺失失败，未启动浏览器；原 failed proof 不改写。随后对原包正式 `verify-stop` 返回零、资源结算 true，追加独立清理证据；修复后使用上述全新包验证。

未运行全仓/完整 L1/L2/L3/L4 或真实模型；TR-M01 仍待用户体验验收，仅阻塞体验签收。Spec/Plan 已同步本轮所有权、错误分类及必要测试安排，Plan 不维护实时结果。
