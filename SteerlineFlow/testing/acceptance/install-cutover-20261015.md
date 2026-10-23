# 新安装目录迁移与部署验收

日期：2026-10-15。用户授权创建新安装目录，迁移全部三份项目；已确认只迁移定义/依赖，任务统一 open 且暂停，旧状态和交付历史留档。目标 `C:\SteerlineInstall`，旧 `C:\SteerlineRuntime` 正常停机并完整保留。

实施与最小测试安排见 [Plan](../../plans/install-deploy.plan.md)。本记录不把一次性迁移做成长期兼容功能。

## 范围与检查

| ID | 自动检查与通过条件 | 证据 | 结果 |
| --- | --- | --- | --- |
| IC-A01 | 两仓源码构建三个实际 EXE，嵌入 Web；安装 SHA 与构建包相符，execution v3 固定实际 Runner | `.evidence/install-migration/build-deploy.log`、安装目录 deployment.json | 通过：最终实际 BAT 更新退出 0，三个 SHA 与安装文件相符 |
| IC-A02 | 三项目配置、原 Git 目标/分支和三个本机 profiles 保留，旧 web_address 移除；不复制凭据 | `.evidence/install-migration/config-migration.json`、安装目录 etc/execution.json | 通过 |
| IC-A03 | 正式 task validate/create 导入，编号/契约/依赖与导出一致，20 个 open、manual+paused、零候选/Run | 安装目录 `migration/20261015-task-definitions/*.task-batch.json`、validate/create 回执与 verification.json | 通过：SteerLine 0；SteerlineTest 5/4 条边；merge-h-308-code 15/4 条边；逐项一致，SQLite integrity/foreign_key 均通过 |
| IC-A04 | 原状态/已交付证据/旧 Workflow 与 PR 引用留档，旧目录未删除 | 安装目录 migration/manifest.json、各项目 history.json、旧目录 | 通过：原 1 closed、2 in_progress、17 open；3 个旧 Workflow、1 份交付证据 |
| IC-A05 | 新宿主/Web/三项目与三个独立 Runner 服务 ready；实际一键更新后原实例正常退出、数据/方案不变、新实例全部 ready | `.evidence/install-migration/runner-services-before-deploy.json`、安装目录 migration 的停启/部署/验证记录 | 通过：5 个原进程按创建时间核实已退出；clean=true，三个项目资源/锁均结算；新 3 Runner detached/ready；数据/配置/方案摘要与 DB ID 更新前后一致 |
| IC-A06 | PS 语法、差异格式、旧安装目标拒绝、调试禁止使用新安装 Runner | `.evidence/install-migration/protected-target.log`、debug-protection.log；PS Parser、git diff --check | 通过 |

只进行必要安装操作和只读核验，没有在安装目录派发模型任务或 PR 测试，不执行 L2/L3/full。Windows host 定向权限修复由相邻 Runner 仓库的 `docs/acceptance/windows-broker-install.md`（WB-A01–WB-A04）记录。

## Windows 启动修复

自动化宿主 kill-on-close Job 会使直接 start 明确拒绝；一次性桌面计划任务同样受到 Job 限制，已移除。安装启动改由 WMI 独立创建普通用户的隐藏启动进程，未安装持久任务或提高权限。

首次 Runner 启动发现 WMI 额外启用原有特权；旧校验拒绝启动。修复仅在挂起的原返回子进程上禁用这些特权，再严格比对，缺失特权仍拒绝。真实隔离 EXE service start/status/stop 和 Windows token 反例通过；三项目随后 ready。初次失败停机回执不 clean，原失败记录保留；在核实三个 Runner 均 not_running、无 bootstrap、无安装进程、尚无业务 Run 后，单独更新 Runner 并按正式入口重启。最终一键更新必须以正常 clean 停机完成，不绕过该门槛。

## 历史限制

最终安装包：`C:\SteerlineInstall\deployments\20261015-085544-163bcef4`；完整 BAT 输出 `migration/20261015-task-definitions/oneclick-deploy-final.log`、退出码 `oneclick-final-exit.txt`（0）、逐进程与摘要核验 `deployment-verification.json`、数据核验 `verification.json`。构建使用 SteerLine `dee6ad82` 工作区和 Runner `af545563` 工作区，安装时含本次尚未提交的改动，dirty 状态已如实记录；后续源码提交不改写该历史构建身份。旧目录无活动服务；Gitea token 保留原 User 环境变量引用，未复制值。首次更新的短暂 Windows rename 拒绝保留在 oneclick-deploy.log；最终重跑通过，脚本对相应文件占用错误有有限重试。

SteerlineTest 任务 101 原已交付（PR #250）；102/105 在导出时分别等待 #252/#251 合并。新安装的这些任务按用户选择回到 open，暂停派发；旧 PR 不由迁移修改。恢复派发前先处理旧任务的实际业务状态。未构造假的 Runner Run，也未恢复旧执行；完整执行日志仍在旧目录。安装验证不能替代此前本机 Codex 实际交付/恢复的待验或失败结果，Orca 验收仍按用户要求取消。


## 提 PR 前定向复验（2026-10-15）

仅复验本次脚本与权限改动，不重复实际迁移、不重启安装服务。部署预检使用原脚本的拒绝分支，不创建或写入安装/调试目录。

- 原安装工作区的三个脚本 Parser 检查通过；同步主干 PR #327 后，旧 `debug-runtime.ps1` 按主干删除，不复活退役入口。最终 PR 对仍存在的 `build-deploy.ps1`、`new-debug-release.ps1` 重新执行 PowerShell 5.1 Parser 检查，通过。
- 7 个预检反例：BAT 传入旧安装目录、PS 传入调试目录/两个源码目录/相对目录、新旧安装版 Runner 被调试生成器引用；全部退出非零并匹配预期拒绝原因，BAT 的 `-NoPause` 与退出码传递一并核验。
- 证据：`.evidence/install-migration/pr-preflight.log`、`pr-preflight-summary.json`；这些是本机原始记录，不包含凭据，也未把 EXE、安装数据或私有历史上传仓库。
- Runner Windows host 的两项测试与一个子场景通过、零 skip，新增反例验证缺少启用权限时拒绝且不会替子进程启用权限；由其专属验收记录结果。
- `git diff --check` 通过；此前实际 BAT 更新与迁移结果继续只归属上文的确切构建，不作为业务模型/恢复验证。

主干同步：PR #327 合入后的 modify/delete 冲突按主干删除 `scripts/debug-runtime.ps1` 解决；本 PR 不修改退役脚本或重新提供旧隔离入口。测试说明沿用主干新的隔离夹具入口，仅补部署导航；首次安装包及旧版本结果保持原身份。
