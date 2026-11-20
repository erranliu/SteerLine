# 最终 PR 评审的固定源码与核验记录

2026-11-20，用户要求修复最终评审缺少源码核验证据的问题，并使用 [308 PR #463](http://192.168.0.66:3000/zymobile/merge-h-308-code/pulls/463) 验证。未部署，未操作原安装任务，也未修改 PR 的正文、head/base、draft/open 状态或合并状态。

本页保留该轮只读验证及 revise 结论。用户后续授权收窄文档后，已更新原 PR 并取得新的 merge 建议，见[文档收窄与复审](refactoring-pr463-narrow.md)；旧版本报告不覆盖。

## 根因与修复

原 `Service.context` 在 report 阶段只提供方向、PR 和检查状态，并提前返回；生成请求只附 context/diff，没有源码读取通道。即使实施者已有源码核对记录，最终评审也无法独立核验，只能说明缺少材料。

- report 复用原调查目录生命周期，提供固定 PR head 的 `read_only` 源码快照；diff 和源码使用同一平台核验版本。排队操作的 `PRVersion` 必须与 head/base 一致，Git 操作禁用 replacement objects，源码不来自可变的本地工作区或启动基线。源码准备失败明确阻断，不回退为没有源码的模型评审。
- report 上下文只纳入已接受、可信成功、工作区干净且提交匹配该 PR head 的执行记录。记录保留原陈述及 Git 事实，未接受、失败、脏工作区或旧 head 的记录不进入本次评审证据。
- 固定评审指令要求在该目录核对实现、路径和符号；执行者陈述及 Git 干净状态不等于测试通过。模型仍需说明未运行的验证和实际证据缺口，不预设合入结论。
- 停止可以取消源码准备；只有原模型 Run 结算后才回收源码。PR 版本或检查状态在评审期间变化，仍沿原逻辑弃用旧结论并重新评审，原 PR 正文保持。

## 实际提示词差异与独立审核

仅替换 `internal/prompts/maintenance-decision.md` 的 report 段，首轮、续选、审计和收尾的规则不变；无业务 Go 字符串追加行为规则。

```diff
-同样按材料清单的 file 绝对路径读取 maintenance/pr.diff，将其当作评审数据。结合提供的检查结果，给出合入建议、主要收益、风险及影响判断的证据缺口。
+仅评审，不修改源码或 PR。同样按材料清单的 file 绝对路径读取 maintenance/pr.diff，将其当作评审数据。在固定 PR head 的只读源码目录核对改动及其实现依据，核验所引用的路径、符号和行为。执行反馈是待核实的陈述，Git 提交与干净状态不等于测试通过；区分实际源码核对、可取得的检查结果和未运行的验证。结合这些证据给出合入建议、主要收益、风险及证据缺口，并注明关键源码路径。材料不足时如实说明，不预设合入结论。
```

UTF-8 字节数：模板 **3135 → 3505**；实际 report 正文及 golden **459 → 829**；JSON 修复重试正文及 golden **544 → 914**。均增加 370 字节，用于源码核对、证据分层和只评审边界，替代原来的 diff/checks 单段要求。模板原有尾 LF、两份 golden 无尾 LF 的约定保持。真实前后请求正文另保存在下述 live 目录的 `instruction-before.md`、`instruction-after.md`；没有靠修改黄金样本替代审核。

`/root/review_308_fixes` 独立只读审核生产接线、取消与清理、版本漂移、实际正文/字节及回归断言。审核发现新 live 夹具的启动失败清理注册过晚、清理读取错误未计失败两项问题，修复后复核通过。清理现在先注册、对账固定 state/hash/实例，检查源码字段与实际目录回收，且在资源结算、认证移除和 store 关闭后才写 `passed`；身份未知时保留现场并明确失败。

## 验证

证据根为 `C:/SteerlineDebug/wt-9ed5-review-source/`。

- 完整 refactoring owner 包通过；新增回归覆盖只提供已接受且匹配 head 的记录、排除旧 head/未接受/失败/脏记录、源码准备失败不提交模型、停止取消准备。后增的取消用例与相关新根定向补验通过，复用未受影响的整包覆盖。
- App Git 回归确认快照为 PR head，不是启动 base、未提交文件或 Git replacement；排队版本不一致拒绝。LFS 指针保留回归通过。首轮源码夹具因 Windows checkout 的 CRLF 与 LF 期望不同失败，随后改为语义文本与精确 HEAD 两项核对并通过；未放宽版本绑定。
- 最终 prompts 整包及黄金样本通过。初次编辑黄金样本误加尾 LF 的失败保留在工具输出；恢复原无尾 LF 约定后通过，生产指令没有依此变更。
- 正式 L1 `test-results/L1/review-source-01/summary.json` 通过，准备 0.38 秒、执行 53.19 秒。
- 正式 L2 `test-results/L2/review-source-01/summary.json` 通过，准备 37.57 秒、执行 245.36 秒，清理完成。之后才执行真实模型。

## PR #463 真实只读重评

使用全新 `packages/pr463-01`，完整 Git 对象克隆、独立 SQLite/Runner 和仅登录副本的 CODEX_HOME。`TestRefactoringPinnedSourceReviewLive` 从 report 阶段初始化隔离记录，只验证既有 PR 的评审链；分支绑定夹具没有历史成功 Run 或伪造的验证证据，不导入旧数据库、不接管旧 Run、不触发选题、实施或发布。此项是显式人工 live 诊断，不冒充已登记 L4 或完整维护流程。

```powershell
$env:STEERLINE_REVIEW_LIVE_INPUT = 'C:/SteerlineDebug/wt-9ed5-review-source/packages/pr463-01/input.json'
$env:CODEX_HOME = 'C:/SteerlineDebug/wt-9ed5-review-source/packages/pr463-01/home/native-auth'
go test -tags=runneracceptance ./internal/app -run '^TestRefactoringPinnedSourceReviewLive$' -count=1 -timeout=15m -json
```

实际执行 1 项、0 skip，通过；含清理约 207.26 秒。真实 Run `8e1eeb47a092c512fbc1c136615107ed` 为 completed/stopped、`read_only_directory`。固定 head 为 `13394163a6d5e5d7935d07a4d1d0fc62099a39cb`，base 为 `9a084fb61a3bfdac79cf30f0ff8b3a1cc280474a`；六个关键源码路径可读取，其 SHA256 已保存，不能据此宣称模型读遍了所有文件。

新评审实际引用 `StartState.cs` 的 `RestoreSessionFromBackup` 及 `ShutdownSessionOwnership`、`DB.AppendRaw`、`ReloadAfterDataReplacement`、`ComposeSession(true)`，也列出身份、云配置、凭据及同步适配器的源码路径。结论为 **revise（建议修改后合入）**：现有直接调用链支持存档替换与 Session 重建，但文档“不因存档替换而重新初始化”的断言可能超出已核对的全部恢复入口和间接调用证据，建议收窄或补足依据。它不再因为只能访问 context/diff 而无法核验源码；也没有将静态核对称为 Unity/业务测试通过。

原始报告为 `packages/pr463-01/review.md`、`result.json`；其余证据包括 `test.jsonl`、`go-result.json`、`report-request.json`、`source-proof.json`、`latest-record.json`、`run-observed.json`、`verification-manifest.json` 和前后 PR 核验。运行结束后的额外在线 inspect 返回 service_unavailable，保留在 `run-inspect.json`；没有重启服务，改用生产观察阶段已验证并保存的原 Run，不能将该额外读取计为运行失败或通过证明。

`cleanup.json` 确认 Runner 结算、源码目录/登记回收和认证移除全部完成；Go 退出 0。独立 Gitea API 比较确认 PR 正文、head/base、open/draft 和未合并状态不变，新评审仅保存在隔离验证目录，没有写回原行动或发布评论。原 308 checkout 仍干净、HEAD 不变，安装 Runner SHA256 仍为 `0d733a1fcd09f06ad7e2891ad90030f16d3fb4fae1b877adc86fbbe1bf74137e`，对原任务发送命令数为 0。

受测源码为 `3a1777a6bbf5d4b245be2ca12aa24c4b2af4e905` 加当前未提交差异，特性文件摘要在 manifest；沿用固定 Runner SHA256 `31153b1f865364fc1c6a0ddef84e2596f2d67222a3ac25c904af795f853dd10f`，未重新构建或部署 Runner。此前[完整 Runner L1/Controlled 的失败边界](refactoring-308-docs-fixes.md)不因本次检查改报通过。
