# App领域专项与实际参数报告：验收规划及断言梳理

日期：2026-11-03。状态：已实施，自动验收与独立审核通过。依据[Spec](../../spec/app-domain-test-suites.spec.md)与[Plan](../../plans/app-domain-test-suites.plan.md)。用户已授权按Spec＋Plan实施、独立审核通过后提PR；重型用例仅梳理、不放宽断言，不进入roadmap。

## 验收项与准备

| ID | 方法 / 最小通过条件 | 当前状态 |
| --- | --- | --- |
| ADT-A1 | 自动：go.deployment的host/localservice活动owner全量及App9/8根与升级排空原根通过，原子写入/回收/冻结/ACL原断言和清理保留 | 正式passed/complete：24根79终态，0fail/skip |
| ADT-A2 | 自动：go.roadmap全量owner、App11根与host1根通过，LF/CRLF及重开边界完整，live不执行 | 正式passed/complete：32根72终态，0fail/skip |
| ADT-A3 | 自动：源码/平台/标签/定义漂移、缺失/重复/额外/skip等负例经最终报告/managed/进度明确失败 | 完整owner03通过，229终态0fail/skip，独审已复核 |
| ADT-A4 | 自动：各类实际命令/构建标签/并发层次与每组及顶层报告逐项相符，默认/mixed/未知/不适用不伪装实测 | L1/Full/专项/诊断/Web/E2E实际参数与原始命令一致，核验通过 |
| ADT-A5 | 自动静态核对＋文档审阅：四根全部子例/动态fault、断言与层级、前置/资源/清理矩阵，原函数及辅助正文/期限零变化 | 矩阵已独立复核，保护文件零diff；不实施移层/去重 |
| ADT-A6 | 自动：最终L1/L2、代表L3、Web、Full原断言完整通过，旧普通根0缺失、0实际skip、必要清理完整 | L1/L2/代表L3/Web与首次Full通过，原626根零缺失，清理完整 |
| ADT-A7 | 自动静态核对＋独立审核：清单/文档/实现一致，提示词实际正文diff及字节审计，修正与证据复核通过 | 正文/字节、实现与最终证据独立终审通过 |

无新增人工项。实际命令、环境、执行时点和通过条件由Plan逐项映射；实际实施前固定commit/dirty、Go/Git/Node/平台、定义摘要与新外部scope绝对路径。原始JSONL、stdout/stderr、summary、managed、清理和失败放各次独立叶目录，不新增入Git。文档轮没有源码执行scope，也不复制上一轮报告为本轮结果。

规划基线 `4920f452d2358b62c3230f7b5f4a04516a4aa862`。上轮Full1572.484s、App1260.187s只作为瓶颈定位；以下134.56/112.57/60.46/63.07秒也只是该次原始根耗时，不作为稳定性或当前性能通过依据。

## 重型断言矩阵（静态梳理）

本表初稿来自规划源码与历史日志；实施轮已按cca5114逐分支复核并冻结，独立审阅与实际结果见后文。当前四根及全部子测试都保留原L4普通Go集成归属；表中“规则候选”只解释直接验证职责，不授权迁移、合并或删除。共用fixture不代表重复覆盖。

### H1：Runner预设冻结，134.56秒

来源：[runner_preset_test.go](../../../internal/app/runner_preset_test.go)，根 `TestRunnerPresetFreeze`。

