# 远端维护执行验收

真实验收使用 SteerLine 基线 `a7cc509b` 加本分支的候选源码。SteerRunner 在独立 worktree 中提供远端 `execute/1` 的 Git 取证。[SteerRunner #170](https://github.com/erranliu/SteerRunner/pull/170)

| 检查 | 状态 |
| --- | --- |
| Runner 能力及定向覆盖 | 已通过。用 `git.status` 对原受管工作区采样，在前、中、后核对原实例，并拒绝已转交 Run。现场只读探针验证了台式机 64G 的 RPC 响应。 |
| SteerLine 定向测试与 L1 | 候选版 L1 通过。报告根 `C:/SteerlineDebug/wt-b99d-remote-maintenance/test-results/L1/remote-delivery-final-1794824825607`。只读重试及模板的定向 Go 测试通过。 |
| SteerLine L2 | 磁盘恢复后复跑通过。报告根 `C:/SteerlineDebug/wt-b99d-remote-maintenance/test-results/L2/26bdbc093e5c46ee94ad5f3c14caf78c`。 |
| 64G 首次远端验证 | 原注册 checkout 有 Orca 会话，正式环境检测拒绝准备。报告根 `C:/SteerlineDebug/wt-b99d-remote-maintenance/test-results/L4/ee580842cf06455eb31f6bb7c0bf081e`。 |
| 64G 隔离 checkout 验证 | 本次测试环境正式获得 ready。公开维护资格随后因预检使用了要求正式提示词的提交 schema 而拒绝。诊断已定位并修复，未进入写入。检测原始证明位于 `C:/SteerlineDebug/wt-b99d-remote-maintenance/test-results/L4/46e4dc9456aa49de836cd7bd3dfe1814/scenarios/refactoring.remote-orca/environment-proof.json`。 |
| Runner Final 与 64G 两轮写入 | Runner `534e382e6edce24e3d20c221dfca4c8145ddff47`。固定 EXE 的 SHA-256 为 `EC043894780B9F72DAE74FD27D1F761894E774BF4647D4F22235CD966D69F7E8`。第一轮由本机只读 SteerAgent 审计采纳 `972ab142`。第二轮在原工作区续作并提交 `0d082a0d`。两轮均有绑定原 Run 的已验证干净 Git 取证。 |
| 64G 草稿 PR 和原 PR 冲突续作 | 第二轮的本机只读目录探针报 `native_read_only_directory_unavailable`。该次未到 PR 阶段，Runner、测试 checkout、本地临时目录和远程测试引用已清理。报告根 `C:/SteerlineDebug/wt-b99d-remote-maintenance/test-results/L4/accepted-rounds-64g-1794821995643`。后续完整通过见下方记录。 |

L4 入口为 `go run ./cmd/test-feedback -level=L4 -suite=refactoring -env=live -scenario refactoring.remote-orca -out <新报告根>`，使用台式机 64G 的显式 Orca Codex 方案和本轮固定 Runner。测试基线及真实草稿 PR 只使用隔离的 `erranliu/SteerlineTest` 分支。


## 只读决策重试

隔离诊断构建复现了 `schema_mismatch`。报告根 `C:/SteerlineDebug/wt-b99d-remote-maintenance/test-results/L4/read-only-triage-64g-1794822516757`。本机模型返回了 schema 定义，正式校验拒绝该结果。此前第二轮也曾被原生只读目录探针中止。两个失败沿用同阶段、同轮次的一次重试预算。原 Run 已停止并完成绑定核验后才能重试。再次失败阻断流程。

`schema_mismatch` 重试复用现有固定 JSON 修复正文。实际正文从 316 增至 628 UTF-8 字节，增长包含两个 LF 和以下一段正文。

> 上次回复未通过 JSON 解析。重新读取本阶段的文件材料，按本次输出 schema 重新生成。先完成 schema 要求的短字段，再写长文本字段。检查所有字段仍位于同一个 JSON 对象内，最后一个字段写完后才关闭对象。对象外不要输出任何文字。

原生目录探针失败的重试保留冻结的本机执行配置和 316 字节的通常正文。固定模板和黄金样本均无差异。独立复审核对了原 Run 来源、同轮共享重试预算、实际请求和失败后的阻断。

## 远端夹具

隔离测试 checkout 的 fixture 指令从 291 增至 872 UTF-8 字节，本机夹具正文保持 291 字节。增加以下整行及行尾 LF，确保测试提交位于原受管分支、回执包含已提交差异，并核对完整身份。

> For each authorized implementation write, run node check.cjs and git diff --check, stage only that round's permitted file, commit it on the managed branch, and verify that git status --porcelain is empty before reporting done. In the summary include the changed file and concise before-and-after code from the committed diff, plus the actual check results. Use exactly the output contract supplied by the runner for each action. Before sending the final JSON, compare action_id and operation_id character for character with the top-level request IDs, including the last character.

测试行动要求从 397 增至 481 UTF-8 字节，增加“每轮结束前把本轮改动提交到原受管工作区，使 git status 干净。” 独立复审核对了候选提交与审计采纳的 Run 绑定，以及测试夹具两处新增要求没有改变生产正文和校验。

使用有界只读重试版本的验收，首轮采纳 `5e2cef53`，第二轮复用同一工作区并提交 `d67063cf`。原工作区补证保持同一干净提交，但最终回执的 `operation_id` 漏掉最后一个字符，服务拒绝采纳。没有生成测试 PR，Runner 和现场均清理。报告根 `C:/SteerlineDebug/wt-b99d-remote-maintenance/test-results/L4/bounded-decision-retry-64g-1794822911041`。


## 64G 全链通过

正式 `refactoring.remote-orca` 的 `passed` 和 `cleanup_complete` 均为 true。报告根 `C:/SteerlineDebug/wt-b99d-remote-maintenance/test-results/L4/git-diff-evidence-64g-1794823720650`。

- 第一轮补证确认原工作区干净，本机只读审计采纳 `9fe1e354`。第二轮复用同一工作区和最近 Run，提交 `d1995cc5` 后干净，本机审计采纳。GitHub compare 核实两次提交范围依次只有 `alpha.cjs`、`beta.cjs`。观察到每次决策都是本机 `generate/1`，冻结访问模式为 `read_only`。
- 在原工作区创建一个真实草稿 PR，首版 head 为 `d1995cc5`。基线推进后，`git merge-tree` 证实两个文件都发生内容冲突。新 Run 复用原工作区，将同一个 PR 更新至 `a3e10a77`，新 base 为 `e27919c2`。前后两版代码均通过隔离 `node check.cjs`，PR 正文保持一致，本机只读评审核对了确切的 head/base。模型给出的合并建议是 `insufficient_evidence`。[测试 PR #847](https://github.com/erranliu/SteerlineTest/pull/847)
- 验收清理关闭测试 PR，移除临时 base/head 分支、Runner 和远端 checkout。公开的验收摘要与场景分别达到完整通过和清理状态。
