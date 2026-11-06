# 部署前安装目录访问权限修复

2026-11-06，普通开发，基线 `01356834`；用户报告 `build-deploy.ps1:75` 的 `Test-Path C:\SteerlineInstall\bin\steerline.exe` 拒绝访问。初轮修改源码和隔离夹具；用户随后要求同步日常源码并重新部署，并在解释 DiscardRuns 副作用后明确授权，实际维护见末节。

## 原因与修复

只读检查实际安装目录确认：根目录 owner 为当前用户，DACL 为受保护的当前用户单条 FullControl，但规则没有目录/文件继承标志；`bin`、`etc` 的访问规则列表为空，三个 EXE 的权限查询和 bin 枚举均拒绝访问。

`billingconfig.Store.change` 调用 `httpfs.EnsurePrivateDirectory(s.Home)`，将共享安装根目录设为本应只用于私有 API 子目录的无继承 ACL。Windows 随后移除依赖父目录继承的子项访问权。部署脚本的 EXE 存在性检查首先暴露错误，并非 EXE 缺失；以忽略错误替代访问核验会绕开已有安装判断。

本次价表保存仅按需创建 home，保留已有 home ACL；价表仍使用私有临时文件、条件写、跨进程锁和原子替换。新增 Windows 回归覆盖连续保存及切换 active 版本后，安装根、三个 EXE、项目配置、数据库和私有 API 目录的 ACL/内容不变，billing.json 仍满足私有文件权限。

部署在原 EXE 检查前匹配上述确切形态：当前用户拥有、受保护、单条当前用户 Allow FullControl、无继承及传播标志。仅为该根规则增加 ContainerInherit/ObjectInherit，不改 owner、不向其他身份授权、不递归重置子项 ACL。受保护生命周期记录不继承，保持原权限；其他 ACL 不自动更改。使用 .NET 仅写 DACL，避免 PowerShell `Set-Acl` 额外索取普通用户没有的 SeSecurityPrivilege。未变更停止、资源结算、Runner 升级及 DiscardRuns 的业务门槛。

## 验证

证据根：`C:\SteerlineDebug\wt-380f-deployment-home-acl`。正式通过与中间失败分别保留在独立目录；源码包含本次未提交修改，不导出干净提交的受管历史记录。

| 范围 | 命令与证据（相对证据根） | 结果 |
| --- | --- | --- |
| 价表 owner 全量 | `go run ./cmd/test-feedback -out <证据根>/diagnostics/billing-20261106T080420540Z -- ./internal/billingconfig -count=1`；该目录 `summary.json` / `go-test.jsonl` | 通过，3.376 秒；含新增 Windows 安装权限回归。 |
| 权限边界 | 分别用 `powershell.exe` / `pwsh.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-deployment-home-access.ps1 -OutDir <全新目录>`；`diagnostics/acl-ps5-20261106T080815474Z/result.json`、同时间的 `acl-ps7-.../result.json` | 两版本各 6 项通过；复现查询拒绝、恢复访问、受保护记录不变及其他规则不改。 |
| 完整假部署连接 | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-build-deploy-pipeline.ps1 -OutDir <证据根>/diagnostics/pipeline-20261106T080815474Z`；该目录 `result.json`、各场景日志 | 15 项通过；包括错误 ACL 下的真实入口 DiscardRuns 前置检查、后续停机/安装/NoStart；无真实安装。 |
| 原停止/升级门禁 | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-deployment-stop.ps1` | 通过；单次 Go prepare 调用、包/调用身份、pin 复用和升级门槛保持。 |
| 固定 L1 | `go run ./cmd/test-feedback -level=L1 -out <证据根>/test-results/L1/20261106T080815474Z`；`summary.json` | passed=true、complete=true，准备 0.3 秒、执行 19.1 秒。 |
| 固定 L2 | `go run ./cmd/test-feedback -level=L2 -out <证据根>/test-results/L2/20261106T080815474Z`；`summary.json` | passed=true。 |
| 部署 owner 专项 | `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.deployment -out <证据根>/test-results/L4/20261106T081058095Z`；`summary.json` | passed=true、complete=true，467.304 秒；宿主、Windows 服务、应用和 Runner 升级四组通过、零跳过。 |
| 测试清单一致性 | `go run ./cmd/test-feedback -out <证据根>/diagnostics/catalogue-20261106T081158234Z -- ./cmd/test-feedback -run 'Test(TMRegisteredCatalogueMatchesExecutableManifest\|ScenarioContentListMatchesRegisteredIDsAndLevels)$' -count=1`；`summary.json` | 通过。 |
| 静态检查 | 四个变更/新增 PowerShell 文件 Parser；`python scripts/check_doc_links.py`；`git diff --check` | Parser 无错误；712 份 Markdown、4370 个本地链接零错误；无空白错误。 |

