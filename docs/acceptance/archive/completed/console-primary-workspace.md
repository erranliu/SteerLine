# 控制台三窗体验与 PR #234 质量审计

绑定 [Spec](../../../archive/spec/completed/console-primary-workspace.spec.md) / [Plan](../../../archive/spec/completed/console-primary-workspace.plan.md)。实现基线 `172cea6c`，首次交付 `6e920356`；2026-10-07 质量审计同步主分支 `ae32f300`，合并提交 `d6e36a3d`。

## 已有实现验证

首次 console/query/client 124 项定向回归、真实 app/SQLite/受控 worker 原地反馈接线与 Windows ConPTY 通过。固定 L1 的既有集成组累计 15 分钟超时、原报告不完整；缺失的 TM04/TM06/TM08 定向补跑 passed/complete=true，登记场景无遗漏，不拼成一次成功 L1。

原始事实固定在 [首次验证快照](https://github.com/erranliu/SteerLine/tree/6e920356c95d552d027f3ba326d3b6a431565665/docs/validation/console-primary-workspace)：原报告、失败、命令、哈希及终端记录保持原内容。当前文件树依照主分支清理规则只保留本摘要，原始文件仍在本地，不删除现场、不恢复主分支已退役的验证目录。

## 审计发现与修复

1. 可见性门槛原先只计行数，允许完整 Task/候选目标被截断后提交。现在确认门槛与实际标题/目标行共用表示和显示列宽；目标不完整则不发送。
2. 回答和停服模式可能残留 `recoveryActive`，提交后回到旧核验页，迟到核验也会覆盖当前提示。离开核验时清除模式、取消其读取代次，回答/选择/确认互斥。
3. 最新已终态回执及其答案预览会占满两行，遮住更早的未决请求。未决/失败/未知回执优先，预览不能挤掉其他待核实事项。
4. final 索引失败、变空或仅有 gap 时，旧成功预览未失效；有效 final 与 gap 并存时，诊断没有显示。索引失效清除预览并拒绝迟到正文，已知 gap 即使有规范预览仍明确展示。
5. 被轮次/Attempt 核验拒绝的响应被当作已完成缓存，之后同一条 final 不再读取。缓存明确区分正在读取和已验证完成；拒绝的结果不形成缓存命中。
6. F8 翻页超出正文末尾后，向上翻页仍停留末尾。操作全文与问题全文按实际内容/视口有界滚动，一次向上即可移动。
7. 主分支验证目录清理导致文档冲突；保留新的证据目录规则、人工确认链接与本 PR 三窗交互，不把大量原始日志重新带入版本库。

## 审计验证

原始输出根目录：`C:/Users/lmas1/AppData/Local/Temp/steerline-console-audit-234-idlezave/`。各运行独立子目录，保留失败，不覆盖旧报告。

| 场景与阶段 | 命令/证据 | 结果 |
| --- | --- | --- |
| 初次复现 CPW-02/03/04 | `go run ./cmd/test-feedback -out=<根>/before -- ./internal/console -run '^TestCPWAudit' -count=1` | 模式残留、未决回执遮挡、final 失效及无效缓存真实失败；截断目标断言初稿过宽，后改为具体长 Task ID，不把该初稿当作截断证明 |
| F8 滚动复现 | 同一入口，`-out=<根>/paging-before`、`-run '^TestCPWAuditF8Paging'` | 未修复时向上仍卡在正文末尾，真实失败 |
| 修复后定向 | console/query/client 全包回归，原始 `after-*/summary.json` | 最终 `after-delivery/summary.json`：134 项通过，passed/complete=true，无 skip；包含新的 10 项含父/子断言记录，保持原身份/草稿/完整正文/窗口宽度断言 |
| 当前 Windows ConPTY | `go test -c -tags=ps1terminal` 编译、运行 `TestCPWWindowsTerminalInput`，原始 terminal 捕获/VT 输出 | 当前输入/模式/目标可见性构建 PASS，80×24，完整中文两行、F8 中 Enter/F2 零控制请求、F6/F7、Tab/Shift+Tab 与窄屏两次 F2 一次原候选提交；其后仅回执错误优先级细分由定向回归覆盖。捕获 `terminal-capture.json`、VT `terminal-vt.json`；不证明物理 IME/所有终端字体 |
| 静态、源码及 PR | go vet、产品构建、文档链接/任务身份/差异检查、GitHub PR 状态 | go vet、产品构建、文档链接、任务身份及差异格式检查通过；平台状态以最终头提交核对，不修改 runner 或绕过必需检查 |

本轮不重跑 Full、L2/L3 或整个 L1：没有改变服务/worker 协议、执行副作用、发布/恢复或数据库格式；新行为由有针对性的 console fixture 和一次当前 ConPTY 验证。原 L1 限制继续如实保留。

审计终端 fixture SHA-256：`d9762ba39f7d7284460918e1e3ebc21be2d43e0aa2940d0bbf1eee2fd3a8ef96`。原始复现和终端字节记录留在上述临时目录，旧记录可从固定提交读取。

PR #234 已取消草稿并核对为 `MERGEABLE` / `CLEAN`，主分支冲突已解决；平台未配置可见检查条目（statusCheckRollup 为空），不将其称为 CI 通过。

## 再次同步主分支

主分支推进到 `ee1ab5d6` 后，README、会话展示、Spec 索引和退役控制台约束出现文档冲突。保留主分支的集中维护结构：README 使用简短入口，交互规则由 `product-design/execution-console.md` 维护；会话文档仅维护读取/持久语义；Spec 索引保留本 PR，同时沿用草稿候选与正式 DAG 准入区分。删除主分支已退役的 `roadmap/console-constraints.md`，Spec 改引新设计文档。

控制台/product/app 实现未改变。链接检查通过（229 个 Markdown、1396 条本地导航、0 错误）；任务身份静态检查通过（84 个身份文件、13 条草稿、0 条正式任务，不代表外部 DAG 查询）；差异格式和无未解决冲突检查通过。此轮只做文档静态验证，未重跑产品或模型测试。

## 合并前最终复审

从 `cb97ce4e` 再次检查当前差异，新增四个 `TestCPWFinal*` 场景，修复前全部真实失败：退出核验未取消读取代次；单候选 F6/F7 在没有实际目标变化时关闭编辑器；帮助入口忽略草稿保存失败；组合文字因子串匹配被当作模式键。修复后精确分派模式键、按实际候选变化保存/退出、帮助保存失败保留原编辑器、Esc 取消原核验读取。

本次原始根目录：`C:/Users/lmas1/AppData/Local/Temp/steerline-console-merge-234-1tj4sfgj/`，失败 `before/`、回归 `after/`、当前 L1 `l1/` 均保留。console/query/client 138 项通过，passed/complete=true；主分支优化后的当前固定 L1 passed/complete=true，耗时 95.27 秒。该 L1 使用当前固定选择和并行入口，不将旧版超时报告重标为通过，也不声称两版范围相同。go vet、构建、文档链接、静态任务身份和差异检查通过；不运行 Full、L2/L3/live，不宣称验证了外部 DAG。既有 ConPTY 保留其真实构建采样，本轮新增键盘状态边界由定向测试覆盖。
