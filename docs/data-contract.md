# runtime v2 / result v1 数据契约

编码为 UTF-8，时间为 RFC3339 UTC。完整例子：[runtime](../examples/runtime.json)、[result](../examples/result.json)。结果的可执行 JSON Schema 与程序一起嵌入：[result.schema.json](../internal/steerline/result.schema.json)。

## 顶层

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| schema_version | integer | 当前 2；可读取 v1 历史快照，下一次保存为 v2，拒绝其他未知版本 |
| updated_at | timestamp | 最近一次成功发布快照的时间，含心跳写入 |
| heartbeat_at | timestamp | 最近成功发布的活跃时间；读者用于判断过期 |
| service.instance_id | string | 每次启动新的随机实例 ID，不等于 claimant |
| service.pid | integer | 服务进程 PID |
| service.status | string | running / stopping / stopped / error |
| service.mode | string | scheduler（正常轮询派发）/ retry-one（仅重试指定已有运行）/ maintenance（人工恢复命令） |
| service.claimant | string | 稳定 Beads 领取者，用于恢复不完整记录 |
| service.error | string/null | 最近服务级错误，查询恢复后可能清除 |
| repository.path | string | 解析后的仓库绝对路径 |
| repository.remote / branch | string | 明确的目标远端和主线 |
| repository.identity | string | 规范化 Git common-dir，用于跨 worktree 锁与恢复校验 |
| current_task | string/null | 当前准备或执行的候选任务；fetch 阶段可能尚未领取，没有则 null |
| slot_occupied | boolean | 主线获取/worktree 准备/worker 占用，或因 unknown 旧执行而保守保留 |
| runs | array | 本服务管理的全部运行概要，初始为空数组 |

## 每项运行

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| task_id / title | string | Beads 任务身份和领取时标题 |
| run_id / claimant | string | 本次随机运行 ID / 固定领取者 |
| completion | object/null | 派发时固定的 `{command:{executable,args},timeout}`；null 使用原生 close。重启按该记录继续，配置变更不能跳过旧运行的结项尾部 |
| beads_context | object/null | 派发时的 beads_dir、project_id、database、backend；回调前再次核对。旧记录可能为 null，不包含认证信息 |
| beads_status | string | 最近核实的 Beads 状态，不以本地预测替代 closed |
| execution_status | string | pending / running / succeeded / failed / unknown |
| finish_task_status | string | pending / waiting_pr_merge / completed / blocked |
| codex_session_id | string/null | 从 thread.started.thread_id 得到，不从日志文字猜测 |
| process | object/null | PID、创建时间标识 created、executable 完整路径；均来自 OS |
| started_at / ended_at | timestamp/null | 服务记录的执行起止时间；崩溃导致终止时间未知时仍为 null |
| workspace_intent | object/null | 创建前持久化的 path、branch、base_commit；只表示计划创建，不证明目录已就绪。旧记录或领取孤儿可能为 null |
| worktree_path / worktree_branch / base_commit | string/null | 调度器创建并核验的隔离目录、实现分支及起点 Git hash；准备成功前均为 null，不由 Codex 结果覆盖 |
| pr.url | string/null | 当前运行的精确 PR 关联 |
| pr.state | string | open / merged / closed_unmerged / unknown |
| pr.merged_at / merge_commit | timestamp/string/null | 平台核实的合并证据 |
| pr.queried_at / query_error | timestamp/string/null | 最近查询时间和错误；失败时 state=unknown |
| last_event | string/null | 最近 JSON 事件的类型摘要，最多 512 字符，不存完整对话 |
| log_path / result_path | string/null | 每次运行独立的事件和结果文件；恢复孤儿记录时可能为 null |
| summary / error | string/null | 技能结果摘要（最多 4096 字符）/ 当前阻塞或失败原因 |

`process.created` 在 Windows 为进程创建 FILETIME 的十六进制标识，Linux 为 boot ID 和 /proc starttime。PID 可复用，读取器不能仅凭 PID 判定存活。遗留进程的核验结论写入 error；unknown 不代表服务已接管或恢复该会话。

