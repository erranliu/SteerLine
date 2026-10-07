# #243 后全项目回归与修复

2026-10-07 开始、2026-10-08 完成（Asia/Shanghai）。按用户要求 rebase 到最新主线、运行所有测试并修复问题。最终基线为 `634bc72380d02710e4585d493be23b472c460a77`（PR #243），以下七份最终报告全部来自 clean `9112027eacb06669650dd4887ea974a8e4f780bf`。之后的验收记录提交只修改文档与结果索引。

## 最终结果

| 范围 | 结果 | 命令（省略独立临时 `-out`） |
| --- | --- | --- |
| Full，三 worker | 1221 条父子记录，0 失败，15 默认 skip；完整覆盖，约 58 分钟 | `go run ./cmd/test-feedback -workers=3` |
| L1 | 425 条记录，0 失败、0 skip | `go run ./cmd/test-feedback -level=L1` |
| 完整 L2 | controlled 门槛、真实本地 Codex 与 Orca smoke，3/3 | `go run ./cmd/test-feedback -level=L2` |
| L3 controlled | 独立交付、等待合并后重启，2/2 | `go run ./cmd/test-feedback -level=L3 -suite=delivery,recovery -env=controlled` |
| L3 live | 本地 Codex / Orca 的交付与恢复，4/4 | `go run ./cmd/test-feedback -level=L3 -suite=delivery,recovery -env=live` |
| L3 candidates | 双候选、重启、反馈、连接故障、替代候选、在途落选、唯一交付与后继，1/1 | `go run ./cmd/test-feedback -level=L3 -suite=candidates -env=live '-scenario=candidates.local'` |
| 带 tag 基础设施及边界 | 173 条记录，0 失败，1 默认 sandbox preflight skip | `go run ./cmd/test-feedback -- -p=1 -parallel=1 -tags=e2e ./cmd/test-feedback ./tests/e2e/internal/... ./tests/e2e/cmd/... ./internal/testkit/boundary/... -count=1` |
| Python | roadmap-runner 18/18、任务身份脚本 10/10 | `python .agents/skills/roadmap-runner/scripts/test_state.py`；`python scripts/test_roadmap_task_id.py` |

注册表 10/10 场景全部通过，最终场景均 `cleanup_complete=true`，真实场景均 `usage_verified=true`。候选使用独立 manifest，源码固定为上述最终验证 commit。父子记录不当作独立场景数量，默认 skip 不算原生验收通过。

环境为 Windows、Go 1.26.4、codex-cli 0.159.2、真实 `gpt-6-luna/medium`，采用当前固定 `workspace-write`、`unelevated`、`on-request`、`auto_review` 和网络访问方案。本次未批量开启历史原生 opt-in，也不声明人工界面或 race 已验。

## 修复与通过条件

- Full 每个 worker 顺序运行 execution 与 integration。独立旧主线运行在 60 分钟总期限处被截断，仍有五个 integration 顶层用例缺少终态；Full 总期限改为 75 分钟，保留 L1 的 60 分钟期限、分组 45 分钟期限及完整覆盖判定。
- 原会话恢复正向 fixture 曾宣称会话可用、轮次已完成，却缺少原 session、工作区与冻结权限快照。补齐绑定事实；不可用会话 fixture 保留明确的原 ID，继续验证人工处理及不增加 Attempt。生产的原会话校验与 PR 合并门槛保持。
- 写端口边界清单移除已退役的 `conversation_replay.go` / `conversation_position.go`；其余现存实现仍检查 concrete SQL、广义事务与存储内部访问。
- final-only 历史接线等待完整 final 与 turn 完成事实，替代依赖退役增量事件的五条数量门槛。原 session/turn/attempt、正文完成/摘要、源 checkpoint 和业务 Decision 不推进的断言保留。
- 宿主 review fixture 的本地代理需先启动测试进程再启动 Git，预算由 5 秒改为 15 秒；integration 正向 fixture 的代理预算由 15 秒改为 30 秒。任务 CLI 的构建、回放及两类固定文件拒绝共享预算由 90 秒改为 3 分钟。原准备期限、控制台 45 秒观察条件及业务/负向断言保留。

恢复、宿主 review、写边界、历史采集、原终端重连与控制台、任务 CLI 的定向复测均通过；随后再次完整执行上述最终矩阵。

## 证据与诊断

[最小结果索引](all-tests-after-243-2026-10-08.json) 保存源码、选择、默认 skip、场景 proof、报告路径与 SHA-256。原始报告保留于 `C:\Users\lmas1\AppData\Local\Temp\steerline-all-tests-20261007-214015816`，不提交完整日志、凭据、数据库、原始 prompt 或 capability，不承诺临时目录永久保留。

初次 Full 与 E2E 叠加运行时的命令/启动超时、无效 `-out` 参数位置、60 分钟 Full 截断、主线合入 #243 后中断的旧源码复跑、#243 的过期 fixture/清单/等待条件失败以及首次不完整 fixture 绑定均保留原报告。旧主线的已通过报告只作诊断，未用于拼接最终场景覆盖。

最终源码的文档链接、任务身份与差异检查通过。静态身份检查覆盖 84 份身份文件、10 个草稿和 3 个正式文档行，不证明实际中心 DAG 回执。初始任务 49 共享 ref 名称冲突在 rebase 最新规划后解除，未覆盖共享 ref。

一次中断的 `delivery.local-codex` 临时现场未清理：自动审批两次以 `blocked by policy` 拒绝删除 `C:\Users\lmas1\AppData\Local\Temp\steerline-e2e-509613987`。相关本次进程已停止并核对，目录保留；不将该中断运行标为清理完成。全部最终场景的独立现场已由 runner 成功清理。
