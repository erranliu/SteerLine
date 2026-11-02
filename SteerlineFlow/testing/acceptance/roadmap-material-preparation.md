# Roadmap 材料自动准备验收

归属：[Spec](../../spec/roadmap-material-preparation.spec.md)、[Plan](../../plans/roadmap-material-preparation.plan.md)。2026-11-02 用户授权。承接 LF/CRLF 修复；本记录不改写原历史结果。证据根为仓库外 `C:\SteerlineDebug\wt-762b-roadmap-materials`，下表使用其相对路径。

| ID | 范围 | 状态与证据 |
| --- | --- | --- |
| RM-A01 | 静态准备命令与安全/幂等检查 | 通过；`diagnostics/citation-boundary-20261102T155353865Z/summary.json`，53 项含子例、零 skip。含真实 Windows junction，目录外零写入/零枚举、只读检查和自定义指南保留。 |
| RM-A02 | 自动冻结动态材料、正式文件渠道与独立 manifest | 通过；`diagnostics/citation-owner-20261102T155208294Z/summary.json`，69 项含子例、零用例 skip；model 包无测试仅编译。实际 Git/SQLite 发布与 CRLF checkout 流程不准备动态 JSON。 |
| RM-A03 | 原请求/缺能力/漂移/路径/manifest 反例 | 通过；上两份报告及 `diagnostics/client-final-20261102T155622482Z/summary.json`。Observe 层同时篡改 Run/result 的代码文件 SHA/大小仍被原 Git 字节校验拒绝；缺文件/读取失败/改字节拒绝。Runner 对应定向报告在下文登记。 |
| RM-A04 | 真实 Runner/Codex + 隔离 Git/provider/SQLite 草稿转 DAG、正常停止清理 | 通过；`diagnostics/live-platform-digests-20261102T155125711Z/summary.json`，229.971s，真实两次 generate、6 份文档、2 个正式任务/1 条依赖、实际隔离 Git 发布合并和 SQLite 入图；同目录 evidence 保存原请求、Run 观察、完整正文及资源结算。 |

## 真实读取与版本

SteerLine 受测基线 `7d85a9e908102a4f12210bbba91408f39362e6a7` 加本轮工作树；各 summary 保存完整 dirty 清单和命令。Runner 独立源码基线 `66cc0acf50b675c75458d9d6281139e16158a3a3` 加本轮修改；原 `C:\Project\SteerRunner` 未修改。受测 EXE 为 `bin/steerrunner-verified.exe`，SHA-256 `df27f15bc61767edcfdd6713b6572d5747d4889cfccaace8846d1dcef3dba9ec`，resource digest `a968df6ee06ae6a16650ed9f0eee3ab878e9406a4541ab44fa6548674161c707`；同目录 `.exe.build.json` 固定生产源码摘要 `da646a2baebf1e1d3120c272cf139c3bd3d190fa5e9c6e6b9e4c38ed1ae91f88`。

正式入口：`go run ./cmd/test-feedback -out <独立目录> -- -tags=e2e ./internal/app -run '^TestRoadmapLocalCodexPromotion$' -count=1 -timeout=20m`；设置上述 `STEERRUNNER_EXE` 与本轮 `STEERLINE_ROADMAP_EVIDENCE`。本机 Codex 使用原正式只读 generate，不替换模型。固定源码 commit=`388de15b257459f0134baba086775f1735520dba`，草稿在提交后仅工作树改为随机 `SumInput51fbe397`；解析 evidence 和生成正文实际含该标记，`source.modified=true`，证明采用本次上传文件而非 Git 旧草稿。

解析 Run=`7cfda41580f5cc643a07b91a6674b24e`，文档 Run=`1b83695dc0d5edf65ae68403a1ccbbbc`，均 completed/stopped。文档发布 merge commit=`b342d92882345e5bea17a0eb3d4b5301c2e1ac63`；`admitted.json` 的 formal_mapping 为 T1→0、T2→1，依赖 0→1，重放不重复。`runner-settlement.json` 为公开正常停止及核验后 `resources_settled=true`。此 PR provider 为隔离本机 HTTP，实际使用 Git 分支及合并；不将其描述为真实 GitHub/Gitea 外部发布或浏览器 E2E。自动派发关闭。

## 提示正文与独立审核

下表按 UTF-8、canonical LF、含模板末尾换行统计，不修改原请求或文件摘要。实际 diff 保留于对应文件，golden 只核对渲染；两个 golden 去除原先误加的末尾换行，匹配既有 renderText 行为。

