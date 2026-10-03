# 最新主干后的测试基线

后续已完成[设计审计与修复](../baseline-repair/README.md)，完整回归通过。以下保留修复前的失败基线，不代表当前候选状态。

日期：2026-10-01。用户要求补充完整基线，并先 rebase 最新代码。本分支已 rebase 到 `origin/main` 的 `14ee2c8`，候选 `61f5d16`。汇总和完整失败/跳过名单见 [results.json](results.json)。当前整体基线失败，不能用此前最小验证或本次 E2E 通过声明全量通过。

## 实际执行

| 范围 | 命令/选择 | 结果 |
| --- | --- | --- |
| 完整确定性 Go 回归 | `go run ./cmd/test-feedback -workers=3` | 失败；准备约 7.1 秒，执行 783.6 秒。第二组 `TestWorkerEnvironmentER3RecheckKeepsOriginalDispatch` 空指针崩溃，中断其后 10 个顶层用例 |
| 缺失用例补跑 | 同一候选，原枚举清单中未出现终态的 10 个用例，精确 `-run`、`-count=1`、独立 home | 全部补跑完成；耗时 106.3 秒，`TestDeadWorkerRecoveryPreservesDecisionAndFreshAnswer` 失败 |
| E2E 基础设施 | `go test -tags=e2e ./tests/e2e/internal/... -count=1` | 修正夹具换行后通过；testenv 52.320 秒 |
| L2 | `go run ./cmd/test-feedback -level=L2 -env=controlled` | 修正后通过，清理通过；时间及构建/源码身份见 results.json 指向的原始报告 |
| L3 | `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled` | 修正后通过，清理通过；时间及构建/源码身份见 results.json 指向的原始报告 |

全量及补跑合并后，343 个核心顶层入口均有执行终态，无剩余未执行核心入口。失败为 47 个顶层测试，包含子测试时为 49 个失败终态；18 项测试/子测试跳过，其中包括未启用的真实平台验收。没有启用真实 Codex/Orca、远端或 UI 的验收开关。包无测试文件的 skip 不计入上述 18 项。

全量工具的 `passed/complete` 仍为 false，原崩溃报告不会因补跑而改写。这里的“均有执行终态”只表示用例已尝试执行，失败/崩溃不等同于行为断言全部通过。

## 本轮修正与剩余问题

rebase 将已提交 Markdown 模板检出为 CRLF。测试项目此前原样复制这些字节，而当前产品的验收标题匹配要求 LF，导致两个场景及两项目隔离检查都在 `task validate` 阶段失败。提交 `67427ec` 仅在测试环境复制任务材料时将 CRLF 规范为 LF；提交、引用摘要和校验均基于规范后的真实文件，未修改产品校验或写入业务状态。上述三个 tagged 范围随后重新通过。

全量和缺失补跑基于修正前的 `61f5d16`；换行修正不涉及这些默认回归用例或生产代码，因此没有重复执行约十三分钟的完整套件。E2E 与环境检查基于 `67427ec`。这是两个明确标注的版本范围，不声明同一 SHA 的全部测试通过。

其余失败集中在配置/权限快照、服务生命周期与恢复、任务交付/丢弃、CLI 和控制台等范围，具体名单在 results.json。尚未逐项判定是夹具过时还是产品缺陷，本轮不通过跳过、放宽断言或恢复旧协议处理它们。下一步应定向定位这些存量失败，再建立通过的基线。

用户要求 rebase 前，旧候选 `2e86e6b` 的完整运行也失败；其补跑在收到 rebase 指令后停止，原始日志保留在主机临时目录。旧结果不用于本页的新候选统计。当前页保存可审阅的计数、名单和原始报告位置，原始完整 JSON/stderr 和构建产物仍保留在各报告的输出根。
