# 验收摘要

2026-10-07 清理：本目录只保留仍被当前开发使用的验收结论、限制和最近基线；原始运行产物、重复重跑、旧故障诊断和迁包清单退出当前文件树。历史结果只证明记录中的源码和环境，不代替当前改动的验证。

| 内容 | 入口 |
| --- | --- |
| 最近全项目基线及逐次源码、命令、哈希 | [2026-10-07 基线](latest-baseline.md) · [结果索引](baseline-results.json) |
| 精简固定 L1 与三个独立进程并行 | [L1 优化验证](l1-optimization.md) · [结果索引](l1-optimization-results.json) |
| 尚未闭合的原生验收及已有通过边界 | [待补验收](pending-acceptance.md) |
| #39 本机多候选交付与恢复 | [本机候选验收](candidate-acceptance.md) |
| #38 CC5 用户人工确认 | [人工确认原文](manual-acceptance.md) |
| #234 三窗主界面体验及质量审计 | [主界面验收](console-primary-workspace.md) |
| #231 三窗及内嵌反馈 | [控制台验收](product-console.md) |
| 原生准备工具证据 | [准备验收](worker-readiness.md) |
| RoadmapGuide、草稿/正式静态边界与执行技能 | [规划文档检查](roadmap-planning.md) |

## 历史取证

清理前固定快照：`172cea6c6530b6693b742324a9c1d4f9aeb17b6d`。原验证目录见[Git 历史快照](https://github.com/erranliu/SteerLine/tree/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation)。旧规格中的证据链接固定到这个版本；当时的命令、SHA、路径、失败和 skip 保持原事实，不重新标记为当前通过。

本地无需还原目录即可读取原记录：

```powershell
git show 172cea6c6530b6693b742324a9c1d4f9aeb17b6d:docs/validation/candidate-native-acceptance/acceptance.json
```

## 新证据

所有 `cmd/test-feedback` 入口默认输出到系统临时目录 `steerline-test-feedback/<运行时间>`。显式 `-out` 仍可选择新的空目录；本目录的子目录被 Git 忽略，避免原始产物再次进入版本库。失败证据保留在该次运行目录，不覆盖；需要长期保留时只提交最小摘要，列明源码、命令、范围、结果及限制。必要测试样本放对应包的 `testdata`，测试方法维护在[测试指南](../testing.md)和[E2E 指南](../../tests/e2e/README.md)。

## 测试计划与本次结果

本次只删除历史产物、迁移三个不变样本、修正文档导航及 runner 默认输出目录。未选择 L1/Full/L2/L3 或付费原生组：产品业务、协议与场景登记没有修改；所需证据来自以下最小检查，不新增模型场景或测试环境。

| 级别 / 组或场景 ID | 环境 | 命令 | 时点及通过条件 | 结果与证据位置 |
| --- | --- | --- | --- | --- |
| 文档静态 / doc-links、task-identities、whitespace | Windows、Python 3、Git | `python scripts/check_doc_links.py`；`python scripts/roadmap_task_id.py check`；`git diff --check` | 清理后、交付前；本地导航存在、任务身份一致、无空白错误 | 通过；终端输出与本摘要 |
| 文档静态 / historical-targets、fixture-integrity | 当前仓库、Git/Python | `git cat-file --batch-check` 核对固定链接；`git show 172cea6c6530b6693b742324a9c1d4f9aeb17b6d:<原文件路径>` 与新文件比较 | 删除后；全部历史文件/目录存在，三个样本及 baseline-results 与原 Git blob 一致（忽略 checkout CRLF） | 80 个固定历史目标存在；三个样本与基线索引未改内容 |
| 包级定向 / Orca transcript parser + scoped default output | Windows、Go 1.26.4，无 live 开关 | `go run ./cmd/test-feedback -- ./internal/execution/adapters/orca -run '^(TestReadinessEvidenceUsesActualPlatformToolBlocks\|TestActualSavedCommandWrapperProvidesProbeEvidence\|TestActualStringifiedCommandWrapperProvidesProbeEvidence)$' -count=1 -timeout=2m` | 迁样本及默认输出修改后；三父测试及负向分支命中，通过、无 skip，报告位于临时目录 | 通过；`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261007T020005.622435000Z/summary.json` |
| 包级定向 / runner options、output status filtering | 同上，无模型 | `go test ./cmd/test-feedback -run '^(TestScopedOptionsAndNativeSwitchGate\|TestFilterGeneratedRunOutputPreservesOtherValidationChanges\|TestGeneratedRunOutputPathsOutsideRepositoryExcludeNothing)$' -count=1 -timeout=2m` | 默认输出修改后；选择与 native 开关守卫、仓库内/外输出过滤断言通过 | 3 个父测试通过，0.734 秒；终端输出 |

执行证据只说明本次清理，没有将历史产品验收重标为当前源码通过。原运行目录保留失败与原始输出；本目录子目录通过 ignore 验证保持不跟踪，根目录摘要仍可提交。
