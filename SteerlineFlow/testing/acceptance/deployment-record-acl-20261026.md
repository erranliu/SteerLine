# 2026-10-26 部署恢复记录的 Windows ACL 修复

用户授权普通缺陷修复及实际安装维护，不启动 roadmap。基线为 `0ab5e3a59077776fd2969a7941590b1467cf94ba` 加本次修改。自动回归使用隔离 home/公开 Runner fixture；安装环境只用于正式维护。

## 原因与修复

失败包 `C:\SteerlineInstall\deployments\20261026-061917-3400926a` 在替换安装文件前被阻止。候选 Runner 的 87 项接管预检全部 compatible。系统事件 1074/6006/6005 与 OS 启动时间确认北京时间 13:48–13:49 重启；最新应用实例创建于 12:13，原 shutdown.json 却属于 12:12 的前一实例。因此正常停止和离线核验均不能复用旧回执，需要进入既有中断恢复。

中断恢复通过 `ReadPrivateJSON` 读取 `<home>/deployment.json`，但部署脚本以普通继承 ACL 写入该文件。真实文件的 DACL 未保护，并含 Administrators、SYSTEM、Users、Authenticated Users 继承项，触发 `local API Windows ACL inherits access from another directory`。既有 Go 恢复 fixture 直接使用私有写入器，部署脚本 fixture 又未核对记录权限，因而漏测写入与读取的差异。

本次将现有路径/JSON 文件工具提取到 `scripts/deployment-files.ps1`，仅安装根 deployment.json 选择私有写入：在临时文件创建时附加当前用户所有、禁用继承、单条 FullControl ACL，写完 flush 后用 MoveFileEx 原子替换，保留新文件的 ACL；不能使用会保留旧目标 ACL 的 ReplaceFile。兼容本项目实际入口 Windows PowerShell 5.1 和 PowerShell 7。失败保留原文件并清理临时文件。恢复拒绝诊断附带具体 deployment.json 路径。

停机身份、业务原始记录、原生资源和候选兼容性校验保持原要求，不新增历史格式读取或降级通道。

## 自动验证

证据 scope：`C:\SteerlineDebug\wt-1e3d-deployment-acl`。

| 范围 | 命令 / 证据 | 结果 |
| --- | --- | --- |
| PowerShell 写入 / 真实 Go 恢复读取 | `go run ./cmd/test-feedback -out <scope>\diagnostics\20261026T072813331Z -- ./internal/app -run '^TestDeploymentRecordScriptMeetsRecoveryACL$' -count=1 -timeout=120s` | 通过，24.110s wall；PS5/PS7 均覆盖新建、替换普通继承 ACL 文件、替换私有文件，以及 Windows 共享锁拒绝替换时原内容/权限保持、临时文件清理。 |
| 实际部署脚本隔离夹具 | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-build-deploy-pipeline.ps1 -OutDir <scope>\diagnostics\pipeline-20261026T072829497Z` | 8 项通过；成功部署实际写出的记录逐次核对 owner、禁止继承及唯一当前用户 FullControl。 |
| 原停止脚本门禁 | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-deployment-stop.ps1` | 通过；单次 Go 准备调用、调用号/包绑定、pin 复用和接管门槛保持。 |
| 固定 L1 | `<scope>\test-results\L1\20261026T072931348Z\summary.json` | passed=true，准备 0.3s、执行 6.4s。 |
| 固定 L2 | `<scope>\test-results\L2\20261026T072931348Z\summary.json` | passed=true，full-L2，准备 15.320s、执行 72.263s，cleanup_complete=true。 |
| 中断恢复及旧失败回执围栏 | `<scope>\diagnostics\recovery-20261026T072942210Z\summary.json` | 42 项通过，零失败/跳过，complete=true，149.632s wall；涵盖原宿主/launch/锁/Runner writer/旧回执/缺失记录/配置/原输入/三代构建及失败恢复边界。 |
| 文档及空白检查 | `python scripts/check_doc_links.py`、`git diff --check` | 587 份 Markdown、3662 个本地导航链接零错误，无空白错误。 |

## 实际维护

维护证据目录：`C:\SteerlineInstall\maintenance\deployment-acl-20261026`，限制为当前用户访问。原部署记录及 SDDL/文件摘要、执行配置、原宿主实例/launch/失败停机回执/Runner pin 已归档。维护前只读快照核对三个项目共 20 个任务、22 个 Candidate、65 份生成记录，SQLite integrity/foreign-key 均通过。系统事件及启动时间保存于 `reboot-events.json` / `os-boot.json`。

持有安装 `.deploy.lock` 并核对记录摘要/原 owner 后，仅修正原 deployment.json 的 ACL 为当前用户所有、禁用继承、单条 FullControl；修复前后内容 SHA256 完全一致，`acl-repair.json` 保存新 SDDL 及摘要。

日常源码 `C:\Project\SteerLine` 与本工作树 HEAD 相同且原先干净，窄范围同步本次七份源码/测试/维护文档，清单保存在 `synced-files.json`。随后沿正式 `scripts/build-deploy.ps1 -InstallHome C:\SteerlineInstall -RunnerRepo C:\Project\SteerRunner -UpdateRunner -NoStart -NoPause` 执行；未修改 Runner 原有 docs 脏状态。

