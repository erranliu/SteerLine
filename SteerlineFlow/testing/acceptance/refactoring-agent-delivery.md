# 重构与维护的 SteerAgent 决策与交付验收

## 当前方案：独立 PR 合入评审

维护改用现有 `execute/1` 已验证最终回执。独立评审读取已核验的 PR 版本、Git 差异及可用检查，只保存至原行动。以下正文同步和初始化门禁证据属于前一方案。SteerRunner #157 已关闭，当前方案使用该 PR 之前的固定构建验证。

| 提示词资产 | 变更前 → 变更后（UTF-8 字节） | 业务原因 |
| --- | --- | --- |
| `maintenance-decision.md` | 293 → 317 | 使用原文件清单定位固定版本差异 |
| `maintenance-execute.md` | 507 → 503 | 直接交付原三级指令和事实，单行 JSON 保留原指令换行，回执由最终回复承载 |
| `materials/maintenance-followup.md` | 3418 → 2873 | 删除正文同步和从工作区补评审证据的流程，保留原 PR 冲突续作 |
| `materials/maintenance-report.md` | 2559 → 994 | 只按已核验的 PR 证据评估合入价值、风险和新增结构债 |

完整资产及实际调用方样本差异见 [本轮差异](evidence/refactoring-pr-review-prompts.diff)。首轮正文和三种策略字节未变。历史联验结果按受测版本保留。

独立审核确认四份资产、Spec 正文与实际样本。决策短正文实际渲染 316 字节，可写黄金样本渲染 589 字节，实际写入长度随原三级指令及事实变化。审核确认可写回执绑定原 Run，评审 schema 仅允许建议与正文。审核发现历史费用补查会抢占 PR 版本轮询，已将补查排在活跃行动和 PR 版本检查之后并补回归。

前置能力验证和主体接线已完成，逐项验证范围见下文。关联 [Spec](../../spec/refactoring-agent-delivery.spec.md) · [Plan](../../plans/refactoring-agent-delivery.plan.md) · [测试选择](../../plans/refactoring-agent-delivery.tests.json)。原最小闭环验收见 [历史证据](refactoring-maintenance.md)。

## 前置能力验证