| 实际子例 / 验证单元 | 必须保留的事实与失败条件 | 直接验证层级 / 前置与资源 |
| --- | --- | --- |
| `parameter_digest` | ID/名称/JSON对象顺序不改参数摘要；显式默认及空workflow保留区分；顶层/嵌套/数组重复键拒绝，原配置重复键不先丢失 | owner规则候选；不需要服务启停。当前仍属于本根，不删除重复键任何原反例 |
| `reject_replacement_in_live_runtime` | A绑定不变，替换B后submit/reply/resume/stop/inspect与产品控制均拒绝；零业务派发；A不能停止/结算B，Close报告原资源未知；释放锁是原路径注释说明，该子例没有重新获锁的直接断言 | App集成；真实目录/数据库与公开进程Runner fixture，cleanup明确停止自有B |
| `reject_mismatched_run_after_explicit_restart` | 原A不能假结算；显式重建后原Run输入摘要不匹配即拒绝准入，零业务派发 | App集成；原Run/请求/输入固定及公开观察；不是仅比较字符串的单测 |
| `accept_replacement_after_explicit_restart` | 只有重建的host绑定B；旧A保持原绑定；读取新预设而历史候选名/Run/输入/Submit/Actions不变；不自动发新输入；A旧stop重放不能停止B；B最终停机证明绑定原实例/请求并结算 | App集成；多次查询、启停/重建是被测动作，不预先当冗余准备删除 |
| `reject_changed_build_without_compatible_takeover` | 状态格式/namespace不兼容即失败；不改原build pin，不启动或替换原服务，保留原停机身份 | App与Runner边界集成；fixture版本/命名空间变化为故障输入，不是真实Runner升级证明 |

`seedStoppedPublicRun`包括正式Task/材料/候选创建和存储资格前置；它直接写入ready仅服务生命周期夹具，不证明环境准备实际执行，准备链的独有证据仍归原readiness L3。后续优化候选只调查准备/查询成本，尚无可删除断言或步骤结论。

### H2：部署中断恢复，112.57秒

来源：[deployment_recovery_test.go](../../../internal/app/deployment_recovery_test.go)，根 `TestDeploymentInterruptedRecovery`。

| 分支 | 必须保留的事实与失败条件 | 直接验证层级 / 准备与副作用 |
| --- | --- | --- |
| `stale`、`missing` | 在startup协调锁仍持有下恢复且恰调用一次cycle；保留旧回执是否存在的事实、原中断host与新host区分；恢复不把native未知写成已验证；候选/Run/PR/input字节不变 | App集成；原部署文件、检查点、Runner观察和候选快照，restore只恢复本例基线 |
| `preparation-update` | 经原中断恢复来源，writer退出verified但native仍unverified，原恢复证明完整 | App集成；验证组合准备语义，不由普通owner枚举替代 |
| `preparation-failed-cycle`、`preparation-replace` | failed-cycle只调用一次、replace不调用cycle，均blocked；不得重试或绕过goal | App集成与局部门禁；本轮不拆分 |
| 23项fault循环 | 每项必须返回error；前20项cycle不得进入，末3项cycle恰执行一次；意外进入原startup直接Fatal。故障输入涉及原candidate/Run/generation/文件，但不能把构造输入或restore误写成该负例已断言所有副作用不变 | App集成；活体身份、租约、原回执、包/配置/状态绑定和项目锁均为原门禁输入，不改为纯fixture返回码测试 |

23项fault按源码循环展开如下。每项都保留error断言和cycle次数；表中未列的候选/文件不变性不冒称已由该负例直接证明。

| fault | 实际故障输入 | cycle期望 |
| --- | --- | --- |
| `active-host` | 仍活着的当前进程替代原已退出host身份 | 0 |
| `unknown-launch` | launch只有attempt，没有process身份 | 0 |
| `foreign-launch` | launch进程创建代次不同 | 0 |
| `host-lease` | 持有backend目录锁 | 0 |
| `web-lease` | 持有Web目录锁 | 0 |
| `runner-active` | 原Runner观察仍ready | 0 |
| `matching-receipt` | 原host存在匹配的dirty shutdown回执 | 0 |
| `conflicting-receipt` | 同instance回执的进程创建代次冲突 | 0 |
| `corrupt-receipt` | shutdown JSON损坏 | 0 |
| `missing-release` | 缺deployment记录 | 0 |
| `foreign-release` | deployment记录home不同 | 0 |
| `changed-exe` | Web可执行字节不同 | 0 |
| `wrong-pin` | Runner pin的SHA不同 | 0 |
| `changed-config` | 项目配置原字节变化 | 0 |
| `checkpoint-host` | checkpoint的host创建代次不同 | 0 |
| `project-lock` | 持有原项目run目录锁 | 0 |
| `removed-project` | 原项目配置移除 | 0 |
| `incompatible` | 删除原candidate.RunID的公开inspection | 0 |
| `missing-run` | 同样删除原Run的公开inspection；当前与上项构造相同，先记录，不自动去重 | 0 |
| `wrong-input` | 公开inspection的InputDigest不同 | 0 |
| `failed-cycle` | 原cycle返回startup失败 | 1 |
| `no-new-receipt` | cycle返回nil但不生成新回执 | 1 |
| `changed-during-cycle` | cycle改变全局执行配置字节且不补证明 | 1 |

