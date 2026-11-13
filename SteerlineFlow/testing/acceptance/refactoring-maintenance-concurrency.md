# 重构与维护并发与运行环境选择验收

2026-11-12 · 受控功能、候选报告修复和冲突末审 JSON 重试已验收。最新真实联验确认两条可写 Run 重叠及主行动交付，副行动报告登记阻断，未进入原 PR 冲突续作。[Spec](../../spec/refactoring-maintenance-concurrency.spec.md) · [Plan](../../plans/refactoring-maintenance-concurrency.plan.md) · [正式测试选择](../../plans/refactoring-maintenance-concurrency.tests.json)

环境与证据遵循[测试指南](../../testing.md#环境与证据)。受控场景在 Windows 使用独立 home、项目、数据库、Runner/provider 夹具和 Edge。真实场景使用固定独立 Runner EXE、本机 Codex 与自有远端测试分支。实际构建及执行命令在实施时固定。验收从以下最小结果判断，通过记录链接原始证据。

| ID | 方式、时点与步骤 | 最小通过条件 | 状态 |
| --- | --- | --- | --- |
| RMC-A01 | 自动，P2 后。配置就绪、未就绪及不支持维护的环境，同键两方案。切换默认方案、关闭门禁、模拟查询失败和同 ID 配置修改。分别在表单确认后、行动排队中、首个可发送意图保存前后使资格失效或重新检测。另验证目标分支正常前进且环境依据相同或不同。 | 环境分组和定向原因准确。陈旧表单保留草稿并要求确认。先失效则零派发并释放项目和共享占位，先获得授权则沿原输入推进。依据相同时保留冻结提交，依据不同时明确阻断。查询未知时兄弟行动可先推进。阻塞启动资格。 | 已验收，受控及定向 |
| RMC-A02 | 自动，P1–P2 后。用受控屏障让同项目同环境的前三条保持执行，提交第四条，另一项目另发一条。用可控时钟让一条行动先排队 3 分钟、推进 2 分钟、等共享配额 5 分钟、再推进 4 分钟、停止待结算 2 分钟。重放或乱序返回原观察。另设恰好耗尽时间额度的行动。 | 峰值占位为 3，第四条等到原占位释放，第二项目名额独立。占位等待共享容量时暂停预算，停止未结算时保留占位。示例结算后预算用时为 6 分钟、总历时为 16 分钟，重复观察不重复累计。恰好耗尽的行动进入收尾，收尾消耗继续记录。阻塞多行动推进。 | 已验收，受控及定向 |
| RMC-A03 | 自动，P1–P2 后。一条等待人工后答复，一条停止或遇未知回执/观察超时，另一条继续推进。验证未发送取消、已发送停止、重启、迟到受理、关闭排空，以及原 PR 在续作排队中关闭或合并。 | 待答复结算后让位，答复排在已入队行动之后。未发送取消清掉共享登记，已发送未知保留名额且零重发。结算只释放原行动的占位。关闭或合并的原 PR 保留原交付并且本次续作零派发。阻塞生命周期。 | 已验收，受控及定向 |
| RMC-A04 | 自动，P3 后。Edge 宽窄屏用键盘打开侧栏、提交后定位、返回列表、浏览器前进后退、展开历史和打开活动深链。让已选行动完成、翻页后冲突续作换组。关闭侧栏后再返回受理回执，并注入列表或详情单独失败、断线和切项目。 | 换组无重复行，选中详情和历史范围保持。前进后退恢复选择，窄屏返回恢复列表位置。迟到回执更新原行动，不抢焦点或覆盖新草稿。列表失败时已核实的在线详情仍可操作，详情失效时不能用旧状态发命令。刷新和浏览零执行命令。阻塞 Web 交互。 | 已验收，受控 Edge、HTTP 及组件 |
| RMC-A05 | 自动，P4 受控项通过后。固定 Runner 在同项目、同环境同时启动两条有独立改动范围的维护行动，交付后构造一个原 PR 的目标分支冲突并执行续作。 | 公开 Run 时间证明实际执行重叠。核对各自的 worktree、分支、head/base、PR 正文与报告、费用归属。续作复用原身份并更新原 PR，全部自有 Run、服务和测试资源结算。阻塞真实并发交付声明。 | 最新联验确认可写 Run 重叠，#809 已登记首份报告，#810 发布重试后因空候选 PR 绑定阻断。真实双交付未通过，未进入冲突续作，新基线和第二份报告仅在受控场景验证。两个草稿 PR、远端分支和本轮 Runner 已清理 |
| RMC-M01 | 人工，自动交互项通过后。分别用宽屏和窄屏体验新建、容量已满提交、返回原行动、处理待答复和查看交付。 | 能辨认环境与方案的关系、行动排队原因、当前操作目标和各自交付物。体验签收单独登记。 | 待验收，参与人和窗口待安排 |

## 2026-11-11 验证记录

受测为 `40d710fb` 加本工作区改动。报告根为 `C:\SteerlineDebug\wt-317b-refactoring-concurrency\`。各级报告的 `summary.json` 保存实际选择、受测身份和结果。

| 入口 | 结果 | 相对报告根 |
| --- | --- | --- |
| `l1.minimum` | 通过 | `test-results/L1/20261111-192108-796` |
| `smoke.delivery` | 通过 | `test-results/L2/20261111-191309-524` |
| `runtime.service-lifecycle`、`runtime.async-activities`、`runtime.refactoring-maintenance` | 通过 | `test-results/L3/20261111-191309-524` |
| `runtime.refactoring-maintenance` 影响范围复验 | 通过。包含三条 Run 同时执行、第四条等待及接位。宽窄屏截图见场景下的 `browser-start`、`browser-delivered` | `test-results/L3/20261111-192108-796` |
| `go.storage` | 通过 | `test-results/L4/20261111-192354-584-go.storage` |
| `go.environment` | 通过 | `test-results/L4/20261111-192354-584-go.environment` |
| `go.http` | 通过 | `test-results/L4/20261111-192740-849-go.http` |
| 资格失效、并发计时、慢提交、原 PR 关闭及分页的定向测试 | 通过 | `diagnostics/20261111-193032-747-maintenance-edges` |
| Web 生产构建及维护页面、原请求交互组件 | 构建通过，2 个测试文件 16 项通过 | 本轮开发定向输出 |
| `refactoring.local-codex` | 未启动。自动审批要求用户明确授权使用本机登录向 `erranliu/SteerlineTest` 推送本次测试分支和创建 draft PR。真实执行重叠、真实 PR 及清理尚未验收 | 无执行报告 |

## 2026-11-12 真实并发联验

用户明确授权使用本机 Codex 登录，在 `erranliu/SteerlineTest` 推送本次专用测试分支、创建两个 draft PR，并在联验后关闭 PR、删除本次分支。两次运行使用相同的 `40d710fb` 工作区实现及固定 Runner `0.2.0-dev.4-maintenance`（源码 `c404d6e5c50cc32650f9412f9b8b3449c5789e15`，EXE SHA-256 `d83152a9712486025b18304d3496e62f57072d3d51088f3315632f8c24275773`）。

执行入口为 `go run ./cmd/test-feedback -level=L4 -suite=refactoring -env=live -scenario refactoring.local-codex -out <本次独立目录>`。每次的受测构建见报告 `build/frozen-binaries.json`。

| 运行 | 结果 | 相对报告根 |
| --- | --- | --- |
| 首次真实联验 | 未通过。公开 Run 证据确认两条只读决策实际重叠。主行动通过首轮实现和审计，在第二次选题时收到找不到冻结材料的阻断结论。材料清单记录了独立输入目录。无 PR 创建，场景和 Runner 清理均确认完成。 | `test-results/L4/20261112-102507-378-refactoring.local-codex` |
| 真实联验补测 | 未通过。北京时间 10:36:31，公开 Runner A–B–A 采样确认两条原生 Run 同时处于执行中。两条行动各自产生 draft PR。副行动 #799 已交付一份报告。主行动 #800 完成两轮改动并发布，PR 正文与候选报告不一致后进入修复，末审因缺少报告原文进入待答复，场景在冲突续作前退出。场景和 Runner 清理均确认完成。 | `test-results/L4/20261112-103349-008-refactoring.local-codex-recheck` |

补测的 `maintenance-view.json` 保留了 #800 发布、修复和末审。末审要求提供 `report_candidate.report`，修复操作也报告上下文中缺少该字段。该次受测实现生成修复操作时清空候选报告，只在发布或冲突操作成功时重新保存候选。此次未形成主行动已核验报告，双行动的完整交付与费用对账、原 PR 冲突续作仍未验收。

独立 GitHub 复查确认 [#799](https://github.com/erranliu/SteerlineTest/pull/799) 和 [#800](https://github.com/erranliu/SteerlineTest/pull/800) 均为已关闭、未合并的 draft PR。首次与补测的两个基线分支，以及两个 PR 的 head 分支均已删除。核验见补测场景下的 `remote-cleanup-verified.json`，本机结算见两次的 `scenario.json` 与 `maintenance-live-runner-cleanup.json`。

## 2026-11-12 候选报告正文修复

修复及末审沿用发布操作封存的候选报告、原 PR、head/base 与费用快照。只读审计给出修复决定时保留候选。修复确实改变工作区时重新发布，工作区无法核实或 PR 正文仍与原文不一致时阻断交付。固定提示词正文与材料组装入口沿原实现，修复上下文使用既有 `report_candidate` 字段传递已保存的报告原文。

| 运行 | 结果 | 相对报告根 |
| --- | --- | --- |
| 维护报告修复及 PR 来源定向测试 | 通过。覆盖候选报告逐字传递到修复与末审、费用快照、改动 HEAD、未知工作区和末审不能放行正文不一致。 | `diagnostics/20261112-111903-396-report-repair` |
| `runtime.refactoring-maintenance` | 通过。夹具先写入与封存报告不一致的 PR 正文。可写修复沿原工作区把原文写回原 PR，末审再次读取同一份报告。最终只交付一份对应报告，再走受控原 PR 冲突续作。 | `test-results/L3/20261112-111255-245-report-repair` |
| `l1.minimum` | 通过。 | `test-results/L1/20261112-111903-396-report-repair` |
| `smoke.delivery` | 通过。 | `test-results/L2/20261112-111903-396-report-repair` |
| `refactoring.local-codex` | 自动验收未通过。真实执行重叠，主行动完成两轮并交付 #803。副行动创建 #802，但结果的 PR URL 附带状态说明，平台查询拒绝。通过公开「立即停止」「结束并交付」命令恢复一次，仍返回不合格 URL，双交付检查等待到期。两条行动的完整交付及费用对账未通过。 | `test-results/L4/20261112-111836-471-report-repair-live` |
| #803 真实冲突续作补验 | 经人工协助核实。续作用原 PR、原分支、原工作区及写者链，保存两版报告。复核确认真实 Git 冲突解决后原简化和基线改动均保留，`node check.cjs` 通过，GitHub 正文与第二份报告逐字一致。 | 同上，场景中的 `supplement-conflict-result.json` |

真实补测沿用本节前述固定 Runner 和 L4 入口，受测为 `40d710fb` 加本工作区修复。公开 Run 重叠见 `maintenance-native-overlap.json`。副行动的公开恢复命令回执见 `peer-public-recovery.log`。自动场景因副行动核验等待超时退出，原始失败保留于 `summary.json` 与 `e2e-1.raw.log`。

补验在同一专用基线上构造了 `git merge-tree` 可复现的冲突，并通过公开 `refactoring_conflict` 命令续作 #803。末审接受后，平台正文仍与封存报告不一致，SteerLine 保持核验状态。核实同一 PR、分支、提交及已封存结果后，人工将候选报告原文写回测试 PR，应用随后按原校验进入已交付。正文变更前后 SHA-256 与封存原文对照见 `supplement-report-body-recovery.json`，实际冲突、公开命令及代码检查见 `supplement-conflict-proof.txt`、`supplement-conflict-command.log` 与 `supplement-conflict-tree-check.log`。该结果独立于自动整链状态。

独立复查确认 [#802](https://github.com/erranliu/SteerlineTest/pull/802) 和 [#803](https://github.com/erranliu/SteerlineTest/pull/803) 均已关闭且未合并。本次基线和两个 head 分支已删除，本机 Runner 已结算。复查见 `remote-cleanup-verified.json`，场景清理见 `scenario.json` 和 `maintenance-live-runner-cleanup.json`。

## 2026-11-12 更新主线后的复验

PR 分支更新至 `main` 的 `805cfcae`，SQLite schema 提升到 49，同时包含项目并发名额索引及主线的 Roadmap 规划表。受控场景核对只读决策使用原项目目录。真实并发和冲突续作证据沿用上节的受测版本，本轮未重新运行真实场景。

| 入口 | 受测提交及结果 | 相对报告根 |
| --- | --- | --- |
| `./internal/refactoring ./internal/storage/sqlite ./internal/app -run 'Refactoring\|Maintenance' -count=1` | `350bd505`，53 项通过 | `diagnostics/20261112-115929-474-pr-rebase` |
| `l1.minimum` | `350bd505`，通过 | `test-results/L1/20261112-115929-474-pr-rebase` |
| `smoke.delivery` | `350bd505`，通过并确认清理 | `test-results/L2/20261112-115929-474-pr-rebase` |
| `runtime.refactoring-maintenance` | `350bd505`，浏览器在受理回执返回前已开始等待「待答复」，该断言超时 | `test-results/L3/20261112-115929-474-pr-rebase` |
| `runtime.refactoring-maintenance` | `ca229362`，改为先等受理回执，两个浏览器阶段通过，场景在提交并发组时耗尽 5 分钟时限，已确认清理 | `test-results/L3/20261112-121435-848-pr-receipt-recheck` |
| `runtime.refactoring-maintenance` | `ac1a79b7`，延长该场景的测试时限，遇周期性基线检查拒绝并发组提交，已确认清理 | `test-results/L3/20261112-122137-845-pr-deadline-recheck` |
| `runtime.refactoring-maintenance` | `1364c180`，通过。核对两轮交付、候选报告正文修复和末审、原 PR 冲突续作、宽窄屏浏览器，以及三条 Run 重叠、第四条排队后接位。已确认清理 | `test-results/L3/20261112-122915-874-pr-baseline-recheck` |

受控并发组按顺序提交四条行动，每次从公开查询绑定当前环境与基线。若周期性检查导致明确拒绝，测试重新读取并确认后再提交。排队、停止后接位与资源清理仍由同一场景检查。

## 2026-11-12 PR URL 状态说明修复与真实复验

维护发布和冲突续作保留 Runner 封存的原始结果。使用尾随状态说明前的 URL 构建候选并查询平台，含第二个绝对 URL 的值由严格校验拒绝。平台继续核对目标仓库、分支、原 PR、head 和报告正文。受测版本为 [#515](https://github.com/erranliu/SteerLine/pull/515) 的干净提交 `0132c5b7a778334f5a98d86babde8572ba4af6d3`。

| 入口 | 结果 | 相对报告根 |
| --- | --- | --- |
| 维护 PR 核验及候选报告定向测试 | 23 项通过。使用本地 HTTP 验证带状态说明的 URL、外仓和多个 URL，以及原有 head、正文和关闭状态检查 | `diagnostics/20261112-124535-751-pr-url` |
| `runtime.refactoring-maintenance` | 通过。发布与续作夹具返回尾随 `(draft; open)` 的 URL，核对原始结果保留、交付使用平台 URL、报告修复和末审，以及原 PR 冲突续作。已确认清理 | `test-results/L3/20261112-124535-751-pr-url` |
| `refactoring.local-codex` | 自动验收未通过。12:55:31（北京时间）公开 Runner 采样确认两条原生 Run 重叠。主行动完成 `alpha.cjs` 首轮，随后两次只读审计均返回 `human_input_required`，在双交付和冲突续作前退出。该次创建了一个 draft PR，未形成双交付或费用对账证据 | `test-results/L4/20261112-125222-283-pr-url-live` |

真实入口沿用本页固定 Runner 和 `refactoring.local-codex` 命令。重叠证据见 `maintenance-native-overlap.json`，阻断见 `maintenance-view.json` 和 `e2e-1.raw.log`。独立核实 [#804](https://github.com/erranliu/SteerlineTest/pull/804) 已关闭且未合并，专用基线和唯一 head 分支已删除。证据见 `remote-cleanup-verified.json`、`scenario.json` 和 `maintenance-live-runner-cleanup.json`。

## 2026-11-12 只读审计材料路径修复与真实补验

固定 Runner 的 `human_input_required` 表示原生提问结束了 `generate/1`。上轮的原问题未保留，无法确认两次审计的具体提问。在隔离本机探针中，按上轮首轮实施证据重建审计请求。旧指令只列逻辑名称，模型在原项目目录找不到材料，返回结构化 `ask`。改用 Runner 清单中的绝对 `file` 路径后，同一审计证据得到 `accept`。缺少材料应返回 `blocked` 并给出读取错误，实际用户取舍继续使用结构化 `ask`。

`maintenance-decision.md` 的渲染正文由 251 增至 509 个 UTF-8 字节，实际正文差异及独立审核记录见 [#515](https://github.com/erranliu/SteerLine/pull/515)。固定规则和两份材料仍通过 Runner 冻结文件交付。联验在清理前保存两条行动视图，并按原行动、Run、命令、会话和轮次保留失败提问。

| 入口 | 结果 | 相对报告根 |
| --- | --- | --- |
| 同一首轮审计的本机对照 | 旧指令返回找不到材料的 `ask`，修订指令返回 `accept`。公开 `audit.json` 保留材料清单、决策及原生来源 | `diagnostics/20261112-132326-native-audit-repro`、`diagnostics/20261112-132502-native-audit-fixed` |
| 提示词与维护定向测试 | `go test ./internal/prompts -count=1 -v` 及维护的实际文件交接、只读失败限次重试测试通过 | 本轮定向输出 |
| `l1.minimum`、`smoke.delivery` | 干净提交 `0e788034`，均通过。后续提交只收紧 live 失败取证 | `test-results/L1/20261112-132636-435-audit-materials`、`test-results/L2/20261112-132636-435-audit-materials` |
| `runtime.refactoring-maintenance` | 干净提交 `c2393697`，受控整链和清理通过 | `test-results/L3/20261112-132636-435-audit-materials` |
| `refactoring.local-codex` | 干净提交 `c2393697`，固定 Runner。自动场景确认重叠，原主行动在 `prepare_session` 连接关闭后未能对账，等候交付超时。两次替代尝试使用同一专用基线。第一条因转录路径中的一个字符而明确阻断，第二条通过两轮工程审计并发布 #806 | `test-results/L4/20261112-133351-748-audit-materials-live` |

真实补验使用原自动场景仍在运行的隔离项目和公开操作入口。13:58:40（北京时间）的 A–B–A 采样确认 #806 的报告修复与 #805 的续作决策实际重叠。`git merge-tree` 证明专用基线与 #805 存在真实冲突。#805 复用原 PR、分支和 worktree，保留双方改动，`node check.cjs` 通过，GitHub 正文与第二份交付报告逐字一致。证据见 `supplement-native-overlap.json`、`supplement-peer-conflict-proof.txt` 和 `supplement-peer-conflict-verified.json`。

#806 的末审接受修正冻结费用说明后的报告。GitHub 正文与修复结果逐字一致，应用仍核验修复前的候选报告，因正文不同停在核验阶段。通过同一行动重新收尾后，新的末审发现 #805 补验已推进基线，而重新发布的报告仍引用旧基线。场景截止时 #806 未登记交付，本轮自动双交付未通过。证据见 `supplement-primary-verification-stall.json`、`supplement-primary-repair-platform.json` 和 `supplement-primary-latest-view.json`。

独立确认 [#805](https://github.com/erranliu/SteerlineTest/pull/805) 和 [#806](https://github.com/erranliu/SteerlineTest/pull/806) 均已关闭且未合并，专用基线及两个 head 分支均已删除，见 `remote-cleanup-verified.json`。隔离应用和 Runner 服务已退出，临时登录已移除。原主行动的 Run `6f8d8abcb0482e637abedb41c8d82e7e` 的原停止回执仍为 `unknown`，公开离线检查无法证明原生写者已退出。按原流程保留隔离现场，见 `supplement-original-stop-requests.json`、`supplement-runner-offline-inventory.json` 和 `supplement-runner-cleanup-recheck.log`。

## 2026-11-12 更正报告、重新收尾基线和真实双交付

同一 PR、head、base 的更正报告须由原可写 Run 封存，且工作区 HEAD 已核实为干净。满足这些条件时，末审及平台逐字核验使用更正报告，仍采用该次发布的冻结费用。重新收尾读取原冻结远端和分支的当前目标提交，保持行动的起始提交及原 worktree 复用身份。报告基线在末审前和登记交付前复核。过期报告保留原 PR 和已交付记录，行动可以重新收尾。

固定规则 `materials/maintenance.md` 的 UTF-8 正文由 5161 增至 5586 字节，真实复验后的续作规则调整至 5812 字节。独立审核核对了正文、摘要、目标提交和原 PR 接线。实际差异见 [#515](https://github.com/erranliu/SteerLine/pull/515)。

| 入口 | 结果 | 相对报告根 |
| --- | --- | --- |
| 定向测试、L1、L2、维护 L3 | `5fd7b1dd`，48 项定向测试及三级测试通过，覆盖更正报告和旧基线重新收尾 | `diagnostics/20261112-142819-706-closure-recheck`、`test-results/{L1,L2,L3}/20261112-142819-706-closure-recheck` |
| `refactoring.local-codex` | `5fd7b1dd`，真实双交付完成，场景冲突续作未通过。Runner 和远端资源已清理 | `test-results/L4/20261112-143845-103-closure-recheck-live` |
| 最终定向测试及 L1 | `db4d5c60`，40 项定向测试及 L1 通过。覆盖旧基线在末审前阻断、原 PR 重新收尾和更正报告核验 | `diagnostics/20261112-151454-013-final-closing-base`、`test-results/L1/20261112-151454-013-final-closing-base` |
| 最终 L2 | `db4d5c60`，通过并确认清理 | `test-results/L2/20261112-150811-951-conflict-base-guard` |
| 最终维护 L3 | `db4d5c60`，受控整链通过并确认清理 | `test-results/L3/20261112-150811-951-conflict-base-guard` |

真实场景使用本页固定 Runner 和 `refactoring.local-codex` 命令。北京时间 14:41:36 确认只读 Run 重叠，14:48:56 补证两条原行动的可写 Run 重叠，分别见 `maintenance-native-overlap.json` 和 `maintenance-native-write-overlap.json`。副行动初次返回无效的重复收尾决定，通过公开 `refactoring_finish` 对同一行动重试一次。两条原行动分别在 [#807](https://github.com/erranliu/SteerlineTest/pull/807) 和 [#808](https://github.com/erranliu/SteerlineTest/pull/808) 登记交付，平台报告及 Git 产物通过场景检查。

场景在双交付后推进专用基线，`git merge-tree` 证实 `alpha.cjs` 和 `beta.cjs` 均存在内容冲突，并提交 #808 原 PR 续作。可写结果仍使用起始基线 `95b7d87df65532ec2247702d5f3e5a5268cd4e60`，本次目标为 `9b8ad2325ab5ba63c7782f9690eceafde31016af`。两次末审指出差异，返回的决策都含 JSON 对象外的额外内容，未通过 Runner 的 JSON 校验。#808 保留首份交付，第二份报告未登记。新增防护在末审前复核当前基线，规则明确要求在原分支同步目标提交并验证它是交付 head 的祖先。这些追加修正以最终受控回归验证。

独立核实 #807、#808 均已关闭且未合并，专用基线及两个 head 分支已删除。场景确认本轮 Runner 结算，见 `remote-cleanup-verified.json`、`scenario.json` 和 `maintenance-live-runner-cleanup.json`。上一轮原 Run `6f8d8abcb0482e637abedb41c8d82e7e` 的 `prepare_session` 在连接关闭后未取得与原操作匹配的回执，随后原停止请求已受理，结果仍为 `unknown`。同一固定 Runner 的离线复查返回服务已退出且原生写者退出未核实。上一轮隔离现场继续保留。

## 2026-11-12 冲突末审 JSON 重试

#808 的两份冲突末审输出都在长 `prompt` 字段后提前闭合对象，接着在对象外写入 `summary` 和 `question`。Runner 将两份输出判为 `invalid_json` 并封存原文。受控回归在修复前复现原文被再次送入重试上下文。修复后，原行动的同一轮同一阶段最多重新生成一次。重试保留 Runner 诊断、原 PR、候选报告及本次目标提交，从模型输入移除畸形原文，历史中仍可核对原始输出。两次均无效时，行动阻断并保留首份交付。

`maintenance-decision.md` 的常规渲染正文由 509 增至 722 个 UTF-8 字节，重试正文为 901 字节。完整示例将 `prompt` 放在最后，重试段要求重新读取冻结材料并输出完整对象。Runner 仍执行原有严格 JSON 校验。独立审核核对了常规和重试正文、材料组装及候选传递。[实际正文差异](https://github.com/erranliu/SteerLine/commit/3e6afda6d1a59819361be7aa63c38b5da0993ac6)

| 入口 | 结果 | 相对报告根 |
| --- | --- | --- |
| 提示词黄金样本、维护定向测试 | `3e6afda6`，黄金样本通过，45 项定向测试通过。覆盖有效重试登记新基线和第二份报告，以及连续两次无效时保留原 PR 和首份报告 | `diagnostics/20261112-152836-768-audit-json-retry` |
| `l1.minimum`、`smoke.delivery` | `3e6afda6`，均通过 | `test-results/{L1,L2}/20261112-152836-768-audit-json-retry` |
| `runtime.refactoring-maintenance` | `3e6afda6`，通过并确认清理。原 PR 冲突末审注入畸形 JSON，操作历史保留原文，有效重试登记第二份报告 | `test-results/L3/20261112-152836-768-audit-json-retry` |
| `refactoring.local-codex` | `3e6afda6`，固定 Runner。两条行动的可写 Run 重叠。主行动两轮后完成交付。副行动发布重试创建了草稿 PR，应用在更正报告绑定校验阻断。场景未进入原 PR 冲突续作 | `test-results/L4/20261112-153814-199-audit-json-live` |

北京时间 15:40:25 和 15:47:22 的公开 Runner A–B–A 采样分别证实只读和可写 Run 重叠，见 `maintenance-native-overlap.json`、`maintenance-native-write-overlap.json`。主行动在 [#809](https://github.com/erranliu/SteerlineTest/pull/809) 登记首份报告，场景核对了正文、head、起始基线 `34c14eee8d605fdab712c124452ded200397e257` 和改动范围，交付树的 `node check.cjs` 通过。证据见 `maintenance-view.json`、`initial-report.json` 和 `initial-tree-check.log`。

副行动首轮发布报告称创建命令被执行策略拒绝，封存结果为 `blocked` 且 PR 字段为空。确认本轮基线上暂无 PR 后，通过公开 `refactoring_answer` 同意在原分支重试。副行动随后创建 [#810](https://github.com/erranliu/SteerlineTest/pull/810)，更正正文并最终封存新结果。两次封存的 head/base 相同，但更正结果中的 #810 与初始候选的空 PR 字段不匹配，应用以「更正报告未绑定原PR及候选head/base」阻断。证据见 `peer-before-public-answer.json`、`peer-public-answer-receipt.json`、`maintenance-peer-view.json` 和 `e2e-1.raw.log`。双交付检查中止，专用基线未推进，本轮未登记新基线或第二份报告。

独立复查确认 #809、#810 均为已关闭、未合并的草稿 PR，专用基线和两个 head 分支均已删除。本轮 Runner 清理已确认。证据见 `remote-cleanup-verified.json`、`scenario.json` 和 `maintenance-live-runner-cleanup.json`。
