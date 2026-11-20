# 308 项目真实文档维护流程测试

2026-11-19，用户要求在 `merge-h-308-code` 上执行真实“更新过期文档”流程。使用独立应用 home、完整 Git 对象克隆、本机真实 Codex 和修复历史索引启动策略后的固定 Runner。原安装应用、原任务 `f292bdcfcf4b4854ad10819f0c4834a9` 及 `C:/Projects/merge-h-308-code` 不作业务修改，不部署、不合并。

本页保留修复前的失败事实。用户后续授权修复后，真实流程已完成并创建 draft PR #463，见[修复与补测](refactoring-308-docs-fixes.md)；下方原失败不改报通过。

本次为经公开应用 API 发起的人工 live 专项，不冒充已登记 L4 `refactoring.local-codex`。该正式场景使用 GitHub 小型代码夹具；本次使用真实 308/Gitea、Git LFS 和真实项目规则，结果分开记录。

## 最终结果

**未通过完整交付。** 聚焦行动 `a02710763d334c8c91b3a65ef5657e24` 完成真实选题、实施与独立验收，最终被可信成功门禁阻断；没有发布 PR。

- 基线：`AIRefactor` 的 `9a084fb61a3bfdac79cf30f0ff8b3a1cc280474a`。正式环境检测通过项目工具、Android Editor 模块和 Unity batchmode 授权；这不代表 Unity 业务测试通过。
- 唯一文档目标：`docs/Code/persistence-ownership.md`“完整远程备份恢复”中的过期 CloudLogin 名称和生命周期表述。核对当前 `CloudServiceRuntime`、`PlatformAccountLogin`、`PlayerIdentitySession` 和恢复时重建 Session 的实现。
- 实施提交：`4635a99802ca77e47fd6b9c615b8f700c16e87d4`，`docs: clarify backup restore lifecycle`。实际提交仅改该文档一行，保留其余恢复约束、历史证据、代码、测试定义和资源。`git diff --check` 退出 0；执行者报告已核对源码路径和符号，没有宣称运行 Unity/业务测试。
- 原文：“Player、FTUE、World、任务、活动、道具、仓库和商业服务由同一恢复后的存档重新构造；App 级 SDK、CloudLogin、R2 凭据和同步适配器不重新初始化。自动采集的常驻兼容对象显式重绑数据。”
- 新文：“App 级云服务配置（`CloudServiceRuntime`）与同步适配器继续使用；身份由当前 `PlatformAccountLogin` / `PlayerIdentitySession` 状态管理。持久化 owner 保留并重载被替换的数据树，Player、FTUE、World、任务、活动、道具、仓库和商业服务等 Session 所属服务从恢复后的存档重新构建。自动采集的常驻兼容对象显式重绑数据。”
- 真实实施 Run `07b20312956e1cd65abd2566428a4b09` 完成，独立审计 Run `c23be2f2d5250760dba3fbd8b22f33ce` 建议接受；系统检查仍发现 `?? dev/`，以“原执行缺少可信成功证据，需要补证或修复”阻断。接受建议没有绕过工作区干净门禁。

## 已复现的 Runner 阻碍

Runner `internal/execution/workspace/prepare.go` 创建 worktree 时使用 `-c core.hooksPath=/dev/null`。在本机 Windows/Git LFS 上，会在新 worktree 的 `dev/null/` 中生成 `post-checkout`、`post-commit`、`post-merge`、`pre-push` 四个 Git LFS hook，导致工作区初始就有未跟踪内容。

独立本地小型 LFS 夹具采用相同 worktree 命令，无模型、无外部网络，复现 `?? dev/` 和全部四个 hook；证据为 `C:/SteerlineDebug/wt-9ed5-308-docs/diagnostics/lfs-hooks-01/result.json`。这不是用户原工作区遗留文件。没有为通过测试而删除实际工作区内容、改变干净判定或手工发布 PR；本次只记录测试，不修改该处生产代码。

## 其他尝试与证据边界

全部输出根为 `C:/SteerlineDebug/wt-9ed5-308-docs/`，失败不覆盖、不改报通过。

1. `packages/run-01`：测试准备使用部分克隆，首轮快照从本地 Git 获取缺失对象失败，尚未发起选题模型。另有一次环境检测请求字段错误被明确拒绝，随后改用协议字段的新请求通过检测。两者都是本次测试驱动准备问题，不归因于 308 原任务或产品。
2. `packages/run-02`：改为完整对象克隆。真实选题 Run `210c3d5fdeb67386346f98ade4c3c612` 原生执行成功，选中 CloudLogin 过期说明，但返回 `question:null`，Runner 按维护 Schema 的 object 类型校验失败。原始 final 保存在 `evidence/first-run-inspect.json`。系统内置的一次重试在新快照 checkout 上耗尽 30 秒；该时限来自本次测试包漏配 `command_timeout`，不是 308 原配置。原安装项目的时限为 `2m`，后续已对齐，不能将这个超时计为原项目缺陷。
3. `packages/run-03`：完整克隆、对齐原项目 `command_timeout=2m` 及轮询/心跳/关闭时限。通用调查行动 `0d214445925241b59bb11378d459f4a2` 正常返回无需改动，终态 delivered、无 PR；不计为文档修改链通过。独立核对 CloudLogin 候选后，以具体文档目标创建上述聚焦行动，推进到实施和独立验收，再触发已复现的 LFS 工作区阻碍。

因此，真实模型启动已有成功证据；本次不能宣称文档 PR 交付通过，也不能用后继成功选择覆盖此前 `question:null` 失败。

## 版本、保存与清理

- SteerLine 基线 `3a1777a6bbf5d4b245be2ca12aa24c4b2af4e905` 加当前 blocked 展示修复，固定程序 SHA256 `ff24f5712b17771d7e966f70959a324e8993d124dbd488b4c4d53b396fc1f724`。
- Runner 基线 `8ebbc717776eb2ad85371060566776c4402773fa` 加索引复用/初始化锁修复，SHA256 `71fbe6336366e87181b5a735dd2ea201ecc3542189889339995b7dcdacd1fa6e`。完整 L1/Controlled 的原失败仍按[启动专项](refactoring-runner-blocked.md)及 Runner 验收记录保留，本次不替换其资格。
- 最终证据位于 `packages/run-03/evidence/`：`test-result.json`、`targeted-start-request.json`/`targeted-start-receipt.json`、`targeted-view-*.json`、三个 `runner-<RunID>.json`、`document-change.patch`、`persistence-ownership.updated.md`、`verify-stop.json` 和 `cleanup-summary.json`。补丁与完整文档已从实际提交保存，不依赖运行工作区长期存在。
- 三个隔离 home 均经正式 `stop` 和 `verify-stop` 确认原生/Runner 资源结算，私有认证副本已删除。PR 未创建，无需关闭远端 PR 或删除远端分支。
- 原 308 工作区仍干净，HEAD 仍为 `9a084fb61a3bfdac79cf30f0ff8b3a1cc280474a`；安装 Runner SHA256 仍为 `0d733a1fcd09f06ad7e2891ad90030f16d3fb4fae1b877adc86fbbe1bf74137e`，未部署，未向原任务发出命令。

本次未改变生产代码、固定提示词、Schema 或正式测试定义。新记录的本地链接和 `git diff --check` 已核对。