该根共有2个receipt子例、3个preparation子例和23个fault子例。每次保留原restore及锁释放；两个正向receipt子例直接比较恢复前后候选完整JSON。准备会建真实仓库/配置、生成原停机Run与检查点、收集原文件快照；末尾restore是清理动作，不额外宣称全程零副作用证明。本轮不改构造或恢复顺序。

### H3/H4：Roadmap发布与准入，60.46 / 63.07秒

来源：[roadmap_publication_test.go](../../../internal/app/roadmap_publication_test.go)，根 `TestRoadmapPublicationAndAdmissionWithRealGit`、`TestRoadmapPublicationAndAdmissionWithCRLFCheckout`；共同调用原 `roadmapPromotionChainWithLineEndings` 的controlled分支。

| 验证单元 | 必须保留的事实与失败条件 | 直接验证层级 / 前置与副作用 |
| --- | --- | --- |
| 公开静态准备链 | fresh preparation.Ready=false；经roadmap_prepare_static成功后未提交/发布仍Ready=false；提交push后Ready=true | App公开动作集成，三个不同阶段的门槛均为直接断言，不能归成同一步初始化 |
| 原始材料与固定输入 | 正式静态材料准备、解析/确认、冻结code commit；脏工作区不能进入固定文档快照；生成文档、调查结果、publication preview及无提前正式映射 | App跨owner集成；真实Git、原资料与Task写入，Runner/provider只在公开边界受控 |
| provider首次创建与合入门禁 | POST创建原PR后故意丢回执；合入请求必须给原head SHA、do=merge且非force，随后在本地bare仓库真实合入 | App跨系统流程；HTTP provider是受控边界，本地Git操作是真实owner行为 |
| 发布未知与继续 | 首次publish必须error且pr_unknown，不能把丢回执当applied；observe恢复awaiting-merge，显式resume到awaiting-admission且MergedCommit非空 | App集成；绑定原发布请求和原PR，不是重发新请求 |
| 发布后零提前Task副作用 | TaskGraph仍0任务；主工作区HEAD仍为原code，源Markdown仍逐字节等于sourceBody | App集成；直接观察数据库、Git与文件，不用最终状态替代 |
| 正式准入 | admit后state=admitted、Mapping恰2；TaskGraph恰2任务/1依赖边 | App集成；编号分配的更细原子性/来源拒绝由另选App根直接覆盖，不冒称本根核对每个编号值 |
| 准入对账与重放 | reconcile后Mapping摘要不变、首项Spec路径不改名；resumeAdmission成功，最终creates=1/merges=1 | App集成；本地bare仓库/HTTP provider、临时分支与数据库，server/store/目录归创建者清理 |
| CRLF独有分支 | 检出换行差异不制造source.Modified；保留真实原始body/digest；只改换行允许原确认，实质源修改拒绝启动发布且无Publication副作用 | App与Git/字节边界集成；不能因共用主链就删去CRLF根或归一化掉原字节断言 |
| live分支边界 | 两个普通根固定controlled，不执行带e2e标签的原生模型分支 | 范围说明；不把controlled链通过登记为真实模型/远端PR通过 |

