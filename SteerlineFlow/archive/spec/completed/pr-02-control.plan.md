# PR 2 Plan：服务控制协议与定向执行

实施状态：已实现，完成协调审核修复。Windows 最终验证：`go test ./... -count=1 -timeout 10m` 通过（steerline 306.106s）。

当前实现已增加 runtime v4、控制协议/客户端/回执、候选快照、定向执行、manual/auto、暂停/恢复、停服取消和 v1–v3 迁移。定向证据位于 `internal/steerline/control_test.go`、`service_test.go`、`workspace_test.go` 与 `cli_test.go`。

控制专项证据覆盖：真实子进程竞争同一 request ID 且不同内容不可覆盖；fake Beads claim/start 计数；manual 多 tick、仅启动 B、同 ID 重扫及不同请求抢槽；心跳与候选刷新不推进 revision；checkpoint、claim、worktree、worker、终态 checkpoint/receipt 崩溃恢复；applied-receipt 写失败不派发、终态回执失败阻塞并从 checkpoint 重建；stop 取消长准备命令；旧实例/过期/旧 revision/错误仓库拒绝；v1–v3 只读不迁移与持锁保存迁移；原生 CLI 候选、控制回执、stop_service 和旧 stop 路径。

依据：[Spec](pr-02-control.spec.md)。依赖：PR 1。

协调审核补充了 `control_review_test.go` 中的 21 个回归场景，覆盖请求路径与 payload、准入过期、停服受理事实、回执故障保持可控、执行槽/决策/重试守卫、恢复证据和版本范围。修复后 `go test ./... -count=1 -timeout 10m` 全量通过（Windows 服务包 306.106 秒）；Linux amd64 编译、文档链接和 JSON 样例检查通过。未操作真实 Beads/Orca 或生产状态目录。

## 实施步骤

1. **先提取派发原语。** 从 `Service.dispatch` 分离候选刷新、统一执行槽守卫、选定任务的 fetch/claim/prepare/start。自动 dispatch 保留原选择行为；定向入口严格匹配 ID 并重新查询 ready。用现有 fake Beads/worker/Orca 验证提取前后的一致性。
2. **实现模式与状态版本。** 修改 `model.go`、`store.go`、`service.go`，加入 v4 字段和受控的版本推进方法。把心跳和 activity 更新排除在 control revision 之外。通过统一函数维护 slot/current_task，包含 waiting_decision 与未确定 operation 的保留。
3. **实现指令发布与回执。** 新增 `control_protocol.go`、`control_client.go`、`control_store.go`；客户端负责请求 ID、只发布一次与回执查询，服务端严格解码并检查仓库/实例/期限/版本。为同 ID 发布增加跨进程互斥/不可覆盖语义，不能用普通覆盖写冒充幂等。
4. **实现串行处理与 checkpoint。** 在服务主循环加入指令扫描/调度；所有变更在一个状态机中执行。先持久化 operation，再执行外部动作；完成 checkpoint 与终态回执分阶段发布。启动恢复必须先处理未完成 operation，再允许常规自动 dispatch。
5. **保持 stop 可取消。** 把新 stop 请求接到现有取消通道，避免主循环长时间等待 Git/Orca 时不能退出。确认取消中途的 active operation 与 stop 各自有正确回执；停止不能伪装成启动成功。
6. **提供候选与 CLI 接口。** 持锁服务发布 `tasks.json`，读取层返回缓存/错误状态。`main.go` 加 `-dispatch`、control 动作入口、按 ID 查询回执；参数验证与服务内动作验证共用类型。
7. **更新契约。** 更新 `docs/data-contract.md`、`docs/state-model.md`、`docs/activity.md`、README 和 `examples/runtime.json`。说明 v4 不支持旧二进制降级、pause 的边界和指令成功的含义；控制指令审计独立于 activity。

## 关键实现约束

- `NewService` 有恢复和目录副作用，控制客户端与读取层不能调用它来“检查状态”。
- 既有 Store 会深拷贝 snapshot，继续保持；不要让心跳 goroutine 访问正在修改的 runs。
- 指令接收快照、任务列表刷新与自动 dispatch 可能相邻发生。以服务执行时的 guard 和真实 claim 仲裁为准，不把客户端看到的 ready 当作事实。
- 两个锁（Git common-dir、run_dir）的覆盖范围延续到指令处理和回执恢复；不能为了在线操作另建第二个 Service 写同一文件。
- operation 不确定且没有完整 run 的情况必须有可展示的恢复身份和阻塞原因，PR 5 才能处理它。

## 验证

- 新增协议单测：不可覆盖发布、同 ID 内容比较、版本/过期/实例校验、回执状态及坏输入。
- 用现有 `newFixture` 的 Claims/Starts 计数验证 C-A1～C-A5、C-A7；使用两客户端进程测试文件层并发，不能仅用 goroutine mock 代替。
- 加入可注入故障点，逐项覆盖 C-A6：副作用前、claim 后、worktree 后、worker 身份保存后、终态 checkpoint 后、回执前。重建服务后断言没有第二次 claim/start/send。
- 对卡住的假 Git/Orca/完成命令发 stop，验证取消、进程树收尾和回执状态。
- 用 v1/v2/v3 fixtures 验证迁移和只读不迁移；使用现有 CLI 默认 run 测试保证兼容。
- 执行控制模块定向测试，然后运行 `go test ./... -timeout 10m`。若超时，定位到具体测试并记录原因，不能把超时当通过；该 PR 影响核心调度，完整回归是合并条件。

## 完成条件

- C-A1～C-A10 有覆盖，CLI 能完整演示“manual 服务 → 查询候选 → start 指定 B → 查询回执 → pause/stop”。
- 崩溃恢复的确定/不确定结果有可检查文件证据。
- 请求内容、实例、run/attempt 和回执可相互追踪；协议文档与实际 JSON 一致。

## 风险与回退

这是首次写 runtime v4 的 PR。升级前停止旧服务并保留原快照；v4 运行后不能用旧二进制覆盖它。若需撤销，使用能够读取 v4 的修复版本；恢复旧快照仅适用于没有发生新 claim/执行副作用的场景，不能把备份恢复作为通用回退。
