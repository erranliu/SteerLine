# #266 后全项目回归与修复

2026-10-09 开始、2026-10-10 完成（Asia/Shanghai）。按用户要求 rebase、运行所有测试、修复问题并更新或退役过期测试。主线固定为 `21d9735057f990f9fb83a678c5293eca8c8c520c`（PR #266）；交付 PR 为 [#267](https://github.com/erranliu/SteerLine/pull/267)。这是直接授权的维护工作，不新增 roadmap 任务。

## 修复

- 重复相同 worker 等待原因时，使用实际持久化的 `waiting_input` 判断幂等，保留原 Decision 与控制修订号；有新原始操作回执时仍提交。新增回归同时验证重复等待不改状态、新回执可从 SQLite 读取。
- 已接受反馈面对尚未原生提交的冻结下一步输入时，可在原会话开始反馈阶段；已有 worker、原 turn/session 操作或未知 pending input 仍保留，不重复派发。MC3 与服务反馈回归验证原会话、Attempt、工作区和输入身份。
- fixture、历史接线、恢复、候选与发布断言适配 execute/report/publish 三步，final 必须绑定原轮次；controlled helper 分别执行本地提交、只读报告和正式发布，禁止重复或乱序输入。
- L2 门槛与严格场景枚举采用当前登记；移除已退役旧本机 Orca 的公开测试包装，退役不满足当前 final 契约的旧单轮远端场景，保留历史驱动与能力拒绝回归。
- 工作区删除核验当前 Orca 正式回执、路径消失及 Git 注销，实际记录 trash 留存情况；安装/调试隔离使用 10 分钟业务期限、11 分钟 Go 父期限和普通 30 秒命令期限。

## 结果

| 范围 | 结果 | 命令（省略独立临时 `-out`） |
| --- | --- | --- |
| Full，三 worker | 1426 条父子记录，0 失败，35 默认 skip；完整覆盖，约 29 分钟 | `go run ./cmd/test-feedback -workers=3` |
| L1 | 477 条记录，0 失败、0 skip | `go run ./cmd/test-feedback -level=L1` |
| 完整 L2 | controlled 门槛、本地 Codex 真实交付，2/2 | `go run ./cmd/test-feedback -level=L2` |
| L3 controlled | 交付、等待合并后重启，2/2；安装/调试隔离，1/1 | `go run ./cmd/test-feedback -level=L3 -suite=delivery,recovery -env=controlled`；`-suite=runtime` |
| L3 live | 本地 Codex 交付与恢复，2/2 | `go run ./cmd/test-feedback -level=L3 -suite=delivery,recovery -env=live` |
| L3 native | 真实 Orca 工作区删除与重启，1/1 | `go run ./cmd/test-feedback -level=L3 -suite=workspace -env=native` |
| 带 tag 基础设施及边界 | 222 条记录，0 失败，1 默认 sandbox preflight skip | `go run ./cmd/test-feedback -- -p=1 -parallel=1 -tags=e2e ./cmd/test-feedback ./tests/e2e/internal/... ./tests/e2e/cmd/... ./internal/testkit/boundary/... -count=1` |
| 最后改动定向复验 | runner 78 条、testenv 17 条，0 失败、0 skip | `go run ./cmd/test-feedback -- ./cmd/test-feedback -count=1`；`-- -tags=e2e ./tests/e2e/internal/testenv -count=1` |
| PowerShell 路径防护 | 默认 10 项通过；未选择需机器权限的 symbolic-link 项 | `scripts/test-debug-runtime-isolation.ps1`（独立临时结果目录，默认范围） |
| Python | roadmap-runner 18/18、任务身份脚本 10/10 | `python .agents/skills/roadmap-runner/scripts/test_state.py`；`python scripts/test_roadmap_task_id.py` |

当前注册表 8/8 场景均通过，所有最终场景 `cleanup_complete=true`。三个真实 Codex 场景均核验正数 usage；完整 L2 的真实 [SteerlineTest PR #180](https://github.com/erranliu/SteerlineTest/pull/180) 已合并。native 工作区场景不启动模型，不把它计为模型交付覆盖。父子记录不当作独立场景数量。

Full 来自 clean `99c1d6dea44200fbca75cac75d12de6306eb16fe`，之后只修改 runner、测试/fixture、E2E 期限与文档，SteerLine 生产实现未变。L1、基础设施、真实模型与路径防护来自 clean `726d3540fd3e8b0f158cd0563f8614b1cd599e3f`；testenv 来自 clean `e3a894463fea834aa5854eeede1bb4c2e1b8910a`；native 来自 clean `8e1799f65b80d4de78c993b2f744f960d0a20d29`；最后 controlled 与 runner 来自 clean `0f5cae6e76bf20e61c5da7140fdd416670548863`。随后仅提交本结果和导航。

环境为 Windows、Go 1.26.4、codex-cli 0.159.2，真实 Codex 固定 `gpt-6-luna/medium`、`workspace-write`、`unelevated`、`on-request`、`auto_review` 与网络访问。默认原生 opt-in、退役用例和人工界面未验；本次没有 race 结果。旧本机 Orca 模型场景不再登记；supervised 分步流程仍缺少原生 final 来源，见[当前能力缺口](../../orca-remote-l3.md#当前分步流程能力缺口)，不声明该能力通过。

## 证据与诊断

[最小结果索引](all-tests-after-266-2026-10-10.json) 保存每次精确源码、skip、场景 proof、报告路径和 SHA-256，并核对当前 8 项注册表无遗漏或重复。原始报告在 `C:\Users\lmas1\AppData\Local\Temp\steerline-all-tests-20261009-200603568`；仓库不保存完整日志、数据库、凭据或 prompt，不承诺临时目录永久保留。

初次过期 fixture、错误注册门槛、阶段数量、trash 假设、删除回执解码和 runtime 命令/父期限失败及中断 Full 均保留为诊断，未作为最终通过证据。一次早期 Go `found pointer to free object` fatal 与一次服务恢复回答冲突未在后续定向、严格 checkptr 和最终 Full 中复现；未认定其根因已修复。最终 E2E 使用独立运行的通过报告，没有从失败汇总拼接场景。

最终 Markdown 导航、任务身份与 `git diff --check` 均通过；它们只证明静态文档一致性。