两个普通根固定live=false，因此不进入actualProject项目来源分支，expectedTasks=2、expectedEdges=1；actualProject的6任务/6边和原生模型证据只属于live。共同辅助还保留所有require错误检查、promotionAction响应类型检查、awaitPromotion目标状态检查及原有有界等待。controlled路径关闭HTTP server和store、使用独立临时目录；原生Runner停止与VerifyServiceSettlement只在live分支执行，不把它写成本轮两个普通根的直接清理断言。实施第一步冻结共同辅助源码并逐项复核矩阵，不存在已批准的跨层去重或“两个链只保留一个”的决定。

## 后续候选与限制

仅允许输出建议：区分纯摘要规则与真实生命周期；识别重复建库、Git发现、查询或启停是否只是前置；指出可共享的只读输入以及仍须独立的可写现场。每项标明直接受测行为、拟替代证据、尚缺的等价证明与风险。没有证明时标“待确认”，不把循环fault、负例、真实绑定、原回执或资源清理折叠掉。

本轮不设重型根/Full提速门槛，不做长集择优、条件skip、重试或放宽期限。最终梳理完成与代码验收分开登记；本文件新增不代表ADT-A5已通过。

## 文档轮检查与后续结果

以下为规划轮历史：当时只做文档链接/根名/平台/标签/分支清单静态核对，功能、性能、独立实现审核、提交及PR均未执行。实施后按新scope分别追加每项实际命令、commit/dirty、原始证据、错误、修正、原断言与正文/字节审计及审核结论，不能用上轮Full或性能结果填本轮通过。

2026-11-03文档轮检查完成：

- `python scripts/check_doc_links.py`：662个Markdown、4122个本地导航链接，0错误；`git diff --check`通过。
- 规划轮4920逐文件静态核对部署App9/8根、Roadmap当时App7根/host1根；实施集成cca后App增至11根，见下文；核对localservice仅Windows测试、Roadmap live的e2e标签，正式源根名与表格一致。
- 原源码的23个deployment fault与矩阵逐项一一对应，Runner预设5个显式子例均列出；修正了初稿对故障数量的误计，并区分直接断言与准备/restore动作。Roadmap未知发布、原PR对账、合入、零提前Task、准入数量/重放及CRLF独有分支逐项梳理。
- 只有本Spec、Plan、本记录及两处导航共5个Markdown变更；Go、正式test-list、L1–L4列表、CI、固定正文和黄金均未改。没有运行本轮功能/性能测试，ADT-A1至A7仍待验收；静态文档检查不替代实施后的独立审核。


### 实施基线集成

实际基线为`cca5114297564df176b27145e66fda71d2582682`，上一轮PR #438已合并。本轮继承主干Roadmap静态初始化四根：`TestRoadmapStaticInitializationAndPublication`、`TestRoadmapStaticInitializationRejectsGitOperation`、`TestRoadmapInitializationPendingReceiptReconcilesWithoutWriting`、`TestRoadmapPublishedGuideMustBeValidUTF8AndBounded`，位于`internal/app/roadmap_preparation_test.go`，全部纳入明确映射；App原7根全保留，当前11根。重型源码按此集成基线冻结，主干已有更新不记作本轮修改；本轮仍不改业务/测试辅助正文。


### 独立断言梳理复核与后续候选

独立只读审核adt_review确认23个fault、cycle次数、正向候选JSON比较及负例直接error/cycle层次准确；明确补列主干公开静态准备三阶段断言，并纠正Close释放锁的证据层次。protected-source-baseline.json冻结976个生产/业务测试/资产文件，全部相对实际基线字节无变化，重型根及辅助没有改写。

| 候选 | 本轮结论 | 后续若要改变，尚缺的等价证据 |
| --- | --- | --- |
| C1 摘要规则独立owner | 仅候选；parameter_digest仍原样执行 | ID/名称/对象顺序、显式默认/空workflow、三类重复键和原配置严格解析全部输入/断言逐项映射；不能替代真实生命周期链 |
| C2 准备/Git发现/重复查询 | 待确认，没有耗时细分证据，不称已证明冗余 | 实际调用身份、可变状态读取边界、准备与被测动作区别、清理归属及耗时来源；共享只能是有明确等价证明的只读前置 |
| C3 incompatible与missing-run相同构造 | 记录相同输入事实，两个原子例保留 | 各自产品语义与实际拒绝路径的对应证明；同返回error和名称相似不足以批准去重 |

