# 项目统一就绪与新执行准入

2026-11-06，按用户确认直接实施，不调度 roadmap。源码基线 `a54280510802c1208d9a1a3ff0ec473cad4ebd24`，报告采集时含本次未提交修改。行为正文见[项目维护](../../../docs/product-design/project-maintenance.md)。未部署安装版，未修改活动项目、配置或数据库。

## 已实现

- 项目维护主状态固定为“已就绪 / 未就绪”，同步和测试安排数量为说明，生成进度消失不移除主状态。断线、查询失败按未就绪处理，普通刷新不清除已有有效结果。
- 后端 `readinessState` 统一汇总：远端观察成功、本地配置目标分支与目标提交一致、工作区干净且无进行中 Git 操作、全部 open/in_progress 任务都有当前提交的完整材料及有效测试安排。closed/discarded 不参与。取消或失去成功原结果的生成不能借旧 ready 材料放行。
- 项目维护响应与 `/maintenance/readiness` 复用同一投影。任务启动、新增并行候选和自动派发界面消费后端资格，不在前端重算同步或任务总表规则。环境跳过设置不绕过项目门槛。
- 所有新候选在准备时和最终预留前核对项目资格；SQLite 候选事务重核全部未完成任务的材料/生成，失败零候选、零任务领取。已预留请求在新门槛之前识别重放，保留原版本；恢复、继续、反馈、发布及交付路径未增加项目就绪门槛。
- 不改提示词、黄金样本或生成接线，不增加持久格式、兼容读取、迁移或回退路径。

## 验证记录

报告根 `C:\SteerlineDebug\wt-a45c-project-readiness`，每次执行独立目录；下列路径相对报告根，以 `summary.json` 为入口。

| 范围 / 实际命令 | 结果与证据 |
| --- | --- |
| 定向 owner：`go run ./cmd/test-feedback -- ./internal/app ./internal/storage/sqlite -run 'TestProjectReadiness\|TestMaintenanceCandidateAdmission\|TestPMS\|TestNative101Generated' -count=1` | 通过；`diagnostics/owner-8191a68843b74865bc5e41ae311ab7f3/summary.json`。覆盖当前任务就绪而兄弟任务缺失/失败/生成中/过期时全项目阻断、closed/discarded 排除、同步差异/游离 HEAD/错分支/脏工作区阻断、预留前再次核对、原生成复用及只读零生成。 |
| L1：`go run ./cmd/test-feedback -level=L1` | 通过；`test-results/L1/00304259fad34258a35e9a004204be67/summary.json`。 |
| L2：`go run ./cmd/test-feedback -level=L2` | 通过；`test-results/L2/2193983867b24429919424f7a0fd1ce7/summary.json`，受控真实 Git/SQLite 核心交付及清理。 |
| Web 全集：`go run ./cmd/test-feedback -level=L4 -suite=web -scenario web.unit` | 最终通过；`test-results/L4/web-ebae8b11b93f477895ccf4f0832a3cf2/summary.json`。含手动/并行启动和已打开确认框再次阻断、服务结果变化、查询失败、项目切换、自动派发开关与刷新稳定性。初次 `web-d1f6f5bacf1c4716928c17eb5b186c49` 失败，原因是旧页面夹具缺少新增就绪契约，补齐夹具后完整复验通过，未放宽断言。 |
| SQLite/路径全集：`go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.storage` | 通过；`test-results/L4/storage-1dc4a721e5ce4f2181c2ef605a8d7b79/summary.json`。含兄弟任务失效及生成已取消但材料尚为 ready 时的事务阻断、回滚，项目未就绪后的原候选重放。 |
| HTTP 全集：`go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.http` | 通过；`test-results/L4/http-1d4193c5f8984a18becbf02afeea9efe/summary.json`，与 L2 分开执行，包含原跨进程快照压力。 |
| 应用 API 路由：`go run ./cmd/test-feedback -- ./internal/product/applicationhttp -run '^TestPMReadRoutesKeepMaintenanceWritesOnCommands$' -count=1` | 通过；`diagnostics/routes-32f6a16be58c4c2b9396702d76fb954e/summary.json`。 |

初次沙箱 Go 验证被临时 Git 仓库配置写权限阻断；失败证据保留在本次聊天的可写外部目录 `C:\Users\lmas1\.codex\visualizations\2026\11\06\01a1aef5-0ebc-7c22-bfe7-6043bf09cf2b\diagnostics\initial-507d63170bff464d88179600c17ad8ee`。改用正常权限、独立夹具与输出后 owner 通过，不将沙箱失败计为产品通过。

最终 `npm run build` 通过，日志：`C:\SteerlineDebug\wt-a45c-project-readiness\diagnostics\build-adaacf5c2cc34236991bd5499e469dd9.log`。文档导航检查通过（697 个 Markdown、4284 个本地导航链接、零错误），`git diff --check` 通过。

未执行真实模型、真实 PR 链或安装部署；本次 DOM/受控测试不代替完整浏览器现场验收，上一轮浏览器长场景的限制仍见[原准备验收](project-maintenance-preparation.md)。

## PR 前主干同步补验

2026-11-06，提交前同步 `origin/main` 的 `6d124832`，保留主干执行配置和启动回执修复。唯一冲突是验收索引，保留双方记录；代码自动合并。以下在源码干净的 `e0ade35c` 上执行，报告根沿上文；本节之后仅补充验证记录。

| 范围 | 结果 / 报告相对路径 |
| --- | --- |
| L1 | 通过；`test-results/L1/pr-aaadda8a800c47c398f52cf13824226e/summary.json`。 |
| L2 | 通过；`test-results/L2/pr-c904769f5a0a490892242dfe64bff2f7/summary.json`。 |
| Web 全集 | 318 项全部通过；`test-results/L4/pr-web-a122a6c36e9947a0b66875bd619bd255/summary.json`；随后 `npm --prefix web run build` 通过。 |
| 项目就绪 / 候选事务定向回归 | 通过；`diagnostics/pr-owner-c3250a2d36124ef8a84dc5b8358a371a/summary.json`。实际选择 `./internal/app ./internal/storage/sqlite -run 'TestProjectReadiness\|TestPMSCandidateSnapshotCASVersionsAndReplay\|TestMaintenanceCandidateAdmission' -count=1`。 |

同步后文档导航检查通过（701 个 Markdown、4316 个本地导航链接，零错误），差异检查通过。此前 SQLite/HTTP 全集仍按上文原基线与报告追溯，不扩大本次补验范围。
