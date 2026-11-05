# Plan：测试早期反馈与专项范围收敛

日期：2026-11-03。状态：已实施，自动验收与独立审核通过。依据 [Spec](../spec/test-early-feedback.spec.md)，用户现已要求当前聊天直接实施、不放宽断言、独立审核通过后提 PR；不启动 roadmap。测试选择/真实结果和审核写入[专属验收](../testing/acceptance/test-early-feedback.md)，输出遵循[路径清单](../paths.md)。

## 实施顺序

1. **冻结基线与内容清单**。实施开始时确认实际 commit/dirty、工具链和依赖，导出普通 Go 包/顶层根清单；逐项核对 Spec 八个environment文件24声明根及额外 PM一根/Developer三根/shared一根。对原 L1 做三次独立输出计时，冻结旧 go.environment 完整包集供最终同源码配对计时；不提前重复执行长集，历史1321秒等数据只作背景。保存原 definition 摘要，不修改历史报告。
2. **L1 前移仓库检查**。在原 boundary owner 分组追加既有 TestThreeLayerRepositoryBoundary，保持原守卫/黄金/前端选择。补实际选择与失败传播测试，独立导出源码加入 model→executionconfig 非活动依赖，用正式 L1 命令验证失败；原源码不得残留探针。只复用进程内扫描缓存，不改矩阵、标签或 Check。
3. **HTTP owner 完整归组**。先给 nativequery 补真实子进程行为测试，复用现有测试角色方式，不引入真实 Runner。然后将 go.http 固定为 Spec 七包，更新对应非零/精确包检查、正式目录和包装器说明；保留原 HTTP、快照和 discovery 用例。运行完整专项确认无编译项冒充测试。
4. **环境选择按组精确执行**。localSuite 以最小分组结构表达原 owner 全量及 App 固定根，复用现有 runGroup/报告汇总，其他专项保持原选择。定义列出八个environment文件、29声明根及Windows29/其他28的明确活动选择和附加根来源；仅选择守卫解析源文件核对漂移，正式运行使用冻结的明确根名。分别校验每组包/根/子项及聚合状态，不能让全局 -run 过滤 owner 包，也不能用另一包同名根补齐缺项。
5. **同步可读与机器定义**。更新 testing.md、l1.md/l4.md、test-list.json、local/scoped 清单校验及 test-web.ps1 的范围说明；仅使用已支持 JSON 字段。检查 E2E 登记未误变、正式 ID/摘要与实现一致。L2/L3场景和并行策略、Full普通范围、提示词正文及黄金文件不改。
6. **完整验证与独立审核**。执行下表，对照基线清单及性能；审核实际差异、提示词正文/字节和 L1/CI 接线。修正确认问题后只复验受影响项；实现稳定后最终 Full 一次完整执行，不在运行中改实现。失败保留原始报告，修正后另开目录，绝不自动重试掩盖失败。按用户现已给出的授权，审核与验收通过后提交、推送并新建 PR，不合并。

## 选择与完整性验证

在 cmd/test-feedback owner 中补有实际意义的反例：环境 owner 包被误套 App 筛选、同名根出现在错误包、App 缺根/多根/重复根、子测试开始后无终态、真实 skip、空匹配、一组失败但另一组通过、原始报告写失败、正式定义/源文件新增或重命名漂移。原 summary/退出码/managed/完成值都必须保留失败，不能只断言解析函数返回 false。

不通过生产依赖矩阵例外、临时标签或跳过更新黄金样本解决失败。nativequery 测试保留原行为上界，不延长生产 deadline；跨进程角色不产生空通过测试。owner/专项命令中的每包必须非零，Full无测试包仍采用既定编译项口径。

## 验证安排

所有 `<out>` 均为仓库外全新绝对目录，命令在仓库根运行；PowerShell 中 scenario 使用空格参数。入口默认环境按现有 isolated/controlled 定义，不借用用户安装现场。七包HTTP含固定30秒窗口的Windows跨进程压力，按独立压力观测安排，不与本机E2E/Full或其他压力任务并行；本轮并行负载下失败原始证据保留，复验不改期限/断言。L3自身原并行策略不变。