## 重试揭示的独立 schema 阻塞

新包 `C:\SteerlineInstall\deployments\20261026-073355-64081c32` 的构建及接管预检通过，停止/恢复阶段不再报告 ACL 错误，转而报告 `unsupported_schema: unsupported SQLite schema 31; only schema 32 is accepted`。部署 exit 1，仍在安装替换前停止；没有完成部署或启动。完整证据为 `build-deploy.log`、`deploy-result.json`、`deploy-timings.json` 和包内预检。

`snapshot.py installed` 通过：重试前后三个项目数据库及所核对业务记录完全一致，现有三个 EXE 仍匹配原 deployment.json，profiles 保持。此处文件名 state-installed.json 仅沿用快照工具的比较模式，不能解读为部署成功。

只读比较原安装源码 `80f98a37…` 和当前源码：schema 32 只新增 `project_environments` / `environment_requests` 两张表，其余既有表/索引/触发器定义未变；三个实际数据库均与固定 schema 31 定义相符，新表均不存在。`schema-upgrade-preflight.json` 保存检查结果，`schema31-definitions.json` / `schema32-additions.json` 保存固定定义。

首次重试后向用户明确说明 AGENTS.md 的旧数据迁移授权要求，在未获授权时保持数据库及原安装文件不变。用户随后明确选择“允许一次性升级并继续部署”；以下维护按该授权执行，不新增产品自动兼容/迁移逻辑。

## 用户授权的一次性 schema 升级

授权回执为维护目录 `schema-upgrade-authorization.json`。使用独立同用户隐藏 WMI 进程执行 `recover-upgrade.ps1`，持有安装 `.deploy.lock`；数据库备份/升级时另持 startup-coordination 的同一字节范围锁。

1. `backup-original/` 保存三个完整 SQLite 备份；`backup-original.json` 保存各备份 SHA256 及所有原表的行数/内容摘要，备份后核对完整性与逐表内容。
2. 在 schema 31 尚未改动时，用原安装 CLI 执行正式 `recover-deployment --runner-exe <073355 包 Runner> --json`。原安装正常启停并产生新宿主停机证明，`original-recovery.json` 保留旧失败回执和新恢复证明，restartable=true。
3. 再保存 `backup-preupgrade/` 完整备份及逐表摘要。每个备份先复制到独立 rehearsal 数据库，事务内新增两张空表并登记版本 32，核对所有原表逐行摘要完全一致、完整性/外键通过，原备份不变；结果为 `upgrade-rehearsal.json`。
4. 对三个安装数据库分别执行同一事务，核对原表内容完全一致；`schema32-applied.json` / `schema-upgrade.log` 确认三个 schema 32、两张新增表为空。`schema32-verify-restart.json` 为当前构建的正式离线验证：restartable=true，三个项目通过；原生资源结算仍为 false，没有将历史未知提升为已结算。`recover-upgrade-result.json` 为 exit 0。

`recovery-table-comparison.json` 另与维护最初的 state-before.json 核对：三个项目共 20 个任务、22 个 Candidate、65 份生成记录及契约/依赖/派发授权逐条相同。

## 最终部署及启动

沿同一正式 build-deploy 入口，以 `-UpdateRunner -NoStart` 完成新包 `C:\SteerlineInstall\deployments\20261026-075736-13c2b3d2`，exit 0；`schema32-deploy-timings.json` 为 succeeded=true、78.26s（仅此次构建/部署，不含此前原安装恢复、备份升级及后续启动）。SteerLine 源码为本记录基线加修复，Runner 源码为 `74d594c5f00e32e97d2ee911db5ac787302022bc-dirty`，原有 Runner docs 脏文件未修改。

包内 shutdown-verification.json 为 prepared / restart_verification，host_exit/project_closure/writer_exit/compatibility 均 verified，native_resources 保留 unverified。安装三个 EXE 与新 deployment.json 摘要一致，实际由修复脚本生成的安装记录 DACL 受保护；`installed-verification.json` 保存结果。

替换后、启动前的 `snapshot.py prestart` 通过，state-prestart.json 证明任务/契约/依赖/派发授权逐条保持，Candidate/生成记录集合及 Run/input/提交信封/原构建/来源/PR 绑定保持，执行 profiles 不变、SQLite integrity/foreign-key 通过。

随后用独立同用户隐藏进程执行正式 `start --no-open` 并等待全部项目 ready。`updated-start-result.json` 为 exit 0；`status-final.json` 核实后端/Web ready、三个项目 ready 且 online、degraded=false；`http-final.json` 为 HTTP 200。最终 `snapshot.py started` 通过并保存 state-started.json，再次核对上述全部原业务绑定及数据库完整性。没有通过重建任务、清库或伪造停机回执恢复运行。

源码修复和维护记录已同步回日常源码目录。L1/L2 与定向自动回归结果见上表；一次性维护使用真实安装环境，不宣称完成真实模型/PR 专项或原生资源结算验收。
