# 待合并 PR 冲突解决验收记录

2026-11-06。关联 [Spec](../../spec/pr-conflict-resolution.spec.md) 与 [Plan](../../plans/pr-conflict-resolution.plan.md)。普通开发，无任务编号。两仓已实现，自动验收与独立审核通过；用户已授权 Runner 独立 worktree/PR，不部署安装环境。PCR-M01 仅体验走查待安排。

## 自动项

| ID | 方法 / 操作 | 环境与构建 | 通过条件 | 安排 / 阻塞范围 | 状态与证据 |
| --- | --- | --- | --- | --- | --- |
| PCR-A01 | Runner owner 两种开发流程两轮 message 和原生问答；固定 EXE 公开受控专项 | controlled；构建固定值见下文 | 公开受理与 can_message 一致；原工作区/会话/配置保持，轮次结果独立；无现场恢复探针、替代资源或开发流程重跑；原内容流程与 execute 回归通过 | Runner 接线后，阻塞真实启用与跨仓联验 | 通过；正式 L1、5 项固定 EXE 受控、消息 owner 与问答补验，详见下文 |
| PCR-A02 | SteerLine 定向、HTTP/存储/Web 正式集合及现有公开 fixture 闭环；以产品命令触发并读取本轮结果 | isolated / controlled；受测源码见下文 | 按钮、原候选/PR、短正文、少量事实、持久 message、活动/问答/final 接通；无主动冲突检测、额外现场查询或正文业务解析 | 调用方接线后，阻塞产品自动验收 | 通过；owner、HTTP/存储/Web 全集与真实产品命令两轮，详见下文 |
| PCR-A03 | 两仓故障注入与现有受控闭环；覆盖重复点击、保存失败、并发、旧 revision、回执丢失、重启、停止/退出/交付及旧 final；L1/L2 和 `recovery.waiting-merge-restart` 必要回归 | controlled / isolated；同构建重启，固定原请求与受测源码 | 零重复投递、零新资源；原请求可对账；终态无新输入；原 PR/审计和历史回复保留，本次结果不串轮 | 持久输入与展示接线后，阻塞可靠性签收 | 通过；两仓消息/回执故障 owner、同构建重启、正式回归与 recovery-01 |
| PCR-A04 | 既有 live 驱动：驱动准备独立 PR 和冲突→正式产品操作→Agent 解决/验证/推送→正式查询读取结果→再次触发无冲突→现有交付/停止路径 | live；固定构建、profile、模型、权限和命令见下文 | 原 Run/worktree/session/分支/PR 持续使用，真实冲突消除并更新原 PR；无冲突轮可结束，未自动合并；回传与资源结算有证据。Driver 只准备/查证/清理，不代解或代推 | 两仓固定构建及独立测试资源就绪后，阻塞真实链交付声明 | 通过；live-06 和最终 fixture 的 live-07/summary.json 均 passed=true、cleanup_complete=true |

具体命令与新场景登记安排见 Plan 第 3 节。不预先强制新增场景；Runner 受控/原生驱动命令执行前补齐。独立 scope 使用 `C:/SteerlineDebug/<scope-id>/`，报告按[路径清单](../../paths.md)每次独占目录保存。执行后在本文逐项登记日期、精确命令、受测 commit/EXE 摘要、实际结果、原报告路径和未验范围；失败保留，复验另起目录。

## 人工项

| ID | 方法 / 操作 | 准备与参与 | 通过条件 / 阻塞范围 | 状态与证据 |
| --- | --- | --- | --- | --- |
| PCR-M01 | 从待合并候选找到按钮，触发一次操作，阅读活动、问题、待核实提示和最终回复 | 自动闭环通过后；参与人和窗口待安排，使用独立调试包及自有 PR | 操作和结果易理解；只阻塞体验签收，不阻塞自动实现/验证。功能错误回归对应自动项处理 | 待验收；仅规划，无证据 |

提示词实施的实际正文 diff、字节数前后值、原因、验证及独立审核记录随实现 PR 登记；实际变更与独立审核结果在实施后登记。

## 审计修订与实际实现

- 边界修订：Runner 只管 Run 的输入/停止；候选参与、Task 终态留给 SteerLine。复用现有 owner、重启与 live 入口，取消强制新增 L3/L4 框架和多模型组合要求。按钮只放候选详情操作区，避免重复操作入口。
- 复审修复：message 保留 pr_ready 游标，原提示词只冻结为 caller.message；跳过业务审计和正常续作的恢复/现场预检。Orca 保存原结果的 runtime fence 并直接发送。未知消息在 Call 前持久化 ObserveOnly；Web 不补交未知请求。回复区域明确包含历史，不把旧 final 作为本轮回复。
- 独立审核：独立只读 reviewer 完成文档、实现和短提示词/接线审阅；上述发现均直接修正。独立审核通过未代替真实 Runner/PR 验收。

### 实际提示词差异

原资产不存在（0 字节）→ `internal/prompts/pr_conflict_resolution.md` 269 UTF-8 字节（含末尾 LF）。新增完整正文：

