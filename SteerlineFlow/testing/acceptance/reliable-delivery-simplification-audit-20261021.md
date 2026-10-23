# 可靠性交付简化方案可行性审计

审计对象为[可靠性交付链路的简化建议](../../../docs/architecture/reliable-delivery-chain.md#可以简化的流程)，基线为 SteerLine `9e02854e` 加当前未提交部署恢复修改，Runner 本地源码为 `8261310d`。本次检查源码、故障窗口与既有定向测试；未修改生产实现或原建议，也未使用安装环境。

结论：方向可行，但不能将全部建议作为已经具备实施条件的删减清单。八项中三项可先做，五项需补契约、明确排他边界或缩小首版范围。最应调整的是“省去逐命令 status”不能直接依赖响应后的身份检查；“直接离线证明”尚缺完整原项目集合的来源及证明有效范围。

## 逐项结论

| 原建议 | 结论 | 允许的首版范围与前置 |
| --- | --- | --- |
| 按更新目标组织停止与核验 | 可先实施 | 复用现有退出/兼容/结算函数，统一内部结果与错误表达；reset 只消费自己的严格证明，不因复用协调器而自动启动或停止现场 |
| 收窄逐命令 service status | 有条件可行，调整实施顺序 | 先落实实际受理前的完整身份及 ready 围栏，再取消预查询；当前 expected-instance-id 不能单独替代现有全部检查 |
| Go 与 PowerShell 统一语义所有者 | 可先实施 | Go 统一领域判定，脚本保留退出码、staged 包绑定、路径/实际进程/字节核验和证明落盘；不删除替换边界检查 |
| 清理历史覆盖文档 | 可先实施 | 依据当前源码和较新已实施契约收口长期规则，保留独有历史验收，保留尚未验收能力的范围说明 |
| 直接离线证明替代原版启动停服 | 有条件可行，当前不宜直接替换 | 补齐原宿主完整 census 的可核对来源，定义锁和证明时效；新 worker 前的原停止义务受理继续保留 |
| 合并产品控制文件与 SQLite 命令记录 | 有条件可行 | 首版只改普通命令传输和结果查询；已生效回执补查与旧输入重放必须分开，暂不改变领域事务和候选 outbox |
| 合并观察快照、事件与不可变校验 | 分开实施，后置 | 同边界投影/事件需要 Runner 公开契约；散列缓存暂不随之引入，继续保留调用与安装边界的实际字节校验 |
| 在已有包中记录安装阶段 | 有条件可行，后置 | 首次替换前固定完整目标配置、所有 staged 摘要和操作来源；阶段标记只是恢复线索，实际磁盘才决定哪些步骤可续做 |

“可先实施”是源码可行性结论，不代表对应改造已经完成验收。其余五项并非方向被否定，而是现稿不足以直接指导安全删除旧路径。

## 需要补齐的关键问题

### P1 省略预查询的替代校验必须发生在副作用前

原建议位于[实例围栏与预查询](../../../docs/architecture/reliable-delivery-chain.md#用实际命令的实例围栏减少预查询)。当前 [ValidateInstance](../../../internal/runnerclient/binding.go) 同时检查 ready、instance、EXE SHA 和 resource digest；[Client.call](../../../internal/runnerclient/client.go) 传给实际命令的是 expected-instance-id，普通响应封套不包含统一的目标构建/资源身份。

Runner `internal/host/server.go` 的 dispatch 入口核对协议、连接目标实例/token/目录及 expected instance。它没有接收调用方期望的 EXE/resource 参数；业务命令直接转给 engine，不能将 service status 的 ready 判定和完整绑定判定视为已经等价地搬到该入口。

如果仅删除预查询，原来拒绝的同实例状态/构建/资源不符路径缺少替代判定；如果只在 submit/reply/resume 的响应中补身份，可能先受理动作再发现错配，已经来不及阻止副作用。因此，这项不宜作为先删代码、再补测试的轻量优化。

最小修正有两条可选路径：在实际命令的服务受理前核对完整期望绑定和状态；或者先明确并验证服务实例与完整构建/资源身份不可变、连接目标可核验且无隐式启动的契约，并据此给出等价性证明。若选用响应元数据，它只能补充结果核对，不能单独承担写操作的前置许可。所选 EXE 的实际摘要核验仍保留。

通过条件包括：替代实例在真正发送时出现、stopping 状态、错误 resource/构建、EXE 被替换及环境检查隐式启动反例，均在新副作用前拒绝；原请求正文/ID 不因拒绝被重新生成。现有替代实例测试只覆盖其中一部分。

### P1 直接离线证明尚缺原宿主完整项目集合的权威来源

原建议要求核对冻结配置和完整项目集合。但 [loadApplicationCatalog](../../../internal/app/application_catalog.go) 重新读取当前 execution.json 与 etc；[Identity](../../../internal/product/applicationcontract/contracts.go) 只保存宿主身份，完整项目关闭集合目前在正常 shutdown 中发布；[ServeApplication](../../../internal/app/application_service.go) 没有在启动记录中持久化一份绑定原宿主的完整 census。

目标场景恰好是当前宿主 shutdown 缺失或陈旧。现有[中断恢复准入](../../../internal/app/deployment_recovery.go) 能用仍存在的项目 checkpoint 和额外数据库扫描发现一部分配置删除，却不能将“当前能找到的项目”直接等同于“原宿主全部项目”：若某项目配置和其 caller 数据库均已不在，单靠当前目录和剩余 checkpoint 无法证明该项目原本不存在，也无法还原它的待停止义务。

因此不能把复用当前准入直接写成“原集合未变已经证明”。这项证据来源缺口也值得单独检查当前中断恢复；重新运行原版同样不能重建已丢失的 caller 事实。本次没有通过删除现场来复现，结论来自身份结构与读取范围的源码核对。

若坚持完整原集合是放行门槛，首版需在原宿主开始接受业务前，将必要 census 与宿主身份、配置摘要绑定持久化，可扩展既有生命周期记录，不必另建证明服务。已有公开证据足以恢复原集合的情况也可使用，但必须说明完整性来源。找不到原清单或停止义务时继续阻断，不以空目录、兼容报告或当前配置补造过去的成员集合。

通过条件至少包括：仅删配置、配置和 caller 数据库同时缺失、项目启动未完成、空 home、额外未纳管状态、原取消义务未决；各自必须得到有依据的放行或阻断，不能把“没有读到”当作“原本没有”。

### P1 离线 JSON 证明不是跨步骤持有的 writer lease

同一项离线建议写了“全程持有应用启动协调锁”和“writer lease 保护下取证”。这两句话还不足以确定替换与下一次启动的排他边界。

Runner 的 `internal/host/client.go:Status` 和 `internal/host/takeover.go:VerifyTakeover` 只在各自调用内取得 writer lock，返回后便释放；[应用启动协调锁](../../../internal/localservice/coordination.go)是另一把锁，直接 Runner 启动不取得它。项目核验中的仓库/run-dir 锁也在 [verifyProjectOfflineMode](../../../internal/app/application_verify_stop.go) 返回时释放。[部署脚本](../../../scripts/build-deploy.ps1)随后分步骤安装文件，不能把此前 JSON 结果视为一份仍持有排他权的租约。

如果在核验返回后、替换或新 worker 启动前出现新写者，原报告中的 revision、完整集合和停止义务可能已经变化。现稿“替换边界重核”需要明确核验与哪一个动作原子衔接，不能只多查询一次后宣称彻底关闭窗口。

首版需要明确：哪些入口参与同一协调；在最终替换边界如何处理新写者；新 Runner 实际取得 writer lease 后如何重新读取当前记录、复核候选并受理原停止义务；拒绝时已经安装哪些字节、允许哪些后续动作。若要求取证到替换期间绝无 writer，则需要相应排他协议或同锁内的原子动作，不能借用应用协调锁的名称替代。若只保证启动前重新准入，就应把证明范围限定为该时点，而不是保证报告永久有效。

通过条件是主动在 status 返回后、takeover 返回后、项目锁释放后及最终启动前插入另一个 writer；旧证明不能授权错误写入或丢失停止责任。正常退出路径和旧失败记录仍不能因此放宽。

### P2 本地命令合并必须区分补查旧效果和重放旧输入

[Controls.Handle](../../../internal/taskrunner/control.go) 会先返回已经保存的原结果；发现原候选或 Action 已保存但外层回执缺失时，即使实例、revision 已变，也可以补回 applied。与此同时，[Bridge.RecoverInputs](../../../internal/runnerbridge/bridge.go) 会取消 pending 输入，将 unknown 输入标记 observe-only，不在重启后补发。

把文件收件箱合并进 SQLite 是可行的传输收口，但不能变成“重启扫描全部 pending 后重新 apply”的普通持久队列，也不能变成“旧实例命令一律拒绝，连已经成立的原结果也不返回”。这两种简化分别会引入重复输入或丢失已经提交的效果。

最小改法是保留现有按原请求查结果及恢复已保存效果的顺序，首版只替换 inbox/receipt 的承载位置；先不把多个领域事务硬合成一个通用事务接口。宿主停止与初始化入口继续独立，避免 actor 关闭后无法查询或停服。

通过条件须分别覆盖未受理旧命令、已预留候选但外层回执缺失、Action 已存但产品回执缺失、仅 Runner 已受理而本地绑定未知，以及同 ID 换正文。两项旧回执补查/旧输入零重发的现有定向测试本次均通过，应保留为后续改造基线。

### P2 仅有安装阶段和 EXE 摘要不足以确定续装目标

当前 [build-deploy.ps1](../../../scripts/build-deploy.ps1) 在内存中读取并调整 execution.json，安装三个程序后才写该配置和最终 deployment.json。既有包在首个替换前没有一份已经固定的完整目标配置及最终 manifest。

若只补一个“安装到第几步”的标记，电脑中断后读取当前 execution.json 继续做，会把原包目标与后来配置修改混在一起；崩溃还可能发生在某个文件已经替换、阶段标记尚未更新的窗口。

首版应在首次替换前，将完整目标配置字节、全部 staged 文件摘要、来源和原替换依据固定进既有包；恢复按实际磁盘匹配旧/目标字节决定可续步骤，阶段只作线索。身份、配置或候选状态发生变化时重新核验，旧证明不因与包放在一起就永久有效。完成文件和配置校验后才发布最终 manifest；不附带自动回滚或旧数据迁移。

通过条件包括每个 EXE 替换前后、配置写入前后和最终 manifest 发布前后中断，以及中断后另改配置；恢复不能默默采纳另一份目标，也不能把混合版本报告完成。

## 其余可行性判断

按目标组织停止与 Go/PowerShell 所有权收口可以作为同一小范围改造：事实收集仍调用原函数，统一结果同时保留服务退出和资源结算，脚本不再复制全部领域判定。不得将 reset 的只读安全检查改为自动恢复/停止动作，也不能把原先匹配当前实例的失败回执当成一般“可恢复”。

投影与事件合并需要 Runner 给出同一读边界上的 Run revision、事件范围和 next cursor；否则删掉末次 inspect 会使事件超出已观察投影的窗口失去检查。可保留纯读取优化为后续独立工作。EXE 散列缓存与该接口没有必然依赖，首版不合并推进，避免为减少一次读而引入更多跨调用缓存状态。

文档收口的行为风险最低，但应同步删除被替代的相反要求，而非继续增加覆盖段落。它不需要改 Task/Run 记录、兼容历史 schema 或扩大实际验收范围。

## 建议实施顺序

1. 先做按目标组织停止、Go/PowerShell 领域判定收口和现行文档整理，沿既有锁及证明能力交付。
2. 独立确定实例/构建/resource/ready 的实际受理前契约，再减少 status 预查询；性能改进不能先于等价校验。
3. 普通产品命令传输合并独立实施，保留旧效果补查和重启零补发，不与 Runner 协议改造捆绑。
4. 完成原 census 来源、证明有效范围及新 writer 并发设计后，再替换原版启动停服的恢复路径。
5. 观察批量化和安装续装按独立收益后置；暂不引入长期散列缓存、通用恢复服务或新全局证明版本。

## 本次验证

原始日志位于仓库 `.evidence/reliability-simplification-audit-20261021/`。

| 验证对象 | 命令 | 结果 |
| --- | --- | --- |
| 真正发送时实例变化、原 stop 与资源结算 | `go test ./internal/runnerclient -run '^(TestRunnerPresetFreeze\|TestServiceSettlementRequiresOriginalFinalProof)$' -count=1 -v -timeout=60s` | 通过，2.200s，`runnerclient.log` |
| 显示版本及跨实例补查原控制结果 | `go test ./internal/taskrunner -run '^(TestSharedRunnerControlRecoversMissingOuterReceipt\|TestRunnerStartRequiresDisplayedTaskAndGraphVersions)$' -count=1 -v -timeout=60s` | 通过，1.296s，`task-control.log` |
| 重启输入撤销、未知零补发、迟到绑定及持久启动 stop | `go test ./internal/runnerbridge -run '^(TestSteerRunnerOutboxBoundaries\|TestSteerRunnerRestartOriginalBinding\|TestPrepareStartupStop)$' -count=1 -v -timeout=60s` | 通过，1.273s，`bridge.log` |

这些结果确认的是当前实现的必要边界，不是未实现简化方案的通过证明。没有运行全量、真实部署、OS 重启或模型执行；新增的等价性、census 缺失、新 writer 插入和部分安装用例仍是各项实施的准入要求。