| 验收 | 测试 ID / 范围 | 命令、时点及通过条件 |
| --- | --- | --- |
| A1 | L1 选择及原始违规 | 步骤2后，正常源码 `go run ./cmd/test-feedback -level=L1 -out <out>` 通过；独立源码副本同命令在加入真实 model→executionconfig 依赖后失败且精确定位；选择缺失/skip反例由执行器测试覆盖。 |
| A2 | nativequery owner | 步骤3先 `go run ./cmd/test-feedback -out <out> -- ./web/server/nativequery -count=1`；实际程序解析/cwd/取消及子进程结算/输出边界全部执行，不需要原生工具登录。 |
| A2 | go.http | `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.http -out <out>`；正式七包全部非零、0skip、passed/complete、managed一致。另运行 `scripts/test-web.ps1 -EvidenceDir <out>` 验证包装器实际委托及构建/HTTP/Web报告。共享Web构建串行。 |
| A3 | 执行器完整 owner | 步骤4/5后 `go run ./cmd/test-feedback -out <out> -- ./cmd/test-feedback -count=1`；两组选择、清单、漂移、所有失败传播反例与现有回归通过。 |
| A3 | go.environment | `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.environment -out <out>`；四个owner包全量及App活动根（Windows29/其他28）/子项全部终态完整，0skip；逐项与冻结定义核对。 |
| A4 | 原矩阵与完整 Full | `go run ./cmd/test-feedback -out <out> -- ./internal/testkit/boundary -count=1`；矩阵与源扫描不变。最终 `go run ./cmd/test-feedback -level=L4 -suite=go -out <out>` 全部通过、0测试skip，旧普通根无缺失、新nativequery测试纳入，编译项独立列举。 |
| A5 | 最终固定 L1/L2 | 实现及审核修正结束后正式 L1；L2 通过独立同用户 `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-l2.ps1 -OutDir <out>`；passed、0skip、L2 cleanup_complete。 |
| A5 | Web/受影响 L3 | `go run ./cmd/test-feedback -level=L4 -suite=web -scenario web.unit -out <out>`；独立包装器分别选择 `-Level L3 -Suite testing -Scenario testing.project-environment-readiness` 与 `-Suite runtime -Scenario runtime.shared-runner-projects`，当前scope04两个ID均须按新源码重新验证、各自passed/0skip/cleanup_complete。历史scope01原组合曾在额外负载下共享超时、环境通过，随后只补共享并通过；原组合仍failed，不作为当前通过。不改场景/并发实现及期限，不测试真实模型。 |
| A5/A7 | 编译/链接/正文/审核 | 全部 `go test -tags=e2e ./tests/e2e/... -run '^$'` 只计编译；`python scripts/check_doc_links.py`、`git diff --check`；逐字节核对固定Markdown/黄金/writer（保留用户CRLF），给出前后字节数与实际正文diff，独立审核L1选择、CI实际差异或零差异及报告可信性。 |
| A6 | 性能对照 | L1原/新各3次，条件一致、输出全新，记录总耗时/中位/最大及冷缓存事实；go.environment按下一节同源码成对比较。目标未达不宣称性能验收完成，不通过删断言、条件skip或放宽timeout达标。 |

## 环境专项性能对照

基线旧定义是 environmentadaptation、model、remotetest 和完整 app。步骤1冻结其包/根范围；最终在同一实现源码上另用诊断命令执行相同旧四包范围一次，与正式新 go.environment 一次成对比较，顺序如实记录，保持相同Go/Git/Runner fixture配置。计时不与其他回归/Full并行，准备相同构建缓存条件，不清缓存只测一侧；L1对照同样遵守。最终同源码诊断不是正式go.environment，不生成或冒称旧定义managed通过。

目标是新专项执行耗时较同源码旧四包至少减少40%；同时新增中性executionconfig包，减少的只能是无关App根。不得只用原1321秒Full或另一次冷编译作分母。无需为目标反复运行长集取最好值；当前只有一对长运行时以该实测比例登记，数据不足不写“中位数改善”。若需要声称中位数，则补足原/新各三次配对样本，否则只接受为一次样本目标，后续持续观察。Full本轮只要求完整性，不承诺加速。

## 规划阶段文档检查（历史）

此前文档轮仅新增 Spec/Plan/验收规划与导航，运行链接、文档范围和 diff 检查，没有执行功能测试。用户现已授权实施，按上表实际验证，不以规划轮或其他历史测试填充本次通过。