| 唯一正文 | 字节前→后 | 实际变更与业务原因 |
| --- | --- | --- |
| internal/prompts/roadmap-parse.md | 253→364 | 原“固定提交的草稿/图/映射”改为“本次固定文件 roadmap-inputs/source.md、existing-task-graph.json、existing-roadmap-source-map.json”；规则仍从固定 Git 读取，明确使用 Runner file 路径。 |
| internal/prompts/roadmap-documents.md | 398→511 | 原“固定 Git 的草稿/confirmed-planning.json”改为本次文件材料；保留同一固定提交的静态规范与代码调查，原草稿路径仅作来源说明。 |
| internal/prompts/materials/roadmap-documents.md | 1363→1676 | 原文/确认以文件材料为准；新增 repository/input_file 来源区分；原“investigation.sha256 使用固定快照内容摘要”改为“仅返回 source、path、reason；文件摘要由平台从固定文件核验并填写，不由模型计算或抄写”。其余禁止实现/改规划、完整正文、问题阻断与上限规则保持。 |
| internal/prompts/materials/roadmap-spec-guide.md | 0→4250 | 打包现有 SpecGuide 作为缺失时的初始化默认文件；已有项目指南保留，不作为运行时兜底。 |

最终完整规则 SHA-256=`4f04f38c14ba54d90d6ebb0000f6a21652a5dd224afd074efd9a10a909ff827e`；默认指南=`140acc1062a7838474a11947b55087c8c5194a18ec0be9cd5c8c313db92f230b`。解析完整规则正文未改。Runner 的既有固定 frame 正文不变，公开 materials 描述增加 file 类型；file 内容不展开进提示词。业务接线、实际正文和摘要由独立只读审核 `review_roadmap_newlines` 审阅；审核要求补来源类型展示、真实 junction 与原 Git 引用反例，修复补验后复审无剩余发现，未以 golden 代替批准。

## 回归与保留的失败

- SteerLine 固定 L1：`test-results/L1/final-20261102T155509473Z/summary.json`、固定 L2：`test-results/L2/final-20261102T155508401Z/summary.json`、前端全集 web.unit：`test-results/web/20261102T072952422Z-96a458fe/summary.json` 均通过。前端生产构建通过；文档链接 648 文件/4058 链接零错误，diff 检查通过。
- Runner 最终 owner：`diagnostics/runner-owner-current-20261102T155337968Z/go-test.jsonl` 通过，含正式材料原字节/未内联、冻结后缺失/篡改不重建、Windows junction 首次准备及同内容恢复拒绝、原 execute 冷停止。
- Runner 完整 L1：`test-results/runner-L1-20261102T072435495Z/summary.json` 保持失败；framework、native compile/零匹配守卫通过。失败为既有 workflow boundary 引用已不存在的 conversation 目录、resume 缺冻结 budget、Orca sample-feed 时间断言。前两项在未修改基线分别由 `diagnostics/runner-baseline-boundary-20261102T155221618Z` 和 `diagnostics/runner-baseline-20261102T074257673Z` 复现；Orca 同次基线定向复跑通过，仅记录不稳定失败，不把它改写为通过。未放宽原 L1，也未把本专项扩大成 Runner 全量 L2/发布资格。
- 真实链失败均保留：首次旧调用方不接受 input_file 引用；随后提前采用 settling 结果导致确认后无法生成文档；最后模型抄错 testing.md 一位摘要。分别通过显式来源类型、等待 completed/stopped、平台读取原 Git 并填写摘要修复，均未重发未知请求或放宽固定来源检查。最终新现场完整通过。
- 初次本项目 L1 的 golden 尾换行和嵌套 Runner clone 源码扫描失败已保留；修正 golden 末尾换行及将 clone 移出源码树，没有改检查器。普通沙箱下 junction/Git 权限失败不算业务失败或通过，实际具备临时目录权限的独立复验见上述报告。

用户与项目无需手写动态 JSON。安装版尚未部署；真实外部 PR、浏览器整链及后续业务实现不在本轮通过范围内。

## PR 与最新主分支同步

