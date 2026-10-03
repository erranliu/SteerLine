# #72 TUI 与中心实现解耦交付与验证

执行日期：2026-10-04（Asia/Shanghai）。worktree `C:/Users/lmas1/.codex/worktrees/773a/SteerLine`，分支 `codex/72-tui-client`，执行模型 `gpt-6.1-sol / high`。起始 detached HEAD 与发布前 fetch 均核对为 #71 PR195 / #69 PR194 实际主干 `6dce3cf1abf293e18bd78f92677c72a3fa2f0210`；只完成 #72。

## 最终所有者与实际路径

- `clientcontracts`：项目纯引用、快照、小接口、非秘密方案摘要/固定 digest/支持原因与只读恢复报告。复用 contracts/execution；不引用 Config、Service、中心 Runtime 类型、数据库、平台或组装。候选与自动派发的既有输入 guard 从原 UI/adapter 迁到唯一纯产品所有者，经 InputChecks 消费，中心仍执行最终资格与版本校验。
- `steerlineclient`：唯一 CLI/TUI 产品 adapter；每次产品读取/命令沿原 QueryService、CommandService、文件 control/answer 端口。恢复核验调用原只读中心用例，游标调用原 query codec。项目展示摘要在 Open/Project 时深复制。没有第二客户端、查询、规则或业务写入入口。
- `steerlineapp`：共享 `openProjectClient` 唯一构造；`PrepareConsole` 在 UI 外加载配置/身份、保留无效项目和同仓配置冲突、按固定配置/状态路径打开每项目一个 adapter 并注入。服务窗口文件探测、回答者环境默认值、方案编译/解析与执行目录支持检查都由产品资源构造提供。
- `console`：输入、导航、任务/会话选择、滚动/折叠、正文版本/字节预算/Unicode、阅读锚点、请求和决策草稿、偏好。只持有按项目注入的产品客户端；构造前拒绝缺失、重复 view key 或错项目客户端。无效配置中恢复的待查请求继续保留原 ID 并返回错误。偏好锁/原子文件写入属于 UI，格式、键、Windows/Linux OS 锁与隔离语义保持。
- `activityview`：原活动格式化实现整体迁入唯一纯展示所有者；console 与原中心/CLI 渲染入口共用，不复制格式化实现。中心保留原公开渲染入口，UI 不经中心调用。
- `Model.Close` 与外层退出清理：只释放偏好及客户端资源；不发 stop_service、不终止 service/worker。原操作 ID 在提交前持久化，错误仅查原回执；indeterminate 不表示成功或产生新 ID。

## 删除清单与并行交集

删除 `FileQuerySource/FileControlClient`、原 `QuerySource/ControlClient/DecisionCommandClient`、`productRef/withClient/candidateFreshness` 映射、`ProjectRef.Config`、`ConfigSource/ProjectConfigFile` 及 NewModel 的默认构造。原 LoadProjects/身份解析迁到外层；console 的中心类型别名、方案 ResolveExecutionProfile/ExecutionDirectory 消费、直接 InspectRecovery、游标编码、服务窗口 stat、回答者环境解析以及中心 RepoLock/AtomicWrite 依赖均删除。

全体 console 生产/测试源码只通过纯产品端口/共享 DTO 读取与提交；实际依赖守卫覆盖 30 个 Go 文件、所有 OS/tag 的测试/生产和 helper 图，禁止 root、SQL、存储/写端口、query/projection、具体平台、workspace/publication、默认 adapter、app/integration。Windows 164 与 Linux 155 个生产依赖均无禁用项；原 #67 的文本、根包/契约图和反向依赖检查不放宽。负向 fixture 对生产、测试及传递 helper 实际拒绝，接到既有 quick。

与 #70 的构造/测试交集：`steerlineapp/client.go` 只增产品摘要、恢复及支持能力闭包，共享 reader/command/store 构造保持原唯一实现；`steerlineapp/console.go` 属本项 console 构造段；`client_project_test.go` 保留 #69 BatchTx 和损坏数据库 fixture，只改产品消费者；`cmd/steerline/native_task_test.go` 复用既有真实 Service checkpoint/control 夹具；`coreboundary/check.go`、`cmd/test-feedback/scoped.go` 扩展 console 守卫。根包 `recovery.go` 只把公开只读报告定义移到产品纯包，`render.go` 只迁纯格式化实现。未修改 Store、写端口、领域记录或 contracts 既有字段。发布前 main 未出现新的已合并 #70 接线；没有将 #70 作为业务前置或保留第二构造。

## 测试映射与非零命中

[test-mapping.json](test-mapping.json) 列原 console 的 58 个顶层测试逐项旧路径、新路径与 run/pass 次数；全部非零。唯一迁包 `TestLoadProjectsSupportsZeroPartialAndExplicitConfigs` 转到 app，仍断言零配置、部分损坏、正确身份、外部显式路径与错误行键。UI 不再使用执行目录/codec/config 注册 fake，改用中性产品摘要与 typed fake；方案编译/固定摘要由原 app profile 与真实 CLI/TUI 接线断言覆盖。