## scope02：同步fd844789（历史）

首轮Full及同源码性能全部完成后，main更新至fd844789（PR #426/#428）。暂停代码修改，保存首轮scope01与源身份，再在干净main完成原L1三次和普通根冻结（588根），恢复本轮实现并将readiness新两根明确加入；当前七文件22根加原PM根共23。重新核对源选择、负例和文档/JSON，自主修改范围仍不含主干业务。scope02重新验证23根、受影响L1/L2/HTTP/Web/L3、同源码性能配对与最终Full；原scope01的585根/21根/74.283%仅为旧源码历史，不替代当前版本。


### scope02：审核夹具补正对配对的限定（历史）

旧四包reference完整通过后，因nativequery新Start负例使用不存在路径，先用非空输出目录门让编排拒绝后续测试并停止，再补真实相对home、复制的有效测试二进制与绝对路径成功控制；原负例保留。门目录仅是编排暂停记录，不算测试失败/通过，Full尚未开始。focused正式运行另用全新environment-focused-final。

两次全树hash因此不同，唯一差异为web/server/nativequery/process_test.go，前后hash及冻结清单保存在scope02。该_test.go不被旧四包或新环境组的测试构建编译；环境所选生产/测试、依赖生产源码、执行器、定义、断言和23根均完全相同。独立只读审核确认同一受测实现及相同运行条件的比较仍有效，保留reference计时，不写成全树完全相同。性能仍无其他回归并行、不清缓存或挑最快样本、40%目标不变。补正后再次冻结，随后执行新环境专项与最终Full；最后独立复验HTTP包装器及L1三次。


## scope03：同步82935861与最终验证

main继续合入#429/#431，直接影响环境文件、Git读取、HTTP/Web与E2E辅助。主动取消旧fd尚未终态Full，仅保留部分证据；只停止经脚本路径/创建时间验证的本轮进程树并核对无捕获进程残留，不把取消当断言失败或通过。安全保存实现后快进82935861，在干净主干完成原L1三次与71包591普通根清单，再恢复实现；两份README/路径文档保留全部主干新内容。环境集合新增资格不依赖项目文档测试一根，共24根，原23根全保留，JSON/指南同步。

本轮全部受影响验证在scope03另开目录：先新L1三次、独立副本真实违规、执行器/boundary与标签编译；完整Web/HTTP包装器独立运行；L2及两个L3均按新源码重新验证且无压力并行；最后在冻结实现上串行执行旧四包参考、新环境专项与最终Full。此次两侧完整运行相关源码/定义hash保持一致，不沿用scope02的68.739%或未完成Full。当前阶段禁止为了追随无关main推进而反复改基线，提PR前只评估真实冲突及直接选择漂移；需要集成时才按影响补验。


## scope03：共享Runner累计超时的最小修正

原shared/L3 failed、0skip、cleanup完整，root309.53s，业务精确300s；b候选仅剩约23.15s。两份资格/材料ready和a候选applied均完整。reference/新环境/Full未开始，流水线已停止。保留原始报告、命令和场景文件，不直接重跑。

初稿拟改SubmitControlInbox并显式传目标；独审发现第三个configuration调用点及该路径省去本地CanControl/配置门禁，因此撤回、未实现。最终S4只删除MainCandidate重复身份解析，保留BuildHostConfig内解析和原Targeted完整门禁、payload、原回执等待；不改三个调用方/场景。driver命令证据添加真实起止与wall，在原真实transport测试追加证据断言，不新增普通顶层根。

先独审该收窄Spec/Plan，再实施、普通driver完整owner与E2E编译、最终正式L1及受影响原场景验证（新输出目录）。运行无并行压力的环境配对与最终Full前再冻结源。原Web/HTTP/环境owner源码未变，旧通过只按影响保留，不能凑shared通过。所有原期限、任务/材料/资格、候选身份/PR与重启断言不变。

09:36只读进程快照确认本机另有两组非本轮Go/场景任务（详见post-failure-machine-workloads.json）。只能声称本聊天串行，不能宣称整机无其他验收；该快照不能反证09:08失败时的全部负载或唯一根因。等待独占窗口期间完成只读核对及轻量owner验证，不终止其他任务。正式长集记录实际条件，不清缓存、不自动重试、不加超时，不拿旧源码/旧负载样本冒充当前目标达成。


