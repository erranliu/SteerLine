# PR399 冲突解决与合入审计

2026-10-27，用户要求解决 [PR399](https://github.com/erranliu/SteerLine/pull/399) 冲突并审计是否适合合入。审查针对进度修复增量，不将新主分支的 roadmap、部署与远端环境功能重新算作本 PR 新增功能。

## 结论

冲突已解决，代码审查未发现阻塞合入的缺陷；建议保持草稿，暂缓立即合入，等待原约定的连续两次真实进度链验收。组件和受控联验通过不替代该验收，也不证明主线新环境适配/result-file 代际已完成真实远端联验。

受控进度场景首轮在维护接口读取时超时，未进入进度断言；独立 main 对照及 PR 独立复核通过，尚未稳定复现，也未证明原因已消除。保留这项验证风险，不能称作已修复的主分支缺陷或覆盖原失败记录。没有自动合并、部署或更换在途轮次二进制。

## 冲突与代码范围

- 合入目标 `main` 为 `02a8e1e77ff4e14840e81943180e0590b33b28f9`，本地冲突解决提交/实际受测来源为 `ed03cc6a8076edde14f4927e309e8bc36796481f`；后续仅审计文档提交另记，不冒充在文档 head 重跑代码测试。
- 唯一冲突是 `internal/app/http_consumer_boundary_test.go`。采用 main 的 Web 宿主限定例外，未采用 PR 原先面向所有消费者的较宽例外；该文件与 main 无差异。其余依赖闭包仍逐包检查，未允许数据库或执行所有者进入 HTTP 消费端。
- 审查实施/审计执行与发布/报告的提示组装、用户 override 保留、同源 PowerShell 写入器嵌入、必跑测试与 Plan 定向测试的生成优先级。调用方仍验证 Runner 公开配置 schema，冻结安排、严格布尔字段解析和原 Run/输入身份未放宽；无业务测试 ID 硬编码、旧版读取或回退。
- 写入器实际接入项目登记入口，由原结果、输入与清理核验决定布尔值。发布/报告收到只读提示；这属于步骤指令约束，不是新增 OS 文件权限隔离。错误字段仍由原严格解析拒绝，不能仅凭提示或模型回复认定真实链成功。
- `projects_test.go` 维护既有 restart 启动器断言，与现有实现一致；没有在本 PR 新增重启功能或变更安装流程。

## 验证

证据根 `C:/SteerlineDebug/wt-507e-pr399-review/`，各次原报告及失败均保持独立。

| 范围 | 实际结果 | 原证据 |
| --- | --- | --- |
| 固定 L1 | 通过，干净受测来源 ed03cc6a | `L1-20261027/summary.json` |
| 固定 L2 | 通过，cleanup_complete=true；受控 worker/provider，无模型请求 | `L2-20261027/summary.json` |
| 直接受影响 app | HTTP 边界、restart 启动器、固定测试快照三项全部通过，无 skip | `app-affected-20261027/summary.json` |
| 三个受影响 owner 包 | runnerclient、testmanagement、testmanagement/model 均完成无失败；显式原生 opt-in 未选入，不算原生验证通过 | `owners-20261027/summary.json` 及原 JSONL |
| 较广 app 运行 | 总默认10分钟预算耗尽时处于新主分支 roadmap 实际 Git 用例，整体 incomplete；不是全 app 通过 | `owners-20261027/summary.json` |
| 进度 L3 首轮 | 维护接口 GET 超时，失败，清理通过；未进入进度断言 | `progress-L3-20261027/summary.json` |
| 未修改 main 对照 | 同一场景通过，来源02a8e1e7，清理通过 | `main-progress-L3-20261027/summary.json` |
| PR 独立 L3 复核 | 同一来源 ed03cc6a，通过/重跑清零/非法内容保留/断连恢复/重启身份与零额外输入/closed保留/清理全部通过 | `progress-L3-isolated-20261027/summary.json` |

第一次 L3 命令被 PowerShell 参数解析拒绝，发生于测试执行前；改用显式 argv 数组后才执行首轮。拒绝记录为 `L3-command-rejection.json`，不作为产品失败或通过。

原生 opt-in 未执行项：

- `TestEnvironmentCheckPublishedContract`
- `TestProjectEnvironmentExecutionPublishedResultRoundTrip`
- `TestRunnerFullBindingPublicCLI`

`audit-result.json` 保存上述报告 SHA 与审计来源。未运行业务项目全量测试；业务断言及夹具修复由用户合并的 PR428 交付，不以此审计替代其完整业务/人工验收。

## 原进度复验及最新暂停决定

PR428 已确认合入 AIRefactor `485d88bad9081d3fd8d892116569f763038f7b7e`。用户随后恢复原进度复验，新 `post428-01` 已接纳实际10项安排；历史 qualified-01/02 不重放、不重新计数。新轮仍使用已批准的原 Caller8205/Runner7c/SDK 子环境，和本次受测 main 集成 ed03cc6a 分开记录；它不是当前 main 新环境代际的替代证明。

截至本次审计没有连续2/2通过结论。原报告、进度/API/UI、真实发布与原 Run/宿主/原生资源清理仍须逐轮独立核实。详细历史见[原进度修复复验](test-progress-0of6-20261026.md)、[Spec](../../spec/runtime-test-progress.spec.md)和[Plan](../../plans/runtime-test-progress.plan.md)。

2026-10-27 15:44，按用户新的职责划分暂停依赖输入链路的业务复验，等待 SteerRunner 对话完成 PR66 独立验收并合入。本会话不继续 Runner 输入逻辑开发，也不以 SteerLine 代码、补 Enter、重发或较宽确认代替 Runner。后续包必须在用户告知合入后核实实际来源与资格，通过正式入口新建；旧现场不热换。

本次只读复核 GitHub：PR399 仍为 open/draft，head `2445e501d1ce099665d3f9f90b4944cb41e9cd2d`，base `02a8e1e77ff4e14840e81943180e0590b33b28f9`，mergeable=true。运行时代码仍为已审查及受测的 ed03cc6a，没有新的代码增量或阻塞代码发现。14个变更文件仍属于进度写入器、提示/安排、测试及文档；本会话未在 SteerLine 增加原生粘贴、终端发送或启动确认实现。

独立事项盘点：冲突、定向审计、固定 L1/L2、三项受影响 app 及独立进度 L3 已完成，保留上表实际结果，不重跑。较广 app incomplete 与首次 L3 超时仍保留为未覆盖范围和原因未定的历史风险，不能改写为通过或已修复。当前不扩展到无关功能或额外全量测试；文档及资源收尾完成后等待 PR66。PR399 继续保持草稿，不合并、部署或声称当前 main 新环境代际已取得原生资格。

## PR66 合入后的 rebase 与接线核实

用户随后告知 PR66 已合入并要求 rebase、完成收尾。独立 API 确认 PR66 实际 merge commit 为 `e588b8e64cdac013d6b0c4985baabbc15b3fc18e`，merged_at 为 `2026-10-27T08:19:47Z`。合入树与原 PR head `f2b32c332fc60ca5483b958ac53435f82207c5f8` 完全一致；与 Runner 干净受测产品源 `ffb9c510280d470e064e48770b8eea30174d82c4` 直接比较仅有七个验收文档/证据文件差异。没有把 GitHub 的三点比较结果当作两个提交的直接树差异。

PR399 从 `484925e8` rebase 到最新 `origin/main` `02a8e1e7`，新 head `cf62f4fd7ffa2550b32a7519a896794ee64c63a8`。重放历史断言维护时再次保留 main 的 Web 宿主限定。rebase 前后完整文件树均为 `655e7eb8c3bc04e3a7ce1d231d25e978cabd75d5`，没有运行时代码变化；以精确旧 head 的 force-with-lease 更新了原 PR，未覆盖其它更新。

针对新固定 Runner 补跑此前因未提供 EXE 而跳过的 `TestEnvironmentCheckPublishedContract` 和 `TestRunnerFullBindingPublicCLI`，实际两项通过，无 skip，耗时9.851秒；只使用独立 idle service 与公开 CLI，不启动模型或 Unity。原报告为 `C:/SteerlineDebug/wt-507e-pr399-post66/runner-public-contract-01/summary.json`，干净来源 cf62f4fd。已有 L1/L2、受控进度及 owner 回归未重复执行，原失败和未验范围不变。

使用 Runner 参考构建源 ffb9c510、版本 `0.2.0-dev.3-r3`、EXE SHA256 `a745ab738152e974850cbaca7851fb13c0d33cd08a915cda555f1ae465556347`，正式入口分别生成当前 PR 包 `packages-main/dev-20261027-162933-cf62f4fd` 和原复验代际包 `packages-original/dev-20261027-162933-8205bf0e`，均位于上述 post66 根且 ready/clean。这是同一已验产品源码的构建，不冒充 merge commit 重新构建；Runner 的原生探针使用 test EXE，也不冒充该 Runner EXE 的完整业务资格。旧包、home 和接受 pin 均未替换。

原业务 base485d 不含当前 main 新环境适配所需的 `SteerlineFlow/environment.md`。原进度两轮继续按已冻结 Caller8205 的独立复验代际进行，与当前 PR 构建及新环境资格分开；这不是产品运行时回退路径。Runner 负责原生输入实现，本会话不追加其逻辑、探针或重发。PR399 暂不计连续两轮完成、不自动合并或部署；后续原始进度结果仍按逐轮完整证据登记。

## 后续增量与当前结论

rebase 后真实复验又确认两项调用方增量：Plan 自身必跑项必须与规范要求先取并集；部分接线项目必须逐个固定 ID 核对真实入口，不能仅凭 helper 存在判兼容。两项均为生成/执行指令约束，不硬编码项目 ID，不改解析、安排、进度或通过标准。业务八项原报告通过但原进度6/8的负例保持不变；确定性项目入口缺口已提交 [PR435](http://192.168.0.66:3000/zymobile/merge-h-308-code/pulls/435)，等待审阅合入后才选新实际基线。

新增的 HTTP 回执快照修复属于 SteerLine 本地命令持久化，不涉及 Runner 原生输入。原生产读者持有完整旧快照时，原子发布新 outcome/discovery 必须成功；读者继续看到旧完整字节，后续读取看到新完整字节。源码 `a1583ee6` 保留原公开格式、envelope/digest、私有 ACL、regular/reparse 限制和有界失败；持续排他句柄仍报错，不添加兼容实现、copy/delete 回退或业务命令重试。Windows 路径按 UTF-16 编码并包含终止符；反例、首次编码失败及 Unicode 回归均保留。实现依据为 [FILE_RENAME_INFO](https://learn.microsoft.com/en-us/windows/win32/api/winbase/ns-winbase-file_rename_info) 与 [FILE_RENAME_INFORMATION](https://github.com/MicrosoftDocs/windows-driver-docs-ddi/blob/staging/wdk-ddi-src/content/ntifs/ns-ntifs-_file_rename_information.md) 的公开契约，不修改进程权限或全局配置。

最终主 PR 与对应原 Caller 源 `07fcb905` 的七 owner、固定 L1/L2、受控进度和宿主生命周期均有实际通过报告；main 首次组合 L3 的维护 GET 超时及随后同来源隔离通过分开保留，原因未确认。owner原生 opt-in分别跳过3/2项，不作为原生资格。详见[复验记录](test-progress-0of6-20261026.md)的精确路径、源码和未验范围。

当前审查没有新增阻塞代码发现；原约定连续两轮仍未完成，PR399继续草稿。没有自动合入、部署或在 SteerLine 新增终端粘贴/输入/启动确认实现。真实业务修复 PR435 的协议42+15项通过不冒充 Unity业务、clean发布head或完整人工验收；其合入和后续新基线两轮仍待完成。

2026-10-28 最新结果：PR435 已按用户明确指令正常合入，实际3b59842c。正式原Caller07fcb905包上的新entryfixed-01保留全部九项，原报告/输入/cleanup、正式entry writer、原始与closed进度/API/UI9/9均已独立核实；但后续发布输入未确认接受、停留原生草稿，没有真实PR。原Run/宿主/拥有资源已完整结算，第二轮未启动、连续数0/2。代码未变化，本次只更新验收和等待边界，不重复已完成受控回归。详见[本轮证据](test-progress-0of6-20261026.md#2026-10-28-pr435-合入后九项进度通过发布输入未确认)。PR399保持草稿；Runner输入职责不迁入SteerLine，clean发布head/完整业务/人工和当前main新环境原生资格仍未验。
