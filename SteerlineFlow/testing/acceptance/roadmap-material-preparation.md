# Roadmap 材料自动准备验收

归属：[Spec](../../spec/roadmap-material-preparation.spec.md)、[Plan](../../plans/roadmap-material-preparation.plan.md)。2026-11-02 用户授权。承接 LF/CRLF 修复；本记录不改写原历史结果。证据根为仓库外 `C:\SteerlineDebug\wt-762b-roadmap-materials`，下表使用其相对路径。

| ID | 范围 | 状态与证据 |
| --- | --- | --- |
| RM-A01 | 静态准备命令与安全/幂等检查 | 通过；`diagnostics/citation-boundary-20261102T155353865Z/summary.json`，53 项含子例、零 skip。含真实 Windows junction，目录外零写入/零枚举、只读检查和自定义指南保留。 |
| RM-A02 | 自动冻结动态材料、正式文件渠道与独立 manifest | 通过；`diagnostics/citation-owner-20261102T155208294Z/summary.json`，69 项含子例、零用例 skip；model 包无测试仅编译。实际 Git/SQLite 发布与 CRLF checkout 流程不准备动态 JSON。 |
| RM-A03 | 原请求/缺能力/漂移/路径/manifest 反例 | 通过；上两份报告及 `diagnostics/client-final-20261102T155622482Z/summary.json`。Observe 层同时篡改 Run/result 的代码文件 SHA/大小仍被原 Git 字节校验拒绝；缺文件/读取失败/改字节拒绝。Runner 对应定向报告在下文登记。 |
| RM-A04 | 真实 Runner/Codex + 隔离 Git/provider/SQLite 草稿转 DAG、正常停止清理 | 通过；`diagnostics/live-platform-digests-20261102T155125711Z/summary.json`，229.971s，真实两次 generate、6 份文档、2 个正式任务/1 条依赖、实际隔离 Git 发布合并和 SQLite 入图；同目录 evidence 保存原请求、Run 观察、完整正文及资源结算。 |

## 真实读取与版本

SteerLine 受测基线 `7d85a9e908102a4f12210bbba91408f39362e6a7` 加本轮工作树；各 summary 保存完整 dirty 清单和命令。Runner 独立源码基线 `66cc0acf50b675c75458d9d6281139e16158a3a3` 加本轮修改；原 `C:\Project\SteerRunner` 未修改。受测 EXE 为 `bin/steerrunner-verified.exe`，SHA-256 `df27f15bc61767edcfdd6713b6572d5747d4889cfccaace8846d1dcef3dba9ec`，resource digest `a968df6ee06ae6a16650ed9f0eee3ab878e9406a4541ab44fa6548674161c707`。

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