2026-11-12 使用 SteerLine 基线 `400e15dcfab24074d0062caeb46d35043a5e09e8`，工作树仅有待审文档。通过临时 Go overlay 运行两个诊断用例，2 项通过、零跳过。探针源码、实际命令和原始报告保存于 `C:\SteerlineDebug\wt-1851-refactoring-agent-preflight\diagnostics\preflight-20261112T083901Z-5d21ad1d\`。结果入口为 `report/summary.json`，逐项日志见 `report/go-test.jsonl`。

| 能力 | 实测结果 | 实施判断 |
| --- | --- | --- |
| 固定 Runner 的首轮只读目录和后续无目录调用 | Runner `0.2.0-dev.4-maintenance`，源码 `c404d6e5c50cc32650f9412f9b8b3449c5789e15`，EXE SHA-256 为 `d83152a9712486025b18304d3496e62f57072d3d51088f3315632f8c24275773`。首轮绑定独立 Git 目录，前后 HEAD 均为同一启动提交且工作区干净。后续请求仅带文件材料，实际工作目录是该 Run 的独立空目录。两次均核对只读派生配置、材料 manifest 和已验证的原生 Final，Run 与服务均已停止并核实结算 | 复用现有 `generate/1`。宿主保证首轮目录内容固定。无目录调用的验证范围为请求授权、运行目录及正式文件通道，未验证操作系统级的任意路径读取隔离 |
| PR 的 head/base 核验 | Gitea HTTP 夹具沿实际 `QueryClient` 查询同一 PR，head 保持不变，`base.sha` 从一个完整 SHA 变为另一个，当前查询结果完全相同。命令型查询接受现有结果，加入 `base_revision` 后被当前严格解码拒绝。本机 `gh` 列出 `headRefOid` 和 `baseRefOid`，现有映射只请求 head SHA | 在现有结果模型增加可选 `base_revision`，Gitea 映射已解析的 base SHA，命令型维护配置供给该字段。只在维护交付要求可信版本，普通任务继续接受原字段集合 |

Runner 使用同源原生协议夹具，未调用真实模型。PR 对比使用本地 HTTP 与命令结果夹具，未修改远端。GitHub 登录令牌无效，本次匿名查询也未能连接，因此真实平台联验仍归 RAD-A07。两项结果支持在本期完成接线，主体实现和全部验收项仍待执行。

## 验收项

| ID | 方式与操作 | 最小通过条件 | 状态 |
| --- | --- | --- | --- |
| RAD-A01 | 自动。冻结启动提交后调查，在主目录变化与执行目录不可见的夹具中推进 | 首轮读到固定提交目录，无法核实时阻断；首条三级指令后的审计、答复恢复、收尾和报告均仅有行动文件材料，无仓库或目录授权；实际来源和封存校验通过 | 已验收 |
| RAD-A02 | 自动。检查三种策略、阶段材料及实际可写请求，用多轮记录和大反馈验证上下文 | 各阶段读取对应一级正文和适用策略，冻结生效。后续决策只携带当前所需事实，报告使用最终版本证据和轮次摘要，原记录仍可追溯。材料大小受现有限额约束，关键证据缺失或超限有具体诊断。可写业务提示只含三级指令，同步的 PR 身份、版本及报告原文仅从随附材料读取 | 已验收 |
| RAD-A03 | 自动。用浏览器完成首轮与后续两种人工答复；覆盖 Runner `needs_input` | 结构化问题和推荐可读；等待期间无后继投递且暂停计时；有效答复恢复正确阶段，由 SteerAgent 形成新的三级指令；Runner 反馈先由 SteerAgent 判断 | 已验收 |
| RAD-A04 | 自动。注入无效输出、当前阶段不允许的 kind、旧答复、重复命令、重启及停止/结束竞态 | 无效结果无可写投递；原问题持久，合法答复最多推进一次且恢复原用途；停止或收尾取消旧待答，迟到结果沿原请求结算；同一行动和 worker 按阶段表推进 | 已验收 |
| RAD-A05 | 自动。受控执行预算收尾、有依据提前收尾、首轮直接无改动和报告缺证 | 首轮有充分无改动证据且零写执行时，直接生成报告，零额外收尾决策、零 execute、零 PR。正常交付先核验版本，再生成报告并原样同步。报告补证封存后只回核验和报告，零新轮次、零重新发布；同一缺口无新证据时停止重复补证 | 已验收 |
| RAD-A06 | 自动。注入发布回执丢失、同步结果未知、同步明确失败、head 变化和仅 base 变化，执行原 PR 冲突续作 | 未知只对账原号，可恢复失败复用同一正文及指令。版本变化保留原 PR 和旧报告，停止旧版同步或交付；核验并确认新证据覆盖后才生成新版。版本反复变化且证据不足时等待显式继续，已交付 PR 由手动冲突续作推进。最终平台正文与保存原文一致 | 已验收 |
| RAD-A07 | 自动，真实链。运行固定 Runner 和本机 Codex 专项，使用获授权隔离测试仓库 | 同 worktree 多轮、真实 PR 核验后生成报告并原样同步、真实冲突后更新同一 PR；收集原生来源及平台证据，清理本次资源 | #843 真实整链通过，初次与冲突后版本的 head/base 及 PR 正文均经独立核验，资源已清理。新版同步的 Runner 回写失败，由现有平台对账确认交付 |
| RAD-R01 | 自动，回归。固定 L1/L2 | 两个正式测试集通过且证据完整 | 已验收 |

执行入口与准备见 [Plan](../../plans/refactoring-agent-delivery.plan.md#3-验证安排)和[正式清单](../test-list.json)。受控准备使用隔离 home、Git、SQLite、公开 Runner/PR 夹具和浏览器；原生及真实链固定 Runner 构建身份，真实链另需本机 Codex、测试仓库权限和独立测试分支。RAD-A01 至 A06 的受控结果单独记录，RAD-A07 验证实际原生模型与外部 PR 边界。

## 提示词审核与结果登记

完整正文见 [Git 正文差异](evidence/refactoring-agent-delivery-prompts.diff)。固定文件使用 UTF-8/LF，渲染后的短指令省略文件末尾 LF。

| 资产 | 文件字节数（前 → 后） | 业务原因 |
| --- | --- | --- |
| `maintenance-decision.md` | 252 → 293 | 按 Runner 材料清单的 file 路径读取本阶段规则、上下文及适用策略，按输出约定回传 |
| `maintenance-execute.md` | 548 → 507 | 固定正文说明无 BOM 的结果文件协议、字段类型和 needs_input 回传，执行业务正文来自本次三级指令 |
| `materials/maintenance.md` | 5161 → 0 | 原公共正文按首轮、后续和报告的权限及用途拆分 |
| `materials/maintenance-first.md` | 0 → 1294 | 固定启动目录的只读调查、首条执行指令及结构化人工请求 |
| `materials/maintenance-followup.md` | 0 → 3418 | 依据行动上下文审计、补证、结构化人工请求、收尾和复核新 PR 版本，明确区分审计、选题及冲突续作 |
| `materials/maintenance-report.md` | 0 → 2559 | 已核验版本上的报告及引用结构化版本事实的正文同步指令 |
| `materials/maintenance-{cautious,autonomous,aggressive}.md` | 0 → 214、229、241 | 按已选策略冻结单段正文 |

实际请求的短指令为决策 251 → 292 字节、可写执行 547 → 506 字节。首轮及后续决策各读取三份正式文件，报告读取规则与上下文两份文件。可写初始化中只有三级指令和本次数据，报告 CRLF 与尾部空格由交付测试逐字核对。独立复核批准最终正文及接线，确认正式写入点登记范围仅从原 `service.go` 迁到 `operations.go`。复核测试 `go test ./internal/prompts ./internal/steeragent/maintenance ./internal/testkit/boundary -count=1` 通过。

正式验收逐项登记受测版本、命令、结果、原报告路径、清理事实及未验范围，遵循 [测试指南](../../testing.md#环境与证据)。

## 受控与回归结果

受测源码为 `400e15dcfab24074d0062caeb46d35043a5e09e8` 加本期工作树改动。证据根为 `C:\SteerlineDebug\wt-1851-refactoring-agent-delivery\`，各级实际命令、构建身份、终态和清理由相应的 `summary.json` 与原场景记录保存。

| 验证 | 结果及证据 |
| --- | --- |
| 定向维护、应用、决策与提示词 | 通过。见 `diagnostics/owners-final-20261112T091239Z-b3c6b60d/summary.json`。覆盖首轮无改动直出报告、问题重启恢复和命令重放、结束时取消待发实施、同步回执未知与明确失败、head/base 变化、反复漂移后显式继续及大反馈上下文边界 |
| L1 `l1.minimum` | 通过。见 `test-results/L1/20261112T091239Z-b3c6b60d/summary.json`。首轮暴露了提示词写入点移动及纯依赖边界问题，已收敛写入点登记，结构化问题由维护模块校验并保存 |
| L2 `smoke.delivery` | 通过。见 `test-results/L2/20261112T090226Z-d1000b8e/summary.json`，普通任务核心交付及资源清理通过 |
| L3 `runtime.refactoring-maintenance` | 最终复验通过。见 `test-results/L3/20261112T091239Z-b3c6b60d/summary.json`。完成实际浏览器启动、选项答复、两轮和失败修复、同一 worktree 发布与同步、原 PR 冲突续作和完整资源清理。首轮构建缺 Web 产物，已构建后复验 |
| L4 `go.http` | 通过。见 `test-results/L4/go-http-20261112T090226Z-d1000b8e/summary.json` |
| L4 `go.storage` | 通过。见 `test-results/L4/go-storage-20261112T091050Z-f827e982/summary.json`。首轮额外启用了旧任务冻结请求的可选真实 Runner 校验，固定 Runner 拒绝该夹具的旧配置字段。按登记的隔离 SQLite 范围复验通过，维护的 Runner 公开契约另由下项验证 |
| L4 `go.runner-result` | 通过。见 `test-results/L4/runner-result-20261112T090226Z-d1000b8e/summary.json`。固定 Runner 和同源原生协议夹具验证只读目录、后续无目录、文件 manifest、停止封存和原工作区持有者 |
| 最终提示词和阶段复验 | 通过。见 `diagnostics/final-native-protocol-20261112T092737Z-6901678d/summary.json`，覆盖最终正文、输出约定、源码边界、实际材料和审计阶段的轮次准入事实 |

## 真实 GitHub 联验

沙箱外 GitHub CLI 登录有效。首次真实联验使用 `test-results/L4/live-20261112T091239Z-b3c6b60d/`，首轮曾触发原生提问并自动重试，随后完成真实实施。审计读取到新轮次可用，将下一个切片误作本轮补做。通过正式 `refactoring_stop` 停止该测试行动，场景确认资源清理完成。依据实际结果收紧了人工请求输出通道和审计与选题阶段的权限说明，最终定向复验通过。

第二次整链见 `test-results/L4/live-20261112T092737Z-6901678d/`。两次首轮调用都返回 `human_input_required`，行动在第一条可写指令前阻断，场景确认清理完成。单独的真实首轮探针随后完成只读调查并返回可信的结构化结果，见 `diagnostics/native-first-history-20261112T093403Z-be2cb60d/report/summary.json` 和公开历史。探针结算原 Runner 服务，仅覆盖首轮，未创建 PR。

确认跑见 `test-results/L4/live-confirmation-20261112T093623Z-72cb7454/`，同样在首条可写指令前阻断并清理完成。该次保存的原 Run 公开历史表明模型将逻辑材料名当成了工作目录内的相对路径。决策短指令改为按 Runner 材料清单的 `file` 字段读取实际文件。独立复核确认字段含义、固定正文差异与渲染字节。

材料路径修正后的定向验证通过，见 `diagnostics/material-location-20261112T094048Z-08be9325/summary.json`。随后的真实整链见 `test-results/L4/live-material-location-20261112T094048Z-08be9325/`。首轮一次完成，`alpha.cjs` 的实施与审计通过，SteerLine 另开第二轮选择 `beta.cjs`。第二轮 Runner 报告 `STEERLINE_RESULT_FILE` 指向的文件不可用，公开结果为 `read_failed`。审计保留失败事实，行动在发布前阻断。固定 Runner 将结果文件初始化声明为限时的尽力操作，现有证据尚不足以区分这一轮的具体初始化或读取原因。场景确认资源清理完整。本次未取得真实 PR 的 head/base 核验、报告原样同步或冲突续作证据，这些行为已由受控场景覆盖。

固定 Runner 的结果文件补测为同一 worktree 上的四个写入 Run 分配不同种子，等待每个 Run 的结果真正封存，再核验原 Run、摘要及完整字节。首次补测读取到了仍处于 `pending` 的快照，结果保留在 `test-results/L4/runner-seed-reuse-20261112T095108Z-0ac1d386/`。按封存状态等待后的定向复验通过，见 `diagnostics/reuse-sealed-result-20261112T095357Z-694fe0d3/summary.json`。该证据验证固定 Runner 在受控夹具下正确初始化并封存各后继的文件，未解释真实模型第二轮的 `read_failed`。

### 第二轮结果文件的最小复现

2026-11-12 使用同一固定 Runner `c404d6e5c50cc32650f9412f9b8b3449c5789e15` 和受控原生协议夹具运行两条 `execute/1`。首轮在受管 worktree 中提交一个本地改动，第二轮引用原 Run 并复用同一 worktree。两轮有不同的结果文件种子。夹具在 `turn/start` 读取进程环境中的 `STEERLINE_RESULT_FILE`，记录实际路径和种子摘要。调用方独立检查对应路径，并等待原 Run 停止和公开结果封存。两组探针都完成取证，Runner 服务已结算。证据根为 `C:\SteerlineDebug\wt-1851-runner-result-root-cause\diagnostics\two-run-20261112T101120Z-31f1c7ef\`。

| 组别 | 进入原生执行时 | 封存后 |
| --- | --- | --- |
| 无注入延迟 | 首轮正确读取自己的种子并写回。第二轮 `47ec9d3025395c8c80f9543048ff0c99` 收到正确的本轮路径，但进程打开路径失败，调用方独立读取同一路径也失败。此时公开结果仍为 `pending`，诊断已是 `read_failed` | 首轮封存版本 1，正文逐字匹配；第二轮封存版本 0，诊断为 `read_failed` |
| 第二轮初始化 Git 探测延迟 5.2 秒 | 在 worktree 的 `git rev-parse --path-format=absolute --git-path info/exclude` 注入延迟。首轮读取成功；第二轮路径仍正确，文件在 `turn/start` 时不可用，公开待封存诊断已是 `read_failed` | 首轮封存版本 1；第二轮封存版本 0，诊断为 `read_failed` |

每组的原生进程取证见 `native-calls.jsonl`，调用方及封存对照见 `stages.json`，延迟命中的命令见 `git-shim/delay.jsonl`。故障已在封存前出现，且注入路径等于本轮约定路径，定位到 Runner 派发前的初始化。固定 Runner 的 `prepareRuntimeFiles` 为结果文件提供独立四秒的尽力初始化窗口，失败仍允许原生输入派发。延迟组证明初始化超时足以产生相同的外部现象。无延迟组和真实失败的内部原因只有泛化后的 `read_failed`，尚不能确定实际耗时的具体 Git 或文件步骤。

### 必需初始化修复

Runner 在独立 worktree 实施了 `result_file.require_initialization`。开启时在原准备期限内核验本 Run 的固定文件、种子 SHA-256 与写入结算，随后才派发首条业务输入。普通调用仍使用各通道独立四秒的尽力准备。SteerLine 先确认目录公布该能力，仅维护可写请求发送该标志。初始化阻断后，行动记录保留 Runner 的原诊断并发起原 Run 的停止，等到原结果封存才结束本次操作。已确认零派发的发布可通过原行动重新收尾，仍沿用既有工作区和 PR 边界。

独立复核确认三级业务提示词、种子与报告原文字节均无变化。实际短指令继续为决策 292 字节、可写执行 336 字节。

Runner 源码提交为 `ed2ac946f8806f6ae3edfb13da8b78b6ec47d072`，独立构建版本为 `0.2.0-dev.4-required-result-init`，EXE SHA-256 为 `cb88b7a3982f1e9ab681e3f7955299ed5db33d03a54c4dfb3c5b240828e512e3`。证据根为 `C:\SteerlineDebug\wt-1851-required-result-init\`。

- Runner `TestRequiredResultInitializationGatesFirstNativeInput` 通过。正确种子恰好派发一次，缺失、摘要不符和超时均零原生输入、零持久化输入意图。既有的普通调用故障、独立超时回归和公开 schema 断言通过。见 `diagnostics/runner-gate/go-test.log`。
- 原 5.2 秒延迟复验通过。第二轮 Run `03fa59c02b9689b4a8c62419c9fa3268` 在原生输入前读到自己的种子，两次均封存为版本 1，原 Runner 服务已结算。见 `diagnostics/replay-delay-3a6c4b68/` 的 `stages.json`、`native-calls.jsonl` 和 `git-shim/delay.jsonl`。
- SteerLine `TestMaintenanceInitializationFailureStopsOriginalAndAllowsFinish` 和 `TestRefactoringStartupReconcilesOriginalStop/initialization-failed` 通过。正常观察和离线恢复都在停止前保存初始化失败。停止及封存后诊断保留，原行动可收尾。原发布结果未知仍阻断收尾。实际提示词文件交付断言通过。
- SteerLine L1 和 L2 通过。见 `test-results/L1/retry-4afb0bf3/summary.json` 与 `test-results/L2/20261112T102928Z-dee5c534/summary.json`。初次 L1 发现一个普通调用测试夹具仍使用旧 Go 类型，修正类型后通过，普通请求 JSON 未变。
- 受控 `runtime.refactoring-maintenance` 复验通过，见 `test-results/L3/maintenance-5e7c4941/summary.json`。
- `go.runner-result` 中的公开维护契约测试通过，四次执行复用同一 worktree，各自取得正确种子并完成封存。整组首次因普通 101 阻断子例未初始化 progress 而失败，该次停止回执未知，后续按夹具路径检查无匹配的残留进程。相同固定构建的该子例定向复验通过。见 `test-results/L4/native-result-3d12f43d/go-owner/test.jsonl` 与 `diagnostics/native-101-retry-9f61675c/go-test.log`。

### 首次真实交付与冲突入口修复

固定新 Runner 的真实整链见 `test-results/L4/live-20261112T102440Z-653300cd/`。两轮均封存成功，测试 PR #811 取得实际 head `20f0125f31f7ecd9dc945844eef5f2bc2d69d00a`、base `67894aea503d37aca36f950ffdabf799a58411dd`，GitHub 正文逐字等于保存的报告，SHA-256 为 `d92e72d6f1ad423fcb0b646a9fc4ccc0a9dab9c7169a5e96efb85cb6e92fd0a2`。推进 base 后 Git 在两个文件产生真实冲突。冲突入口曾丢弃最新核验事实，SteerAgent 使用旧版本返回了 `accept`，原阶段正确阻断。场景已清理测试 PR、分支及运行资源。

修复后，同一行动保存手动冲突入口核验得到的当前 PR，收尾上下文增加 `continue_conflict` 事实。原报告保留旧版本绑定。后续规则第四段的实际改动如下。

```diff
-实施、修复和验证仅在受管工作区内执行。进入收尾后，只推进已有成果。按需要求 Runner 完成必要验证、检查目标分支变化、解决冲突并复核收益。可交付时要求 Runner 在原维护分支发布 PR，或更新已有的同一个 PR，保持待审。发布结果未知时，先核实原操作和 PR 状态。没有可交付改动时，回传无改动结论及证据。已有 PR 的续作始终保留原 PR 身份。
+实施、修复和验证仅在受管工作区内执行。收尾（mode=finish）只推进已有成果，返回 execute 和三级指令，要求 Runner 检查目标分支变化、完成必要验证并回传交付结果。continue_conflict 为 true 时，以上下文 pr 的当前 head/base 为准，要求 Runner 处理原 PR 的冲突，保留两边有效改动并验证，只更新原 PR 的维护分支。已有 PR 保持待审。初次交付可在原维护分支发布 PR。发布结果未知时，先核实原操作和 PR 状态。没有可交付改动时，要求 Runner 回传无改动结论及证据。
```

后续一级规则 2355 → 2498 字节。决策和可写短指令继续分别为 292、336 字节。独立复核通过正文、字节及同一 PR 的新版本接线。文件材料、角色限制、源码守卫和新版本冲突请求定向验证通过。

修复冲突接线后的第二次真实联验见 `test-results/L4/live-conflict-20261112T104645Z-4bc9a943/`。两轮封存成功，发布并核验测试 PR #812。初次同步的原生 Run 使用匿名 GitHub API，私有仓库返回 404，Runner 回传 `needs_input` 并保留原正文。场景清理完整。冲突路径未执行到。

报告规则最后一段增加已认证的版本查询和正文文件更新路径。GitHub 使用运行环境中的 `gh`，其他平台使用其已认证工具。报告正文仍取原请求附带数据。实际正文改动如下。

```diff
-正式报告返回 report。有 PR 时，同时生成一条同步用的三级提示词，引用随附材料中的已核验 PR、head/base 和报告正文。要求 Runner 核对版本后将随附正文原样写入该 PR 正文并回传结果。若版本已变化，回传实际版本。无 PR 时，仅交付报告正文供 SteerLine 展示。
+正式报告返回 report。有 PR 时，同时生成同步三级提示词，引用随附材料中的原 PR、head/base 和 context.report。要求 Runner 使用执行环境已认证的 PR 工具核对版本。GitHub 使用本机 gh pr view <PR URL> --json headRefOid,baseRefOid 获取当前版本。匹配时，解析随附 JSON，将 context.report 原样写成无 BOM 的 UTF-8 文件，保留换行和尾部空白，再用该文件更新原 PR 正文。GitHub 使用 gh pr edit <PR URL> --body-file <文件>。版本变化时回传实际 head/base，暂停同步。无 PR 时，仅交付报告正文供 SteerLine 展示。
```

报告一级规则 1341 → 1625 字节。独立复核通过正文、字节、原 PR 和 head/base 边界，核对了 Spec、实际资产及差异记录。

最终规则的 L1、L2 和受控 `runtime.refactoring-maintenance` 均通过，见各级 `auth-sync-20261112T110815Z-1e2293c5/summary.json`。第三次真实联验见 `test-results/L4/live-authenticated-sync-20261112T110431Z-e25ecfc0/`。本机 Codex 完成两轮并创建测试 PR #813，但发布 Run 的已封存文件未能解出与本次操作匹配的结果，SteerLine 停在原发布待核实。本次证据没有保留该文件原文，无法再区分 JSON 格式、身份或结果值。场景已核实清理，最终同步规则尚未在真实模型整链执行到。

### 真实 GitHub 目标分支与冲突定向验证

定向验证复用 SteerLine 的维护状态推进，通过确定性 Runner 夹具控制阶段与结果，使用真实 Git 分支和 GitHub draft PR。首次发现 PR 的 `baseRefOid` 在目标分支推进后仍为旧提交，见 `diagnostics/focused-github-d99e258e/go-test.log`。第二次观察 45 秒仍为旧值，见 `diagnostics/focused-github-78d0b338/go-test.log`。两次均确认清理。

真实 API 复验确认直接读取目标分支引用可取得当前提交。测试 PR #816 初次和冲突后的 head/base 与平台事实一致，两次正文逐字等于保存报告，包含夹具的 CRLF 和尾部空白。真实 Git 内容冲突解决后更新原 PR，保留原维护改动和并发基线证据，原目标分支和维护分支已删除，PR 已关闭。见 `diagnostics/focused-github-93c82eb8/proof.json`、`conflict-proof.txt` 和 `go-test.log`。

维护验收与调试模板使用 GitHub GraphQL 的 `baseRef.target.oid` 返回目标分支当前提交。同步规则的实际正文改动如下。

```diff
-正式报告返回 report。有 PR 时，同时生成同步三级提示词，引用随附材料中的原 PR、head/base 和 context.report。要求 Runner 使用执行环境已认证的 PR 工具核对版本。GitHub 使用本机 gh pr view <PR URL> --json headRefOid,baseRefOid 获取当前版本。匹配时，解析随附 JSON，将 context.report 原样写成无 BOM 的 UTF-8 文件，保留换行和尾部空白，再用该文件更新原 PR 正文。GitHub 使用 gh pr edit <PR URL> --body-file <文件>。版本变化时回传实际 head/base，暂停同步。无 PR 时，仅交付报告正文供 SteerLine 展示。
+正式报告返回 report。有 PR 时，同时生成同步三级提示词，引用随附材料中的原 PR、head/base 和 context.report。要求 Runner 使用执行环境已认证的 PR 工具核对版本。GitHub 使用本机 gh pr view <PR URL> --json headRefOid,baseRefName 获取当前 head 与目标分支，再用 gh api repos/<原PR仓库>/git/ref/heads/<目标分支> --jq .object.sha 读取该分支当前提交作为 base。匹配时，解析随附 JSON，将 context.report 原样写成无 BOM 的 UTF-8 文件，保留换行和尾部空白，再用该文件更新原 PR 正文。GitHub 使用 gh pr edit <PR URL> --body-file <文件>。版本变化时回传实际 head/base，暂停同步。无 PR 时，仅交付报告正文供 SteerLine 展示。
```

报告一级规则 1625 → 1762 字节。独立复核通过真实查询字段、报告原文文件同步和原 PR 边界。Spec、实际资产、差异记录及摘要断言一致。

最终 GitHub 查询与独立 Git 引用 API 的交叉核验通过。测试 PR #817 的初次 head/base 为 `c8b37dda6fea973b147cbc00864ae3d74804ad55` / `58d683a597e94624ef02dd6f3e1417012bbca9ac`，冲突续作后为 `1022f4afe88c0e5300df76882c0c18fcf8f2a8c7` / `aae2d88e2c7b4e3fe887aa880e70241dbee36ec3`。同一 PR 的两次报告原文字节匹配，原 PR 已关闭且两个测试引用已删除。见 `diagnostics/focused-github-graphql-5b64dcf3/proof.json`、`conflict-proof.txt` 及 `go-test.log`。本次平台验证由确定性 Runner 夹具驱动。

最终 SteerLine L1、L2、L3 维护场景均通过，见各级 `github-target-tip-20261112T112543Z-2c280ca4/summary.json`。Runner 实现在独立 worktree 提交，GitHub 确认分支与 main 可合并。[SteerRunner #157](https://github.com/erranliu/SteerRunner/pull/157)

### 真实发布封存结果的操作校验

2026-11-13 复查上次测试 PR #813。场景已清理项目、Runner 状态和原生会话，本次无法恢复该 Run 的原结果。用同一个固定 Runner 和真实 Codex 在测试仓库执行一条可写发布请求，保留种子、公开封存信封与解码后的原始字节，再清理该次资源。第一条探针选到了不可用的桌面包 EXE，切换到项目测试发现的本地原生 Codex 后成功执行。

单步探针 Run `43c647be3f7a5111767386aa01beff7b` 创建了测试 PR #818。原文件为 643 字节，SHA-256 为 `031e6fa5bfd7c98970c9077e45d8469545b99e7eec1af689cde03d7cf171c25b`。它是无 BOM 的有效 JSON，`action_id`、`operation_id` 均与原种子相同，`outcome` 为 `done`。其中 `pr` 是包含 number、url、state、draft 的对象。将原字节解码到 SteerLine 的结果类型会在 `pr` 字段产生类型错误。原 Runner 封存和停机证明完整，测试 PR 与分支已清理。原件见 `C:/SteerlineDebug/wt-1851-publication-seal/diagnostics/publish-673a8e6d/` 的 `seed.bin`、`sealed-envelope.json`、`sealed-result.bin` 和 `comparison.json`。证据证明同类失败可由字段类型触发，#813 的具体原字节仍未知。

在固定短接线中明确已存在的 JSON 契约。回传格式、字段类型及 ID 错误分别记录到操作反馈。业务指令仍取 SteerAgent 的三级指令。测试在真实发布、同步和冲突操作完成后保存公开封存数据与原始字节，再判断交付和清理。

```diff
-读取 STEERLINE_RESULT_FILE 中的 instruction 及随附 context。执行 instruction，并将结果原子写回同一文件。保留 action_id、operation_id，返回 outcome（done、needs_input 或 blocked）、summary、head、base、pr、no_changes、question。需要输入时，以 needs_input 和 question 结束本次操作。
+解析 STEERLINE_RESULT_FILE 指向的 JSON，执行其中的 instruction，使用随附 context。将单个无 BOM 的 UTF-8 JSON 对象原子写回同一文件。逐字复制原顶层 action_id 和 operation_id。outcome 字符串取 done、needs_input 或 blocked。summary、head、base、pr、question 均为字符串，缺值用空字符串。head、base 有值时填写完整提交，pr 有值时只填写完整 PR URL。no_changes 为布尔值。需要输入时，返回 needs_input 和 question。
```

可写执行短指令实际渲染为 336 → 506 个 UTF-8 字节。独立复核核对实际正文和字节、三级指令及原报告传递、错误结果拒绝与封存取证。回归使用相同的 PR 对象形态，要求具体字段诊断且无法形成候选。

修复版真实整链 `test-results/L4/live-typed-publication-20261113T023154Z-96e80369/` 保存了测试 PR #819 的原发布封存。两项 ID 匹配，`pr` 为 URL 字符串，`outcome=done`。SteerLine 通过发布与平台版本核验后进入报告阶段。报告模型将下一次执行的 `context.report` 误作当前依赖，返回 `needs_evidence` 和 `prompt`，因此在报告输出校验处阻断。测试资源已清理。

后续规则正文改动如下。

```diff
-正式报告返回 report。有 PR 时，同时生成同步三级提示词，引用随附材料中的原 PR、head/base 和 context.report。要求 Runner 使用执行环境已认证的 PR 工具核对版本。GitHub 使用本机 gh pr view <PR URL> --json headRefOid,baseRefName 获取当前 head 与目标分支，再用 gh api repos/<原PR仓库>/git/ref/heads/<目标分支> --jq .object.sha 读取该分支当前提交作为 base。匹配时，解析随附 JSON，将 context.report 原样写成无 BOM 的 UTF-8 文件，保留换行和尾部空白，再用该文件更新原 PR 正文。GitHub 使用 gh pr edit <PR URL> --body-file <文件>。版本变化时回传实际 head/base，暂停同步。无 PR 时，仅交付报告正文供 SteerLine 展示。
+证据齐备时返回 kind=report，report 字段填写你现在撰写的完整正文。有 PR 时，prompt 字段另写一条给后继 Runner 的同步三级指令。SteerLine 会保存本次 report，并在后继执行时才把原文放入其 context.report。缺证时返回 kind=needs_evidence，仅用 summary 描述缺失的证据。无 PR 时只交付报告正文。