```text
解决当前 PR 与目标分支的合并冲突，保留双方修改意图，完成必要验证，提交并推送到原分支以更新原 PR。遇到无法判断的问题时询问用户。简要报告处理结果和验证情况。

PR：{{.PR}}
目标分支：{{.Base}}
```

实际调用黄金样本从不存在（0 字节）→ 288 UTF-8 字节（无末尾换行），使用 `https://example.test/repo/pull/1` 与 `main`。正文组装只在 prompts 包完成，业务传原事实，Runner 不追加行为要求。新增原因是用户主动解决原 PR 冲突；不列 Git 命令、算法或所有边界，让 Agent 按原上下文推理。原提示词、项目规范和回答限制未替换。新增实际投递正文断言、原请求故障测试及复用 live fixture 的双方独立标记；黄金样本用于暴露实际正文，审核由独立 reviewer 完成。

### 验证进展

报告根为 `C:/SteerlineDebug/pr-conflict-5c27/`；以下为实际执行，不回写 Plan 的执行状态。

| 范围 | 实际结果 / 报告 |
| --- | --- |
| 新命令、短正文、SQLite 回执丢失恢复、未知消息只读与 barrier 保存失败 | 定向通过：`caller-owner-04/summary.json`；之后新增 barrier 断言纳入收尾定向回归。 |
| SteerLine 正式 L1/L2 | `L1-02/summary.json`、`L2-01/summary.json` 通过。 |
| Web / 生产构建 | `web-01/summary.json` 通过；生产 TypeScript/Vite 构建通过。初次构建仅测试 DTO 夹具缺必填字段，补齐后通过。 |
| HTTP / 存储 | `http-02/summary.json`、`storage-02/summary.json` 通过。 |
| 原候选重启 | `recovery-01/summary.json` 通过。 |
| Runner | 正式 L1 `runner-L1-01/summary.json`、消息同构建重启 `runner-message-restart.log`、固定 EXE 5 项受控 `runner-controlled-01/result.json` 均通过。最后原生问答补验：`go test ./internal/application -run '^TestDevelopmentRunMessagesRetainOriginalWorkflow$' -count=1`，两种开发流程通过（0.554 秒），只补测试，不改 EXE。 |
| 主干整合复验 | SteerLine 合入 `b3c654ed`，整合提交 `8f104c43`；`merged-L1-01`、`merged-L2-01`、`merged-web-01`、`merged-http-01`、`merged-storage-01` 的 summary.json 全部通过。Runner 整合 `a12b24dc`，整合提交 `9f0b0872`；Orca 适配器全包与消息 owner 通过（`runner-merged-adapter.log`、`runner-merged-message.log`）。 |
| 真实链 | 正式 `smoke.local-codex` 增加真实冲突与无冲突两轮，`live-06/summary.json`（336.5 秒）与最终 fixture `live-07/summary.json`（316.9 秒）均通过，清理完成。使用整合 Runner 固定 EXE，详见下文。 |

保留原失败：普通沙箱拒绝临时 Git 写入；`caller-owner-02/03` 是新 SQLite 夹具缺少原 Spec/Plan 路径和派发准备，修正后 04 通过。`http-01` 现有大回执测试遇 Windows FileRenameInfoEx Access denied，定向 `http-failure-recheck-01` 和正式 02 通过；不改文件发布规则。`storage-01` 与定向 cleanup recheck 暴露现有 ACL 测试故意 deny 后未恢复导致 TempDir 清理失败，仅补 t.Cleanup 恢复原 DACL，原拒绝断言保持，02 通过。

真实前置失败也分别保留：`live-01` 的旧夹具未显式提供 full_access，`live-02` 未声明/准备当前项目环境；均未进入候选业务执行且现场清理完成。夹具现已使用显式原生权限、声明 Git/gh 工具要求，并通过现有正式环境适配入口准备；不伪造 ready 或绕过准入。驱动在推进 base 前核对原候选标记，并用 Git merge-tree 证明真实文本冲突；这仅为外部验收，不进入产品前置。

`live-03` 的环境已就绪，但驱动误用受控 helper 查询真实 Run，改为本次固定 EXE；`live-04` 的原 PR 已就绪，但旧 implement 脚本只暂存 result.txt，未落实 Spec 的候选标记，冲突前断言正确失败，脚本补写并暂存 conflict.txt；`live-05` 的发布成功，但报告 URL 紧邻中文句号被原发布解析器识别成不同候选，尚未进入 message。以上原报告保留且清理完成；不修改生产解析器或放宽冲突断言。

### 真实联验的固定构建与操作