实现提交 `3d10e4c9`，配套 Runner 提交 `97680b81`；已创建 [SteerLine #430](https://github.com/erranliu/SteerLine/pull/430) 和 [Runner #89](https://github.com/erranliu/SteerRunner/pull/89)，两者保持草稿，等待配套能力及 Runner 既有完整 L1 问题处理。未合并或部署。

SteerLine 同步 `origin/main=fd84478926bea32fbfa687ece56e931470924faf`，仅验收索引冲突，保留双方条目；合并提交 `3b3170361d6560724a4c73a4b67d367c49ddc3c1` 后固定 L1、L2、web.unit 全部通过，报告分别为 `test-results/L1/pr-main-20261102T160211942Z/summary.json`、`test-results/L2/pr-main-20261102T160207900Z/summary.json`、`test-results/web/pr-main-20261102T160234750Z/summary.json`。文档链接为 649 文件/4061 链接零错误。真实 Runner/Codex 链是同步前上述固定工作树的结果，未将其标为同步后重跑。

## 2026-11-02 后续：Runner L1 修复与配套复验

用户追加授权修复 Runner L1，验证通过后把两 PR 转正式。Runner 修复提交 `14eba14`：依赖守卫扫描全部实际 execution 目录，报告恢复夹具使用正式 FreezeInput 补齐原预算并核对恢复时 Prompt/Budget 不变，样本流测试等待墙钟严格晚于两次原接收时间并核对 Drain 未重打时间戳；未修改生产预算准入和接收代码，原夹具正文 UTF-8 17→17 字节。定向 22 项含子例及样本流重复 100 次通过、零 fail/skip，独立只读审核无发现。

同步 Runner main=`761f916af352d22f51eab4b5967f9a390eeebd17` 后干净提交 `06a9214ad54dcaa9283368103de80be1c73e602f` 的完整 L1 在三项原失败通过、734 项顶层通过后仍因 application 包旧 12 分钟总上限 panic，保留 `test-results/runner-L1-repaired-20261102T161644437Z/summary.json` 为失败。聚合预算随后调整为 20 分钟，不改变测试集合、skip、断言或用例/产品期限；完整入口复验完成后在本节追加，不以定向通过冒充完整通过。

本次 SteerLine 精确提交 `7355fc5e33dc26cee11c6f7f7402cd828fa1db03` 配合上述新 Runner 构建重跑真实链，EXE SHA-256=`41de9144c4c999298628e24511f24767f5f3cb5d2959b23edfd80fc55b35be08`，source digest=`30dc64bec16156fdb0e62af9b5911d4c21392aa5bccf1789e44d7a8f2cba1463`，resource digest 不变。`diagnostics/live-runner-l1-repaired-20261102T161722633Z` 被模型抄错 confirmed_digest 的 schema 校验拒绝、零采用，原 Run stopped/resources_settled=true；不改写失败。独立新现场 `diagnostics/live-runner-l1-repaired-confirmation-20261102T162140859Z/summary.json` 通过（243.930s）：工作树草稿实际读取、六份完整文档、隔离 Git/provider 实际发布合并、SQLite T1→0/T2→1 和 0→1 依赖、重放幂等与正常停止均通过。

新现场解析 Run=`9a77997f5308f652d31e58ecc24ef4bf`，文档 Run=`4553f250a32aca1c5193c531974302b5`，均 completed/stopped；固定源码 commit=`891d670882f6c0e78584c227fc1c35f1990ac309`，merge commit=`1653291aa1e80b33bc6315b499b4e29a4189f176`；同目录 `evidence/runner-settlement.json` 公开 resources_settled=true。没有重发原请求或放宽 schema 校验。此后 Runner 只改测试/验收，不将该真实链误标为新生产行为。SteerLine 原精确 head 的远端 Prompt governance CI 已通过；最终 PR 资格以本轮完整报告及公开回执为准，安装版仍未部署。

20 分钟聚合预算的第二轮完整 L1 又暴露原 `TestOrcaPreSendReconnect/provider-query-exhausted` 的 20ms 调度竞争（首次连接前耗尽，opens=0）。`test-results/runner-L1-complete-20261102T163508518Z` 保留为失败；核实唯一报告路径、父 Go 进程和精确 EXE/PID 后终止该已知失败轮剩余 application 测试，`operator-abort.json` 保存身份与原因，不计通过。Runner 提交 `b11219e` 只修测试：保留正常 3s readiness 预算，在原连接/查询回调等待同一个真实 deadline，新增 DeadlineExceeded/原查询一次断言；取消在首次实际连接中触发，核验 Canceled。原零发送、一次打开及禁止重放断言保留。重连全表和取消重复 10 轮、150 个父/子 pass 事件、零 fail/skip，见 `diagnostics/reconnect-deadline-repair-20261102T164422437Z/go-test.jsonl`；独立审核通过。

最终 Runner 干净提交 `b11219e8fc9bfa5dc357202e58aded5aa4591df7` 的完整正式 L1 **通过**：`test-results/runner-L1-final-20261102T164640208Z/summary.json` 为 status=passed、validation_scope=complete、exit_code=0；738 个顶层测试、36 个包通过、零失败事件，29 项终端扩展、framework、native driver compile/零匹配守卫全部通过。16 项既有显式原生 opt-in/helper skip 沿用原白名单、不计原生通过，零意外跳过。总耗时 1028.197s；application 实际 771.223s，原中断处审计用例为9.26s，20分钟仅是整包聚合预算。此结果更新前文因 Runner L1 暂缓转正式的结论；各次原始失败不改写。

按用户授权补齐两仓验收记录后，将 [Runner #89](https://github.com/erranliu/SteerRunner/pull/89) 和 [SteerLine #430](https://github.com/erranliu/SteerLine/pull/430) 转入正式审阅，最终状态以公开PR回执核对。本轮 SteerLine 只有验收文档更新；既有固定 L1/L2/web.unit 和新组合真实链的精确受测版本保持上述记录，不为纯结果登记重复运行模型。安装版未部署。
