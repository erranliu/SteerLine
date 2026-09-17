# runtime / result 数据契约 v1

编码为 UTF-8，时间为 RFC3339 UTC。完整例子：[runtime](../examples/runtime.json)、[result](../examples/result.json)。结果的可执行 JSON Schema 与程序一起嵌入：[result.schema.json](../internal/steerline/result.schema.json)。

## 顶层

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| schema_version | integer | 当前 1；程序拒绝未知恢复版本 |
| updated_at | timestamp | 最近一次成功发布快照的时间，含心跳写入 |
| heartbeat_at | timestamp | 最近成功发布的活跃时间；读者用于判断过期 |
| service.instance_id | string | 每次启动新的随机实例 ID，不等于 claimant |
| service.pid | integer | 服务进程 PID |
| service.status | string | running / stopping / stopped / error |
| service.claimant | string | 稳定 Beads 领取者，用于恢复不完整记录 |
| service.error | string/null | 最近服务级错误，查询恢复后可能清除 |
| repository.path | string | 解析后的仓库绝对路径 |
| repository.remote / branch | string | 明确的目标远端和主线 |
| repository.identity | string | 规范化 Git common-dir，用于跨 worktree 锁与恢复校验 |
| current_task | string/null | 当前本服务启动的执行任务；没有则 null |
| slot_occupied | boolean | worker 占用，或因 unknown 旧执行而保守保留 |
| runs | array | 本服务管理的全部运行概要，初始为空数组 |

## 每项运行

| 字段 | 类型 | 含义 |
| --- | --- | --- |
| task_id / title | string | Beads 任务身份和领取时标题 |
| run_id / claimant | string | 本次随机运行 ID / 固定领取者 |
| beads_status | string | 最近核实的 Beads 状态，不以本地预测替代 closed |
| execution_status | string | pending / running / succeeded / failed / unknown |
| finish_task_status | string | pending / waiting_pr_merge / completed / blocked |
| codex_session_id | string/null | 从 thread.started.thread_id 得到，不从日志文字猜测 |
| process | object/null | PID、创建时间标识 created、executable 完整路径；均来自 OS |
| started_at / ended_at | timestamp/null | 服务记录的执行起止时间；崩溃导致终止时间未知时仍为 null |
| worktree_path / base_commit | string/null | 技能交接的隔离目录和完整 Git hash；服务不创建或切换 worktree |
| pr.url | string/null | 当前运行的精确 PR 关联 |
| pr.state | string | open / merged / closed_unmerged / unknown |
| pr.merged_at / merge_commit | timestamp/string/null | 平台核实的合并证据 |
| pr.queried_at / query_error | timestamp/string/null | 最近查询时间和错误；失败时 state=unknown |
| last_event | string/null | 最近 JSON 事件的类型摘要，最多 512 字符，不存完整对话 |
| log_path / result_path | string/null | 每次运行独立的事件和结果文件；恢复孤儿记录时可能为 null |
| summary / error | string/null | 技能结果摘要（最多 4096 字符）/ 当前阻塞或失败原因 |

`process.created` 在 Windows 为进程创建 FILETIME 的十六进制标识，Linux 为 boot ID 和 /proc starttime。PID 可复用，读取器不能仅凭 PID 判定存活。遗留进程的核验结论写入 error；unknown 不代表服务已接管或恢复该会话。

execution_status=succeeded 仅表示进程正常退出，仍可能 finish_task_status=blocked（结果缺失/无效）或 waiting_pr_merge。只有重新核实对应 PR 确实合入目标主线，且 Beads show 返回 closed，才写 completed。finish_task_status 不是“finish 技能函数是否返回”。

PR 查询失败会清除 state 的可信结论并写 unknown，但保留过去读到的合并字段用于诊断；读者必须同时看 state、queried_at 和 query_error。程序不会仅凭缓存的 merged_at 关闭任务。

## 执行结果

所有八个键都必须出现，不允许额外键：task_id、run_id、outcome、pr_url、worktree_path、base_commit、summary、error。

- task_id/run_id 必须与此次派发精确匹配。
- outcome 仅允许 pr_submitted 或 blocked。
- pr_submitted 必须提供 PR URL、绝对工作目录、40 或 64 位小写十六进制 base commit；summary/error 可为 null。
- blocked 可将未知字段设为 null，应在 error 中解释原因。
- 最多读取 2 MiB，拒绝缺失字段、额外字段、多个 JSON 值、Markdown 包装或自然语言。

Codex 通过 `exec --json --output-schema <file> --output-last-message <per-run-result> ... -` 启动，提示词经 stdin 传入。事件 stdout 与结果文件独立；结果提交不等于任务完成。

PR 命令适配器的数据结构与 result 不同，见 README 的 PR 查询部分。命令查询是可信的本机适配边界，必须直接验证平台事实。

## 历史与读取规则

运行记录不自动删除；日志保留策略见 README。外部读取器只能读 runtime，不写回；不能把这个文件当作队列或任务数据库。整个快照原子替换，不支持增量读取半个文件。

Windows 打开文件时建议允许 FILE_SHARE_DELETE；无法控制的读取库应重试短暂共享冲突。过期 heartbeat 意味着服务/磁盘可能异常，须显示状态过期，不能把最后一次 running 当成当前事实。重大版本改变通过 schema_version 区分。
