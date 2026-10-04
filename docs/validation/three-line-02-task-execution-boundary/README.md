# #74 任务选择与执行结果边界

仅实现任务 #74。起点 `39059035152a76a8a0d8f0036eb3fa2f6155ac90` 已含 #73/PR210；原类型与 TaskGraph 每快照转换一次的修复保持。实际窄端口、生产调用链和唯一迁出定义见 [ownership.md](ownership.md)；Service/CommandService/Config 逐字段/方法及 #75/#76 可并行的文件清单见 [integration-ownership.md](integration-ownership.md)。

## 实现与验收

| Spec | 实际交付 |
| --- | --- |
| TM02.1 | app 显式注入 task.Reader/WorkPort/ResultPort；调度 Ready 与 first/later candidate 的 Fixed/Claim 真实消费任务端口。任务不读 mode/slot/session/PR，后续候选保持原领取/契约/base。 |
| TM02.2 | ClaimCommit/ResultCommit/DiscardCommit 是固定声明，无任意事务句柄；SQLite 重检原身份/前值/任务/图/集合版本。结果、唯一执行 Delivery、Task closed、DAG/event 一次事务；重复原结果保持原回执和 DB/图版本，异内容/旧版本/未知/失败不释放后继。 |
| TM02.3 | task 拥有人意图与任务声明，execution 拥有 PR/候选核验及停止/清理。Service/Config/CommandService 与 app/cmd/runner 已实际拆文件，后继只改自己的文件，#76 负责公共段单点集成。 |

结果元数据新增唯一 task_results，SQLite schema 14 仅接当前格式，不迁移、双写或回退；PR/session/资源证据继续由执行模型持有。产品/后端 API 不公开 ResultPort，Task 应用仅持任务读端口和纯任务声明。

## 实际最小验证

Windows / Go 1.26.4，临时 Git/SQLite/受控后端/provider。命令由独立 PowerShell 子进程设置本轮 STEERLINE_HOME，不更改父应用环境；无原生 opt-in、无模型请求。原 Go JSON、原请求/身份/版本/回执与清理结果均保留。精确包、旧/新 parent/subtest、命令和每次 run/pass 事件见 [test-mapping.json](test-mapping.json)。

| 入口 | 结果 | parent/subtest run / pass / fail / skip | 证据 |
| --- | --- | --- | --- |
| `claim/run-03` | 通过 | 13 / 13 / 0 / 0 | [原始报告](claim/run-03/summary.json) |
| `result/run-03` | 通过 | 23 / 23 / 0 / 0 | [原始报告](result/run-03/summary.json) |
| `extra/run-02` | 通过 | 14 / 14 / 0 / 0 | [原始报告](extra/run-02/summary.json) |
| `l1-repair/run-01` | 通过 | 39 / 39 / 0 / 0 | [原始报告](l1-repair/run-01/summary.json) |
| `l1/run-03` | 通过 | 236 / 236 / 0 / 0 | [原始报告](l1/run-03/summary.json) |
| `recovery/run-01`（L3 controlled；唯一既有 waiting-merge-restart） | 通过 | 场景非零命中；原执行不重派，恢复后交付/清理通过 | [报告](recovery/run-01/summary.json) |
| Windows/Linux `go build ./...` | 通过 | 仅生产构建，不冒充 Linux 行为 | [Windows](build/windows.log)、[Linux](build/linux.log) |

因为改到 app 重启注入、新提交持久关联及 schema，按 Plan 只追加既有 controlled `recovery.waiting-merge-restart`。没有运行 Full、L2、整组 L3/live 或远端流程；不以本机结果声明远端认证。PR212 的一期本机范围文档将顺序集成，保持用户暂停范围。

## 自查修复与历史报告

- `result/run-01` 如实保留失败：discard 对规范化 JSON 和结构体字段序摘要混用；改为比较同一规范内容。竞争断言加强为顺序提交的第一方必须成功，另一方只能因版本/结项仲裁拒绝。
- `claim/run-02` 如实保留失败：首次领取原有重放允许版本前值改变后返回原预留。恢复既有行为和原断言；后续 candidate/规范 result 保留其严格原请求/版本。`claim/run-03` 通过。
- 自查修复重复结果只读重放增加 DB revision 的问题：private replay marker 回滚读事务；原回执和 DB/图版本不变，CAS 测试实际验证。
- `l1/run-01` 如实保留 task 包无测试的 package skip 和旧 Snapshot owner 守卫失败。原 task 断言按所有者迁到 task/work_test.go；CM69 只替换旧 owner，并用全 OS/tag 纯投影矩阵收紧 taskview（禁 usecase/query/SQL）。定向修复通过，无测试豁免。
- `l1/run-02` 所有实际 Go 用例 0 fail/skip，runner 报告一个不存在的中间 subtest 名；Go 的 `t.Run("steerline/taskview")` 仅生成完整名字。移除错误的中间名字赋值，保留实际完整 subtest 和原 guard；最终 L1 固定入口在 `l1/run-03`。
- 删除偶发同名文件拆分风险：原 execution_snapshot.go 的政策资源实现完整保留，task 快照方法实际拆到 service_task_snapshot.go；编译及生产接线验证保持。
- 自查完整实际 diff，恢复与本任务无关的格式改动；旧断言不以镜像实现、零命中、skip 或 mock 能力代替。生产/测试源码指纹见 [source-index.json](source-index.json)。

当前没有已确认但未解决的 #74 实现问题。当前 PR 未合并；完成后执行聊天停止写入，等待协调聊天独立复审、修复与真实合并。#75/#76 只能在其硬前置合并后消费本次接口；本聊天没有启动、接管或催促任何其它任务，#77/#79/#80/#81/#82 保持用户暂停。