没有实施任何候选；不新增业务断言、分段埋点或重跑长集来包装本轮梳理成果。


主干直接部署门禁补充：`internal/runnerclient/upgrade_test.go`的原升级排空根精确纳入go.deployment，原候选证明、两业务读取器与故障子例保留。Roadmap live文件的两个e2e根均显式排除且核对实际标签，非当前平台源码也检查。


## 本轮实施与早期结果

用户已授权实施及独立审核后提PR。工作分支codex/app-domain-test-suites，实际基线cca5114297564df176b27145e66fda71d2582682；独占证据scope为`C:/Users/lmas1/AppData/Local/Temp/steerline-adt-20261103-036f478c/`。未使用安装现场、真实模型或远端测试PR。规划stash按固定SHA7243e202保留；恢复导航的一处冲突已同时保留主干静态初始化条目与本轮条目。

首次普通清单枚举与早期编辑重叠，编译失败，保留baseline-attempt01-*，不计测试失败/通过。随后用baseline-overlay.json把cmd/test-feedback精确映射到HEAD原字节并排除新增Go文件，其他976保护文件未变；只枚举得72包626普通根，不执行业务测试。当前新增6个执行器顶层根，最终Full预期全部保留并纳入，不能用本次枚举填Full通过。

早期sandbox重定向Temp的原子替换Access denied及两次实现诊断日志保留，未修改权限或断言；正式同用户原生独立临时环境执行。owner01因新正式ID尚未同步L4列表被原清单测试准确阻断；列表同步后owner02通过，最后按独审补Go布尔解析边界后owner03完整通过，229终态0fail/skip，wall48.5501244s。最后1760个运行相关文件和976个生产/业务测试/资产保护文件冻结；源与定义任何漂移均中止后续队列。

独立adt_review先审设计与实现，确认新增四个Roadmap根和升级排空原根直接归属；修正未知自定义flag的arity及test.*命名空间误报，采用unknown而非猜测，并补原参数根反例。Go -c按本机Go标准strconv.ParseBool处理；Full保持原3条串行core→integration流水线＋other-packages，只修报告worker上限为4，原命令/调度无改动。审核已复核owner03、L1/Web的真实参数与managed SHA，最终批准待完整链终态。

正式L1执行21.2739094s、122终态，通过且0skip；完整Web执行18.2910933s、266终态，通过且0skip/todo。go.deployment四组共24根79终态全部通过（host全量、Windows localservice全量、App9根、升级排空1根），完整入口313.4386472s；全部实际参数count1/25m/p1/parallel1，managed SHA匹配。Roadmap、L2/L3和Full尚待实际终态，不继承历史。

提示词接线治理当前实际正文diff均为空：固定Markdown11文件12066→12066字节，黄金7文件2742→2742且原CRLF/raw字节一致；其余组装4744→4744、测试5786→5786、writer3717→3717、embedding264→264、CI1246→1246（非黄金按Git LF，raw长度另列）。23文件逐项SHA和前后字节存prompt-byte-audit.json。业务原因是新增领域定向反馈与纠正执行参数证据，不改固定规则或更新黄金。本轮重型用例/辅助及全部生产源码保持原字节，矩阵仅梳理。


### 两专项及L2终态与排程修正

go.roadmap三组32根72终态passed/complete、0fail/skip，完整入口272.1846542s，包含owner20根、App11根和host1根；两个live根均未执行。期间观察到外部application回归，故只记功能与实际耗时，不作纯空闲性能对照。L2随后完整通过并cleanup_complete，外层119.3691556s。首次监控只按父子树归属，独立L2代理可能出现在未识别进程中，不以该字段冒称全机零负载；续验监控加入唯一scope路径边界匹配，原监测不覆盖或改写。

