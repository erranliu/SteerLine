# Plan：收敛执行与反馈产品流程

唯一行为依据：[Spec](simplified-product-flow.spec.md)。实施顺序：环境旁路与独立命令 → 规范会话/活动 → 统一单步副作用结果交接 → worker-only 与完整消费者切换 → 最小验证 → 新 PR。

复用现有 actor/workerMessages、query、persistence、codec；不新增通用框架。记录删除/消费者与旧断言到新语义映射；展示主包、全仓生产及含测试净行数。旧行为测试改为新语义，不删除原身份、未知结果、提交屏障及真实交付断言。

## 测试计划

环境：Windows amd64，临时 STEERLINE_HOME，隔离 Git/SQLite/协议 mock/loopback provider。证据归 `bin/simplified-product-flow/`，归档摘要至 `docs/validation/simplified-product-flow/`。

| 级别/组、场景 | 命令 | 时点 | 通过条件/证据 |
| --- | --- | --- | --- |
| L1 定向：环境旁路/反馈拒绝/命令归属 | `go run ./cmd/test-feedback -out bin/simplified-product-flow/environment-final -- ./internal/steerline -run '^TestSimplifiedEnvironment|^TestEnvironmentRecheck' -count=1` | 环境消费者切换后 | 新入口实际命中；业务问题/草稿保留，拒绝/旧身份/恢复不补投，summary complete/pass |
| L1 定向：规范来源/副作用调度/发布责任 | `go run ./cmd/test-feedback -out bin/simplified-product-flow/focused-final -- ./internal/steerline ./internal/steerline/console ./internal/steerlineclient ./internal/steerlineapp -run '^TestSimplified|^TestAppServerLateReply|^TestPublicationOriginal' -count=1` | 源码稳定后 | 相关场景实际命中；数据库单一展示源、非阻塞、intent/receipt/ack、未知/过期/提交失败、worker-only 拒绝旧责任保持 |
| L1 quick | `go run ./cmd/test-feedback -level=L1 -out bin/simplified-product-flow/rebased-quick` | 最终一次 | 指定包和 parent/subtest 全部实际 run/pass，0 fail/skip；旧输入升级按断言映射更新，不放宽隔离守卫 |
| L3 controlled：delivery.basic、recovery.waiting-merge-restart | `go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=controlled -out bin/simplified-product-flow/rebased-controlled` | 完整产品切换后 | helper 在 worker 环境真实 commit/push/create loopback PR；真合并后唯一交付，重启不重发、原身份/回执/资源保留；支持 fixture 的 worker 发布能力需同步实现 |
| 最小原生执行证据 | `go run ./cmd/test-feedback -level=L3 -suite=delivery -env=live '-scenario=delivery.local-codex' -out bin/simplified-product-flow/rebased-live-local`；终端独立用 `delivery.orca-codex` / `frozen-cleanup-live-terminal`，固定 gpt-6-luna / medium，loopback provider | 原生启动调度变更后 | 保持原身份/权限、每步屏障及无重复输入；工具或环境不可用如实记限制，不用 mock 认证 |
| 构建/静态 | Windows/Linux amd64 产品构建；`git diff --check`；全 OS/tag 守卫与消费者清单 | 提交前 | 可构建、无第二调用路径、禁用依赖/悬空旧消费者为零 |

不跑 Full：范围以受影响代表、quick 和两项受控交付/恢复验证为主。不默认跑完整 L2/全部 live；原生验证只选调度改变的必要证据。没有新增模型或平台支持。

## 最终核对

- rebase 到 #201 后的 5aafff4，保留 #200 的原生修复与公开 SQLite reader 接线；不更改三线模块化或候选路线图。
- 额外定向：原保存输入、未知发送、旧 Attempt、刷新投影、晚到 terminal receipt、回执提交失败/屏障、stop/discard、原 provider 重启/remote drift。命令及命中数量见验证摘要。
- 当前 actor 暂存同一 run 的观察消息，等其原副作用回执提交后消费，避免 opaque envelope 覆盖。其它 run 和控制入口继续处理。actor 退出后的最终自有进程收尾使用同一个后台结果路径并等待结果，不向已关闭的消息循环投递。
- 全仓编译入口为 `go run ./cmd/test-feedback -build-only -out bin/simplified-product-flow/rebased-build-final`，不接受 `--` 后额外参数。Windows/Linux CLI 构建与最终静态检查独立记录。

最终清理进度改动后，单独执行 frozen-cleanup-unit-final、cleanup-progress-final、boundary-final 与 stop-binding-final。测试均 complete/pass；冻结 guard 不改，具体命令见 validation.json。