### 外部新负载下的性能补验安排

用户选定等待其他测试结束，09:48窗口曾连续确认无Go任务；但10:19:51其他任务重新启动完整go.environment，首次当前配对的新侧在并行负载下完成。功能通过及62.871%观测保留，严格A6条件未达，不将数字冒称受控目标完成。Full已启动，继续按原断言获取实际终态，不停止他人测试；之后再等待窗口，在全新reference-final/focused-final目录做一次原/新配对。源代码保持冻结、同缓存策略、参数和40%目标不变，不挑最快结果。此时Full已在最终实现稳定后执行，后续仅补性能排程且代码不变，不额外重复Full。


### Full SQLite首写超时后的原条件诊断

本次Full实际失败，仅一项roadmap首写在原2s ctx耗尽（父/子2终态），全部普通根仍保留，不能算A4通过。独审未发现确定锁泄漏/逻辑错；不改SQL持久性或原期限。停止过时的“必须Full先通过”等待队列，改为空闲60秒后：原SQLite root单次（两子例/count1/2s）诊断，通过才启动新目录严格性能pair，之后Full-final整套。任何失败即停止，不自动重试；若安静仍复现，先追加证据与最小修正SpecPlan独审再改代码。源码无更改时不重复无关HTTP/Web/L1/L2/L3，本轮仍要求最终Full完整实际通过。

## scope04：实际冲突后的最终验证

快进main 02285e1c并解决四处文档冲突，原主干Developer边界保留。独审确认三项Developer App根与环境准入直接相关，附加集合由PM一根扩为PM一根加Developer三根，共27，原24全部保留；SQLite policy根仍在原存储专项/Full。源码定义、文档、JSON同步后复核。

在scope04新目录等待整机Go任务连续60秒为空，安全保存实现以干净02285e1c冻结普通包/根及原L1三次，恢复同一实现后规范行尾并冻结源码/定义。新L1三次、执行器/边界owner、独立副本违规、标签编译、Web/HTTP、L2与两个L3按受影响新源码重验，各阶段串行且长集开始前重新等待空闲窗口。最后冻结实现串行运行旧四包参考、新27根专项及最终Full，保存运行期间整机负载观察；严格性能条件不满足则如实登记，不以旧数据替代。失败即停并保留原证据，不自动重试或调整原期限/断言。scope03旧Full及SQLite单次诊断全部保留，不能用于新版本A4。

### scope04计时证据时钟一致性

首次owner中原DTO断言及执行器/boundary通过，新增计时等式失败：真实UTC起止差0.1888422s，monotonic Sub为0.1888574s，差15.2微秒。保留owners原报告；独审批准仅将两次捕获改为time.Now().UTC()，wall_seconds与序列化时间戳统一真实墙钟，不取整/钳制，不改变1微秒一致性、正值、先后、DTO或任何期限断言。重新冻结源码，在新目录复验owner及新L1，再继续既定长集。

### scope04正式长集等待恢复

首轮environment-reference-final已完整通过：158根345终态、0skip，wall1204.1647449s。12:24:49见一个短go-run，事后父子均退出，不能确认其为外部回归或本轮独立短路径现场，不认定归属或唯一影响；后续等待又在12:26/12:28观察到go-test短任务。原60秒门无法避开重新启动的负载，严格性能条件未闭合，首轮仅记功能通过和不确定耗时，不纳入新比例。

独审通过后仅暂停经PID/脚本路径及无Go后代、无focused/Full目录验证的本轮纯等待PS，未停止任何测试或他人。排程加强为连续5分钟（16次、间隔20秒）无Go及Node构建/测试后，在预先指定的新目录environment-reference-clean→environment-focused-clean→Full-final串行运行；监测结合唯一scope、reference私有短目录与进程PID/创建时间，分类公开测试级别/ID，不保留原始命令行。1056源码与定义冻结不变，全部原断言/范围/timeout不变；L1/L2/Web/L3不重复。若新链再次出现实际外部压力，保留结果并如实标性能条件，不自动循环重试或择优。

## scope05：共享环境材料的必要集成

