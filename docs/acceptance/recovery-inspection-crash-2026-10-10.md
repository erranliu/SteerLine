# 只读恢复核验崩溃修复验收

日期：2026-10-10（Asia/Shanghai）。范围：控制台按 `u` 发起的只读恢复核验。普通开发修复，无 roadmap Task ID。

## 原因与修复

`recoveryService` 不持有受管服务的 `ResourcePort`。`lifecycleForRun` 原先直接取得 `s.resources.WorkerTreeAbsent` 方法，导致空接口 panic。现在仅在资源接口存在时传递该能力；缺少能力时不宣称已核实写进程退出。正常服务继续传递原有检查能力。

不涉及接纳人工追加轮次、推进 reconciling 或任务恢复策略。

## 自动验收

| ID | 场景与通过条件 | 结果 |
| --- | --- | --- |
| RI-01 | `TestRecoveryInspectionReadsBoundRunWithoutServiceResources`：完整 `InspectRecovery` 入口核验 reconciling 的绑定执行，无受管资源接口仍返回进程、工作区与领取报告；原 execution/attempt/process 身份与运行快照不变 | 已通过；修复前同一用例复现 `worker_lifecycle.go` 的空指针 panic |
| RI-02 | `TestCD64ReviewReceiptAndRecoveryIsolation/recovery_preview_uses_original_lifecycle_port` 及 `TestRecovery*`：原生命周期绑定、原会话恢复限制和领取核验回归通过 | 已通过 |
| RI-03 | `TestCPWRecoverySummaryIsBoundAndExpires`：控制台报告显示、身份绑定、过期处理和迟到响应隔离通过 | 已通过 |

环境：既有 Go 隔离夹具，临时 Git 仓库、SQLite 与 home；无真实模型或外部 PR 操作。未使用或修改 `C:\SteerlineRuntime` 安装现场。

命令：

```powershell
go run ./cmd/test-feedback -- ./internal/execution -run '^(TestRecovery.*|TestCD64ReviewReceiptAndRecoveryIsolation)$/^recovery_preview_uses_original_lifecycle_port$' -count=1
go run ./cmd/test-feedback -- ./internal/console -run '^TestCPWRecoverySummaryIsBoundAndExpires$' -count=1
git diff --check
```

证据位置：

- 修复前 panic：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261010T112729.531770900Z/summary.json` 及同目录 `go-test.jsonl`。
- RI-01/RI-02：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261010T112830.621345300Z/summary.json`。
- RI-03：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261010T112854.289614500Z/summary.json`。

未选择 L2/L3 或 Full：本次仅修复只读对象构造后的空接口引用，定向用例已覆盖崩溃入口和显示路径。

## 安装环境验证

用户随后明确授权部署。2026-10-10 19:32（Asia/Shanghai）通过正式 `install` 入口直接替换 `C:\SteerlineRuntime\bin\steerline.exe`；替换前安装版服务与控制台均已关闭。

- 安装程序与源码构建 SHA-256 相同：`EFC2E1EE4E54C33DCAE74132A1FAAA35FB576A9845A189563143E1977C62B6F1`。
- 构建来源：`6d4b4fa2d67b77156eeda64d103ce71c95f3450b`，`source_dirty=true`，包含本次未提交修复。
- 安装版 `home -config C:\SteerlineRuntime\etc\SteerlineTest.json` 成功，核对实际 exe、home、配置及项目状态路径。
- 15 个配置、数据库及启动器文件哈希保持不变。未启动服务或推进任务。
- 部署证据与旧二进制保留于 `C:/SteerlineRuntime/backup/recovery-inspection-fix-20261010-193222/`，详情见其中 `deployment.json`。

本次未启动安装版控制台作交互验收；崩溃修复验证使用上述隔离回归。