原排程的L3调用漏了Plan已规定的-env controlled，在parseOptions即被拒绝，validation-17.log/stages保留，原L3目录未创建、场景未启动，不计功能失败或通过。两次尝试核实纯等待窗口均因状态已转换而安全拒绝，没有停止任何进程。只修外部后续调用，run-continuation.ps1以明确controlled和全新L3-final目录执行该代表场景后继续首次Full，不重跑已过L1/Web/两个专项/L2，不改源码或断言；原排程文件保持原字节作证据。此错误不是自动重试失败测试。


### L3代表路径及剩余Full

L3-final按原controlled的runtime.dual-home-operation真实通过，准备5.6684667s/执行155.3124894s，完整外层162.4945794s，1根1终态、0fail/skip、cleanup_complete；原双home独立请求、另一项目/provider不变和零候选断言保留。实际标签e2e、count1/parallel1与原binary argv对应，Go构建并发为not_applicable，不以控制器20冒充子测试默认值。所有已完成正式项的managed SHA和原始run/terminal已通过只读核验。Full仍在等当前其他Go测试结束，尚未启动，不能登记通过。


## 最终自动验收终态（当前）

首次Full实际完整通过：72包632普通根1652终态、0fail/0skip，14个无普通测试包单列编译项；普通基线626根全部保留，恰新增domain_scope_test.go三个执行器根及execution_parameters_test.go三个参数根。准备13.6819671s、执行1824.4843924s、完整外层1839.4108152s，运行期存在外部集成负载，故不作纯空闲或提速比较。Full原三条core→integration串行流水线＋other-packages未变，workers/group_worker_limit如实为4，Go包/测试参数按每组实际命令核对；test2json的-p包名未被误报为Go并发，冻结标签来自真实构建argv。

final-evidence-verification.json核验全部正式项的原始run/terminal唯一性、完整终态、精确根/包、实际参数与managed SHA；原L3入口参数错误被单独保留并要求确认原目录未创建，不能把它计为通过。1760运行文件与976保护文件、23项正文资产SHA全量一致，业务/重型测试/辅助/提示词/矩阵/CI无本轮修改。既有原断言、超时、次数、轮询和测试分级保留；重型根只有断言矩阵、直接/前置证据区分和C1-C3待等价证明候选，无移层/去重实现。

L1完整入口23.2072311s、执行21.2739094s，122终态；Web完整入口19.4039817s、266终态；部署313.4386472s、24根79终态；Roadmap272.1846542s、32根72终态；L2完整入口119.3691556s、执行106.4845427s，L3-final完整入口162.4945794s、执行155.3124894s，二者均cleanup_complete。上述全部0fail/skip，Web无todo。独立终审针对最终证据进行，批准后提交并提PR，当前尚未合并或部署。


### 最终独立终审与交付

2026-11-03，独立只读审核者adt_review重新读取全部正式原始JSONL、实际argv/build记录、managed SHA及源码/正文资产，确认626个基线根与632个最终根逐项对应、6新增根准确、1652终态唯一且0fail/skip、14编译项如实单列。HEAD overlay的24个原Go文件与HEAD原字节一致、4新增Go文件排除，基线是枚举不冒称测试通过。L3-final的controlled及cleanup、两个专项的精确范围、默认/mixed/未知/不适用与Full实际4worker解释均正确，明确批准当前差异提交提PR，无剩余阻塞；审核未运行测试或修改文件。

重型业务/辅助/全部生产及CI/矩阵/提示词均无本轮diff；原失败和外部负载限制保留，原断言/期限/调度未放宽。ADT-A1至A7均完成。文档667文件4136本地链接0错误、git diff --check及Go格式检查通过。相对实际最新origin/main cca5114297564df176b27145e66fda71d2582682的20文件临时索引预览干净，实际分支/索引未被预览修改。之后仅登记审核状态并交付commit/PR，不合并或部署。
