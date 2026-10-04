# 最新主分支后的全量修复验收（2026-10-05）

用户明确要求 rebase 最新主分支，按 `tests/e2e/README.md` 重跑全部测试，发现问题修复并提 PR。本次最终基线是 #217 合并后的 `df9e2f855a25fca3d82f635b6f04c5d404581ae6`；最终所有入口均验证同一干净源码 `3ebd521586ba8503158187ace287d4f55a9f872a`。本目录是在测试结束后补充的文档归档。

## 修复

1. **候选恢复互相阻塞。** 本地候选断连后，其保存输入的恢复路径反复报错；`resumeWorkerFlow` 立即返回，排在其后的 Orca 候选虽已保存第三次反馈并创建原生 Task，却得不到继续派发机会。改为访问所有独立 run，最后用 `errors.Join` 返回原诊断。没有重发未知结果的请求、替换原会话或自动重派候选。`TestCandidateResumeErrorDoesNotStarveSibling` 强制故障候选排在前面，修复前稳定失败，修复后及最终 Full 均通过。
2. **重构后的写边界守卫。** 原守卫仍读取已删除的 `query_command.go` 等文件。更新到 `query_construction.go`、当前 `service_*.go`、`internal/task` 和 `internal/storage/ports` 的实际所有者，保留 SQL、具体事务和宽泛 domain writer 的禁用检查，所有者源文件零匹配仍失败。
3. **原终端启动断言。** 两项 Orca 原终端/重启测试仍期待命令直接包含模型，而产品现在通过绑定的 `codex-session-launch -config session-launch.json` 启动。改为核对精确配置路径、项目/任务/执行/attempt/原工作区与原终端绑定，以及冻结资源中的完整可执行文件和参数。受控 fixture 的 `gpt-6-luna/high/workspace-write` 保持不变；真实 E2E 仍为 `gpt-6-luna/medium` 和完全访问。
4. **候选测试合同和诊断。** 一次真实 worker 把进度文字写入 `result.txt`，导致正式发布脚本拒绝。Spec 和每次反馈现在明确字面内容、单个 LF、UTF-8 无 BOM、读回比较后发布。正式脚本及内容断言保留；测试不代写结果。发布等待绑定该次原 command ID，已结束且没有可信 PR 的原反馈立即报告实际错误，不误判旧回合或在途输入。增加连续三回合及 open PR 后服务重启的原会话回归。
5. **测试报告防覆盖。** 一次 Full 错用了已有输出目录，暴露出只有 E2E 入口拒绝非空目录的问题。所有执行入口现在在启动测试前统一拒绝非空 `-out`，保护已有失败报告；显式旧日志重新汇总入口保留原行为。回归覆盖 Full、L1、L2、编译和定向入口，并检查原报告字节未变、没有新增测试产物。

临时诊断代码已全部移除。产品实现的改动仅为候选恢复循环的错误隔离，其他改动属于测试、fixture 或 runner。

## 最终结果

环境为 Windows/amd64、Go 1.26.4、codex-cli 0.159.2、本机 Orca。用户明确要求全量，故本次选择 Full、L1、全部 10 个登记场景、带 tag 基础设施、E2E 编译及 Python 离线测试。

| 入口 | 结果 | 原始目录 |
| --- | --- | --- |
| Full，3 worker | `passed=true`、`complete=true`；1121 条父/子测试记录，0 失败，18 跳过 | `full-freeze-03` |
| L1 | 完整通过 | `l1-freeze-03` |
| L2，受控门槛 + 两条真实链路 | 3/3 通过；`coverage=full-L2`；清理完成 | `l2-freeze-03` |
| L3 controlled：delivery/recovery | 2/2 通过；清理完成 | `l3-controlled-freeze-03` |
| L3 live：本地/Orca delivery/recovery | 4/4 通过；usage 验证及清理完成 | `l3-live-freeze-03` |
| L3 live：`candidates.local` | 整轮通过，约 18 分钟；原 20 分钟限时；usage 验证及清理完成 | `candidates-freeze-04` |
| E2E 带 tag 基础设施 | 通过；1 项专用 sandbox 凭据检查默认跳过 | `infra-freeze-03` |
| 全部带 tag E2E 包编译 | 通过，仅证明编译，不替代场景通过 | `e2e-compile-freeze03` |
| roadmap-runner Python 离线脚本 | 18/18 通过，无真实任务/会话/PR 操作 | `python-freeze03.txt` |

