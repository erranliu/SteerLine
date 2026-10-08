# 任务 #83 验收规划：本机安装与调试路径隔离

唯一任务归属：#83。来源：[Spec](../spec/local-runtime-isolation.spec.md) / [Plan](../spec/local-runtime-isolation.plan.md)。本文件是该任务专属验收文档，不覆盖其他任务；验收流程见[入口](README.md)。

状态：LI1-A–LI4-A 已通过自动验收；无本任务人工验收项。执行依据仍为固定 Spec/Plan commit `8dfa4c5b4df106e878a9abc7b4f518bf2559d67c`。以下记录本次实际命令、构建身份、结果与临时证据。

## 自动验收

以下是源场景的全部自动子项，均由 Plan 映射至 `runtime.installed-debug-isolation` 受控场景；实际结果和证据位置逐项登记于下表及执行记录。

| 验收项 | 方法 | 规划行为与通过条件 | 状态 / 结果 |
| --- | --- | --- | --- |
| LI1-A | 自动 | 两个独立测试 clone、不同 home 的安装版与调试版同时运行；配置、状态、日志、服务发现和写命令不串线 | 已验收；两项目分别交付，project/database ID、任务、服务身份及目录均隔离；见最终场景记录 |
| LI2-A | 自动 | 调试 build、停启及入口重复生成不改安装 exe 摘要、原服务实例或任务事实；调试服务/控制台/CLI 及子进程使用实际调试 exe/home | 已验收；安装 exe SHA-256 全程 `be1d08dfc38e6c5b0bb1858653cb2b21da72fa745528135e2f56566b7e711047`，安装服务实例 `68c664c8097e45a1af85f3895be49ea4` 不变；调试重启使用 home 内 exe 并发布新实例，任务事实保持 |
| LI3-A | 自动 | 缺少调试路径、home 重叠、显式状态/工作区越出调试 home 拒绝；同仓库跨 home 第二写者被原锁拒绝；误连实例的写命令仍被拒绝 | 已验收；缺 exe、普通及 junction home 重叠、`run_dir`/`worktree_root` 越界、运行中 build、过期实例/配置写入和同 clone 跨 home 第二写者均按条件拒绝；拒绝请求未发布 |
| LI4-A | 自动 | Windows 中文/空格路径及无关 cwd 可启动并显示准确身份；维护说明区分中心停服、外部执行结算、同构建重启和人工更新 | 已验收；准确显示 exe/home/config、源码 commit `8dfa4c5b4df106e878a9abc7b4f518bf2559d67c` 与 `dirty=true`；维护步骤见[本机安装与源码调试](../runtime-install-debug.md) |

环境：Windows/amd64 controlled；两个独立 Git clone/home、真实 SteerLine 进程、SQLite、Git 和受控 worker/provider；不发模型请求。每次输出目录独立，服务和临时资源由场景登记并清理。

## 本次执行记录

环境：Windows/amd64 controlled；两个独立 target Git clone/home；另创建同一 clone 的第二 home 验证仓库锁；受控 worker/provider，无模型请求。场景 `runtime.installed-debug-isolation` 通过，`passed=true`、`complete=true`、`cleanup_complete=true`、零 skip，耗时 224.28 秒。源码构建身份为 commit `8dfa4c5b4df106e878a9abc7b4f518bf2559d67c`、`dirty=true`。

自动验收/结果登记命令：

```powershell
go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled '-scenario=runtime.installed-debug-isolation' '-out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-runtime-20261008-11'
go run ./cmd/test-feedback -level=L3 -suite=delivery -env=controlled '-scenario=delivery.basic' '-out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-delivery-20261008-01'
go run ./cmd/test-feedback -level=L1 '-out=C:\Users\lmas1\AppData\Local\Temp\steerline-task83-l1-20261008-06'
go test ./internal/app -run 'TestConsoleProjectRefExposesCurrentRuntimeIdentity|TestNativeConfigRejects(ASecondRunDir|WorktreeOutsideHome)$' -count=1
go test ./internal/console -run '^TestConsoleDiagnosticsShowRuntimeIdentity$' -count=1
go test ./internal/app/host -run '^TestWindowsStartServiceRequiresVisibleWindow$' -count=1
go test ./cmd/steerline -count=1
go test -tags=e2e ./tests/e2e/suites/runtime -run '^$' -count=1
git diff --check
```

Linux 代表性源码编译：PowerShell 临时设置 `GOOS=linux`、`GOARCH=amd64`，运行 `go build -o C:\Users\lmas1\AppData\Local\Temp\steerline-task83-linux-build-20261008-01\steerline ./cmd/steerline`，随后恢复原环境变量；命令成功，产物 22,991,569 字节。本结果只表示 Linux 编译通过，不是 Linux 安装验收。

最终隔离场景的 L3 原始报告位于 `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-runtime-20261008-11`：`summary.json`、`e2e-1.jsonl`、`scenarios/runtime.installed-debug-isolation/scenario.json`、`runtime-identities.json`、`resources.json`，以及安装/调试两侧的 waiting-merge 与 completed 快照、命令日志、路径拒绝日志。共享 testenv 路径回归 `delivery.basic` 也通过，`passed=true`、清理完成、零 skip；报告位于 `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-delivery-20261008-01`。最终 L1 summary 位于 `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-l1-20261008-06\summary.json`，`passed=true`、`complete=true`，3 workers/12 groups。Linux 编译产物保留在 `C:\Users\lmas1\AppData\Local\Temp\steerline-task83-linux-build-20261008-01\steerline`。

LI1-A 由两侧同时运行及分别等待合并/完成的快照验证。LI2-A 记录安装 exe 构建前后 SHA-256 与安装服务实例 ID；debug 源码 exe 在重启前后 SHA-256 均为 `dd43089d0a7d84384c872be3a3cd77f53a64c208f2f6d790091cac0fd2e37a47`，debug 服务实例从 `ea5b2f26c4440529e5243c134fbd5245` 变为 `9214812cb3e3a9dae160ab603af2724c`，任务与 Delivery 保持。LI3-A 证据包含 junction alias 重叠拒绝、运行中的 debug exe 阻止 build、home 外状态/工作区拒绝、过期 instance/config 写入拒绝且 request 数量不变、同 clone 跨 home 锁拒绝。LI4-A 使用包含中文和空格的目录及无关 cwd，控制台/CLI 与 service.log 均显示本次实际路径和构建身份；生成入口把 `CODEX_HOME` 指向各自 home 下的 `codex` 目录。

开发期的初始试跑曾发现构建 provenance 缺失、Windows PowerShell 5.1 的 UTF-8/哈希/junction 清理差异，以及随 home containment 新错误顺序变化的旧断言；均已修正并由最终场景/L1 复验。失败试跑证据保留在 `%TEMP%\steerline-task83-runtime-20261008-01` 至 `-04`、`-10`；最终通过记录以 `-11` 为准。以上均为实现/验证修正，不存在待人工处理的自动失败。

临时证据目录在用户主机保留供协调聊天复审；仓库只登记上述最小身份、摘要与结果。

## 人工验收

本任务无人工验收项。一期 24 小时与组合签收安排见独立[一期活动](phase1.md)。

## 记录与交接

执行时按 LI1-A–LI4-A 逐项记录实际命令、环境/构建、日期、结果与临时证据位置；execution-map 同步实现状态、验收摘要并链接本文件。独立阶段活动引用本任务结果，不覆盖本文件明细。
