# 308 维护链修复提交与验证汇总

2026-11-20，按用户要求整理 SteerLine 和 SteerRunner 本轮修复并分别提交 PR。这里只汇总既有验证和交付范围，不重新执行真实任务，不部署、不合并，也不停止、恢复或提交原安装任务。

## 提交范围

SteerLine 源码基线为 `3a1777a6bbf5d4b245be2ca12aa24c4b2af4e905`，提交分支为 `codex/refactoring-308-review-fixes`。

- 最新在途 Runner Run 已阻断时，摘要、详情、执行对话与复制信息显示 blocked 和实际诊断；保留原操作与观察/停止责任，不自动替代执行，不允许提前收尾。
- 维护决策接受可选的 `question:null` 并按缺省处理；ask 仍要求有效问题，非 ask 仍禁止问题对象。
- 最终 PR 评审取得固定 PR head 的只读源码，与 diff/PRVersion 绑定；只提供已接受、可信成功、干净且提交匹配的执行记录。源码准备失败阻断，停止和清理沿原生命周期。
- 固定评审正文要求核对路径、符号和实现，并区分执行者陈述、源码事实、实际测试结果及未验范围。正文 diff、字节值、Schema 差异和独立审核见下表记录，不以 golden 更新代替审核。
- 新 live helper 仅在显式输入下从既有 PR 的 report 阶段启动全新隔离现场，不导入旧记录或接管旧 Run，不包含实施/发布步骤。

Runner 独立分支 `codex/isolate-appserver-history` 的基线为 `8ebbc717776eb2ad85371060566776c4402773fa`。对应改动为复用 Codex 原生 home/config 的索引、同 home 跨进程初始化锁，以及 Windows worktree 创建使用 `os.DevNull` 防止 LFS 生成 `dev/null` hooks。两条 PR 各自保留源码、回归与验收记录，不提交 EXE、登录文件、运行数据库或原始模型日志。

## 已执行验证及复用边界

下表报告均为原受测基线加当时未提交差异；提交动作不会将其改写成新 commit 的干净源码报告。此次未改生产代码、依赖、提示词或测试定义，不因提交/推送重复运行同范围测试。

| 范围 | 结果与原证据 | 详细记录 |
| --- | --- | --- |
| blocked 展示 | query/refactoring/model owner 212 项、页面组件 33 项、前端构建、正式 L1/L2/go.http 通过；`C:/SteerlineDebug/wt-9ed5-refactoring-blocked/` | [诊断与展示修复](refactoring-runner-blocked.md) |
| nullable question 与 LFS | Schema 边界及完整 workspace owner 通过，修复后的 SteerLine 正式 L1/L2 通过；`C:/SteerlineDebug/wt-9ed5-308-docs/` | [修复及真实交付](refactoring-308-docs-fixes.md)，[原失败](refactoring-308-docs-live.md) |
| 最终源码评审 | refactoring owner、App 固定提交/替换对象/LFS 回归、prompts 通过；最终 L1 准备 0.38 秒/执行 53.19 秒，L2 准备 37.57 秒/执行 245.36 秒且清理完成；`C:/SteerlineDebug/wt-9ed5-review-source/test-results/L1/review-source-01/summary.json` 与 `L2/review-source-01/summary.json` | [源码评审接线、正文与独立审核](refactoring-review-source.md) |
| GitHub 完整真实维护链 | 正式 L4 `refactoring.local-codex` 1 项通过、零跳过、清理完成；双 Session 并发、PR 发布及冲突续作完成；`C:/SteerlineDebug/wt-9ed5-refactoring-live/test-results/L4/local-codex-01/summary.json` | [真实完整链](refactoring-runner-blocked.md#后续真实完整重构测试) |
| 308/Gitea 真实文档链 | 独立现场完成选题、实施、验收、发布及评审，初始建议 insufficient_evidence；`C:/SteerlineDebug/wt-9ed5-308-docs/packages/run-04/evidence/test-result.json` | [真实交付](refactoring-308-docs-fixes.md) |
| #463 源码重评与文档收窄 | head `13394163` 的真实只读评审建议 revise；按用户授权收窄到直接调用事实后，head `8c484f37` 的真实评审建议 merge，1 项通过/零 skip/263.85 秒，内置 JSON 重试一次及资源清理有原证据 | [源码重评](refactoring-review-source.md)，[收窄与复审](refactoring-pr463-narrow.md) |
| Runner 资格 | 启动锁/合成历史探针、Codex owner、LFS workspace owner 和最终 EXE 停止专项通过；原完整 L1 的两个计时失败、Controlled 的既有 standalone 失败仍保留，不能声明完整 L1/Controlled 或部署资格 | Runner PR 内 `docs/acceptance/codex-startup-index-reuse.md`、`windows-lfs-worktree-hooks.md` |

本次交付前复核 `packages/pr463-01/verification-manifest.json` 中五个评审实现/夹具/模板文件的 SHA256，全部匹配；Runner 使用正式 `Get-SRSourceIdentity` 核对生产输入摘要，仍为 `75824ff79737b01e5fd974f4ea91dcc098a4095f059f6a8f6d757d0ed53de9d3`，与 `steerrunner-windows-lfs.exe.build.json` 一致。真实链 EXE SHA256 为 `31153b1f865364fc1c6a0ddef84e2596f2d67222a3ac25c904af795f853dd10f`。此前报告的失败、skip 和 dirty 身份不覆盖或补签。

仅整理记录的最终检查：`python scripts/check_doc_links.py` 核对 824 份 Markdown、5,024 个本地链接，零错误；Runner 本轮 4 份指南/验收文档的 25 个本地链接无缺失；两仓 `git diff --check` 通过。创建 PR 前已刷新两仓 main；SteerLine 的 `b9ce126e` 是独立部署恢复改动，与本轮文件无重叠，Runner main 仍为原基线。GitHub 实际 mergeable 状态在 PR 创建后核对，不把本地无重叠当成远端批准。

## 原现场与保留资源

原任务 `f292bdcfcf4b4854ad10819f0c4834a9` 未收到停止、恢复或重提命令。只读核对 `C:/Projects/merge-h-308-code` 仍干净、HEAD 为 `9a084fb61a3bfdac79cf30f0ff8b3a1cc280474a`；`C:/SteerlineInstall/bin/steerrunner.exe` SHA256 仍为 `0d733a1fcd09f06ad7e2891ad90030f16d3fb4fae1b877adc86fbbe1bf74137e`。

308 的 [PR #463](http://192.168.0.66:3000/zymobile/merge-h-308-code/pulls/463) 保留 open draft、未合并，本次整理不再修改它。真实验证现场均按各自记录结算并移除认证副本。未运行 Unity 或恢复业务测试，不将源码/文档复审的 merge 建议表述为行为验证通过。
