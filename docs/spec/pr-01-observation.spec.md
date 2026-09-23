# PR 1 Spec：统一观察层与读取一致性

状态：待实现。依赖：无。对应 [Plan](pr-01-observation.plan.md)。

## 目标

让 viewer 和流式 watch 共享可靠的状态与日志读取规则，保证用户始终在查看所选运行及所选 attempt。该 PR 交付后，现有 viewer 的信息可靠性可以单独验收。

## 功能要求

### O1：项目快照

- 读取接口返回完整 Runtime 及读取状态，包含仓库、service、heartbeat、current_task、slot 和 runs；不再只返回 `[]Run`。
- 区分无快照、有效快照、读取失败、未知版本和快照过期。读取失败时可保留最后一次成功内容，但必须显著标记其采样时间和错误。
- 默认过期阈值为 `max(3 × config.heartbeat, 15s)`；测试使用可注入时钟。未来时间戳标记时钟异常，不能借此永久判为新鲜。
- runtime 中的 running 是服务上次报告的状态，heartbeat 新鲜度单独呈现。本 PR 不凭 PID 宣称服务仍存活；PR 3 增加实例探测。
- 新读取层容忍 runtime v1/v2/v3；不写 runtime、不启动服务、不执行任务提供方的写操作。

### O2：一个日志读取引擎

- viewer 与 watch 共用逐条读取引擎，默认优先 `activity.jsonl`，缺失时归一化 `events.jsonl`。`--raw` 保持原始行输出。
- cursor 至少关联完整路径、文件身份、读取代次和已消费字节；不能只用 offset 或 run_id。
- 只消费以换行结束的完整记录；末尾半行留待补齐。offset 按实际字节计算，覆盖 UTF-8 和 CRLF。
- 检测文件截断及同路径原子替换，包括新文件等长/更大；重开后返回 reset 信号，让视图清空当前日志代次，不能拼接新旧文件。
- 同一 activity 文件代次中按 seq 去重；同 seq 的重复记录跳过，新完整记录的 seq 严格回退时从该记录开始新逻辑代次并通知视图 reset。此时文件 offset 继续向前，不能反复回到同一旧前缀造成死循环。不同 run、attempt 或文件代次的相同 seq 互不冲突。
- raw fallback 使用有状态 normalizer。重开或切源时重置 normalizer；`from-start=false` 也要正确识别原始宿主格式。
- activity 后续出现时，以新的读取代次切换到它；viewer 替换 fallback 视图。流式 watch 在一次调用中固定已选源，避免输出重复历史；再次调用选择新源。
- 每批最多 256 条或约 1 MiB；单条上限 8 MiB。单条可独占一批；超长/非法记录需产生读取诊断并推进到下一完整行，不能无限占内存或卡住后续有效记录。
- 非法 JSON 的诊断规则针对 normalized activity；raw fallback 的未知内容仍由 normalizer 产生 unknown 事件，`--raw` 对上限内完整行原样输出，不以 JSON 校验过滤原始文本。
- viewer 内存保留最多 4000 条事件；`watch --from-start` 可以逐批读完全部历史。初次打开大文件不能把全量事件一次装进内存。

### O3：异步读取与身份

- 当前选择保存为 run_id；重新排序、插入待决策运行、刷新出错均不改变已存在的选中对象。首次进入才按 pending 决策优先规则选择。
- 日志身份为 `(run_id, log_path, generation)`。同 run 的 retry 改变 log_path 时清空 offset 和旧视图，加载新 attempt。
- 每个日志流至多一个请求在途；响应带请求代次和起始 cursor。切项目、切运行、切路径或更晚响应已应用后，丢弃过期响应。
- 选到没有日志的记录时显示空态，不能残留上一项日志。选中 run 消失时选择明确的相邻项并清空详情。
- 滚动与跟随分开：向上翻页退出自动跟随；恢复跟随跳到最新。后台可继续有界收集日志，用户滚动位置以事件锚点保持。

### O4：流式兼容

- watch 一次调用固定一个选定 run 和该次解析到的日志路径，不自动切换到其他任务或重试 attempt；界面提示其身份。
- TTY 下显式 `--follow=false` 或 `--from-start=false` 走流式模式，使参数生效。`--json`、`--raw`、`--no-tui` 继续强制流式。
- 稳定 activity JSONL 字段、未知 kind 的容忍规则和离线读取保持兼容。读取诊断写 stderr，不能污染 JSON stdout。

## 验收标准

| ID | 场景与结果 |
| --- | --- |
| O-A1 | 正在选择 B 时 A 变为 pending 决策并置顶，仍选中 B，日志不变 |
| O-A2 | 同 run 切换 attempt 路径，从新文件头/所选起点加载，不沿用旧 offset |
| O-A3 | 一条 UTF-8 JSON 被分两次追加且最后才写换行，最终显示一次完整事件 |
| O-A4 | 文件缩短、同长度替换、变大替换均检测并 reset，不串入旧代次内容 |
| O-A5 | A→B→A 快速切换并人为倒序返回读取结果，无重复、丢尾或错配日志 |
| O-A6 | activity 缺失可看 raw fallback；不存在任何日志时显示正确空态 |
| O-A7 | 快照读取失败保留有时间标记的旧视图；不会显示假“实时正常” |
| O-A8 | 大文件持续输出时内存有界，末尾坏记录不影响后续合法行 |
| O-A9 | TTY 下 follow=false 可结束；JSON/raw stdout 不包含状态提示或诊断 |

## 不包含的交付

任务启动、派发模式、后台进程启停、决策协议改造在后续 PR 中实现。本 PR 不改变 runtime schema 或任务状态机。
