# 2026-11-06 schema 40 放弃旧运行部署

用户要求修复 `deploy.stop-and-verify` 的 schema 40/41 阻断，并最终明确：保留任务列表、固定契约和依赖，放弃旧运行状态。实际安装维护与隔离自动测试分别留证，不把安装现场作为测试夹具。

## 原因与修复

异步活动中心把 SQLite schema 从 40 改为 41。显式 `discard-runs` 的配置加载预检、检查点、任务导出和最终项目核验都沿用普通读取入口，因此在归档前就拒绝原数据库。

仅显式放弃模式新增 schema 40 只读入口，核对完整原表/索引/约束定义和原项目/宿主/Runner 归属；保留路径、链接、共享文件、锁、停机回执和原生资源结算门禁。使用原正式任务批量校验与创建入口重建新的 schema 41 库，全部任务 open、manual/paused，原数据库与旧运行状态归档。普通启动、查询、写入仍拒绝 schema 40；未知版本、变形定义或外来归属仍阻断。不改旧库版本号，不原地迁移或继续旧执行。

旧格式夹具真实移除 schema 41 新增表/索引，并恢复 `maintenance_settings.generation_batch BLOB`，不只修改版本戳。回归覆盖保留编号/标题/类型/优先级/目标/完整契约/依赖、清空运行状态、原库字节不变、正常读写拒绝旧格式、缺停机回执、外来宿主及损坏定义拒绝。

## 隔离验证

证据 scope：`C:\SteerlineDebug\wt-c30e-deploy-schema40`，受测源码为 `3ee95909` 加本次工作区修改。

- 首次定向完整 `TestDeploymentDiscardRuns`：`diagnostics/20261106T072144541Z-decb66854116402bbd683c97fee669bb`，失败。任务图保留通过，但两个 schema 40 全流程仍被配置加载的前置普通校验阻断；补齐专用前置校验后再验证，未动安装目录。
- 修复后定向 `^TestDeploymentDiscardRuns$/^offline-flow$/^schema40`：`diagnostics/20261106T072757281Z-43d1bf7fe90d4e5185cbc994fa5e9b87`，通过，56.066 秒。
- L4 `go.storage`：`test-results/L4/20261106T072702689Z-1d7eeb4af4b349f3957e076438f9ce35`，通过。
- 最终代码 L1：`test-results/L1/20261106T072905001Z-6a8d5dd883f64ef5bd847f13e8713020`，通过。
- 最终代码 L2：`test-results/L2/20261106T072924044Z-992e79c1caa84413a183f322dc3f492a`，通过。
- L4 `go.deployment`：`test-results/L4/20261106T072819600Z-b5de3de6101a4ce09c63f0bd42c901d2`，通过，包含完整 `TestDeploymentDiscardRuns`、宿主/Windows 服务、应用停机/中断恢复与升级组。
- `python scripts/check_doc_links.py`：710 个 Markdown、4367 条本地导航链接，零错误；`git diff --check` 通过。

首次沙箱 `npm ci` 未正常退出，使用正常用户环境执行后成功。一次测试包装调用因 PowerShell 参数数组拼接被入口拒绝，未执行测试；修正参数后使用新目录执行，不记为测试通过。所有原失败日志保留。

## 安装维护基线

安装 home 为 `C:\SteerlineInstall`。维护前宿主和 Web 均已退出，原 Runner 停机回执声明资源结算；三个库均为 schema 40。任务基线：merge-h-308-code 15 个任务/4 条依赖、SteerLine 0/0、SteerlineTest 5/4，共 20 个任务、8 条依赖，候选均为零。

配置摘要及 Runner 以外执行配置保存在 `installation/20261106T072600778Z-0b4b785c15394b3f846d48aba66ddc5f/before.json`。

## 实际部署与最终核对

使用本工作区修复后的正式入口执行：

```powershell
.\scripts\build-deploy.ps1 -InstallHome C:\SteerlineInstall -RunnerRepo C:\Project\SteerRunner -UpdateRunner -DiscardRuns -NoPause -TimingPath C:\SteerlineDebug\wt-c30e-deploy-schema40\installation\20261106T072600778Z-0b4b785c15394b3f846d48aba66ddc5f\deploy-timings.json
```

部署成功，125.52 秒；原阻断阶段 `deploy.stop-and-verify` 成功，81.02 秒。部署包为 `C:\SteerlineInstall\deployments\20261106-073742-6e40e0a2`，原状态完整归档至 `C:\SteerlineInstall\maintenance\discard-runs-c1b78106fe5a4ebdbcbabdddc4c1a6d0`。源码记录为 `3ee95909` 加本次未提交修改（`steerline_source_dirty=true`），没有把修改复制到另一源码 checkout。

维护证据子目录中的 `verify.py` 通过只读事务逐项比对归档库与新库：三个原库 schema 40、新库 schema 41；20 个任务的编号/标题/类型/优先级/目标/契约身份及全部完整契约、8 条依赖完全一致。全部新任务 open、无领取及放弃状态，三项目均 manual/paused；候选、环境资格、测试生成、Roadmap、交付及结果表均空。三库 integrity_check=ok、foreign_key_check 无问题，数据库身份重新生成。项目/Web 配置摘要、Runner 以外执行配置及安装 EXE 与部署包摘要核对通过，结果见 `verified.json`。

2026-11-06 15:40（Asia/Shanghai）最终状态：后端、Web 与三个项目均 ready/online，degraded=false，`http://127.0.0.1:18742` 返回 HTTP 200，见 `status-final.json`。完整命令输出见 `build-deploy.log`，阶段结果见 `deploy-timings.json`，原请求/资源结算与目录切换见归档 `discard.json` 和包内 `shutdown-verification.json`。

未运行新模型任务或真实 PR 交付；新任务仍暂停派发，旧运行不继续。固定提示词和材料未修改。