代表断言原名保留：导航/选中与慢读；会话轮次与阅读锚点；Unicode 粘贴/字节页/正文版本；长方案列表/固定方案/配置不匹配；决策版本/草稿/稳定 command ID；控制 ID 持久化失败、未知原请求只查与异内容拒绝；独立窗口偏好与重开历史轮次。新增只覆盖明确注入/错误身份/退出和 console 依赖缺口。

## 实际验证

环境：Windows amd64 / Go 1.26.4 / 本地 Git、SQLite、文件控制，未调度 worker 或请求模型。原始日志目录：`C:/Users/lmas1/AppData/Local/Temp/steerline-cm72-evidence-96d606f608584c8d80258006444f65ed`。逐组命令、次数与日志 SHA-256 在 [test-results.json](test-results.json)。

| 命令 | 结果 |
| --- | --- |
| `go test -json ./internal/steerline/console -count=1` | 58 顶层 / 62 run-pass，0 fail/skip |
| `go test -json ./internal/steerlineclient -count=1` | 11 顶层 / 21 run-pass，0 fail/skip |
| `go test -json ./internal/steerlineapp -run 'Test(CM71\|FileQuerySource\|CLIExecutionProfile\|LoadProjects\|CD66ExplicitCatalogAndEntrypoints)' -count=1` | 5 顶层 / 11 run-pass，0 fail/skip |
| `go test -json ./cmd/steerline -run '^TestNativeTaskBatchCLI' -count=1` | 1 顶层 / 3 run-pass，真实 CLI/TUI 接线子用例通过 |
| `go test -json ./internal/steerlineintegration -run '^TestCD(67CoreBoundary\|66ExplicitCatalogAndEntrypoints)$' -count=1` | 2 顶层 / 26 run-pass，全 OS/tag + Windows/Linux 图通过，0 fail/skip |
| `go run ./cmd/test-feedback -quick -out <证据目录>/quick` | 稳定后一次：7 组 complete/passed，155 run-pass，0 fail/skip |
| `git diff --check` / staged check | 通过 |

真实接线子用例构造现有正式 Service 身份/checkpoint，复用原 CLI request/indeterminate receipt，由 `PrepareConsole` 注入实际 `*steerlineclient.Client`；断言相同受理配置及 profile digest、深复制摘要、原回执仍 indeterminate、实际项目读取与原恢复只读端口、q 只产生 QuitMsg、Close 后原 service 仍 connected 且 UI client 已关闭。原同 ID 异内容/外项目回执拒绝及请求字节不变断言保留。该测试不以中性 fake 替代默认接线。

未选 L2/L3/full：不改变执行、交付、恢复授权或原生权限映射；已有 quick、真实产品接线及代表交互覆盖此次边界。UI 无 redesign，不增加视觉矩阵或截图。

## 自查与交接

已自行读取完整实际 diff、Spec/Plan、消费者及生产路径；自查修复窗口提示残留文件探测、错项目注入、无效配置待查请求和产品摘要共享 slice。配置/支持原因依然由原实现所有者计算，不解析原生封套、不暴露写者。测试后只补验证文档，生产与测试源码保持验证时版本。无已知未解决问题。一个实现 PR，提交推送后停止写入，等待协调聊天独立复审；不合并、不启动其他任务或 automation。

## 协调聊天独立复审

2026-10-04 协调聊天核对本项 Spec/Plan、完整实际 diff、产品注入/CLI/TUI/恢复/游标/原请求/偏好与退出路径，以及测试迁移。原 console 失败断言文字对照表保存在本 run 的 cm72-test-assertion-message-audit.json；配置枚举的六项随同名测试迁入 app，配置/身份的 UI fixture 改为中性纯值，真实 app/profile/CLI 接线仍核对实际受理摘要。

发现并在 `01e20a8cc452c672a9c1c6cf8f5300c86deeb680` 修复：不启用构建标签下的 console 直接 SQLite 驱动导入可逃过源码守卫。已加入 modernc.org/sqlite 及子路径禁用，现有生产/测试负向 fixture 增加同一驱动样本；新增反例先实际失败，修复后完整 CoreBoundary 通过，不放宽旧 #67 规则。

协调聊天实际验证：UI/client 69 顶层 / 83 run-pass；app/CLI/既有接线与守卫 9 顶层 / 44 run-pass；修复后完整 CoreBoundary 1 顶层 / 22 run-pass，均 0 fail/skip。最终 quick 7 组 complete/passed、156 run-pass，0 fail/skip，源码为上述修复 commit；之后仅更新文档。

首次 quick 的终端恢复场景业务断言已完成，但 Windows 临时目录清理因 SQLite 文件占用失败。Service/SQLite/原 lifecycle 测试不在本项改动范围，已核对正常停止及句柄关闭路径；独立运行原场景业务和清理全部通过，随后完整 quick 全部通过。首次文件占用来源未确认，原失败和诊断日志保留；没有改动清理断言、忽略失败或授予例外。

精确命令/原始 JSON/最终 quick 位于 `C:/Users/lmas1/.codex/task-state/roadmap-runner/steerline-core-decoupling-20261003-01a10252` 的 cm72-coordinator-*.jsonl、cm72-terminal-cleanup-diagnostic.jsonl、cm72-reviewed-quick/（首轮失败）与 cm72-reviewed-quick-final/（最终通过）。git diff --check、修改文档的本地链接检查通过；无已授权验收例外。