- Runner 整合 commit `9f0b0872ba6524316e1fcad83b19bcdf1af90fce`，EXE `C:/SteerlineDebug/pr-conflict-5c27/bin/steerrunner-merged.exe`，version `0.2.0-dev.4`，SHA256 `0936ce56129e4522a110c67f51ad95b357f35be1de083258af502e5897f4c30b`；来源/source_digest 记录在同名 `.build.json`。首次受控 EXE 的 resource_digest 为 `6da5d3f30de3b0a9a4e7614f79e1d86b2db198232964b6d7aaafad2072d5372e`，不将该旧构建 SHA 冒充整合构建。
- SteerLine 为 `8f104c43372771d34f3edc1922658af807160f32` 加报告所列 live.go 夹具差异；每次正式 summary 记录受测工作树、两侧二进制 SHA、精确构建/执行 argv。profile=`local-codex`，adapter=`codex-app-server`，target=`local`，model=`gpt-6-luna`，effort=`medium`，access_mode=`full_access`，workflow=`development/2`。
- 命令：`./scripts/test-live-delivery.ps1 -SteerRunnerExe C:/SteerlineDebug/pr-conflict-5c27/bin/steerrunner-merged.exe -OutDir C:/SteerlineDebug/pr-conflict-5c27/live-06`。真实环境准备使用原正式适配入口，随后正式 HTTP `resolve_pr_conflict` 两轮，等待原请求和本轮 final，核对原 Run/session/worktree/分支/PR。`scenarios/smoke.local-codex/conflict-proof.txt` 保存冲突证明；`live-conflict-round-1.json`、`live-conflict-round-2.json` 保存公开结果，最终断言原 PR head 包含更新的 base 且双方标记均为 1。
- message 两轮均未合并 PR；之后复用既有驱动显式合并自有测试 PR，再验原 TaskDelivery 与停止/资源清理。此合并是外部验收驱动动作，产品没有自动合并能力。
- 实机范围为上述本地组合；Orca 使用适配器受控测试，其他 Agent/远端组合未在本次逐个实机验证。原生问答由普通/审计 message owner 显式验证，真实冲突 Agent 本轮未提出问题，不能声称实机问答通过。PCR-M01 尚未执行。

最终 fixture 使用相同正式命令，仅 OutDir 改为 `C:/SteerlineDebug/pr-conflict-5c27/live-07`，通过且清理完成。原测试 PR 为 `https://github.com/erranliu/SteerlineTest/pull/779`；首轮 head 从 `48331122044a9efafa80ba342317550e6ceef7c1` 更新为 `399db0ecbdacb61ea2ce257a4a08baef18ec0321`，第二轮保持该 head，两个公开快照均仍是原 open、未合并 PR。本轮 SteerLine EXE SHA256 `6ac3fe77ac7cef23cb8256c6eef93d2b70d1d00ee548e0f26216be8aa264ba86`，测试二进制 SHA256 `d4c27226b6fdc3d7116d5884d2b234a2e3c78d950fd77619e44ad45fede24c84`。

### live smoke 提示词与夹具差异

只在既有 `smoke.local-codex` 隔离 profile 新增两项覆盖（原来未配置，各 0 字节），隔离旧发布 URL 标点问题，保持失败/未知如实报告。不会覆盖生产默认提示词或 audited 夹具。独立审核发现的“无条件成功末行”与“仍要求 JSON”矛盾已修正；最终正文如下（无末尾换行）：

`publish`：0→226 UTF-8 字节：

```text
运行 powershell.exe -NoProfile -ExecutionPolicy Bypass -File ./publish.ps1 一次。成功时，将脚本返回的完整 PR URL 单独写一行，末行单独写「本次发布已完成。」；失败或未知如实报告。
```

`publish.report`：0→170 UTF-8 字节：

```text
只报告已有发布结果，不补做工作。成功时，完整 PR URL 单独写一行，末行单独写「本次发布已完成。」；失败或未知如实报告。
```

对应 Spec/Plan 的实际替换片段（均 ASCII，UTF-8 字节等于字符数）：

```diff
-Stage only result.txt. Do not modify any other tracked file.
+Also change conflict.txt to exactly base=0 candidate=1 followed by a newline. The base flag belongs to the target branch; the candidate flag belongs to this task. Stage only result.txt and conflict.txt; preserve these two independent flags when integrating later target changes.
-which must return the JSON result contract in its native final reply
+which reports the actual publication result
-return strict result JSON in the same native final
+report the actual publication result in the same native final
```

三段分别为 60→278、68→43、50→61 字节。前者让任务 Agent 先产生可证明的冲突，implement.ps1 相应写入/暂存该候选标记；后两者移除与本场景普通发布输出互斥的 JSON 要求。新环境 manifest/脚本只声明并实际核对 Git/gh；产品 conflict message 无环境探针。新增业务提示词、组装/接线、fixture 正文与所有上述修正均经独立只读审核批准，黄金样本不是批准来源。

### 两仓交付

Runner 独立 worktree `C:/Users/lmas1/.codex/visualizations/2026/11/06/01a1af05-6e5e-7d63-b331-9209cbd4bcf7/SteerRunner`，分支 `codex/pr-ready-messages`，[PR 125](https://github.com/erranliu/SteerRunner/pull/125)。SteerLine 分支 `codex/pr-conflict-resolution`，配套提交独立交付；两侧均未部署安装环境。