当前baseline为e5cd4f2a，共享材料直接改变环境实现/夹具并新增environment_script_windows_test.go。旧守卫合并后必报未登记，故按原规则必须集成；不是追随无关main。原27+脚本1+shared1，Windows29/其他28；全部声明源码跨平台检查，只有执行选择按固定GOOS属性过滤Windows根。元数据/源码漂移、Windows/其他活动列表及错误平台额外/缺失反例先独审再验证。main的共享manifest5、双方paths、live note、固定正文与角色接线完整保留。

scope04最终Full/27专项及所有短集均通过，72包610原根零缺失、618实际根1550终态，但只作旧基线证据；未执行拟定的idle-final pair。scope05在新的仓库外目录冻结干净e5普通根与原L1三次，恢复实现后新L1三次/owner/真实违规副本/标签编译、完整WebHTTP、L2和原两个L3均按新源码重验；最后连续5分钟无Go/Node构建测试后串行运行旧四包参考、新正式专项及Full，期间源码与定义hash冻结，原期限/断言不变。已确认其他活跃聊天整轮completed，不向其发送消息。所有调用用新目录，任何失败保留并停，不自动重试；后续main仅在真实冲突或直接选择漂移时集成，不追随无关Roadmap推进。

### scope05仅补无效新侧的预先指定恢复

独立审核明确：原Spec/Plan未要求两侧紧邻、未禁止中间运行同代码Full；有效reference-final为唯一旧侧，57采样0其他。首个新侧因预先定义的外部负载条件受限，不能按耗时筛选。Full完整实际通过并核对1903源码/定义、Go版本/GOOS/GOARCH/GOCACHE/GOFLAGS及fixture参数不变后，确认外部进程结束并连续5分钟无Go/Node测试，仅执行一次focused-idle-final；与原有效旧侧配对。公开中间Full的起止、间隔与可能OS缓存差异，持续as-found、不单侧清缓存或强制预热，count1确保不复用测试结果。原393.6720124s/66.331%样本完整保留不计受控比例；新侧不能因更慢而放弃，若再干扰/失败保留并停，不循环。40%目标、全部原断言和25m/p1/parallel1不变，不重跑已过Full与短集。用户未答可选协调问题不授权发消息，原等待条件实际解除后可继续既有验收。

### scope05 Full：PID夹具发布窗口的最小修正

原Full在TestWindowsOwnedWorkerRunsInJobAndStopsWithOwner失败，platform_windows_test.go:110严格strconv.Atoi收到空字符串，root0.22s。helpers_test.go中spawn-owned-child在child.Start成功后os.WriteFile直接创建目标，再写正PID；父循环只要ReadFile成功即严格解析，形成可见空文件窗口。原失败保留，整套继续取终态后再实施。

仅拟将测试helper的PID发布改为同包现有AtomicWrite；不改生产实现、child.Start顺序、父ReadFile/Atoi/正PID、真实身份、Job成员和Close后树消失任何原断言，不改两段3s/20ms或加条件skip/重试。方案先独审；修正后完整host owner及该原根固定次数压力诊断，再新目录完整Full，不能用局部通过覆盖原失败。原有效旧参考/环境源码不包含host内部_test.go，其复用须核对依赖与源码差异后独审，不预先宣称同一全树。

### scope05 PID发布修正实施与保留参考

独审批准后仅helpers_test.go的PID发布一行改AtomicWrite，父测试完全不动。预先固定原根20轮全部通过（pid-publication-20），完整host owner通过（host-pid-owner）。冻结前后1903文件仅该内部_test.go不同，实际SHA及差异清单存pid-fix-source-diff.json；四包参考及环境组不编译host内部测试，生产源码/定义/选中测试不变，独审允许保留有效旧参考，不能写全树完全相同。新冻结后L1-pid三次与Full-pid-final为新输出，不覆盖Full-final原失败；其他未受影响短集不重复。随后已批准的一次focused-idle-final恢复仍以新Full真实通过、工具链/缓存条件不变及5分钟无外部回归为前提。


## 最终实施与验收归档

本轮实现及TEF-A1至A6自动项已完成，实际命令、原失败与补验、范围/源码身份、提示词字节及受控单次性能结果统一见[验收记录](../testing/acceptance/test-early-feedback.md)。最终Full为72包620根1572终态0fail/skip，原612根全保留；最终L1外层中位14.437秒，环境专项一次对照减少67.198%。不放宽原断言、期限或Full范围，独立审核通过后提PR。
