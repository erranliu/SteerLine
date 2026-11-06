# 待合并 PR 冲突解决验收规划

2026-11-06。关联 [Spec](../../spec/pr-conflict-resolution.spec.md) 与 [Plan](../../plans/pr-conflict-resolution.plan.md)。普通开发，无任务编号。审计修订后实施中，待验收；用户已授权两仓实现和 Runner 独立 worktree/PR，不部署安装环境。

## 自动项

| ID | 方法 / 操作 | 环境与构建 | 通过条件 | 安排 / 阻塞范围 | 状态与证据 |
| --- | --- | --- | --- | --- | --- |
| PCR-A01 | Runner 定向受控测试；观察固定 EXE 的目录，再在原开发 Run 连续发送两次 message，其中一次经过原生问答 | controlled；Runner commit、EXE SHA/version/resource_digest 和命令实施时固定 | 公开受理与 can_message 一致；原工作区/会话/配置保持，轮次结果独立；无现场恢复探针、替代资源或开发流程重跑；原内容流程与 execute 回归通过 | Runner 接线后，阻塞真实启用与跨仓联验 | 待验收；无执行证据 |
| PCR-A02 | SteerLine 定向、HTTP/存储/Web 正式集合及现有公开 fixture 闭环；以产品命令触发并读取本轮结果 | isolated / controlled；SteerLine commit 与 fixture 版本执行前固定 | 按钮、原候选/PR、短正文、少量事实、持久 message、活动/问答/final 接通；无主动冲突检测、额外现场查询或正文业务解析 | 调用方接线后，阻塞产品自动验收 | 待验收；无执行证据 |
| PCR-A03 | 两仓故障注入与现有受控闭环；覆盖重复点击、保存失败、并发、旧 revision、回执丢失、重启、停止/退出/交付及旧 final；L1/L2 和 `recovery.waiting-merge-restart` 必要回归 | controlled / isolated；同构建重启，固定原请求与受测源码 | 零重复投递、零新资源；原请求可对账；终态无新输入；原 PR/审计和历史回复保留，本次结果不串轮 | 持久输入与展示接线后，阻塞可靠性签收 | 待验收；无执行证据 |
| PCR-A04 | 既有 live 驱动：驱动准备独立 PR 和冲突→正式产品操作→Agent 解决/验证/推送→正式页面/查询读取结果→再次触发无冲突→现有停止路径 | live；固定两仓 commit/Runner EXE；本次实际使用的开发组合，profile/模型/权限与完整命令执行前固定 | 原 Run/worktree/session/分支/PR 持续使用，真实冲突消除并更新原 PR；无冲突轮可结束，未自动合并；回传与资源结算有证据。Driver 只准备/查证/清理，不代解或代推 | 两仓固定构建及独立测试资源就绪后，阻塞真实链交付声明 | 待验收；无执行证据 |

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
| Runner | 正式 L1 `runner-L1-01/summary.json` 通过；新消息同构建重启定向 `runner-message-restart.log` 通过；固定 EXE 受控专项执行中。 |
| 真实链 | 正式 `smoke.local-codex` 增加冲突与无冲突两轮，执行中。固定 Runner EXE 版本 `0.2.0-dev.4`，来源 `22cc3751f1d618f315ebbda27595b06a0c79a544` 加本 worktree 变更，SHA256 `49c30ed6e93171d21008392d8960242ac37a7d149fa1ecf35231646a5645fcc4`，构建记录 `bin/steerrunner.exe.build.json`。 |

保留原失败：普通沙箱拒绝临时 Git 写入；`caller-owner-02/03` 是新 SQLite 夹具缺少原 Spec/Plan 路径和派发准备，修正后 04 通过。`http-01` 现有大回执测试遇 Windows FileRenameInfoEx Access denied，定向 `http-failure-recheck-01` 和正式 02 通过；不改文件发布规则。`storage-01` 与定向 cleanup recheck 暴露现有 ACL 测试故意 deny 后未恢复导致 TempDir 清理失败，仅补 t.Cleanup 恢复原 DACL，原拒绝断言保持，02 通过。

真实前置失败也分别保留：`live-01` 的旧夹具未显式提供 full_access，`live-02` 未声明/准备当前项目环境；均未进入候选业务执行且现场清理完成。夹具现已使用显式原生权限、声明 Git/gh 工具要求，并通过现有正式环境适配入口准备；不伪造 ready 或绕过准入。驱动在推进 base 前核对原候选标记，并用 Git merge-tree 证明真实文本冲突；这仅为外部验收，不进入产品前置。
