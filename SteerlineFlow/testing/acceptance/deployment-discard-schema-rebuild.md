# 放弃旧运行时直接重建状态

本次修复以 `3a1777a6bbf5d4b245be2ca12aa24c4b2af4e905` 为基线。原安装的 schema 51/52 报错及用户授权维护结果见[实际恢复记录](deployment-schema52-recovery-20261119.md)；实际安装未作为本次代码回归的测试现场。

## 行为

显式 `-UpdateRunner -DiscardRuns` 沿原只读旧库入口导出任务图，通过正式任务入口建立当前 schema 新库，归档原项目、应用/Web 及 Runner 状态。任务编号、正文、优先级、目标、固定契约和依赖保留，任务全部 open、自动派发关闭。旧数据库不修改版本号、不原地迁移，也不作为当前运行库继续读取。

补齐先前普通部署已替换程序、但新版因旧 schema 尚未打开任何项目便失败的情况。匹配的停机回执须确认全部项目没有实例且已关闭本地资源并释放锁；当前宿主、Web 的精确进程退出仍须核实。原项目检查点继续要求 stopped、非 ready、完整实例与冻结摘要、原仓库及同一安装 Runner 命名空间；独立核实检查点实际所属的旧 OS 进程已退出，不将最近失败宿主当作其所有者。

若项目检查点记录的是已经被替换的 Runner 构建，复用 `VerifyInstalledUpgradeDrained` 冷查询原 Run 和控制的排空状态后，才允许当前构建参与放弃结算。原生资源未知、未完成恢复授权或输入、构建/命名空间不符均阻断。部分项目已打开、活动检查点、旧宿主存活、损坏/陈旧回执或锁占用不进入此恢复路径。普通更新、重启及缺失回执恢复的原检查保持原语义。

`discard.json` 增加 `unstarted_host`，项目证明保存实际的 `checkpoint_process`。原失败停机回执原样归档，任务准备、原 Run 停止信封、资源核验、配置/包摘要重查和带中断标记的目录切换仍沿原入口。

## 隔离验证

证据根：`C:\SteerlineDebug\wt-e0bd-discard-schema`。新增用例归原 `TestDeploymentDiscardRuns`；可读列表与机器清单同步说明新增覆盖。

首轮正式专项中，完整 `TestDeploymentDiscardRuns` 通过，耗时 1247.3 秒；原 App 合并组随后在中断恢复检查中达到 25 分钟总时限，整套报告保持失败，不作为整体通过复用。将放弃重建根单独放入 `go-deployment-discard`，其余 App 根仍在 `go-deployment-app`，合计仍为 Windows 11 / 其他平台 10 根，全部串行、保留全部子项及每组 count1/25m/p1/parallel1。部署整套有界时限从 30 调整为 45 分钟。

测试分组只允许同包的明确互斥根拆分，全包 owner 与局部根混合、重复根或缺失/额外执行仍拒绝。跨 Windows/Linux/macOS 核对 App 根并集与完整原选择一致，现有失败、缺报告、skip、零匹配及进度传播检查保留。

schema 51→52 仅修改维护记录的语义，没有 SQL 表定义变化，因此 schema 51 夹具沿其实际表结构保存旧版本记录。任务图用例核实全部任务/契约/DAG 保留、原库字节不变、普通当前格式读写拒绝旧库。离线全流程覆盖正常原宿主及新版未打开项目的失败回执、已更换 Runner 的冷核验、未完成恢复授权拒绝，以及各归属与生命周期边界。

## 验证结果

业务修复冻结于 `cf30b9de17d0120d17e10f8f6739d03c9d795f1d`，分组编排冻结于 `b9dd1b3681cbe0823bacccb4e05bc22c53ed4009`。下列路径均相对上述证据根，正式调用使用对应独立绝对 `-out` 目录。

| 验证 | 结果与证据 |
| --- | --- |
| `go run ./cmd/test-feedback -level=L1` | 通过；`test-results/L1/final-20261119/summary.json`。受测为 `3a1777a6` 加业务修复，代码与随后 `cf30b9de` 一致。后续测试编排差异由下述完整 owner 与最终 CI 补充覆盖。 |
| `go run ./cmd/test-feedback -level=L2` | 通过；`test-results/L2/final-20261119/summary.json`。冻结源码为干净的 `cf30b9de`，受控交付与清理完成。后续未改产品代码或本场景定义，复用此结果。 |
| `go run ./cmd/test-feedback -- ./cmd/test-feedback -count=1` | 全包通过，68.301 秒；`diagnostics/partition-20261119/summary.json`。受测分组代码随后冻结于 `b9dd1b36`，包含三平台根并集、互斥分组、失败/缺证据/skip/进度传播检查。 |
| `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.deployment` | 最终完整通过，1056.092 秒；`test-results/L4/deployment-partitioned-20261119/summary.json` 与 `managed-go.deployment.json`。源码为干净的 `b9dd1b36`；五组全部 complete/passed，共 164 个测试终态、0 跳过，包含完整原 App 与新增放弃场景。 |
| PowerShell 部署入口 | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-deployment-stop.ps1` 退出 0；原调用控制台确认单次 Go 准备、调用/包绑定、固定 Runner 复用及升级预检。 |
| GitHub CI 的完整 L1 | `b9dd1b36` 的 [Prompt governance 运行](https://github.com/erranliu/SteerLine/actions/runs/44458825559) 成功。工作流实际执行固定 L1，包含提示词快照与源码边界；本次没有修改工作流或提示词。 |

首轮定向 `diagnostics/unstarted-initial/summary.json` 在编译阶段失败，已修正应用/宿主两层 OS 进程身份的显式字段映射。`diagnostics/unstarted-fixed/summary.json` 的唯一断言失败发生在 `unstarted-live-owner` 的夹具启动等待阶段，尚未进入新恢复逻辑；该用例在两次后续正式专项中均通过。原定向失败及原 App 合并组超时的 `test-results/L4/deployment-final-20261119/` 全部保留，未覆盖或改写为通过。

源码最终只追加验证记录；文档导航与 `git diff --check` 在提交验证记录前检查。

本次未修改固定提示词、正文组装接线、提示词资产或其专属测试/CI；未再次部署实际安装，不创建新模型任务或真实交付 PR。
