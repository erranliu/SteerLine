# #36 后的活动规划与中心旧任务清理

日期：2026-09-29。用户明确允许直接精简 roadmap、删除失效 Spec/Plan，并同步受控清理 `C:\SteerlineRuntime` 中的旧中心任务；随后允许通过原可见服务窗口恢复服务。

## 清理结果

- 中心项目为 `c:\project\steerline\.git`，配置为 `C:/SteerlineRuntime/etc/SteerLine.json`。与 `merge-h-308-code` 项目同号任务无关。
- 旧任务 #14–#24、#26–#35 共 21 项，均通过正式 `task discard` 命令取得 `discarded` 回执，SQLite 只读回查一致。任务身份、固定契约、事件和历史执行保留。
- #26/#27 合为新 #37；#33 的旧步骤规则提取取消，#34 必需的执行上下文整理纳入 #36，#35 全面 SQL 搬包退回产品方向。
- 其余 16 项保留有效范围并续建新身份 #38–#53。旧批次已绑定包含旧执行约束的固定契约，不能在原 ID 上换绑新 Spec；旧 ID 永不复用。
- 活动规划为 #36–#53 共 18 项。新规格及依赖需先集成到目标分支，再按固定 commit 登记中心；本次未创建这批替代中心任务，也未运行它们。
- 删除旧 #26/#27/#33/#34/#35 的 5 组 Spec/Plan、失效的模块拆分共同约束和临时审计文档；有效任务的 Spec/Plan、两份 roadmap、阶段说明及共同约束同步更新。

逐项新旧编号见 [replacement-map.json](replacement-map.json)。待登记清单及依赖见 [planned-tasks.json](planned-tasks.json)。历史规格可以从 Git 历史读取，原身份文件仍保留。

## 命令与回查

执行前的版本、历史运行与固定 request_id 见 [discard-requests.json](discard-requests.json)。每项使用：

```text
C:\SteerlineRuntime\bin\steerline.exe task discard -config C:/SteerlineRuntime/etc/SteerLine.json -task-id <ID> -request-id <原请求ID> -expected-version <采样版本> -reason <已保存理由> -by lmas1
```

结果保存于 [receipts/](receipts/)。首次 #14 因运行中的服务持仓库锁而失败，错误保留于 [14.stderr.txt](receipts/14.stderr.txt)；停服后沿原请求成功。未直接写数据库业务表。

[readback.json](readback.json)记录 21 项 `discarded`、这批任务的 Delivery 数仍为 0，以及 8 项历史工作区 `cleanup_pending`（#14：3，#15：2，#26：1，#30：2）。丢弃回执不等于外部工作区已删除；这些现场和待清理事实仍保留，未伪造清理完成。

## 服务恢复阻塞

为取得正式 CLI 所需仓库锁，先通过 `control -action stop_service -wait` 停止 SteerLine 项目服务，请求 ID 为 `c1a1db7bc7b200b3c3e8bb8141e0dc54`，原实例为 `694c597e54bd5259aa2306ea9f5062be`。该控制命令返回 applied。其余项目服务未停止。

内置 `start-service` 返回旧启动记录并报 `Access is denied`。用户授权后使用原 `service-SteerLine.bat` 打开可见窗口，但当前二进制在启动持久化阶段退出，错误为 `critical domain persistence: invalid step attempt status "abandoned"`。原始输出见 [service-restart.log](service-restart.log)。**本次未恢复 SteerLine 项目服务。**

只读核对定位到旧 #14/#15/#30 各一条 StepAttempt/Execution 的 `abandoned` 状态，更新时间为 2026-09-29 03:21 UTC 左右，早于本次 03:57 UTC 的清理前快照；原始执行状态也已留在 discard-requests 中。WorkflowRun 允许 abandoned，但当前 StepAttempt/Execution 枚举不允许该值。[旧重置事件](historical-reset-events.json)进一步确认这些记录来自早先的 `confirm_unfinished_reset` 操作，记录的 `confirmed_process_stopped` 为 false；d2de 工作树的开发版本定义了 abandoned，但随后部署的主线版本未包含该定义。服务启动重新持久化这些历史记录时失败；runtime.json 因而仍旧，不能用它否定数据库中的丢弃结果。

按项目规则，遇到不支持的状态阻止有副作用启动。未扩大枚举兼容、改写历史执行状态、清空数据库或移除外部绑定。恢复服务需要另行核实这 3 条历史执行的外部停止证据及状态写入来源，再决定明确的人工修复；不能仅因丢弃成功推断外部 worker 已停止。

## 文档验证边界

[文档检查结果](document-checks.json)覆盖任务身份与名称、Spec/Plan 入口、依赖无环、活动编号及本地文档链接；另执行 `python scripts/roadmap_task_id.py check` 与 `git diff --check`，均通过。本次是规划清理，不代表 #36 或后继任务的实现/真机验收通过。