execution_status=succeeded 仅表示进程正常退出，仍可能 finish_task_status=blocked（结果缺失/无效）或 waiting_pr_merge。只有重新核实对应 PR 确实合入目标主线、配置的结项命令成功返回匹配回执（若有），且 Beads show 返回 closed，才写 completed。任务已 closed 但回调失败时仍 waiting_pr_merge，重启后重跑幂等回调。finish_task_status 不是“finish 技能函数是否返回”。

准备 worktree 时 execution_status=pending；创建或接入检查失败为 failed/blocked；遗留 pending/running 为 unknown/blocked。重启只核对目录、共享 Git 元数据、分支和起点祖先关系，不重建、不重跑。实际工作目录等字段只在创建及技能/Beads 核验全部通过后写入；创建后检查失败时，目录可能已存在，应查看 workspace_intent 和 error。

兼容 v1 的等待合并记录：保留其 PR 查询，不把旧的技能自报工作目录升级为调度器核验事实，workspace_intent/worktree_branch 保持 null。旧 blocked/unknown 或孤儿运行缺少调度器准备记录，不能通过新版 accept-result 冒充已核验的运行，需人工处理原任务；程序不生成虚构的准备证据。

PR 查询失败会清除 state 的可信结论并写 unknown，但保留过去读到的合并字段用于诊断；读者必须同时看 state、queried_at 和 query_error。程序不会仅凭缓存的 merged_at 关闭任务。

## 执行结果

所有八个键都必须出现，不允许额外键：task_id、run_id、outcome、pr_url、worktree_path、base_commit、summary、error。

- task_id/run_id 必须与此次派发精确匹配。
- outcome 仅允许 pr_submitted 或 blocked。
- pr_submitted 必须提供 PR URL，并原样回传调度器提供的绝对工作目录、40 或 64 位小写十六进制 base commit；不能用最后的实现 HEAD 替代 base。summary/error 可为 null。
- blocked 可将未知字段设为 null，应在 error 中解释原因。
- 最多读取 2 MiB，拒绝缺失字段、额外字段、多个 JSON 值、Markdown 包装或自然语言。

Codex 通过 `exec --json --output-schema <file> --output-last-message <per-run-result> ... -` 启动，提示词经 stdin 传入。事件 stdout 与结果文件独立；结果提交不等于任务完成。

进程工作目录为调度器创建的 worktree。接收结果或人工 accept-result 时，同时核验目录对应同一 Git common-dir、仍在分配的实现分支上，且 HEAD 保留 base_commit 作为祖先；核验失败即阻塞，不接受结果中的路径或提交改写事实。blocked 结果可用 null，但提供的目录/起点同样必须与调度记录一致。

PR 命令适配器的数据结构与 result 不同，见 README 的 PR 查询部分。命令查询是可信的本机适配边界，必须直接验证平台事实。

completion-request.json 与 completion-result.json 是每次运行目录下的最近一次结项请求和回执。服务通过 stdin 传请求，读取 stdout，非零退出时也保存返回的部分写入证据；这两份文件不会包含 PR 认证 token。详情见 [结项命令协议](completion-command.md)。completion 和 beads_context 是 v2 的可空扩展字段；不要用旧二进制读取新程序写入的快照。

显式 retry 不改变 task_id/run_id/worktree/base_commit。log_path/result_path 指向本次 attempts/<attempt_id> 目录；该目录的 previous-run.json 保留上一次运行概要，原日志和结果不移动、不覆盖。重试准备状态同样先落盘，意外中断后仍变成 unknown，不能自动恢复。service.mode 是 v2 的新增可选读取字段。

## 历史与读取规则

运行记录不自动删除；日志保留策略见 README。外部读取器只能读 runtime，不写回；不能把这个文件当作队列或任务数据库。整个快照原子替换，不支持增量读取半个文件。

Windows 打开文件时建议允许 FILE_SHARE_DELETE；无法控制的读取库应重试短暂共享冲突。过期 heartbeat 意味着服务/磁盘可能异常，须显示状态过期，不能把最后一次 running 当成当前事实。重大版本改变通过 schema_version 区分。