同步指令要求 Runner 用已认证的 PR 工具核对原 PR 及已核验 head/base。GitHub 使用 gh pr view <PR URL> --json headRefOid,baseRefName 取得 head 和目标分支，再用 gh api repos/<原PR仓库>/git/ref/heads/<目标分支> --jq .object.sha 取得当前 base。版本匹配时，Runner 从后继请求的 JSON 取 context.report，原样写入无 BOM UTF-8 文件，保留换行及尾部空白，再用 gh pr edit <PR URL> --body-file <文件> 更新原 PR。版本变化时回传实际 head/base，暂停同步。
```

报告一级正文由 1762 增至 1910 字节。独立复核通过当前生成、后继 Runner 数据来源及原 PR 同步边界。实际正文、Spec 和摘要一致。

并行取证对照 `test-results/L4/live-sealed-evidence-20261113T022856Z-845ae83c/` 使用类型修订前的构建，测试 PR #820 亦通过原发布及报告生成。同步 Run 的原结果已经保存，模型报告已核对 head/base，但执行环境拦截 PR 正文更新，回传 `blocked`。场景未进入冲突续作。

最终整链 `test-results/L4/live-report-chronology-20261113T024522Z-295824df/` 中，真实 Codex 完成两轮并发布测试 PR #821。原发布结果通过校验，GitHub 版本核验后生成报告。同步 Run 回传 `blocked`，确认 head/base 一致并报告两次 `gh pr edit` 被原生执行环境拒绝。发布和同步原字节分别存于场景目录的 `sealed-maintenance-results/9201a9a37a50273f153d8a14b1d243d1/` 和 `sealed-maintenance-results/3324b4622c5515f62c21a738b3447d3c/`。场景未进入冲突续作，已核实测试资源清理及 Runner 停机。对应版本的 owner、L1、L2 和 L3 维护受控场景通过，报告见 `report-chronology-20261113T024608Z-644000ca/`。

单步真实同步复现使用测试 PR #822 和同一固定 Runner。原生 Run `58195220d1f395a5906d7ff08aaada1a` 回传 `blocked`，报告的启动错误为 `CreateProcess: Rejected by policy`。独立 GitHub 读回确认原正文保持。原始工具回执只有调用 ID 和空输出，尚不能归因到 Runner 或具体权限层。见 `C:/SteerlineDebug/wt-1851-publication-seal/diagnostics/sync-7120de96/` 的 `sealed-result.bin`、`sync-proof.json`、`native-sync-tool-receipts.json` 和 `go-test.log`。测试分支已清理，原 Runner 已结算。

### 原生命令拒绝的最小复现

以同一固定 Runner 和相同 GitHub 登录复现测试 PR #823。原生 rollout 的请求和完整返回保存在 `C:/SteerlineDebug/wt-1851-native-pr-edit/diagnostics/sync-c9faefff/native-command-events.jsonl`。失败调用把解析结果、写临时报告、`gh pr edit` 和递归删除临时目录合在一条命令，原生执行工具返回 `CreateProcess … blocked by policy`。Codex turn 的实际策略为 `danger-full-access`、`approval_policy=never`。宿主以同一账户直接运行 `gh pr edit --body-file` 成功，GitHub 读回逐字匹配，见 `direct-gh-receipt.json`。拒绝发生在原生执行工具的进程启动阶段，完整回执未指明具体命中的规则。

测试 PR #824 仅调用独立的 `gh pr edit`。同一原生工具成功启动命令，app-server 原始 `item/completed` 显示退出码 1，原因是模型传入了不存在的报告文件路径。该请求未更新远端。见 `C:/SteerlineDebug/wt-1851-native-pr-edit/diagnostics/isolated-command-0259b525/native-appserver-exact-command.jsonl`。两次原 Runner 服务均已结算，测试分支已删除。

SteerLine 明确先保存报告并核对路径，再单独执行 PR 更新。实际正文变化如下。

```diff
-版本匹配时，Runner 从后继请求的 JSON 取 context.report，原样写入无 BOM UTF-8 文件，保留换行及尾部空白，再用 gh pr edit <PR URL> --body-file <文件> 更新原 PR。
+版本匹配时，Runner 先解析 STEERLINE_RESULT_FILE，在该文件所在目录将 context.report 原样写入 report.md，使用无 BOM UTF-8，保留换行及尾部空白。确认文件存在并取得绝对路径后，单独执行 gh pr edit <PR URL> --body-file <报告文件绝对路径> 更新原 PR。报告文件随受管工作区清理。
```

报告一级正文 1910 → 2062 字节。独立复核核对正文、Spec、原 PR 与版本边界，以及结果文件同目录和报告原样同步。Runner 实现及权限配置未改动。

同步规则修订后的真实运行 `C:/SteerlineDebug/wt-1851-native-pr-edit/test-results/L4/live-separate-sync-20261113T032141Z-b54120ce/` 发现第二轮审计使用启动提交比较累计差异，将首轮已接受的改动误判为本轮越界。SteerLine 现从前轮已接受操作中提取经核验且干净的工作区 HEAD。首轮使用启动提交。没有可信起点时由审计补证，修复请求继承同一本轮起点。

```diff
-成果审计（mode=audit）只判断本轮切片。根据实际差异、原生执行结果和验证结果，判断范围、收益和代价。
+成果审计（mode=audit）只判断本轮切片。以 round_start 为起点核对本轮差异，保留此前已接受的改动。缺少本轮起点或对应差异时，要求 Runner 补证。PR 在收尾阶段处理。根据本轮差异、原生执行结果和验证结果，判断范围、收益和代价。
```

后续一级正文 2498 → 2666 字节。独立复核通过正文、Spec、差异及实际接线，定向测试覆盖首轮、后继、修复请求和脏工作区等反例。

带本轮起点的真实运行 `C:/SteerlineDebug/wt-1851-native-pr-edit/test-results/L4/live-separate-sync-20261113T033450Z-d416a7ce/` 通过两轮审计并创建测试 PR #825。报告回复两次均在 `report` 后提前闭合对象，再返回 `prompt`，触发 `invalid_json: invalid character after top-level value`。原始反馈与已通过的发布封存均保留，场景确认清理。

```diff
-证据齐备时返回 kind=report，report 字段填写你现在撰写的完整正文。有 PR 时，prompt 字段另写一条给后继 Runner 的同步三级指令。
+证据齐备时返回单个 JSON 对象，kind 为 report。将当前撰写的完整正文写入 report 字符串。有 PR 时，在同一个对象的 prompt 字符串中填写后继 Runner 的同步三级指令。按 JSON 转义字符串内的换行和引号，最后关闭对象。
```

报告一级正文 2062 → 2178 字节。独立复核确认同一对象的生成约定和原报告解码后的字节传递。

真实复验 `C:/SteerlineDebug/wt-1851-native-pr-edit/test-results/L4/live-one-report-object-20261113T035023Z-9544bb18/` 首次通过测试 PR #826 的初次报告原样同步。推进目标分支后，两个文件产生真实内容冲突，原生 Codex 合入新基线，保留两轮变更和基线改动，验证后推送原 PR。冲突后 head 为 `7fa80f3266bc4e4f93ec2b5c843b72b5ff98b283`，base 为 `4ea26f37467a0d8731d96ead48db330762327064`。报告输入顶层 `pr` 保存了这组平台独立核验及 open 状态，模型仍将 Runner 自身匿名查询 404 误读为没有平台证据，阻断新版报告。保留封存及 Git 冲突证据。测试 PR 已关闭，测试引用已删除。原维护 Run 已结算，独立停机复核识别到状态目录还有一条与维护无关的 Unity Run，保留隔离现场。

```diff
-有 PR 时，以已核验的 PR 及完整 head/base 为依据。
+有 PR 时，行动上下文顶层 pr 是 SteerLine 最新独立核验的原 PR、状态及完整 head/base。若它与 final_evidence 的 pr、head、base 匹配，平台证据已齐备。Runner 的查询失败或旧报告属于历史反馈。
```

报告一级正文 2178 → 2358 字节。独立复核通过，明确平台状态来自 SteerLine 独立核验，与当前结果中的原 PR 和 head/base 一致时，历史查询失败不构成版本缺证。

使用平台优先级规则的真实复验 `C:/SteerlineDebug/wt-1851-native-pr-edit/test-results/L4/live-verified-report-20261113T042444Z-e3600bb5/` 在第二轮实施提前创建了测试 PR #827。发布操作更新正文后未写回结果，Runner 封存的是原种子，SteerLine 按 `outcome` 无效阻断。测试 PR 与引用已清理。停机证明再次因状态目录内一条与本场景无关、无维护关联标记的 Unity Run 未结算而保留现场。L1、L2 和定向测试通过，L3 第一次超时且清理确认，独立复验 `verified-report-retry-20261113T043903Z-de385f79` 通过。

```diff
-实施、修复和验证仅在受管工作区内执行。收尾（mode=finish）只推进已有成果，返回 execute 和三级指令，要求 Runner 检查目标分支变化、完成必要验证并回传交付结果。
+实施、修复和验证仅在受管工作区内执行。为这些阶段生成的三级指令只允许本地改动、提交和验证，明确禁止推送、创建 PR 和更新 PR 正文。收尾（mode=finish）只推进已有成果，返回 execute 和三级指令，要求 Runner 检查目标分支变化、完成必要验证并回传交付结果。正文更新留给报告生成后的独立同步阶段。
```

后续一级正文 2666 → 2854 字节。独立复核确认后续实施、修复、验证和收尾的原阶段边界。#827 的越界操作经已保存的执行结果确认属于第二轮实施。

阶段约束后的真实复验 `C:/SteerlineDebug/wt-1851-native-pr-edit/test-results/L4/live-stage-boundaries-20261113T044410Z-7b0599ee/` 完成两轮并在收尾发布 #828。Runner 的封存 base 为 `88b18aff7779206eb924765395ed39c7a5255c17`，独立平台核验为 `88b18aff7779206eb924537395ed39c7a5255c17`。版本审计返回 `accept`，但未提供能覆盖平台版本的执行证据，SteerLine 阻断在报告前。此轮测试清理完整。最终定向测试及 L1、L2、L3 均通过，见 `stage-boundaries-20261113T044510Z-945e2953/`。真实 Codex 已分别验证初次报告原样同步与同一 PR 的实际冲突续作，新版报告同步及最终真实整链尚未通过。

```diff
+-PR 版本变化后的审计以新核验的版本为准。证据覆盖新版本时返回 accept；需要补证或重验时返回 execute，限定在新版本。
+PR 版本变化后的审计以新核验的版本为准。先对照原 PR、head 和 base。三者均匹配且证据覆盖新版本时返回 accept；任何一项不匹配或证据不足时，返回 execute，要求 Runner 按已核验版本补证或重验。
-要求 Runner 回传与最终版本对应的关键差异、验证结果、遗留事项，以及供 SteerLine 核验的 PR 标识和完整 head/base 提交。
+要求 Runner 回传与最终版本对应的关键差异、验证结果、遗留事项，以及供 SteerLine 核验的 PR 标识和完整 head/base 提交。PR URL 和提交值直接取自已核验的命令输出。
+```

后续一级正文 2854 → 3011 字节。独立复核确认原 PR 与版本核验、按既有流程补证及字节差异。

版本精确比对的首次真实复验 `live-exact-version-20261113T045851Z-680d37e3` 在环境准备阶段报 `json: unknown field "exit_code"`，确认清理。独立重试 `live-exact-version-retry-20261113T050122Z-66824869` 完成两轮，收尾指令错误沿用了实施的本地限制，Runner 回传两个可交付提交且没有 PR。场景正确阻断并清理。该规则版本定向测试及 L1、L2、L3 通过，见 `exact-version-20261113T045953Z-9e4acf4d/`。

```diff
-初次交付可在原维护分支发布 PR。
+初次交付有可交付改动时，收尾三级指令必须要求 Runner 推送原维护分支，确认该分支和目标分支尚无原 PR 后创建 draft PR，并回传其 URL。已存在原 PR 时沿用该 PR。
```

后续一级正文 3011 → 3181 字节。独立复核确认初次发布的推送和草稿创建条件，以及原 PR 续作和未知发布对账边界。

单步同步正例使用同一固定 Runner 和真实 Codex。测试 PR #832 的报告文件先在本轮结果文件同目录创建并确认，再由原生工具单独执行 `gh pr edit --body-file`。原生封存回传 `done`；直接宿主对照之前的 GitHub 读回已逐字匹配，包括 CRLF 和尾部空格。原 Run 已结算，测试引用已清理。见 `C:/SteerlineDebug/wt-1851-native-pr-edit/diagnostics/verified-body-path-7c97935a/` 的 `native-sync-proof.json`、`native-command-events.jsonl` 和 `sealed-result.bin`。

真实整链 `C:/SteerlineDebug/wt-1851-native-pr-edit/test-results/L4/live-explicit-publish-20261113T051206Z-1c137969/` 的测试 PR #834 在 head `535260198372c8926fad6914c275c104a927bb51`、base `fa58c8b4e6703a7e9f88567c01a3682858ffd9ce` 上初次原样同步通过，正文 SHA-256 为 `b427ac421ea31e5a6fccbfc3210b4c71afc57b40feebc1372169fb5326560e58`。随后制造真实冲突并触发原 PR 续作，收尾模型返回该阶段不允许的 `accept`，在派发前被后端拒绝。清理通过。

SteerLine 让只读决策的输出 schema 和消费校验共用同一阶段允许值。收尾 schema 只公布 `execute`、`ask`、`blocked`。此项提示词正文变化为 0 字节，独立复核覆盖六种用途和报告独立 schema。定向测试覆盖收尾拒绝 `accept`、原 PR 续作及实际材料；固定 Runner 调用夹具按首轮与后续选题使用对应 schema。

阶段 schema 后的真实整链见 `C:/SteerlineDebug/wt-1851-native-pr-edit/test-results/L4/live-stage-schema-20261113T053951Z-998e1c31/`。首轮通过，第二轮修改和补证通过，Runner 在发出后续审计模型输入前报告 `native_read_only_directory_unavailable`。清理通过，未创建 PR。Runner 的 `verifyContentReadiness` 把原生 RPC 调用错误、非零退出和就绪标记不匹配合为同一诊断，当前证据无法细分。可先做仅带正式文件材料的无目录 `generate/1` 定向复现，记录原生 `command/exec` 回执、退出码和就绪标记匹配结果。若现有输出无法取证，再评审在 Runner 既有 `content-readiness.json` 增加脱敏诊断的最小改动。

最终定向测试及 L1、L2 通过。最终 L3 在等待维护交付时触及场景期限，清理通过。见 `stage-schema-20261113T054111Z-03dba31c/`。先前明确发布动作版本的 L3 两次在浏览器启动步骤未完成，均核实清理，见 `explicit-publish-20261113T051255Z-ff9c7292/` 与 `explicit-publish-retry-20261113T052517Z-3930853d/`。

### 无目录只读预检取证

在 `400e15dcfab24074d0062caeb46d35043a5e09e8` 加本任务未提交变更上，使用原固定 Runner `0.2.0-dev.4-required-result-init` 和真实本机 Codex。以 `go test -overlay <overlay.json> ./internal/runnerclient -run '^TestRealNoDirectoryReadinessProbe$' -count=1 -timeout 11m -v` 提交四个无 `directory`、无 `repository`、仅含两份文件材料的 `generate/1`。第一个直连，后三个通过临时本机 WebSocket 转发器采集原生 JSON-RPC 请求及完整响应，启动真实 Codex 时仅改监听端点。权限、放行条件和 Runner 二进制均沿用原配置。

证据目录为 `C:/SteerlineDebug/wt-1851-readonly-readiness/diagnostics/20261113T055929Z-14b5bcbb/`。四个只读 Run 都获得已核实的原生终态，测试只判定预检。三份 `native-command-*.jsonl` 共保存六次 `command/exec` 原生应答，覆盖线程准备和发出模型输入前两处检查。均无 RPC 错误，退出码为 `0`，`stderr` 为空，`stdout` 去首尾空白后等于本轮 `SR_CONTENT_READY:<command_id>`，耗时约 1.2–2.0 秒。`settlement.json` 核实 Runner 已结算。上次失败没有原生应答留存，定向复验未复现，仍无法把上次失败归到三类中的某一类。

正式 `refactoring.local-codex` 直连复验为 `C:/SteerlineDebug/wt-1851-readonly-readiness/test-results/L4/live-20261113T060220Z-0e1b1287/`。正式执行器过滤了用于采集的可执行文件覆盖变量，Runner 的原预检证明另存于 `diagnostics/live-trace-20261113T060220Z-0e1b1287/direct-runner-readiness/`。本次经过先前失败的无目录审计，测试 PR #838 的初次报告、head `723eae30a2fd990d5609fe0fda2e0a2001b80ba0` 和 base `6931a5b82280acc2c497589a64fa0c2d4940efc2` 经 GitHub 独立核验。夹具在 `alpha.cjs` 和 `beta.cjs` 制造实际冲突。SteerAgent 在冲突收尾只要求复核旧报告正文；Runner 按该指令回传 `blocked`，没有更新代码。见 `maintenance-view.json` 的 `finish` 和 `conflict` 操作、`conflict-proof.txt` 及封存 `result.bin`。清理通过。

只改 SteerLine 的后续一级正文，资产由 3181 增至 3418 UTF-8 字节。以下替换了原收尾段中 `continue_conflict` 的两句，并单独成段。其他收尾规则保持原字节，完整发出正文见本验收随附差异。

```diff
-continue_conflict 为 true 时，以上下文 pr 的当前 head/base 为准，要求 Runner 处理原 PR 的冲突，保留两边有效改动并验证，只更新原 PR 的维护分支。已有 PR 保持待审。
+continue_conflict 为 true 时，本次任务是续作原 PR 的代码冲突。此前的正文同步属于历史。三级指令必须要求 Runner 用 Git 获取最新目标分支提交，在原维护工作区解决冲突并保留双方有效改动，重新验证后只推送原 PR 的维护分支。回传原 PR URL 和实际的新 head/base。原 PR 保持待审。不要把旧报告或正文核对作为本次续作的步骤或放行条件。
```

独立复审通过。实际正文、Spec 和完整差异记录逐字节一致，SHA-256 为 `fb89ac11fa7b22725e8be9bb9ad48b9d3ad12d35cf05ac5500ac8edf73d713cd`。原 PR 核验和维护分支绑定沿原接线，权限及平台放行条件无变更。

本修订的 `go test ./internal/prompts ./internal/refactoring ./internal/steeragent/maintenance -count=1`、正式 L1、L2 和 `runtime.refactoring-maintenance` L3 均通过。正式分级报告位于 `C:/SteerlineDebug/wt-1851-readonly-readiness/test-results/` 下对应级别的 `conflict-instruction-20261113T062419Z-2711d9de/summary.json`。

该版本的正式 L4 复验在同名目录中保存。预检通过，测试 PR #840 的核验 base 是 `8a3ddd6006f8e0eeb77a1747f7d355d38a4f7f3e`。报告正文与 `context.pr.base_revision` 均含正确值，但 SteerAgent 在同步三级指令中抄成 `8a3ddd6006f8e0eeb77a6ce31c3173afc`。Runner 回传实际完整值并暂停同步。`maintenance-view.json` 的 `report` 和 `sync`、相应封存原文保留了这一差异，清理通过。场景同时暴露多轮夹具的文件约束有歧义：本次仅记录第一轮，多个修复操作在同轮中切换文件。只明确夹具所要求的两个独立切片与先后关系，实际两轮、两文件、版本、原 PR、报告原文及清理的断言全部沿用。

报告一级正文由 2358 增至 2559 UTF-8 字节。实际变动如下，完整正文见随附差异。已核验版本使用既有结构化事实，Runner 同步前仍需核对最新平台版本。

```diff
-同步指令要求 Runner 用已认证的 PR 工具核对原 PR 及已核验 head/base。GitHub 使用 gh pr view <PR URL> --json headRefOid,baseRefName 取得 head 和目标分支，再用 gh api repos/<原PR仓库>/git/ref/heads/<目标分支> --jq .object.sha 取得当前 base。版本匹配时，Runner 先解析 STEERLINE_RESULT_FILE，在该文件所在目录将 context.report 原样写入 report.md，使用无 BOM UTF-8，保留换行及尾部空白。确认文件存在并取得绝对路径后，单独执行 gh pr edit <PR URL> --body-file <报告文件绝对路径> 更新原 PR。报告文件随受管工作区清理。版本变化时回传实际 head/base，暂停同步。
+同步指令要求 Runner 从 STEERLINE_RESULT_FILE 的 context.pr 读取 url、repository、head_revision 和 base_revision。同步三级指令只引用这些字段名，不抄写 URL、仓库名或提交值。对 GitHub，使用 gh pr view <context.pr.url> --json headRefOid,baseRefName 取得 head 和目标分支，再用 gh api repos/<context.pr.repository>/git/ref/heads/<目标分支> --jq .object.sha 取得当前 base。逐字比较查询结果与 context.pr.head_revision 和 context.pr.base_revision。版本匹配时，在结果文件所在目录将 context.report 原样写入 report.md，使用无 BOM UTF-8，保留换行及尾部空白。确认文件存在并取得绝对路径后，单独执行 gh pr edit <context.pr.url> --body-file <报告文件绝对路径> 更新原 PR。报告文件随受管工作区清理。版本变化时回传实际 head/base，暂停同步。
```

报告正文变更与真实场景夹具变更均通过独立复审。资产、Spec 和完整差异记录匹配 SHA-256 `5344024544603b86d44e12d570f9aba43ca850ec7a861242c0b1360e855d3dec`。报告数据与已核验事实的原有接线、同一 PR 和正文逐字回读断言继续生效。

最终 `refactoring.local-codex` L4 通过，原报告位于 `C:/SteerlineDebug/wt-1851-readonly-readiness/test-results/L4/sync-context-pr-20261113T065401Z-219aff6e/summary.json`。同目录 `scenarios/refactoring.local-codex/` 中的 `refactoring-live-proof.json` 确认两轮、七次可写执行沿原工作区、单一 draft PR #843、真实冲突、两版源码验证和新版报告原样同步。`conflict-proof.txt` 保存两文件的冲突输出。下表两版均使用原 PR，独立 GitHub 回执中的正文逐字匹配报告。

| 版本 | GitHub 核验 head | GitHub 核验 base | 正文 UTF-8 SHA-256 | 原 GitHub 回执 |
| --- | --- | --- | --- | --- |
| 首次 | `90b046a0febd1b7a03aac8d6985b434f5d8eb724` | `0b934e14b1d178db715972782de39954543e12ea` | `b5d98dd371174bbc037c9784a759e3615d5523f8adb41b0e789a9ff5b317eb7e` | `github-1794554648047414400.json` |
| 冲突续作 | `8586efd4787d23f915b17577627110f5760bf255` | `5ad54d2a8a87b885d2188fcfd64fd19cc9c53032` | `301d6d56e948ca3798f3a6838b2cb7020466494d7a1fb86ad2bcb4441736302a` | `github-1794555138800967500.json` |

最终复验的十五份预检证明均通过，位于 `C:/SteerlineDebug/wt-1851-readonly-readiness/diagnostics/sync-context-pr-20261113T065401Z-219aff6e/direct-runner-readiness/`。#843 已关闭，两条专属引用已删除，Runner 停止结算与场景清理均经核实。该版本提示词定向检查、正式 L1 和 L2 通过，同名 L3 首次等待交付超时且清理通过。

冲突后同步的 Runner 已成功更新 GitHub 正文，但回写结果文件失败，Runner 封存了初始种子。`maintenance-view.json` 最后一个 `sync` 保留了这一诊断，封存文件的 SHA-256 为 `289941df43fa94370dcb2997075cfd18c53eac08fa268785385bae67bc5eb6e3`。SteerLine 依既有对账流程独立读取同一 PR 的新版 head/base 和正文，三项均匹配才标记交付。本次证明真实更新及平台对账，未证明新版同步的 Runner 结果文件成功回写。

同版本的 `runtime.refactoring-maintenance` L3 独立复跑在 `C:/SteerlineDebug/wt-1851-readonly-readiness/test-results/L3/sync-context-pr-retry-20261113T073306Z-9e5be074/summary.json`。仍在等待受控场景交付时达到期限，未通过；清理经核实。前一个提示词版本的 L3 通过记录仍保留，不能替代最终版本的受控结果。
