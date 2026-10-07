# Plan：固定版本与双入口

任务 ID：40；任务名称：固定版本与双入口。配套 [Spec](versioned-runtime.spec.md)。状态：延后，不作为当前执行入口。下列步骤保留为待重定草案；重新启用时核对 #83 已交付能力与当前格式，重新固定前置和实现 PR 范围。

## 待重定的实施草案

1. 审计 InstallSelf、InstallLaunchers、SteerlineHome/SteerlineBinDir、项目选择与 re-exec 的路径解析，列出服务窗口、控制台和任务 CLI 及其子进程的全部入口，确定 manifest 和唯一 channel 选择记录。把每次调用的精确 exe/build/home/channel/摘要收敛为一次解析的固定身份，沿现有启动和发现路径传递。
2. 实现不可变版本发布、摘要校验、首次初始化与受保护的选择提交；交付 dev select 的离线锁/外部结算/同格式检查及请求幂等，stable 切换留给 #41。
3. 接通 stable/dev 的三个入口与子进程身份传递，在现有发现、客户端/服务握手及服务端命令入口分别核验 build/home/channel、实例和项目/配置；不以 config_digest 代替构建身份，不在调用中途重读选择或从共享 bin/PATH 猜版本。直接 exe 调用执行相同校验。
4. 用两份含 dirty dev 的 Windows 构建执行 VR1–VR5，验证 dev 连续切换并真实启动，以及安装重入、选择写入中断、误连和路径场景；加入配置内容相同但 build/channel/home 不匹配、入口解析后选择改变的样例，确认原进程仍报告原身份。回归已有服务窗口生命周期。
5. 向 #41 提供可调用的版本校验和选择接口及故障样例；保存 本次临时目录中的原始命令/进程证据，并在任务专属验收文档登记摘要，按共同约束验证。

## 测试计划（延后实施与自动验收）

任务专属记录：[验收文档](../acceptance/versioned-runtime.md#自动验收)。覆盖 VR1-A、VR2-A、VR3-A、VR4-A、VR5-A，保留原范围等待重定，不启用本任务。原静态检查只用于规划修订，不证明这些自动项通过。

重新启用时先固定实际范围、前置、构建/测试名称及环境，再在既有 driver/native/evidence/testenv 内补齐最小版本/维护驱动，登记 runtime 场景与精确验收项断言。以下入口为规划新增，尚未实现/登记；不得按表提前执行或占用生产目录。

| 验收项 | 级别 / 组 / 场景 | 环境 / 命令 | 执行时点 | 通过条件 | 证据与登记 |
| --- | --- | --- | --- | --- | --- |
| [VR1-A、VR2-A、VR3-A、VR4-A、VR5-A](../acceptance/versioned-runtime.md) | L3 / runtime / runtime.versioned-entries（待新增） | Windows、两独立 clone/home、固定产物、当前格式、受控后端；`go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled '-scenario=runtime.versioned-entries' '-out=<本次空临时目录>'` | 重新规划准入、驱动/场景实现/登记及能力准备后 | 每项原身份、选择/恢复与拒绝断言通过；报告完整，清理成功；不以安装/调试隔离替代版本/提升证据 | 临时 summary.json、scenarios/runtime.versioned-entries/；本任务专属验收文档逐项结果 |
| 必要回归（非全部产品验收） | 实际受影响 owner + L1 | 重定时选择相关入口的精确定向命令；Go 接线变化时 `go run ./cmd/test-feedback -level=L1` | 改动完成后一次；新影响才补跑 | 实际命中、无受影响消费者回退 | 各自临时目录与本任务记录 |
| 文档静态（非产品验收） | doc-links / task-identities / whitespace | Python/Git；`python scripts/check_doc_links.py`；`python scripts/roadmap_task_id.py check`；`git diff --check` | 当前草案交付前 | 导航/身份/格式一致，延后状态明确 | 本次静态输出 |

需要真实原会话/后端能力时在重定阶段明确是否复用原证据或增加最小 live 子场景，并固定 chain/权限/命令与清理；controlled 不证明原生权限。默认不选 Full、完整 L2/全部 live 或无关平台组合。当前只是改 Plan，不执行上列产品入口。

执行自动验收及在任务专属文档登记实际源码/摘要、命令、逐项结果/证据，并同步 execution-map 属于重新启用后的交付步骤。Plan 不存实时通过状态；人工项如需新增按允许范围集中后置，关键先验和真实失败仍阻塞。