Full 的 18 项跳过包含 Windows 平台条件、helper 入口及未启用的专用原生探针；不算通过，也不声明已经验收远端实验、elevated sandbox 或独立人工探针。具体跳过名称见 [results.json](results.json)。测试总数包含父/子记录，不能当作独立业务场景数。

最终 E2E 覆盖与 `tests/e2e/suites.json` 的 10 个 ID 精确相等，没有遗漏、重复或 gate-only 替代。所有真实场景检查同一身份、正数 usage、固定模型/effort；重启和最终交付轮询不增加 usage。

真实测试 PR：L2 的 [#98](https://github.com/erranliu/SteerlineTest/pull/98)、[#99](https://github.com/erranliu/SteerlineTest/pull/99) 已合并；最终候选的 [#101](https://github.com/erranliu/SteerlineTest/pull/101) 胜出交付、[#103](https://github.com/erranliu/SteerlineTest/pull/103) 后继交付已合并，原断连候选 [#100](https://github.com/erranliu/SteerlineTest/pull/100) 和替换落选候选 [#102](https://github.com/erranliu/SteerlineTest/pull/102) 已关闭。本次测试分支和原生资源均按所有权清理。

## 命令与证据

在仓库根目录执行，每次 `-out` 使用新空目录。下面省略该唯一输出参数，实际分组命令、报告 SHA-256、源码、时间、构建摘要和各场景结果保存在 [results.json](results.json)。

```powershell
go run ./cmd/test-feedback -workers=3
go run ./cmd/test-feedback -level=L1
go run ./cmd/test-feedback -level=L2
go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=controlled
go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=live
# 新 manifest 的 source_commit 匹配运行时 HEAD，固定 Orca/local 两套 profile
$env:STEERLINE_CANDIDATE_ACCEPTANCE_CONFIG = '<新 manifest 绝对路径>'
go run ./cmd/test-feedback -level=L3 -suite=candidates -env=live '-scenario=candidates.local'
go run ./cmd/test-feedback -- -tags=e2e ./tests/e2e/internal/... ./tests/e2e/cmd/... -count=1
go run ./cmd/test-feedback -- -tags=e2e ./tests/e2e/... -run '^$' -count=1
python -B .agents/skills/roadmap-runner/scripts/test_state.py
```

原始报告根目录：`C:\Users\lmas1\AppData\Local\Temp\steerline-all-tests-latest-20261004-ea1ed3bf`。各场景保留公开状态、原请求和资源身份、服务日志、原生 usage/对话诊断及清理回执。仓库仅保存精简结果及原始报告哈希，不提交认证、数据库、原始 prompt 或 dispatch capability。

## 失败与重跑

初始 Full 的旧路径守卫、旧启动断言，以及候选错误文件内容均保留独立失败报告。真实停滞曾在移除早期诊断后复现，之后延迟诊断与确定性回归共同定位恢复循环提前返回，最终实现修复后重新冻结源码验证所有入口。

同一最终源码的 `candidates-freeze-03` 已完成故障隔离、替换与胜出交付，但在后继初始回合到达 20 分钟，整轮失败且清理完成。等待其余并行回归结束后，以新目录/新现场运行 `candidates-freeze-04`，完整通过。保留原 20 分钟期限和全部业务断言；未修改源码、合成完成或重用失败现场。

早期误用 `full-fixed` 输出目录后，尚未覆盖的旧 summary 和 other-packages 日志另存为 `full-fixed-prior-64db-evidence`；原 core 日志被后续启动重写，该目录明确只作部分诊断，不能作为完整验收。最终 Full 使用全新 `full-freeze-03`，源码冻结且全部原始分组报告一致。报告引用及这项限制见 [diagnostics.json](diagnostics.json)。