中间失败独立保留：`diagnostics/pipeline-20261106T080420540Z`、`pipeline-20261106T080518057Z`、`pipeline-20261106T080624852Z` 中的日志记录修复器或夹具的 `Set-Acl` 索取 SeSecurityPrivilege；两者均改为 .NET 仅写 DACL 后补验通过。`diagnostics/acl-ps5-20261106T080744165Z` 的首次夹具调用误用了 PowerShell 保留变量，修正命名后补验通过。`test-results/L1/20261106T080547161Z` 因本工作树缺少 Vitest 依赖而 incomplete；按规范执行 `npm --prefix web ci` 后，以上新 L1 目录完整通过。没有用后次成功覆盖原失败报告。

以上自动回归均使用隔离现场；后续实际安装维护独立记录如下。

## 用户授权同步与正式部署

维护证据根：`C:\SteerlineDebug\wt-380f-deployment-home-acl\installation\20261106T082105595Z`，仅当前用户访问并向其证据文件继承。原目录备份、同步前后摘要见 `source-before/`、`synced-files.json`；用户指定的 `C:\Project\SteerLine` 与工作树同为 `01356834` 且原先干净，只同步本次 10 份源码、测试和文档。Runner 源码 `C:\Project\SteerRunner` 干净，提交 `458185030c2b43cb93045c546956a79a898cc9ab`，未修改。

先保存根 ACL 与原部署记录，再通过修复函数恢复已确认的当前用户继承规则。恢复后保存执行配置、项目/Web/价表摘要、三个项目的一致性 SQLite 备份及原 status。原宿主/Web ready，但三个项目 failed/offline，degraded=true；三个项目检查点仍为原宿主的 running/ready，后续原停机回执却声明 local_closed。这些证据未经手工修正。

最初拟沿用报错时的 UpdateRunner/DiscardRuns，自动审批因尚无明确的旧运行重置授权拒绝，调用未执行。先运行保留旧运行的普通部署：构建成功，`deploy.stop-and-verify` 因原资源未结算和停止检查点不符而失败，安装文件保持。用户随后回复“给你授权”，授权记录保存于 `authorized-discard-20261106T082400747Z/authorization.json`。该次正式 DiscardRuns 调用仍在原项目 stopped 检查点核验处失败，尚未创建归档或切换业务目录；两次原日志与计时分别保留，不覆盖失败结果。

权限已经恢复后，通过原安装版本的正式启动入口重新启动，全部项目 ready，再由下一次正式部署正常停服生成真实 stopped 检查点。使用现有 Windows 启动代理保留原用户/环境并脱离部署宿主 Job，过程证据为 `original-recovery/`。未删除或改写旧回执、检查点、数据库、pin，也未绕过原资源和项目身份校验。

随后从 `C:\Project\SteerLine` 执行：

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File C:\Project\SteerLine\scripts\build-deploy.ps1 -InstallHome C:\SteerlineInstall -RunnerRepo C:\Project\SteerRunner -UpdateRunner -DiscardRuns -NoPause -TimingPath C:\SteerlineDebug\wt-380f-deployment-home-acl\installation\20261106T082105595Z\redeploy-20261106T082650911Z\deploy-timings.json
```

部署 exit 0、succeeded=true，总耗时 117.30 秒，停止/结算/归档阶段 87.29 秒。新包为 `C:\SteerlineInstall\deployments\20261106-082709-ac81eac1`；原状态归档为 `C:\SteerlineInstall\maintenance\discard-runs-6158cf749dd3493998a8471e8c34069e`。部署记录如实保存 SteerLine `01356834` 加本次 dirty 修改和 Runner `45818503`，三个 EXE 实际摘要与记录匹配。

`redeploy-20261106T082650911Z/verified.json`、`verify.py` 保存最终只读核对：三个原项目共 20 个任务的编号、标题、类型、优先级、目标及契约身份、完整契约、8 条依赖与维护前备份和归档一致；新任务全部 open、无领取，项目全部 manual/paused，旧候选不进入新库，三库 integrity_check=ok 且 foreign_key_check 为空。项目配置、完整执行配置中 Runner 以外的字段、价表和 Web 配置保持；安装根 ACL 与恢复后的值相同。

北京时间 2026-11-06 16:30 最终核验：后端、Web 和 3 个项目全部 ready/online、degraded=false，页面 HTTP 200；对应 `status-final.json`、`http-final.json`。原运行已按授权归档，不恢复旧执行；派发保持暂停。维护后文档结果另同步回日常源码，记录在证据根 `documentation-sync.json`。
