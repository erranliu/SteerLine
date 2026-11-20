# PR #463 文档收窄与真实复审

2026-11-20，按用户要求处理[上一轮评审](refactoring-review-source.md)的内容建议。只修改 308 的文档并更新原 PR，没有部署、合并或操作原安装任务。

## 文档更新

在全新独立克隆 `C:/SteerlineDebug/wt-9ed5-pr463-narrow/packages/edit-01/repo` 检出原 PR 分支。原 head 为 `13394163a6d5e5d7935d07a4d1d0fc62099a39cb`，提交 `8c484f37d5dbc0b5de8750bebdeaa19a8fa9df81`（`docs: limit restore lifecycle claims to direct calls`）已正常快进推送到同一分支，无强推。

唯一文件为 `docs/Code/persistence-ownership.md`，只替换一条说明，其余原字节保持。删除跨入口的“不因存档替换而重新初始化”保证，改为：

> 完整恢复通过 `ComposeSession(true)` 从恢复后的存档重新构造 Player、FTUE、World、任务、活动、道具、仓库和商业服务等 Session 级对象。就 `RestoreSessionFromBackup` 的直接调用顺序而言，`PlatformAccountLogin` / `PlayerIdentitySession`、`CloudServiceRuntime` / `RemoteSaveStorage`、`MergeSyncAdapter` 未被列为显式初始化步骤；本条仅描述该方法的直接调用，不据此推断其他恢复入口或间接调用是否重新初始化这些服务。自动采集的常驻兼容对象显式重绑数据。

静态核对实际恢复方法中 `ShutdownSessionOwnership()`、`DB.AppendRaw(data)`、`ReloadAfterDataReplacement()`、`ComposeSession(true)` 的顺序，以及六个引用源码路径；不将直接调用体内的缺省扩展为间接调用或其他入口的生命周期保证。`git diff --check` 通过，远端 PR 文件列表仍只有该 Markdown。未修改代码、资源、测试定义或历史证据，未运行 Unity/业务测试。

原 PR 描述引用了旧提交号，因此在复审前同步为新提交及本次静态核验范围。核验用 API token 的写入请求被服务端以 read:repository 拒绝，未产生修改；随后使用用户既有 `tea` 登录完成同一 PR 的描述更新，没有输出或修改凭据。PR 标题和 WIP/draft 状态保持。

## 真实复审

复用未变化的最终源码评审实现，特性文件摘要与上一轮 manifest 一致。按纯文档变更规则复用已通过的 SteerLine L1/L2，只补本次文档静态核验和新 head 的真实评审，不重复运行无关集合。

使用全新私有 store/Runner/CODEX_HOME，从 report 阶段只读评审 [PR #463](http://192.168.0.66:3000/zymobile/merge-h-308-code/pulls/463)；不导入旧数据库或接管旧 Run，不启动实施或发布流程。

实际入口：`go test -tags=runneracceptance ./internal/app -run '^TestRefactoringPinnedSourceReviewLive$' -count=1 -timeout=15m -json`，输入为 `C:/SteerlineDebug/wt-9ed5-pr463-narrow/packages/edit-01/input.json`。新 head `8c484f37d5dbc0b5de8750bebdeaa19a8fa9df81`、base `9a084fb61a3bfdac79cf30f0ff8b3a1cc280474a`。

- 1 项通过、0 skip，含清理约 263.85 秒，Go 退出 0。不是正式登记的 L4，也不宣称 Unity 或恢复行为验证通过。
- 首次模型 Run `6d4390d8919dd12a2b11ea32bd082ba9` 在 JSON 后附额外内容，格式校验失败；保留原始结果，系统内置的一次 JSON 重试仍绑定同一 head，没有再改文档或 PR。
- 最终 Run `4d96b37a0cefbda5e0bb9ea784ce6763` completed/stopped，产生有效独立报告，建议为 **merge（建议合入）**。评审核对恢复方法、`ComposeSession` 的 `restoringBackup` 分支、正常启动的 `InitializeLogin`、`CloudServiceRuntime.Initialize` 和同步适配器接线，认为新措辞与所查实现相符。
- 评审明确：直接调用顺序不能证明不存在静态状态、事件或间接初始化；新文案已经避免这种推断。合入建议只针对文档与静态核对，不代表运行验证。

证据根为 `C:/SteerlineDebug/wt-9ed5-pr463-narrow/packages/edit-01/`。`review.md`、`result.json` 为新报告；`review-attempts.json` 保留失败与有效两次请求/Run，`source-proof.json` 保留同 head 的源码可用性摘要，`run-observed.json` 为有效原生结果。试图追加读取重试目录时，该目录已按结算流程删除；没有重建目录或重启任务，也没有将这次额外读取算作验证通过。

## 收尾

`pr-before.json` / `pr-after.json` 确认复审期间正文、head/base、标题和 open/未合并状态不变。`cleanup.json` 确认私有 Runner 结算、源码目录回收和认证副本删除。编辑 checkout 保持干净。

`evidence/static-verification.json`、`document-change.patch`、`validation-scope.json`、`final-checks.json` 保存静态范围、实际差异、复用依据和最后核对。原 308 checkout 仍干净、HEAD 仍为 `9a084fb61a3bfdac79cf30f0ff8b3a1cc280474a`；安装 Runner SHA256 仍为 `0d733a1fcd09f06ad7e2891ad90030f16d3fb4fae1b877adc86fbbe1bf74137e`，原任务命令数为 0。PR #463 保留 draft，未合并，未部署。
